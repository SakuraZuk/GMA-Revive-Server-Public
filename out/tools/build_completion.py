"""执行正式Windows/Linux游戏及真实库测试构建，记录源码与产物哈希。"""
import hashlib, json, subprocess, sys
from datetime import datetime
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]

def source_hashes():
    files=[p for folder in ('cmd','internal') for p in (ROOT/folder).rglob('*')
           if p.is_file() and p.suffix in {'.go','.sql','.json','.py','.html','.js'}]
    files += [ROOT/'go.mod',ROOT/'go.sum']
    files += [ROOT/p for p in ('deploy/data/hotfix.json','deploy/data/hotfix_startup_v10.py','deploy/data/activity-schedules.json',
                               'deploy/data/gameplay-rules.env','deploy/hs-game-gameplay.conf',
                               'deploy/hs-game-native-pvp.conf','deploy/native-pvp-run-python2.sh',
                               'out/native-pvp-engine-linux-resources.tar.gz',
                               'out/native-pvp-linux-packages/libpython2.7-minimal_2.7.18-8+deb11u1_amd64.deb',
                               'out/native-pvp-linux-packages/libpython2.7-stdlib_2.7.18-8+deb11u1_amd64.deb',
                               'out/native-pvp-linux-packages/python2.7-minimal_2.7.18-8+deb11u1_amd64.deb') if (ROOT/p).exists()]
    return {p.relative_to(ROOT).as_posix():hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(files)}

def main():
    sys.stdout.reconfigure(encoding='utf-8')
    start=datetime.now().isoformat(); before=source_hashes()
    result=subprocess.run([__import__('os').environ.get('COMSPEC', 'cmd.exe'),'/d','/c','deploy\\build-game.cmd'],cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
    stamp=datetime.now().strftime('%Y%m%d-%H%M%S'); log=ROOT/('out/completion-build-'+stamp+'.log')
    log.with_suffix('.raw').write_bytes(result.stdout)
    encoding='utf-8'
    try: body=result.stdout.decode(encoding)
    except UnicodeDecodeError:
        # CMD在批处理chcp之前的路径错误可能使用系统中文代码页；保留原字节并严格转UTF-8。
        encoding='cp936';body=result.stdout.decode(encoding)
    log.write_text(body,encoding='utf-8')
    after=source_hashes(); code=result.returncode if before==after else 125
    artifacts=['out/bin/windows/gameserver.exe','out/bin/linux-amd64/gameserver','out/bin/linux-amd64/dbstore.test']
    report={'开始时间':start,'结束时间':datetime.now().isoformat(),'退出码':code,'命令':'deploy/build-game.cmd','日志':str(log),
            '命令输出编码':encoding,'源码构建期间一致':before==after,'源码哈希':before,'产物哈希':{p:hashlib.sha256((ROOT/p).read_bytes()).hexdigest() for p in artifacts if (ROOT/p).exists()}}
    (ROOT/'out/completion-build.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({k:v for k,v in report.items() if k!='源码哈希'},ensure_ascii=False))
    if code: print(body)
    return code

if __name__=='__main__': raise SystemExit(main())
