# -*- coding: utf-8 -*-
"""npk_repack.py — NXPK 重打包器:替换指定条目并全链路自检。

加密链(npk_script_decode.py 逆向的逆过程):
    marshal = dump_marshal(code)
    wrapped = b'\\x00' + marshal + b'\\x00'      # 首尾各补 1 字节(解密端 [1:-1] 去除)
    xored   = bytes(b^233 if i<99 else b ...)     # 对合
    stored  = rotor.encrypt(zlib.compress(xored, 9))
    block   = lz4_compress(stored)                # 容器外层 codec=2
容器:头部(NXPK/count/.../index_off)+数据块+28 字节索引条目。
hash64 语义未确认,原样保留;条目按原顺序重排 offset。

用法:
    python out/tools/npk_repack.py build src.npk dst.npk manifest.json
      manifest.json: {"<HASH>": <新 marshal bytes 的文件路径>, ...}
"""
import json
import struct
import sys
import zlib
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'prototype'))
from rotor_compat import Rotor
from npk_unpack import parse_archive, decode_entry, lz4_decompress, DecodeError
from marshal_dump_py3 import dump_marshal
from mem_marshal_extract import Loader

BINDING = Path(r'./out/npk_decoded/script_loader_bindings.json')
XOR_N = 99
XOR_K = 233


def encode_script(marshal_bytes):
    wrapped = b'\x00' + marshal_bytes + b'\x00'
    xored = bytes(b ^ XOR_K if i < XOR_N else b for i, b in enumerate(wrapped))
    key = json.loads(BINDING.read_text(encoding='utf-8'))['rotor_key'].encode('ascii')
    return Rotor(key).encrypt(zlib.compress(xored, 9))


def lz4_compress(src):
    """标准 LZ4 block 压缩(贪心 4 字节哈希),与客户端解码器兼容。"""
    n = len(src)
    out = bytearray()
    anchor = 0
    i = 0
    table = {}

    def emit_match(out, anchor, start, match_len, offset):
        lit = start - anchor
        ml = match_len - 4
        token = ((15 if lit >= 15 else lit) << 4) | (15 if ml >= 15 else ml)
        out.append(token)
        # token 半字节==15 时解码器至少读 1 个扩展字节,ext==0 也必须显式写 0
        if lit >= 15:
            ext = lit - 15
            while ext >= 255:
                out.append(255)
                ext -= 255
            out.append(ext)
        out += src[anchor:start]
        out += struct.pack('<H', offset)
        if ml >= 15:
            ext = ml - 15
            while ext >= 255:
                out.append(255)
                ext -= 255
            out.append(ext)

    mflimit = n - 12  # 匹配不得进入最后 5 字节,参考 LZ4 参考实现边界
    while i < mflimit:
        seq = src[i:i+4]
        h = (int.from_bytes(seq, 'little') * 2654435761 >> 16) & 0xFFFFF
        cand = table.get(h)
        table[h] = i
        if cand is not None and i - cand < 65536 and src[cand:cand+4] == seq:
            # 扩展匹配;LZ4 end-of-block 规则:最后 5 字节必须是字面量
            j = i + 4
            k = cand + 4
            last_lit = max(n - 5, 0)
            while j < last_lit and src[j] == src[k]:
                j += 1
                k += 1
            emit_match(out, anchor, i, j - i, i - cand)
            anchor = j
            i = j
        else:
            i += 1
    # 尾部全字面量
    lit = n - anchor
    token = (15 if lit >= 15 else lit) << 4
    out.append(token)
    if lit >= 15:
        ext = lit - 15
        while ext >= 255:
            out.append(255)
            ext -= 255
        out.append(ext)
    out += src[anchor:n]
    return bytes(out)


def replace_consts(code, mapping):
    """递归替换 code 树 consts 中的 bytes 字符串(字节码/LOAD_CONST 索引不变)。

    mapping: {旧 bytes: 新 bytes}。返回替换次数。原地修改 code dict。
    """
    count = 0

    def subst(obj):
        nonlocal count
        if isinstance(obj, bytes) and obj in mapping:
            count += 1
            return mapping[obj]
        if isinstance(obj, tuple):
            return tuple(subst(x) for x in obj)
        if isinstance(obj, list):
            return [subst(x) for x in obj]
        if isinstance(obj, dict) and obj.get('type') == 'code':
            obj['consts'] = subst(obj['consts'])
            return obj
        return obj

    code['consts'] = subst(code['consts'])
    return count


def rebuild(src_path, dst_path, replacements):
    entries, data = parse_archive(src_path)
    out = bytearray()
    out += data[:24]  # 原头部(count 等保持;索引偏移最后回填)
    new_entries = []
    for e in entries:
        h = '%08X' % e['hash']
        if h in replacements:
            stored = encode_script(Path(replacements[h]).read_bytes())
            block = lz4_compress(stored)
            new_e = dict(e)
            new_e['_block'] = block
            new_e['_stored'] = len(stored)
            new_e['_replaced'] = True
        else:
            new_e = dict(e)
            new_e['_block'] = data[e['offset']:e['offset']+e['stored_size']]
            new_e['_stored'] = e['decoded_size']
            new_e['_replaced'] = False
        new_entries.append(new_e)
    # 数据区从头部后开始(原数据区可能从 24 开始)
    offset = 24
    for ne in new_entries:
        ne['_offset'] = offset
        offset += len(ne['_block'])
    index_off = offset
    header = bytearray(data[:24])
    struct.pack_into('<I', header, 20, index_off)
    out = bytearray(header)
    for ne in new_entries:
        out += ne['_block']
    for ne in new_entries:
        out += struct.pack('<IIIIQI', ne['hash'], ne['_offset'],
                           len(ne['_block']), ne['_stored'],
                           ne['hash64'], ne['codec'])
    Path(dst_path).write_bytes(bytes(out))
    return len(new_entries), index_off


def verify(dst_path, replacements):
    """全链路读回:unpack→解密→Loader 完整消费。"""
    entries, data = parse_archive(dst_path)
    ok = 0
    for e in entries:
        raw = decode_entry(e, data)
        h = '%08X' % e['hash']
        if h in replacements:
            key = json.loads(BINDING.read_text(encoding='utf-8'))['rotor_key'].encode('ascii')
            inflated = zlib.decompress(Rotor(key).decrypt(raw))
            marshal = bytes(b ^ XOR_K if i < XOR_N else b
                            for i, b in enumerate(inflated))[1:-1]
            code = Loader(marshal).r_object()
            assert isinstance(code, dict) and code.get('type') == 'code', h
            ok += 1
    return ok, len(entries)


if __name__ == '__main__':
    cmd = sys.argv[1]
    if cmd == 'build':
        src, dst, manifest = sys.argv[2:5]
        reps = json.load(open(manifest, encoding='utf-8'))
        n, idx = rebuild(src, dst, reps)
        ok, tot = verify(dst, set(reps.keys()))
        print('entries=%d index_off=%d replaced-verified=%d/%d' % (n, idx, ok, len(reps)))
