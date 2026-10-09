import sys,json
from pathlib import Path
from server_ops import connect,run
sys.stdout.reconfigure(encoding='utf-8')
c=connect()
try:
 text=run(c,"python3 - <<'PY'\nimport json\nfrom pathlib import Path\np=Path('/opt/hs-server/tls/lego-data')\nfor f in (p/'certificates').glob('*.json'):\n d=json.loads(f.read_text());print(json.dumps({k:d[k] for k in ['domain','certUrl','certStableUrl'] if k in d}))\nprint('账户元数据数量',len(list((p/'accounts').rglob('account.json'))))\nprint('证书文件',','.join(f.name for f in (p/'certificates').glob('*')))\nPY")
 version=run(c,'/opt/hs-server/tls/lego --version')
 Path('./out/lego-audit.log').write_text(text+'\n'+version,encoding='utf-8');print(text);print(version)
finally:c.close()
