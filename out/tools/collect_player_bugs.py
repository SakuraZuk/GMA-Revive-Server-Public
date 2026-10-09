"""只读收集本次玩家故障日志和全角色状态，不输出账号口令。"""
import json
import sys
import argparse
import re
from datetime import datetime
from pathlib import Path
from server_ops import connect, run, ROOT

sys.stdout.reconfigure(encoding='utf-8')
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--live', action='store_true', help='采集新线上窗口，保留原始证据')
parser.add_argument('--logs-only', action='store_true', help='仅采日志，不重新读取全服存档')
parser.add_argument('--pid', type=int, help='只统计指定游戏进程，隔离发布前后的日志')
parser.add_argument('--since', default='2026-10-09 00:49:00', help='服务器北京时间起点')
args = parser.parse_args()
if not re.fullmatch(r'\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}', args.since):
    raise SystemExit('日志起点格式无效')
if args.pid is not None and args.pid <= 0:
    raise SystemExit('游戏进程编号必须为正整数')
destination = ROOT / 'out/evidence/player-bugs-20261009'
if args.live:
    destination /= 'followup-' + datetime.now().strftime('%H%M%S')
destination.mkdir(parents=True, exist_ok=True)
status_path = destination / '采集状态.json'
status = {'开始时间': datetime.now().isoformat(), '日志起点': args.since,
          '状态': '采集中', '日志已保存': False, '角色快照已保存': False}
if args.pid is not None:
    status['游戏PID'] = args.pid
def save_status():
    status_path.write_text(json.dumps(status, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
save_status()
client = None
try:
    client = connect()
    # 编码推送体量较大，单独保存业务事件；不得输出鉴权参数。
    command = "journalctl -u hs-game --since '" + args.since + "' --no-pager -o short-iso | grep -v '推送编码'"
    if args.pid is not None:
        command = "journalctl -u hs-game _PID="+str(args.pid)+" --since '"+args.since+"' --no-pager -o short-iso | grep -v '推送编码'"
    raw_log = destination / 'game.log'
    body = '\n'.join(line for line in raw_log.read_text(encoding='utf-8').splitlines() if '推送编码' not in line) if raw_log.exists() else run(client, command, timeout=90)
    (destination / 'events.log').write_text(body, encoding='utf-8')
    status['日志已保存'] = True
    save_status()
    errors = [line for line in body.splitlines() if any(word in line for word in ('失败', '异常', 'panic', 'fatal'))]
    from collections import Counter
    print('错误分类', json.dumps(Counter(re.search(r'entity_message (\w+) 业务失败', line).group(1) if re.search(r'entity_message (\w+) 业务失败', line) else '其他服务事件' for line in errors), ensure_ascii=False))
    print('\n'.join(line[:500] for line in errors if 'client_sa_log' not in line)[-17000:])
    if args.logs_only:
        status['状态'] = '日志采集完成'
        status['仅日志'] = True
        save_status()
        raise SystemExit(0)
    program = r'''
from pathlib import Path
from urllib.parse import urlsplit
import os, subprocess
raw = next(line.split('=',1)[1].strip().strip(chr(34)).strip(chr(39)) for line in Path('/opt/hs-server/data/database.env').read_text().splitlines() if line.startswith('HS_DATABASE_URL='))
u = urlsplit(raw)
env = dict(os.environ, PGHOST=u.hostname, PGPORT=str(u.port), PGUSER=u.username, PGPASSWORD=u.password, PGDATABASE=u.path.lstrip('/'), LD_LIBRARY_PATH='/opt/hs-server/pgsql/runtime/usr/lib/x86_64-linux-gnu')
sql = "SELECT coalesce(json_agg(json_build_object('uid',a.uid,'oid',encode(a.avatar_oid,'hex'),'hostnum',a.hostnum,'level',a.level,'created_at',a.created_at,'nickname_set',a.nickname_set,'state',p.state,'revision',p.revision,'updated_at',p.updated_at) ORDER BY a.uid),'[]'::json) FROM avatars a LEFT JOIN avatar_progress p USING(avatar_oid)"
result = subprocess.run(['/opt/hs-server/pgsql/runtime/usr/lib/postgresql/15/bin/psql','-X','-A','-t','-v','ON_ERROR_STOP=1','-c',sql], env=env, capture_output=True, text=True, check=True)
print(result.stdout.strip())
'''
    players = json.loads(run(client, "python3 - <<'PY'\n" + program + "\nPY", timeout=60))
    (destination / 'players.json').write_text(json.dumps(players, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    status['角色快照已保存'] = True
    status['状态'] = '采集完成'
    print('全角色数量', len(players))
    for player in players[:12]:
        state = player.get('state') or {}
        battle = state.get('server_battle') or {}
        active = {k: v for k,v in state.get('guide_tasks',{}).items() if v.get('task_status') != 2}
        print(json.dumps({'uid':player['uid'],'oid':player['oid'],'level':player['level'],'cards':len(state.get('cards',[])),'guides':active,'battle':{k:battle.get(k) for k in ('dungeon_id','ended','started','client_authority','uuid')},'updated_at':player['updated_at']},ensure_ascii=False))
except BaseException as error:
    if isinstance(error, SystemExit) and error.code == 0:
        raise
    status['状态'] = '部分采集失败' if status['日志已保存'] else '采集失败'
    # 只保存异常类型，避免远端错误文本带出授权环境或口令。
    status['异常类型'] = type(error).__name__
    raise
finally:
    status['结束时间'] = datetime.now().isoformat()
    save_status()
    if client is not None:
        client.close()
