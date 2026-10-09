# -*- coding: utf-8 -*-
"""
exe_xref_scan.py — PC hsqsl.exe (x86 PE) 定点逆向
x86 字符串引用 = 指令流内嵌绝对地址立即数（push/mov imm32）。
流程：PE 头解析 → .rodata 字符串 RVA → .text 搜 4 字节立即数命中 →
capstone 反汇编命中点周边 → 存 out/ghidra/exe_xref.asm
"""
import struct
import sys
from capstone import Cs, CS_ARCH_X86, CS_MODE_32

EXE = r'.\prototype\neteasehsqsl\hsqsl.exe'
OUT = r'.\out\ghidra\exe_xref.asm'

STRINGS = [
    b'Failed to uncompress npk file(ZLIB)',
    b'Failed to uncompress npk file(LZ4)',
    b'LoadHeader: Invalid npk file',
    b'LoadWithIndices: Invalid npk file',
    b'AllNpkFilesHash::Load',
    b'NXNpk',
]


def parse_pe(data):
    """最小 PE32 解析：返回 (image_base, sections=[(name, va, vsize, raw_ptr, raw_size)])"""
    assert data[:2] == b'MZ'
    pe_off = struct.unpack_from('<I', data, 0x3C)[0]
    assert data[pe_off:pe_off + 4] == b'PE\x00\x00'
    machine = struct.unpack_from('<H', data, pe_off + 4)[0]
    nsec = struct.unpack_from('<H', data, pe_off + 6)[0]
    opt_size = struct.unpack_from('<H', data, pe_off + 20)[0]
    opt = pe_off + 24
    magic = struct.unpack_from('<H', data, opt)[0]
    assert magic == 0x10b, 'not PE32 (magic=0x%x)' % magic
    image_base = struct.unpack_from('<I', data, opt + 28)[0]
    secs = []
    sec_off = opt + opt_size
    for i in range(nsec):
        s = sec_off + i * 40
        name = data[s:s + 8].rstrip(b'\x00').decode('ascii', 'replace')
        vsize, va, rsize, rptr = struct.unpack_from('<IIII', data, s + 8)
        secs.append((name, va, vsize, rptr, rsize))
    return image_base, secs


def rva_to_off(secs, rva):
    for name, va, vsize, rptr, rsize in secs:
        if va <= rva < va + max(vsize, rsize):
            return rptr + (rva - va)
    return None


def find_string_rvas(data, secs):
    out = {}
    for s in STRINGS:
        start = 0
        while True:
            i = data.find(s, start)
            if i < 0:
                break
            # off -> rva
            for name, va, vsize, rptr, rsize in secs:
                if rptr <= i < rptr + rsize:
                    out.setdefault(s, []).append(va + (i - rptr))
                    break
            start = i + 1
    return out


def main():
    data = open(EXE, 'rb').read()
    base, secs = parse_pe(data)
    print('image_base=0x%x sections=%s' % (base, [(n, hex(v)) for n, v, *_ in secs]))
    srvas = find_string_rvas(data, secs)
    for s, rvas in srvas.items():
        print('%r -> RVAs %s (VA %s)' % (s.decode()[:36], [hex(r) for r in rvas],
                                         [hex(base + r) for r in rvas]))

    # 在 .text 搜引用（立即数 = VA）
    text = None
    for name, va, vsize, rptr, rsize in secs:
        if name == '.text':
            text = (va, rptr, rsize)
            break
    if not text:
        print('no .text'); return
    tva, tptr, tsize = text
    tdata = data[tptr:tptr + tsize]
    md = Cs(CS_ARCH_X86, CS_MODE_32)

    hits = []
    for s, rvas in srvas.items():
        for rva in rvas:
            va = base + rva
            needle = struct.pack('<I', va)
            start = 0
            while True:
                i = tdata.find(needle, start)
                if i < 0:
                    break
                hits.append((base + tva + i, s))
                start = i + 1
    print('code hits: %d' % len(hits))

    with open(OUT, 'w', encoding='utf-8') as f:
        f.write('# hsqsl.exe x86 定点反汇编：代码地址为 VA，映像基址 0x%x\n' % base)
        for a, s in hits:
            f.write('# 0x%08x refs %r\n' % (a, s.decode()[:40]))
        for a, s in sorted(hits):
            # 命中点前后 0x600 反汇编
            lo = max(0, a - base - tva - 0x400)
            code = tdata[lo:lo + 0xA00]
            f.write('\n; ==== 0x%08x refs %r ====\n' % (a, s.decode()[:40]))
            for ins in md.disasm(code, base + tva + lo):
                mark = '   ; <<<<' if ins.address <= a < ins.address + ins.size else ''
                f.write('0x%08x  %-8s %-40s%s\n' % (ins.address, ins.mnemonic, ins.op_str, mark))
    print('saved ->', OUT)


if __name__ == '__main__':
    main()
