# -*- coding: utf-8 -*-
"""静态复核玩家属性和指定数据记录，不执行客户端字节码。"""
import argparse
import json
import sys
from pathlib import Path
from mem_marshal_extract import Loader
from export_client_catalogs import walk, instructions, text
from neox_dis import disasm

ROOT=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')
p=argparse.ArgumentParser()
p.add_argument('mode',choices=['props','table','method'])
p.add_argument('source')
p.add_argument('names',nargs='*')
a=p.parse_args()
if a.mode=='table':
    value=json.loads((ROOT/'out/client_catalogs/tables'/a.source).with_suffix('.json').read_text(encoding='utf-8'))
    print(json.dumps(value if not a.names else {key:value[key] for key in a.names if key in value},ensure_ascii=False,indent=2))
else:
    code=Loader((ROOT/'out/npk_scripts'/a.source).read_bytes()).r_object()
    for path,owner in walk(code):
        if a.names and not any(path.endswith('/'+name) or path==name for name in a.names):continue
        if a.mode=='method':
            shallow={**owner,'consts':[{'type':'函数引用','name':text(v['name'])} if isinstance(v,dict) and v.get('type')=='code' else v for v in owner['consts']]}
            disasm(shallow)
            continue
        span=[]
        for offset,op,arg in instructions(owner):
            span.append((offset,op,arg))
            if op!=116:continue
            if any(item[1]==96 and text(owner['names'][item[2]])=='prop' for item in span):
                consts=[text(owner['consts'][item[2]]) for item in span if item[1]==153 and isinstance(owner['consts'][item[2]],(str,bytes))]
                print(json.dumps({'类':path,'字段':text(owner['names'][arg]),'参数':consts,'偏移':offset},ensure_ascii=False))
            span=[]
