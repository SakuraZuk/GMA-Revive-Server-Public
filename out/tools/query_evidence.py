# -*- coding: utf-8 -*-
"""检索静态目录的模块、方法和数据条目，输出中文 UTF-8 摘要。"""
import json,sys,re
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8');sys.stderr.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]/'out'
mode,pattern=sys.argv[1:3]
if mode=='modules':
 for row in json.loads((root/'npk_scripts/android_inventory.json').read_text(encoding='utf-8'))['modules']:
  if re.search(pattern,row['filename']):print(row['hash'],row['filename'],row['marshal_file'])
elif mode=='methods':
 for row in json.loads((root/'npk_scripts/android_inventory.json').read_text(encoding='utf-8'))['modules']:
  names=[method['name'] for method in row['methods'] if re.search(pattern,method['name'])]
  if names:print(row['hash'],row['filename'],', '.join(names))
elif mode=='rpc':
 for category,rows in json.loads((root/'client_catalogs/rpc-catalog.json').read_text(encoding='utf-8')).items():
  if not isinstance(rows,list):continue
  for row in rows:
   if re.search(pattern,row.get('方法',row.get('调用方法',''))):print(category,json.dumps(row,ensure_ascii=False))
elif mode=='table':
 value=json.loads((root/'client_catalogs/tables'/(pattern+'.json')).read_text(encoding='utf-8'))
 for key in sys.argv[3:]:print(json.dumps(value['数据'].get(key),ensure_ascii=False,indent=2))
elif mode=='records':
 value=json.loads((root/'client_catalogs/tables'/(pattern+'.json')).read_text(encoding='utf-8'))['数据']
 field=sys.argv[3];wanted={int(key) for key in sys.argv[4:]}
 def walk(item):
  if isinstance(item,dict):
   if isinstance(item.get(field),int) and item[field] in wanted:
    print(json.dumps(item,ensure_ascii=False,indent=2));return
   for child in item.values():walk(child)
  elif isinstance(item,list):
   for child in item:walk(child)
 walk(value)
