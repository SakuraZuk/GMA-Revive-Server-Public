# -*- coding: utf-8 -*-
"""从 NeoX 原解释器跳转表直接保存 STORE_MAP/STORE_ATTR/UNARY_NEGATIVE 证据。"""
from pathlib import Path
import struct
import pefile
import capstone

ROOT=Path(__file__).resolve().parents[2]
pe=pefile.PE(str(ROOT/'prototype/neteasehsqsl/hsqsl.exe'),fast_load=True)
pe.parse_data_directories(directories=[pefile.DIRECTORY_ENTRY['IMAGE_DIRECTORY_ENTRY_EXPORT']])
base=pe.OPTIONAL_HEADER.ImageBase
exports={base+item.address:item.name.decode('ascii','backslashreplace') for item in pe.DIRECTORY_ENTRY_EXPORT.symbols if item.name}
def read(va,size): return pe.get_data(va-base,size)
dis=capstone.Cs(capstone.CS_ARCH_X86,capstone.CS_MODE_32)
lines=[]
for op in (8,67,99,132):
    slot=read(0xce3eec+op-3,1)[0]
    va=struct.unpack('<I',read(0xce3d40+slot*4,4))[0]
    lines.append(f'操作码 {op}，跳转槽 {slot}，处理地址 0x{va:x}')
    for index,ins in enumerate(dis.disasm(read(va,240),va)):
        label=''
        if ins.mnemonic=='call' and ins.op_str.startswith('0x'):
            label=' ; '+exports.get(int(ins.op_str,16),'内部函数')
        lines.append(f'0x{ins.address:x}: {ins.mnemonic} {ins.op_str}{label}')
        if index>=44: break
(ROOT/'out/dis/store-opcode-evidence.asm').write_text('\n'.join(lines)+'\n',encoding='utf-8')
print('\n'.join(lines))
