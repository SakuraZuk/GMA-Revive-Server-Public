# -*- coding: utf-8 -*-
"""将本次玩家日志修补资料整合到原有四份交接文档。"""
from pathlib import Path
import json
import sys

root = Path(__file__).resolve().parents[2]
if '--pending-tip' in sys.argv:
    status = ('原生Asio单字典恢复修补已随runtime2026100907发布并独立核验，游戏PID8130、SHA `310d023f9fb20ffe6fdba87ebb08cf1ef7546c785c59c8a7c7c83db1e9259947`。'
              '09:36新窗口无恢复断言，但UID1120的引导拒绝回调触发std::string类型异常；新增UTF-8字节错误回包及任务ID诊断候选已通过原生回调/网关/全仓测试和构建，'
              '正在完成实库门，候选尚未发布，不称引导全流程已通过。')
    for filename in ('SERVER.md', 'out/HANDOFF.md', 'out/REPAIR-TASKS.md', 'out/PROGRESS.md'):
        path = root / filename
        text = path.read_text(encoding='utf-8')
        begin = text.index('\n\n', text.index('\n## ')) + 2
        end = text.index('\n\n', begin)
        path.write_text(text[:begin] + status + text[end:], encoding='utf-8')
    print('四份交接已记录引导错误回包候选，未冒充已发布')
    raise SystemExit(0)
if '--pending' in sys.argv:
    previous = json.loads((root / 'out/native-solo-release-verification.json').read_text(encoding='utf-8'))
    next_index = json.loads((root / 'deploy/data/hotfix.json').read_text(encoding='utf-8'))['runtime']['index']
    status = ('上一批runtime%s已组合发布并独立核验通过，游戏PID%s、SHA `%s`。'
              '09:26:08新日志仍有UID612恢复断言；原生AsioGateClient以一个位置字典调用method(parameters)，'
              '仅兼容位置参数及展开关键字仍漏掉真实网络形态。runtime%s接续候选已修正，'
              '原生Python2RpcMethod单字典及ObjectId专项通过，正在完成构建和实库门；候选尚未发布，恢复黑屏未关闭。') % (
                  previous['热更版本'], previous['游戏PID'], previous['游戏SHA256'], next_index)
    for filename in ('SERVER.md', 'out/HANDOFF.md', 'out/REPAIR-TASKS.md', 'out/PROGRESS.md'):
        path = root / filename
        text = path.read_text(encoding='utf-8')
        begin = text.index('\n\n', text.index('\n## ')) + 2
        end = text.index('\n\n', begin)
        text = text[:begin] + status + text[end:]
        text = text.replace('线上审计仍为0903', '09:00时线上为0903')
        text = text.replace('恢复脚本版本9，同时读取原生_2/ _3关键字参数；', '恢复脚本版本10，兼容Asio单位置字典、原生_2/_3展开关键字及本地位置参数；')
        path.write_text(text, encoding='utf-8')
    print('四份交接已记录当前线上批次和接续候选')
    raise SystemExit(0)
if '--final' in sys.argv:
    release = json.loads((root / 'out/server-release.json').read_text(encoding='utf-8'))
    hotfix = json.loads((root / 'out/hotfix-bridge-release.json').read_text(encoding='utf-8'))
    remote = json.loads((root / 'out/remote-release-verification.json').read_text(encoding='utf-8'))
    native = json.loads((root / 'out/native-solo-release-verification.json').read_text(encoding='utf-8'))
    pg = json.loads((root / 'out/remote-database-verification.json').read_text(encoding='utf-8'))
    build = json.loads((root / 'out/completion-build.json').read_text(encoding='utf-8'))
    sha = build['产物哈希']['out/bin/linux-amd64/gameserver']
    assert remote['状态'] == native['状态'] == pg['状态'] == '通过'
    assert sha == release['SHA256'] == remote['游戏二进制SHA256'] == native['游戏SHA256']
    assert native['热更SHA256'] == hotfix['SHA256']
    assert pg['SHA256'] == build['产物哈希']['out/bin/linux-amd64/dbstore.test']
    stamp = release['备份'].rsplit('completion-', 1)[1]
    status = ('本轮修补及4101/4102后续重复入场、原生Asio恢复参数及引导错误提示字节类型修正已组合发布并独立核验通过，runtime%s、游戏PID%s。'
              '游戏SHA `%s`，热更SHA `%s`，发布报告 `out/completion-release-%s.json`。'
              '完整Go测试、vet及正式构建通过，真实PG%s项通过/0跳过，另%s项辅助通过；'
              '原生Python2回调、恢复和计时器专项通过。发布与静态合同不代表所有Android玩法通过。') % (
                  native['热更版本'], native['游戏PID'], sha, hotfix['SHA256'], stamp, pg['PASS数量'], pg['其他PASS数量'])
    for filename in ('SERVER.md', 'out/HANDOFF.md', 'out/REPAIR-TASKS.md', 'out/PROGRESS.md'):
        path = root / filename
        text = path.read_text(encoding='utf-8')
        begin = text.index('\n\n', text.index('\n## ')) + 2
        end = text.index('\n\n', begin)
        text = text[:begin] + status + text[end:]
        text = text.replace('0905新增抽卡/合成', '本轮新增抽卡/合成')
        if filename == 'SERVER.md':
            text = text.replace('## 全服玩家故障深查与0905修补', '## 全服玩家故障深查与本轮修补')
            old_auto = '## 自动化日志复查（2026-10-09 07:00，访问仍阻断）'
            old_player = '## 玩家分发测试故障修补（2026-10-09，0904候选未发布，SSH阻断）'
            if old_auto in text and old_player in text:
                left, right = text.index(old_auto), text.index(old_player)
                text = (text[:left] + '## 历史自动化复查与访问阻断（2026-10-09 03:04至07:00）\n\n'
                        '三个只读复查窗口均未取得新日志，游戏SSH首行握手及授权热更主机中转失败；未上传或重启，0904候选未发布。'
                        '原报告和followup-065914等采集目录保留，不能用02:28历史导入错误代替这些窗口的新异常。09:00访问已恢复，后续以本页顶部实际发布与新窗口为准。'
                        '采集工具连接前写采集状态.json，分别保存日志/角色快照状态，失败保留已取得文件且仅记录异常类型；避免空目录冒充无异常或保存授权参数。'
                        '当时实库连接池收窄、复用SFTP降低共享数据库和SSH压力，未删并发业务断言、未放宽3秒登录期限。\n\n'
                        + text[right:])
                text = text.replace(old_player, '## 历史玩家故障批次与0904候选阻断证据（2026-10-09）')
            marker = '- 在线快照：'
            tip_note = ('- 引导失败回包：09:36:14 UID1120收到[False,msg]后在原生guide_task_mgr.server_callback→tips.show_tips→set_string触发std::string类型错误。'
                        '仅将失败msg改为UTF-8字节，使网关原样保留并编码为msgpack bin，避免encoding=utf-8将文本转换为unicode；布尔失败、中文内容和任务校验规则保留。'
                        '失败日志补UID/OID/task_id及原因，不伪造激活或完成。Go专项验证拒绝不改进度与实际网关bin编码；Python2执行本版原生失败回调，'
                        'C++文本入口由严格str类型夹具验证，不冒充Android画面验收。\n')
            if tip_note not in text:
                text = text.replace(marker, tip_note + marker, 1)
            packet_note = ('原生调用补证：0906发布后09:26:08 UID612仍上报505/10205断言；DCE9232F的AsioGateClient.entity_message字节码确认调用method(parameters)，'
                           '外层包装收到一个位置字典，1F09877D的RpcMethod才转换_0至_3。版本10先识别该单字典，并兼容展开关键字/本地位置参数；'
                           'test_login_recovery_native_keywords.py使用本版原生RpcMethod和真实bson.ObjectId验证单字典调用、旧残留替换及同UUID幂等。'
                           '之前仅位置/展开关键字的夹具不覆盖实际网络入口，不作为恢复黑屏关闭依据。\n\n')
            if packet_note not in text:
                text = text.replace('修补依据及接口约束：\n\n', '修补依据及接口约束：\n\n' + packet_note, 1)
            extra = ('- 活动重复入场：首批发布后09:12:21现场同一4101战斗UUID连续创建三次，次秒客户端上报新建及prepare断言。'
                     '将已证实重复的4101/4102纳入同连接同UUID幂等准备；只回复调用方callback，保留Loaded、seed、UUID及已扣体力，'
                     '不重发start_server_battle_ok/prepare。单元用例覆盖两个副本连续三次重复；学会守护专用重新布阵既有用例仍通过。\n')
            if extra not in text:
                text = text.replace(marker, extra + marker, 1)
            text = text.replace('09:00日志目录followup-090021，', '09:00日志目录followup-090021，发布后窗口followup-091258及followup-091504；首次切换后重连窗口仍有租约/读取超时，09:13后新窗口无上述超时或rarity上报，不以短窗口称稳定。日志分类工具classify_player_logs.py排除同一次异常参数副日志后按异常/入口聚合，')
        path.write_text(text, encoding='utf-8')
    print('四份原有中文交接已绑定最终发布及独立核验报告')
    raise SystemExit(0)
status = "0905候选已完成正式Windows/Linux构建，游戏SHA `11968857ba6fc2f33e0f8e81153f6ceaab49afb9cefdf9dea7ee32662f459f44`；全仓Go测试及vet、原生Python2回调转型和恢复/计时器专项通过。真实数据库核验执行中，尚未发布，不得称线上已修复。"
summary = "09:00访问恢复后直接采集全服08:45至09:00客户端及服务端日志，222角色/17839卡存档结构审计未发现等级、品阶或UUID越界；这不证明UI数值正常。线上审计仍为0903游戏SHA `f7671857f95841c2822cea626d8ca0ee0fdece7d42d9977e117c61df20a5983c`、PID616。原有授权继续有效，工具直接使用已保存连接参数，不需要用户重新授权或提供逐个UID。"
details = """修补依据及接口约束：

- 抽卡结果：4名玩家上报字典没有rarity，原生BB2B671E回调将cards交给E88E0EDF界面；EB773222调用AFDD4FE7的rpc.revert_args。成功单抽/十连及碎片合成回调补card.card_list与__custom_type尾标记，属性存档仍为原结构，扣料/发卡事务不变。Python2实际原生RPC验证单抽/十连转为card对象并可读取rarity，未以夹具自造类型掩盖故障。
- 连续暖恢复：27名玩家41次新建战斗断言及后续prepare断言。多轮失败客户端残留UUID可能早于服务器上一轮UUID，收到服务器明确hs_recover_previous_uuid恢复授权后调用原生Avatar.raw_clear_battle清理残留，再进入原生新建；同一新UUID不重复新建，无恢复标记不清对象。恢复脚本版本8；不调用通关、不发奖、不改已结算状态。
- 计时器：纠正旧0903错误导入为from guis import gui_utils；保留到期/开启/关闭/开服候选与下一次日刷新，恒定卡池索引不再无限搜索日期。实际Android模块目录约束专项通过。
- 在线快照：现场hs-game平均CPU约305%，旧每连接100毫秒调用AdminPlayer，行锁事务读取并解析完整角色JSON。新增HumanAvatarSnapshotAccounts.HumanAvatarSnapshot(ctx, oid, since)返回Avatar、revision、changed、error；PostgreSQL用一条MVCC联表查询，无FOR SHARE，版本未变不传输JSONB，元数据仍每次读取。连接按OID缓存版本，无变化保留原进度，跨进程写入revision变化立即重载；错误不推进缓存，不放宽登录期限。新增实库用例验证首次读取、无变化、独立存储更新、元数据修改和不存在角色拒绝。

证据集中 `out/evidence/player-bugs-20261009/`；09:00日志目录followup-090021，resumed Go及实库输出resume-go-test.log/resume-pg.log，原生静态堆栈、回调类型与恢复专项均保存。HTTP解析失败、缺失语音切换/聊天入口、回归活动及未配置卡池继续逐项取证，不能用假成功回调关闭。角色UI数值、新手全流程和持续负载须发布后新窗口与客户端实际操作复核。
"""
server = root / 'SERVER.md'
text = server.read_text(encoding='utf-8')
heading = '## 全服玩家故障深查与0905修补（2026-10-09）'
text = text.replace('# 幻书启示录服务端技术说明\n', '# 幻书启示录服务端技术说明\n\n' + heading + '\n\n' + status + '\n\n' + summary + '\n\n' + details, 1)
server.write_text(text, encoding='utf-8')
for filename, section in (
    ('out/HANDOFF.md', '## 玩家引导与黑屏故障接续（2026-10-09）'),
    ('out/REPAIR-TASKS.md', '## 玩家分发故障现行验收门（2026-10-09）'),
    ('out/PROGRESS.md', '## 玩家分发故障修补进展（2026-10-09）'),
):
    path = root / filename
    text = path.read_text(encoding='utf-8')
    begin = text.index(section) + len(section)
    end = text.index('\n## ', begin)
    replacement = '\n\n' + status + '\n\n' + summary + '\n\n0905新增抽卡/合成原生回调转型、连续失败暖恢复清理和按进度版本读取的在线快照，并纳入0904原生gui_utils导入修正。具体源模块、接口与测试见SERVER顶部。0901至0903为修补沿革，02:28旧验证与03:00至07:00访问阻断不作为当前状态；历史报告保留。未取得新版本客户端复测前，角色UI/数值、教学、回归活动及HTTP相关门继续开放。\n'
    path.write_text(text[:begin] + replacement + text[end:], encoding='utf-8')
print('四份原有中文交接已更新，候选未冒充已发布')
