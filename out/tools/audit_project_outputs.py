# -*- coding: utf-8 -*-
"""核对项目文档 UTF-8、旧导出覆盖与现行数据文件，避免保留过期入口。"""
import json
from pathlib import Path
import sys

ROOT=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')
for path in sorted(ROOT.rglob('*.md')):
    text=path.read_text(encoding='utf-8-sig')
    if '\ufffd' in text: raise ValueError('文档含乱码替换字符：'+str(path))
    print('文档 UTF-8 通过：'+str(path.relative_to(ROOT)))
required=('server-smoke.json','gate-local-smoke.json','gate-remote-before-restart.json',
          'gate-remote-after-restart.json','gate-remote-final.json',
          'gate-remote-auth-rejection.json','remote-release-verification.json',
          'time-contract-verification.json','client_catalogs/verification.json')
reports={}
for relative in required:
    path=ROOT/'out'/relative
    text=path.read_text(encoding='utf-8')
    if '\ufffd' in text: raise ValueError('验收记录含乱码：'+relative)
    reports[relative]=json.loads(text)
avatars=[reports[name]['角色编号'] for name in ('gate-remote-before-restart.json','gate-remote-after-restart.json','gate-remote-final.json')]
assert len(set(avatars))==1
assert reports['gate-remote-auth-rejection.json']['错误码']==9002
assert reports['gate-remote-auth-rejection.json']['下发角色或玩家成功消息'] is False
print('文档引用的验收记录齐全，重启前后和最终部署的角色 OID 一致。')
for relative in ('out/rpc-catalog.json','out/datas/_index.json'):
    path=ROOT/relative
    if path.exists():
        value=json.loads(path.read_text(encoding='utf-8-sig'))
        print('旧导出结构：'+relative+' '+str(list(value)[:10]))
old=ROOT/'out/datas'
if old.is_dir():
    files=[path for path in old.rglob('*.json') if path.name!='_index.json']
    missing=[]
    for path in files:
        value=json.loads(path.read_text(encoding='utf-8-sig'))
        module=value.get('module','').replace('\\','/')
        new=ROOT/'out/client_catalogs/tables'/module.removeprefix('datas/').removesuffix('.py')
        if not new.with_suffix('.json').exists(): missing.append(module or path.name)
    print(json.dumps({'旧数据文件数':len(files),'现行目录未覆盖旧文件数':len(missing),'未覆盖示例':missing[:5]},ensure_ascii=False))
