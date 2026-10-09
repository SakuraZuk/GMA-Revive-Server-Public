# -*- coding: utf-8 -*-
"""
uni_lz4.py — Unicorn 模拟执行 hsqsl.exe 标准 LZ4 解码器（VA 0x1309310）
=====================================================================
2026-10-05 修正：旧反汇编的代码地址为 RVA；解码器 VA=0x1309310。
函数是标准 LZ4 解码逻辑，cdecl 签名为 f(src,dst,源长,目标容量)。
索引 +8 是存储长度，+12 是解压后长度；先前 size/csize 命名反了。
模拟必须到达返回哨兵；严禁自动映射未知内存掩盖错误。
模拟：映射 PE 全部节区 @0x400000 + 栈 + 源/目标缓冲，直接 call。
"""
import struct
import sys

from unicorn import *
from unicorn.x86_const import *

EXE = r'.\prototype\neteasehsqsl\hsqsl.exe'
LZ4_RVA = 0xF09310
LZ4_DECOMP = 0x1309310
STACK = 0x6000000       # 栈区（远离映像）
BUFS = 0x7000000        # src/dst 缓冲区


def load_sections(uc, data):
    pe_off = struct.unpack_from('<I', data, 0x3C)[0]
    nsec = struct.unpack_from('<H', data, pe_off + 6)[0]
    opt_size = struct.unpack_from('<H', data, pe_off + 20)[0]
    opt = pe_off + 24
    base = struct.unpack_from('<I', data, opt + 28)[0]
    # 整体映射（.text 需可执行）
    size_of_image = struct.unpack_from('<I', data, opt + 56)[0]
    uc.mem_map(base, (size_of_image + 0xFFF) & ~0xFFF, UC_PROT_ALL)
    uc.mem_write(base, data[:0x1000])  # 头
    for i in range(nsec):
        s = opt + opt_size + i * 40
        vsize, va, rsize, rptr = struct.unpack_from('<IIII', data, s + 8)
        uc.mem_write(base + va, data[rptr:rptr + rsize])
    return base


def lz4_call(src, size, csize, verbose=False):
    """模拟调用 f(src, dst, srcLen, dstLen)——参数序由反汇编验证"""
    data = open(EXE, 'rb').read()
    uc = Uc(UC_ARCH_X86, UC_MODE_32)
    load_sections(uc, data)
    uc.mem_map(STACK, 0x100000, UC_PROT_ALL)
    uc.mem_map(BUFS, 0x1000000, UC_PROT_ALL)

    src_addr = BUFS + 0x1000
    dst_addr = BUFS + 0x200000   # 2MB 源距离，安全
    uc.mem_write(src_addr, src)

    RET_MAGIC = 0x5000000
    uc.mem_map(RET_MAGIC, 0x1000, UC_PROT_ALL)
    uc.mem_write(RET_MAGIC, b'\xF4')  # hlt → 停机

    esp = STACK + 0x80000
    # cdecl 参数：push 逆序
    args = [src_addr, dst_addr, csize, size]  # f(src,dst,srcLen,dstLen) 反汇编验证
    esp -= 4 * 4
    uc.mem_write(esp, b''.join(struct.pack('<I', a) for a in args))
    esp -= 4
    uc.mem_write(esp, struct.pack('<I', RET_MAGIC))  # 返回地址

    regs = {UC_X86_REG_ESP: esp, UC_X86_REG_EBP: STACK + 0x90000}
    for r, v in regs.items():
        uc.reg_write(r, v)

    try:
        uc.emu_start(LZ4_DECOMP, RET_MAGIC, timeout=2_000_000, count=2_000_000)
    except UcError as e:
        if verbose:
            print('uc err:', e, 'eip=%s' % hex(uc.reg_read(UC_X86_REG_EIP)))
        return None
    if uc.reg_read(UC_X86_REG_EIP) != RET_MAGIC:
        raise RuntimeError("模拟未到达返回地址，不能把 EAX 当作函数返回值")
    ret = uc.reg_read(UC_X86_REG_EAX)
    if verbose:
        print('ret eax = 0x%x (%d)' % (ret, ret))
    if ret != size:
        return None
    out = uc.mem_read(dst_addr, size)
    return bytes(out)


if __name__ == '__main__':
    # 测试：增量包 0x9d33611e（取反后的容器体）
    import os
    sys.path.insert(0, r'.\out\tools')
    from npk_analyze import parse_npk

    v, e, d = parse_npk(r'.\files\netease\h62\Documents\script.npk')
    m = {x['hash']: x for x in e}
    for h in (0x9d33611e, 0x90ad7c63, 0x9ec4cd3f):
        x = m[h]
        raw = d[x['offset']:x['offset'] + x['size']]
        r = lz4_call(raw, x["csize"], len(raw), verbose=True)
        if r:
            print('hash=0x%08x -> %dB head: %s' % (h, len(r), r[:48].hex()))
            print('   ascii:', ''.join(chr(b) if 32 <= b < 127 else '.' for b in r[:120]))
        else:
            print('hash=0x%08x FAILED' % h)
