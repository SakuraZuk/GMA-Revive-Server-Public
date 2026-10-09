# -*- coding: utf-8 -*-
"""独立只读核验本批PVP运行时、双主机哈希与进程，仅读取原生运行时四个环境变量。"""
import datetime, hashlib, json, re, shlex, sys
from server_ops import ROOT, connect, run, audit
from deploy_hotfix_bridge import connect as connect_hotfix, audit as audit_hotfix
sys.stdout.reconfigure(encoding='utf-8')
def report(name):return json.loads((ROOT/'out'/name).read_text(encoding='utf-8'))
build=report('completion-build.json');runtime=report('native-pvp-production-runtime.json')
release=report('server-release.json')
hotfix_sha=hashlib.sha256((ROOT/'deploy/data/hotfix.json').read_bytes()).hexdigest()
client=connect()
result={'时间':datetime.datetime.now().isoformat()}
try:
    result['游戏审计']=audit(client)
    assert run(client,'systemctl is-active hs-game')=='active'
    game_sha=run(client,'sha256sum /opt/hs-server/bin/gameserver').split()[0]
    assert game_sha==build['产物哈希']['out/bin/linux-amd64/gameserver']==release['SHA256']
    assert run(client,'sha256sum /opt/hs-server/data/hotfix.json').split()[0]==hotfix_sha
    assert run(client,'sha256sum /etc/systemd/system/hs-game.service.d/60-native-pvp.conf').split()[0]==build['源码哈希']['deploy/hs-game-native-pvp.conf']
    assert run(client,'readlink /opt/hs-server/native-pvp/current')==runtime['当前链接']
    assert run(client,'readlink -f /opt/hs-server/native-pvp/current')==runtime['目录']
    for name in ['battle_native.py','battle_native_host.py','battle_native_worker.py','sync_pvp_authority.py','activity_buffs_native.py']:
        remote=runtime['目录']+'/workspace/internal/nativepvp/'+name
        assert run(client,'sha256sum '+shlex.quote(remote)).split()[0]==build['源码哈希']['internal/nativepvp/'+name]
    pid=run(client,'systemctl show hs-game -p MainPID --value')
    assert pid.isdigit() and int(pid)>0
    assert run(client,'readlink /proc/'+pid+'/exe')=='/opt/hs-server/bin/gameserver'
    assert run(client,'sha256sum /proc/'+pid+'/exe').split()[0]==game_sha
    program='import pathlib; print("\\n".join(v.decode() for v in pathlib.Path("/proc/'+pid+'/environ").read_bytes().split(bytes([0])) if v.startswith(b"HS_NATIVE_PVP_")))'
    env=run(client,'python3 -c '+shlex.quote(program)).splitlines()
    expected={
        'HS_NATIVE_PVP_DIRECTORY':'/opt/hs-server/native-pvp/current/workspace/internal/nativepvp/runtime/native_engine',
        'HS_NATIVE_PVP_PYTHON':'/opt/hs-server/native-pvp/current/python2/run-python2',
        'HS_NATIVE_PVP_WORKER':'/opt/hs-server/native-pvp/current/workspace/internal/nativepvp/battle_native_worker.py',
        'HS_NATIVE_PVP_RESOURCE_SHA256':'bed95c22e6e6adf4ccf01cabb5156b4888ccc5744f0e192d24501e6df3c19ef3'}
    assert dict(v.split('=',1) for v in env)==expected
    policy_program='import pathlib; print("\\n".join(v.decode() for v in pathlib.Path("/proc/'+pid+'/environ").read_bytes().split(bytes([0])) if v.startswith((b"HS_REMAINING_GAMEPLAY_POLICY=",b"HS_SPECIAL_DRILL_REOPEN="))))'
    policy_env=run(client,'python3 -c '+shlex.quote(policy_program)).splitlines()
    assert dict(v.split('=',1) for v in policy_env)=={'HS_REMAINING_GAMEPLAY_POLICY':'local-20261008-v1','HS_SPECIAL_DRILL_REOPEN':'timber-12-20261008'}
    hero_program='import pathlib; print("\\n".join(v.decode() for v in pathlib.Path("/proc/'+pid+'/environ").read_bytes().split(bytes([0])) if v.startswith(b"HS_NEW_AVATAR_ALL_HEROES=")))'
    hero_env=run(client,'python3 -c '+shlex.quote(hero_program)).splitlines()
    hero_expected=[line for line in (ROOT/'deploy/data/gameplay-rules.env').read_text(encoding='utf-8').splitlines() if line.startswith('HS_NEW_AVATAR_ALL_HEROES=')]
    assert len(hero_expected)==1 and hero_env==hero_expected, '新角色全英雄开关未加载到游戏进程'
    result['新角色全英雄环境']=hero_env
    policy_sha=build['源码哈希']['deploy/data/gameplay-rules.env']
    assert run(client,'sha256sum /opt/hs-server/data/gameplay-rules.env').split()[0]==policy_sha
    runtime_index=int(run(client,'python3 -c '+shlex.quote('import json; print(json.load(open("/opt/hs-server/data/hotfix.json"))["runtime"]["index"])')))
    expected_index=json.loads((ROOT/'deploy/data/hotfix.json').read_text(encoding='utf-8'))['runtime']['index']
    assert runtime_index==expected_index and runtime_index>=2026100805
    result.update({'剩余玩法环境':policy_env,'正式规则SHA256':policy_sha,'热更版本':runtime_index,'特别演练恢复数量':12})
    journal=run(client,'journalctl -u hs-game _PID='+pid+' -n 100 --no-pager -o cat')
    client_errors=[line for line in journal.splitlines() if '客户端事件 ' in line and 'hs_client_error' in line]
    errors=[line for line in journal.splitlines() if '客户端事件 ' not in line and re.search(r'panic|fatal|Traceback|failed|error',line,re.I)]
    assert not errors, '本批启动日志异常'
    result['玩家客户端异常样本']=client_errors
    result.update({'游戏SHA256':game_sha,'游戏PID':pid,'运行进程SHA256一致':True,'当前运行时':runtime['当前链接'],'PVP环境变量名值':env,'启动错误':errors})
finally:client.close()
client=connect_hotfix()
try:
    result['热更审计']=audit_hotfix(client)
    assert run(client,'systemctl is-active hs-hotfix')=='active'
    assert run(client,'sha256sum /opt/hs-hotfix/data/hotfix.json').split()[0]==hotfix_sha
    pid=run(client,'systemctl show hs-hotfix -p MainPID --value')
    assert pid.isdigit() and int(pid)>0
    result.update({'热更PID':pid,'热更SHA256':hotfix_sha,'状态':'通过','边界':'只读哈希、进程、运行时、环境及启动日志；未进行Android验收'})
finally:client.close()
(ROOT/'out/native-solo-release-verification.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({k:v for k,v in result.items() if '审计' not in k},ensure_ascii=False,indent=2))
