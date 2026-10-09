# -*- coding: utf-8 -*-
"""同步当前原生运行时的夏活宿主及客户端镜像注释；语法树必须完全相同。"""
import ast
import hashlib
import json
import shlex
import sys
from datetime import datetime
from pathlib import Path
from server_ops import ROOT, connect, run

sys.stdout.reconfigure(encoding='utf-8')
record = json.loads((ROOT/'out/private-release-workspace.json').read_text(encoding='utf-8'))
stage = Path(record['工作目录'])
runtime = json.loads((stage/'out/native-pvp-production-runtime.json').read_text(encoding='utf-8'))
base = runtime['目录']
if not base.startswith('/opt/hs-server/native-pvp/releases/') or '..' in base:
    raise ValueError('原生运行时目录不属于项目')
stamp = datetime.now().strftime('%Y%m%d-%H%M%S')
client = connect()
try:
    if run(client,'readlink -f /opt/hs-server/native-pvp/current') != base:
        raise ValueError('原生当前链接已变化')
    rows = []
    # 两份代码先全部验证，再开始同步，避免第一份更新后同源检查暂时失配。
    for name in ('internal/nativepvp/activity_buffs_native.py','internal/game/activity_buffs_script.py'):
        target = base+'/workspace/'+name
        current = (stage/name).read_bytes()
        if run(client,'readlink -m '+shlex.quote(target)) != target:
            raise ValueError('原生目标解析越界')
        with client.open_sftp() as sftp:
            with sftp.open(target) as f: previous = f.read()
        # 不删除文档字符串或常量；仅注释及空白不会进入AST。
        if ast.dump(ast.parse(previous.decode('utf-8')), include_attributes=False) != ast.dump(ast.parse(current.decode('utf-8')), include_attributes=False):
            raise ValueError('运行时代码语义不同，禁止注释同步')
        rows.append((name,target,current,hashlib.sha256(previous).hexdigest(),hashlib.sha256(current).hexdigest()))
    files = []
    for name,target,current,before,after in rows:
        backup = target+'.comment-backup-'+stamp
        temporary = target+'.comment-stage-'+stamp
        if before != after:
            run(client,'test ! -e '+shlex.quote(backup)+' && cp -a '+shlex.quote(target)+' '+shlex.quote(backup))
            if run(client,'sha256sum '+shlex.quote(backup)).split()[0] != before:
                raise ValueError('注释同步备份哈希不一致')
            with client.open_sftp() as sftp:
                with sftp.open(temporary,'wb') as f: f.write(current)
            if run(client,'sha256sum '+shlex.quote(temporary)).split()[0] != after:
                raise ValueError('注释候选上传哈希不一致')
            if run(client,'sha256sum '+shlex.quote(target)).split()[0] != before:
                raise ValueError('注释目标在同步前发生变化')
            run(client,'chmod --reference='+shlex.quote(target)+' '+shlex.quote(temporary)+' && mv -T '+shlex.quote(temporary)+' '+shlex.quote(target))
        if run(client,'sha256sum '+shlex.quote(target)).split()[0] != after:
            raise ValueError('注释同步后的哈希不一致')
        files.append({'文件':name,'旧SHA256':before,'新SHA256':after,'备份':backup if before!=after else None})
    command = ('cd '+shlex.quote(base+'/workspace')+' && HS_NATIVE_PVP_PYTHON2='+shlex.quote(base+'/python2/run-python2')+
               ' /usr/bin/python3 out/tools/test_remaining_activity_native.py')
    proof = run(client,command,timeout=60)
    report = {'状态':'注释同步及真实Linux原生专项通过','文件':files,'执行语法树完全相同':True,
              '原生专项末行':proof.splitlines()[-1] if proof else '',
              '边界':'原生当前链接未切换，仅两份夏活文件注释与空白同步，无业务代码更改'}
finally:
    client.close()
destination = ROOT/'out/evidence/player-bugs-20261009/repair12/原生注释同步.json'
destination.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
runtime['注释同步'] = report
(stage/'out/native-pvp-production-runtime.json').write_text(json.dumps(runtime,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps(report,ensure_ascii=False))
