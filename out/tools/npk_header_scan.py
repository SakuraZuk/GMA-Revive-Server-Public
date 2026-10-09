# -*- coding: utf-8 -*-
"""
npk_header_scan.py — 全量扫描块头部结构，找字段规律
取反视角下头部 = 0f [00...尾字节] e68c [body]
统计：varint段长、尾字节值 vs (size, csize, size-csize, hash) 的相关性
"""
import struct
import sys

sys.path.insert(0, r'.\out\tools')
from npk_analyze import parse_npk


def main(path):
    version, entries, data = parse_npk(path)
    rows = []
    bad = 0
    for e in entries:
        if e['csize'] < 8 or e['size'] > 0x7fffffff:
            continue
        raw = data[e['offset']:e['offset'] + min(e['csize'], 96)]
        inv = bytes(b ^ 0xFF for b in raw)
        if inv[0] != 0x0f:
            bad += 1
            if bad <= 3:
                print('bad head: hash=0x%08x inv=%s' % (e['hash'], inv[:8].hex()))
            continue
        # 从 pos1 找 e68c
        idx = inv.find(b'\xe6\x8c', 1, 40)
        if idx < 0:
            bad += 1
            if bad <= 6:
                print('no e68c: hash=0x%08x inv=%s' % (e['hash'], inv[:24].hex()))
            continue
        varint = inv[1:idx]  # 不含 e68c
        rows.append((e['hash'], e['size'], e['csize'], len(varint), varint.hex(), inv[idx + 2:idx + 6].hex()))
    print('parsed %d blocks, bad %d' % (len(rows), bad))
    # varint 长度分布
    from collections import Counter
    lc = Counter(r[3] for r in rows)
    print('varint len dist:', dict(sorted(lc.items())))
    # varint 长度 vs csize 关系（分段）
    print('\nlen  n    csize_min  csize_max  size_min  size_max')
    for ln in sorted(lc):
        sel = [r for r in rows if r[3] == ln]
        print('%3d  %4d  %8d  %9d  %8d  %9d' % (
            ln, len(sel),
            min(r[2] for r in sel), max(r[2] for r in sel),
            min(r[1] for r in sel), max(r[1] for r in sel)))
    # 尾字节 vs size 低字节？（取 varint 长度=2 的样本细看）
    print('\nsample varint len==2:')
    for r in [r for r in rows if r[3] == 2][:12]:
        print('hash=0x%08x size=%5d csize=%5d varint=%s body=%s' % (r[0], r[1], r[2], r[4], r[5]))
    print('\nsample varint len==1:')
    for r in [r for r in rows if r[3] == 1][:12]:
        print('hash=0x%08x size=%5d csize=%5d varint=%s body=%s' % (r[0], r[1], r[2], r[4], r[5]))


if __name__ == '__main__':
    p = sys.argv[1] if len(sys.argv) > 1 else r'.\work\apk\assets\script.npk'
    main(p)
