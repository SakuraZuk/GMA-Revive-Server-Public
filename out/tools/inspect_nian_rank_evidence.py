# -*- coding: utf-8 -*-
"""仅静态检查年兽及榜单marshal，不执行Android恢复代码。"""
import datetime, json, shutil, sys
from pathlib import Path
from mem_marshal_extract import Loader
root = Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
if len(sys.argv) > 1 and sys.argv[1] == 'archive':
    dest = root / 'out/backups' / ('before-nian-daily-' + datetime.datetime.now().strftime('%Y%m%d-%H%M%S'))
    dest.mkdir(parents=True)
    for name in ['completion-build.json','remote-database-verification.json','database-test.log','native-solo-linux-verification.json','native-pvp-linux-verification.json','native-authority-linux-go-verification.json','continuation-go-checks.json']:
        shutil.copy2(root/'out'/name, dest/name)
    print(str(dest))
    raise SystemExit
def text(value):
    return value.decode('utf-8', errors='replace') if isinstance(value,bytes) else str(value)
files = {p.name:p for p in (root/'out/npk_scripts/android_base').glob('*.marshal')}
files.update({p.name:p for p in (root/'out/npk_scripts/android_patch').glob('*.marshal')})
rows=[]
references=[]
for path in files.values():
    try:
        code=Loader(path.read_bytes()).r_object()
        filename=text(code['filename'])
        def find(c, prefix=''):
            name=prefix+'.'+text(c['name'])
            constants=[text(v) for v in c.get('consts',[]) if isinstance(v,(bytes,str))]
            names=[text(v) for v in c.get('names',[])]
            if 'sub_ranks' in names or 'sub_ranks' in constants:
                references.append({'模块':path.stem,'路径':filename,'方法':name})
            for child in c.get('consts',[]):
                if isinstance(child,dict) and child.get('type')=='code':find(child,name)
        find(code)
        if 'monster_nian' not in filename and ('rank' not in filename or 'gui' not in filename): continue
        methods=[]
        def visit(c):
            methods.append(text(c['name']))
            for child in c.get('consts',[]):
                if isinstance(child,dict) and child.get('type')=='code':visit(child)
        visit(code)
        rows.append({'模块':path.stem,'路径':filename,'方法':methods})
    except (KeyError,ValueError,TypeError):pass
(root/'out/nian-rank-module-inventory.json').write_text(json.dumps(rows,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
(root/'out/nian-sub-rank-references.json').write_text(json.dumps(references,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'模块数':len(rows),'sub_ranks全包引用':references},ensure_ascii=False))
