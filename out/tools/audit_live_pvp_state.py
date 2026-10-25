# -*- coding: utf-8 -*-
"""只读统计在线角色PVP状态；不导出玩家标识、卡牌、房间或指令正文。"""
import json
import shlex
import sys
from datetime import datetime
from server_ops import ROOT, connect, run

REMOTE = r'''
import json,os,shlex,subprocess
from pathlib import Path
from urllib.parse import urlsplit,unquote
line=next(value for value in Path('/opt/hs-server/data/database.env').read_text().splitlines()
          if value.startswith('HS_DATABASE_URL='))
u=urlsplit(shlex.split(line.split('=',1)[1])[0])
env=os.environ.copy()
env.update(PGHOST=u.hostname,PGPORT=str(u.port or 5432),PGUSER=unquote(u.username),
           PGPASSWORD=unquote(u.password or ''),PGDATABASE=u.path.lstrip('/'),
           LD_LIBRARY_PATH='/opt/hs-server/pgsql/runtime/usr/lib/x86_64-linux-gnu',
           PGOPTIONS='-c default_transaction_read_only=on -c statement_timeout=5000',PGCONNECT_TIMEOUT='5')
sql="""
WITH online AS (SELECT DISTINCT avatar_oid FROM hs_game_presence
 WHERE expires_at > extract(epoch FROM now())),
sessions AS (SELECT state->'server_battle' AS b, state->'sync_pvp_match' AS m,
 state->'server_social'->'human_room' AS r FROM avatar_progress JOIN online USING(avatar_oid)),
pvp AS (SELECT * FROM sessions WHERE b IS NOT NULL AND
 ((m->>'battle_uuid'=b->>'uuid') OR (r->>'uuid'=b->>'uuid')))
SELECT json_build_object(
 '在线角色数',(SELECT count(*) FROM online),
 '在线PVP会话数',count(*),
 '进行中',count(*) FILTER(WHERE coalesce(b->>'finished','false')!='true'),
 '实际已加载',count(*) FILTER(WHERE b->>'loaded'='true' AND coalesce(b->>'finished','false')!='true'),
 '桥已就绪',count(*) FILTER(WHERE b->>'bridge_ready'='true' AND coalesce(b->>'finished','false')!='true'),
 '服务端已开战',count(*) FILTER(WHERE b->>'started'='true' AND coalesce(b->>'finished','false')!='true'),
 '客户端已开战',count(*) FILTER(WHERE b->>'bridge_started'='true' AND coalesce(b->>'finished','false')!='true'),
 '加载握手齐备但未开战',count(*) FILTER(WHERE b->>'loaded'='true' AND b->>'bridge_ready'='true'
 AND coalesce(b->>'started','false')!='true' AND coalesce(b->>'finished','false')!='true'),
 '已结束',count(*) FILTER(WHERE b->>'finished'='true'),
 '真人会话镜像',count(*) FILTER(WHERE r->>'uuid'=b->>'uuid')) FROM pvp;
"""
result=subprocess.run(['/opt/hs-server/pgsql/runtime/usr/lib/postgresql/15/bin/psql','-X','-A','-t','-v','ON_ERROR_STOP=1','-c',sql],env=env,
                      stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=10)
if result.returncode:raise SystemExit('PVP匿名只读查询失败；未输出数据库凭据或原始错误')
print(json.dumps(json.loads(result.stdout.decode()),ensure_ascii=False))
'''

def main():
    sys.stdout.reconfigure(encoding='utf-8')
    client = connect()
    try:
        state = json.loads(run(client, 'python3 -c '+shlex.quote(REMOTE), timeout=20))
        pid = run(client, 'systemctl show hs-game -p MainPID --value')
        if not pid.isdigit() or int(pid) <= 0:
            raise RuntimeError('游戏服务未运行')
        sha = run(client, 'sha256sum /proc/'+pid+'/exe').split()[0]
    finally:
        client.close()
    report = {'采样时间': datetime.now().isoformat(), '游戏PID': pid, '运行SHA256': sha,
              '匿名PVP状态': state, '边界': '在线角色当前持久状态聚合；不是完整一局或画面验收。'}
    (ROOT/'out/live-pvp-state-audit.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(report,ensure_ascii=False,indent=2))

if __name__ == '__main__':
    main()
