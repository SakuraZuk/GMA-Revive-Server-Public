# -*- coding: utf-8 -*-
"""在仓库外准备正式发布工作目录，私有分发地址从已授权备份读取。"""
import copy
import json
import os
import re
import shutil
import sys
from datetime import datetime
from pathlib import Path

sys.stdout.reconfigure(encoding='utf-8')
root = Path(__file__).resolve().parents[2]
private = Path(os.environ['LOCALAPPDATA'])/'hs-server'
backup = private/'deployment-config/hotfix.json'
production = json.loads(backup.read_text(encoding='utf-8'))
public = json.loads((root/'deploy/data/hotfix.json').read_text(encoding='utf-8'))
from server_ops import connect, run
client = connect()
try:
    live = json.loads(run(client, 'cat /opt/hs-server/data/hotfix.json'))
finally:
    client.close()
for key in set(production) | set(live):
    if key not in ('runtime', 'startup_scripts') and production.get(key) != live.get(key):
        raise ValueError('私有备份的正式分发配置与现网不一致')
if production['runtime']['index'] != live['runtime']['index']:
    raise ValueError('私有备份版本已落后于现网，禁止覆盖')
for version, live_script in live['startup_scripts'].items():
    for variable in ('GAME_IP', 'PATCH_URL'):
        pattern = r"(?m)^\s*"+variable+r"\s*=\s*('[^']*'|\"[^\"]*\")"
        old = re.search(pattern, production['startup_scripts'][version])
        actual = re.search(pattern, live_script)
        if old is None or actual is None or old[1] != actual[1]:
            raise ValueError('私有备份启动分发目标与现网不一致')
if '--verify-live-only' in sys.argv:
    print(json.dumps({'状态':'私有分发备份与现网配置一致','现网版本':live['runtime']['index']},ensure_ascii=False))
    raise SystemExit(0)
stage = private/'release-workspaces'/datetime.now().strftime('%Y%m%d-%H%M%S')
stage.mkdir(parents=True, exist_ok=False)
for folder in ('cmd', 'internal', 'deploy'):
    def ignored(path, names):
        return ['runtime'] if Path(path).name == 'nativepvp' and 'runtime' in names else []
    shutil.copytree(root/folder, stage/folder, ignore=ignored)
for name in ('go.mod', 'go.sum'):
    shutil.copy2(root/name, stage/name)
shutil.copytree(root/'out/tools', stage/'out/tools', ignore=shutil.ignore_patterns('__pycache__'))
needed = ('gameplay-policy-approval.json', 'remaining-policy-proposal.json',
          'native-pvp-linux-verification.json', 'native-pvp-production-runtime.json', 'remote-database-verification.json',
          'native-pvp-engine-linux-resources.tar.gz')
for name in needed:
    shutil.copy2(root/'out'/name, stage/'out'/name)
shutil.copytree(root/'out/native-pvp-linux-packages', stage/'out/native-pvp-linux-packages')
script = next(iter(production['startup_scripts'].values()))
template_path = stage/'deploy/data/hotfix_startup_v10.py'
template = template_path.read_text(encoding='utf-8')
for variable in ('GAME_IP', 'PATCH_URL'):
    pattern = r"(?m)^(\s*"+variable+r"\s*=\s*)('[^']*'|\"[^\"]*\")"
    value = re.search(pattern, script)
    if value is None:
        raise ValueError('私有备份缺少分发变量：'+variable)
    template, count = re.subn(pattern, lambda m: m[1]+value[2], template)
    if count != 1:
        raise ValueError('启动模板分发变量数量不一致')
template_path.write_text(template, encoding='utf-8')
candidate = copy.deepcopy(production)
candidate['runtime'] = public['runtime']
candidate['startup_scripts'] = public['startup_scripts']
(stage/'deploy/data/hotfix.json').write_text(json.dumps(candidate,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
# 在私有目录重新组合，新业务正文与原地址各自沿用可信来源。
import subprocess
subprocess.run([sys.executable,str(stage/'out/tools/merge_battle_bridge_hotfix.py')],cwd=stage,check=True)
body = (stage/'deploy/data/hotfix.json').read_text(encoding='utf-8')
if re.search(r'192\.0\.2\.(10|20)', body):
    raise ValueError('正式候选仍包含公开示例地址')
report = {'状态':'私有正式目录准备完成','工作目录':str(stage),'源码目录':str(root),
          '热更版本':candidate['runtime']['index'], '边界':'尚未构建、实库验收或发布；地址不写回公开源码'}
(root/'out/private-release-workspace.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'状态':report['状态'],'热更版本':report['热更版本']},ensure_ascii=False))
