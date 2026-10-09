# -*- coding: utf-8 -*-
"""
npk_known_plain.py — 已知明文攻击恢复 NXPK XOR 密钥流 K
原理：
  块明文 = Python2.7 marshal code 对象（'c'=0x63 开头，头部 17 字节多为小整数 LE）
  加密 = plain ^ K（K 全包固定，每块从 0 开始）
  游戏进程堆里残留解密后的 marshal 缓冲 → 内存模式搜索
  对齐验证：K[2]=0x19, K[3]=0x73, K[4]=0x88（由全量统计+marshal 结构确认）
输出：out/keys/keystream.bin（恢复的 K 前缀）+ 对应文件列表
"""
import re
import sys
import os
from collections import Counter

sys.path.insert(0, r'.\out\tools')
from npk_analyze import parse_npk

K_CHECK = {2: 0x19, 3: 0x73, 4: 0x88}  # 已知 K 字节（高置信）

# marshal code 头: 'c' argcount(4LE) nlocals(4LE) stacksize(4LE) flags(4LE)
# 常见小模块：高位字节=0 → 模式 63 ?? 00 00 00 ?? 00 00 00 ?? 00 00 00 ?? 00 00 00
HEAD_RE = re.compile(
    b'\x63.\x00\x00\x00.\x00\x00\x00.\x00\x00\x00.\x00\x00\x00', re.DOTALL)


def load_ciphers(paths):
    """加载所有 npk 的 (hash, cipher bytes)"""
    out = []
    for p in paths:
        v, entries, data = parse_npk(p)
        for e in entries:
            if e['size'] and e['size'] < 0x7fffffff and e['csize'] >= 32:
                c = data[e['offset']:e['offset'] + e['csize']]
                out.append((e['hash'], c))
    return out


def main():
    mems = [r'.\work\memdump\mem_engine.bin', r'.\work\memdump\mem_native.bin']
    npks = [r'.\work\apk\assets\script.npk', r'.\files\netease\h62\Documents\script.npk']

    ciphers = load_ciphers(npks)
    print('loaded %d ciphers' % len(ciphers))

    # 预建 hash 前缀映射: cipher 前8字节（前缀匹配候选明文用全部 ciphers 逐个试太慢）
    # 优化：把 ciphers 按前 5 字节建索引? K[1]、K[5] 变化 → cipher[1] = plain[1]^K1 变化
    # 直接做法：对每个明文候选 P，计算 K_cand[0]=P[0]^0xf0? 不，cipher[0] 全部 =0xf0（因为 plain[0]='c' 恒定）
    # K0 = 0x63 ^ 0xf0 = 0x93。同理 K2/K3/K4 检查位已知。
    # 逐个 cipher 对齐验证 K[2..4] 三个字节 → 每 P 需对 ciphers 全扫（3112 × 候选数）
    # 候选数可能几千，3112×几千 = 千万次 3 字节比较，Python 太慢 → 用 numpy 思路：向量化

    import numpy as np
    C = np.zeros((len(ciphers), 64), dtype=np.uint8)
    for i, (h, c) in enumerate(ciphers):
        n = min(64, len(c))
        C[i, :n] = np.frombuffer(c[:n], dtype=np.uint8)

    keystream_votes = {}   # pos -> Counter(byte)
    matched = []
    total_cand = 0
    for mempath in mems:
        with open(mempath, 'rb') as f:
            mem = f.read()
        for m in HEAD_RE.finditer(mem):
            p = m.start()
            P = np.frombuffer(mem[p:p + 64], dtype=np.uint8)
            # 候选 K 向量: K[i] = P[i] ^ C[:, i]
            Kc = P[None, :] ^ C  # (ncipher, 64)
            # 验证 K[2]=0x19 K[3]=0x73 K[4]=0x88
            ok = (Kc[:, 2] == 0x19) & (Kc[:, 3] == 0x73) & (Kc[:, 4] == 0x88)
            idxs = np.nonzero(ok)[0]
            total_cand += 1
            for j in idxs:
                h, c = ciphers[j]
                n = min(len(c), 64)
                for pos in range(n):
                    keystream_votes.setdefault(pos, Counter())[int(Kc[j, pos])] += 1
                matched.append((h, p, mempath, len(c)))
        print('scanned %s: total marshal-head candidates=%d' % (os.path.basename(mempath), total_cand))

    print('\nmatched (plain, cipher) pairs: %d' % len(matched))
    for h, p, mp, ln in matched[:20]:
        print('  hash=0x%08x len=%d  @%s+0x%x' % (h, ln, os.path.basename(mp), p))

    # 投票合成 K
    K = bytearray()
    pos = 0
    while pos in keystream_votes:
        c = keystream_votes[pos]
        best, cnt = c.most_common(1)[0]
        K.append(best)
        pos += 1
    print('\nrecovered keystream length=%d' % len(K))
    print('K head:', K[:32].hex())
    os.makedirs(r'.\out\keys', exist_ok=True)
    with open(r'.\out\keys\keystream.bin', 'wb') as f:
        f.write(bytes(K))
    print('saved -> out/keys/keystream.bin')


if __name__ == '__main__':
    main()
