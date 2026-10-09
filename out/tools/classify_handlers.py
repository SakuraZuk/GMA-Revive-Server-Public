# -*- coding: utf-8 -*-
"""批量反汇编 107 个 opcode handler,按访存/调用特征分类:
[ebp-0x4c]=consts, [ebp-0x18]=names, frame=[ebp+8](+0x18 globals,+0x1c locals,
+0x24 stacktop, +0x6c localsplus), [ebp-0x10]=instr_ptr(写=跳转), esi=栈顶。
"""
import pefile, capstone, json, struct
from collections import Counter

exe_path = r'.\prototype\neteasehsqsl\hsqsl.exe'
pe = pefile.PE(exe_path, fast_load=True)
base = pe.OPTIONAL_HEADER.ImageBase
img = pe.get_memory_mapped_image()
md = capstone.Cs(capstone.CS_ARCH_X86, capstone.CS_MODE_32)

# 导入表符号化
iat = {}
try:
    for entry in pe.DIRECTORY_ENTRY_IMPORT:
        dll = entry.dll.decode()
        for imp in entry.imports:
            if imp.address:
                iat[imp.address] = (dll.split('.')[0] + '!' + (imp.name.decode() if imp.name else '?')).lower()
except Exception:
    pass

op2handler = {int(k): v for k, v in json.load(open('op2handler.json')).items()}

def analyze(va):
    feats = []
    calls = []
    n = 0
    stop_jmp = 0
    for ins in md.disasm(img[va-base:va-base+600], va):
        n += 1
        if n > 160:
            break
        t = ins.mnemonic + ' ' + ins.op_str
        if 'ebp - 0x4c' in t:
            feats.append('CONST')
        if 'ebp - 0x18' in t:
            feats.append('NAME')
        if ins.mnemonic == 'call':
            op = ins.op_str
            if op.startswith('0x'):
                tgt = int(op, 16)
                name = iat.get(tgt)
                if name:
                    calls.append(name)
                    feats.append('CALL:' + name)
                else:
                    calls.append(hex(tgt))
        if 'ebp - 0x10' in t and ins.mnemonic == 'mov' and 'dword ptr [ebp - 0x10]' == ins.op_str.split(',')[0]:
            feats.append('SETIP')
        if '+ 0x6c]' in t:
            feats.append('LOCALSPLUS')
        if '+ 0x1c]' in t and 'ebp + 8' in ''.join(''):  # placeholder
            pass
        if ins.mnemonic == 'jmp':
            stop_jmp += 1
            if stop_jmp >= 2:
                break
    return feats, calls

report = {}
for op in sorted(op2handler):
    va = op2handler[op]
    feats, calls = analyze(va)
    f = Counter(x for x in feats if x in ('CONST', 'NAME', 'SETIP', 'LOCALSPLUS'))
    cc = Counter(c for c in feats if c.startswith('CALL:'))
    report[op] = dict(handler=hex(va), feats=dict(f), calls=cc.most_common(4))

KNOWN = {83: 'RETURN_VALUE', 101: 'LOAD_NAME', 153: 'LOAD_CONST'}
for op in sorted(report):
    r = report[op]
    extra = '  <= 已确认:' + KNOWN[op] if op in KNOWN else ''
    print('op %3d @%s %-40s %s%s' % (
        op, r['handler'],
        json.dumps(r['feats'], ensure_ascii=False),
        json.dumps(r['calls']), extra))
json.dump(report, open('handler_features.json', 'w'))
print('saved handler_features.json')
