"""打印本项目引擎运算指令证据，避免靠参考服猜运算。"""
import json
from pathlib import Path
root=Path(__file__).resolve().parents[2]
for name in ['op2handler.json','handler_features.json']:
    p=root/'out/tools'/name
    obj=json.loads(p.read_text(encoding='utf-8'))
    print(name)
    if isinstance(obj,dict):
        for key,value in obj.items():
            if key in ['39','44'] or 'Divide' in str(value) or 'Multiply' in str(value):
                print(key,json.dumps(value,ensure_ascii=False)[:3000])
    else:
        print(str(obj)[:1000])
import pefile,capstone
pe=pefile.PE(str(root/'prototype/neteasehsqsl/hsqsl.exe'))
base=pe.OPTIONAL_HEADER.ImageBase
pe.parse_data_directories()
exports={base+s.address:s.name.decode('ascii','replace') for s in pe.DIRECTORY_ENTRY_EXPORT.symbols if s.name}
raw=pe.get_memory_mapped_image()
md=capstone.Cs(capstone.CS_ARCH_X86,capstone.CS_MODE_32)
for va in [0xce1eab,0xce1ee3]:
    print('handler',hex(va))
    for x in md.disasm(raw[va-base:va-base+120],va):
        print(hex(x.address),x.mnemonic,x.op_str,exports.get(int(x.op_str,16),'') if x.mnemonic=='call' and x.op_str.startswith('0x') else '')
