# -*- coding: utf-8 -*-
"""只反汇编本版录像相关marshal与pickle操作码，绝不反序列化执行pickle。"""
import hashlib
import json
import pickletools
import sys
from pathlib import Path
from mem_marshal_extract import Loader
from neox_dis import disasm

ROOT=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
OUTPUT=ROOT/'out/native-record-evidence'
OUTPUT.mkdir(exist_ok=True)
modules=['42B11C32','D617FEA4','AFDD4FE7','A66FD60A','362EE013']
inventory=json.loads((ROOT/'out/npk_scripts/android_inventory.json').read_text(encoding='utf-8'))
rows={r['hash']:r for r in inventory['modules']}
result={'说明':'Android1.0.128原生证据；未执行marshal或pickle','模块':[],'pickle样本':[]}
methods=set();custom_types=set();method_sources=[];custom_type_sources=[]
def text(v):return v.decode('utf-8') if isinstance(v,bytes) else v
def scan(c,filename,source):
    ins=[];b=c['bytecode'];i=0
    while i<len(b):
        op=b[i];arg=int.from_bytes(b[i+1:i+3],'little') if op>=90 else None
        ins.append((op,arg));i+=3 if op>=90 else 1
    for pos,(op,arg) in enumerate(ins):
        if op!=116:continue
        name=text(c['names'][arg])
        prior=ins[max(0,pos-9):pos]
        if pos>=3 and ins[pos-1]==(131,1) and ins[pos-2][0]==100 and ins[pos-3][0]==153 and any(p in (96,101,155) and text(c['names'][a])=='sync_method' for p,a in prior):
            methods.add(name);method_sources.append({'方法':name,'模块':filename,'来源':source})
    for child in c['consts']:
        if isinstance(child,dict) and child.get('type')=='code':
            name=text(child['name'])
            if filename.startswith('custom_types\\') and child['argcount']==0 and '__module__' in [text(n) for n in child['names']]:
                qualified=filename.removesuffix('.py').split('\\')[-1]+'.'+name
                custom_types.add(qualified)
                custom_type_sources.append({'类型':qualified,'模块':filename,'来源SHA256':source})
            scan(child,filename,source)
for row in inventory['modules']:
    filename=row['filename']
    if filename.startswith(('battle_logic\\','custom_types\\')):
        source=ROOT/'out/npk_scripts'/row['marshal_file'];data=source.read_bytes()
        scan(Loader(data).r_object(),filename,hashlib.sha256(data).hexdigest())
methods.discard('gm_do_trigger_action')
catalog={'methods':sorted(methods),'custom_types':sorted(custom_types),'sources':method_sources,'custom_type_sources':custom_type_sources,'拒绝方法':['gm_do_trigger_action：不开放GM脚本动作给玩家录像']}
(ROOT/'internal/game/native_record_catalog.json').write_text(json.dumps(catalog,ensure_ascii=False,indent=2),encoding='utf-8')
for key in modules:
    row=rows[key];source=ROOT/'out/npk_scripts'/row['marshal_file'];data=source.read_bytes()
    with (OUTPUT/(key+'.asm')).open('w',encoding='utf-8',newline='\n') as handle:
        print('源模块',row['filename'],'SHA256='+hashlib.sha256(data).hexdigest(),file=handle)
        disasm(Loader(data).r_object(),out=handle)
    result['模块'].append({'模块':row['filename'],'来源':str(source.relative_to(ROOT)),'SHA256':hashlib.sha256(data).hexdigest()})
for base in ['files','com.netease.hsqsl']:
    for sample in (ROOT/base).rglob('battle.txt'):
        raw=sample.read_bytes();operations=[]
        try:
            for op,arg,pos in pickletools.genops(raw):
                operations.append({'位置':pos,'操作码':op.name,'参数':str(arg)[:500]})
            result['pickle样本'].append({'来源':str(sample.relative_to(ROOT)),'SHA256':hashlib.sha256(raw).hexdigest(),'大小':len(raw),'操作码':operations})
        except Exception as e:
            result['pickle样本'].append({'来源':str(sample.relative_to(ROOT)),'错误':str(e)})
(OUTPUT/'manifest.json').write_text(json.dumps(result,ensure_ascii=False,indent=2),encoding='utf-8')
print(json.dumps({'模块数':len(modules),'pickle样本数':len(result['pickle样本']),'报告':'out/native-record-evidence/manifest.json'},ensure_ascii=False))
