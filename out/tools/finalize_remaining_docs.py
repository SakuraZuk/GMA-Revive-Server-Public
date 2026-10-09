"""把已独立核验的十五组最终事实合入四份现行MD及审计，保留历史备份。"""
from pathlib import Path
from datetime import datetime
import hashlib,json,re,shutil,sys
ROOT=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
def load(name):return json.loads((ROOT/'out'/name).read_text(encoding='utf-8-sig'))
def main():
    r=load('remaining-completion-verification.json')
    if '独立核验通过' not in r['状态']:raise ValueError('没有最终独立核验，不得关闭文档发布状态')
    release=load('server-release.json');hotfix=load('hotfix-bridge-release.json');runtime=load('native-pvp-production-runtime.json')
    stamp=datetime.now().strftime('%Y%m%d-%H%M%S');backup=ROOT/'out/backups'/('remaining-final-docs-'+stamp)
    backup.mkdir(parents=True,exist_ok=False)
    docs=['SERVER.md','out/HANDOFF.md','out/REPAIR-TASKS.md','out/PROGRESS.md']
    count=r['真实PostgreSQL通过数量'];aux=r['真实PostgreSQL其他通过数量']
    summary=f'默认及正式规则全仓、vet、最终构建和Linux真实原生全部通过；真实PostgreSQL {count} PASS、0 SKIP，另{aux}项辅助通过；游戏/两端热更/私有原生运行时已统一发布并独立核验。Android完整玩法与持续负载仍待验收'
    detail=f'''本轮实际关闭十五组服务器实现或审计纠正：31条成就已接真实业务，12个原有特别演练已按明确授权恢复。G06是本版无入口的审计纠正，不新增虚构配方；学会范围为此次六条成就所需会员与雅努斯基础。

{summary}。

当前游戏SHA256 `{r['游戏SHA256']}`；双端热更SHA256 `{r['热更SHA256']}`，runtime `{r['热更版本']}`；正式规则SHA256 `{r['正式规则SHA256']}`。实际进程游戏PID `{r['游戏PID']}`、独立热更PID `{r['热更PID']}`。私有原生运行时 `{runtime['目录']}`，原链接 `{runtime.get('原当前链接')}`。

组合发布报告 `{release['组合报告']}`；游戏回退目录 `{release['备份']}`；热更回退文件 `{hotfix['备份']}`。逐文件原存在状态与SHA以组合报告为准，只恢复所属项目文件/单元；若回退私有运行时，先核当前链接仍属于本批，再恢复原相对链接并重启所属游戏服务。

完整证据见 `out/remaining-completion-verification.json`、`out/remaining-local-verification.json`、`out/native-solo-linux-verification.json`、`out/remote-database-verification.json`、`out/native-solo-release-verification.json`。首次旧初音空映射与第二次委托登录遗漏的失败证据保存于 `out/backups/remaining-pg-failure-*`；修复后完整重跑。克苏鲁按实际回调名/原生整数核验，保留SAN成本、冷恢复、重复及整盒断言；邮件与演练实库用有效昵称进入业务，不放宽生产校验。

本轮构建缓存及临时目录为项目自有 `.\\_buildcache`、`.\\_buildtmp`：执行Go检查/构建前仅在本次命令环境设置GOCACHE/GOTMPDIR，不改全局环境。私有包已同步活动统计revision 3，归档SHA `{runtime['输入SHA256']['out/native-pvp-engine-linux-resources.tar.gz']}`；3098个实际Python模块及一项重定向条目维持原资源输入SHA `bed95c22e6e6adf4ccf01cabb5156b4888ccc5744f0e192d24501e6df3c19ef3`。14:56重登恢复的服务端永久入口已随本次游戏二进制上线；既有MuMu热更/登录/教学恢复证据保留，最终胜利结算及十五组Android操作继续单独验收。
'''
    for name in docs:
        path=ROOT/name;shutil.copy2(path,backup/(path.name+'.bak'));s=path.read_text(encoding='utf-8-sig')
        s=s.replace('均处于最终核验中：正在验证，最终数据由主代理填写',summary)
        s=s.replace('正在验证，最终数据由主代理填写',summary)
        s=s.replace('本轮十五组最终验证与发布仍核验中','本轮十五组服务器实现及统一发布已完成')
        s=s.replace('本轮十五组最终验证与发布核验中','本轮十五组服务器实现及统一发布已完成')
        s=s.replace('最终远端55项须重跑',f'最终远端{count}项已完整通过，0跳过')
        s=s.replace('最终远端55项仍须重新验证',f'最终远端{count}项已完整通过，0跳过')
        s=s.replace('需最终完整重跑','最终已完整重跑通过')
        s=s.replace("本轮十五组最终验证和发布仍核验中", "本轮十五组服务器实现及统一发布已完成")
        s=s.replace("生产基线与本轮待发布源码必须分别判断", "源码、生产版本与Android验收按证据分别判断")
        s=s.replace("55项真实PostgreSQL", f"{count}项真实PostgreSQL")
        s=s.replace("55项真实PG及组合发布", f"{count}项真实PG及组合发布")
        s=s.replace("此前通过快照遇新修复/漂移需最终重绑", "已重绑本次最终源码和产物")
        s=s.replace("主代理生成后填最终结果", "已记录最终结果")
        s=s.replace("最终SHA/PID/回退由主代理根据实际报告填入", "最终SHA/PID/回退见文末实际发布记录")
        s=s.replace("十五组统一最终核验中", "十五组统一发布并独立核验完成")
        s=s.replace("【本轮正在验证】默认/正式全仓、vet、构建、55实库及组合发布最终数据由主代理填写", f"【服务器发布完成】默认/正式全仓、vet、构建、{count}项实库和组合发布均通过")
        s=s.replace("【源码已补/审计纠正，最终核验中】", "【源码及发布完成/审计纠正】")
        s=s.replace("sourceconsistent要最终重绑，不沿用修复前快照", "已绑定最终同一源码，修复前快照保留历史证据")
        s=s.replace("首次40PASS后初音panic不计全量成功，不写55PASS", f"首次40PASS后初音panic保留失败记录；修复后完整{count} PASS")
        s=s.replace("（生成后填实际时间、SHA、PASS/SKIP、回退与运行期清单）；本节不预填未发生的发布", "（已记录实际时间、SHA、PASS/SKIP、回退与运行期清单）")
        if name=='out/HANDOFF.md':
            s=re.sub(r'^1\. 最终源码核验：.*$',f'1. 本轮最终源码核验与发布已完成：{summary}。首次失败与源码漂移证据保留；后续新改动须重新绑定构建/实库，不再重复施工已关闭的十五组。',s,flags=re.M)
        first,sep,rest=s.partition('\n')
        s=first+'\n\n> '+r['时间']+' 十五组服务器统一交付已完成。'+summary+'。当前游戏SHA `'+r['游戏SHA256']+'`，热更SHA `'+r['热更SHA256']+'`。\n'+rest
        if name=='SERVER.md':s+='\n### 17.11 最终统一验证、发布与接续\n\n'+detail
        else:s+='\n## 十五组最终交付记录\n\n'+detail
        if '\ufffd' in s:raise ValueError('最终文档含乱码：'+name)
        path.write_text(s,encoding='utf-8')
    path=ROOT/'out/remaining-functions-audit-20261008.json';shutil.copy2(path,backup/(path.name+'.bak'));audit=load(path.name)
    audit['北京时间']=r['时间'];audit['状态']=r['状态'];audit['范围']='当前十五组源码、整仓、真实Linux原生、完整实库、组合发布及独立核验；Android与持续负载另验'
    for group in audit['归并后的实现与审计纠正']:
        group['状态']='本版无入口，已纠正历史审计' if group['编号']=='G06' else '源码、整仓、真实实库及统一发布完成'
        group['最终验证与发布']='out/remaining-completion-verification.json'
        for source in group.get('源码证据',[]):
            p=ROOT/source['文件']
            if p.is_file():source['SHA256']=hashlib.sha256(p.read_bytes()).hexdigest()
    audit['验证与发布'].pop('55项真实PG',None)
    audit['验证与发布'].update({'默认及正式全仓/test/vet/Linux真实原生/最终构建':'全部通过，绑定最终源码','完整真实PG':f'{count} PASS、0 SKIP，另{aux}辅助通过','组合发布':'游戏、两端热更、私有运行时通过并独立核验','最终报告':'out/remaining-completion-verification.json'})
    audit['全部已读MD']={str(p.relative_to(ROOT)).replace('\\','/'):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(ROOT.rglob('*.md'))}
    audit['最终游戏SHA256']=r['游戏SHA256'];audit['最终热更SHA256']=r['热更SHA256'];audit['最终热修索引']=r['热更版本']
    path.write_text(json.dumps(audit,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'状态':'现行MD与审计已绑定实际最终结果','备份':str(backup),'文档数量':len(audit['全部已读MD'])},ensure_ascii=False))
if __name__=='__main__':main()
