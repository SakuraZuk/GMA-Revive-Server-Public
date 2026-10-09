# -*- coding: utf-8 -*-
"""保存完整Go验证证据，只在控制台展示失败与摘要。"""
from pathlib import Path
import subprocess, sys, json, re
sys.stdout.reconfigure(encoding='utf-8');sys.stderr.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
go=Path(__import__('os').environ.get('HS_GO', 'go'))
target=sys.argv[1] if len(sys.argv)>1 else 'all'
args={'game':['test','./internal/game','-count=1'],'all':['test','./...','-count=1'],'vet':['vet','./...']}[target]
result=subprocess.run([str(go)]+args,cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
body=result.stdout.decode('utf-8')
path=root/'out'/('completion-'+target+'-test.log')
path.write_text(body,encoding='utf-8')
lines=body.splitlines()
selected=set()
for i,line in enumerate(lines):
    if line.startswith(('--- FAIL','FAIL','ok ','? ')) or any(word in line for word in ('undefined:','build failed','syntax error','panic:')):
        selected.update(range(i,min(i+5,len(lines))))
for i in sorted(selected):print(lines[i][:900] + (' …完整内容见日志' if len(lines[i])>900 else ''))
print(json.dumps({'退出码':result.returncode,'日志':str(path)},ensure_ascii=False))
raise SystemExit(result.returncode)
