"""只展示指定原生函数和状态字段；输出均为UTF-8。"""
import json,sys
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
mode=sys.argv[1];names=sys.argv[2:]
if mode=='gosource':
 for p in (root/'internal').rglob('*.go'):
  lines=p.read_text(encoding='utf-8').splitlines()
  for i,line in enumerate(lines):
   if any(n in line for n in names): print(p.relative_to(root),i+1,line)
 sys.exit()
if mode=='tables':
 for p in (root/'out/client_catalogs/tables').glob('*.json'):
  if any(n in p.stem for n in names): print(p.stem)
 sys.exit()
if mode=='nativefind':
 from mem_marshal_extract import Loader
 from neox_dis import disasm
 inv=json.loads((root/'out/npk_scripts/android_inventory.json').read_text(encoding='utf-8'))
 for mod in inv['modules']:
  c=Loader((root/'out/npk_scripts'/mod['marshal_file']).read_bytes()).r_object()
  def visit(c,path=''):
   def text(x):return x.decode('utf-8',errors='replace') if isinstance(x,bytes) else str(x)
   local=path+'/'+text(c['name']);const=[text(x) for x in c['consts'] if isinstance(x,(bytes,str))]+[text(x) for x in c['names']]
   hits=[x for x in const if any(n in x for n in names)]
   if hits:
    print(mod['hash'],mod['filename'],local,hits)
    with (root/'out/dis'/('activity-'+mod['hash']+'.asm')).open('w',encoding='utf-8') as f:disasm(Loader((root/'out/npk_scripts'/mod['marshal_file']).read_bytes()).r_object(),out=f)
   for x in c['consts']:
    if isinstance(x,dict) and x.get('type')=='code':visit(x,local)
  visit(c)
 sys.exit()
if mode=='asmsearch':
 for p in (root/'out/dis').glob('*.asm'):
  try:lines=p.read_text(encoding='utf-8').splitlines()
  except UnicodeDecodeError:continue
  header=''
  for i,line in enumerate(lines):
   if line.startswith('code '):header=line
   if any(n in line for n in names):print(p.name,i+1,header,line)
 sys.exit()
if mode=='search':
 for p in (root/'out/client_catalogs/tables').glob('*.json'):
  obj=json.loads(p.read_text(encoding='utf-8')).get('数据',{})
  for k,v in (obj.items() if isinstance(obj,dict) else enumerate(obj)):
   st=json.dumps(v,ensure_ascii=False)
   if any(n in st for n in names): print(p.stem,k,st[:5000])
 sys.exit()
if mode=='ref':
 import ast
 for p in list((root/'_ref/GMA-Revive-Server-main').glob('*logic.py'))+[root/'_ref/GMA-Revive-Server-main/server.py']:
  source=p.read_text(encoding='utf-8');tree=ast.parse(source)
  for fn in ast.walk(tree):
   if isinstance(fn,(ast.FunctionDef,ast.AsyncFunctionDef)) and fn.name in names:
    print(p.name,'\n',ast.get_source_segment(source,fn))
 sys.exit()
if mode=='schema':
 from mem_marshal_extract import Loader
 from export_client_catalogs import instructions,text
 inv=json.loads((root/'out/npk_scripts/android_inventory.json').read_text(encoding='utf-8'))
 for mod in inv['modules']:
  if not any(n in mod['filename'] for n in names):continue
  c=Loader((root/'out/npk_scripts'/mod['marshal_file']).read_bytes()).r_object()
  def visit(c):
   ops=list(instructions(c));last=[]
   for i,(_,op,arg) in enumerate(ops):
    if op==116 and i>=5:
     recent=ops[max(0,i-9):i]; ns=[text(c['names'][a]) for _,o,a in recent if o==96]; const=[c['consts'][a] for _,o,a in recent if o==153]
     if 'prop' in ns:print(mod['hash'],text(c['name']),text(c['names'][arg]),[text(x) if isinstance(x,bytes) else x for x in const])
   for x in c['consts']:
    if isinstance(x,dict) and x.get('type')=='code':visit(x)
  visit(c)
 sys.exit()
if mode=='table':
 for name in names:
  name,*keys=name.split(':')
  obj=json.loads((root/'out/client_catalogs/tables'/(name+'.json')).read_text(encoding='utf-8'))['数据']
  if keys:
   if '键值条目' in obj:obj=[x for x in obj['键值条目'] if isinstance(x['键'],list) and str(x['键'][0]) in keys]
   else:obj={k:v for k,v in obj.items() if any((str(k)==q[1:]) if q.startswith('=') else (q in str(k) or q in json.dumps(v)[:100]) for q in keys)}
  print(name,json.dumps(obj,ensure_ascii=False))
else:
 for p in (root/'out/dis').glob('activity-*.asm'):
  lines=p.read_text(encoding='utf-8').splitlines();show=False
  for line in lines:
   if line.startswith('code '):
    show=any('/'+name+' ' in line or '/'+name+'/' in line or (mode=='props' and name in str(p)) for name in names)
   if show:print(line)
