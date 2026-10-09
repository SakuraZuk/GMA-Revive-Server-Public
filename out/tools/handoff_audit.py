# -*- coding: utf-8 -*-
"""交接只读快照：服务、备份和指定测试角色；不输出连接口令。"""
import json
import sys
from server_ops import ROOT, connect, run, audit
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')
client = connect()
try:
    result = audit(client)
    result['二进制副本'] = run(client, 'find /opt/hs-server/bin -maxdepth 2 -type f -path "*/backup*/gameserver" -printf "%p\\n" | sort')
    script = '''from pathlib import Path
from urllib.parse import urlsplit
import os, subprocess
raw=next(line.split('=',1)[1].strip().strip(chr(34)).strip(chr(39)) for line in Path('/opt/hs-server/data/database.env').read_text().splitlines() if line.startswith('HS_DATABASE_URL='))
u=urlsplit(raw)
env=dict(os.environ, PGHOST=u.hostname, PGPORT=str(u.port), PGUSER=u.username, PGPASSWORD=u.password, PGDATABASE=u.path.lstrip('/'), LD_LIBRARY_PATH='/opt/hs-server/pgsql/runtime/usr/lib/x86_64-linux-gnu')
sql="SELECT json_build_object('角色标识',encode(a.avatar_oid,'hex'),'UID',a.uid,'名字',a.nickname,'性别',a.gender,'已取名',a.nickname_set,'任务',p.state->'guide_tasks','战斗',p.state->'server_battle','版本',p.revision) FROM avatars a JOIN avatar_progress p USING(avatar_oid) WHERE encode(a.avatar_oid,'hex')='6ac32910b2e86acd79b13df0';"
r=subprocess.run(['/opt/hs-server/pgsql/runtime/usr/lib/postgresql/15/bin/psql','-X','-A','-t','-v','ON_ERROR_STOP=1','-c',sql],env=env,capture_output=True,text=True,check=True)
print(r.stdout.strip())
'''
    result['实机账号状态'] = json.loads(run(client, "python3 - <<'PY'\n" + script + '\nPY'))
    (ROOT/'out/handoff-state.json').write_text(json.dumps(result, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    print(json.dumps({k:result[k] for k in ['检查时间','数据库目标','二进制副本','实机账号状态']},ensure_ascii=False,indent=2))
finally:
    client.close()
