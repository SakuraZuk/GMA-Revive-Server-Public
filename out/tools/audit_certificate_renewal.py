import sys,json
from pathlib import Path
from server_ops import connect,run
sys.stdout.reconfigure(encoding='utf-8')
c=connect()
try:
 text=run(c,"python3 - <<'PY'\nfrom pathlib import Path\nfor p in Path('/etc/letsencrypt/renewal').glob('*.conf'):\n s=p.read_text()\n if 'service0000002.r18sex.net' not in s:continue\n print('配置='+str(p))\n for l in s.splitlines():\n  if '=' in l and l.split('=',1)[0].strip() in ['authenticator','installer','server','pre_hook','post_hook','renew_hook','webroot_path','cert','chain','fullchain']:\n   print(l)\nfor base in ['/etc/letsencrypt/renewal-hooks/pre','/etc/letsencrypt/renewal-hooks/post','/etc/letsencrypt/renewal-hooks/deploy']:\n for p in Path(base).glob('*'):\n  if p.is_file():\n   s=p.read_text()\n   if 'hs-sdk' in s or '/opt/hs-server' in s: print('项目钩子='+str(p)+'\\n'+s)\nPY")
 Path('./out/certificate-renewal-audit.log').write_text(text,encoding='utf-8');print(text)
finally:c.close()
