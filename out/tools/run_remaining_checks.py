"""整仓默认与正式运营规则回归；不连接生产、不把本地跳过当实库通过。"""
from pathlib import Path
import datetime,hashlib,json,os,shlex,shutil,subprocess,sys
from build_completion import source_hashes
ROOT=Path(__file__).resolve().parents[2]
GO=Path(r__import__('os').environ.get('HS_GO', 'go'))
sys.stdout.reconfigure(encoding='utf-8')

def main():
    previous=['remaining-whole-default.log','remaining-whole-approved.log','remaining-vet.log','remaining-local-verification.json']
    available=[ROOT/'out'/name for name in previous if (ROOT/'out'/name).exists()]
    if available:
        backup=ROOT/'out/backups'/('remaining-checks-'+datetime.datetime.now().strftime('%Y%m%d-%H%M%S'))
        backup.mkdir(parents=True,exist_ok=False)
        for path in available:shutil.copy2(path,backup/(path.name+'.bak'))
    files=[str(p) for base in ('internal','cmd') for p in (ROOT/base).rglob('*.go')]
    subprocess.run([str(GO.with_name('gofmt.exe')),'-w']+files,cwd=ROOT,check=True)
    before=source_hashes();start=datetime.datetime.now().isoformat()
    formal=os.environ.copy();formal.pop('HS_TEST_DATABASE_URL',None)
    for line in (ROOT/'deploy/data/gameplay-rules.env').read_text(encoding='utf-8-sig').splitlines():
        if not line.strip() or line.lstrip().startswith('#'):continue
        key,sep,value=line.partition('=');words=shlex.split(value)
        if not sep or len(words)!=1:raise ValueError('正式运营环境格式无效')
        formal[key]=words[0]
    base=os.environ.copy()
    for key in ('HS_TEST_DATABASE_URL','HS_REMAINING_GAMEPLAY_POLICY','HS_SPECIAL_DRILL_REOPEN'):
        base.pop(key,None)
    result=[]
    for name,command,env in [('默认全仓',[str(GO),'test','./...','-count=1'],base),('正式规则全仓',[str(GO),'test','./...','-count=1'],formal),('整仓vet',[str(GO),'vet','./...'],formal)]:
        path=ROOT/'out'/({'默认全仓':'remaining-whole-default.log','正式规则全仓':'remaining-whole-approved.log','整仓vet':'remaining-vet.log'}[name])
        with path.open('wb') as log:done=subprocess.run(command,cwd=ROOT,env=env,stdout=log,stderr=subprocess.STDOUT)
        result.append({'名称':name,'退出码':done.returncode,'日志':str(path.relative_to(ROOT)),'日志SHA256':hashlib.sha256(path.read_bytes()).hexdigest()})
        print(name+' '+('通过' if done.returncode==0 else '失败'),flush=True)
    after=source_hashes()
    report={'开始':start,'结束':datetime.datetime.now().isoformat(),'状态':'通过' if before==after and all(row['退出码']==0 for row in result) else '失败或源码漂移','源码核验期间一致':before==after,'项目检查':result,'源码SHA256':after,'边界':'本地整仓和静态检查；本地PostgreSQL/缺省原生跳过不计入实库或Android证据'}
    (ROOT/'out/remaining-local-verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    if report['状态']!='通过':raise SystemExit(1)

if __name__=='__main__':main()
