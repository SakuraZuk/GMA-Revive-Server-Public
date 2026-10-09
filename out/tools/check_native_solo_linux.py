"""独立Linux真实原生专项；只写项目私有验证目录，绑定上传文件哈希。"""
from pathlib import Path
import datetime,hashlib,json,os,shlex,subprocess,sys
from server_ops import connect,run,upload_resumable
from build_completion import source_hashes
root=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
stamp=datetime.datetime.now().strftime('%Y%m%d-%H%M%S')
started=datetime.datetime.now().isoformat()
before=source_hashes()
previous=json.loads((root/'out/native-pvp-linux-verification.json').read_text(encoding='utf-8'))
base=previous['远端目录']
if not base.startswith(('/opt/hs-server/data/verification/native-','/opt/hs-server/data/verification/native-solo-')) or '..' in base:raise ValueError('原生验证基础目录越界')
remote='/opt/hs-server/data/verification/native-'+stamp+'-solo'
local=root/'out/bin/native-solo-linux';local.mkdir(parents=True,exist_ok=True)
env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
for package,name in [('./internal/nativeengine','nativeengine.test'),('./internal/game','game-native.test')]:
 subprocess.run([__import__('os').environ.get('HS_GO', 'go'),'test','-c','-o',str(local/name),package],cwd=root,env=env,check=True)
archive=root/'out/native-pvp-engine-linux-resources.tar.gz'
client=connect()
try:
 pid=run(client,'systemctl show hs-game -p MainPID --value')
 if run(client,'readlink -m '+shlex.quote(remote))!=remote:raise ValueError('验证目录解析越界')
 run(client,'test ! -e '+shlex.quote(remote)+' && mkdir -m 700 '+shlex.quote(remote))
finally:client.close()
files=[(archive,remote+'/resources.tar.gz'),(local/'nativeengine.test',remote+'/nativeengine.test'),(local/'game-native.test',remote+'/game-native.test')]
client=connect()
try:
 reuse=run(client,'sha256sum '+shlex.quote(base+'/resources.tar.gz')).split()[0]==hashlib.sha256(archive.read_bytes()).hexdigest()
 if reuse:run(client,'cp '+shlex.quote(base+'/resources.tar.gz')+' '+shlex.quote(remote+'/resources.tar.gz'))
finally:client.close()
for source,dest in files:
 if source==archive and reuse:continue
 previous_hash=next((value for key,value in previous.get('输入SHA256',{}).items() if key.endswith('/'+source.name)),None)
 if previous_hash==hashlib.sha256(source.read_bytes()).hexdigest():
  client=connect()
  try:
   if run(client,'sha256sum '+shlex.quote(base+'/'+source.name)).split()[0]!=previous_hash:raise ValueError('原Linux专项产物已漂移')
   run(client,'cp '+shlex.quote(base+'/'+source.name)+' '+shlex.quote(dest))
  finally:client.close()
  continue
 upload_resumable(connect,source,dest)
hashes={dest:hashlib.sha256(source.read_bytes()).hexdigest() for source,dest in files}
client=connect()
try:
 for dest,digest in hashes.items():
  if run(client,'sha256sum '+shlex.quote(dest)).split()[0]!=digest:raise ValueError('Linux专项上传SHA不符')
 run(client,'tar -xzf '+shlex.quote(remote+'/resources.tar.gz')+' -C '+shlex.quote(remote)+' && cp -a '+shlex.quote(base+'/python2')+' '+shlex.quote(remote+'/python2')+' && mkdir -p '+shlex.quote(remote+'/workspace/internal/game')+' && chmod 700 '+shlex.quote(remote+'/nativeengine.test')+' '+shlex.quote(remote+'/game-native.test'),timeout=60)
 env_remote='HS_NATIVE_PVP_TEST_PYTHON3=/usr/bin/python3 HS_NATIVE_PVP_TEST_PYTHON2='+shlex.quote(remote+'/python2/run-python2')+' HS_NATIVE_PVP_TEST_WORKER='+shlex.quote(remote+'/workspace/internal/nativepvp/battle_native_worker.py')+' HS_NATIVE_PVP_TEST_DIRECTORY='+shlex.quote(remote+'/workspace/internal/nativepvp/runtime/native_engine')+' HS_NATIVE_PVP_TEST_RESOURCE_SHA256=bed95c22e6e6adf4ccf01cabb5156b4888ccc5744f0e192d24501e6df3c19ef3'
 # run-python2入口按自身目录定位，复制不引用旧工作树。
 logs=[]
 for binary,pattern in [('nativeengine.test','Test'),('game-native.test','TestNativeSolo|TestNativePvpRoom')]:
  command='cd '+shlex.quote(remote+'/workspace/internal/game')+' && '+env_remote+' '+shlex.quote(remote+'/'+binary)+' -test.run '+shlex.quote(pattern)+' -test.v -test.count=1 -test.timeout=90s'
  logs.append(run(client,command,timeout=110))
 command='cd '+shlex.quote(remote+'/workspace')+' && HS_NATIVE_PVP_PYTHON2='+shlex.quote(remote+'/python2/run-python2')+' /usr/bin/python3 out/tools/test_remaining_activity_native.py'
 logs.append(run(client,command,timeout=60))
 logs.append(run(client,'cd '+shlex.quote(remote+'/workspace')+' && /usr/bin/python3 out/tools/test_activity_metrics_script.py',timeout=30))
 after=run(client,'systemctl show hs-game -p MainPID --value')
finally:client.close()
log=root/('out/native-solo-linux-'+stamp+'.log');log.write_text('\n'.join(logs),encoding='utf-8')
unchanged=before==source_hashes()
if not unchanged:raise ValueError('Linux原生专项期间源码变化，日志保留，拒绝作为最终通过')
if any('--- SKIP:' in output for output in logs):raise ValueError('Linux专项有跳过，不能作为完整通过')
report={'开始时间':started,'结束时间':datetime.datetime.now().isoformat(),'状态':'通过','远端目录':remote,'输入SHA256':hashes,'源码哈希':before,'源码专项期间一致':unchanged,'SKIP数量':0,'日志':str(log.relative_to(root)),'游戏PID前后':[pid,after],'范围':'项目私有验证；原生引擎与单端/双端房间专项，未发布生产、未操作Android'}
(root/'out/native-solo-linux-verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
# 原数据库工具读取此运行时入口；保留上一报告在本批备份。
(root/'out/native-pvp-linux-verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
(root/'out/native-authority-linux-go-verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({key:value for key,value in report.items() if key!='源码哈希'},ensure_ascii=False,indent=2))
