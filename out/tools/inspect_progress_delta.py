# -*- coding: utf-8 -*-
"""递归打印完整进度断言的精确差异，不输出未变化的大块存档。"""
import json,sys
from pathlib import Path
root=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
def compare(a,b,path):
    if a==b:return
    if isinstance(a,dict) and isinstance(b,dict):
        for key in sorted(a.keys()|b.keys()):compare(a.get(key),b.get(key),path+'/'+key)
    else:print(json.dumps({'路径':path,'之前':a,'之后':b},ensure_ascii=False))
for line in (root/'out/database-test.log').read_text(encoding='utf-8').splitlines():
    if '变化字段' not in line:continue
    field,values=line.split('变化字段',1)[1].split('：之前=',1)
    a,b=values.split('；之后=',1)
    compare(json.loads(a),json.loads(b),field)
