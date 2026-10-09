"""只读提取真实PG重复结果的成就字段差异。"""
from pathlib import Path
import json,sys
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
for line in (root/'out/database-test.log').read_text(encoding='utf-8').splitlines():
 if '变化字段achves：之前=' not in line:continue
 before,after=line.split('之前=',1)[1].split('；之后=',1)
 a,b=json.loads(before),json.loads(after)
 print(json.dumps({key:{'之前':a.get(key),'之后':b.get(key)} for key in set(a)|set(b) if a.get(key)!=b.get(key)},ensure_ascii=False,indent=2))
