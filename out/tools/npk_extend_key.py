# -*- coding: utf-8 -*-
"""
npk_extend_key.py — 扩展密钥流 K 到全长
方法：重新扫描内存 marshal 残片，对每个命中 (明文P, 密文C) 对，
取 min(len(C), 16384) 长度计算 K 候选；前缀须与已恢复 K 一致才采纳；
逐字节投票合成最长 K。
"""
import re
import sys
import os
from collections import Counter

import numpy as np

sys.path.insert(0, r'.\out\tools')
from npk_analyze import parse_npk

K_SEED = bytes.fromhex('93311973883021c26e5687f6c70ff1c10808e55a3071b6cba35c43e3cc6322ef')
K_CHECK = {2: 0x19, 3: 0x73, 4: 0x88}
WINDOW = 16384

HEAD_RE = re.compile(
    b'\x63.\x00\x00\x00.\x00\x00\x00.\x00\x00\x00.\x00\x00\x00', re.DOTALL)


def main():
    mems = [r'.\work\memdump\mem2_engine.bin', r'.\work\memdump\mem2_native.bin']
    npks = [r'.\work\apk\assets\script.npk', r'.\files\netease\h62\Documents\script.npk']

    ciphers = []
    for p in npks:
        v, entries, data = parse_npk(p)
        for e in entries:
            if e['size'] and e['size'] < 0x7fffffff and e['csize'] >= 32:
                c = data[e['offset']:e['offset'] + e['csize']]
                ciphers.append((e['hash'], c))
    print('ciphers: %d' % len(ciphers))

    C = np.zeros((len(ciphers), 64), dtype=np.uint8)
    for i, (h, c) in enumerate(ciphers):
        n = min(64, len(c))
        C[i, :n] = np.frombuffer(c[:n], dtype=np.uint8)

    votes = {}
    hits = []
    for mempath in mems:
        with open(mempath, 'rb') as f:
            mem = f.read()
        for m in HEAD_RE.finditer(mem):
            p = m.start()
            P64 = np.frombuffer(mem[p:p + 64], dtype=np.uint8)
            Kc = P64[None, :] ^ C
            ok = (Kc[:, 2] == 0x19) & (Kc[:, 3] == 0x73) & (Kc[:, 4] == 0x88)
            for j in np.nonzero(ok)[0]:
                h, c = ciphers[j]
                n = min(len(c), WINDOW, len(mem) - p)
                # 完整窗口 K 候选
                Pw = mem[p:p + n]
                Kfull = bytes(a ^ b for a, b in zip(Pw, c[:n]))
                # 前缀一致性检查（与 K_SEED 64B 或当前投票共识比较）
                if Kfull[:len(K_SEED)] != K_SEED:
                    # 找最长一致前缀，截断采纳
                    ok_len = 0
                    for a, b in zip(Kfull, K_SEED):
                        if a != b:
                            break
                        ok_len += 1
                    if ok_len < 32:
                        continue
                    Kfull = Kfull[:ok_len]
                votes.setdefault(len(Kfull), []).append(Kfull)
                hits.append((h, p, os.path.basename(mempath), n))

    print('hit pairs: %d' % len(hits))
    # 每个长度层级投票
    K = bytearray(K_SEED)
    best_len = len(K_SEED)
    level_stats = sorted(votes.items())
    # 逐位置投票（合并所有层级）
    pos_votes = {}
    for _, ks in level_stats:
        for k in ks:
            for i, b in enumerate(k):
                pos_votes.setdefault(i, Counter())[b] += 1
    # 找最长连续一致段（票数最高的字节值序列）
    maxpos = max(pos_votes) + 1 if pos_votes else 0
    out = bytearray()
    for i in range(maxpos):
        c = pos_votes.get(i)
        if not c:
            break
        best, cnt = c.most_common(1)[0]
        total = sum(c.values())
        if cnt * 2 <= total:  # 无多数 → 停
            print('vote split at pos %d: %s' % (i, c.most_common(3)))
            break
        out.append(best)
    if len(out) > len(K):
        K = out
    print('final keystream length=%d' % len(K))
    print('head 96B:', K[:96].hex())
    with open(r'.\out\keys\keystream.bin', 'wb') as f:
        f.write(bytes(K))
    print('saved -> out/keys/keystream.bin')
    # 覆盖率统计
    covered = sum(1 for h, c in ciphers if len(c) <= len(K))
    print('blocks fully covered by K: %d / %d' % (covered, len(ciphers)))


if __name__ == '__main__':
    main()
