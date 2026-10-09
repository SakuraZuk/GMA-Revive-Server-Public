"""运行活动剩余组定向Go验收并写可引用日志；PG仍由真实数据库专项验收。"""
from pathlib import Path
import datetime,hashlib,json,subprocess,sys
root=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
command=[r__import__('os').environ.get('HS_GO', 'go'),'test','./internal/game','-run','TestRemainingActivity|TestActivityMikuActualRPCBattleReceiptAndLeave','-count=1','-v']
started=datetime.datetime.now().isoformat()
result=subprocess.run(command,cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
log=root/'out/remaining-activity-go-verification.log';log.write_bytes(result.stdout)
report={'开始时间':started,'结束时间':datetime.datetime.now().isoformat(),'退出码':result.returncode,'命令':command,'PASS数量':result.stdout.count(b'--- PASS:'),'日志SHA256':hashlib.sha256(result.stdout).hexdigest(),'边界':'本地Go活动实现测试；不是PG、远端发布或Android实机验收'}
(root/'out/remaining-activity-go-verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
print(json.dumps(report,ensure_ascii=False));print(result.stdout.decode('utf-8'))
raise SystemExit(result.returncode)
