# -*- coding: utf-8 -*-
"""按 Android 热修覆盖优先刷新既有反汇编，删除中文截断乱码。"""
import json
from pathlib import Path
from mem_marshal_extract import Loader
from neox_dis import disasm

ROOT=Path(__file__).resolve().parents[2]
inventory=json.loads((ROOT/'out/npk_scripts/android_inventory.json').read_text(encoding='utf-8'))
byhash={item['hash']:item for item in inventory['modules']}
count=0
for output in (ROOT/'out/dis').glob('*.asm'):
    if output.stem not in byhash: continue
    module=byhash[output.stem]
    code=Loader((ROOT/'out/npk_scripts'/module['marshal_file']).read_bytes()).r_object()
    with output.open('w',encoding='utf-8') as stream: disasm(code,out=stream)
    count+=1
print('既有 Android 反汇编已刷新：',count)
