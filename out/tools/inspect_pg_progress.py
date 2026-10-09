"""只读查看当前隔离数据库核验日志末尾，不接触生产玩家。"""
import json
import re
import shlex
import sys
from server_ops import ROOT, connect, run
sys.stdout.reconfigure(encoding='utf-8')
report = json.loads((ROOT / 'out/remote-database-verification.json').read_text(encoding='utf-8'))
path = report['远端目录']
if not re.fullmatch(r'/opt/hs-server/data/verification/db-\d{8}-\d{6}', path):
    raise SystemExit('核验目录不匹配')
client = connect()
try:
    print(run(client, 'tail -n 8 ' + shlex.quote(path + '/test.log'), timeout=20))
finally:
    client.close()
