"""只检查学会相关原生模块的成长引用，不扫描其它3099模块。"""
import json,pathlib,sys
sys.stdout.reconfigure(encoding='utf-8')
sys.path.insert(0,str(pathlib.Path(__file__).parent))
from mem_marshal_extract import Loader
root=pathlib.Path(__file__).resolve().parents[2]
inv=json.loads((root/'out/npk_scripts/android_inventory.json').read_text(encoding='utf-8'))
def txt(v):return v.decode('utf-8') if isinstance(v,bytes) else str(v)
for entry in inv['modules']:
 if 'league' not in entry['filename']:continue
 def walk(c,path=''):
  path=path+'/'+txt(c['name'])
  texts=[txt(x) for x in c['names']]+[txt(x) for x in c['consts'] if isinstance(x,(str,bytes))]
  hits=[x for x in texts if 'growth' in x]
  if hits:print(entry['hash'],entry['filename'],path,hits)
  for x in c['consts']:
   if isinstance(x,dict) and x.get('type')=='code':walk(x,path)
 walk(Loader((root/'out/npk_scripts'/entry['marshal_file']).read_bytes()).r_object())
