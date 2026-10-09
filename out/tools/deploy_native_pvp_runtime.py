"""部署项目私有Python2原生PVP运行时；不安装系统包，不修改全局解释器。"""
import hashlib,json,shlex,sys
from datetime import datetime
from pathlib import Path
from build_completion import source_hashes
from server_ops import ROOT,connect,run,upload_resumable

def sha(path): return hashlib.sha256(path.read_bytes()).hexdigest()

def main():
    sys.stdout.reconfigure(encoding='utf-8')
    build=json.loads((ROOT/'out/completion-build.json').read_text(encoding='utf-8'))
    if build.get('退出码')!=0 or build.get('源码哈希')!=source_hashes():
        raise SystemExit('原生运行时部署拒绝未绑定的源码或构建')
    pg=json.loads((ROOT/'out/remote-database-verification.json').read_text(encoding='utf-8'))
    linux=json.loads((ROOT/'out/native-authority-linux-go-verification.json').read_text(encoding='utf-8'))
    if pg.get('状态')!='通过' or pg.get('SKIP数量')!=0 or pg.get('PASS数量',0)<42 or linux.get('状态')!='通过':
        raise SystemExit('原生运行时缺少真实Linux房间或真实PostgreSQL通过证据')
    assets=[ROOT/'out/native-pvp-engine-linux-resources.tar.gz',
            ROOT/'out/native-pvp-linux-packages/libpython2.7-minimal_2.7.18-8+deb11u1_amd64.deb',
            ROOT/'out/native-pvp-linux-packages/libpython2.7-stdlib_2.7.18-8+deb11u1_amd64.deb',
            ROOT/'out/native-pvp-linux-packages/python2.7-minimal_2.7.18-8+deb11u1_amd64.deb',
            ROOT/'deploy/native-pvp-run-python2.sh']
    hashes={p:sha(p) for p in assets}
    for p,value in hashes.items():
        if build['源码哈希'].get(p.relative_to(ROOT).as_posix())!=value:
            raise SystemExit('原生运行时输入未绑定正式构建：'+str(p))
    stamp=datetime.now().strftime('%Y%m%d-%H%M%S-%f')
    base='/opt/hs-server/native-pvp'; stage=base+'/staging/'+stamp; release=base+'/releases/'+stamp
    client=connect()
    try:
        if run(client,'readlink -m '+shlex.quote(base))!=base: raise ValueError('原生运行时目录越出项目')
        run(client,'test ! -e '+shlex.quote(stage)+' && test ! -e '+shlex.quote(release)+' && mkdir -p '+shlex.quote(stage)+' '+shlex.quote(base+'/releases')+' && chmod 700 '+shlex.quote(stage))
    finally: client.close()
    remote={p:stage+'/'+p.name for p in assets}
    for local,target in remote.items(): upload_resumable(connect,local,target)
    client=connect()
    old=''
    try:
        for local,target in remote.items():
            if run(client,'sha256sum '+shlex.quote(target)).split()[0]!=hashes[local]: raise ValueError('运行时上传哈希不一致')
        current=base+'/current'
        old=run(client,'if test -L '+shlex.quote(current)+'; then readlink '+shlex.quote(current)+'; elif test -e '+shlex.quote(current)+'; then echo INVALID; fi')
        if old=='INVALID' or old and not old.startswith('releases/'): raise ValueError('原生运行时current不属于项目版本目录')
        resource=remote[assets[0]]; wrapper=remote[assets[-1]]
        debs=' '.join(shlex.quote(remote[p]) for p in assets[1:4])
        command=('mkdir '+shlex.quote(release)+' && tar -xzf '+shlex.quote(resource)+' -C '+shlex.quote(release)+
                 ' && mkdir -p '+shlex.quote(release+'/python2')+' && for package in '+debs+'; do dpkg-deb -x "$package" '+shlex.quote(release+'/python2')+'; done && '
                 'cp '+shlex.quote(wrapper)+' '+shlex.quote(release+'/python2/run-python2')+' && chmod 700 '+shlex.quote(release+'/python2/run-python2')+
                 ' && test -f '+shlex.quote(release+'/workspace/internal/nativepvp/battle_native_worker.py')+
                 ' && test -d '+shlex.quote(release+'/workspace/internal/nativepvp/runtime/native_engine/script'))
        run(client,command,timeout=90)
        verify=('cd '+shlex.quote(release+'/workspace')+' && HS_NATIVE_PVP_PYTHON2='+shlex.quote(release+'/python2/run-python2')+
                ' /usr/bin/python3 '+shlex.quote(release+'/workspace/out/tools/test_native_pvp_engine.py'))
        proof=run(client,verify,timeout=120)
        remaining_verify=('cd '+shlex.quote(release+'/workspace')+' && HS_NATIVE_PVP_PYTHON2='+shlex.quote(release+'/python2/run-python2')+
                ' /usr/bin/python3 '+shlex.quote(release+'/workspace/out/tools/test_remaining_activity_native.py'))
        remaining_proof=run(client,remaining_verify,timeout=60)
        metrics_proof=run(client,'cd '+shlex.quote(release+'/workspace')+' && /usr/bin/python3 out/tools/test_activity_metrics_script.py',timeout=30)
        run(client,'ln -s '+shlex.quote('releases/'+stamp)+' '+shlex.quote(base+'/current.new-'+stamp)+' && mv -Tf '+shlex.quote(base+'/current.new-'+stamp)+' '+shlex.quote(current))
        if run(client,'readlink '+shlex.quote(current))!='releases/'+stamp: raise ValueError('原生运行时原子切换失败')
        report={'时间':datetime.now().isoformat(),'状态':'生产私有运行时已准备并激活','版本':stamp,'目录':release,'当前链接':'releases/'+stamp,
                '原当前链接':old or None,'输入SHA256':{p.relative_to(ROOT).as_posix():v for p,v in hashes.items()},'远端实测末行':proof.splitlines()[-1] if proof else '',
                '剩余活动原生实测末行':remaining_proof.splitlines()[-1] if remaining_proof else '',
                '统计兼容专项末行':metrics_proof.splitlines()[-1] if metrics_proof else ''}
    finally: client.close()
    path=ROOT/'out/native-pvp-production-runtime.json';path.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(report,ensure_ascii=False,indent=2))

if __name__=='__main__': main()
