import sys,json
from pathlib import Path
from server_ops import connect,run
sys.stdout.reconfigure(encoding='utf-8')
c=connect()
try:
 body=run(c,"python3 - <<'PY'\nfrom pathlib import Path\nfor base in ['/opt/hs-server/tls','/opt/hs-server/data','/etc/letsencrypt','/root/.acme.sh','/root/.lego']:\n p=Path(base)\n if p.exists():\n  print('目录='+base)\n  for x in p.iterdir():print(x.name+(' [目录]' if x.is_dir() else ' [文件]'))\nPY")
 Path('./out/certificate-path-audit.log').write_text(body,encoding='utf-8');print(body)
finally:c.close()
