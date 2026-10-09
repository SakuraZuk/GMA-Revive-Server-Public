# -*- coding: utf-8 -*-
"""六项尾部实库排错：复用根已校验产物，不覆盖正式全套验收报告。"""
import hashlib
import json
import shlex
import sys
import time
from pathlib import Path
from server_ops import connect, run

sys.stdout.reconfigure(encoding='utf-8')
root = Path(__file__).resolve().parents[2]
base = '/opt/hs-server/data/verification/db-20261008-143636'
expected = '38c66ae215b2e837ec8730c32f36924be35262c743e77d0f8350405ced1c5283'
verification = json.loads((root / 'out/remote-database-verification.json').read_text(encoding='utf-8'))
if verification['远端目录'] != base or verification['SHA256'] != expected:
    raise SystemExit('原全套报告已切换，拒绝复用未经本任务授权的目录')
tests = [
    'TestPostgresRemainingCollectionFramesFrozenResidentsEnergyTutorialAndRollback',
    'TestPostgresCollectionVisitLikesPairRollbackAndColdRetry',
    'TestPostgresLeagueActualMembersJanusNativeStatsThirtyReceiptsColdRetryAndTransfer',
    'TestPostgresRemainingMailRandomFrozenAtIssueClaimRollbackAndColdRetry',
    'TestPostgresRemainingStrangerAssistRosterLimitPairAndFrozenRetry',
    'TestPostgresRemainingSpecialDrillActualRPCThreeTasksAndColdRetry',
]
runner = '''import hashlib,json,os,re,shlex,subprocess,time
from pathlib import Path
base=Path(BASE)
expected=EXPECTED
policy_expected=POLICY_EXPECTED
catalog_expected=CATALOG_EXPECTED
native=Path(NATIVE)
tests=TESTS
report={'状态':'执行中','性质':'六项尾部独立实库排错，不替代55全套','开始时间':time.time(),'用例':[]}
report_path=base/'remaining-pg-tail-debug.json'
binary=base/'dbstore.test'
policy=base/'workspace/deploy/data/gameplay-rules.env'
catalog=base/'workspace/deploy/data/hotfix.json'
actual=hashlib.sha256(binary.read_bytes()).hexdigest()
policy_actual=hashlib.sha256(policy.read_bytes()).hexdigest()
catalog_actual=hashlib.sha256(catalog.read_bytes()).hexdigest()
if (actual,policy_actual,catalog_actual)!=(expected,policy_expected,catalog_expected):
 raise SystemExit('远端二进制、正式规则或热更哈希不符')
report.update({'二进制SHA256':actual,'正式规则SHA256':policy_actual,'热更SHA256':catalog_actual,'工作目录':str(base/'workspace/internal/game/dbstore'),'原生运行时':str(native)})
env=os.environ.copy()
allowed={'HS_RUNE_FALLBACK_POOLS','HS_FURNITURE_FUSION_POOLS','HS_SERVER_OPEN_TIME','HS_REMAINING_GAMEPLAY_POLICY','HS_SPECIAL_DRILL_REOPEN'}
for line in policy.read_text(encoding='utf-8').splitlines():
 if not line.strip() or line.lstrip().startswith('#'):continue
 key,sep,value=line.partition('=')
 words=shlex.split(value)
 if not sep or key not in allowed or len(words)!=1:raise ValueError('正式规则不属于已批准环境')
 env[key]=words[0]
if env.get('HS_REMAINING_GAMEPLAY_POLICY')!='local-20261008-v1':raise ValueError('正式政策未启用')
found=False
for line in Path('/opt/hs-server/data/database.env').read_text().splitlines():
 if line.startswith('HS_DATABASE_URL='):
  words=shlex.split(line.split('=',1)[1])
  if len(words)!=1:raise ValueError('数据库环境格式无效')
  env['HS_TEST_DATABASE_URL']=words[0];found=True
if not found:raise ValueError('缺少授权项目数据库环境')
env['HS_NATIVE_PVP_TEST_PYTHON2']=str(native/'python2/run-python2')
env['HS_NATIVE_PVP_TEST_WORKER']=str(native/'workspace/internal/nativepvp/battle_native_worker.py')
env['HS_NATIVE_PVP_TEST_DIRECTORY']=str(native/'workspace/internal/nativepvp/runtime/native_engine')
env['HS_NATIVE_PVP_TEST_RESOURCE_SHA256']='bed95c22e6e6adf4ccf01cabb5156b4888ccc5744f0e192d24501e6df3c19ef3'
for key in ('HS_NATIVE_PVP_TEST_PYTHON2','HS_NATIVE_PVP_TEST_WORKER','HS_NATIVE_PVP_TEST_DIRECTORY'):
 if not Path(env[key]).exists():raise ValueError('原生运行时缺失')
report_path.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\\n',encoding='utf-8')
with (base/'remaining-pg-tail-debug.log').open('wb') as log:
 for name in tests:
  start=time.time()
  try:
   result=subprocess.run([str(binary),'-test.v','-test.timeout=120s','-test.run=^'+name+'$'],cwd=base/'workspace/internal/game/dbstore',env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=130)
   body=result.stdout;code=result.returncode
  except subprocess.TimeoutExpired as exc:
   body=exc.stdout or b'';code=124
  # 仅保存排错输出；任何意外口令/连接串先脱敏。
  body=body.replace(env['HS_TEST_DATABASE_URL'].encode(),'[数据库连接已脱敏]'.encode())
  log.write(body);log.flush()
  text=body.decode('utf-8',errors='replace')
  passed=sum(line.startswith('--- PASS: '+name+' ') for line in text.splitlines())
  failed=sum(line.startswith('--- FAIL: '+name+' ') for line in text.splitlines())
  skipped=sum(line.startswith('--- SKIP:') for line in text.splitlines())
  report['用例'].append({'名称':name,'退出码':code,'状态':'PASS' if code==0 and passed==1 and skipped==0 else 'FAIL','PASS数量':passed,'FAIL数量':failed,'SKIP数量':skipped,'耗时秒':round(time.time()-start,3)})
  report_path.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\\n',encoding='utf-8')
report.update({'状态':'PASS' if all(row['状态']=='PASS' for row in report['用例']) else 'FAIL','结束时间':time.time(),'PASS数量':sum(row['状态']=='PASS' for row in report['用例']),'SKIP数量':sum(row['SKIP数量'] for row in report['用例']),'日志SHA256':hashlib.sha256((base/'remaining-pg-tail-debug.log').read_bytes()).hexdigest()})
report_path.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\\n',encoding='utf-8')
'''
for key, value in {'policy_expected=POLICY_EXPECTED':verification['正式规则SHA256'],'catalog_expected=CATALOG_EXPECTED':verification['热更配置SHA256'],'expected=EXPECTED':expected,'native=Path(NATIVE)':verification['原生运行时'],'tests=TESTS':tests,'base=Path(BASE)':base}.items():
    target = key.split('=')[0] + '='
    runner = runner.replace(key, target + ('Path(' + repr(value) + ')' if key in ('native=Path(NATIVE)', 'base=Path(BASE)') else repr(value)))
client = connect()
try:
    binary_actual = run(client, 'sha256sum ' + shlex.quote(base + '/dbstore.test')).split()[0]
    policy_actual = run(client, 'sha256sum ' + shlex.quote(base + '/workspace/deploy/data/gameplay-rules.env')).split()[0]
    if binary_actual != expected or policy_actual != verification['正式规则SHA256']:
        raise SystemExit('启动前远端二进制或正式规则SHA不符')
    with client.open_sftp() as sftp:
        with sftp.open(base + '/remaining-pg-tail-runner.py', 'wb') as handle:
            handle.write(runner.encode('utf-8'))
    run(client, 'nohup python3 ' + shlex.quote(base + '/remaining-pg-tail-runner.py') + ' >' + shlex.quote(base + '/remaining-pg-tail-launch.log') + ' 2>&1 </dev/null &')
finally:
    client.close()
print('已校验二进制/正式规则SHA并启动六项私有实库排错', flush=True)
last_count = -1
for attempt in range(360):
    time.sleep(2)
    client = connect()
    try:
        with client.open_sftp() as sftp:
            try:
                with sftp.open(base + '/remaining-pg-tail-debug.json') as handle:
                    report = json.loads(handle.read())
            except FileNotFoundError:
                if attempt > 5:
                    raise RuntimeError('尾部排错启动未生成状态，请检查私有启动日志')
                continue
            count = len(report['用例'])
            if count != last_count:
                print(json.dumps({'已执行数量':count,'最新':report['用例'][-1] if count else None},ensure_ascii=False), flush=True)
                last_count = count
            if report['状态'] == '执行中':
                continue
            with sftp.open(base + '/remaining-pg-tail-debug.log') as handle:
                body = handle.read()
    finally:
        client.close()
    (root / 'out/remaining-pg-tail-debug.log').write_bytes(body)
    (root / 'out/remaining-pg-tail-debug.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(report,ensure_ascii=False,indent=2))
    raise SystemExit(0 if report['状态']=='PASS' else 1)
raise SystemExit('专项仍执行中，远端私有报告保留')
