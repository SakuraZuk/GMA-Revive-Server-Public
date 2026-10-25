# -*- coding: utf-8 -*-
"""将本轮全量反馈的已验证证据整合进现有四份资料，不关闭未完成玩家验收。"""
import json
from pathlib import Path

root = Path(__file__).resolve().parents[2]
path = root / 'out/bug-all-repair-matrix-20261012.json'
matrix = json.loads(path.read_text(encoding='utf-8'))
updates = {
    7: '原表2-10五槽真实开战及冷读已由PostgresEquipmentDailyDoubleReceiptColdRetryAndStage210Slots验证；原生槽容量修补1204已上线，Android本关实操待验',
    11: '探索空出战卡恢复布阵、自动开战入口与按需安装已有原版Python2回归；仍需原生锁队/自动推进完整Android流程，不能以服务端节点夹具关闭',
    13: 'RemainingExplorationAllNineChaptersRealNodeProgressAndAchievement从九章真实入口逐节点至boss并校验成就/冷投影；第一关Android复现和资产实际展示待验',
    19: '当前战斗输入异常已定位BattleEntity.battle缺失；原版assemble闭包最终入口修补及自动/手动组件整链通过，1209候选待发布/Android2-17实操，不等于本行已关闭',
    21: 'LogContractHabitWireAndColdCardProperties验证原生card.card_habit与UUID推送/冷属性；原版AI技能1/2/3/100选择及新战斗对象回归通过，Android技能锁操作待验',
    22: '同21：原生技能锁回推/AI选择与新战斗对象已验证，在线玩家当前战斗对象实际操作待验',
    28: 'ReportedDrawPoolsAndFailureCallbacks实际枚举池2/109/122/501/601生成幻书并保存次数，未知池失败不扣料；十连真PG已有冷持久回归，绿色钥匙具体客户端入口待验',
    29: 'PostgresTenDrawAndFirstBonusPersistence覆盖真实十连及首奖励持久；回调携原生card.card_list标记且先同步真实库存，客户端多次点击/延迟问题仍待现场计时',
    30: '隔离原版引擎同点双结界同时存在，均通过范围/阵营/标签选择且各自实际扣血；客户端连续施法创建及渲染未验，不能关闭',
    45: 'LayoutNativeSlotsPersistenceAndActiveBattle、EnterDungeonRestoresSeparateLayout和混合空槽开战已有回归；模型生命周期失效速度原版回归通过；抽卡后实际界面重建和阵容跨轮Android待验',
    53: '九章探索逐节点至boss、完成成就、冷序列化已由RemainingExplorationAllNineChaptersRealNodeProgressAndAchievement通过，真实PG探索事务回滚/冷恢复通过；第六章Android完整结算未验',
    58: '已查原表5010009累计白昼和声Total≥50000、单价3000、限购50，50张3411残页合成角色3411。新增MikuOriginalShopFragmentsComposeAndColdPersistence实际兑换/合成/限购/重复拒绝/冷持久本机通过；这项不在1210冻结候选内，新加PostgresMikuOriginalShopFragmentsComposeAndColdPersistence尚待新候选真PG，Android入口未验；地图/惊喜回归不替代角色获取',
    70: '普通副本冷恢复不会向不存在的battle推结果，活动恢复独立路径已有专项；旧战斗实体所属修补1209待上线；活动卡死的具体场景Android仍待验',
    77: 'ActivityHouseRPCPersistenceDailyAndMoveRollback实际领取骰子、进入63格棋盘、重入保留、遥控/双骰扣费和冷持久已有回归；真实PG棋盘设施/SSR与鬼怪回复通过；收藏室量子骰子入口Android未验',
    5: '探索模式2转换原生普通战斗1、空队回布阵及冷恢复已有源码/原生回归，1204已上线；六章全关卡实际结算和Android仍未完整验收',
    6: '低层装备七日奖励、契印真实生成、重复及跨日唯一性已有回归，1204真PG装备专项通过；用户已批准高层原表基础掉落先行；原石开启/打磨/鉴定及冻结回归本机通过，新候选实库/Android待验，幸运保底另补，不能关闭本行',
    8: 'all_stage_progress_test枚举主线/支线纯剧情实际入口，先同步进度再结果、重复及冷投影通过；真PGStage23Story专项通过，1-7/1-11解锁后Android转场仍待验',
    10: '普通战斗冻结奖励收据及材料溢出邮件1204已上线，真实PG回滚/冷恢复通过；灵智各层原版胜利及Android逐层结算待验',
    12: '原版空锁探索守卫拦截零出战卡并恢复布阵，不把援护当出战；原生安装/热升级/重复执行通过，1204已上线；Android锁队实操待验',
    18: '升格门槛解锁装帧与付费装帧隔离，QQBugAwakenThreshold及真实PGVoiceAndAwakenColdPersistence通过；原版皮肤切换Android待验',
    20: '同18，修补位格对应免费装帧，保留付费装帧获取条件；1204全仓及真PG冷持久通过，Android切换未覆盖',
    34: '原生解锁条件初始化缺失房间，真实布局/冷持久与资产回滚PG专项通过并已上线；实际陈列室家具放置仍须Android核验',
    35: '双倍开启消耗与冻结奖励收据、重复/跨日持久及原生回包已有PG回归，材料溢出1204纳入邮件；本行全部关卡/断线返还规则未完整实机验收',
    37: '启程前十次典藏规则按原生计数与历史迁移实现，单双抽/提前命中/空权重回滚及真实PG冷登录通过；客户端十连显示待验',
    42: '随机看板原生多模式/拥有卡资格/冷属性及真实PGGirlRandomModesColdPersistence通过；客户端随机切换待验',
    44: '1204线上捕获手动输入停用时BattleEntity缺battle异常；原版Python2整链已复现，补当前战斗登记实体所属后完整激活/停用及自动切换通过，旧战斗隔离；新增修补在1207候选，尚未发布/Android待验',
    59: '主线原生收藏室解锁与缺房间迁移已有PG布局/冷读覆盖；陈列大厅第一章家具与图形加载90%仍未定位完成',
    66: '委托昼夜刷新、提交/追回生命周期与PG持久回滚通过；1204补首次按需加载完成提示UUID到模板ID，原生整链通过，Android页面验收待补',
    69: '收藏室属性/布局/冻结住户及PG冷读回归通过；90%图形加载根因仍未确认，不能以数据契约测试关闭',
    71: 'CollectionGather原生资产/不重复发奖、真实PG解锁制造/领取/回滚通过；古币领取Android现场仍待核验',
    72: '委托真实全生命周期、阶段冻结及追回冷恢复PG通过；1204完成窗首次加载修补已发布，线上短采样无该堆栈不能代替实机追回',
    24: 'MuMu既有1.0.128客户端当前账号已实际打开幻书列表与沙之书详情，模型/属性可见；尚未覆盖抽卡后重建、全部幻书和退出路径，不能关闭整行',
    17: '原生领取阈值与内部行编号错位已修；日/周全部五档领取、重复拒绝与冷属性投影随1203全仓及真PG97 PASS通过；任务界面全路径仍待实操',
    31: '同17，原生daily_bonus_active投影20/40/60/80/100阈值，内部发奖收据按行去重；1203已上线，客户端红点现场验收待完成',
    64: '60活跃度阈值投影及真实领取/重复拒绝已有回归，1203全仓/真PG97 PASS通过；不把服务器回调替代所有客户端界面验收',
	54: '原版enter_dungeon闭包单ret契约通过；新增明确体力/锁定/关闭/未知错误码，替代传输层通用13兜底；拒绝资产不变与存储错误隔离专项通过，真PG待最终绑定',
	73: '助战候选最多1000份完整存档读取已定位；新增只读指定卡/装备契印/公开身份与双向关系投影，入选事务仍完整重读核验；真PG投影一致性及换代授权回归待最终绑定',
    56: '原表36700011..36700023配置6出战；旧统一4人拒绝已定位，原生130项槽容量与六关实际开战/冷读通过；223736真PG六人回归已通过，后续新增源码待最终验收/未发布',
    48: '原版choice3失败动画锁已复现；预检/服务器失败返回赠礼页、成功奖励和迟到隔离通过；尚未发布/Android验收',
    63: '同48，441023/441053选项choice3有效；保留服务端资格拒绝，不伪造赠礼成功',
    65: '原表2301102全零出战槽放行已复现；恢复布阵及补齐后开战专项通过；尚未发布/Android验收',
    67: '原表2302102空锁阵容修补通过；revision1加诊断包装精确升级、重复安装通过；尚未发布/Android验收',
    62: '原表地点714/417每章自动补完两阶段已定位；新增结算/登录补齐及唯一性回归通过；223736真PG主线补完已通过，后续源码待最终验收/未发布',
    76: '同62；完整第一章两阶段中技能补完一次，两章累计四阶段/技能两次；不凭反馈直接加技能',
}
updates.update({
    6: '1210已恢复高层装备原表基础奖励及原石三阶段，1211真PG100 PASS/0 SKIP含高层实际入口/冻结奖励/冷恢复/重复及已分解原件不补发；原版Python2与gate ObjectId回调通过，幸运保底依用户决定另补，Android装备及原石操作待验',
    19: 'BattleEntity.battle缺失及原版assemble捕获旧闭包已修，实际最终战斗类入口修补1210/1211已上线，原版Python2完整手动/自动链通过；Android2-17实操待验',
    26: '原表首段story/gaokao/story/gk1_1_1b的choose_character_53：A发209011，B发209012；新增ExamOriginalCharacterChoicesAndColdPersistence经实际Handle覆盖双分支、冷存档、重复/改选不发奖，本机通过；原版Python2资格、双参closure和男女材料判定通过，新增专项未进1211冻结候选，PG及图形剧情转场待验',
    44: '原版最终战斗类输入入口补当前战斗登记实体所属，保留旧所属隔离、完整激活/停用和自动切换；1210/1211已上线，原版assemble组合整链通过；Android网络延迟/手动切换场景未验',
    58: '原表5010009累计白昼和声Total≥50000、单价3000、限购50，50残页合成3411。1211真PG100 PASS/0 SKIP含PostgresMikuOriginalShopFragmentsComposeAndColdPersistence实际兑换→冷登录合成→冷读及重复/限购拒绝，已上线；Android活动商店/合成入口待验',
    70: '普通副本冷恢复不会向不存在的battle推结果，活动恢复独立路径已有专项；旧战斗实体所属修补1210/1211已上线；活动卡死具体Android场景仍待验',
    73: '1211助战资格预选只读指定卡/身份/双向关系，不读无关契印；最终锁内读真实卡和绑定契印并重新授权，只返回self完整存档。真PG投影一致/未知大整数保留/版本不变/整事务回滚通过并已上线；当前日志短窗口不能替代长期高并发与Android切换验收',
})
for row in matrix['行']:
    if row['行'] in updates:
        evidence = updates[row['行']]
        for stale in ('真PG待最终绑定', '真PG投影一致性及换代授权回归待最终绑定', '后续新增源码待最终验收/未发布', '后续源码待最终验收/未发布', '尚未发布/Android验收'):
            evidence = evidence.replace(stale, '1202最终候选真PG95 PASS/0 SKIP并已上线；Android对应玩法未验')
        row['当前证据'] = evidence
    # 明确区分已有实现的验收缺口和仍未定位的根因；在线历史标签保持原快照。
    implemented = {5, 6, 7, 8, 10, 11, 12, 13, 17, 18, 19, 20, 21, 22,
                   28, 29, 31, 34, 35, 37, 42, 44, 45, 48, 53, 54, 56,
                   58, 62, 63, 64, 65, 66, 67, 70, 71, 72, 73, 76, 77}
    if row['行'] in implemented:
        row['本轮状态'] = '相关实现已上线，反馈场景实机验收未完成'
    elif row['行'] in {24, 26, 30}:
        row['本轮状态'] = '部分流程或隔离专项通过，原反馈完整场景未确认'
    elif row['行'] in {59, 69}:
        row['本轮状态'] = '已追到附加加载阶段，实际卡住任务与根因未确认'
    else:
        row['本轮状态'] = '相关共同修补不替代本条根因定位，继续核查'
assert len(matrix['行']) == 70
path.write_text(json.dumps(matrix, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')

section = '''## 2026-10-12 全部70条继续施工：剧情塔、赠礼失败与主线自动补完

本轮继续覆盖全部70条，不以已修补条数代替全清。1202前候选213405与215219均完成全仓Go、vet、Windows/Linux构建，真实PG92 PASS、0 SKIP、5辅助PASS，Linux原生专项通过；这些候选尚未发布。215219包括剧情塔和送礼失败保护，但不含后来新增主线自动补完，因此不能用它的验收替代最终源码。原表1461关卡、219地点、11章节目录核对无差异，只是静态契约，不能作为全部玩法通关。

221419候选包含主线自动补完，真实PG93 PASS、0 SKIP、5辅助PASS，历史缺末尾714不补、补齐后登录补完、独立Store冷读及不重复发放已经通过；全仓Go、vet、构建与Linux原生专项通过。之后继续发现特殊阵型和旧推荐回调缺口，因此该候选仍未发布，也不能作为后来源码的最终验收。

222645候选真实PG有1项六人用例失败，不能发布。失败原因是用例在登录战斗重开后没有发送原生load_entity_finish，服务端正确等待实体加载；补齐实际客户端握手后，223736候选真实PG94 PASS、0 SKIP、5辅助PASS，全仓Go、vet、正式构建及Linux原生专项通过。223736仍未发布：之后继续新增副本业务拒绝和助战投影，必须重新绑定最终源码验收，不将旧候选的94通过当新源码通过。

224846候选真实PG93 PASS、0 SKIP、5辅助PASS，但有2项失败，不能发布：新增拒绝入口夹具把过长账号作为昵称导致取名拒绝，另一个原引导门槛用例仍期望服务层error而不是本次新增1003回调。均已按实际失败修正夹具/断言，不取消昵称长度及引导门槛。225521新候选重新通过全仓Go、vet及正式构建，真PG与Linux原生验证进行中；助战预读实库投影一致性及原陌生助战原子冷恢复在224846已通过，仍须绑定最终候选。

本轮通过MuMu官方CLI定位既有Android12实例2（原ADB16448），启动原有com.netease.hsqsl，无重装/清数据/修改资源。真实客户端1.0.128已显示标题与公告场景，游戏进程2766；尚未进入角色或验证各关卡。22:15后旧1201线上窗口仍有19数据库超时、26客户端异常与4阵容数量拒绝，无服务重启/OOM；不能以客户端进程存活代替登录白光问题修复。两种正式资源入口所有NPK HEAD长度/Range及校验表长度/MD5本机检查通过，不等于vivo网络或完整资源哈希验收。

副本入场原版dungeon_mgr.enter_dungeon.server_callback只接收一个ret，再调用本次callback与show_error_tips。现有传输层已对失败入口提供通用13兜底；新增业务层明确错误码1005（体力）、1003（锁定）、1004（关闭）、1019（未知），条件无法解析和存储故障仍保留诊断错误。原版四种错误闭包回调、实际拒绝资产不变、无效协议/损坏规则隔离及数据库故障不伪造业务拒绝专项通过；已有教学入口与门服协议回归更新为对应原版错误码。真PG新增拒绝回包/独立Store冷读无扣费用例待最终绑定，不能把错误提示当不可用功能完整实现。

refresh_assist_use_times原候选查询读取最多1000份完整进度，即使随后只锁10名也会传输/解码无关库存。新增AssistSummaryOnly只读投影，保留公开身份/头像、指定助战UUID、该卡、该卡装备契印与好友/黑名单；随机预选后原UpdateSocial事务仍完整重读并核验指定卡、关系和授权，锁顺序与双向黑名单保护不变。全量角色读取及其他助战业务不启用该投影。新增真PG与夹具投影一致性和原陌生助战原子/冷重试回归待最终绑定，不能声称线上长负载已通过。

21:40后匿名服务窗口68508行，旧1201进程237608仍运行；18客户端异常、17数据库超时、8布阵数量拒绝，无OOM/自动重启。BF29F0ED.map_mgr.generate_slots按formation_data生成槽，EB947BB0.get_battle_cards分出战/援护列表；本版130种特殊战斗有5/6/7出战或4援护阵型，统一4/2错误拒绝。已合并派生容量到既有battle_story_slots.json，按冻结副本battle_id取原生上限，普通/PVP通用4/2和固定剧情角色空槽保护保留。时代从零重启/传说将被改写36700011..36700023全部6人开战、重复请求和冷读取通过；130项原版generate_slots/get_battle_cards一致，超出关卡上限仍拒绝。新增真实PG六人冻结与独立Store冷读用例须绑定新最终构建执行；不能把该规则根因当作六关完整剧情/Android结算已通过。

推荐好友请求在按需补丁之前可能已捕获旧on_get_search_friend绑定回调，修改类方法不会替换已存闭包。新Avatar请求包装保留原生缓存/RPC及回包，只对确切原版推荐回调按本次friend_items对象身份和tab长度保护，列表销毁/换代的迟到请求不更新新界面；有效推荐仍沿原版刷新。缓存越界失败对照、正常刷新、原生异步闭包捕获和换代隔离通过，见test_feedback_callbacks_native.py。客户端资源及原版玩法数据没有修改。

剧情塔2301102/2302102原表是普通enter_dungeon入口，原版battle_cards.check放行全零出战槽。守卫从探索类型2扩至剧情塔23，固定my_avatar_list、storyline_cards及其他玩法仍沿原版，援护不当出战卡。原表两关空锁阵容、回到布阵、重复触发、补齐后继续通过；旧revision1守卫外还有诊断包装，已从诊断器保存原函数取回原生函数并刷新诊断缓存，热升级和幂等安装通过。资料见test_qq_empty_explore.py与test_ui_callback_repair_native.py；图形对象为夹具。

天体4410的choice3原生先置in_anim，material_mgr消费失败只显示错误、不回调成功，因此界面锁持续。按原版资格、特殊次数降级及RPC保留消费逻辑，只有原版特殊送礼闭包对应的当前界面/请求代次在失败时执行原生after_story返回赠礼页；不播放成功对白、不发奖励。预检失败、服务端失败、真实成功奖励、旧请求/旧界面隔离及次数降级通过，见test_qq_gift_failure_native.py。此项保护不能替代具体送礼资格失败原因或Android选择验收。

沙之书自动补完根因由原版return_to_city→check_chapter_finished→sp_card_enhanced_result.show_special_card定位。sites714（第一章）、417（第二章）各有card_auto_enhance=[4401,2]，必须整章非支线/活动地点全部完成；第一章两阶段包含一次技能补完，两章四阶段包含两次。原表card_forbid_list=[5,6]，common_const确认禁止分解和手动补完。新增ensureMainlineAutoEnhance在真实战斗/纯剧情结算及登录事务补齐历史存档最低应有阶段，首个拥有实例与原版展示一致，不耗素材、不降低已得阶段。单节点缺失不发、两个实例隔离、等级/经验/资产不变、两章累计、冷复制幂等、实际714入口状态在battle_result前同步均通过；新真实PG历史登录用例仍须绑定最终构建执行。原生触发/章节/地点/禁止规则取证为out/bug-*-enhance-*-20261012.txt及bug-chapter-finish-native-20261012.txt。

仍保留结算材料容量拒绝、技能材料本及活动完整结算、六人三组、主城/收藏室加载、vivo更新、推荐/学会/排行榜/助战超时和全部客户端实操缺口。不能删除容量保护或猜测发奖；最新代码尚未最终切换，在线70条状态未回填。新增技术说明整合进既有四份中文UTF-8文档。

'''
# 候选准备/失败阶段已由最终证据替代，不让后续AI把旧的未发布状态当现状。
section = section[:section.index('本轮继续覆盖')] + '''全部70条持续修复，不以一批发布代替全清。正式候选20261012-225521包含剧情塔空阵容保护、赠礼失败解锁、沙之书自动补完、原生特殊阵型、副本业务错误码和助战候选精简投影。全仓Go、vet、Windows/Linux构建通过，真实PostgreSQL95 PASS、0 SKIP、5辅助PASS，Linux原生专项通过。2026-10-12 23:03组合发布1202成功，游戏PID245661，游戏SHA256=7df235b561352d94d2d8f6bac8721a620a9e8b3f290d5f43821f1f42519b867d；独立热更PID106851，双端私有hotfix SHA256=f9136d724a94b9200e256d072fda64105d2c898b07bd220ce17b058d54118828。发布记录out/bug-all-1202-final-completion-release-20261012-230306-810349.json。SHA/PID仅是本次核验时点，不保证未来进程不变。

MuMu既有Android12实例2、原客户端1.0.128已真实进入主城，无重装、清数据或修改资源；该单次主城证据不能关闭全部登录白光、收藏室、都市及各关卡验收。服务器列表后端直连、HTTPS、HTTP已通过。正式资源两种入口所有NPK的HEAD长度/Range与校验表本机长度/MD5通过，不等于vivo实际下载或全资源内容验收。

本批战斗材料容量规则由用户明确选择：容量内到账，超出部分保留邮件，释放材料容量后领取。普通副本与活动结算包装withBattleOverflowMail只在战斗奖励上下文暂存超出量，商店、日常及普通邮件领取仍保留原容量拒绝。按单材料limit_count拆附件，中文邮件、奖励冻结、无过期时间。邮件编号与UUID由本场UUID/材料/拆分序号确定，入库前检查正式邮箱与待投递邮箱冲突；500封满时写server_battle_overflow_mailbox，不丢奖或阻塞结算，删除邮件/登录时自动投递。材料到账、邮件/队列、通关进度与已发奖标志同一资产事务提交。结算及显式恢复同步short_mail_info；容量未释放的领取整盒拒绝，不部分写入。

普通副本显式结果先以短事务预演并保存reward_bonus_plan、reward_result和reward_planned_at，再按收据兑现随机材料/幻书/契印。预演不提交资产或进度；兑现回滚、断线和跨日重试保持相同来源/UUID/数量，不重新抽奖；已冻结胜负不能被退出改成失败。冷登录兑现后只同步属性并进入主城，不向不存在的原生battle推结果；显式恢复仍可补发已持久原盒。首通及每日来源沿用冻结计划，避免跨日变化导致来源不匹配。已完成或归属不符的重复结果不入待重试队列，原版可省略finished_task_list的结果包继续支持。

活动包含独立随机事件及状态转换，本批没有将这些规则全部冻结：材料溢出处理已覆盖活动结算，但不能宣称活动所有随机分支冷重试均已固定。幻书和契印库存上限不是本次材料溢出方案的授权对象，继续保留其容量校验，单独核查相关反馈。用户已批准高层基础掉落先行；契印原石本机实现已接入，幸运保底另补，真实数据库和Android对应流程仍须独立验收。

新增专项覆盖拆分、不过期、冷序列化、容量不足领取不变、释放容量领取、重复结果/附件拒绝、满邮箱队列/编号冲突、普通随机契印预演后回滚及跨日冷恢复。第一次全仓Go已通过；后续补充冷登录场景与邮件属性同步后，232539候选runtime1203正在重新执行全仓Go、vet、正式构建、真实PG和Linux原生绑定验证，尚未发布。真实PG新增两项分别走原表6101随机契印真实结果并注入第二事务回滚，以及固定收据夹具覆盖满邮箱/冷登录/领取；后者只验证兑现链，不作为原版副本掉落证据。

225521之后的源码不能引用1202已发布产物作为验证。私有分发备份旧版本被prepare保护检查拦截，已核对现网游戏二进制与双端hotfix精确匹配1202正式发布SHA后刷新本机私有备份，远端未写入。所有后续候选采用新runtime索引，不能用相同1202对应不同正文。

'''+section[section.index('副本入场原版'):section.index('仍保留结算材料容量拒绝')]+'''全部70条矩阵仍保留逐项验收缺口，在线状态未回填。主城/收藏室/都市加载、vivo更新、具体赠礼资格失败原因、高层装备、活动所有关卡/动画、长负载及Android实操继续处理。新增技术说明整合现有四份中文UTF-8资料，不创建重复长期文档。

'''
section = section.replace('232539候选runtime1203正在重新执行全仓Go、vet、正式构建、真实PG和Linux原生绑定验证，尚未发布。', '232539候选runtime1203全部绑定验证通过：全仓Go、vet、Windows/Linux构建、真PG97 PASS/0 SKIP/5辅助PASS、Linux原生0 SKIP；23:33组合发布成功，23:40只读核验游戏PID248226、运行进程与构建SHA一致、独立热更PID107244、双端配置SHA一致及服务器列表直连/HTTPS/HTTP通过。游戏SHA256=06c88cbb6d494d1b459a9ab7bb9e5f5c5c465c94ee42445bd28153449fb7a111，双端hotfix SHA256=553a75371812484dbd490e6f3b42452b6ffc3f529b95f8a3a5176c887f0cd7b4。报告归档out/bug-all-1203-final-*。')
section += '''1202发布后23:09至23:27匿名日志采样24360行，3数据库超时、10客户端异常，无OOM/panic/服务重启/战斗序号拒绝；异常包含外部遥测错误和委托完成窗ObjectId类型转换，不能宣称该窗现场已完全修复。原版general_tips_mgr.real_show_tips在分发时才获取gui.consign_finish_new，完成窗类可能在首次回包中加载而早于5秒轮询。新增提示入口转换，入队前把真实拥有卡UUID映射模板ID，空卡只显示已经提交的真实奖励，其他提示类型/参数/排队/演出保留。原版show_tips及real_show_tips执行到未包装完成窗的整链回归通过；完整安装器在完成窗模块尚不存在时也保护入口，重复安装不叠加。工具test_feedback_callbacks_native.py和test_ui_callback_repair_native.py通过；该修补不在1203产物内。

继续发现普通剧情塔的额外任务奖励在原副本奖励包装之后发放，扩大同一结果事务的溢出收集边界，内层只复用同战斗上下文，异战斗嵌套仍拒绝。普通结果预演也包含settleRemainingDungeon；reward_extra_plan_start划分副本与额外任务收据，防止把任务奖励误当副本奖励重复兑现。原表7001三任务实际入口/冻结/完整资产增量专项通过，其他绝密层和探索回归通过。server_battle_overflow_mailbox只持久保存，InitialProperties删除该内部属性，客户端仅收到原生short_mail_info。新234411候选runtime1204重新执行完整绑定验证中，未发布；不能把1203通过当本次新源码通过。

MuMu原进程2766日志23:38确认由1202加载1203；当前账号实际幻书列表及沙之书详情显示模型/属性，未消耗道具、未抽卡或改造资产。仅是这个账号的这两页，不覆盖收藏室、都市、全幻书或抽卡后场景重建。高层装备本的基础掉落表已查出，但额外幸运保底规则缺少原版服务端证据，用户已经明确批准先恢复原表基础掉落、保底另补；本轮按此范围实现，不增加未知保底规则。

'''
section = section.replace('新234411候选runtime1204重新执行完整绑定验证中，未发布；不能把1203通过当本次新源码通过。', '234411候选runtime1204已通过全仓Go、vet、Windows/Linux构建、真实PG97 PASS/0 SKIP/5辅助PASS及Linux原生专项。23:54组合发布成功，2026-10-13 00:00只读核验游戏PID250393及运行二进制SHA一致，独立热更PID107577，双端hotfix SHA一致，服务器列表直连/HTTPS/HTTP通过。游戏SHA256=689b39a9bf513b4dc946f3cf1843072c75f367a1be3ae2ea7428b5c8a42bd663；hotfix SHA256=66af7a61a5c4738c83bcf55c198cab39a399d50a08dd3d6dc0f56c1d0ca7203b。报告归档out/bug-all-1204-final-*；这些证据仍不代替Android对应玩法验收。')
section += '''活动排行榜进一步消除整份存档传输：山海子榜仅返回当前mountain_dungeon，年兽子榜仅返回当前nian，年兽总榜仅返回冻结nian_total；同时保留已选头像、头像框、自定义头像清除状态及公开同步竞技成绩。原COUNT与页面仍处于同一只读REPEATABLE READ快照，服区过滤、冻结阵容、同分OID顺序和个人全量名次不变。新增1107角色分页/外观/无关资产隔离回归，当前本机专项编译通过但数据库跳过，真实PG须在新1205候选重新验证；不能引用1204产物作为这项通过证据。

'''
section += '''2026-10-13的1204线上匿名采样仍有战斗输入异常：原版battle_mgr.on_allow_player_input调用fashion.activate(False)，combat.on_deactivate访问BattleEntity.battle缺失。源码新增当前所属绑定保护，仅当gworld.get_battle与本管理对象一致、该实体和entity_infos已登记且所属为空时补self，随后完整执行原生停用/激活、鼠标取消及自动切换检查。既有非空所属不覆盖，旧战斗不接入新对象，其他异常不吞。真实Python2原函数整链复现/修补、自动切换检查、旧战斗隔离、按需加载安装及幂等通过。原生assemble闭包提前捕获组件函数，因此包装local/shadow/record/watch实际战斗类最终入口，完整安装/原生组合闭包回归通过；1205排行与1206早期输入候选各真PG97 PASS/0 SKIP但未发布，最终1207候选Linux专项因SFTP断线残留写句柄报Text file busy失败，禁止发布；新候选以新inode激活且复核哈希后重新验收；只能定位具体堆栈，不能据此宣布清单44已实机修复。

'''
section += """2026-10-13续修：1210候选010441完成全仓Go、vet、Windows/Linux构建、Linux原生、真PG99 PASS/0 SKIP/5辅助PASS并于01:16组合发布，已核验游戏PID255491、SHA256=a2aaa77e84524ade448368ec0d0dd934dd5f7581a160b09b6bc18f4c56eb12dc、独立热更PID108512、双端hotfix SHA256=4df7e4f87c162dae1aaee24198059b68713c47e98dbc6b9ccbd74d74a3848941、服务器列表三入口通过，归档out/bug-all-1210-final-*。1208候选全仓一致性检查发现服务器内嵌runtime索引仍1207而配置1208，禁止发布；已统一为1209并生成独立005222候选，全仓Go、vet、Windows/Linux构建及Linux原生专项通过，真PG99 PASS/0 SKIP/5辅助PASS，1209已组合发布且01:03只读核验通过，游戏PID253857、SHA256=e91db9ab84c5c820c409fa98ae3e0893c9302825e8e688f05e8397a569004318，独立热更PID108176、双端hotfix SHA256=d554a3928fcd5c5608ebd814c50636c588378d14ef71157dfb96a044c13b4d9b，服务器列表直连/HTTPS/HTTP通过，归档out/bug-all-1209-final-*。补查原石普通RPC回调键类型需显式ObjectId，主源1210已修正，不能把1209实库通过当1210通过。用户已选择高层装备本先恢复原表基础掉落，幸运保底另补。奖励解释器只在装备本当前每日基础来源临时允许Lucky字段，不扩展商店或自选盒资格；完整发固定/随机契印、材料与原石，额外幸运分支尚未启用。原石Type15存current_rune_stone专用状态，不作为普通材料累计。按原表每日1颗、有效2小时；未过期不覆盖，同日过期不绕每日上限，北京时间日界重置次数。旧日冻结兑现不得回拨新日次数或阻塞其他奖励。

原石RPC step_0_rune_stone(callback,materialID)、step_1_rune_stone(callback,UUID列表)、step_2_rune_stone(callback,UUID列表)，均回三参(success,msg,rune_info)。开启最多100次，优先23×1、否则11×10，原石目标盒抽10件隐藏契印；打磨累计最多3件、原表免费；鉴定按原表20项星级/部位金币12价格，校验真实背包容量与UUID冲突，同事务扣费发原件。开启只揭示star/suit，打磨加pos/extra_suit，鉴定才发完整属性。隐藏runes与server_day不下发；普通RPC回调rune_info键显式ObjectId，以真实gate编码Ext42核验，不能只用属性容器自动转换代替；原版RuneStone.load真实ObjectId双字典及阶段校验已通过。相同参数重开属于合法新抽奖，完成阶段重传不重复扣费，已分解鉴定件不能再领。仅明确业务拒绝转换失败回调，存储/目录故障保留诊断错误。本机原石全流程、装备全层七日基础生成、原石冻结跨日兑现回归通过；新实库用例HighEquipmentBaseRuneStoneStagesAndColdRecovery已加入1209候选，1209整套真PG99 PASS/0 SKIP/5辅助PASS，但补查发现普通RPC回调没有自动转换UUID键；1210已显式ObjectId键并有业务/实际gate编码断言，须重新绑定候选验收，不以本机SKIP计通过。

初音获取已确认原版来源：商品5010009需累计50000白昼和声，单价3000和声换1残页，永久限购50，配方3411需50残页并确定生成3411角色。MikuOriginalShopFragmentsComposeAndColdPersistence经真实Service.Handle覆盖累计资格不足不扣料、150000和声兑换50残页、限购重试、原件与获得时间、冷存档和重复合成拒绝，本机通过；新增测试未进入已冻结1210，真实PG整链和Android入口仍待验。不凭本测试给线上角色补发资产。

继续线上匿名审计1209窗口11314行仍有助战刷新5次/排行3次超时，不能宣称消除。排行失败目录明确为4/0同步竞技榜。只读EXPLAIN：原助战候选投影5秒超时；去掉预选契印并jsonb_to_record一次提取后230行872.364毫秒；同步竞技原查询1233.741毫秒，一次提取完整公开外观后690.126毫秒。这是当前只读查询样本，不是长期压力结论。主源新增AssistCandidates(ctx,host,limit)只返回指定卡/身份/双向关系，维持同服OID排序最多1000窗口及原随机十人算法，最终候选仍由UpdateAssistRefresh锁内读取完整指定卡契印并重新授权。非入选候选不解码契印；预选结果不写回任何提供者存档。同步竞技榜改一次jsonb_to_record提取score及外观，正积分/同服/同分OID/limit不变。新增真实PG预选资格字段/无契印隔离断言，原持久授权和回滚用例保留；新代码本机编译通过但PG跳过，待1211重新绑定完整验收。

助战refresh_assist_use_times新增锁内精简投影：self仍完整存档；提供者仅助战卡、绑定契印、公开身份、好友/黑名单。保持原依赖咨询锁、全部avatars排序锁和同顺序progress锁，不用预选旧快照替代锁内授权；提供者进度被回调修改立即拒绝并回滚self，只返回self完整Avatar，禁止把投影替换在线完整存档。其他选择/消耗/开战仍走完整事务，夹具可回退原接口。新增真实PG投影一致/原存档未知大整数/修订不变/整事务回滚用例，本机数据库SKIP，等待新候选真PG验收；不宣布线上超时已消失。

"""
latest = '''最新正式线上为1211（2026-10-13 01:34核验），候选012017完成全仓Go、vet、Windows/Linux构建、Linux原生专项、真实PG100 PASS/0 SKIP/5辅助PASS；组合发布报告completion-release-20261013-012944-104110.json。游戏PID257243，运行二进制SHA256=762d949796fba181bfcc2e329ad98c9c804c18e734bbe622b79b79f86ef33b98；独立热更PID108838，双端hotfix SHA256=90a5b7a4084f08537408fa85aacdc1cc79646073b14177f460b0fa068cd54bcc。服务器列表直连/HTTPS/HTTP通过，本机私有分发备份已核对刷新，归档out/bug-all-1211-final-*。下文1204/1209/1210及1211验收前的待验描述为历史过程，不代表最新线上状态。

1211实际验证了助战无契印资格预选、锁内真实装备重新授权及同步竞技榜一次JSON字段提取，新增初音原表商店→残页→冷登录合成→再冷读及重试拒绝通过。1210发布后10594行匿名日志仍有助战刷新1次数据库超时、无客户端异常/panic/OOM；因此不能用旧窗口或单次EXPLAIN宣布性能问题全清。1211发布后01:34至01:36:50匿名窗口4034行、6次登录完成，数据库超时/客户端异常/panic/OOM均0，报告out/bug-all-1211-log-audit-20261013.txt；只是短样本，持续压力仍未完成。MuMu原进程2766在01:34:10加载1211，最近4500行Python堆栈及原生崩溃0，活动/战斗恢复/模型生命周期/原生记录就绪标记存在；不代替目标玩法操作，私有原日志out/bug-android-1211-followup-private-20261013.txt。

继续核查同桌活动角色选择：原表唯一选项奖励在story/gaokao/story/gk1_1_1b的choose_character_53，A/B分别发209011/209012，原版get_gaokao_character_gender按男角色材料判断，否则女角色。新增ExamOriginalCharacterChoicesAndColdPersistence实际Handle双分支/冷持久/重复与改选拒绝本机通过；test_exam_character_native.py执行原版资格、实际双参closure回调和材料性别函数通过。此专项在1211冻结后新增，仅为本机/原版函数证据，尚无新PG或Android图形剧情转场证据，不据此关闭26行。

收藏室90%进一步确认原版场景各阶段比例0/70/75/90/100，90至100为additional_task；jump_to_room把dormitory_main.init_show交给loading_task，后者等待map_mgr.init_maps及家具/角色加载完成计数。取证out/bug-scene-mgr-native-20261013.txt、out/bug-dormitory-init-native-20261013.txt、out/bug-dormitory-map-native-20261013.txt；尚未取得实际卡住任务与异常，未强行跳过加载完成屏障，不宣称根因已修。

资料维护同步修复：历史反馈表编号不同，update_all_repair_evidence.py改为按反馈正文匹配，撤除按相同行号写入其他问题的错误证据；四份既有中文资料持续整合，不新增长期MD。70条本轮状态明确区分：40条相关实现已上线但反馈场景实机未验，3条部分流程/隔离专项通过但完整场景未确认，2条收藏室已追到附加加载但实际任务/根因待确认，25条仍需独立定位。以上均不是完整关闭计数；高层幸运保底按用户决定另补。

'''
section = section.replace('\n\n', '\n\n' + latest, 1)
for name in ('SERVER.md', 'out/HANDOFF.md', 'out/PROGRESS.md', 'out/REPAIR-TASKS.md'):
    target = root / name
    body = target.read_text(encoding='utf-8')
    body = body.replace('按用户要求跳过，当前无法修复：', '历史诊断（本轮恢复全量核查，不再跳过）：')
    heading = section.split('\n', 1)[0]
    if heading in body:
        start = body.index(heading)
        end = body.find('\n## ', start + len(heading))
        body = body[:start] + body[end + 1:] if end >= 0 else body[:start]
    first = body.find('\n') + 1
    body = body[:first] + '\n' + section + body[first:]
    if name.endswith('REPAIR-TASKS.md'):
        lines = body.splitlines()
        evidence_by_feedback = {row['反馈']: row['当前证据'] for row in matrix['行']}
        generated_evidence = set(updates.values()) | {row['当前证据'] for row in matrix['行']}
        for i, line in enumerate(lines):
            if not line.startswith('| '):
                continue
            cells = line.split('|')
            if len(cells) < 5 or not cells[1].strip().isdigit():
                continue
            # 历史清单编号曾调整；只能按反馈正文匹配，不能按相同行号覆盖别的问题。
            feedback = next((cell.strip() for cell in cells[2:-2]
                             if cell.strip() in evidence_by_feedback), None)
            if feedback is not None:
                cells[-2] = ' ' + evidence_by_feedback[feedback] + ' '
            elif cells[-2].strip() in generated_evidence:
                cells[-2] = ' 历史编号曾与本轮证据错配，已撤除错误证据；该反馈须独立复核，不据此判定修复。 '
            lines[i] = '|'.join(cells)
        body = '\n'.join(lines) + '\n'
    target.write_text(body, encoding='utf-8')
print('全部70条证据及四份现有中文资料已更新；未改在线状态。')
