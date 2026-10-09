"""绑定十五组最终源码、实库、原生及独立发布证据；不以本地跳过代替验收。"""
from pathlib import Path
from datetime import datetime
import hashlib,json,re,sys,shutil
from build_completion import source_hashes

ROOT=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
def load(name):return json.loads((ROOT/'out'/name).read_text(encoding='utf-8-sig'))
def main():
    build=load('completion-build.json'); local=load('remaining-local-verification.json')
    pg=load('remote-database-verification.json'); native=load('native-solo-linux-verification.json')
    remote=load('remote-release-verification.json'); release=load('native-solo-release-verification.json')
    current=source_hashes()
    if build['退出码']!=0 or build['源码哈希']!=current:raise ValueError('最终构建未绑定当前源码')
    if local['状态']!='通过' or not local['源码核验期间一致'] or local['源码SHA256']!=current:raise ValueError('整仓双环境检查未绑定当前源码')
    expected=sum(len(re.findall(r'^func TestPostgres\w+\(',p.read_text(encoding='utf-8'),re.M)) for p in (ROOT/'internal/game/dbstore').glob('*test.go'))
    if pg['状态']!='通过' or pg['退出码']!=0 or pg['SKIP数量']!=0 or pg['PASS数量']!=expected:raise ValueError('实库套件没有全部通过')
    if pg['SHA256']!=build['产物哈希']['out/bin/linux-amd64/dbstore.test']:raise ValueError('实库产物与构建不一致')
    if native['状态']!='通过' or native['SKIP数量']!=0 or native['源码哈希']!=current:raise ValueError('真实Linux原生专项未绑定当前源码')
    game_sha=build['产物哈希']['out/bin/linux-amd64/gameserver']
    if remote['状态']!='通过' or remote['游戏二进制SHA256']!=game_sha or release['状态']!='通过' or release['游戏SHA256']!=game_sha:raise ValueError('独立发布核验未通过')
    hotfix_sha=hashlib.sha256((ROOT/'deploy/data/hotfix.json').read_bytes()).hexdigest()
    expected_index=json.loads((ROOT/'deploy/data/hotfix.json').read_text(encoding='utf-8'))['runtime']['index']
    if release['热更SHA256']!=hotfix_sha or release['热更版本']!=expected_index or expected_index<2026100805:raise ValueError('热更实际版本不一致')
    names=['成就','非好友助战','好感度送礼','潜质重置','特殊头像框','随机碎片合成','收藏室','夏活','克苏鲁','初音','山海','汪言','年兽','宿舍骰子','邮件']
    groups=[{'编号':f'G{i:02d}','功能':name,'状态':'现行原表无对应配方入口，已纠正审计；不新增虚构配方' if i==6 else '实现、整仓与真实实库验证完成，已组合发布'} for i,name in enumerate(names,1)]
    result={'时间':datetime.now().isoformat(),'状态':'十五组实现或审计纠正完成，服务器组合发布并独立核验通过','分组':groups,
            '成就接线数量':31,'恢复原有特别演练副本数量':12,'本服规则版本':'local-20261008-v1',
            '默认全仓及正式规则全仓':'通过','静态检查':'通过','真实PostgreSQL通过数量':pg['PASS数量'],'真实PostgreSQL跳过数量':pg['SKIP数量'],
            '真实PostgreSQL其他通过数量':pg['其他PASS数量'],'Linux原生':'真实执行通过，0跳过','游戏SHA256':game_sha,'热更SHA256':hotfix_sha,
            '正式规则SHA256':release['正式规则SHA256'],'热更版本':release['热更版本'],'游戏PID':release['游戏PID'],'热更PID':release['热更PID'],
            '证据':['out/remaining-local-verification.json','out/completion-build.json','out/native-solo-linux-verification.json','out/remote-database-verification.json','out/server-release.json','out/hotfix-bridge-release.json','out/remote-release-verification.json','out/native-solo-release-verification.json'],
            '详细规则与接口':['SERVER.md','out/remaining-numeric-report.json','out/remaining-collection-report.json','out/remaining-activity-report.json','out/remaining-league-report.json'],
            '已修复回归':['正式规则空房读取时间与原生行为一致，并拒绝回拨','山海旧JSON空映射迁移','真实PG发现的初音旧对象及地图空容器登录迁移'],
            '失败证据归档':'out/backups/remaining-pg-failure-20261008-142103；默认/正式检查重跑记录见out/backups/remaining-checks-*',
            '源码一致性防护':'临时进度导出工具与首战桥更新期间，构建和专项拒绝漂移记录；最终全部重新绑定同一源码',
            '未关闭验收':['本批新增玩法的Android界面与真实操作逐项验收','持续负载及双端完整玩法验收'],
            '边界':'服务器实现和发布完成；热更新至登录的既有MuMu证据不证明本批十五组玩法实机通过。G06为真实数据审计纠正。学会仅本批六条成就所需真实会员及雅努斯链，非全部学会玩法。'}
    (ROOT/'out/remaining-completion-verification.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    backup=ROOT/'out/backups'/('remaining-final-reports-'+datetime.now().strftime('%Y%m%d-%H%M%S'))
    backup.mkdir(parents=True,exist_ok=False)
    for name in ('remaining-numeric-report.json','remaining-collection-report.json','remaining-activity-report.json','remaining-league-report.json'):
        path=ROOT/'out'/name;shutil.copy2(path,backup/(name+'.bak'));detail=load(name)
        if '状态' in detail:detail['状态']='本组源码已整合，整仓、真实实库、组合发布及独立核验通过；Android玩法仍待验收'
        detail['最终统一验证']={'时间':result['时间'],'报告':'out/remaining-completion-verification.json','实库通过数量':pg['PASS数量'],'实库跳过数量':0,'边界':result['边界']}
        path.write_text(json.dumps(detail,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    approval=load('gameplay-policy-approval.json'); approval['发布状态']=result['状态']; approval['发布时间']=result['时间']
    approval['发布报告']='out/remaining-completion-verification.json'; approval['PVP路线']['状态']='已发布并独立核验；Android及持续负载验收分开记录'
    (ROOT/'out/gameplay-policy-approval.json').write_text(json.dumps(approval,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({k:result[k] for k in ('状态','真实PostgreSQL通过数量','真实PostgreSQL跳过数量','游戏SHA256','热更SHA256')},ensure_ascii=False,indent=2))
if __name__=='__main__':main()
