"""导出本版异步AI战斗习惯的原生读取契约。"""
from pathlib import Path
import sys,json,hashlib
sys.path.insert(0,str(Path(__file__).parent))
from mem_marshal_extract import Loader
from neox_dis import disasm
root=Path(__file__).resolve().parents[2]
inventory=json.loads((root/'out/npk_scripts/android_inventory.json').read_text(encoding='utf-8'))
output=root/'out/dis/native-solo-ai-contract.asm'
with output.open('w',encoding='utf-8',newline='\n') as handle:
 for digest,names in [('51169E3F',{'auto_battle_habit'}),('2CC3F05D',{'set_battle_candidate'}),('AE9F02E4',{'__init__','load'})]:
  row=next(v for v in inventory['modules'] if v['hash']==digest)
  path=root/'out/npk_scripts'/row['marshal_file']
  print('本版原生模块：',row['filename'],'SHA256：',hashlib.sha256(path.read_bytes()).hexdigest(),file=handle)
  def walk(node):
   name=node['name'].decode('utf-8') if isinstance(node['name'],bytes) else node['name']
   if name in names: disasm(node,out=handle);return
   for child in node['consts']:
    if isinstance(child,dict) and child.get('type')=='code':walk(child)
  walk(Loader(path.read_bytes()).r_object())
sys.stdout.reconfigure(encoding='utf-8')
print(output.read_text(encoding='utf-8'))
