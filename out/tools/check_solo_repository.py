"""接续全仓检查；按顺序执行并记录真实退出码，不把SKIP当原生实测。"""
from pathlib import Path
import datetime,json,subprocess,sys
root=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
commands=[[r__import__('os').environ.get('HS_GO', 'go'),'test','./...','-count=1'],[r__import__('os').environ.get('HS_GO', 'go'),'vet','./...'],[sys.executable,'out/tools/verify_battle_bridge_hotfix.py']]
rows=[]
for index,command in enumerate(commands):
 started=datetime.datetime.now().isoformat()
 result=subprocess.run(command,cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 log=root/('out/solo-repository-'+datetime.datetime.now().strftime('%Y%m%d-%H%M%S')+'-'+str(index)+'.log')
 log.write_bytes(result.stdout)
 rows.append({'命令':command,'开始时间':started,'结束时间':datetime.datetime.now().isoformat(),'退出码':result.returncode,'日志':str(log.relative_to(root))})
 if result.returncode:print(result.stdout.decode('utf-8'));break
report={'时间':datetime.datetime.now().isoformat(),'状态':'通过' if len(rows)==len(commands) and all(r['退出码']==0 for r in rows) else '失败','检查':rows,'边界':'全仓缺省PG与原生专项可能SKIP；真实Linux/PG单独记录'}
(root/'out/continuation-go-checks.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps(report,ensure_ascii=False,indent=2))
raise SystemExit(0 if report['状态']=='通过' else 1)
