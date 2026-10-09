"""只读审计本项目资源和数据库会话，不输出凭据或其他项目进程参数。"""
import json
import sys
from server_ops import ROOT, connect, run
sys.stdout.reconfigure(encoding='utf-8')
command = r'''python3 - <<'PY'
import json, os, subprocess
from pathlib import Path
owned=[]
for entry in Path('/proc').iterdir():
    if not entry.name.isdigit(): continue
    try:
        exe=os.readlink(str(entry/'exe'))
        cwd=os.readlink(str(entry/'cwd'))
        if not (exe.startswith('/opt/hs-server/') or cwd.startswith('/opt/hs-server/')): continue
        status=(entry/'status').read_text()
        fields={line.split(':',1)[0]:line.split(':',1)[1].strip() for line in status.splitlines() if ':' in line}
        owned.append({'pid':entry.name,'exe':exe,'cwd':cwd,'name':fields.get('Name'),'rss':fields.get('VmRSS'),'threads':fields.get('Threads')})
    except (OSError,ValueError): pass
print(json.dumps({'owned':owned,'memory':[line for line in Path('/proc/meminfo').read_text().splitlines() if line.startswith(('MemTotal:','MemAvailable:'))],'load':Path('/proc/loadavg').read_text().strip()},ensure_ascii=False))
game_pid=subprocess.check_output(['systemctl','show','hs-game','-p','MainPID','--value'],text=True).strip()
print(subprocess.check_output(['ps','-p',game_pid,'-o','pid,pcpu,pmem,etimes'],text=True))
query="SELECT json_build_object('max_connections',current_setting('max_connections'),'reserved',current_setting('superuser_reserved_connections'),'sessions',(SELECT json_agg(json_build_object('pid',pid,'user',usename,'app',application_name,'state',state,'wait_type',wait_event_type,'wait',wait_event)) FROM pg_stat_activity WHERE datname='hs'))"
env=dict(os.environ,LD_LIBRARY_PATH='/opt/hs-server/pgsql/runtime/usr/lib/x86_64-linux-gnu')
args=['runuser','-u','hspg','--','/opt/hs-server/pgsql/runtime/usr/lib/postgresql/15/bin/psql','-h','/opt/hs-server/pgsql/socket','-p','15432','-d','hs','-X','-A','-t','-c',query]
result=subprocess.run(args,env=env,capture_output=True,text=True,timeout=30)
print(result.stdout if result.returncode==0 else result.stderr)
PY'''
client = connect()
try:
    body = run(client, command, timeout=120)
    path = ROOT / 'out/evidence/player-bugs-20261009/capacity.log'
    path.write_text(body, encoding='utf-8')
    print(body)
finally:
    client.close()
