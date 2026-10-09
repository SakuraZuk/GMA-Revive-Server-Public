# -*- coding: utf-8 -*-
"""
so_xref_scan2.py — Thumb-2 MOVW/MOVT xref 扫描
ARM Thumb-2 用 movw/movt 对加载 32 位地址：
  movw Rd, #lo16 ; movt Rd, #hi16  →  Rd = (hi16<<16)|lo16
全量反汇编 .text，找组合值 == 目标字符串地址的指令对，输出所在函数区域
"""
import struct
from elftools.elf.elffile import ELFFile
from capstone import Cs, CS_ARCH_ARM, CS_MODE_THUMB

SO_PATH = r'.\work\apk\lib\armeabi-v7a\libclient.so'
OUT = r'.\out\ghidra\so_xref2.asm'

TARGETS = {
    0x21f9d88: 'ZLIB_str',
    0x21f9ddc: 'LZ4_str',
    0xd50938: 'LoadHeader_str',
    0xd5090c: 'LoadWithIndices_str',
    0xf02ed4: 'AllNpkFilesHash_str1',
    0xf02f00: 'AllNpkFilesHash_str2',
}


def main():
    f = open(SO_PATH, 'rb')
    elf = ELFFile(f)
    text = elf.get_section_by_name('.text')
    base = text['sh_addr']
    data = text.data()
    print('.text vaddr=0x%x size=0x%x (%.1f MB)' % (base, len(data), len(data) / 1e6))

    md = Cs(CS_ARCH_ARM, CS_MODE_THUMB)
    md.detail = True
    md.skipdata = True

    hits = []          # (addr_of_movw, target)
    pending = {}       # reg -> (addr, lo16)
    count = 0
    for ins in md.disasm(data, base):
        count += 1
        if ins.mnemonic == 'movw':
            # movw rd, #imm
            try:
                ops = ins.op_str.split(', ')
                reg = ops[0]
                imm = int(ops[1].replace('#', ''), 0)
                pending[reg] = (ins.address, imm)
            except Exception:
                pass
        elif ins.mnemonic == 'movt':
            try:
                ops = ins.op_str.split(', ')
                reg = ops[0]
                imm = int(ops[1].replace('#', ''), 0)
                if reg in pending:
                    addr_w, lo = pending[reg]
                    val = (imm << 16) | (lo & 0xFFFF)
                    if val in TARGETS:
                        hits.append((addr_w, val))
                        print('HIT %s @ movw 0x%x -> 0x%x' % (TARGETS[val], addr_w, val))
                    del pending[reg]
            except Exception:
                pass
        # 寄存器被覆写时清 pending（粗略：同寄存器新定义）
        if ins.mnemonic in ('pop', 'ldr') and ',' in ins.op_str:
            pass
    print('disassembled %d insns, hits=%d' % (count, len(hits)))

    with open(OUT, 'w') as f:
        f.write('# Thumb-2 movw/movt xref hits\n')
        for a, v in hits:
            f.write('# 0x%08x -> 0x%08x %s\n' % (a, v, TARGETS[v]))
        # 反汇编每个 hit 前后 0x800
        md2 = Cs(CS_ARCH_ARM, CS_MODE_THUMB)
        for a, v in hits:
            start = a - 0x600
            off = a - base
            code = data[off - 0x600: off + 0x800]
            f.write('\n; ==== hit %s movw@0x%x target=0x%08x ====\n' % (TARGETS[v], a, v))
            for ins in md2.disasm(code, start):
                f.write('0x%08x  %-8s %s\n' % (ins.address, ins.mnemonic, ins.op_str))
    print('saved ->', OUT)


if __name__ == '__main__':
    main()
