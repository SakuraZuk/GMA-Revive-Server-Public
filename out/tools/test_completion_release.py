"""发布边界故障注入：全预备成功才提交，第二主机失败必须恢复第一主机。"""
import hashlib,io,json,shlex,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
from deploy_completion_release import apply_batches, ReleaseFailure,validate_database,validate_local,validate_rune_pools,Batch,ROOT

class FakeBatch:
    def __init__(self,unit,events,fail=None): self.unit,self.events,self.fail=unit,events,fail
    def event(self,stage):
        self.events.append((self.unit,stage))
        if stage==self.fail: raise RuntimeError(self.unit+' '+stage+' 注入故障')
    def prepare(self): self.event('prepare')
    def install(self): self.event('install')
    def verify(self): self.event('verify')
    def rollback(self): self.event('rollback')
    def acquire_lock(self): pass
    def release_lock(self): pass

class RemoteModel:
    """只模拟项目文件及SSH状态，调用正式Batch方法；不连接真实主机。"""
    def __init__(self):
        self.files={}; self.links={}; self.commands=[]; self.active=True; self.unit_owned=True
        self.fail_restore=set(); self.held=False; self.restart_failures=0
    def connect(self): return ClientModel(self)
    def audit(self,client):
        if not self.active: raise ValueError('模拟新服务启动失败')
        return {'项目目录':'/opt/hs-server'}
    def run(self,client,command):
        self.commands.append(command); words=shlex.split(command)
        if command.startswith('readlink '):
            target=words[-1]
            if target.startswith('/proc/'): return '/opt/hs-server/bin/gameserver'
            return self.links.get(target,target)
        if command.startswith('systemctl show '):
            path='/opt/hs-server/bin/gameserver' if self.unit_owned else '/opt/foreign/bin/server'
            return ('Id=hs-game.service\nFragmentPath=/etc/systemd/system/hs-game.service\nWorkingDirectory=/opt/hs-server\n'
                    'MainPID=91\nActiveState='+('active' if self.active else 'failed')+'\nExecStart={ path='+path+' ; argv[]='+path+' ; }')
        if command=='ss -lntp': return 'LISTEN 0 128 0.0.0.0:9000 0.0.0.0:* users:(("gameserver",pid=91,fd=3))'
        if command.startswith('if test -f '): return 'yes' if words[3].rstrip(';') in self.files else 'no'
        if command.startswith('test ! -e ') or command.startswith('mkdir ') or command.startswith('if test -e '): return ''
        if command.startswith('sha256sum '): return hashlib.sha256(self.files[words[1]]).hexdigest()+'  '+words[1]
        if command.startswith('cp -a '):
            source,target=words[2:4]
            if source in self.fail_restore: raise RuntimeError('模拟备份恢复I/O故障')
            self.files[target]=self.files[source]
            if '&&' in words:
                destination=words[-1]; self.files[destination]=self.files.pop(target)
            return ''
        if command.startswith('chmod '):
            stage,target=words[-2:]; self.files[target]=self.files.pop(stage); return ''
        if command.startswith('mv '): self.files[words[2]]=self.files.pop(words[1]); return ''
        if command.startswith('rm -f '): self.files.pop(words[2],None); return ''
        if command.startswith('systemctl restart '):
            if self.restart_failures:
                self.restart_failures-=1; self.active=False; raise RuntimeError('模拟部分提交后203/EXEC启动失败')
            self.active=True; return ''
        if command.startswith('systemctl daemon-reload') or command.startswith('for attempt '): return ''
        raise AssertionError('未定义的模型命令：'+command)
    def reap(self,client,target):
        self.commands.append('REAP '+target); return ['模拟遗留SFTP句柄PID']

class ChannelModel:
    def __init__(self,closed=False): self.closed=closed
    def exit_status_ready(self): return self.closed

class StreamModel(io.StringIO):
    def __init__(self,text,channel): super().__init__(text); self.channel=channel

class ClientModel:
    def __init__(self,remote): self.remote=remote; self.closed=False; self.channel=None; self.owns=False
    def exec_command(self,command,timeout=10):
        if not command.startswith('flock -n -E 73 '): raise AssertionError(command)
        self.owns=not self.remote.held
        if self.owns: self.remote.held=True
        self.channel=ChannelModel(not self.owns)
        return (StreamModel('',self.channel),StreamModel('HS_COMPLETION_LOCKED\n' if self.owns else '',self.channel),StreamModel('',self.channel))
    def get_transport(self): return self
    def is_active(self): return not self.closed
    def set_keepalive(self,interval): pass
    def close(self):
        self.closed=True
        if self.channel: self.channel.closed=True
        if self.owns: self.remote.held=False; self.owns=False

class CompletionReleaseTests(unittest.TestCase):
    def test_all_prepared_before_any_install(self):
        events=[]; apply_batches([FakeBatch('game',events),FakeBatch('hotfix',events)])
        self.assertEqual(events,[('game','prepare'),('hotfix','prepare'),('game','install'),('game','verify'),('hotfix','install'),('hotfix','verify')])
    def test_stage_failure_never_changes_live_files(self):
        events=[]
        with self.assertRaises(RuntimeError): apply_batches([FakeBatch('game',events),FakeBatch('hotfix',events,'prepare')])
        self.assertEqual(events,[('game','prepare'),('hotfix','prepare')])
    def test_changed_inputs_after_upload_never_install(self):
        events=[]
        def reject(): raise ValueError('构建后输入漂移')
        with self.assertRaises(ValueError): apply_batches([FakeBatch('game',events),FakeBatch('hotfix',events)],reject)
        self.assertEqual(events,[('game','prepare'),('hotfix','prepare')])
    def test_real_database_gate_rejects_old_hash_skips_and_failure(self):
        with tempfile.TemporaryDirectory() as temporary:
            root=Path(temporary); binary=root/'out/bin/linux-amd64/dbstore.test'
            binary.parent.mkdir(parents=True); binary.write_bytes(b'db-test')
            sha=hashlib.sha256(binary.read_bytes()).hexdigest()
            build={'产物哈希':{'out/bin/linux-amd64/dbstore.test':sha},'源码哈希':{'deploy/data/hotfix.json':'测试目录哈希'}}
            valid={'状态':'通过','退出码':0,'PASS数量':1,'SKIP数量':0,'SHA256':sha,'远端SHA256':sha,'热更配置SHA256':'测试目录哈希','远端热更配置SHA256':'测试目录哈希'}
            report=root/'out/remote-database-verification.json'
            with patch('deploy_completion_release.ROOT',root):
                report.write_text(json.dumps(valid),encoding='utf-8'); validate_database(build)
                for change in ({'远端SHA256':'旧构建'},{'SKIP数量':1},{'退出码':1},{'PASS数量':0},{'状态':'执行中'},{'远端热更配置SHA256':'旧目录'}):
                    with self.subTest(change=change):
                        report.write_text(json.dumps(dict(valid,**change)),encoding='utf-8')
                        with self.assertRaises(ValueError): validate_database(build)
                binary.write_bytes(b'new-unverified')
                with self.assertRaises(ValueError): validate_database(build)
    def test_commit_or_verification_failures_restore_both_hosts(self):
        for host in ('game','hotfix'):
            for stage in ('install','verify'):
                with self.subTest(host=host,stage=stage):
                    events=[]
                    batches=[FakeBatch('game',events,stage if host=='game' else None),FakeBatch('hotfix',events,stage if host=='hotfix' else None)]
                    with self.assertRaises(ReleaseFailure) as caught: apply_batches(batches)
                    self.assertEqual(events[-2:],[('hotfix','rollback'),('game','rollback')])
                    self.assertEqual(caught.exception.rollback_errors,[])
    def test_database_gate_binds_formal_policy_and_rejects_missing_or_changed_hash(self):
        with tempfile.TemporaryDirectory() as temporary:
            root=Path(temporary); binary=root/'out/bin/linux-amd64/dbstore.test'
            binary.parent.mkdir(parents=True);binary.write_bytes(b'db-test')
            sha=hashlib.sha256(binary.read_bytes()).hexdigest()
            build={'产物哈希':{'out/bin/linux-amd64/dbstore.test':sha},'源码哈希':{'deploy/data/hotfix.json':'目录','deploy/data/gameplay-rules.env':'正式规则'}}
            valid={'状态':'通过','退出码':0,'PASS数量':41,'SKIP数量':0,'SHA256':sha,'远端SHA256':sha,'热更配置SHA256':'目录','远端热更配置SHA256':'目录','正式规则SHA256':'正式规则','远端正式规则SHA256':'正式规则'}
            report=root/'out/remote-database-verification.json'
            with patch('deploy_completion_release.ROOT',root):
                report.write_text(json.dumps(valid),encoding='utf-8');validate_database(build)
                for key in ('正式规则SHA256','远端正式规则SHA256'):
                    for value in (None,'旧规则'):
                        with self.subTest(key=key,value=value):
                            report.write_text(json.dumps(dict(valid,**{key:value})),encoding='utf-8')
                            with self.assertRaisesRegex(ValueError,'正式运营规则'):validate_database(build)
    def test_rollback_failure_preserves_original_and_continues_other_host(self):
        events=[]; game=FakeBatch('game',events,'verify'); hotfix=FakeBatch('hotfix',events,'rollback')
        with self.assertRaises(ReleaseFailure) as caught: apply_batches([game,hotfix])
        self.assertEqual(events[-2:],[('hotfix','rollback'),('game','rollback')])
        self.assertEqual(caught.exception.rollback_errors[0]['服务'],'hotfix')
        self.assertIn('game verify',str(caught.exception))

    def native_policy(self):
        catalog=json.loads((ROOT/'internal/game/shop_catalog.json').read_text(encoding='utf-8'))
        tables=json.loads((ROOT/'internal/game/rune_tables.json').read_text(encoding='utf-8'))
        required=[key for key,row in catalog['rune_drops'].items() if not any(pair[1]>0 for pair in row['suit_id_weight'])]
        suit=next(row['_id'] for row in tables['tables']['runes_suits'] if row.get('drop_flag')==1)
        return catalog,tables,{key:[[suit,100]] for key in required}
    def test_native_required_pools_cannot_be_removed_by_empty_proposal(self):
        catalog,tables,pools=self.native_policy()
        self.assertEqual(len(pools),17)
        validate_rune_pools(pools,catalog,tables)
        with self.assertRaisesRegex(ValueError,'阻断发布'): validate_rune_pools({},catalog,tables)
        del pools['400']
        with self.assertRaisesRegex(ValueError,'400'): validate_rune_pools(pools,catalog,tables)
    def test_rune_policy_rejects_truthy_invalid_shapes_unknown_ids_and_weights(self):
        catalog,tables,pools=self.native_policy(); valid=pools['400'][0]
        bad=(1,'有值',[[valid[0],0]],[[valid[0],-1]],[[valid[0],True]],[[999999999,100]],
             [[valid[0],100],[valid[0],200]],[[valid[0],9223372036854775808]],[[valid[0],100,1]])
        for pool in bad:
            with self.subTest(pool=pool):
                with self.assertRaises(ValueError): validate_rune_pools(dict(pools,**{'400':pool}),catalog,tables)
    def test_local_validation_binds_expected_hashes_and_requires_approval(self):
        catalog,tables,pools=self.native_policy()
        with tempfile.TemporaryDirectory() as temporary:
            root=Path(temporary)
            inputs={'internal/game/shop_catalog.json':json.dumps(catalog),'internal/game/rune_tables.json':json.dumps(tables),
                'deploy/data/hotfix.json':'{}','deploy/data/activity-schedules.json':'[]',
                'deploy/data/gameplay-rules.env':"HS_RUNE_FALLBACK_POOLS='"+json.dumps(pools)+"'\n",
                'deploy/hs-game-gameplay.conf':'# HS_GAMEPLAY_POLICY\n[Service]\nEnvironmentFile=/opt/hs-server/data/gameplay-rules.env\n',
                'deploy/hs-game-native-pvp.conf':'# HS_NATIVE_PVP_AUTHORITY\n[Service]\nEnvironment=HS_NATIVE_PVP_PYTHON=/opt/hs-server/native-pvp/current/python2/run-python2\nEnvironment=HS_NATIVE_PVP_WORKER=/opt/hs-server/native-pvp/current/workspace/internal/nativepvp/battle_native_worker.py\nEnvironment=HS_NATIVE_PVP_DIRECTORY=/opt/hs-server/native-pvp/current/workspace/internal/nativepvp/runtime/native_engine\nEnvironment=HS_NATIVE_PVP_RESOURCE_SHA256=bed95c22e6e6adf4ccf01cabb5156b4888ccc5744f0e192d24501e6df3c19ef3\n',
                'out/bin/linux-amd64/gameserver':'正式游戏产物'}
            for key,value in inputs.items():
                target=root/key; target.parent.mkdir(parents=True,exist_ok=True); target.write_text(value,encoding='utf-8')
            hashes={key:hashlib.sha256((root/key).read_bytes()).hexdigest() for key in inputs if not key.startswith('out/')}
            game_hash=hashlib.sha256((root/'out/bin/linux-amd64/gameserver').read_bytes()).hexdigest()
            (root/'out/completion-build.json').write_text(json.dumps({'退出码':0,'源码哈希':hashes,'产物哈希':{'out/bin/linux-amd64/gameserver':game_hash}}),encoding='utf-8')
            approval=root/'out/gameplay-policy-approval.json'
            approval.write_text(json.dumps({'状态':'等待批准','正式文件SHA256':hashes['deploy/data/gameplay-rules.env']}),encoding='utf-8')
            with patch('deploy_completion_release.ROOT',root),patch('deploy_completion_release.source_hashes',return_value=hashes),patch('deploy_completion_release.validate_database'),patch('deploy_completion_release.subprocess.run') as verifier:
                with self.assertRaisesRegex(ValueError,'明确批准'): validate_local()
                approval.write_text(json.dumps({'状态':'用户明确批准','正式文件SHA256':hashes['deploy/data/gameplay-rules.env']}),encoding='utf-8')
                files,hotfix,expected=validate_local()
                self.assertEqual(expected['/opt/hs-server/bin/gameserver'],game_hash)
                self.assertEqual(expected['/opt/hs-hotfix/data/hotfix.json'],hashes['deploy/data/hotfix.json'])
                self.assertEqual(len(files),6); self.assertEqual(len(hotfix),1)
                self.assertTrue(verifier.call_args.kwargs['check'])
                self.assertIn('-E',verifier.call_args.args[0])
                approval.write_text(json.dumps({'状态':'用户明确批准','正式文件SHA256':'旧配置'}),encoding='utf-8')
                with self.assertRaisesRegex(ValueError,'明确批准'): validate_local()
    def batch_model(self,remote=None):
        remote=remote or RemoteModel(); targets=['/opt/hs-server/bin/gameserver','/opt/hs-server/data/hotfix.json']
        batch=Batch('hs-game','/opt/hs-server',remote.connect,remote.audit,{target:Path('模型输入') for target in targets},'模型批次',
                    {target:hashlib.sha256(b'new').hexdigest() for target in targets})
        for target in targets:
            saved=batch.backup+'/'+target.rsplit('/',1)[1]; stage=target+'.stage-'+batch.stamp
            batch.rows[target]={'原来存在':True,'备份':saved,'旧SHA256':hashlib.sha256(b'old').hexdigest(),'新SHA256':hashlib.sha256(b'new').hexdigest(),'临时文件':stage}
            remote.files[target]=b'new'; remote.files[saved]=b'old'; remote.files[stage]=b'new'
        return batch,remote,targets
    def test_expected_hash_cannot_follow_changed_then_restored_local_input(self):
        with tempfile.TemporaryDirectory() as temporary:
            local=Path(temporary)/'gameserver'; local.write_bytes(b'changed')
            target='/opt/hs-server/bin/gameserver'; expected=hashlib.sha256(b'accepted').hexdigest()
            remote=RemoteModel(); batch=Batch('hs-game','/opt/hs-server',remote.connect,remote.audit,{target:local},'模型批次',{target:expected})
            self.assertEqual(batch.hashes[target],expected)
            with patch('deploy_completion_release.run',side_effect=remote.run),patch('deploy_completion_release.reap_holders',side_effect=remote.reap):
                batch.acquire_lock()
                with self.assertRaisesRegex(ValueError,'预检后有变化'): batch.prepare()
                local.write_bytes(b'accepted')
                self.assertEqual(batch.hashes[target],expected)
                self.assertFalse(any(command.startswith('cp -a ') for command in remote.commands))
                batch.release_lock()
    def test_lock_contention_and_disconnect_prevent_commit(self):
        first,remote,targets=self.batch_model(); second,_,_=self.batch_model(remote)
        with patch('deploy_completion_release.run',side_effect=remote.run),patch('deploy_completion_release.reap_holders',side_effect=remote.reap):
            first.acquire_lock()
            with self.assertRaisesRegex(RuntimeError,'发布锁'): second.acquire_lock()
            first.lock_client.close()
            with self.assertRaisesRegex(RuntimeError,'断开'): first.install()
            second.acquire_lock(); second.assert_lock(); second.release_lock()
            self.assertFalse(remote.held)
    def test_install_rechecks_unit_and_stage_hash_before_live_mutation(self):
        for fault in ('unit','stage','backup-link'):
            with self.subTest(fault=fault):
                batch,remote,targets=self.batch_model()
                for target in targets: remote.files[target]=b'old'
                with patch('deploy_completion_release.run',side_effect=remote.run),patch('deploy_completion_release.reap_holders',side_effect=remote.reap):
                    batch.acquire_lock()
                    if fault=='unit': remote.unit_owned=False
                    elif fault=='stage': remote.files[batch.rows[targets[1]]['临时文件']]=b'tampered'
                    else: remote.links[batch.backup]='/opt/foreign/releases'
                    with self.assertRaises(ValueError): batch.install()
                    self.assertEqual([remote.files[target] for target in targets],[b'old',b'old'])
                    batch.release_lock()
    def test_failed_new_service_does_not_block_real_batch_rollback(self):
        batch,remote,targets=self.batch_model(); remote.active=False
        with patch('deploy_completion_release.run',side_effect=remote.run),patch('deploy_completion_release.reap_holders',side_effect=remote.reap):
            batch.acquire_lock(); batch.rollback(); batch.release_lock()
        self.assertEqual([remote.files[target] for target in targets],[b'old',b'old']); self.assertTrue(remote.active)
    def test_one_restore_failure_still_restores_other_file(self):
        batch,remote,targets=self.batch_model(); remote.fail_restore.add(batch.rows[targets[0]]['备份'])
        with patch('deploy_completion_release.run',side_effect=remote.run),patch('deploy_completion_release.reap_holders',side_effect=remote.reap):
            batch.acquire_lock()
            with self.assertRaisesRegex(RuntimeError,'I/O'): batch.rollback()
            batch.release_lock()
        self.assertEqual(remote.files[targets[0]],b'new'); self.assertEqual(remote.files[targets[1]],b'old')
    def test_rollback_refuses_foreign_file_and_symlink_but_restores_owned_file(self):
        for fault in ('file','symlink','unit'):
            with self.subTest(fault=fault):
                batch,remote,targets=self.batch_model()
                if fault=='file': remote.files[targets[0]]=b'other-release'
                elif fault=='symlink': remote.links[targets[0]]='/opt/foreign/server'
                else: remote.unit_owned=False
                with patch('deploy_completion_release.run',side_effect=remote.run),patch('deploy_completion_release.reap_holders',side_effect=remote.reap):
                    batch.acquire_lock()
                    with self.assertRaises((RuntimeError,ValueError)): batch.rollback()
                    batch.release_lock()
                self.assertNotEqual(remote.files[targets[0]],b'old')
                if fault!='unit': self.assertEqual(remote.files[targets[1]],b'old')
    def test_locks_release_on_prepare_and_commit_failures(self):
        for failure in ('prepare','install'):
            events=[]; locks=[]
            class LockedFake(FakeBatch):
                def acquire_lock(self): locks.append(self.unit)
                def release_lock(self):
                    if self.unit in locks: locks.remove(self.unit)
            batches=[LockedFake('game',events),LockedFake('hotfix',events,failure)]
            with self.assertRaises(RuntimeError): apply_batches(batches)
            self.assertEqual(locks,[])
    def test_partial_install_restart_failure_restores_real_batch_and_reaps_only_binary(self):
        batch,remote,targets=self.batch_model(); remote.restart_failures=1
        for target in targets: remote.files[target]=b'old'
        with patch('deploy_completion_release.run',side_effect=remote.run),patch('deploy_completion_release.reap_holders',side_effect=remote.reap),patch.object(batch,'prepare',return_value=None):
            with self.assertRaisesRegex(ReleaseFailure,'203/EXEC') as caught: apply_batches([batch])
        self.assertEqual(caught.exception.rollback_errors,[])
        self.assertEqual([remote.files[target] for target in targets],[b'old',b'old'])
        self.assertFalse(remote.held)
        stage=batch.rows[targets[0]]['临时文件']; temp=targets[0]+'.rollback-'+batch.stamp
        reap=[command for command in remote.commands if command.startswith('REAP ')]
        self.assertEqual(reap,['REAP '+stage,'REAP '+temp])
        commit=next(i for i,command in enumerate(remote.commands) if command.startswith('chmod ') and targets[0] in command)
        restore=next(i for i,command in enumerate(remote.commands) if shlex.split(command)==['mv',temp,targets[0]])
        self.assertLess(remote.commands.index('REAP '+stage),commit)
        self.assertLess(remote.commands.index('REAP '+temp),restore)

if __name__=='__main__': unittest.main()
