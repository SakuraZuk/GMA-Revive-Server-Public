# -*- coding: utf-8 -*-
"""
uni_matrix.py — 组合矩阵测试：找出 PC 解码器期望的输入形态
历史探索脚本，仅用于对比错误输入与正确原始 LZ4 输入。
2026-10-05：修正地址为 VA 0x1309310，size=存储长度，csize=解压长度。
结果不得根据 EAX 是否为零或首字节是否为 c 判断成功。
"""
import struct
import sys

sys.path.insert(0, r'.\out\tools')
from unicorn import *
from unicorn.x86_const import *
import uni_lz4 as U
from npk_analyze import parse_npk

ZLIB_FN = 0x12D6A00


def call_fn(fn, src, size, csize, verbose=False):
    data = open(U.EXE, 'rb').read()
    uc = Uc(UC_ARCH_X86, UC_MODE_32)
    U.load_sections(uc, data)
    uc.mem_map(U.STACK, 0x100000, UC_PROT_ALL)
    uc.mem_map(U.BUFS, 0x1000000, UC_PROT_ALL)
    src_addr = U.BUFS + 0x1000
    dst_addr = U.BUFS + 0x200000
    RET = 0x5000000
    uc.mem_map(RET, 0x1000, UC_PROT_ALL)
    uc.mem_write(RET, b'\xF4')
    uc.mem_write(src_addr, src)

    esp = U.STACK + 0x80000
    args = [src_addr, dst_addr, len(src), size]   # f(src, dst, srcLen, dstLen)
    esp -= 16
    uc.mem_write(esp, b''.join(struct.pack('<I', a) for a in args))
    esp -= 4
    uc.mem_write(esp, struct.pack('<I', RET))
    uc.reg_write(UC_X86_REG_ESP, esp)
    uc.reg_write(UC_X86_REG_EBP, U.STACK + 0x90000)
    try:
        uc.emu_start(fn, RET, timeout=2_000_000, count=2_000_000)
    except UcError as e:
        return None, 'uc:%s' % e
    if uc.reg_read(UC_X86_REG_EIP) != RET:
        raise RuntimeError('没有到达返回哨兵')
    ret = uc.reg_read(UC_X86_REG_EAX)
    out = bytes(uc.mem_read(dst_addr, size))
    return (ret, out) if any(out) else (ret, None)


def main():
    v, e, d = parse_npk(r'.\files\netease\h62\Documents\script.npk')
    x = [y for y in e if y['hash'] == 0x9d33611e][0]
    cipher = d[x['offset']:x['offset'] + x['size']]
    inv = bytes(b ^ 0xFF for b in cipher)
    vi = inv.find(b'\xe6\x8c', 1, 48)

    variants = {
        'cipher_full': cipher,
        'inv_full': inv,
        'inv_skip0F': inv[1:],
        'inv_skip_varint': inv[vi:],
        'inv_skip_magic': inv[vi + 2:],
        'cipher_off1': cipher[1:],
    }
    for vname, src in variants.items():
        # zlib uncompress 的签名不同，不能使用同一参数布局测试。
        for fname, fn in (('LZ4', U.LZ4_DECOMP),):
            ret, out = call_fn(fn, src, x["csize"], len(src))
            tag = '长度吻合' if ret == x['csize'] else '失败'
            print('%-16s %-5s ret=%-12s %s %s' % (
                vname, fname, hex(ret) if isinstance(ret, int) else ret,
                tag, out[:32].hex() if isinstance(out, bytes) else ""))


if __name__ == '__main__':
    main()
