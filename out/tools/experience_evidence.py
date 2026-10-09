# -*- coding: utf-8 -*-
"""只读Android原生证据检索，不执行客户端代码。"""
import contextlib, io, json, re, sys
from pathlib import Path
from mem_marshal_extract import Loader
from neox_dis import disasm

sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')
root = Path(__file__).resolve().parents[2]
mode, pattern = sys.argv[1:3]
if mode == 'md':
    for p in sorted(root.rglob('*.md')):
        text = p.read_text(encoding='utf-8-sig')
        print(str(p.relative_to(root)), len(text), '\n'+text)
elif mode == 'table':
    for name in sys.argv[2:]:
        value = json.loads((root/'out/client_catalogs/tables'/(name+'.json')).read_text(encoding='utf-8'))
        data = value['数据']
        print(name, len(data), json.dumps(data, ensure_ascii=False, indent=2))
elif mode == 'sample':
    data=json.loads((root/'out/client_catalogs/tables'/(pattern+'.json')).read_text(encoding='utf-8'))['数据']
    print('总条目',len(data))
    print(json.dumps(dict(list(data.items())[:int(sys.argv[3])]),ensure_ascii=False,indent=2))
elif mode == 'rows':
    for name in sys.argv[3:]:
        data=json.loads((root/'out/client_catalogs/tables'/(name+'.json')).read_text(encoding='utf-8'))['数据']
        for key,row in data.items():
            if re.search(pattern.replace(',', '|'),json.dumps(row,ensure_ascii=False)):print(name,key,json.dumps(row,ensure_ascii=False))
elif mode == 'filter':
    data=json.loads((root/'out/client_catalogs/tables'/(pattern+'.json')).read_text(encoding='utf-8'))['数据']
    field,value=sys.argv[3],int(sys.argv[4])
    print(json.dumps({k:r for k,r in data.items() if isinstance(r,dict) and r.get(field)==value},ensure_ascii=False,indent=2))
elif mode == 'heads':
    heads=json.loads((root/'out/client_catalogs/tables/head_box_info.json').read_text(encoding='utf-8'))['数据']
    targets=json.loads((root/'out/client_catalogs/tables/base_target.json').read_text(encoding='utf-8'))['数据']
    for key,h in heads.items():
        if h.get('unlock_target_id'):print(key,json.dumps(h,ensure_ascii=False),json.dumps(targets.get(str(h['unlock_target_id'])),ensure_ascii=False))
elif mode == 'frameitems':
    heads=json.loads((root/'out/client_catalogs/tables/head_box_info.json').read_text(encoding='utf-8'))['数据']
    materials=json.loads((root/'out/client_catalogs/tables/materials.json').read_text(encoding='utf-8'))['数据']
    for key,row in materials.items():
        if row.get('type')==9:
            head=heads.get(str(row['target_id']),{})
            print(key,'目标',row['target_id'],'类型',head.get('head_type'),'小时',head.get('limit_days'),'描述',row.get('desc'))
elif mode == 'errors':
    data=json.loads((root/'internal/game/collection_catalog.json').read_text(encoding='utf-8'))['errors']
    for name,value in data.items():
        if int(pattern)<=value<=int(sys.argv[3]):print(name,value)
elif mode == 'check':
    data=json.loads((root/'internal/game/collection_catalog.json').read_text(encoding='utf-8'))['errors']
    for p in (root/'internal/game').glob('collection*.go'):
        for name in re.findall(r'runeReject\("([^"\n]+)"',p.read_text(encoding='utf-8')):
            if name not in data:print('缺失原生错误码',p.name,name)
elif mode == 'achv':
    ach=json.loads((root/'out/client_catalogs/tables/achv.json').read_text(encoding='utf-8'))['数据']
    targets=json.loads((root/'out/client_catalogs/tables/base_target.json').read_text(encoding='utf-8'))['数据']
    for key,a in ach.items():
        if re.search(pattern,json.dumps(a,ensure_ascii=False)):
            print(key,json.dumps(a,ensure_ascii=False),json.dumps({str(t):targets[str(t)] for group in a['target_id'] for t in group},ensure_ascii=False))
elif mode == 'collection_achv':
    ach=json.loads((root/'out/client_catalogs/tables/achv.json').read_text(encoding='utf-8'))['数据']
    targets=json.loads((root/'out/client_catalogs/tables/base_target.json').read_text(encoding='utf-8'))['数据']
    for key,a in ach.items():
        rows=[targets[str(t)] for group in a['target_id'] for t in group]
        if any(1000<=t['target_type']<1100 for t in rows):print(key,a['name'],a['desc'],a['target_need_count'],json.dumps(rows,ensure_ascii=False))
elif mode == 'commodity':
    data=json.loads((root/'out/client_catalogs/tables/commodity.json').read_text(encoding='utf-8'))['数据']
    for k,r in data.items():
        if r['commodity_type']!=1:print(k,json.dumps(r,ensure_ascii=False))
elif mode == 'special_commodities':
    data=json.loads((root/'out/client_catalogs/tables/commodity.json').read_text(encoding='utf-8'))['数据']
    for k,r in data.items():
        if r.get('rune_commodity_list') or r.get('give_bonus_id') or r.get('open_server_days') is not None or r.get('discount_ratio_range'):print(k,json.dumps(r,ensure_ascii=False))
elif mode == 'refs':
    data=json.loads((root/'out/npk_scripts/android_inventory.json').read_text(encoding='utf-8'))['modules']
    for mod in data:
        for method in mod['methods']:
            if pattern in json.dumps(method,ensure_ascii=False) and pattern not in method['name']:print(mod['hash'],mod['filename'],method['name'])
elif mode == 'dis':
    module, method = pattern, sys.argv[3].replace(',', '|')
    c = Loader((root/'out/npk_scripts/android_base'/(module+'.marshal')).read_bytes()).r_object()
    def walk(c, prefix=''):
        name=c['name'].decode('utf-8') if isinstance(c['name'], bytes) else c['name']
        full=prefix+'/'+name
        if re.search(method, full):
            local=dict(c);local['consts']=[x if not isinstance(x,dict) or x.get('type')!='code' else '<嵌套方法>' for x in c['consts']]
            disasm(local,prefix)
        for child in c['consts']:
            if isinstance(child,dict) and child.get('type')=='code':walk(child,full)
    walk(c)
elif mode == 'constants':
    from export_client_catalogs import instructions, text
    c = Loader((root/'out/npk_scripts/android_base'/(pattern+'.marshal')).read_bytes()).r_object()
    ops=list(instructions(c)); selected=sys.argv[3].replace(',', '|') if len(sys.argv)>3 else '.*'
    for i,(_,op,arg) in enumerate(ops):
        if op==116 and i and ops[i-1][1]==153:
            name=text(c['names'][arg]);value=c['consts'][ops[i-1][2]]
            if re.search(selected,name):print(name,repr(value))
elif mode == 'definition':
    c = Loader((root/'out/npk_scripts/android_base'/(pattern+'.marshal')).read_bytes()).r_object()
    selected=sys.argv[3].replace(',', '|')
    def definitions(c):
        buf=io.StringIO();local=dict(c);local['consts']=[x if not isinstance(x,dict) or x.get('type')!='code' else '<嵌套方法>' for x in c['consts']];disasm(local,out=buf)
        lines=buf.getvalue().splitlines();shown=set()
        for i,line in enumerate(lines):
            if re.search(selected,line):
                for j in range(max(0,i-12),min(len(lines),i+2)):
                    if j not in shown:print(lines[j]);shown.add(j)
        for child in c['consts']:
            if isinstance(child,dict) and child.get('type')=='code':definitions(child)
    definitions(c)
elif mode == 'ref':
    text=(root/'_ref/GMA-Revive-Server-main'/pattern).read_text(encoding='utf-8')
    lines=text.splitlines()
    start,end=map(int,sys.argv[3:5])
    print('\n'.join(str(i+1)+': '+s for i,s in enumerate(lines) if start<=i+1<=end))
elif mode == 'lines':
    text=(root/pattern).read_text(encoding='utf-8-sig')
    start,end=map(int,sys.argv[3:5])
    print('\n'.join(str(i+1)+': '+s for i,s in enumerate(text.splitlines()) if start<=i+1<=end))
