# -*- coding: utf-8 -*-
"""只读查看项目私有数据库验证目录的产物大小和启动状态，不读取连接参数。"""
import json, sys
from server_ops import connect, run
sys.stdout.reconfigure(encoding='utf-8')
client=connect()
try:
    program='''import json,pathlib
root=pathlib.Path('/opt/hs-server/data/verification')
rows=[]
for p in sorted(root.glob('db-*'))[-3:]:
 row={'目录':str(p),'产物':{name:(p/name).stat().st_size for name in ['dbstore.test','dbstore.test.part','test.log'] if (p/name).exists()}}
 for name in ['started.json','exit.json']:
  if (p/name).exists():row[name]=json.loads((p/name).read_text())
 rows.append(row)
print(json.dumps(rows,ensure_ascii=False))'''
    import shlex
    print(run(client,'python3 -c '+shlex.quote(program)))
finally:client.close()
