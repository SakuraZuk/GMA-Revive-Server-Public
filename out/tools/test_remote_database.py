"""断线可恢复的真实数据库验收，独立产物、日志、退出码和哈希。"""
import hashlib,json,shlex,sys,time,paramiko
from datetime import datetime
from pathlib import Path
from server_ops import connect,run,upload_resumable
from build_completion import source_hashes
from upload_database_delta import upload_database_delta
sys.stdout.reconfigure(encoding='utf-8');sys.stderr.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
local=root/'out/bin/linux-amd64/dbstore.test';sha=hashlib.sha256(local.read_bytes()).hexdigest()
build=json.loads((root/'out/completion-build.json').read_text(encoding='utf-8'))
if build.get('退出码')!=0 or build.get('源码哈希')!=source_hashes() or build.get('产物哈希',{}).get('out/bin/linux-amd64/dbstore.test')!=sha:
 raise SystemExit('当前源码/数据库产物与已完成的正式构建不一致，拒绝验收')
catalog=root/'deploy/data/hotfix.json';catalog_sha=hashlib.sha256(catalog.read_bytes()).hexdigest()
if build['源码哈希'].get('deploy/data/hotfix.json')!=catalog_sha:raise SystemExit('测试热更配置未绑定正式构建')
policy=root/'deploy/data/gameplay-rules.env'
policy_sha=hashlib.sha256(policy.read_bytes()).hexdigest() if policy.exists() else None
if policy_sha is not None and build['源码哈希'].get('deploy/data/gameplay-rules.env')!=policy_sha:
 raise SystemExit('正式运营规则未绑定构建，拒绝数据库验收')
native_report=json.loads((root/'out/native-pvp-linux-verification.json').read_text(encoding='utf-8'))
native_base=native_report.get('远端目录','')
if native_report.get('状态')!='通过' or not native_base.startswith('/opt/hs-server/data/verification/native-') or '..' in native_base:
 raise SystemExit('真实数据库验收缺少已核验的项目私有Linux原生运行时')
stamp=datetime.now().strftime('%Y%m%d-%H%M%S')
remote='/opt/hs-server/data/verification/db-'+stamp
client=connect()
try:run(client,'mkdir -p '+remote+'/workspace/internal/game/dbstore '+remote+'/workspace/deploy/data && chmod 700 '+remote)
finally:client.close()
if not upload_database_delta(local,remote+'/dbstore.test'):
 upload_resumable(connect,local,remote+'/dbstore.test')
upload_resumable(connect,catalog,remote+'/workspace/deploy/data/hotfix.json')
if policy_sha is not None:upload_resumable(connect,policy,remote+'/workspace/deploy/data/gameplay-rules.env')
runner='''import os,json,subprocess,hashlib,time,shlex
from pathlib import Path
root=Path(__file__).parent
expected=EXPECTED
expected_catalog=EXPECTED_CATALOG
expected_policy=EXPECTED_POLICY
binary=root/'dbstore.test'
actual=hashlib.sha256(binary.read_bytes()).hexdigest()
catalog_actual=hashlib.sha256((root/'workspace/deploy/data/hotfix.json').read_bytes()).hexdigest()
policy_path=root/'workspace/deploy/data/gameplay-rules.env'
policy_actual=hashlib.sha256(policy_path.read_bytes()).hexdigest() if expected_policy is not None else None
if actual!=expected or catalog_actual!=expected_catalog or policy_actual!=expected_policy:
 (root/'exit.json').write_text(json.dumps({'code':125,'sha256':actual,'hotfix_sha256':catalog_actual,'policy_sha256':policy_actual,'error':'验收产物、热更配置或正式运营规则哈希不一致'}))
 raise SystemExit(125)
env=os.environ.copy()
native_base=NATIVE_BASE
native_python=native_base+'/python2/run-python2'
native_worker=native_base+'/workspace/internal/nativepvp/battle_native_worker.py'
native_directory=native_base+'/workspace/internal/nativepvp/runtime/native_engine'
for native_path in (native_python,native_worker,native_directory):
 if not Path(native_path).exists():raise ValueError('PVP原生验收运行时缺失：'+native_path)
env['HS_NATIVE_PVP_TEST_PYTHON2']=native_python
env['HS_NATIVE_PVP_TEST_WORKER']=native_worker
env['HS_NATIVE_PVP_TEST_DIRECTORY']=native_directory
env['HS_NATIVE_PVP_TEST_RESOURCE_SHA256']='bed95c22e6e6adf4ccf01cabb5156b4888ccc5744f0e192d24501e6df3c19ef3'
if expected_policy is not None:
 for line in policy_path.read_text(encoding='utf-8').splitlines():
  if not line.strip() or line.lstrip().startswith('#'):continue
  key,separator,value=line.partition('=')
  if not separator or key not in {'HS_RUNE_FALLBACK_POOLS','HS_FURNITURE_FUSION_POOLS','HS_SERVER_OPEN_TIME','HS_REMAINING_GAMEPLAY_POLICY','HS_SPECIAL_DRILL_REOPEN','HS_NEW_AVATAR_ALL_HEROES'}:
   raise ValueError('测试正式运营变量不属于批准范围')
  words=shlex.split(value)
  if len(words)!=1:raise ValueError('测试正式运营值无效')
  env[key]=words[0]
for line in Path('/opt/hs-server/data/database.env').read_text().splitlines():
 if line.startswith('HS_DATABASE_URL='):
  env['HS_TEST_DATABASE_URL']=shlex.split(line.split('=',1)[1])[0]
(root/'started.json').write_text(json.dumps({'pid':os.getpid(),'time':time.time(),'sha256':actual}))
try:
 with (root/'test.log').open('wb') as log:
   result=subprocess.run([str(binary),'-test.v','-test.timeout=180s'],cwd=root/'workspace/internal/game/dbstore',env=env,stdout=log,stderr=subprocess.STDOUT,timeout=190)
 code=result.returncode
except subprocess.TimeoutExpired:code=124
(root/'exit.json').write_text(json.dumps({'code':code,'sha256':actual,'hotfix_sha256':catalog_actual,'policy_sha256':policy_actual,'time':time.time()}))
'''.replace('EXPECTED_CATALOG',repr(catalog_sha)).replace('EXPECTED_POLICY',repr(policy_sha)).replace('NATIVE_BASE',repr(native_base)).replace('EXPECTED',repr(sha))
client=connect()
try:
 with client.open_sftp() as sftp:
  with sftp.open(remote+'/runner.py','wb') as f:f.write(runner.encode('utf-8'))
  sftp.chmod(remote+'/dbstore.test',0o755)
 run(client,'nohup python3 '+remote+'/runner.py >'+remote+'/runner-launch.log 2>&1 </dev/null &')
finally:client.close()
report={'远端目录':remote,'SHA256':sha,'热更配置SHA256':catalog_sha,'正式规则SHA256':policy_sha,'原生运行时':native_base,'工作目录':remote+'/workspace/internal/game/dbstore','状态':'执行中'}
(root/'out/remote-database-verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
client=None
sftp=None
for attempt in range(100):
 time.sleep(5)
 try:
  # 核验期间复用通道，避免每两秒重新握手争抢共享主机CPU。
  if client is None:
   client=connect()
   sftp=client.open_sftp()
  try:
   with sftp.open(remote+'/exit.json') as f:status=json.loads(f.read())
  except FileNotFoundError:continue
  try:
   with sftp.open(remote+'/test.log') as f:body=f.read().decode('utf-8')
  except FileNotFoundError:
   body='验收日志尚未创建：'+str(status.get('error','远端启动前拒绝'))+'\n'
 except paramiko.AuthenticationException:
  raise
 except (OSError,paramiko.SSHException,EOFError):
  if client is not None:client.close()
  client=None;sftp=None
  continue
 sftp.close();client.close();sftp=None;client=None
 passed_names=[line.split()[2] for line in body.splitlines() if line.startswith('--- PASS:')]
 passes=sum(name.startswith('TestPostgres') for name in passed_names)
 other_passes=[name for name in passed_names if not name.startswith('TestPostgres')]
 skips=sum(x.startswith('--- SKIP:') for x in body.splitlines())
 complete=status['code']==0 and passes>0 and skips==0 and status['sha256']==sha and status.get('hotfix_sha256')==catalog_sha and status.get('policy_sha256')==policy_sha
 report.update({'状态':'通过' if complete else '失败或未完整执行','退出码':status['code'],'远端SHA256':status['sha256'],'远端热更配置SHA256':status.get('hotfix_sha256'),'远端正式规则SHA256':status.get('policy_sha256'),'PASS数量':passes,'SKIP数量':skips,'其他PASS数量':len(other_passes),'其他PASS用例':other_passes,'远端启动错误':status.get('error')})
 (root/'out/database-test.log').write_text(body,encoding='utf-8')
 (root/'out/remote-database-verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
 print(json.dumps(report,ensure_ascii=False,indent=2))
 for i,line in enumerate(body.splitlines()):
  if line.startswith(('--- FAIL:','--- SKIP:','panic:')):
   print('\n'.join(body.splitlines()[i:i+5]))
 if not complete:raise SystemExit(1)
 break
else:raise SystemExit('远端验收尚未结束，保留目录，可重新查询日志与退出码')
