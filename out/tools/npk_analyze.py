# -*- coding: utf-8 -*-
"""
npk_analyze.py — NXPK 数据块字节统计分析
目的：验证数据层加密是否为"固定密钥流"（同包内所有条目共享同一 keystream）
方法：
  1. 解析 npk 索引层（头部 NXPK + field@20=索引偏移；条目 28 字节：
     hash(4) offset(4) size(4) csize(4) hash64(8) flag(4)）
  2. 取样多个数据块，按字节位置统计密文取值分布
  3. 若某位置在所有块中恒定 → 明文恒定或 key 恒定；结合 marshal 头假设可恢复 key 前缀
输出：每个位置的 (恒定值/变化数) 统计表
"""
import struct
import sys
from collections import Counter


def parse_npk(path):
    """历史兼容：size=存储长度、csize=解压长度，禁止按字面解释。

    新代码使用 npk_unpack.parse_archive 的 stored_size/decoded_size。
    """
    with open(path, 'rb') as f:
        data = f.read()
    magic = data[:4]
    assert magic == b'NXPK', 'bad magic %r' % magic
    version = struct.unpack_from('<I', data, 4)[0]
    index_off = struct.unpack_from('<I', data, 20)[0]
    # PC 样本有索引后追加块，条目数只能读取头部 +4，不能从 EOF 推算。
    n = version
    if index_off + n * 28 > len(data):
        raise ValueError('索引区越界')
    entries = []
    for i in range(n):
        off = index_off + i * 28
        h, o, s, cs = struct.unpack_from('<IIII', data, off)
        h64 = struct.unpack_from('<Q', data, off + 16)[0]
        flag = struct.unpack_from('<I', data, off + 24)[0]
        entries.append(dict(hash=h, offset=o, size=s, csize=cs, hash64=h64, flag=flag))
    return version, entries, data


def main(path, sample=40):
    version, entries, data = parse_npk(path)
    print('file=%s version=0x%x entries=%d' % (path, version, len(entries)))
    # flag 分布
    fc = Counter(e['flag'] for e in entries)
    print('flag distribution:', dict(fc))
    # csize vs size（判断是否压缩）
    same = sum(1 for e in entries if e['csize'] == e['size'])
    smaller = sum(1 for e in entries if e['csize'] < e['size'])
    bigger = sum(1 for e in entries if e['csize'] > e['size'])
    print('csize==size: %d, csize<size: %d, csize>size: %d' % (same, smaller, bigger))

    # 取样（取 csize 最小的前 sample 个块，小文件更可能是短脚本）
    es = sorted(entries, key=lambda e: e['csize'])[:sample]
    maxlen = min(min(e['csize'] for e in es), 64)
    pos_vals = [Counter() for _ in range(maxlen)]
    for e in es:
        blob = data[e['offset']:e['offset'] + maxlen]
        for i, b in enumerate(blob):
            pos_vals[i][b] += 1
    print('\n== per-position ciphertext distribution (first %d bytes, %d smallest blocks) ==' % (maxlen, sample))
    for i in range(maxlen):
        c = pos_vals[i]
        if len(c) == 1:
            v = next(iter(c))
            print('pos %3d: CONST 0x%02x' % (i, v))
        else:
            top = c.most_common(3)
            print('pos %3d: VAR   n=%d top=%s' % (i, len(c), ['0x%02x x%d' % (v, n) for v, n in top]))

    # 首块明文展示
    print('\n== sample blocks hex (first 3) ==')
    for e in es[:3]:
        blob = data[e['offset']:e['offset'] + min(e['csize'], 48)]
        print('hash=0x%08x size=%d csize=%d hash64=0x%016x' % (e['hash'], e['size'], e['csize'], e['hash64']))
        print('  ', blob.hex())


if __name__ == '__main__':
    p = sys.argv[1] if len(sys.argv) > 1 else r'.\work\apk\assets\script.npk'
    s = int(sys.argv[2]) if len(sys.argv) > 2 else 40
    main(p, s)
