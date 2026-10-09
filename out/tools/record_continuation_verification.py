# -*- coding: utf-8 -*-
"""汇总当前发布、实库、文档证据的文件SHA，保留旧汇总为历史快照。"""
import datetime, hashlib, json, shutil, sys
from server_ops import ROOT
from build_completion import source_hashes
sys.stdout.reconfigure(encoding='utf-8')
names=['continuation-go-checks.json','completion-build.json','remote-database-verification.json','native-solo-linux-verification.json','native-pvp-production-runtime.json','server-release.json','hotfix-bridge-release.json','remote-release-verification.json','native-solo-release-verification.json','handoff-doc-check.json','solo-migration-tools-archive.json']
if (ROOT/'out/remaining-functions-audit-20261008.json').exists():names.append('remaining-functions-audit-20261008.json')
reports={name:json.loads((ROOT/'out'/name).read_text(encoding='utf-8')) for name in names}
build=reports['completion-build.json'];pg=reports['remote-database-verification.json'];release=reports['server-release.json'];native=reports['native-solo-release-verification.json']
assert build['退出码']==0 and build['源码哈希']==source_hashes()
assert reports['continuation-go-checks.json']['状态']=='通过'
assert reports['native-solo-linux-verification.json']['源码哈希']==build['源码哈希']
assert pg['状态']=='通过' and pg['SKIP数量']==0 and pg['PASS数量']>=44
assert pg['SHA256']==build['产物哈希']['out/bin/linux-amd64/dbstore.test']
assert release['SHA256']==build['产物哈希']['out/bin/linux-amd64/gameserver']==native['游戏SHA256']
assert native['状态']=='通过' and reports['handoff-doc-check.json']['结果']=='通过'
for name,value in build['产物哈希'].items():assert hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==value
files=dict(build['源码哈希'])
for folder in ['out/tools','deploy']:
 for path in (ROOT/folder).rglob('*'):
  if path.is_file() and path.suffix in {'.py','.ps1','.sh','.cmd','.sql','.json','.env','.conf'}:
   files[path.relative_to(ROOT).as_posix()]=hashlib.sha256(path.read_bytes()).hexdigest()
for path in ROOT.rglob('*.md'):files[path.relative_to(ROOT).as_posix()]=hashlib.sha256(path.read_bytes()).hexdigest()
manifest={'生成时间':datetime.datetime.now().isoformat(),'范围':'正式构建输入及现行MD、deploy、out/tools文件SHA；只哈希，不复制配置内容或口令','状态':'源码、实库、生产和Android分层；当前已发布，Android/负载仍未验',
 '源码哈希':build['源码哈希'],'文件':[{'路径':name,'SHA256':value} for name,value in sorted(files.items())],
 '本批游戏SHA256':release['SHA256'],'本批PG测试SHA256':pg['SHA256'],'文档数':reports['handoff-doc-check.json']['文档数量']}
manifest_path=ROOT/'out/completion-source-manifest.json'
manifest_path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
target=ROOT/'out/continuation-final-verification.json'
stamp=datetime.datetime.now().strftime('%Y%m%d-%H%M%S')
if target.exists():shutil.copy2(target,ROOT/('out/backups/continuation-final-before-'+stamp+'.json'))
result={'时间':datetime.datetime.now().isoformat(),'状态':'单端PVP权威及年兽周三日奖修正已验证发布；整份A–F未全部关闭',
 '依据':[{'报告':'out/'+name,'SHA256':hashlib.sha256((ROOT/'out'/name).read_bytes()).hexdigest(),'状态':value.get('状态',value.get('结果'))} for name,value in reports.items()],
 '真实数据库':pg,'正式发布':{'游戏SHA256':release['SHA256'],'热更SHA256':native['热更SHA256'],'游戏备份':release['备份'],'游戏PID':native['游戏PID'],'热更PID':native['热更PID'],'原生运行时':reports['native-pvp-production-runtime.json']},
 'PVP权威':{'已经验证':'同步真人/机器人及异步真人离线防守/机器人真实原生唯一计算、映射/FIFO播放、持久Journal、冷恢复与事务计分','生产已激活':True,'边界':'发布前旧会话保留原契约；尚无Android与持续负载证明'},
 '年兽反推':{'已确认':'总榜sub_ranks由服务端下发，未参赛0显示为－；周三不发日榜奖已修正','缺失':'综合公式、未参赛计分、同分排序、小时总榜/周奖'},
 '其余未关闭':'REPAIR-TASKS保留46项编号；未知机制、未接业务、Android完整链与持续负载仍待完成',
 '源码清单SHA256':hashlib.sha256(manifest_path.read_bytes()).hexdigest(),
 '发布故障模型日志SHA256':hashlib.sha256((ROOT/'out/solo-release-safety-tests.log').read_bytes()).hexdigest()}
target.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
if 'remaining-functions-audit-20261008.json' in reports:
 audit=reports['remaining-functions-audit-20261008.json']
 result['余项审计']={'报告':'out/remaining-functions-audit-20261008.json','归并缺口组数':len(audit['归并后的实现或规则缺口']),'缺接线成就':audit['成就复核']['缺接线成就'],'边界':audit['范围'],'其余待关闭验证':audit['仅待验证']}
 target.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'状态':result['状态'],'文件':str(target),'游戏SHA256':release['SHA256'],'PG通过':pg['PASS数量'],'文档数':reports['handoff-doc-check.json']['文档数量']},ensure_ascii=False))
