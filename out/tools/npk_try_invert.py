# -*- coding: utf-8 -*-
"""
npk_try_invert.py — 验证 NXPK 数据层加密 = 逐字节取反 (XOR 0xFF) 假设
流程：块取反 → 找压缩头 (zlib 78 xx / lz4 block) → 解压 → 检查 marshal
测试对象：Android 热修增量 script.npk (12条) + 全量 script.npk (抽样)
"""
import struct
import sys
import zlib

sys.path.insert(0, r'.\out\tools')
from npk_analyze import parse_npk


def lz4_block_decompress(src, dst_size):
    """纯 Python LZ4 block 解码器。
    格式: [token] [lit_len扩展] [literals] [offset:2LE] [match_len扩展] 循环
    token 高4位=literal长度(15表示有扩展字节), 低4位=match长度-4(15表示扩展)
    返回解压 bytes；失败抛异常。"""
    out = bytearray()
    i = 0
    n = len(src)
    while i < n:
        token = src[i]; i += 1
        lit_len = token >> 4
        if lit_len == 15:
            while True:
                b = src[i]; i += 1
                lit_len += b
                if b != 255:
                    break
        out += src[i:i + lit_len]
        i += lit_len
        if i >= n:
            break
        # match
        offset = src[i] | (src[i + 1] << 8); i += 2
        if offset == 0:
            raise ValueError('lz4 offset=0')
        match_len = (token & 0xF)
        if match_len == 15:
            while True:
                b = src[i]; i += 1
                match_len += b
                if b != 255:
                    break
        match_len += 4
        start = len(out) - offset
        if start < 0:
            raise ValueError('lz4 bad offset')
        for _ in range(match_len):
            out.append(out[start])
            start += 1
    if len(out) != dst_size:
        raise ValueError('lz4 size mismatch %d != %d' % (len(out), dst_size))
    return bytes(out)


def try_block(blob, size):
    """对一个(取反后的)块依次尝试各种解压。返回 (method, data) 或 (None, blob)"""
    # zlib
    if blob[:1] == b'\x78':
        try:
            return 'zlib', zlib.decompress(blob)
        except Exception:
            try:
                return 'zlib-raw-deflate', zlib.decompress(blob, -15)
            except Exception:
                pass
    # lz4 block
    try:
        return 'lz4', lz4_block_decompress(blob, size)
    except Exception:
        pass
    # 也许头部有若干字节前缀再接压缩流：扫描 zlib 头
    idx = blob.find(b'\x78\x9c')
    if idx > 0:
        try:
            return 'zlib@%d' % idx, zlib.decompress(blob[idx:])
        except Exception:
            pass
    idx = blob.find(b'\x78\xda')
    if idx > 0:
        try:
            return 'zlib@%d' % idx, zlib.decompress(blob[idx:])
        except Exception:
            pass
    return None, blob


def main(path, limit=999):
    version, entries, data = parse_npk(path)
    print('== %s (version=0x%x, %d entries) ==' % (path, version, len(entries)))
    ok = fail = 0
    for e in sorted(entries, key=lambda x: x['csize'])[:limit]:
        raw = data[e['offset']:e['offset'] + e['csize']]
        inv = bytes(b ^ 0xFF for b in raw)
        method, out = try_block(inv, e['size'])
        if method:
            ok += 1
            head = out[:8].hex()
            note = ''
            if out[:1] == b'c':
                note = ' <- marshal code!'
            print('[OK ] hash=0x%08x csize=%5d size=%5d method=%-14s head=%s%s'
                  % (e['hash'], e['csize'], e['size'], method, head, note))
        else:
            fail += 1
            print('[FAIL] hash=0x%08x csize=%5d size=%5d inv_head=%s'
                  % (e['hash'], e['csize'], e['size'], inv[:16].hex()))
    print('\nresult: ok=%d fail=%d' % (ok, fail))


if __name__ == '__main__':
    p = sys.argv[1] if len(sys.argv) > 1 else r'.\files\netease\h62\Documents\script.npk'
    lim = int(sys.argv[2]) if len(sys.argv) > 2 else 999
    main(p, lim)
