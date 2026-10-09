# -*- coding: utf-8 -*-
"""从 hsqsl.exe 提取 PyEval_EvalFrameEx 并定位 opcode switch 跳转表。
"""
import pefile, capstone, sys

exe_path = r'.\prototype\neteasehsqsl\hsqsl.exe'
pe = pefile.PE(exe_path, fast_load=True)
pe.parse_data_directories(directories=[
    pefile.DIRECTORY_ENTRY['IMAGE_DIRECTORY_ENTRY_EXPORT']])
base = pe.OPTIONAL_HEADER.ImageBase
exp = pe.DIRECTORY_ENTRY_EXPORT.symbols
rva = None
for s in exp:
    if s.name == b'PyEval_EvalFrameEx':
        rva = s.address
        print('PyEval_EvalFrameEx RVA 0x%x VA 0x%x' % (rva, base+rva))
if rva is None:
    sys.exit('not found in exports')

data = pe.get_memory_mapped_image()[rva:rva+0x40000]
md = capstone.Cs(capstone.CS_ARCH_X86, capstone.CS_MODE_32)
md.detail = True

# 线性扫函数前 ~0x8000 字节,找 `jmp reg`(间接跳转)与其前最近的 `mov reg, [base + reg*4]`
# x86 MSVC switch:  mov eax, dword ptr [T + eax*4]; add eax, <codebase>; jmp eax
# 或:  cmp eax, N; ja default; jmp dword ptr [T + eax*4]
instrs = list(md.disasm(data, base + rva))
hits = []
for idx, ins in enumerate(instrs[:20000]):
    if ins.mnemonic == 'jmp' and not ins.op_str.startswith('0x'):
        # 找前 12 条里的跳转表装载
        window = instrs[max(0, idx-14):idx]
        tbl = None
        for w in window:
            if w.mnemonic in ('mov', 'lea', 'movzx') and ('*4]' in w.op_str or '*8]' in w.op_str):
                tbl = w
        hits.append((ins.address, tbl.address if tbl else None,
                     (tbl.op_str if tbl else ''),
                     window[-1].mnemonic + ' ' + window[-1].op_str))
        if len(hits) > 40:
            break

for h in hits:
    print('jmp@0x%x  table-ld@%s %s' % (h[0], hex(h[1]) if h[1] else '-', h[2]))
