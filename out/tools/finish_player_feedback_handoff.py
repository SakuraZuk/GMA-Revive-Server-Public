"""把升格循环、归还、抽卡的发布证据整合到原有四份中文交接，不新建MD。"""
import hashlib
import json
import re
from pathlib import Path

root = Path(__file__).resolve().parents[2]
load = lambda name: json.loads((root/name).read_text(encoding='utf-8'))
build = load('out/completion-build.json')
pg = load('out/remote-database-verification.json')
live = load('out/remote-release-verification.json')
native = load('out/native-solo-release-verification.json')
release = load('out/server-release.json')
hotfix = load('out/hotfix-bridge-release.json')
sha = build['产物哈希']['out/bin/linux-amd64/gameserver']
hotfix_sha = hashlib.sha256((root/'deploy/data/hotfix.json').read_bytes()).hexdigest()
assert pg['状态']=='通过' and pg['SKIP数量']==0
assert pg['SHA256']==build['产物哈希']['out/bin/linux-amd64/dbstore.test']
assert live['状态']==native['状态']=='通过'
assert live['游戏二进制SHA256']==release['SHA256']==sha
assert native['热更SHA256']==hotfix['SHA256']==hotfix_sha
pid = native['游戏PID']
stamp = release['备份'].rsplit('completion-',1)[1]
report = 'out/completion-release-'+stamp+'.json'
assert load(report)['状态']=='游戏与独立热更组合发布通过'
status = f'''当前发布为 runtime2026100909、游戏PID{pid}。游戏SHA `{sha}`，热更SHA `{hotfix_sha}`；组合发布报告 `{report}`，已完成备份、原子切换及独立远端/原生运行时核验。完整Go测试、vet、正式构建通过；真实PostgreSQL {pg['PASS数量']}项通过、0跳过，另{pg['其他PASS数量']}项辅助通过。原生Python2实际RPC回调类型专项通过。'''
detail = '''

### 证据与原因

本轮重新读取项目18份MD，记录 `out/evidence/player-bugs-20261009/ascend-md-read.json`。全角色日志从09:35扩展采集到10:08，保存 `followup-100814/`；抽卡实际请求含2、109、122、501、601，原服务仅配置1、3；decompose_cards实际命中“业务方法尚未实现”。4102为升格试炼二层，日志出现09:40、09:41、09:43等重复恢复重开。

原生908202EE的net_delay在3秒心跳超时后调用client_need_recover_battle；原服务收到该请求便更换UUID、重开准备阶段。普通观察事件原本每条都锁行、解析并重写全部玩家JSONB；实际日志还有观察事务超时后连续跳号。两条链分别造成假延迟恢复丢波次和结果无法结算。旧日志分类未统计“战斗事件拒绝”，本轮分类工具已补入；09:35至10:08原始窗口有150次跳号、169次归属不符及2次超时，混合包含0906/0907进程，不拿此统计当0908上线结果。

### 接口和持久化规则

- 普通PVE观察：仍以do_command("__battle_event__",[envelope])上行。UUID、连续序号、握手、单位结构与显式胜方校验保留；回合诊断进连接缓存，最多128条/2MiB。ready/started/settings、结算及其他玩家事务带入检查点，result超时在同连接Tick每秒重试；收据防止二次发奖。非真人房间/排队连接的背景快照由100毫秒改为1秒；普通观察不再逐条额外读取PG。真人及单端原生PVP分支保持原事务路径。
- client_need_recover_battle：同一连接、同一UUID、已开局且最近60秒仍有合法观察事件时，保留当前波次；有暂存结果则先重试结算。新连接无缓存、长期失联或未开局仍按既有冷恢复重开；不恢复中途HP/AP，不假胜利、不补造掉落。
- connect_server(type=1)/Resume：0908上线跟踪发现UID1238的4101在真正TCP重连后从13上报，而持久检查点只到8，后续全部跳号。0909将未完成普通战斗的冷恢复接入实际Resume入口，随connect_reply整批下发新UUID、hs_recover_previous_uuid及原生prepare，复用已鉴权Avatar，不等待旧客户端再次超时请求恢复。真人、异步及单端原生PVP仍由各自恢复链处理。Fixture重连也改读最新角色状态，避免旧凭据快照掩盖这个路径。
- decompose_cards(callback_id,uuids)：按原生2948C748的get_decompose_materials与get_decompose_return_exp，优先speical_decompose_rule，否则稀有度固定材料；经验为当前经验×exp_factor加各历史等级最大经验×对应factor，取整入材料4。固定材料不按补完次数倍增。锁定、禁止归还、委托、防守、进行中战斗、最后一本可战斗卡、重复/外来UUID和溢出拒绝。删卡、返料、累计获得、卸契印、清理阵容空槽/助战及保留获得历史同事务完成。回调只有一个原生bonus.bonus，contain_items可to_list；失败先handle_error_msg，再回空bonus释放原界面等待。
- random_cards(callback_id,pool_id,count)：已接入35个非轮换非GM卡池，玩家报告的2/109/122/501/601均在内；费用、限次、稀有度、活动解锁及逐卡权重来自本版原包表。权重以整数精确保存表中4位小数；仍保留已有首抽/十连保底和计数。所有合法回调编号的业务拒绝均返回五参回调，空card.card_list与box.box保留原生类型，不再只记日志让UI等待。
- 跟进附带属性修正：10:44:10 UID756上报prev_exam_correct_info触发int没有iteritems。127E9B33声明该属性为exam.exam_choose_info，BE4D210F声明其键为Int、值为choose_info；不能发送平铺correct_num/total_num整数。现在按题号投影本角色已提交答案为每题correct_num 0或1/total_num 1，未答题为空字典并由原生get_info生成零统计；原生真实load/get_info专项通过。不把本角色数据称为原厂全服统计，不改变答题或奖励存档。

新增资产目录为card_return_catalog.json、gacha_native_catalog.json，含原表SHA256。生成入口 `out/tools/generate_player_return_draw_catalog.py`；业务入口card_return_business.go/gacha_native_catalog.go，观察缓存与重试在battle_observe.go/progress.go/service.go，背景读取在human_delivery.go/sync_pvp_human.go。JSON目录不是运营猜值。

### 验证与后续边界

`player_feedback_repair_test.go`覆盖模拟三波300条连续事件、开局/结算超时、同连接延迟恢复、不重复发材料、重复归还拒绝、卸印及玩家报告卡池。对应真实PG用例 `TestPostgresPlayerFeedbackAscendReturnAndReportedDrawPools`验证4102实际RPC、观察阶段不逐条重写JSONB、胜利结算、资产和卡池计数重登录持久化。原生 `test_card_callback_native.py`验证实际rpc.revert_args得到card_list、box和bonus.to_list。完整输出为ascend09-go.log、ascend09-vet.log、ascend09-build.log、ascend09-pg-final.log、ascend-native-callback.log，均在上述证据目录。

0908真实玩家窗口额外证据：10:40:00建立4102 UUID6ac854005785786fffb52348，10:42:48同UUID胜利结算，无重开；UID66随后进入其他材料副本，保存于followup-104431。多名玩家同连接延迟恢复保留原UUID，亦已记录。此证据是实际玩家上行与服务器结算日志，不是Android画面验收，也不能据单例认定全部设备或持续负载通过。0909新增真实PG凭据Resume验证，确认断线丢失的观察事件不会续用旧序号。

0909上线后窗口保存于followup-110004：游戏PID13865在10:58:58保留UID201的4102战斗UUID6ac8582325dd3ec26f61b059、序号105，10:59:01同UUID胜利结算。该窗口未见战斗事件跳号或random_cards/decompose_cards业务失败；没有实际请求日志不能当作这两项的玩家操作通过。窗口仍有上线重连时旧UUID事件拒绝、后台日界/租约/房间刷新超时及活动周期时钟回拨，还有其他未实现接口；这些问题保留为未关闭边界，不称全服无错误。

101日轮换池和9999 GM池继续明确拒绝，不拿静态阶段或GM配置当公开卡池；完整原厂promise_rules仍需单独补证。既有HTTP解析、未实现聊天/语音入口和角色UI数值等旧门没有因本轮测试而关闭。
'''
for name in ('SERVER.md','out/HANDOFF.md','out/PROGRESS.md','out/REPAIR-TASKS.md'):
    path=root/name
    text=path.read_text(encoding='utf-8-sig')
    text=re.sub(r'^## 升格循环、归还幻书与抽卡修补（2026-10-09）\n.*?(?=^## |\Z)','',text,count=1,flags=re.M|re.S)
    # 去掉被新批次替代的重复“当前发布”段；原故障取证保留在原章节。
    text=re.sub(r'^本轮修补及4101/4102[^\n]*\n\n','',text,count=1,flags=re.M)
    first,body=text.split('\n',1)
    section='\n\n## 升格循环、归还幻书与抽卡修补（2026-10-09）\n\n'+status
    section+=detail if name=='SERVER.md' else '\n\n三项业务已实现并发布；详细原包依据、接口、缓存与回调规则见SERVER顶部。升格4102长事件流、归还资产与报告卡池已通过真实PG验证；Android第三波及持续玩家窗口仍以实际日志复核，不将模拟波次当实机验收。\n'
    path.write_text(first+section+'\n'+body.lstrip('\n'),encoding='utf-8')
print('已整合四份原有中文MD，发布哈希与独立核验一致')
