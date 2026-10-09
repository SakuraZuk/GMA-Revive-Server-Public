# -*- coding: utf-8 -*-
"""读取原生剧情资源路径构造，核对资源目录索引身份。"""
import sys
import json
from pathlib import Path
from mem_marshal_extract import Loader
from neox_dis import disasm
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
with (root/'out/dis/remaining-storyline-native-wrapper.asm').open('w',encoding='utf-8') as f:
    disasm(Loader((root/'out/npk_scripts/android_base/AAAE4360.marshal').read_bytes()).r_object(),out=f)
def murmur(data, seed=0x9747b28c):
    mask=0xffffffff
    def rot(x,r):return ((x<<r)|(x>>(32-r)))&mask
    h=seed
    for i in range(0,len(data)//4*4,4):
        k=int.from_bytes(data[i:i+4],'little'); k=(k*0xcc9e2d51)&mask;k=rot(k,15);k=k*0x1b873593&mask
        h^=k;h=rot(h,13);h=(h*5+0xe6546b64)&mask
    tail=data[len(data)//4*4:];k=int.from_bytes(tail,'little')
    if tail:k=k*0xcc9e2d51&mask;k=rot(k,15);k=k*0x1b873593&mask;h^=k
    h^=len(data);h^=h>>16;h=h*0x85ebca6b&mask;h^=h>>13;h=h*0xc2b2ae35&mask;h^=h>>16
    return h
hits=[]
paths=[]
tables=json.loads((root/'out/client_catalogs/tables/guide_task.json').read_text(encoding='utf-8'))['数据']
for tid,row in tables.items():
    args=row.get('do_type_params') or []
    if args and isinstance(args[0],str):paths.append((tid,args[0]))
for tid,name in paths:
    for prefix in ['','storyline/','res/storyline/','res/','zh/','storyline_hant/','res/storyline_hant/','assets/storyline/','script/storyline/']:
        for suffix in ['.ets','.json','']:
            for sep in ['/','\\']:
                path=(prefix+name+suffix).replace('/',sep)
                for leading in ['',sep]:
                    key=murmur((leading+path).encode())
                    if key in [0x7A48A0CC,0x7F4D014C]:
                        hit={'任务':tid,'路径':leading+path,'哈希':f'{key:08X}'}
                        if hit not in hits:hits.append(hit);print(hit)
(root/'out/collection-graph-identity.json').write_text(json.dumps(hits,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
