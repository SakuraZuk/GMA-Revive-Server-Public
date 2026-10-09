# -*- coding: utf-8 -*-
"""
so_xref_scan.py — libclient.so 定点逆向（轻量，替代完整 Ghidra 分析）
方法：
  1. ELF 加载（pyelftools）取 .text/.rodata 段
  2. 找 NXNpk 关键字符串 vaddr
  3. ARM32 字面量池扫描：.text 中出现的 `绝对地址` 4字节（PC 相对 LDR 的池）
     凡指向目标字符串的池项 → 池项所在函数区域
  4. capstone 反汇编该区域前后 4KB，导出汇编 + 保存到 out/ghidra/so_xref.asm
重点找：XOR 解密循环、密钥表初始化（S-box 256 写入循环特征 mov 0..255）
"""
import struct
import sys
from elftools.elf.elffile import ELFFile
from capstone import Cs, CS_ARCH_ARM, CS_MODE_ARM, CS_MODE_THUMB

SO_PATH = r'.\work\apk\lib\armeabi-v7a\libclient.so'

TARGET_STRINGS = [
    b'Failed to uncompress npk file(ZLIB)',
    b'Failed to uncompress npk file(LZ4)',
    b'LoadHeader: Invalid npk file',
    b'LoadWithIndices: Invalid npk file',
    b'AllNpkFilesHash::Load',
    b'NXNpk',
    b'NXNpkLoader',
]


def load_elf(path):
    f = open(path, 'rb')
    elf = ELFFile(f)
    segs = []
    for seg in elf.iter_segments():
        if seg['p_type'] == 'PT_LOAD':
            segs.append((seg['p_vaddr'], seg['p_filesz'], seg['p_memsz'], seg.data()))
    return elf, segs


def read_at(segs, vaddr, size):
    for base, fsz, msz, data in segs:
        if base <= vaddr < base + fsz:
            off = vaddr - base
            return data[off:off + size]
    return None


def find_strings(segs):
    """返回 {字符串: [vaddr...]}"""
    out = {}
    for base, fsz, msz, data in segs:
        for s in TARGET_STRINGS:
            start = 0
            while True:
                i = data.find(s, start)
                if i < 0:
                    break
                out.setdefault(s, []).append(base + i)
                start = i + 1
    return out


def find_literal_refs(segs, target_vaddrs, text_range):
    """在 .text（可执行段）里找指向 target 的 4 字节字面量。返回 {池vaddr: target}"""
    refs = {}
    for base, fsz, msz, data in segs:
        # 只扫可执行区域（粗略：以 .text 为准——直接扫全部 LOAD 也行，找对齐的 u32）
        for t in target_vaddrs:
            needle = struct.pack('<I', t)
            start = 0
            while True:
                i = data.find(needle, start)
                if i < 0:
                    break
                refs[base + i] = t
                start = i + 1
    return refs


def disasm_around(segs, vaddr, before=0x800, after=0x1000):
    """反汇编 vaddr 前后区域（ARM/Thumb 混合，先 ARM）"""
    start = vaddr - before
    code = read_at(segs, start, before + after)
    if code is None:
        return '(out of range)'
    md = Cs(CS_ARCH_ARM, CS_MODE_ARM)
    lines = []
    for ins in md.disasm(code, start):
        lines.append('0x%08x  %-8s %s' % (ins.address, ins.mnemonic, ins.op_str))
    return '\n'.join(lines)


def main():
    elf, segs = load_elf(SO_PATH)
    strs = find_strings(segs)
    for s, addrs in strs.items():
        print('%r -> %s' % (s.decode()[:40], [hex(a) for a in addrs]))

    # 字面量引用扫描
    all_targets = [a for addrs in strs.values() for a in addrs]
    refs = find_literal_refs(segs, all_targets, None)
    print('\nliteral pool refs: %d' % len(refs))

    with open(r'.\out\ghidra\so_xref.asm', 'w') as f:
        f.write('# libclient.so NXNpk xref disassembly (capstone)\n')
        f.write('# strings:\n')
        for s, addrs in strs.items():
            f.write('#   %r -> %s\n' % (s.decode()[:60], [hex(a) for a in addrs]))
        f.write('# literal refs: %s\n\n' % {hex(k): hex(v) for k, v in refs.items()})
        # 每个引用点反汇编周边
        for pool_addr, target in sorted(refs.items()):
            f.write('\n; ==== pool 0x%08x -> str 0x%08x (%r) ====\n'
                    % (pool_addr, target,
                       (read_at(segs, target, 48) or b'').split(b'\x00')[0][:40]))
            f.write(disasm_around(segs, pool_addr, 0x400, 0x400))
            f.write('\n')
    print('saved -> out/ghidra/so_xref.asm')


if __name__ == '__main__':
    main()
