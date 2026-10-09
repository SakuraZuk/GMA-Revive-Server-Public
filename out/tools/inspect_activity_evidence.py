"""按Android目录提取活动原生证据，避免扫描外部参考数值。"""
import json,sys
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
catalog=json.loads((root/'out/client_catalogs/rpc-catalog.json').read_text(encoding='utf-8'))
print('RPC结构',type(catalog),list(catalog)[:10] if isinstance(catalog,dict) else catalog[:1])
names=['exam','summer','cthulhu','miku','mountain','wangyan','monster_nian','house_frage']
def walk(v):
 if isinstance(v,dict):
  if '方法' in v or '调用方法' in v:
   text=json.dumps(v,ensure_ascii=False)
   if any(n in text for n in names):print(text)
  for x in v.values():walk(x)
 elif isinstance(v,list):
  for x in v:walk(x)
if 'rpc' in sys.argv:walk(catalog)
for p in (root/'out/client_catalogs/tables').glob('*.json'):
 if not any(n in p.stem for n in names):continue
 if 'rpc' in sys.argv:continue
 data=json.loads(p.read_text(encoding='utf-8')); rows=data.get('数据')
 print('表',p.stem,'数量',len(rows) if rows is not None else 0,'样例',str(rows)[:450])
