"""统一发布游戏、日程及两台主机热更；任一提交失败恢复整批项目文件。"""
import argparse,hashlib,json,re,shlex,subprocess,sys
from datetime import datetime
from server_ops import ROOT,connect,run,audit,upload_resumable,reap_holders
from deploy_hotfix_bridge import connect as connect_hotfix,audit as audit_hotfix
from build_completion import source_hashes

def digest(path): return hashlib.sha256(path.read_bytes()).hexdigest()

def validate_database(build):
    expected=build.get('产物哈希',{}).get('out/bin/linux-amd64/dbstore.test')
    if not expected or digest(ROOT/'out/bin/linux-amd64/dbstore.test')!=expected:
        raise ValueError('正式数据库验收产物与构建记录不一致')
    report=json.loads((ROOT/'out/remote-database-verification.json').read_text(encoding='utf-8'))
    catalog_hash=build.get('源码哈希',{}).get('deploy/data/hotfix.json')
    if not (report.get('状态')=='通过' and report.get('退出码')==0 and
            report.get('PASS数量',0)>0 and report.get('SKIP数量')==0 and
            report.get('SHA256')==expected and report.get('远端SHA256')==expected and
            catalog_hash and report.get('热更配置SHA256')==catalog_hash and report.get('远端热更配置SHA256')==catalog_hash):
        raise ValueError('当前正式构建尚无完整真实数据库通过证据')
    policy_hash=build.get('源码哈希',{}).get('deploy/data/gameplay-rules.env')
    if policy_hash and (report.get('正式规则SHA256')!=policy_hash or report.get('远端正式规则SHA256')!=policy_hash):
        raise ValueError('真实数据库验收没有绑定本批正式运营规则')

def read_bound(path, hashes):
    """读取构建时绑定的原始字节，避免校验与解析读到不同版本。"""
    raw=path.read_bytes(); key=path.relative_to(ROOT).as_posix()
    if hashes.get(key)!=hashlib.sha256(raw).hexdigest():
        raise ValueError('构建绑定输入已改变：'+key)
    return raw.decode('utf-8-sig').replace('\r\n','\n')

def validate_rune_pools(pools, catalog, tables):
    """必需编号直接取原生构建表；运营提案不参与正式发布条件。"""
    drops=catalog['rune_drops']; suits={str(row['_id']) for row in tables['tables']['runes_suits']}
    required=[]
    for key,row in drops.items():
        weights=row['suit_id_weight']
        if not weights or any(not isinstance(pair,list) or len(pair)!=2 or type(pair[1]) is not int or pair[1]<0 for pair in weights):
            raise ValueError('构建原生契印权重格式无效：'+key)
        if not any(pair[1]>0 for pair in weights): required.append(key)
    for key,pool in pools.items():
        if key not in drops or not isinstance(pool,list) or not pool:
            raise ValueError('运营契印池编号或列表无效：'+str(key))
        seen=set()
        for pair in pool:
            if (not isinstance(pair,list) or len(pair)!=2 or any(type(value) is not int or value<=0 or value>9223372036854775807 for value in pair)
                    or str(pair[0]) not in suits or pair[0] in seen):
                raise ValueError('运营契印套装、权重或重复项无效：'+str(key))
            seen.add(pair[0])
        if sum(pair[1] for pair in pool)>9223372036854775807:
            raise ValueError('运营契印权重合计超过原生整数边界：'+str(key))
    missing=[key for key in required if key not in pools]
    if missing: raise ValueError('原生全零契印池尚无获批准的正式规则，阻断发布：'+','.join(sorted(missing,key=int)))

def validate_local():
    build=json.loads((ROOT/'out/completion-build.json').read_text(encoding='utf-8'))
    if build.get('退出码')!=0 or build.get('源码哈希')!=source_hashes():
        raise ValueError('源码在正式构建后有变化，必须重新构建')
    game=ROOT/'out/bin/linux-amd64/gameserver'
    if build.get('产物哈希',{}).get('out/bin/linux-amd64/gameserver')!=digest(game):
        raise ValueError('正式Linux产物与构建记录不一致')
    validate_database(build)
    # 原校验脚本使用assert；忽略PYTHONOPTIMIZE，确保发布门槛实际执行。
    subprocess.run([sys.executable,'-E',str(ROOT/'out/tools/verify_battle_bridge_hotfix.py')],cwd=ROOT,check=True)
    hotfix=ROOT/'deploy/data/hotfix.json'; schedules=ROOT/'deploy/data/activity-schedules.json'
    for path in (hotfix,schedules): json.loads(read_bound(path,build['源码哈希']))
    files={'/opt/hs-server/bin/gameserver':game,'/opt/hs-server/data/activity-schedules.json':schedules,'/opt/hs-server/data/hotfix.json':hotfix}
    policy=ROOT/'deploy/data/gameplay-rules.env'; rune_pools={}
    if policy.exists():
        approval=json.loads((ROOT/'out/gameplay-policy-approval.json').read_text(encoding='utf-8'))
        policy_text=read_bound(policy,build['源码哈希'])
        if approval.get('状态')!='用户明确批准' or approval.get('正式文件SHA256')!=build['源码哈希'].get('deploy/data/gameplay-rules.env'):
            raise ValueError('运营配置没有匹配的明确批准记录，提案不得自动加载')
        seen=set(); allowed={'HS_RUNE_FALLBACK_POOLS','HS_FURNITURE_FUSION_POOLS','HS_SERVER_OPEN_TIME','HS_REMAINING_GAMEPLAY_POLICY','HS_SPECIAL_DRILL_REOPEN','HS_NEW_AVATAR_ALL_HEROES'}
        for line in policy_text.splitlines():
            line=line.strip()
            if not line or line.startswith('#'): continue
            key,sep,value=line.partition('=')
            if not sep or key not in allowed or key in seen: raise ValueError('运营变量不属于本批或重复')
            seen.add(key); words=shlex.split(value)
            if len(words)!=1: raise ValueError('运营变量必须是一个完整环境值')
            if key=='HS_NEW_AVATAR_ALL_HEROES':
                grant=approval.get('批准范围',{}).get('新角色全英雄',{})
                if words[0] not in {'0','1'} or grant.get('用户原话')!='给现在新角色创建后，直接默认全英雄。做个开关。目前是测试阶段':
                    raise ValueError('新角色全英雄开关缺少明确批准或值无效')
                continue
            if key=='HS_SPECIAL_DRILL_REOPEN':
                drill=approval.get('批准范围',{}).get('特别演练恢复',{})
                if words[0]!='timber-12-20261008' or drill.get('版本')!=words[0] or drill.get('用户原话')!='恢复原有12个副本，继续全部完成' or drill.get('原有副本')!=[2103,2106,2112,2203,2206,2212,2303,2306,2312,2503,2506,2512]:
                    raise ValueError('特别演练恢复缺少逐副本明确批准记录')
                continue
            if key=='HS_REMAINING_GAMEPLAY_POLICY':
                remaining=approval.get('批准范围',{}).get('剩余玩法规则',{})
                if words[0]!='local-20261008-v1' or remaining.get('版本')!=words[0] or remaining.get('用户原话')!='统一采用推荐本服规则，继续全部完成':
                    raise ValueError('剩余玩法版本没有匹配的明确批准记录')
                proposal=ROOT/'out/remaining-policy-proposal.json'
                if remaining.get('提案SHA256')!=digest(proposal):
                    raise ValueError('剩余玩法提案在批准配置绑定后改变')
                continue
            parsed=int(words[0]) if key=='HS_SERVER_OPEN_TIME' else json.loads(words[0])
            if key=='HS_SERVER_OPEN_TIME' and not 0<parsed<=9223372036854775807: raise ValueError('正式开服时间必须为正Unix秒且符合原生整数边界')
            if key!='HS_SERVER_OPEN_TIME' and not isinstance(parsed,dict): raise ValueError('候选池必须为显式字典')
            if key=='HS_RUNE_FALLBACK_POOLS': rune_pools=parsed
        dropin=ROOT/'deploy/hs-game-gameplay.conf'
        if read_bound(dropin,build['源码哈希'])!='# HS_GAMEPLAY_POLICY\n[Service]\nEnvironmentFile=/opt/hs-server/data/gameplay-rules.env\n':
            raise ValueError('运营drop-in与项目定义不一致')
        files['/opt/hs-server/data/gameplay-rules.env']=policy
        files['/etc/systemd/system/hs-game.service.d/50-gameplay-rules.conf']=dropin
    validate_rune_pools(rune_pools,
        json.loads(read_bound(ROOT/'internal/game/shop_catalog.json',build['源码哈希'])),
        json.loads(read_bound(ROOT/'internal/game/rune_tables.json',build['源码哈希'])))
    native_dropin=ROOT/'deploy/hs-game-native-pvp.conf'
    native_text=read_bound(native_dropin,build['源码哈希'])
    if not native_text.startswith('# HS_NATIVE_PVP_AUTHORITY\n[Service]\n') or native_text.count('Environment=HS_NATIVE_PVP_')!=4:
        raise ValueError('PVP权威drop-in与项目定义不一致')
    files['/etc/systemd/system/hs-game.service.d/60-native-pvp.conf']=native_dropin
    hotfix_files={'/opt/hs-hotfix/data/hotfix.json':hotfix}
    expected={target:(build['产物哈希']['out/bin/linux-amd64/gameserver'] if target.endswith('/gameserver') else
                     build['源码哈希'][local.relative_to(ROOT).as_posix()]) for target,local in dict(files,**hotfix_files).items()}
    if build['源码哈希']!=source_hashes(): raise ValueError('正式预检期间构建输入改变')
    return files,hotfix_files,expected

class ReleaseFailure(RuntimeError):
    def __init__(self,cause,rollback_errors):
        super().__init__('组合发布失败；回滚错误数量=%d；原因为%s'%(len(rollback_errors),cause))
        self.rollback_errors=rollback_errors

def apply_batches(batches,precommit=None):
    """全部备份/上传/哈希通过后才提交；超时也按可能已修改处理。"""
    try:
        # 锁由常驻SSH channel持有；任一连接断开后远端flock自动释放。
        for batch in batches: batch.acquire_lock()
        for batch in batches: batch.prepare()
        if precommit: precommit()
        try:
            for batch in batches:
                batch.install(); batch.verify()
        except Exception as error:
            failures=[]
            for batch in reversed(batches):
                try: batch.rollback()
                except Exception as rollback_error: failures.append({'服务':batch.unit,'错误':str(rollback_error)})
            raise ReleaseFailure(error,failures) from error
    finally:
        for batch in reversed(batches): batch.release_lock()

class Batch:
    def __init__(self,unit,base,connector,auditor,files,stamp,expected_hashes):
        self.unit,self.base,self.connector,self.auditor,self.files,self.stamp=unit,base,connector,auditor,files,stamp
        self.backup=base+'/releases/completion-'+stamp; self.rows={}; self.before=self.after=None
        # 不在此重新读取local决定期望值；只能采用获构建/PG验收的清单。
        self.hashes={target:expected_hashes[target] for target in files}
        self.lock_client=self.lock_channel=None
    def acquire_lock(self):
        self.release_lock()
        client=self.connector()
        try:
            lock=self.base+'/.completion-release.lock'
            if run(client,'readlink -f '+shlex.quote(self.base))!=self.base or run(client,'readlink -m '+shlex.quote(lock))!=lock:
                raise ValueError('项目锁目录归属改变')
            command='flock -n -E 73 '+shlex.quote(lock)+' sh -c '+shlex.quote('printf "HS_COMPLETION_LOCKED\\n"; cat >/dev/null')
            stdin,stdout,stderr=client.exec_command(command,timeout=10)
            marker=stdout.readline().strip()
            if marker!='HS_COMPLETION_LOCKED': raise RuntimeError('项目已有发布锁或锁获取失败')
            self.lock_client,self.lock_channel=client,stdout.channel
            self.lock_streams=(stdin,stdout,stderr)
            transport=client.get_transport()
            if transport: transport.set_keepalive(15)
            self.assert_lock()
        except Exception:
            client.close(); self.lock_client=self.lock_channel=None; raise
    def assert_lock(self):
        if (self.lock_client is None or self.lock_channel is None or self.lock_channel.closed or
                self.lock_channel.exit_status_ready() or not self.lock_client.get_transport().is_active()):
            raise RuntimeError('项目发布锁连接已断开，禁止继续提交')
    def release_lock(self):
        if self.lock_client is not None:
            self.lock_client.close()
        self.lock_client=self.lock_channel=None
    def call(self,callback):
        if self.lock_client is not None:
            self.assert_lock(); return callback(self.lock_client)
        client=self.connector()
        try: return callback(client)
        finally: client.close()
    def check_paths(self,client,paths):
        for target in paths:
            dropins=('/etc/systemd/system/hs-game.service.d/50-gameplay-rules.conf','/etc/systemd/system/hs-game.service.d/60-native-pvp.conf')
            allowed_dropins=tuple(value+suffix for value in dropins for suffix in ('','.stage-'+self.stamp,'.rollback-'+self.stamp))
            if not (target.startswith(self.base+'/') or (self.unit=='hs-game' and target in allowed_dropins)):
                raise ValueError('目标不属本项目：'+target)
            if run(client,'readlink -m '+shlex.quote(target))!=target:
                raise ValueError('目标或父目录符号链接越出所属位置：'+target)
    def check_files(self,client,paths):
        self.check_paths(client,paths)
        for target in paths:
            run(client,'test ! -e '+shlex.quote(target)+' || test -f '+shlex.quote(target))
    def inspect(self,running=True,targets=True):
        def inspect(client):
            report=self.auditor(client) if running else {'项目目录':run(client,'readlink -f '+shlex.quote(self.base))}
            if report['项目目录']!=self.base: raise ValueError('项目目录归属改变')
            service=run(client,'systemctl show '+self.unit+' -p Id -p FragmentPath -p WorkingDirectory -p MainPID -p ActiveState -p ExecStart')
            properties=dict(line.split('=',1) for line in service.splitlines() if '=' in line)
            expected=self.base+'/bin/'+('gameserver' if self.unit=='hs-game' else 'hotfixserver')
            if (properties.get('Id')!=self.unit+'.service' or properties.get('FragmentPath')!='/etc/systemd/system/'+self.unit+'.service' or
                    properties.get('WorkingDirectory')!=self.base or re.findall(r'\bpath=([^ ;}]+)',properties.get('ExecStart',''))!=[expected]):
                raise ValueError('项目unit定义或执行路径归属改变')
            if run(client,'readlink -m '+shlex.quote(properties['FragmentPath']))!=properties['FragmentPath']:
                raise ValueError('项目unit文件符号链接归属改变')
            self.check_paths(client,[self.backup,self.base+'/.completion-release.lock'])
            if targets: self.check_paths(client,list(self.files)+[row['临时文件'] for row in self.rows.values()]+[row['备份'] for row in self.rows.values()])
            if not running: return report
            match=re.search(r'^MainPID=(\d+)$',service,re.M)
            if 'ActiveState=active' not in service or not match or int(match[1])<=0: raise ValueError('项目服务未正常运行')
            exe=run(client,'readlink -f /proc/'+match[1]+'/exe')
            if exe!=expected: raise ValueError('项目可执行文件归属改变')
            owned=[line for line in run(client,'ss -lntp').splitlines() if 'pid='+match[1]+',' in line]
            port=9000 if self.unit=='hs-game' else 8082
            if not any(re.search(r':'+str(port)+r'\s',line) for line in owned): raise ValueError('项目监听不属于已核对PID')
            for target in self.files:
                if target.startswith('/etc/'):
                    marker='# HS_NATIVE_PVP_AUTHORITY' if target.endswith('/60-native-pvp.conf') else '# HS_GAMEPLAY_POLICY'
                    run(client,'if test -e '+shlex.quote(target)+'; then grep -q "^'+marker+'$" '+shlex.quote(target)+'; fi')
            report.update({'执行文件':exe,'PID':int(match[1])}); return report
        return self.call(inspect)
    def prepare(self):
        self.assert_lock(); self.before=self.inspect()
        def backup(client):
            run(client,'mkdir -p '+shlex.quote(self.backup)+' && chmod 700 '+shlex.quote(self.backup))
            for target,local in self.files.items():
                self.assert_lock()
                if digest(local)!=self.hashes[target]: raise ValueError('待上传输入在预检后有变化')
                exists=run(client,'if test -f '+shlex.quote(target)+'; then echo yes; else echo no; fi')=='yes'
                saved=self.backup+'/'+target.rsplit('/',1)[1]
                row={'原来存在':exists,'备份':saved,'新SHA256':self.hashes[target],'临时文件':target+'.stage-'+self.stamp}
                self.check_files(client,[target,saved,row['临时文件']])
                if exists:
                    row['旧SHA256']=run(client,'sha256sum '+shlex.quote(target)).split()[0]
                    run(client,'cp -a '+shlex.quote(target)+' '+shlex.quote(saved))
                    if run(client,'sha256sum '+shlex.quote(saved)).split()[0]!=row['旧SHA256']: raise ValueError('备份哈希不一致')
                if target.startswith('/etc/'): run(client,'mkdir -p /etc/systemd/system/hs-game.service.d')
                self.rows[target]=row
        self.call(backup)
        for target,local in self.files.items():
            self.assert_lock(); upload_resumable(self.connector,local,self.rows[target]['临时文件'])
        def verify(client):
            for row in self.rows.values():
                if run(client,'sha256sum '+shlex.quote(row['临时文件'])).split()[0]!=row['新SHA256']: raise ValueError('临时文件哈希不一致')
        self.call(verify)
    def install(self):
        self.assert_lock(); self.inspect()
        def install(client):
            # 字典顺序无关：配置先替换，游戏二进制最后替换。
            for target in sorted(self.rows,key=lambda p:p.endswith('/gameserver')):
                self.assert_lock()
                row=self.rows[target]; mode='755' if target.endswith('/gameserver') else '600'
                self.check_files(client,[target,row['临时文件']])
                if run(client,'sha256sum '+shlex.quote(row['临时文件'])).split()[0]!=row['新SHA256']:
                    raise ValueError('提交前临时文件哈希改变')
                present=run(client,'if test -f '+shlex.quote(target)+'; then echo yes; else echo no; fi')=='yes'
                if present!=row['原来存在'] or (present and run(client,'sha256sum '+shlex.quote(target)).split()[0]!=row['旧SHA256']):
                    raise ValueError('现场目标在备份后改变，禁止覆盖')
                if target.endswith('/gameserver'): reap_holders(client,row['临时文件'])
                run(client,'chmod '+mode+' '+shlex.quote(row['临时文件'])+' && mv '+shlex.quote(row['临时文件'])+' '+shlex.quote(target))
            if any(p.startswith('/etc/') for p in self.rows): run(client,'systemctl daemon-reload')
            run(client,'systemctl restart '+self.unit+' && systemctl is-active --quiet '+self.unit)
            self.wait_listener(client)
        self.call(install)
    def verify(self):
        def verify(client):
            for target,row in self.rows.items():
                if run(client,'sha256sum '+shlex.quote(target)).split()[0]!=row['新SHA256']: raise ValueError('安装文件哈希不一致')
        self.call(verify); self.after=self.inspect()
    def rollback(self):
        try: self.assert_lock()
        except RuntimeError: self.acquire_lock()
        # 新服务可能启动失败；回滚只依赖仍属项目的unit定义和路径。
        self.inspect(running=False,targets=False)
        def rollback(client):
            failures=[]
            for target,row in self.rows.items():
                try:
                    self.assert_lock(); temp=target+'.rollback-'+self.stamp
                    self.check_files(client,[target,row['备份'],temp])
                    present=run(client,'if test -f '+shlex.quote(target)+'; then echo yes; else echo no; fi')=='yes'
                    if present and run(client,'sha256sum '+shlex.quote(target)).split()[0] not in {row['新SHA256'],row.get('旧SHA256')}:
                        raise ValueError('现场文件不属于本批旧/新版本，禁止回滚覆盖')
                    if row['原来存在']:
                        if run(client,'sha256sum '+shlex.quote(row['备份'])).split()[0]!=row['旧SHA256']: raise ValueError('回滚备份哈希改变')
                        run(client,'cp -a '+shlex.quote(row['备份'])+' '+shlex.quote(temp))
                        if target.endswith('/gameserver'): reap_holders(client,temp)
                        run(client,'mv '+shlex.quote(temp)+' '+shlex.quote(target))
                        if run(client,'sha256sum '+shlex.quote(target)).split()[0]!=row['旧SHA256']: raise ValueError('恢复哈希不一致')
                    else: run(client,'rm -f '+shlex.quote(target))
                except Exception as error: failures.append(target+'：'+str(error))
            try:
                self.assert_lock(); self.inspect(running=False,targets=False)
                if any(p.startswith('/etc/') for p in self.rows): run(client,'systemctl daemon-reload')
                run(client,'systemctl restart '+self.unit+' && systemctl is-active --quiet '+self.unit)
                self.wait_listener(client)
            except Exception as error: failures.append('服务恢复：'+str(error))
            if failures: raise RuntimeError('；'.join(failures))
        self.call(rollback); self.inspect()
    def wait_listener(self,client):
        # active可能早于数据库初始化及socket绑定，等待所属PID监听避免误回滚。
        port=9000 if self.unit=='hs-game' else 8082
        command=('for attempt in $(seq 1 50); do pid=$(systemctl show '+self.unit+' -p MainPID --value); '
                 'if ss -lntp | grep -E '+shlex.quote(':'+str(port)+r'\s')+' | grep -F "pid=$pid," >/dev/null; then exit 0; fi; '
                 'sleep 0.2; done; exit 1')
        run(client,command)

def write_report(path,report): path.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')

def main():
    sys.stdout.reconfigure(encoding='utf-8')
    parser=argparse.ArgumentParser(description=__doc__); parser.add_argument('--check',action='store_true',help='只读预检，不上传/替换或重启')
    args=parser.parse_args(); stamp=datetime.now().strftime('%Y%m%d-%H%M%S-%f'); batches=[]
    report={'时间':datetime.now().isoformat(),'状态':'检查中'}; path=ROOT/('out/completion-release-'+stamp+'.json')
    try:
        files,hotfix_files,expected=validate_local()
        batches=[Batch('hs-game','/opt/hs-server',connect,audit,files,stamp,expected),Batch('hs-hotfix','/opt/hs-hotfix',connect_hotfix,audit_hotfix,hotfix_files,stamp,expected)]
        for batch in batches: batch.before=batch.inspect()
        if args.check: report['状态']='只读预检通过'
        else:
            def precommit():
                new_files,new_hotfix,new_expected=validate_local()
                if expected!=new_expected or files!=new_files or hotfix_files!=new_hotfix:
                    raise ValueError('上传批次与当前获验收构建清单不一致')
                for batch in batches:
                    if any(row['新SHA256']!=new_expected[target] for target,row in batch.rows.items()):
                        raise ValueError('上传临时文件未绑定当前获验收构建')
                    batch.assert_lock(); batch.inspect()
            apply_batches(batches,precommit); report['状态']='游戏与独立热更组合发布通过'
    except Exception as error:
        report.update({'状态':'失败','错误':str(error),'回滚错误':getattr(error,'rollback_errors',[])}); raise
    finally:
        report['主机']=[{'服务':b.unit,'备份':b.backup,'文件清单':b.rows,'发布前':b.before,'发布后':b.after} for b in batches]
        write_report(path,report); print(json.dumps({'状态':report['状态'],'报告':str(path)},ensure_ascii=False))
    if not args.check:
        game,hotfix=batches
        write_report(ROOT/'out/server-release.json',{'状态':report['状态'],'SHA256':game.rows['/opt/hs-server/bin/gameserver']['新SHA256'],'备份':game.backup,'文件清单':game.rows,'发布前':game.before,'发布后':game.after,'组合报告':str(path)})
        write_report(ROOT/'out/hotfix-bridge-release.json',{'状态':report['状态'],'SHA256':hotfix.rows['/opt/hs-hotfix/data/hotfix.json']['新SHA256'],'备份':hotfix.rows['/opt/hs-hotfix/data/hotfix.json']['备份'],'发布前':hotfix.before,'发布后':hotfix.after,'组合报告':str(path)})

if __name__=='__main__': main()
