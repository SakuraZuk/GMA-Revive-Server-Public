# 幻书启世录服务端技术说明

## 2026-10-25 GitHub源码同步与当前阅读入口

纯源码副本的构建、全仓测试及vet通过：顶层400 PASS、106 SKIP、0 FAIL。检查前清除HS_*测试配置，副本只含Git源码文件；未携带数据库、客户端资源和原生运行时。106项外部依赖跳过不计作验收。源码隐私审计覆盖774个文件，公开提交使用noreply身份和独立历史。

本次同步本地最新服务端源码及中文资料，源码热修版本为 **runtime2026101305**（普通战斗桥revision16）。2026-10-25重新执行默认全仓、正式规则全仓和整仓vet均通过。生产发布、真实PostgreSQL与Android记录仍属于各自注明的历史时点，本次没有新的生产部署或实机验收。

最近保存的正式发布报告为2026-10-15的1305：真实PG103 PASS、0 SKIP、5辅助PASS，Linux原生专项与双平台构建通过。游戏SHA256=7d857946a4d7f51d8fac753da1bec465a1bbe99d65bc958b63366d18b5d99064；私有热更SHA256=f453c39821328bc2579f5378acc038b5958b10c035231333f23436e29ff281ec。这些哈希用于辨认历史证据，不代表今天重新核验线上；公开示例配置哈希不同。

普通副本保持Android原生计算；同步/异步PVP采用服务端单原生权威。最新修补包含结算短冲突重试、重复入场/prepare幂等、普通手动原生校验、收藏领奖差量推送、抽卡共享资料、迟到页面回调保护、委托拒绝重同步、失败消息字节类型及开战有界重试。普通胜利好感+1属于本服政策，原服公式待校准。

继续保留后台活动排行长事务锁等待，以及陈列大厅、弱网切换、抽卡后编队、镜像/饕餮结算、试炼荒庭终局和800提示来源等验收门；不能将源码上传写为玩法全部修复。详细接口和历史取证集中在SERVER.md，待修矩阵见REPAIR-TASKS.md。下方旧批次只作历史索引，旧PID、会话编号及“正在发布/待轮询”不可作为当前操作指令。

源码发布只包含服务端源码、示例配置、测试、工具和资料；客户端资源、原生运行时、玩家数据、日志、数据库、密钥和构建产物由本机独立保管。公开仓库接续独立公开历史，禁止推送私有分支或标签。重复的整段发布记录已合并为SERVER.md同名章节引用，未增加长期MD。

## 2026-10-15 当前正式运行版本1305及本批回包契约修补

runtime2026101305已组合发布，游戏PID331114，Linux游戏SHA256=7d857946a4d7f51d8fac753da1bec465a1bbe99d65bc958b63366d18b5d99064；独立热更PID122158，双端配置SHA256=f453c39821328bc2579f5378acc038b5958b10c035231333f23436e29ff281ec。本地三检、双平台构建、Linux原生专项0 SKIP、真实PG103 PASS/0 SKIP/5辅助PASS全部在私有工作区绑定同一源码；组合发布及双核验通过，回滚入口/opt/hs-server/releases/completion-20261015-012631-620686。发布报告out/bug-all-1305-final-completion-release-20261015-012631-620686.json；下文1304及更早记录仅为历史。

本批四项证据驱动修补（均先取现网堆栈再改，不带猜测）：

1. 委托刷新拒绝契约（ED61CEA3.refresh闭包以task_id真值判成败）：现网23:18:56/23:19:05两次`refresh_consign_task_one task=323/316 委托任务不存在`后客户端把失败回包(code,非零id)当成功，init_show触发consign_tasks[id] KeyError、委托页崩溃。服务端失败回包改`(code,0)`走原生show_error_tips，且所有委托拒绝回包先重发当前consign_*属性重同步任务列表（consignSyncPushes）。回归TestRemainingConsignRejectResyncTasksAndZeroCallbackID。
2. 线上字符串类型契约：客户端msgpack以encoding='utf-8'解码，raw字符串一律变unicode，进tips.set_string(std::string)必抛cannot convert。现网rune_mgr.py:258堆栈=embed_rune_by_template失败分支show_tips(msg)。契印预设、异步竞技开战、原生录像上传的失败msg统一改[]byte走msgpack bin类型让客户端得到str（basic_rewards/sync_pvp_rewards既有同款）。
3. 阵容预设保存回包晚于页面销毁：battle_team_preset_list.update_team_list访问已释放team_list控件崩溃。客户端热修revision1在控件为None时跳过纯UI重建，预设数据仍由实体属性同步；原生复现+守卫+幂等安装在test_ui_callback_repair_native.py通过。
4. 战斗好感度（反馈"完成战斗无法获得幻书好感度"）：结算界面按get_card_intimacy与战前快照差值显示+N，服务端此前不发。本服政策=普通战斗胜利后每张出战卡好感+1并压MaxIntimacy上限，经card_common_mgr推送在battle_result前到达；原表win_value=0/win_factor=0.125倍增基数及每日上限无客户端证据，未伪造，待真实数据校准。回归TestBattleWinGrantsFightingCardsIntimacy。

开战入口短锁重试：现网2026-10-14晚battle_fighting多次因55P03直接外抛（匿名样本角色连续四把），客户端无开战推送即"进战斗无反应"。enter_dungeon/battle_fighting按1303基础领奖同规则在同一请求内有界重试3次（retryShortLockConflict），整体回滚后重开；永久错误原样外抛。回归TestBattleFightingRetriesShortLockConflict/PermanentStoreErrorPropagates。发布后短窗仍有1次battle_fighting超时：后台活动排行全服单事务120s超时循环（两天87次）是持锁根因，需dbstore分批改造，未在本批伪造修复。

活动商店不显示兑换物品（密世文字/文字活动反馈）定位结论：客户端activity_dictionary_shop等以data.shop过期窗口过滤；现网145项运营热修已把活动201商店5001/5002及商品窗口重定位为2026-10-07→2037-12-31，原生探针check_shop_unlock=True，初音5010同理。服务端无需改动，待玩家客户端复测；若仍空查其runtime是否最新。送礼反馈现网4次拒绝均为正当规则（无特殊剧情/材料不足）、成功11次，无服务端故障证据。LR800提示在npk全模块与文本表穷尽检索无踪迹，保持未定位，不删不造规则。

## 2026-10-13 当前正式运行版本1304及逆向规则

21:17现网边界：20:58起33035行匿名窗口有get_recommend_leagues数据库超时1次、外部遥测异常4条；服务短窗口不得描述为零报错。MuMu已实际验证主线1-4手动攻击及敌方下一轮推进，尚无弱网和终局证据，当前客户端主线进度不足以开放六类全部入口。

1304已组合发布，游戏PID295321，游戏SHA256=9d9e1a6e0860e33f1626323b60f69876c4526f4dfe2be44ade1e02302eafd5e3；热更PID114795，双端配置SHA256=afe7808c96a5b7a25be19583cff259fa2c285cafe97199ee542d9e55325a40a3。独立核验服务/进程/运行时/环境/服务器列表通过，私有备份已刷新；下文1303及更早记录仅为历史。发布报告out/bug-all-1304-final-completion-release-20261013-205255-502981.json。

结算协议：普通客户端result成功上报后不再上报第二次；服务端必须在55P03/40P01/40001或上下文暂时失败后保留深拷贝pending及已冻结的真实奖励，下一Tick重新开启事务核验收据。永久业务错误不自动循环，不伪造成功。数据库验收注入SQLSTATE并执行整体回滚，覆盖Tick恢复、冷读及唯一收据；不是实际死锁制造证明。

客户端协议：普通战斗桥revision16的手动move_to/use_skill仅在当前战斗已开始且尚未result_sent时走原生本地控制器；非法实体/目标/技能仍拒绝，坐标保持原形状，接受命令仍经log_command上报。竞技权威子类恢复原网络入口；各兼容脚本已同步支持16。prepare仅在同id/参数完全一致时幂等，不同参数保留原生断言；登录恢复revision12仅对同UUID重复入场幂等，异UUID无重建授权仍拒绝。

投影与页面规则：收藏领奖比较前后属性，只发实际变化字段及原回调；登录不提前创建系统房间。抽卡push顺序包含card_common_mgr在card_mgr之前，新模板必须同时有共享资料；召唤页面revision2记录迟到结果但不访问已销毁控件、不将当前页面强切回召唤。详细规则与验证见HANDOFF的1304条目，未新建重复长期MD。

当前候选全仓/正式规则/vet、双平台构建、Linux原生专项0 SKIP通过；真实PG103 PASS、0 SKIP、5辅助PASS。Android已成功登录及领取补给，六类玩法完整图形验收尚未完成；不得以这些证据声称无卡死或800提示已定位。

## 2026-10-13 玩家反馈续修：奖励事务短冲突与天体4410跨级卡死

本轮runtime2026101303已组合发布，2026-10-13 19:06独立核验通过：游戏PID289576，运行二进制SHA256=0efc2941e1d4c5c66a89f2f3d468804135fb6551de5bcd496bbfd9b20a3d180f；独立热更PID114047，双端热更SHA256=14c2a5a3adc1ad27dd58268e7cadcfa5cacc89be23947f3558d501e8ea0d3090。游戏、PostgreSQL、SDK、登录和独立热更服务均为active，后端直连、客户端HTTPS及HTTP服务器列表核验通过，本机私有分发备份已刷新。正式发布报告为out/bug-all-1303-final-completion-release-20261013-190233-366967.json。

基础奖励RPC仅在PostgreSQL返回55P03、40P01或40001时执行最多3次有界重试；每次重试都重新开启事务、读取玩家行并核验唯一收据，其他错误仍立即返回。该规则用于避免短暂锁冲突导致客户端没有回调，同时不放宽发奖条件、不重复发奖。

天体运行论4410好感由140跨至160时，服务端送礼、扣材料、好感升级和冷登录投影原本均正确；卡死根因是客户端跨级或切页先销毁girl_gift页面，成功回调仍访问旧页面并抛异常，导致特殊送礼in_anim锁未释放。1303热更改为仅在页面仍存在时调用set_back_bonus，但无论页面是否销毁都继续执行原成功回调；Python2.7原版闭包回归及页面生命周期回归通过。

新增回归覆盖日常60活跃第三档、体力超过自然Max后领取每日50体力、五类幻星辰墟第六层4106/4206/4306/4406/4506完整入场至结算链，以及残夜目标20102每次正常累计1。私有候选默认/正式规则全仓、整仓vet、Windows/Linux构建、Linux原生专项均通过；真实PostgreSQL为102 PASS、0 SKIP，另有5个辅助PASS。源码、本地测试、真实PostgreSQL、Linux原生和正式发布证据相互独立。

发布后2026-10-13 19:06:05至19:17:45匿名短窗口中，游戏日志18757行：panic/fatal/OOM、数据库超时、异常退出、自动重启、PVP播放确认拒绝和战斗事件序号故障均为0，登录完成16次，PVP加载/开始/结算各1次。2条客户端异常是同一个外部性能遥测域名解析失败，不是收藏室、镜像或试炼荒庭堆栈；报告为out/recent-service-log-audit.json。该短窗口不等于Android/MuMu或玩家实机验收。

仍未关闭：陈列大厅领取资源后卡住及重启加载卡死、弱网时手动失效而自动正常且关闭自动后等待数回合、抽卡后幻书界面卡死及阵容不能沿用、镜像关卡结算失败、试炼荒庭自动操作卡住并影响敌方、客户端“LR800已满”提示来源。腾讯表格状态不得仅凭1303服务端证据改为修复完成，需逐项取得对应玩家或Android/MuMu闭环。

## 2026-10-13 玩家继续反馈：初音地图入场循环前置修复

本轮2026101302已组合发布，2026-10-13 16:36独立核验通过：游戏PID283584，运行二进制SHA256=b827294d11f404498bb8eaa165dafd7e73201407de6195f4b55f62426168ad46；独立热更PID113154，双端配置SHA256=bd2837dbd1fbb028eab2397583d270d554ef3e35e9dae6937ec8d6f98e5857c9。游戏进程、原生运行时、正式规则、双端配置及直连/HTTPS/HTTP服务器列表均核验通过，本机私有分发备份已刷新。最终报告out/bug-all-1302-final-completion-release-20261013-163128-955274.json、out/bug-all-1302-final-native-solo-release-verification.json。上传曾发生一次SSH中断，工具重连续传后组合发布成功。

发布后16:36:43至16:37:36短窗口1186行游戏日志、1次登录完成，未见数据库超时、客户端异常或服务重启；匿名报告out/bug-1302-post-release-logs.json。该短样本仅证明服务窗口状态，不证明初音实机完成或全部玩法无BUG。本轮未进行Android/MuMu验收，助战、学会、其他活动及结算反馈仍需按各自证据继续处理。

1301上线后14:30至16:01窗口123582行、106次登录完成，数据库超时/panic/OOM/重启0；9条客户端异常均为外部HTTP遥测错误，不能将此窗口说成玩法无BUG。匿名业务统计out/bug-1301-followup-business.json仍有初音剧情地图尚未进入6次、好友助战次数不足2次/非指定卡1次、学会查询服务器无效1994次、战斗归属拒绝37次，以及未实现的活动入口。归属拒绝可能是旧战斗迟到请求，尚无证据把每次拒绝都计为结算丢失。

初音原版activity_miku_map_entrance.enter_miku_map先核验地图开放日与章节，若miku_enter_map_dungeon_id未通关则play_dungeon_story，剧情收尾直接enter_dungeon；只有通关后才enter_miku_map。证据out/bug-miku-entry-native-20261013.txt。服务端prepareMikuDungeon先要求地图已存在，与原生顺序形成循环前置；首次地图和后续地图均可能无法进入。现仅原表明确指定的地图入场剧情可先openMikuMap初始化；该函数仍核验开放日及章节，后续mikuStoryGate仍核验剧情/进度/节点。不为其他剧情或未知地图补档，不把入场初始化当通关，既有地图保持原进度。

本机TestMikuMapEntranceStoryBeforeFirstMapRPC和原探索/结算测试通过，覆盖真正enter_dungeon首次入口、普通剧情不初始化地图、第二地图不绕前置。新增TestPostgresMikuEntranceBeforeMapAndColdPersistence覆盖实际数据库Handle/独立Store冷读及重复入口不重置探索；候选1302完成默认/正式规则全仓、vet、双平台构建、Linux原生0 SKIP及真实PG101 PASS/0 SKIP/5辅助PASS，新增首次入场冷持久用例通过。本次验收与发布均绑定1302产物，1301记录仅为历史证据。

助战开战拒绝、学会无效服号及未实现活动仍保留定位门，不能因初音一项修复就宣布群友反馈已清。群友未提供时间，本轮直接审计现网和原生调用顺序，不再要求用户代提供时间。资料整合进四份既有中文UTF-8核心MD。

## 2026-10-13 卡门、关卡进入与竞技恢复共用链修复

本轮2026101301已组合发布，2026-10-13 14:30独立核验通过：游戏PID279184，运行二进制SHA256=8fa8a6ccbd2038b3f57853353904e92500c83c8edf8b28a7039b7a7ae3c7c7d5；独立热更PID112357，双端配置SHA256=186a49c5356c3d002bfbe0f4153a4d554b10739e88cc3804bb7baeaaab804a0b。直连/HTTPS/HTTP服务器列表通过，本机私有分发备份已核验刷新。最终报告out/bug-all-1301-final-completion-release-20261013-142620-007172.json及out/bug-all-1301-final-native-solo-release-verification.json；旧1211顶部记录属于此前正式版本。群友反馈没有具体时间，继续直接依照匿名服务日志、现有反馈清单与原版函数定位，不要求用户代为提供复现时间。

14:03现网审计窗口346664行：11次数据库超时、69条客户端异常标记，无panic/OOM/重启；客户端异常包含外部遥测失败，不能把69条都计为玩法故障。明确的玩法堆栈为原版start_server_battle_ok第36行已有battle断言、real_prepare第376行bid=21重复准备断言，以及sync_pvp_mgr周胜奖励失败分支PRINT_ITEM中文编码异常。匿名业务失败统计见out/bug-common-failures-20261013.json；助战锁超时、初音地图/剧情前置状态、多项未实现接口仍需独立处理，不宣称全部BUG已清。

结算后下一副本进入失败原先只记录日志并保留队列，没有返回该请求回调。现改为一次取走请求和夏日双参标记；真实业务拒绝返回原错误码，存储及内部错误返回原版未知错误13；夏日入口保留(ret,box)。当前已经提交的奖励与battle_result仍正常发送，失败入口不重复建战斗，不留下失效队列。新回归覆盖排队后资格变化、普通单参/夏日双参、原结果提交及重复result不重复回调或发奖。

单端同步/异步及真人竞技恢复保持服务端UUID和持久日志，但客户端必须重新创建对象。恢复加载链新增hs_recover_previous_uuid与hs_recover_rebuild授权；客户端登录恢复包装revision11仅在服务端明确授权时调用原生raw_clear_battle，再完整执行原版创建/prepare。既有普通恢复同UUID幂等保持。原版Python2战斗创建函数、真实RpcMethod单字典/关键字入口、连续同UUID重建已验证；模型清理仍为夹具，不能当Android图形验收。真实原生对局和PG恢复专项新增授权字段断言，同时保留UUID/日志/扣票/唯一结果保护。

周胜奖励失败消息改UTF-8字节，保留原两参回调与中文，避免原生Python2 PRINT_ITEM再次发生ascii编码异常；重复领取拒绝及资产原子回滚回归保留。正式运营规则回归额外发现收藏室第三房间仅检查解锁键存在，值0也被初始化；改为大于0才创建，原测试在正式规则下已通过。此项不能证明收藏室90%附加加载任务已全部解决。

首次全仓在合并热修前发现脚本不一致，拒绝沿用；重新合并后通过。首轮正式规则发现零值房间误判，暂停候选发布，修正后重新默认/正式规则全仓、vet及Windows/Linux构建通过；原生Linux最终专项0 SKIP，最终真实PG100 PASS/0 SKIP/5辅助PASS，产物SHA256=c10c9de2e804c1b0fd74c261ae273aada8d4711bcd47297ad102ba8593bd7e89。旧PG100 PASS/0 SKIP只属于修正房间前产物，禁止充当最终验收。2026-10-13本轮ADB无设备，MuMu管理器确认0/1/2均关闭；不代替群友的实际关卡/结算验收。

防复发门槛：共用入口必须验证失败回调形状、延迟请求/重复结果、事务回滚和冷恢复；竞技恢复必须包含原生对象生命周期与播放世代。默认与正式规则均须通过，真实PG不能以SKIP代替，最终源码/二进制/热修哈希须绑定同一候选。发布后14:30至14:31:37匿名窗口1734行、3次登录完成，数据库超时/客户端异常/panic/OOM均0，报告out/bug-common-1301-post-release-logs.json；仅为短窗口，尚无本轮竞技实战或持续负载证据。发布工具新增validate_regression_checks硬门槛：默认全仓、正式规则全仓、vet报告与日志哈希必须匹配最终构建；Linux原生报告必须源码一致且0 SKIP。21项发布边界故障注入通过，覆盖正式规则失败、旧原生报告、有SKIP和日志被改写时阻断发布。中文详细资料整合进既有四份核心MD，不创建重复长期MD。

## 2026-10-12 全部70条继续施工：剧情塔、赠礼失败与主线自动补完

最新正式线上为1211（2026-10-13 01:34核验），候选012017完成全仓Go、vet、Windows/Linux构建、Linux原生专项、真实PG100 PASS/0 SKIP/5辅助PASS；组合发布报告completion-release-20261013-012944-104110.json。游戏PID257243，运行二进制SHA256=762d949796fba181bfcc2e329ad98c9c804c18e734bbe622b79b79f86ef33b98；独立热更PID108838，双端hotfix SHA256=90a5b7a4084f08537408fa85aacdc1cc79646073b14177f460b0fa068cd54bcc。服务器列表直连/HTTPS/HTTP通过，本机私有分发备份已核对刷新，归档out/bug-all-1211-final-*。下文1204/1209/1210及1211验收前的待验描述为历史过程，不代表最新线上状态。

1211实际验证了助战无契印资格预选、锁内真实装备重新授权及同步竞技榜一次JSON字段提取，新增初音原表商店→残页→冷登录合成→再冷读及重试拒绝通过。1210发布后10594行匿名日志仍有助战刷新1次数据库超时、无客户端异常/panic/OOM；因此不能用旧窗口或单次EXPLAIN宣布性能问题全清。1211发布后01:34至01:36:50匿名窗口4034行、6次登录完成，数据库超时/客户端异常/panic/OOM均0，报告out/bug-all-1211-log-audit-20261013.txt；只是短样本，持续压力仍未完成。MuMu原进程2766在01:34:10加载1211，最近4500行Python堆栈及原生崩溃0，活动/战斗恢复/模型生命周期/原生记录就绪标记存在；不代替目标玩法操作，私有原日志out/bug-android-1211-followup-private-20261013.txt。

继续核查同桌活动角色选择：原表唯一选项奖励在story/gaokao/story/gk1_1_1b的choose_character_53，A/B分别发209011/209012，原版get_gaokao_character_gender按男角色材料判断，否则女角色。新增ExamOriginalCharacterChoicesAndColdPersistence实际Handle双分支/冷持久/重复与改选拒绝本机通过；test_exam_character_native.py执行原版资格、实际双参closure回调和材料性别函数通过。此专项在1211冻结后新增，仅为本机/原版函数证据，尚无新PG或Android图形剧情转场证据，不据此关闭26行。

收藏室90%进一步确认原版场景各阶段比例0/70/75/90/100，90至100为additional_task；jump_to_room把dormitory_main.init_show交给loading_task，后者等待map_mgr.init_maps及家具/角色加载完成计数。取证out/bug-scene-mgr-native-20261013.txt、out/bug-dormitory-init-native-20261013.txt、out/bug-dormitory-map-native-20261013.txt；尚未取得实际卡住任务与异常，未强行跳过加载完成屏障，不宣称根因已修。

资料维护同步修复：历史反馈表编号不同，update_all_repair_evidence.py改为按反馈正文匹配，撤除按相同行号写入其他问题的错误证据；四份既有中文资料持续整合，不新增长期MD。70条本轮状态明确区分：40条相关实现已上线但反馈场景实机未验，3条部分流程/隔离专项通过但完整场景未确认，2条收藏室已追到附加加载但实际任务/根因待确认，25条仍需独立定位。以上均不是完整关闭计数；高层幸运保底按用户决定另补。

全部70条持续修复，不以一批发布代替全清。正式候选20261012-225521包含剧情塔空阵容保护、赠礼失败解锁、沙之书自动补完、原生特殊阵型、副本业务错误码和助战候选精简投影。全仓Go、vet、Windows/Linux构建通过，真实PostgreSQL95 PASS、0 SKIP、5辅助PASS，Linux原生专项通过。2026-10-12 23:03组合发布1202成功，游戏PID245661，游戏SHA256=7df235b561352d94d2d8f6bac8721a620a9e8b3f290d5f43821f1f42519b867d；独立热更PID106851，双端私有hotfix SHA256=f9136d724a94b9200e256d072fda64105d2c898b07bd220ce17b058d54118828。发布记录out/bug-all-1202-final-completion-release-20261012-230306-810349.json。SHA/PID仅是本次核验时点，不保证未来进程不变。

MuMu既有Android12实例2、原客户端1.0.128已真实进入主城，无重装、清数据或修改资源；该单次主城证据不能关闭全部登录白光、收藏室、都市及各关卡验收。服务器列表后端直连、HTTPS、HTTP已通过。正式资源两种入口所有NPK的HEAD长度/Range与校验表本机长度/MD5通过，不等于vivo实际下载或全资源内容验收。

本批战斗材料容量规则由用户明确选择：容量内到账，超出部分保留邮件，释放材料容量后领取。普通副本与活动结算包装withBattleOverflowMail只在战斗奖励上下文暂存超出量，商店、日常及普通邮件领取仍保留原容量拒绝。按单材料limit_count拆附件，中文邮件、奖励冻结、无过期时间。邮件编号与UUID由本场UUID/材料/拆分序号确定，入库前检查正式邮箱与待投递邮箱冲突；500封满时写server_battle_overflow_mailbox，不丢奖或阻塞结算，删除邮件/登录时自动投递。材料到账、邮件/队列、通关进度与已发奖标志同一资产事务提交。结算及显式恢复同步short_mail_info；容量未释放的领取整盒拒绝，不部分写入。

普通副本显式结果先以短事务预演并保存reward_bonus_plan、reward_result和reward_planned_at，再按收据兑现随机材料/幻书/契印。预演不提交资产或进度；兑现回滚、断线和跨日重试保持相同来源/UUID/数量，不重新抽奖；已冻结胜负不能被退出改成失败。冷登录兑现后只同步属性并进入主城，不向不存在的原生battle推结果；显式恢复仍可补发已持久原盒。首通及每日来源沿用冻结计划，避免跨日变化导致来源不匹配。已完成或归属不符的重复结果不入待重试队列，原版可省略finished_task_list的结果包继续支持。

活动包含独立随机事件及状态转换，本批没有将这些规则全部冻结：材料溢出处理已覆盖活动结算，但不能宣称活动所有随机分支冷重试均已固定。幻书和契印库存上限不是本次材料溢出方案的授权对象，继续保留其容量校验，单独核查相关反馈。用户已批准高层基础掉落先行；契印原石本机实现已接入，幸运保底另补，真实数据库和Android对应流程仍须独立验收。

新增专项覆盖拆分、不过期、冷序列化、容量不足领取不变、释放容量领取、重复结果/附件拒绝、满邮箱队列/编号冲突、普通随机契印预演后回滚及跨日冷恢复。第一次全仓Go已通过；后续补充冷登录场景与邮件属性同步后，232539候选runtime1203全部绑定验证通过：全仓Go、vet、Windows/Linux构建、真PG97 PASS/0 SKIP/5辅助PASS、Linux原生0 SKIP；23:33组合发布成功，23:40只读核验游戏PID248226、运行进程与构建SHA一致、独立热更PID107244、双端配置SHA一致及服务器列表直连/HTTPS/HTTP通过。游戏SHA256=06c88cbb6d494d1b459a9ab7bb9e5f5c5c465c94ee42445bd28153449fb7a111，双端hotfix SHA256=553a75371812484dbd490e6f3b42452b6ffc3f529b95f8a3a5176c887f0cd7b4。报告归档out/bug-all-1203-final-*。真实PG新增两项分别走原表6101随机契印真实结果并注入第二事务回滚，以及固定收据夹具覆盖满邮箱/冷登录/领取；后者只验证兑现链，不作为原版副本掉落证据。

225521之后的源码不能引用1202已发布产物作为验证。私有分发备份旧版本被prepare保护检查拦截，已核对现网游戏二进制与双端hotfix精确匹配1202正式发布SHA后刷新本机私有备份，远端未写入。所有后续候选采用新runtime索引，不能用相同1202对应不同正文。

副本入场原版dungeon_mgr.enter_dungeon.server_callback只接收一个ret，再调用本次callback与show_error_tips。现有传输层已对失败入口提供通用13兜底；新增业务层明确错误码1005（体力）、1003（锁定）、1004（关闭）、1019（未知），条件无法解析和存储故障仍保留诊断错误。原版四种错误闭包回调、实际拒绝资产不变、无效协议/损坏规则隔离及数据库故障不伪造业务拒绝专项通过；已有教学入口与门服协议回归更新为对应原版错误码。真PG新增拒绝回包/独立Store冷读无扣费用例待最终绑定，不能把错误提示当不可用功能完整实现。

refresh_assist_use_times原候选查询读取最多1000份完整进度，即使随后只锁10名也会传输/解码无关库存。新增AssistSummaryOnly只读投影，保留公开身份/头像、指定助战UUID、该卡、该卡装备契印与好友/黑名单；随机预选后原UpdateSocial事务仍完整重读并核验指定卡、关系和授权，锁顺序与双向黑名单保护不变。全量角色读取及其他助战业务不启用该投影。新增真PG与夹具投影一致性和原陌生助战原子/冷重试回归待最终绑定，不能声称线上长负载已通过。

21:40后匿名服务窗口68508行，旧1201进程237608仍运行；18客户端异常、17数据库超时、8布阵数量拒绝，无OOM/自动重启。BF29F0ED.map_mgr.generate_slots按formation_data生成槽，EB947BB0.get_battle_cards分出战/援护列表；本版130种特殊战斗有5/6/7出战或4援护阵型，统一4/2错误拒绝。已合并派生容量到既有battle_story_slots.json，按冻结副本battle_id取原生上限，普通/PVP通用4/2和固定剧情角色空槽保护保留。时代从零重启/传说将被改写36700011..36700023全部6人开战、重复请求和冷读取通过；130项原版generate_slots/get_battle_cards一致，超出关卡上限仍拒绝。新增真实PG六人冻结与独立Store冷读用例须绑定新最终构建执行；不能把该规则根因当作六关完整剧情/Android结算已通过。

推荐好友请求在按需补丁之前可能已捕获旧on_get_search_friend绑定回调，修改类方法不会替换已存闭包。新Avatar请求包装保留原生缓存/RPC及回包，只对确切原版推荐回调按本次friend_items对象身份和tab长度保护，列表销毁/换代的迟到请求不更新新界面；有效推荐仍沿原版刷新。缓存越界失败对照、正常刷新、原生异步闭包捕获和换代隔离通过，见test_feedback_callbacks_native.py。客户端资源及原版玩法数据没有修改。

剧情塔2301102/2302102原表是普通enter_dungeon入口，原版battle_cards.check放行全零出战槽。守卫从探索类型2扩至剧情塔23，固定my_avatar_list、storyline_cards及其他玩法仍沿原版，援护不当出战卡。原表两关空锁阵容、回到布阵、重复触发、补齐后继续通过；旧revision1守卫外还有诊断包装，已从诊断器保存原函数取回原生函数并刷新诊断缓存，热升级和幂等安装通过。资料见test_qq_empty_explore.py与test_ui_callback_repair_native.py；图形对象为夹具。

天体4410的choice3原生先置in_anim，material_mgr消费失败只显示错误、不回调成功，因此界面锁持续。按原版资格、特殊次数降级及RPC保留消费逻辑，只有原版特殊送礼闭包对应的当前界面/请求代次在失败时执行原生after_story返回赠礼页；不播放成功对白、不发奖励。预检失败、服务端失败、真实成功奖励、旧请求/旧界面隔离及次数降级通过，见test_qq_gift_failure_native.py。此项保护不能替代具体送礼资格失败原因或Android选择验收。

沙之书自动补完根因由原版return_to_city→check_chapter_finished→sp_card_enhanced_result.show_special_card定位。sites714（第一章）、417（第二章）各有card_auto_enhance=[4401,2]，必须整章非支线/活动地点全部完成；第一章两阶段包含一次技能补完，两章四阶段包含两次。原表card_forbid_list=[5,6]，common_const确认禁止分解和手动补完。新增ensureMainlineAutoEnhance在真实战斗/纯剧情结算及登录事务补齐历史存档最低应有阶段，首个拥有实例与原版展示一致，不耗素材、不降低已得阶段。单节点缺失不发、两个实例隔离、等级/经验/资产不变、两章累计、冷复制幂等、实际714入口状态在battle_result前同步均通过；新真实PG历史登录用例仍须绑定最终构建执行。原生触发/章节/地点/禁止规则取证为out/bug-*-enhance-*-20261012.txt及bug-chapter-finish-native-20261012.txt。

全部70条矩阵仍保留逐项验收缺口，在线状态未回填。主城/收藏室/都市加载、vivo更新、具体赠礼资格失败原因、高层装备、活动所有关卡/动画、长负载及Android实操继续处理。新增技术说明整合现有四份中文UTF-8资料，不创建重复长期文档。

1202发布后23:09至23:27匿名日志采样24360行，3数据库超时、10客户端异常，无OOM/panic/服务重启/战斗序号拒绝；异常包含外部遥测错误和委托完成窗ObjectId类型转换，不能宣称该窗现场已完全修复。原版general_tips_mgr.real_show_tips在分发时才获取gui.consign_finish_new，完成窗类可能在首次回包中加载而早于5秒轮询。新增提示入口转换，入队前把真实拥有卡UUID映射模板ID，空卡只显示已经提交的真实奖励，其他提示类型/参数/排队/演出保留。原版show_tips及real_show_tips执行到未包装完成窗的整链回归通过；完整安装器在完成窗模块尚不存在时也保护入口，重复安装不叠加。工具test_feedback_callbacks_native.py和test_ui_callback_repair_native.py通过；该修补不在1203产物内。

继续发现普通剧情塔的额外任务奖励在原副本奖励包装之后发放，扩大同一结果事务的溢出收集边界，内层只复用同战斗上下文，异战斗嵌套仍拒绝。普通结果预演也包含settleRemainingDungeon；reward_extra_plan_start划分副本与额外任务收据，防止把任务奖励误当副本奖励重复兑现。原表7001三任务实际入口/冻结/完整资产增量专项通过，其他绝密层和探索回归通过。server_battle_overflow_mailbox只持久保存，InitialProperties删除该内部属性，客户端仅收到原生short_mail_info。234411候选runtime1204已通过全仓Go、vet、Windows/Linux构建、真实PG97 PASS/0 SKIP/5辅助PASS及Linux原生专项。23:54组合发布成功，2026-10-13 00:00只读核验游戏PID250393及运行二进制SHA一致，独立热更PID107577，双端hotfix SHA一致，服务器列表直连/HTTPS/HTTP通过。游戏SHA256=689b39a9bf513b4dc946f3cf1843072c75f367a1be3ae2ea7428b5c8a42bd663；hotfix SHA256=66af7a61a5c4738c83bcf55c198cab39a399d50a08dd3d6dc0f56c1d0ca7203b。报告归档out/bug-all-1204-final-*；这些证据仍不代替Android对应玩法验收。

MuMu原进程2766日志23:38确认由1202加载1203；当前账号实际幻书列表及沙之书详情显示模型/属性，未消耗道具、未抽卡或改造资产。仅是这个账号的这两页，不覆盖收藏室、都市、全幻书或抽卡后场景重建。高层装备本的基础掉落表已查出，但额外幸运保底规则缺少原版服务端证据，用户已经明确批准先恢复原表基础掉落、保底另补；本轮按此范围实现，不增加未知保底规则。

活动排行榜进一步消除整份存档传输：山海子榜仅返回当前mountain_dungeon，年兽子榜仅返回当前nian，年兽总榜仅返回冻结nian_total；同时保留已选头像、头像框、自定义头像清除状态及公开同步竞技成绩。原COUNT与页面仍处于同一只读REPEATABLE READ快照，服区过滤、冻结阵容、同分OID顺序和个人全量名次不变。新增1107角色分页/外观/无关资产隔离回归，当前本机专项编译通过但数据库跳过，真实PG须在新1205候选重新验证；不能引用1204产物作为这项通过证据。

2026-10-13的1204线上匿名采样仍有战斗输入异常：原版battle_mgr.on_allow_player_input调用fashion.activate(False)，combat.on_deactivate访问BattleEntity.battle缺失。源码新增当前所属绑定保护，仅当gworld.get_battle与本管理对象一致、该实体和entity_infos已登记且所属为空时补self，随后完整执行原生停用/激活、鼠标取消及自动切换检查。既有非空所属不覆盖，旧战斗不接入新对象，其他异常不吞。真实Python2原函数整链复现/修补、自动切换检查、旧战斗隔离、按需加载安装及幂等通过。原生assemble闭包提前捕获组件函数，因此包装local/shadow/record/watch实际战斗类最终入口，完整安装/原生组合闭包回归通过；1205排行与1206早期输入候选各真PG97 PASS/0 SKIP但未发布，最终1207候选Linux专项因SFTP断线残留写句柄报Text file busy失败，禁止发布；新候选以新inode激活且复核哈希后重新验收；只能定位具体堆栈，不能据此宣布清单44已实机修复。

2026-10-13续修：1210候选010441完成全仓Go、vet、Windows/Linux构建、Linux原生、真PG99 PASS/0 SKIP/5辅助PASS并于01:16组合发布，已核验游戏PID255491、SHA256=a2aaa77e84524ade448368ec0d0dd934dd5f7581a160b09b6bc18f4c56eb12dc、独立热更PID108512、双端hotfix SHA256=4df7e4f87c162dae1aaee24198059b68713c47e98dbc6b9ccbd74d74a3848941、服务器列表三入口通过，归档out/bug-all-1210-final-*。1208候选全仓一致性检查发现服务器内嵌runtime索引仍1207而配置1208，禁止发布；已统一为1209并生成独立005222候选，全仓Go、vet、Windows/Linux构建及Linux原生专项通过，真PG99 PASS/0 SKIP/5辅助PASS，1209已组合发布且01:03只读核验通过，游戏PID253857、SHA256=e91db9ab84c5c820c409fa98ae3e0893c9302825e8e688f05e8397a569004318，独立热更PID108176、双端hotfix SHA256=d554a3928fcd5c5608ebd814c50636c588378d14ef71157dfb96a044c13b4d9b，服务器列表直连/HTTPS/HTTP通过，归档out/bug-all-1209-final-*。补查原石普通RPC回调键类型需显式ObjectId，主源1210已修正，不能把1209实库通过当1210通过。用户已选择高层装备本先恢复原表基础掉落，幸运保底另补。奖励解释器只在装备本当前每日基础来源临时允许Lucky字段，不扩展商店或自选盒资格；完整发固定/随机契印、材料与原石，额外幸运分支尚未启用。原石Type15存current_rune_stone专用状态，不作为普通材料累计。按原表每日1颗、有效2小时；未过期不覆盖，同日过期不绕每日上限，北京时间日界重置次数。旧日冻结兑现不得回拨新日次数或阻塞其他奖励。

原石RPC step_0_rune_stone(callback,materialID)、step_1_rune_stone(callback,UUID列表)、step_2_rune_stone(callback,UUID列表)，均回三参(success,msg,rune_info)。开启最多100次，优先23×1、否则11×10，原石目标盒抽10件隐藏契印；打磨累计最多3件、原表免费；鉴定按原表20项星级/部位金币12价格，校验真实背包容量与UUID冲突，同事务扣费发原件。开启只揭示star/suit，打磨加pos/extra_suit，鉴定才发完整属性。隐藏runes与server_day不下发；普通RPC回调rune_info键显式ObjectId，以真实gate编码Ext42核验，不能只用属性容器自动转换代替；原版RuneStone.load真实ObjectId双字典及阶段校验已通过。相同参数重开属于合法新抽奖，完成阶段重传不重复扣费，已分解鉴定件不能再领。仅明确业务拒绝转换失败回调，存储/目录故障保留诊断错误。本机原石全流程、装备全层七日基础生成、原石冻结跨日兑现回归通过；新实库用例HighEquipmentBaseRuneStoneStagesAndColdRecovery已加入1209候选，1209整套真PG99 PASS/0 SKIP/5辅助PASS，但补查发现普通RPC回调没有自动转换UUID键；1210已显式ObjectId键并有业务/实际gate编码断言，须重新绑定候选验收，不以本机SKIP计通过。

初音获取已确认原版来源：商品5010009需累计50000白昼和声，单价3000和声换1残页，永久限购50，配方3411需50残页并确定生成3411角色。MikuOriginalShopFragmentsComposeAndColdPersistence经真实Service.Handle覆盖累计资格不足不扣料、150000和声兑换50残页、限购重试、原件与获得时间、冷存档和重复合成拒绝，本机通过；新增测试未进入已冻结1210，真实PG整链和Android入口仍待验。不凭本测试给线上角色补发资产。

继续线上匿名审计1209窗口11314行仍有助战刷新5次/排行3次超时，不能宣称消除。排行失败目录明确为4/0同步竞技榜。只读EXPLAIN：原助战候选投影5秒超时；去掉预选契印并jsonb_to_record一次提取后230行872.364毫秒；同步竞技原查询1233.741毫秒，一次提取完整公开外观后690.126毫秒。这是当前只读查询样本，不是长期压力结论。主源新增AssistCandidates(ctx,host,limit)只返回指定卡/身份/双向关系，维持同服OID排序最多1000窗口及原随机十人算法，最终候选仍由UpdateAssistRefresh锁内读取完整指定卡契印并重新授权。非入选候选不解码契印；预选结果不写回任何提供者存档。同步竞技榜改一次jsonb_to_record提取score及外观，正积分/同服/同分OID/limit不变。新增真实PG预选资格字段/无契印隔离断言，原持久授权和回滚用例保留；新代码本机编译通过但PG跳过，待1211重新绑定完整验收。

助战refresh_assist_use_times新增锁内精简投影：self仍完整存档；提供者仅助战卡、绑定契印、公开身份、好友/黑名单。保持原依赖咨询锁、全部avatars排序锁和同顺序progress锁，不用预选旧快照替代锁内授权；提供者进度被回调修改立即拒绝并回滚self，只返回self完整Avatar，禁止把投影替换在线完整存档。其他选择/消耗/开战仍走完整事务，夹具可回退原接口。新增真实PG投影一致/原存档未知大整数/修订不变/整事务回滚用例，本机数据库SKIP，等待新候选真PG验收；不宣布线上超时已消失。


## 2026-10-12 全部70条重新施工：1202候选，尚未全部完成

用户明确要求全部修复，本轮覆盖反馈快照中的70条非空反馈，包括旧21条已修复与4条无法修复。历史跳过记录不作为本轮授权；清单、入口与逐项验收条件整合到REPAIR-TASKS及 `out/bug-all-repair-matrix-20261012.json`。不能再把1201两项上线当成全部交付。

本轮新增源码修补：入馆指南普通/活动任务的旧行销毁后 `_ins=None` 时跳过旧行update，原版领奖闭包仍刷新当前任务列表；委托完整RPC还会打开 `consign_finish_new`，该窗按模板编号创建模型，新UUID回包在此产生类型转换异常，现于完成窗入口将UUID映射回真实卡牌模板，旧模板编号保留，空演出仍展示已提交奖励。主页面公共信息查询兼容原生Int键转换的TypeError/ValueError，并精确升级1201包装；详情页保留UUID语义。体力补给原版回调会索引未加载大厅的swallow对象，现保留原生buff/bonus预检与RPC，独立检查有效界面后更新红点，真实奖励始终展示，失败不展示。以上不修改资产、次数或奖励数值。

`out/tools/test_feedback_callbacks_native.py`执行原版委托完整RPC，经详情、主页面、地图、收藏室到完成窗，并保留原生类型转换失败对照；普通/活动任务原版领奖闭包销毁失败对照与活行回退通过；补给原版预检/回调占位异常及真实奖励回归通过。既有UI/入馆场景专项与本轮Go专项通过，图形控件为夹具，不能当Android实操。runtime/内置索引候选升1202，正式候选与发布仍须完成绑定验证；1201现网尚未切换。

最新21:06后日志确认新的委托完成窗类型异常、好友推荐旧行IndexError、材料容量导致结果拒绝、学会/排行榜/助战超时。新增匿名报告 `out/bug-all-followup-anonymous-20261012.json` 只保留错误栈与固定业务摘要，排除含昵称/库存的RPC参数行；日志审计同样排除该类正文。旧过程证据不当零异常证明。

时空回廊两条反馈由原表定位为剧情塔2301102/2302102，原版panel_detail直接走普通enter_dungeon；绝密记录task_tower的extra丢弃不能解释它们。天体送礼模板4410，对应选择441023/441053的choice_level=3；最新窗口无该项拒绝，仍需核查实际原生选择和资格。材料check_limit_count原版函数可封顶，但完整脚本引用扫描没有发现其业务调用者，暂不据单函数猜改发奖或删容量保护。全部玩家验收未完成，70条均保持逐项施工状态。


## 2026-10-12 历史1201批次：组合发布与独立核验完成

最终runtime2026101201已组合发布，游戏PID237608、SHA`d5b578f663388a77c043a9ec7bb68ceebee3a4620761b3ab86bef05bfb1ff50f`；独立热更PID106055，双端私有热更SHA`d985c65b4673000b4a5bd8eb6fe95b1c02bed595bbdf231ab4b10d517e41f8fd`。运行进程、原生运行时、正式规则与候选哈希一致，后端/客户端HTTPS/HTTP服务器列表均通过；本机私有分发备份已同步1201，旧备份在私有候选保存。双端回滚入口为`/opt/hs-server/releases/completion-20261012-210009-228361`和`/opt/hs-hotfix/releases/completion-20261012-210009-228361`；内存覆盖文件独立保留，不属于组合回滚清单，回滚时须同时评估该文件。

最终源码包括：尖叫屋完整位置回包且仅选中项入账；委托UUID回包与主页面UUID转模板编号兼容；周期预读只保留不可变JSON快照，写阶段按revision/xmin重新读取/解码/计算，移除第二份全服解码对象；Go1536MiB软预算和GOGC50；日志窗口覆盖旧进程及systemd异常退出，排除GC诊断数值的HTTP误计。全仓Go、vet、Windows/Linux构建、真实Python2双页面回调及UI专项、Linux原生引擎/单端/双端均通过；最终绑定真实PG92 PASS、0 SKIP、5辅助PASS。`out/bug-final-*.json`与日志是本批最终证据，旧失败与第一次发布证据保留为过程记录，不代替最终验收。

必须保留线上限制：21:03起的完整窗口4126行，OOM/自动重启/战斗序号故障0，但数据库超时20条、客户端异常6条，仍包含main_consign_new原路径ObjectId KeyError；不能证明这些客户端已经运行1201补丁，不能据此判定完全解决或简单忽略。21:06后补充窗口1411行，委托主页面/尖叫屋异常及DB超时0，客户端异常2，OOM/自动重启0；进程237608保持，服务内存约2.01GiB。该窗口没有目标玩法完成次数证明，软预算不是RSS硬上限，长期压力和所有客户端稳定性仍待验收。证据见`out/bug-final-post-log-20261012.json`、`out/bug-final-followup-20261012.json`、`out/bug-final-process-memory-20261012.json`。

当前可交付结论是两项回包契约修补、周期重复对象削减与内存预算已经实现并上线，候选验证和运行产物核验完成；反馈清单全部70条已留档并索引，其余问题继续保留。Android尖叫屋/委托双页面实操、1201实际执行与持续高并发验收未完成，在线表未回填、不标全清。新增技术资料整合进既有四份中文UTF-8核心MD，没有新增重复长期MD。未改`com.netease.hsqsl`资源，未重装或清空客户端。

## 2026-10-12 委托主页面第二条原生路径与最终候选1201

扩大服务日志窗口后，UUID回包在详情页已符合契约，但原版主页面`main_consign_new.get_vo_bubble_by_card_id`实际索引`card_common_mgr`模板编号字典，出现新的ObjectId KeyError。第一版只验证详情页，覆盖不足，故此前“委托两处修补已完整”的结论撤回，未当作最终玩家验收。主页面汇编见`out/consign-main-native-20261012.txt`；匿名现场见`out/bug-full-window-anomalies-20261012.json`。

`internal/game/ui_callback_repair_script.py`新增主页面查询包装：旧模板编号保留；UUID先由实际`card_mgr`映射到`card_id`，再调用原生公共信息及语音逻辑；已移除幻书的迟到演出返回无语音，不重复发奖。详情页继续按UUID使用原版函数。按需加载与修补版本标记保留，重复安装不叠加包装。`out/tools/test_feedback_callbacks_native.py`扩充真实主页面`get_vo_bubble_by_card_id/play_vo/commit_task_succ`，先复现UUID KeyError，再验证UUID、旧模板编号、空列表和已移除幻书均完成；详情页和尖叫屋原版回调仍通过。完整证据见`out/bug-20261012-native-complete.log`，既有UI回归也通过，见`out/bug-ui-native-complete-20261012.log`。图形对象仍为夹具。

热更升到runtime2026101201，公开与私有候选分别生成，现网私有配置在生成前核对1103。最终候选入口已更新`out/private-release-workspace.json`。该候选同时包含尖叫屋、委托双页面、周期预读减少副本与服务日志窗口修正；不能用前一版产物验收替代最终构建。减少副本的第一轮真实PG91 PASS、0 SKIP、5辅助PASS，但整套180秒总期限耗尽，最后冷登录用例未完成，因此记录为失败。失败证据保留`out/bug-memory-first-remote-database-verification.json`与`out/bug-memory-first-database-test.log`；整套预算改360秒，原有单用例时限与并发/回滚断言不变，最终候选重新执行全套，不把超时直接改写为通过。服务器内置bridgeHotfixIndex也同步1201；完整冷启动/运行热更一致性回归、全仓Go、vet及Windows/Linux正式构建已通过，真实库与Linux原生专项正在执行。

完整服务窗口还包含入馆指南销毁控件回调、PVP重入断言、战斗序号拒绝及数据库超时，均继续保留；首个PID过滤报告不能替代这些现场。此次只修证据明确的契约和周期对象重复；其他反馈不宣布关闭。GC诊断中的三位数会匹配旧通用5xx正则，计数不能视为HTTP业务故障数量。

最终1201私有候选已完成全仓Go、vet、Windows/Linux构建、原生双页面回调、UI专项、Linux引擎与单端/双端专项，真实PG92 PASS、0 SKIP、5辅助PASS；此前超时用例在本轮完整通过。候选游戏SHA`d5b578f663388a77c043a9ec7bb68ceebee3a4620761b3ab86bef05bfb1ff50f`，数据库SHA`3a69de8d33c869d761fc8462d50fb252ed2ed8c7f252ec534cf5c6cca27f2517`，私有热更SHA`d985c65b4673000b4a5bd8eb6fe95b1c02bed595bbdf231ab4b10d517e41f8fd`。证据见`out/bug-final-build-20261012.json`、`out/bug-final-database-20261012.json`、`out/bug-final-native-linux-20261012.json`及对应日志。此时生产进程235284、NRestarts0；最终组合发布预检进行中，Android画面与长期高负载仍未通过。

## 2026-10-12 发布后发现OOM：周期预读副本与审计窗口修正

本节覆盖下文“短窗口服务正常”的最终结论。第一次发布后进程234335在20:34被内核OOM终止，234896再次因内存不足退出；旧版本231906也有同类事件。内核显示游戏匿名RSS约3.6GB，主机无swap。证据见`out/bug-post-release-restart-20261012.json`。这说明此前20:03的SIGKILL不能继续只记录为未知信号；内核已确认同窗口存在OOM。但不能声称全部登录、白屏或战斗反馈都由同一原因造成。

源码核查确认周期预读在`internal/game/dbstore/social_all.go`同时保留全服解码Progress、比较用JSON和第二份解码Progress，造成明确的重复存活对象。修补将`periodicProgressSnapshot`收敛为不可变JSON、revision与xmin，预读不再二次解码全服进度。写阶段重新解码已缓存JSON；版本变化仍按Serializable快照重读当前数据库并重新计算，未共用预览修改后的对象，不绕过回滚和跨角色依赖锁。原有`TestPostgresPeriodicPreviewDoesNotBlockAndRecomputes`继续作为并发/重算验收；普通全服管理事务保持原单次回调语义。

已安装`deploy/hs-game-memory.conf`对应的systemd覆盖文件`30-memory-budget.conf`，Go软预算1536MiB、GOGC50；它避免无预算堆增长挤占同机服务，但不是系统硬上限，也不能作为泄漏根因已经全部修复的证明。首次诊断临时加gctrace，进程235284经过274秒仍NRestarts0，早期回收后活跃堆曾约1.3GB，后续回收后约97MB，采样服务内存约1.23GB；证据见`out/bug-memory-budget-20261012.json`与`out/bug-memory-budget-sampling-20261012.json`。该采样发生在减少副本源码发布之前，只证明软预算短窗口缓解；长期高负载仍待验收。临时gctrace在最终发布前撤除。

审计工具`out/tools/audit_recent_service_logs.py`旧查询带当前_PID过滤，服务重启后会漏掉旧进程和systemd的OOM记录，甚至在新进程0行日志时写“未发现异常”。已改成整个服务时间窗口，并把服务异常退出、自动重启纳入异常状态；Python编译与旧PID遗漏回归通过，证据见`out/bug-audit-restart-regression-20261012.log`。先前只按新PID得到的零异常报告保留为历史取证，不再作为稳定性验收。

减少副本候选全仓Go、vet及Windows/Linux构建已再次通过；真实PostgreSQL和Linux原生专项正在重跑，正式切换与后续内存采样须以本节后续绑定报告为准。本次没有新增客户端接口、修改发奖数值或改变热更资源。清单两处回包修补与该内存修补一并保留；Android实机及其余反馈继续独立验收。

## 2026-10-12 反馈清单复核：尖叫屋与委托回包修补已发布

用户反馈来源：https://docs.qq.com/sheet/DSnJ2V1VFaWxhQVVO?tab=BB08J2 。本次逐格只读核对第5至78行，共70条非空文字反馈：在线标记已修复21条、修复中8条、待修复37条、无法修复4条；全部测试列仍为待测试。空白行36/39/40/51不作为BUG；79至95反馈为空。表中行号已变化，后续必须同时核对描述，不能沿用旧行号。在线状态只是反馈表原标签，不代表本轮验收或Android通过；本轮未修改在线表。完整B/D/E快照见`out/bug-feedback-snapshot-20261012.json`。

本轮确认两处真实客户端异常，匿名日志见`out/bug-feedback-error-audit-20261012.json`：委托完成发模板编号2202，原生语音查询按卡牌UUID索引，产生KeyError；尖叫屋发单元素材料数组，原生揭晓动画按三个幽灵的位置索引，产生IndexError。两处均由原版Python2函数对照复现，不能把外部HTTP监控上报超时视作同一个卡死根因。

- 当前第72行“委托无法收回”：`internal/game/consign_rpc.go`在完成事务内把任务模板编号按顺序映射到实际拥有卡牌的UUID，回包`commit_consign_task_success(task_id, card_uuids, box)`的第二参数为ObjectID数组。同模板有多个实例时选择第一个已有实例；旧任务幻书已归还时省略该演出项，奖励与阶段收据仍正常提交。任务存档保持原模板编号，不迁移历史任务、不改变奖励。原生模块C83CDF61的`commit_task_succ`与`get_vo_bubble_by_card_id`证据见`out/consign-native-20261012.txt`。
- 当前第57行“尖叫屋选择后重播动画、无法退出”：`internal/game/house_frage_business.go`的`open_game_site(callback_id, selected_index)`成功回调改为`[true, selected_index, 完整三个材料位置, count]`。揭晓材料由已冻结`site.Game`复制；实际入账仅`site.Game[selected_index]`，未选位置只展示，Opened收据与Double消费仍同事务保存。原生模块F544E824的`on_choice_ghost`证据见`out/ghost-native-20261012.txt`。

验证证据：旧尖叫屋回包专项失败见`out/bug-20261012-before.log`，旧委托失败见`out/bug-20261012-consign-before.log`；尖叫屋修后专项见`out/bug-20261012-ghost-final.log`。`out/tools/test_feedback_callbacks_native.py`执行原包实际回调，三个选择位置均揭晓完整数组且只选择项领奖，委托UUID与已归还空列表均完成演出，旧回包均复现异常，日志见`out/bug-20261012-native.log`。图形对象为夹具，不代表Android实机画面。Go专项覆盖尖叫屋三个位置、双倍次数、冷登录重复选择不改变存档，以及100次委托完成的UUID/成就/收据；真实PG新增`TestPostgresHouseGhostFullReplyColdDuplicate`并扩充委托生命周期断言，验证结果须以本批绑定报告为准。

全仓基线通过，但本地无真库的PG跳过不能写通过。正式候选目录由`prepare_private_release.py`核对现网私有配置生成，入口见`out/private-release-workspace.json`；仅使用私有候选进行正式构建与实库验收。20:03现场游戏进程曾变化，本轮未触发该重启；本轮只读核查游戏SHA仍为昨日1103的`bf0bd9df5e1a9b2b0891372c126a6793efcbb4d0957caf97fc5da29cbdd32ca2`，服务active/running、PID232891、NRestarts6。发布前必须重新核验，不能用历史PID推断他人正在部署。

尚未定位的反馈继续保留。双倍退出已有原生体力返还证据，尚无原生双倍券返还证据，不能凭反馈直接增加退券；战斗结算出现材料容量上限拒绝，不能删除容量保护；送礼现有日志仅证明部分角色提交成功，不证明第48/63行已解决；日常60活跃度与时空回廊两个指定事件仍需实际触发证据。匿名日志另有avatar回调swallow对象不可索引异常，尚未匹配具体入口。原有第23/26/32/33行记录为历史用户要求跳过，不把第三方表格备注视为本次新授权或新指令。

本批私有候选源码验证已完成：全仓Go与vet退出码0，Windows/Linux游戏及热更、Linux数据库测试产物构建通过，源码构建期间哈希一致。真实PostgreSQL92 PASS、0 SKIP、5辅助PASS；新增尖叫屋测试和委托UUID断言均在真实库执行。游戏候选SHA`692f16b89117e3947e530aaed70caefab437b29601e31871054839019886a947`，数据库产物SHA`095da2f7b7542af29d48513b76ede34ae1417f5ea71492e9edce4d9bcc2de41c`。与已发布1103构建清单对比，差异仅两份业务文件及四份回归测试；私有热更和正式规则哈希均保持一致。证据已复制为`out/bug-build-verification-20261012.json`、`out/bug-all-test-20261012.log`、`out/bug-vet-test-20261012.log`、`out/bug-database-verification-20261012.json`及`out/bug-database-test-20261012.log`。Linux原生引擎、单端/双端房间及活动原生专项也已通过，0 SKIP，测试前后游戏PID均232891，源码哈希一致；证据见`out/bug-native-linux-verification-20261012.json`与`out/bug-native-linux-20261012.log`。正式切换已完成，运行产物独立核验见下文；Android画面仍未验收。20:03历史服务重启记录为SIGKILL及自动重启，并非已证明Go panic或OOM；触发来源尚未确定，证据见`out/bug-service-restart-summary-20261012.json`。

正式切换已完成：20:30创建回滚备份`/opt/hs-server/bin/backup-20261012-203045`，随后仅发布游戏二进制。独立复核运行进程PID234335，active/running、ExecMainStatus0、NRestarts0，`/proc/234335/exe`的SHA与本批游戏候选一致。私有热更SHA仍为`4cc16352e6907dde9fcaa091547ef7f3d9afb70ebe759f4d20f5f92fa82e2e59`，独立热更PID99277；未发布热更、未更换资源。发布与运行证据见`out/bug-game-release-20261012.json`、`out/bug-game-process-verification-20261012.json`。首个极短窗口85行游戏日志，panic/fatal/OOM/5xx、数据库超时、客户端异常均0，登录及目标玩法次数也为0，不能作为玩家场景通过。匿名报告见`out/bug-post-release-log-audit-20261012.json`。当前第57/72行已完成源码、原生函数对照、真实库及生产切换，Android实际尖叫屋/委托画面仍待复测；清单其余问题未自动关闭。

## 2026-10-11 实时PVP开战后场地事件打断播放链修复已发布（runtime2026101103）

1102已经修复实际加载屏障，但不能作为整局完成：19:46至19:50三次真实玩家尝试均先出现“该原生命令没有播放确认规则”，随后持续“战斗事件序号跳号”及“上一权威动作尚未播放完成”，0结算。此前审计只覆盖加载异常和推进失败，漏掉do_command的播放确认及序号故障；已新增独立计数与匿名摘要。断线context canceled是随后连接退出的结果，不能单独解释玩家为什么断线或作为本轮业务通过依据。

代码对照和失败回归确认一条确切缺口：MapPlayback按原生kind=field/native_type=4跳过场地自主动作，但旧桥log_command仍把场地magic_field_idle发成command；Checkpoint只接受真正被投递的FIFO命令，因该场地命令无规则而回滚，客户端单向事件序号继续递增，于是永久跳号。新增Go回归先在旧源码失败，修补后场地事件不消费Seen/Boundary、下一权威技能及稍后输入仍可解除屏障；非场地、未映射实体、超出原生数量/缺失参数列表及未知命令仍不得绕过确认。

服务端内部nativeLocalFieldIdle只接受已有原生权威证明并映射的场地实体，以及magic_field_idle原生占位参数（或旧桥空参数）；它仅保留连续观察序号，不执行技能、不推进权威日志、不修改胜方或奖励。客户端原生场地执行保留，场地log_command仅上报snapshot，普通卡仍上报command。竞技专用工厂安装修订从2升到3，以覆盖已安装旧工厂；桥协议31及普通副本rev15保持兼容，Python2工厂、升级、幂等、场地snapshot后普通command连续序号回归通过。未知播放拒绝新增固定事件/命令名/错误类型日志，不记录玩家标识或正文；不通过宽松接受跳号或任意未知命令来伪造恢复。 原版android_base/8364763B.marshal的command_magic_field_idle(self,user,action_info)只转调command_pass_action(user)，不读取action_info；服务端不执行客户端占位参数，只保留观察连续性。

最终候选在仓库外20261011-200220私有目录完成：全仓Go、vet、Windows/Linux正式构建、Linux原生单端/双端和新增场地播放回归均通过，真实PostgreSQL91 PASS、0 SKIP、5辅助PASS。游戏SHA为bf0bd9df5e1a9b2b0891372c126a6793efcbb4d0957caf97fc5da29cbdd32ca2，数据库产物SHA为c2474d020040e8b64e04f05140b18a5dc10082177afc97c5f8754d411a3d7625，双端私有热更候选SHA为4cc16352e6907dde9fcaa091547ef7f3d9afb70ebe759f4d20f5f92fa82e2e59。双主机预检通过，20:15组合发布完成：游戏PID199573，独立热更PID99277；运行进程SHA与本节游戏候选一致，双端热更SHA与本节私有候选一致。双端备份为/opt/hs-server/releases/completion-20261011-201051-315407和/opt/hs-hotfix/releases/completion-20261011-201051-315407。服务器列表后端/HTTPS/HTTP三链通过，私有正式分发备份已同步1103；发布只能使用新私有候选及其绑定验收，不得直接使用公开热更配置。完整玩家整局仍未验收，本机MuMu1.0.128等待手动登录且窗口工具捕获超时；不清空数据或替换热更新资源。

发布后首窗口2190行日志：登录5/5完成，播放确认拒绝、战斗序号故障、客户端异常、数据库超时、panic/fatal/OOM/5xx均0，但新PVP对局数0，不是完整一局通过。20:16在线56角色，当前同步PVP会话0、加载握手齐备未开战0；这些是聚合现状，不代表全部玩家永远无故障。证据已同步out/completion-release-20261011-201051-315407.json、out/remote-release-verification.json、out/server-list-recovery-verification.json及新增审计计数。 后续20:17窗口4693行，登录9/9，PVP播放拒绝/序号故障/客户端异常仍0、对局0；20:16:13出现一次refresh_assist_use_times助战刷新5秒上下文超时，独立保留，不能声称全站零异常或归因于已被修补的PVP场地播放。

## 2026-10-11 实时PVP实际加载屏障重修已发布（runtime2026101102）

本次19:09匿名现场仍有PVP客户端崩溃，1101的缺面板补丁没有完成修复。完整调用栈是`notify_battle_fighting → battle_pvp.on_player_ready → battle_layout.set_ready → switch_editor_mode → set_support_mode`，先缺`support_skills_panel`，补空面板后继续缺`authority_shadow.main_slots_area`。原版Python2的`battle_mgr.handle_battle_entities_create`在同步模式2明确跳过布阵初始化；因此正式shadow场景不能再进入旧布阵界面。此前“就绪通知才能开放开战入口”的解释已经撤回，不得继续补假控件或猜测区域。

源码统一采用选卡阶段已经冻结的阵容，机器人和真人均由实际`load_entity_finish`及桥`ready`推进；机器人直接执行现有开战事务，真人在双角色共同房间事务内标记实际Loaded/Ready，双方就绪后沿原有持久投递开战。真人不再等待原包没有发送的`pvp_load_complete`；旧接口保留兼容。桥先到、实体先到、旧在线会话Tick、同UUID重复通知、冷恢复及原生权威世代沿用已有校验，不改变积分、奖励和结算规则。第一端等待对手时不再提前把连接标为已开始。

客户端1101空面板包装已经撤销，热升级精确恢复原始函数；只拦截正式PVP场景迟到的旧布阵就绪通知，真正布阵场景保留原生行为。`test_ui_callback_repair_native.py`执行本版原生RPC本体、PVP就绪和set_ready入口，先复现异常，再验证正式场景隔离、1101热升级、重复安装及真正布阵回退；图形控件仍为夹具，不冒充Android画面。
本批正式候选在仓库外私有工作目录生成并构建。全仓Go、vet、Python2 UI及权威工厂/对手恢复回归通过；Linux原生引擎、真人共同开战/超时/恢复/唯一结算、同步机器人/异步真人防守/异步机器人专项通过，0 SKIP；绑定当前候选产物、私有热更及正式规则的真实PostgreSQL为91 PASS、0 SKIP、5辅助PASS。跨进程真实PG共同房间用实际load_entity_finish和ready自动开战，不再用手工battle_fighting掩盖加载缺口。

19:29游戏与独立热更组合发布完成，并已再次独立核验。游戏PID`196526`、SHA`5eec0cf7959039cbd6843e20ecf439936553173ab619f78227965f6c6dccc239`；热更PID`98751`，双端私有热更SHA`f4d1beddc3481800096e3f3935e8cfde4544cb454c3bf650324ded53a49f5744`；数据库产物SHA`625256d58bc8514a7b0d17ce3f3da48ad17f97c33998eaa2acad1f132fc496aa`。进程、服务监听、原生运行时和环境一致，后端/客户端HTTPS/HTTP三条列表路由通过；双端回滚备份为`/opt/hs-server/releases/completion-20261011-192829-088143`和`/opt/hs-hotfix/releases/completion-20261011-192829-088143`。私有配置备份已同步1102，公开工作树不能直接发布。
发布后19:42匿名窗口：21500行游戏日志，登录结果/完成23/23；同步PVP实际加载屏障1次、客户端started核验1次、末态结算0次。客户端异常、旧面板崩溃、阵容拒绝、数据库超时、panic/fatal/OOM/5xx均0；仍不能据此宣称完整对局通过。19:32:27一条推进取消紧邻TCP读取EOF；传输层的stopClock在连接退出时取消当前Tick上下文，事务失败保留已提交Journal，不能用独立后台上下文强行继续给已断开的连接推进。补充真实原生回归验证取消后Journal不变、恢复原输入窗口及后续超时重试/冷重建通过，Windows和Linux均0 SKIP，Linux前后游戏PID一致。审计工具新增推进重试/上下文取消/非取消失败及匿名摘要，避免再次漏计取消日志；当前非取消推进失败0。

证据入口：`out/remote-release-verification.json`、`out/native-solo-release-verification.json`、`out/recent-service-log-audit.json`、`out/live-pvp-state-audit.json`、`out/native-journal-cancel-linux-verification.json`。正式候选91项真实PG及完整对局专项为已发布源码的验证；补充取消回归仅增加测试与审计资料，没有再次更改生产二进制或热更。

本机已有MuMu Android 12实例2已启动，实际ADB端口为16448，客户端1.0.128进程和日志可读；启动及列表阶段0 Traceback/AttributeError、0服务器列表失败。客户端日志明确等待手动选服/登录，尚无PVP整局。窗口工具首次FrameArrived timed out，重新选窗口后window capture timed out，未使用旧坐标继续操作；已请求手动验证。未重装、清空数据或修改任何热更新资源，详见out/mumu-pvp-startup-audit.json。


接口与诊断补充：`advancePvpLoadBarrier(ctx,c)`仅内部使用，不新增客户端RPC；判定当前UUID归属、已鉴权、实际加载和桥握手，不解析第二份客户端阵容。机器人输出原有add_fighting_cards/battle_fighting/start/check_on_battle_start；真人通过HumanDelivery持久投递。新增匿名日志区分服务端允许开战、客户端started快照实际核验、权威末态结算，避免把下发通知当作玩家开战。`audit_recent_service_logs.py`的总状态不再由热更正文脱敏探针决定，通过与否分别记录；`audit_live_pvp_state.py`使用只读连接和5秒语句期限，只统计在线角色聚合状态，不保存UID、OID、房间、卡牌和指令正文。部署预检与提交前强制拒绝IPv4/IPv6文档示例网段；仍必须使用prepare_private_release.py生成的私有目录，不得从公共示例配置发布。

`./com.netease.hsqsl`没有修改或删除。本批资料整合到既有四份核心MD，不新增重复长期说明。

## 2026-10-11 近期全日志修复、服务器列表事故恢复与私有正式发布

从15:30起完整审计`hs-game/hs-sdk/hs-login/hs-postgres/hs-hotfix/hs-dns`：登录主链持续完成，未见panic/fatal/OOM/HTTP 5xx；确认真人PVP已跨过加载屏障但旧客户端缺少`support_skills_panel`，真人开战仍解析第二份畸形阵容；16:15连接池波峰来自连接同秒刷新在线租约；另有空契印分解、旧逻辑服号学会查询噪声及公网POST正文污染日志。源码现分别补齐缺失面板空装饰适配、真人PVP冻结阵容、10–14秒稳定错峰、完整快照轮询错峰、空分解幂等、学会旧服号兼容和热更POST正文脱敏。已开始战斗后的重复`battle_fighting`无论携带旧空对象或伪造阵容均只做无状态幂等忽略，不改冻结阵容或会话。

正式发布工具现把`hotfixserver`二进制纳入构建哈希、双主机备份、原子提交和失败回滚；`audit_recent_service_logs.py`仅输出匿名计数并用固定探针验证公网正文不落日志。最终私有候选全仓Go、vet、Python2原生UI、Linux私有原生单端/双端专项通过，真实PostgreSQL为91 PASS、0 SKIP、5辅助PASS。正式运行游戏PID`194382`、SHA256`59f29206bf8bf78ee4d226b40127829424a37c3fb8a4d9ba71a962bac2ecc2a0`；独立热更PID`98307`，私有热更SHA256`ef45ef1c28b4b1428e4b5a29fe29bf426e78347295313c02eee418a4f332d96c`，runtime`2026101101`。

18:36曾错误把公开脱敏`deploy/data/hotfix.json`覆盖线上私有配置，造成客户端获取服务器列表失败。18:41先从两台主机自动备份恢复原私有SHA`80fd4c28...`，18:42后端直连/客户端HTTPS/客户端HTTP三条列表链均通过；随后在仓库外私有工作目录合并runtime、重新构建、Linux原生、真实PG和只读预检后，于18:54发布上述私有正式候选，三条列表链再次通过。公开示例配置没有再用于正式分发，本机私有备份已更新为最终SHA；`./com.netease.hsqsl`始终未修改或删除。

最终PID窗口3550行游戏日志有5次`login_result`和5次`on_refresh_login`，用户验证失败、客户端异常、`support_skills_panel`、阵容形状拒绝、数据库超时、panic/fatal/OOM/HTTP5xx均为0；热更脱敏探针摘要2次、正文泄露0。窗口内没有真人PVP完整对局，所以服务端/原生验证不能替代玩家Android/MuMu实际完成匹配、加载、开战和结算。

## 2026-10-11 同步 PVP 部分玩家仍卡加载：锁定阵容统一兼容已发布

上一批双方就绪屏障上线后，生产已出现4次“同步PVP加载屏障完成”，说明`Loaded + BridgeReady -> notify_battle_fighting`链路已执行。但旧进程又记录一次真实`battle_fighting`失败：`fighting_cards`列表类型或数量无效。这表明部分客户端已离开就绪屏障，却在开战交界处把第二份阵容上报为`null`或旧自定义形状，因严格解析被拒绝后继续停在加载界面。

原生`activity-EE73BC1D.asm`确认`battle_cards.fighting_cards/support_cards`都是`ObjIdList`；`activity-EB947BB0.asm`1568行附近的`battle_layout_mgr.get_battle_cards`则会遍历布阵槽构造该自定义对象，空槽保留为0。因不同旧客户端/序列化路径存在形状差异，服务端不应在同步PVP开战时再依赖它。`send_pvp_cards`已经完成卡牌归属校验，并把权威快照锁定到`SyncPvpMatch.OwnFighting/OwnSupport`。

`internal/game/battle.go`现仅在“当前战斗UUID与已就绪同步PVP匹配UUID一致且主战已锁定”时，忽略客户端重复上报的阵容形状，在事务内重读服务器锁定快照构造`BattleLayout/Team`，然后仍执行卡牌归属校验和原生权威启动。快照使用独立切片，不与匹配状态共享可变底层数组。普通PVE、活动战斗及非当前PVP会话仍严格解析原生字典；同样的`fighting_cards:null`仍会被拒绝。重复`battle_fighting`不重置阵容、不重复启动。

新回归覆盖：就绪后`fighting_cards:null`与旧形状依然使用锁定阵容开战，实际保存的`Battle.Layout/Team`与快照一致，重复请求幂等，普通PVE畸形输入仍拒绝，原桥先到/实体先到/旧会话Tick补发全部保持通过。全仓Go测试、`go vet`、Windows/Linux正式构建通过；真实PostgreSQL 91 PASS、0 SKIP，另5项辅助PASS，真库用例已用畸形阵容完成开战、锁定快照落库及结算。实库产物SHA256为`23bb26b3e5562f2facdceaadd1e1ed54747f585928ce249be2c2c2d55a661416`。Linux项目私有原生引擎及单端/双端房间专项在发布前后均通过、0 SKIP。

15:30仅游戏二进制原子发布完成：PID`184792`，SHA256 `f1c1432f94ef4cbcabba8e5bac82b36315d290a123223761f91c1de1969509a7`，回滚备份`/opt/hs-server/bin/backup-20261011-152751`。独立远程核验确认游戏、PostgreSQL、SDK、登录服务active且9000监听属于新进程；发布后原生专项前后PID不变。游戏主机与独立热更的正式热更SHA均保持`80fd4c28b86a8f9288833f198438fb682cc6f5a53318c20f9d8c008bf24fb5ca`，独立热更PID`93185`且active；本地公开热更与线上私有正式文件SHA不同，通用组合核验因此按设计拒绝，未为通过检查覆盖热更。`./com.netease.hsqsl`未修改或删除。

新PID发布后长窗口有67次角色绑定和72次普通战斗启动，0 panic/fatal、0新`fighting_cards`类型错误。发布重连洪峰的15:30:16及15:31:42曾出现4条短暂deadline：两次助战刷新、一次真人在线租约、一次真人持久房间读取；15:31:43后连续6910行日志0新deadline、0阵容类型错误、0 panic/fatal。抽样时PostgreSQL为17会话（含本次只读审计），无idle-in-transaction或连接耗尽，游戏仍active。窗口内暂无新玩家完成一场同步PVP，因此Android/MuMu或玩家实际进入、开战、结算仍是唯一未关闭门；服务器和真实原生测试不冒充客户端最终通过。

## 2026-10-11 同步 PVP 永久卡准备界面旧修复记录（方案已撤回）

本节为当时发布记录；notify_battle_fighting路径会误入未初始化布阵，结论及实现已被本文件顶部1102替代，不得再照本节下发旧就绪通知。

生产库匿名聚合确认8场同步PVP均已锁定阵容、战斗UUID一致、`Loaded=true`、`BridgeReady=true`，但`Started=false`、`BridgeStarted=false`；其中5场超过1小时，最老约24小时。原生 `activity-A66FD60A`、`activity-C4F986B7`、`activity-3B69CDD4` 证明：同步PVP的实体加载和桥握手完成后，服务器必须分别向机器人和本人下发 `Avatar.notify_battle_fighting`，本人的通知才会执行 `battle_layout.set_ready`、设置 `challenge_ready` 并开放开战入口。旧服务端没有发送该通知；客户端中的 `pvp_load_complete` 是匹配界面音效名，不是真实Android上行RPC，因此双方会永久停在准备。

`internal/game/sync_pvp_contract.go` 新增幂等就绪屏障，仅在同一战斗同时满足`Loaded`、`BridgeReady`、匹配与战斗UUID一致、阵容已锁定且尚未开始时，按机器人、本人顺序各发一次通知。`load_entity_finish`与桥`ready`两种到达顺序都调用同一屏障；Tick会为发布前已经卡住的在线连接补发一次。连接切换到新PVP UUID时清空旧标记，不修改玩家资产、积分或比赛结果。通用`Battle.Status`仍保持原有语义，避免修PVP时破坏普通副本重复入场。

最终全仓Go测试、`go vet`、Windows/Linux正式构建通过。真实PostgreSQL先保留一轮90 PASS、0 SKIP的失败：新增测试把空援护错误编码成`null`，屏障实际已发出两条通知，但严格阵容解析拒绝该测试输入；改为契约要求的空数组后，最终91 PASS、0 SKIP、5辅助PASS，并实际覆盖匹配、桥ready、实体loaded、双方通知、`battle_fighting`和结算。Linux游戏SHA256为`88f8edba38388d5214686ac706144b9cacc73e366f5b05d12c9ee54acddd4ef4`，实库测试产物SHA256为`3be2cfa0b53046ad827ba9dc07ea08a8963df643643e7c75e9da6d60345ab173`。

14:42:53仅游戏二进制原子发布完成，新PID`182120`，回滚备份`/opt/hs-server/bin/backup-20261011-144050`；独立远端二进制、服务和监听核验通过。Linux真实原生引擎及单端/双端房间专项通过、0 SKIP，执行前后游戏PID不变。组合原生核验脚本因本地公开热更SHA与线上正式私有SHA不同而按设计拒绝，没有用覆盖热更消除该差异；独立热更仍active、PID`93185`、SHA256`80fd4c28b86a8f9288833f198438fb682cc6f5a53318c20f9d8c008bf24fb5ca`。发布后匿名聚合显示8份旧ready匹配及锁定阵容仍保留，服务重启已清除旧场景Battle，尚无人重新进入，故当前补发日志、started和bridge_started均为0；这不是修复失败，也不是玩家通过证据。`./com.netease.hsqsl`未修改，仍需玩家Android/MuMu重新进入并完成一场PVP。

## 2026-10-11 数据库查询风暴导致再次登录失败：服务器修复已发布

复合键修复上线后玩家仍偶发进不去，本次按真实线上窗口继续追查，确认不是旧 `unhashable type` 复发。发布前20分钟有324次 `context deadline exceeded`，PostgreSQL明确记录 `too many clients already`、保留连接槽耗尽和 `idle-in-transaction timeout`；约40名玩家时事务约95次/秒。根因有两条：`tickHumanPvp` 把存档中仍保留的已结算/已中止 `HumanRoom` 永久按100毫秒活跃局轮询完整角色JSON；每个连接五秒刷新在线租约后又各自执行全局在线表 `DELETE + SELECT`，数据库抖动后所有连接同步重试，最终占满16个游戏连接并拖住登录、副本、升级和重连。

源码修复位于 `internal/game/human_delivery.go`、`internal/game/sync_pvp_human.go` 和 `internal/game/service.go`：同一进程五秒内的全局在线读取由互斥缓存合并，租约失败改为五秒后重试；完整角色快照按状态分级为普通在线及已结算/已中止房间5000毫秒、匹配等待500毫秒、活跃真人局250毫秒。同进程即时刷新通知继续保留。新增32路并发读取合并和轮询频率回归；跨实例在线列表允许五秒缓存，下一窗口仍须完整完成共同选卡、指令屏障、结果投递与回滚。

全仓 Go 测试、`go vet`、Windows/Linux 构建通过。第一次全量实库为90 PASS、0 SKIP，唯一失败是旧跨实例用例仍要求新租约两秒内可见，该失败没有用于发布；用例改为验证明确的五秒缓存边界后，定向真实PG 47.08秒通过，最终完整真实 PostgreSQL 为91 PASS、0 SKIP，另5项辅助PASS。最终测试产物SHA256 `246fc8cab416ec0a01b01c44a5ec5b090c19a6c918bf9214ddf311d23a0e16a3`。

12:58:28仅游戏二进制原子发布完成：PID `177985`、SHA256 `25b286cf2a789ec424181ff43f82f799f492c2387f0b1d1681fc11910be4be94`，回滚备份 `/opt/hs-server/bin/backup-20261011-125538`。至13:09:41的新PID窗口22156行中，16次玩家绑定、16次 `login_result`、16次 `on_refresh_login`，没有绑定悬空；deadline、连接耗尽、idle-in-transaction、复合键异常和登录收尾延后均为0。数据库10秒样本为17会话、1 active、0 idle-in-transaction，约31.3事务/秒。游戏主机与独立热更主机配置SHA均保持 `80fd4c28b86a8f9288833f198438fb682cc6f5a53318c20f9d8c008bf24fb5ca`，`./com.netease.hsqsl` 未修改。完整无玩家数据证据见 `out/login-pressure-release-20261011.json`。

当前能确认服务器登录链和数据库压力已恢复；本机没有运行中的MuMu实例，ADB 7555未监听，因此仍等待玩家Android/MuMu实际看到UID并进入大厅，不能把服务端9/9冒充界面最终通过。

## 2026-10-11 复合键协议导致登录收尾卡死：已修复并发布

远端旧进程在 `Avatar.client_prop_changed` 下发 `house_unlock_state`、`restroom_grid_info` 时出现 `TypeError: unhashable type: 'list'`。原版 `custom_types.CustomDict.client_dump` 会把 Tuple 键先转为 Python 字符串（如`(1, 2)`、`(1, 0, 0, 0, 0)`），客户端再由 `Tuple.load` 还原；服务端此前把 `[]any` 直接作为 msgpack map 键，客户端 fallback 解包成 list 后中断，表现为登录界面无 UID。修复位于 `internal/mobileproto/msgpack.go`：所有 `mobileproto.Map` 的数组复合键统一按原版 `str(tuple)` 编码，并增加 `TupleKey` 与回归测试；收藏室、初音任务等现有复合键均自动覆盖。

本批全仓 Go 测试、`go vet`、Windows/Linux 正式构建通过；本批真实 PostgreSQL 为 91 个 `TestPostgres` PASS、0 SKIP，另 5 项辅助 PASS。游戏二进制 SHA256 为 `9e43470efc4d6fcc10cdaf57612442bab321c557a5077f33c5a3e9dc669584fa`，已仅原子替换 `/opt/hs-server/bin/gameserver`，备份为 `/opt/hs-server/releases/completion-20261011-050017-128446`，新 PID `167989`，9000/18080 监听和 `hs-game/hs-sdk/hs-login/hs-postgres/hs-dns` 均 active。发布后新 PID 窗口统计：3 次新连接、3 次 `on_refresh_login`、0 次 `unhashable type`、0 次 `hs_client_error`、0 次登录收尾延后、0 次 deadline/lock timeout；旧 PID 在发布前的历史错误仍保留在日志中，不能计入新版本。

线上 `/opt/hs-hotfix/data/hotfix.json` SHA256 仍为 `80fd4c28b86a8f9288833f198438fb682cc6f5a53318c20f9d8c008bf24fb5ca`，本地公开 `deploy/data/hotfix.json` 为 `857054a9f6d5e620c5a3a41ce7e9ad502765aa8eafc1670c3bd85dd2e82a388b`；本次未上传、覆盖或重启热更项目，也未触碰 `./com.netease.hsqsl`。服务端日志证明登录收尾已恢复，不替代 Android/MuMu 界面实机截图验收，需继续观察更长窗口。

## 2026-10-11 登录界面批量卡死紧急修复完成

01:00收藏室房间补建版本把`house_dormitory1/2`的系统解锁在`quick_login`事务中直接扩成`server_collection.rooms`所有权。真实客户端收到含新增空房间的Avatar初始属性后，在`sync_server_time`之后、发送`set_reconnect_auth_msg`之前中断，因此界面看不到UID。故障进程30分钟内98次鉴权、角色绑定全部成功，但只有24次`on_refresh_login`；无鉴权失败、属性编码失败、数据库deadline或lock timeout，故不是账号、UID生成、监听或连接池故障。

01:32先精确回退游戏二进制到上一已验证版本，热更1011、数据库及其他服务不变。随后按错误版本运行时段、空cards/slots、无对应布局三个条件定位18份受影响存档中的21个误写空房间（房间1共18条、房间2共3条）；修复前把受影响行保存到服务器项目0700证据目录，事务内删除且revision递增，再重启使在线连接读取新版本。清理后可疑计数为0，回退进程最近10分钟18次鉴权、绑定和`on_refresh_login`全部完成，三类登录错误均为0。

源码现将第一、第二房间旧档迁移延后到玩家已完成登录后的真实收藏室RPC；`ensureCollection`登录投影不再扩展这两间房，既有第三房间正式政策不变。新增本地及真实PostgreSQL回归同时验证：冷登录完成`on_refresh_login`且不写第一/第二房间，登录后布局请求才按原生系统解锁迁移，两个房间/两条布局冷重登保留，不伪造809/810钥匙，非法第三房间请求不改revision/state。全仓Go测试、vet、双平台正式构建通过；第三轮真实PostgreSQL为91 PASS、0 SKIP、5辅助PASS，前两轮因新增断言误伤既有第三房间政策的失败记录保留，不计作通过。

02:02组合发布完成，管理令牌轮换导致游戏进程再次安全重启后，独立远端/原生核验仍通过。正式游戏SHA256为`ff24f7eed6fde1ee940360b9195e7bc023e97c29589f1acfdc3830b9bc383984`，最终PID162523；热更仍为1011及`80fd4c28b86a8f9288833f198438fb682cc6f5a53318c20f9d8c008bf24fb5ca`。最终进程首个窗口6次新登录全部完成鉴权、绑定和`on_refresh_login`，三类错误为0；仍须继续观察更长窗口，服务端日志证据不等同于Android界面截图。只读检查工具的环境变量脱敏同时修正，意外暴露风险涉及的管理令牌已轮换，新值仅保存在本机私有文件。

## 2026-10-11 收藏室房间候选完整通过，发布进行中

005003候选session88856退出0：全仓Go、vet、双平台构建通过，91项真实PostgreSQL PASS、0 SKIP、5辅助PASS。新增无钥匙旧档按已开放系统补建两个房间、实际摆放及冷重登、非法房间请求revision/state完全不变、再次进入不重置均通过。游戏SHA256为19d9ac8e9e8c16514a3435115652cee93989afe43cf5f00e85208bdf93c757f4，测试产物4257145a9a1063ab0a7dccb1ef63f3d8a0527a6888901b5b1be98fe0edb82357；热更仍1011及80fd4c28b86a8f9288833f198438fb682cc6f5a53318c20f9d8c008bf24fb5ca，本批没有客户端正文变化。

只读预检session12930退出0；组合发布/独立核验session31045从00:56:47执行中，先轮询同一session，不重复发布。D38仍修复中，F38当前为尚未发布的验收说明，待真实发布结果回填；B15/B21及其他广义家具、住客问题未自动关闭。送礼成功原版继续进入girl_status好感度动画和语音，已有ret/bonus_box匹配证据不足以证明送礼卡死修复。

## 2026-10-11 收藏室第一、第二房间系统与存档不一致修补

上一轮为有效进展：跨日与模型保护已正式发布，双周期成功。本轮从新B38及B15/B21的收藏室反馈继续取证。仅只读抽取日志中六次实际保存请求对应角色的收藏室/材料，再读取其系统解锁与通关证据，未修改线上数据。六份样本全部已有house_dormitory1，第一份另有house_dormitory2，实际Collection.Rooms/Placements均为空，且都没有809/810钥匙。它们的空布局校验通过，不能据此说摆放功能通过。

原版guis/gui_utils.is_area_opened直接按dormitory_attrs.room_system_name查询unlock_systems；dormitory_main.setup_unlock_tips_list也按该映射上报on_unlock_system。实际Python2执行9组房间/版本判定通过，原函数SHA256为de38dbfc46c2dbeb5904178a8e88fa133ed95819895fc882fcd3f2784b2e0f37。三个房间规则一致，该函数不读取house_unlock_state或809/810是否存在。现行服务端ensureCollection却仅在本服剩余政策开启时补房间3，遗漏已开放的房间1/2；旧代码新增TestCollectionRoomsFollowNativeSystemUnlocks已明确失败“房间1仍被服务端锁定”。证据收藏室系统房间不一致复现.log，原版接口/开放判定/入口txt和原版执行验收.log。

修补：ensureCollection遍历原目录Rooms，根据各自System对应UnlockSystems版本大于0，仅创建缺失的CollectionRoom；不重置已有住客/能量/布局，不扣费、不生成钥匙、不重发家具，零版本及未开放房间仍拒绝。显式钥匙解锁原接口保留。新Go回归覆盖房间1/2摆放、未开放房间3拒绝及重复初始化持久JSON完全一致；首次CloneProgress对象比较受到规范化影响，改为前后持久JSON直接比较，失败证据保留。收藏室/解锁既有专项通过。新增真实PG回归TestPostgresCollectionSystemRoomsLayoutColdPersistence覆盖无钥匙旧档、两个房间真实RPC摆放、冷重登、非法房间请求revision/state不变及再次进入不重置。

同时在收藏室和头像业务拒绝处记录固定方法/资源编号/错误码与原因；送礼记录拒绝或事务提交完成，只有礼物种类数和回礼数量，不记录完整请求/库存或口令。纯诊断003949候选session42092退出0，90项PG与5辅助通过、0跳过，全仓/vet/构建通过，但没有发布。实际房间修补已另建005003候选，session88856执行全仓、vet、双平台及完整真实PG；后续先轮询同一session，不使用旧诊断候选报告。线上仍a924aa0d游戏与热更1011，尚未发布房间修补。

六份只读存档的本地重放全部通过：把各自真实unlock_systems合入副本，用原有11012地板验证，修补前全部因房间无ownership被拒绝；ensureCollection补齐后第一份有两间，其余各一间，布局校验全部通过、材料序列化前后相同。证据收藏室六存档本地重放.log及源码.go.txt；重放未回写生产，临时测试文件已移出编译目录。原版送礼server_callback两参数ret/bonus_box也已核对，成功后进入girl_gift回调，当前服务端回调形状匹配；没有据此认定送礼卡死已修复。00:52:15旧PID153623完整27770行窗口三类超时均0，保留收藏室修补前完整运行窗口.json/log，证明上一跨日修补的本次约16分钟运行窗口。

腾讯表已回填D38=修复中，F38注明房间系统/存档不一致、六样本重放通过及实库尚在执行；写前核对B38，写后回读保存截图收藏室定位表格回填.png。其他状态未改，按上一全表清单当前为13已修复、19修复中、4无法修复、6待修复；这是该次读写快照，不保证之后协作表没有新增。

## 2026-10-11 空榜跨日与模型保护联合发布完成

session18594退出0：000931组合发布及独立远端/原生核验全部通过。游戏PID153623，SHA256为a924aa0d319e0196ea25b5c08193d68592f67da5a9c9abbae86e2e4fca82bd97；独立热更PID92501，版本2026101011，配置SHA256为80fd4c28b86a8f9288833f198438fb682cc6f5a53318c20f9d8c008bf24fb5ca。六报告已同步根out并归档空榜跨日与模型保护联合发布/，私有配置只同步至用户本机deployment-config/hotfix.json，公开deploy/data/hotfix.json未覆盖为私有分发地址。

00:36:23新PID首个完整窗口1661行，0锁超时、0context、0idle。00:35:58活动周期842角色预读23.713秒、写阶段2.297秒、9个变动角色成功提交，先前跨零点全服空档持续40001路径本轮已成功完成。后续00:37:11完整3094行窗口确认00:36:24竞技/助战周期也成功：842角色预读24.074秒、写阶段2.331秒、49个变动角色提交；三类超时仍均为0。活动与竞技/助战两轮都已成功，但仍不代表长期高并发或下一日界验收。证据空榜跨日联合发布首窗口.json/log及空榜跨日联合发布双周期窗口.json/log均保留。当前没有运行中的测试或发布session。

模型失效速度保护已随1011正式下发；原版Python2夹具、三路热更一致性和90项PG验收通过，但没有Android连续战斗/材料本退出到主线活动的全流程验证，B16保持修复中。现表42项与全部历史反馈继续在案，全表目标未完成。

## 2026-10-11 空榜跨日与模型保护候选验收通过，发布执行中

000931第三轮session18959退出0，全仓Go、vet、双平台构建通过；90项真实PostgreSQL PASS、0 SKIP、5辅助PASS，教学生命周期、空年兽/助战跨日不改revision、真实周奖容量失败整轮回滚均通过。游戏候选SHA256为a924aa0d319e0196ea25b5c08193d68592f67da5a9c9abbae86e2e4fca82bd97，测试产物6641a61c7d59470ebd27eea87854fb98120634e1fec7c0f0fda3a57411e902ab；热更1011配置SHA256为80fd4c28b86a8f9288833f198438fb682cc6f5a53318c20f9d8c008bf24fb5ca。前两轮失败保留，不能把失败夹具修改说成新增生产缺陷修复。

只读预检session40218退出0。组合发布及独立服务/原生核验session18594正在执行，日志位于私有000931候选的out/idle-calendar-deploy.log、idle-calendar-remote-verify.log、idle-calendar-native-verify.log。后续先轮询同一session，不重复发布；待核实实际新PID、哈希及完整周期成功后再报告上线结果。四份现有MD已同步新增五条反馈，文档核验通过；网页未写入，不增加已修复数量。

## 2026-10-11 腾讯清单新增五条反馈

在线逐格复核发现B38新增“收藏室无法放置家具，提示收藏室错误”；B47“无法更改头像”、B48“饕餮之诞活动镇灵台南难度二卡死”、B49“试炼荒庭会卡死”、B50“给幻书送完礼物后会卡死”，五项均待修复且备注空。现有非空42项：13已修复、18修复中、4无法修复、7待修复；B51—59为空。证据十一日联合候选全表复核.json、十一日新增行复核.json、十一日新增行状态.json、新增收藏室反馈详情.json。本次网页只读，未改状态。头像更换界面/实体/客户端校验/回调原生四份txt已取证：原版先检查owned_head_box和资源类型，call_server传回调及编号，成功回调仅接收ret并更新玩家面板、头像组和缓存，与服务端现行参数/回调一致；尚未证明新增无法更改头像的根因，不能直接关闭。

镇灵台南不属于用户明确跳过的镇灵台西，继续排查；试炼荒庭与既有历史问题关联但不能直接判同根因。B38原清空的抽卡黑屏、B39藏书竞斗、B40妄言历史问题继续在案，不以新文本覆盖历史。收藏室00:25:43保存回包确实到达on_update_restroom_grid_info，但当前日志只有7字节长度，没有返回码与请求参数，不能据此认定成功或定位具体拒绝规则；原文保存新增收藏室保存回包原文.log。

旧PID148937的00:13:38起完整采样已有29次lock timeout、3次context deadline、0idle超时，跨日周期仍失败，证据十一日联合候选等待窗口.json/log。新候选尚未发布，不能拿本地或实库通过替代线上性能结果。

## 2026-10-11 联合候选第二轮实库结果与教学刷新夹具修正

000931联合候选session94032退出1：全仓、vet、双平台通过，真实PG为89 PASS、0 SKIP、5辅助PASS，唯一失败TestPostgresClientBattleLifecyclePersistence要求取名后的Tick必含全量material_mgr刷新。新增空档跨日revision/state专项及周奖容量回滚专项通过。第二轮完整报告和日志保留于out/evidence/qq-bugs-20261010/空榜跨日第二次实库失败/；本批没有发布。

核查setNicknameGender实际只直接返回nickname/gender/nickname_flag，并不发布全量刷新；旧测试此前依赖后台空年兽或助战日期写库偶然建立刷新队列。现于真实取名、教学引导完成后和进入战斗前，通过已鉴权POST /admin/player/update提交性别2，确认201后再继续原生命周期。保留Tick全量属性逐项对照实库、不得下发旧权威指令、不得推进战斗或改变资产的断言；没有在生产代码补无意义写入，也未清队列回避核验。session18959重新执行全仓、vet、双平台及真实PG；后续先轮询同一session。线上仍233214及热更1010，不能将候选报告当作已上线。

## 2026-10-11 空年兽账本跨日候选与模型时标联合验收

针对跨零点833角色连续冲突，settleActivityCalendar现在仅对首次初始化账本或已有年兽排行成绩/小时历史的角色推进年兽日期和结算收据。未参赛者保留原空榜账本，不再逐日重写；山海结算和赛季处理仍独立执行，未按年兽条件跳过其他活动。新增prepareFirstNianRanking，在首份有效年兽成绩入账的同一角色事务里接续当前日/周截止基线；已有成绩或小时历史不迁移日期，未来日期明确拒绝。原奖励表、资产发放、跨角色保护锁、版本冲突和整轮回滚不变。

本地专项通过：未参赛跨日持久JSON完全一致、真实零伤害首次参赛建立基线、已有成绩不跳过待结周期、日期回拨拒绝，以及既有日榜/周榜/山海/本服综合奖回归。初次测试用CloneProgress后做对象DeepEqual受到克隆规范化影响，改为直接比较持久JSON前后；未放宽业务不变量。新增TestPostgresIdleActivityCalendarRolloverPreservesVersion直接核对实库revision和state。原社交元数据回载测试补入明确排行成绩，使其仍经过真实周期写入，不再依赖“所有空榜角色每天都改档”的旧行为。

模型独立235711候选session56438已exit0，89项真实PG PASS、0 SKIP、5辅助PASS，全仓/vet/双平台通过；证据归档模型时标独立候选验收/，没有发布。000931联合候选首轮session5592退出1：实库空档跨日版本4变5，定位到PVP后台无条件ensureFriendAssistDay更新空助战日期；另一个周奖容量夹具直接植入成绩而未同步首次参赛基线，旧周日期导致不进入目标奖周期。失败产物与完整日志归档空榜跨日首次实库失败/，不得删除或计作通过。现已仅在Active/Passive/RewardTimes有实际计数时后台重置助战，时钟回拨校验保留，真实助战使用/进入路径继续建立当天基线；空角色不再仅为日期写库。周奖夹具显式植入与其2026-10-20成绩对应的日基线2026-10-19、周基线2026-10-14，保留容量满必须全服回滚及清理后重试全部断言。年兽/助战/周期本地专项通过，session66166退出0。当前session94032重新全仓、vet、双平台和实库，先轮询同一session；尚未发布。热更保持候选1011，线上仍233214游戏及热更1010。本轮未改腾讯表格状态，全表和历史反馈继续在案。
## 2026-10-11 跨零点结算持续冲突，模型候选暂不发布

00:03前后的同PID148937完整跨零点窗口出现活动排行连续三轮40001回滚：00:01:03写阶段7.652秒、00:02:01为8.452秒、00:02:59为4.888秒，均833角色、变动角色返回0；同时完整窗口已累积10次lock timeout，0context、0idle。证据周期互斥跨零点窗口及周期互斥跨零点重试窗口.json/log。此前23:45的9角色成功仅是小范围启动周期，不能代表跨日全量成功；性能与跨日结算仍未关闭。

源码定位到settleActivityCalendar对所有角色无条件推进cal.NianDay/NianWeek，并为没有年兽参赛记录的角色写日榜空收据。跨日时即使未参赛也会变动，缩锁会重新接近全服范围，并在普通改档下整轮冲突；需减少不必要的空账本写入，同时保证首次参赛基线、历史收据和真实排名奖励完整。不能简单跳过整个结算回调、强行提交旧快照或放宽锁超时。下一步应验证未参赛跨日不产生写入、首次参赛在自身事务建立正确截止基线、真实参赛者奖与重试仍一致；尚未实现该候选。

模型失效时标修补235711已补齐三路下发，重新全仓Go/vet/双平台通过。游戏候选SHA256为58ac30696dee12e751753851c03131a7458348137c6f57606d222cd8c84fc62a，热更1011的私有配置SHA256为80fd4c28b86a8f9288833f198438fb682cc6f5a53318c20f9d8c008bf24fb5ca，测试产物451ce87a464fafbf2780d22aae89d1041f9b3b339164bea0d2f4698193857e6e。session56438真实PG仍执行中，先轮询同一session，不得复制旧通过报告。尚未发布1011，当前线上仍游戏479aad04及热更1010。全表状态未改，优先解决新确认的跨日持续冲突后再组织下一发布。
## 2026-10-10 模型失效后速度更新候选，原版复现和修补回归通过

从联合修补跨23点窗口.log定位到22:45:43客户端真实异常：revival_set_battle_preferences(1.75,True)经player_input、battle_base、story_mgr、fashion、model_driver、super_model分发到wrapper.model.set_animator_speed，调用已失效模型get_socket_count抛出“Invalid model which has beed destroy!”。原版wrapper.model.is_valid直接读取_ins.valid；destroy正常路径把_ins替换为swallow并最终标记destroyed，不能只依赖destroyed字段判断引擎已失效的句柄。

新增internal/game/model_lifecycle_script.py：只包装底层set_animator_speed和set_move_timescale，在原生is_valid为假时不再访问引擎对象；动画速度仍保存animator_speed，model_driver仍保存time_scale。有效模型全部继续执行原函数，不捕获或吞掉其RuntimeError；同对象重新有效后立即恢复原处理。安装以类修订号幂等，模块尚未导入或尚未定义类时延迟1500ms再试。通过既有合并器同时进入启动和运行期扩展，候选runtime.index=2026101011，尚未发布。

test_model_lifecycle_native.py实际调用原版Python2的driver.set_time_scale、driver.set_animator_speed、super_model分发闭包、model.is_valid及两个底层速度函数。旧路径复现同类RuntimeError；修补后两种成员顺序均只更新有效模型，挂点特效帧率/动画率保持原逻辑。覆盖缓存、模型恢复、暂停、重复安装、模块迟到和有效模型其他错误继续抛出。引擎句柄为失效边界夹具，因此不是Android实机回归，也不能把此异常直接等同所有连续战斗或白屏反馈。证据模型时标生命周期原生验收.log及模型驱动/底层/复合模型/移动时标原生反汇编文件。

完整启动双版本和运行期正文经Python2编译通过，扩展标记各唯一，清单一致性核验通过。235711私有候选首次全仓session81479退出1：TestClientExtensionsMatchesColdAndRuntimeHotfix检出游戏内扩展仍为1010且缺少新脚本，未进入发布。完整失败日志保留模型时标三路一致性首次失败.log。已补client_extensions.go的go:embed与同顺序组合、battle_observe.go的bridgeHotfixIndex=2026101011及桥头部索引，重新生成私有清单；不改测试预期绕过不一致。当前session56438重新执行全仓Go、vet、双平台构建、真实PG，后续先轮询此session。线上仍233214游戏版本及热更1010，本轮尚未修改腾讯表格状态。循环共享保底、首次对决及其他全表历史问题仍在案。
## 2026-10-10 循环与共享抽卡保底原版取证，未改正式概率

上一轮周期互斥发布已有实际进展。本轮先复核幻术星尘对决：冷重连缺enemy_cards的1005修补仍在，但没有新增证据证明首次对决卡死已解决，因此保持在案。随后核查启程尚缺的20033循环及1060共享规则，新增out/tools/test_shared_promise_native.py，直接执行原版Python2的get_promise_card、get_onetime_promise_card、do_promise、check_promise_rule、refresh_record，使用实际卡表和启程权重；随机选择器仅用于确定性检查候选集合，不生成新概率。

实际原版验证通过：20033阈值固定10，九次未命中后第十次强制稀有度3；提前命中稀有度3会清零count/finish_count，命中稀有度4不会清零；real_count=10在循环重置后保留。1060在count=59且finish_count<2时触发，满足两次则不触发；刷新到60后原版将完成次数记为2，不能把此收据字段反推为历史一定抽得两张典藏。1060被多个卡池共用（完整ID列表见循环与共享保底原生验收.json），不能只在启程维护独立计数。

当前服务端PromiseCardRecord只有rule_id/count/finish_count，未存real_count；handleGachaContract只接入首抽1001和启程1002—1011。原版模块能确认单规则行为，但现有客户端pyc中未找到外层promise_check调用者，尚缺多规则同时触发时的选择/替换顺序及旧档无逐抽来源的迁移依据。不能用通用“稀有度不低于3”逻辑冒充20033，更不能从各池总次数猜已有命中收据。本轮没有修改正式抽卡、卡池概率或玩家存档，也没有发布和修改表格状态。工具及JSON/log为新增原生证据，不是Android抽卡验收；全表目标继续未完成。
## 2026-10-10 周期跨角色互斥及助战拷贝优化已部署，线上重试成功

233214候选全仓Go、vet、双平台构建及89项真实PG PASS、0 SKIP、5辅助PASS通过。session70935组合发布、独立服务/监听核对和原生运行时核验均退出0。游戏PID148937，SHA256为479aad04f0458b02099a619556975846fa2e3e4ff90055d82fd9b34514a813d2；热更PID92086、版本2026101010、SHA256为20706a97a45ae523f8ddfdb77b3a9b46716d64c1db5a13aafc6eea59c4942463。六报告、全部验收/发布日志和私有热更备份已同步，归档周期跨角色互斥发布/。当前没有运行中的验收或发布session。

新PID首窗口23:45:04共3138行，无lock timeout、context deadline或idle超时。823角色活动周期预读25.856秒后无需写；竞技助战周期预读24.056秒、写阶段2.359秒，因并发改档40001整轮回滚。后续23:47:12完整7287行确认：23:45:52下一轮预读22.105秒、写阶段1.802秒，9个变动角色成功提交，三类超时仍均为0。首次冲突和成功重试都保留于周期跨角色互斥重试完成窗口.json/log，不用后续窗口抹掉首冲突。这证明本次后台重试能完成，不是长期高并发、跨日结算或Android全玩法验收。

本批上线仅服务端周期依赖保护与助战资格检查避免全档拷贝，未发布完整资源校验表，未补造玩法或概率规则。本轮网页只读，现表37项非空的13已修复/18修复中/4无法修复/2待修复不变；第38—40行清空反馈与其他历史反馈继续在案。旧231552混合事务失败证据保留。全表目标未完成。
## 2026-10-10 周期跨角色互斥修正候选，完整验收进行中

上一轮真实PG已证明单独把周期设为Serializable不足以保护Read Committed双角色事务。新候选233214在dbstore/periodic_guard.go增加按当前schema隔离的数据库保护锁：普通UpdateSocial、非只读UpdateLeagueSocial、非周期UpdateAllSocial在任何业务读取和行锁之前取得事务级共享锁；周期写阶段独占池连接，先尝试取得会话级独占锁，成功后才建立Serializable事务快照。忙时周期直接拒绝且不推进周期键，后台按原规则重试。普通单角色UpdateProgress不取得此锁，仍以行版本冲突拒绝旧写。

会话锁在事务提交或回滚后独立清理，清理使用新的3秒上下文；无法确认释放时关闭物理连接，禁止带锁连接返回连接池。保护锁不会替代角色行锁、幂等收据或权限校验。此方案仍可能让跨角色请求等待周期写阶段，不能声称全站请求完全无等待；目标是避免周期计算锁住全部普通玩家存档，同时保留跨角色一致性。日志已将“加锁事务耗时”更正为“写阶段耗时”，仍不是实际行锁时长。

验收保留普通无关玩家写入、同角色并发改档拒绝、嵌套快照及版本重读；新增周期独占/释放后重试、周期先开始时社交旧依赖被拒绝、普通社交/全服管理/学会写入先开始时周期避让且下一轮读取新状态。此前失败证据不删除，新的测试要求对应当前互斥契约，不再要求两个周期同时进入受保护阶段。根本地TestSocial辅助测试通过，不是PG通过。

233214候选session53130已退出0：全仓Go、vet、双平台构建及89项真实PG PASS、0 SKIP、5辅助PASS通过。游戏SHA256为479aad04f0458b02099a619556975846fa2e3e4ff90055d82fd9b34514a813d2，测试产物23c06a32d0b7349304ae4c83fdaf1bea4df0873075fa21ae95db4faf078934e5。只读发布预检session75280退出0，组合发布/独立核验session70935进行中，尚不能宣布上线完成。候选包含助战资格拷贝优化，热更保持1010。后续先轮询同一session，保留任何首次失败并同步六报告。

23:33:56旧PID145550完整67692行窗口：4次lock timeout、6次context deadline、0idle；其中3次助战刷新，另3次真人房间/在线租约。证据学会修补待替换完整窗口.json/log。表格现有非空37项：13已修复、18修复中、4无法修复、2待修复。第38—40行已清空，历史的抽卡角色黑屏、藏书竞斗、妄言仍未关闭，不能把清空当作修复。第47—59行无新增，证据全表周期互斥候选复核.json及周期互斥候选新增反馈检查.json。本轮网页只读，没有新增已修复；全表和历史问题目标仍未完成。
## 2026-10-10 周期缩锁候选混合事务实库失败，禁止发布

231552私有候选包含助战资格去除全档拷贝及周期写阶段改用可串行化快照、只锁实际变动角色。session62347已退出0：全仓Go、vet、双平台构建通过，真实PG 87 PASS、0 SKIP、5辅助PASS。首轮独立证据归档out/evidence/qq-bugs-20261010/周期隔离首轮验收/。候选游戏SHA256为e7465630ccfde3992b0ac5a52cb8ea279edfcd4f25e1f25aabc81bc6b6e57b66，未发布；线上仍225602学会修补版本。

首轮新增测试证明普通无关角色写入不受周期计算阻塞、同一角色并发改档拒绝旧快照且重试不重复、两个可串行化周期不能形成写偏差。但普通UpdateSocial仍是Read Committed，不能据此推断混合隔离事务的跨行依赖也安全。已新增TestPostgresPeriodicRejectsMixedSocialWriteSkew：周期依赖乙写甲，普通双角色事务依赖甲写乙，要求旧周期整轮拒绝。session25024已退出1：新增混合事务用例失败，87项原PG仍PASS、0 SKIP、5辅助PASS。23:26:14实库明确记录“错误=<nil>，甲=1，乙=1”，周期未拒绝旧跨行依赖，两个互斥写入同时落库。此候选禁止发布，不能拿原87项通过抵消新增失败。本轮测试产物SHA为71df455ccb943acacd965938e1fa98a646736ceb7c58b21b203e8a853b6c55a6；构建、完整日志和报告归档out/evidence/qq-bugs-20261010/周期混合事务失败/。下一步须修正跨行读依赖保护，并覆盖普通多角色事务及全服管理事务，不能只修改一个测试期望。当前没有运行中的发布或验收session。

未通过混合事务核验前不发布缩锁候选，不增加表格已修复计数。日志中的“加锁事务耗时”目前仍表示整个写阶段，不能解释为实际持锁时间。用户指定无法修复仍限四项；其余全表及历史清空问题继续在案。
## 2026-10-10 学会推荐类型修补已部署，首次运行核验失败须保留

225602候选全仓Go/vet/双平台及85项真实PG PASS、0 SKIP、5辅助PASS通过；组合部署和独立服务/监听核对成功。首轮session84224在verify_native_solo_release.py因“本批启动日志异常”exit1：新PID145550于23:07:04—05出现4条lock timeout（引导完成、战斗结果、观察检查点、battle_fighting）。完整失败日志与摘要已归档，不能写成整批首次全通过。

后续session92342复核exit0，证明游戏/热更哈希、进程、原生运行时及后续最近100条窗口通过，不抵消首轮错误。当前游戏SHA256为3455caebe790ad46868e301b32dd3c188c6e5144a9c1ae0905aa42ccedee3fe5、PID145550；热更1010、PID91768、SHA256为20706a97a45ae523f8ddfdb77b3a9b46716d64c1db5a13aafc6eea59c4942463。六报告、首失败说明及私有分发备份已同步；归档学会推荐类型修补发布/。当前没有运行中的发布/验收session。

23:09:45新PID完整7125行仍4锁超时、另1次context deadline、0idle。23:07:05周期预读21.325秒，写事务1.639秒、仅5角色变化，却仍锁全部814角色；故同时出现1秒锁等待失败并不矛盾。源dbstore/social_all.go在非空预读后false/cache阶段依然对全表avatars和progress FOR UPDATE，这是下一项需要消除的明确范围；必须保持全局排行与奖励一致性，不能直接提交旧预读档或只锁部分行而忽略并发变化。

已部署只包括学会trial_score与两个boss血量容器类型修正，不含根工作树availableAssistCard候选优化、不含完整资源校验表候选。根工作树助战优化已有专项和本地对照，尚需独立全仓/实库/发布。腾讯表B42的差量缺陷说明已保存，D42仍修复中；未新增已修复计数，全表仍13已修复、18修复中、4指定无法修复、5待修复。历史清空和新日志问题仍在案，全表目标持续未完成。

## 2026-10-10 助战候选资格拷贝优化，尚未发布

跨23点窗口在23:01:16记录812角色预读27.343秒，无需加锁；完整45095行仍1锁超时，但新增5次context deadline，分别为3次refresh_assist_use_times及真人在线租约/持久房间读取各1次。证据联合修补跨23点窗口.json/log、跨23点请求超时摘录.json。只证明本次整点未空写全服，不能宣布全部卡顿消失。

定位到validStrangerAssistProvider在最多1000名候选的资格检查末步调用snapshotFriendAssist，后者CloneProgress整份玩家档，虽然资格只需卡存在且非禁战。根工作树新增availableAssistCard复用原有两项检查，资格路径改为只读查卡；真正入选快照仍沿用原深拷贝和契印逻辑，双向黑名单、同服、好友排除及事务内重新验证不变。现有Assist/Stranger专项通过，session63990 exit0。

BenchmarkAssistCandidateValidation在3000材料夹具、20次对照下：原快照资格每次3539075ns、776597B/11948分配，新资格85ns、0B/0分配。证据助战候选拷贝开销对照.log，session55570 exit0。这不是线上压测；仍存在查询最多1000份完整存档的问题。本优化不在225602学会候选内，尚需后续全仓、实库及发布，不能拿学会候选证据覆盖。

225602学会候选已全仓/vet/双平台及85项PG PASS、0 SKIP、5辅助PASS，session99500 exit0。23:02:49开始组合发布，session84224仍执行；先轮询同一session，再独立核验报告归档及私有备份同步。学会候选SHA为3455caebe790ad46868e301b32dd3c188c6e5144a9c1ae0905aa42ccedee3fe5；热更保持1010，不含更新资源候选和助战新优化。

## 2026-10-10 更新差量缺陷取证与学会推荐类型候选

上一轮有具体发布与表格回填，属于进展。本轮修正out/tools/gen_npkfilelist_md5.py：按清单大小和MD5选择Documents、APK缓存或APK内资源；对codec>=62按已验证规则还原实际偏移；拒绝未知编码、重复编号、越界和重叠，整批成功才写输出。旧大包整包跳过逻辑已删除，命令现必须显式--output。没有改正式清单或资源。

运行python out/tools/gen_npkfilelist_md5.py --output out/evidence/qq-bugs-20261010/完整逐文件校验候选.json，16包105734条、4836436字节、MD5 cf1c0302ff7d74aac4c59e03d577f275；script12条与旧表完全一致，两个场景包自动选择匹配清单的APK缓存。生成器边界测试2项通过，覆盖编码偏移、损坏索引拒绝、哈希优先选源和无匹配拒绝。工具不会上传或改NPK。

原生test_complete_patch_table_native.py执行全部真实索引：完整表消除缺键，但15包105722条编码偏移仍被原版_check_file直接放入下载队列并越界；索引还原后队列偏移全有效。check_patch_resource_ranges.py对每包一个小块实际GET Range，16项全部206、Content-Range及MD5一致；char1原编码偏移对照返回416。候选尚缺原生写包及Android完整启动验证，不能直接发布，更不能认定就是该vivo手机故障原因。F42已回填这些事实，D42保持修复中，截图更新差量缺陷回填.png。ADB在线列表仍空。

22:50:24当前PID143577完整19003行窗口仍为1锁超时、0context、0idle；没有随机看板或本人名次RPC日志，不能据此冒充手机回归。新增7条客户端异常中，学会推荐两类错误已定位：trial_score原生Int却发字典；boss_challenge_hps和boss_max_hps原生字典却发列表。leagueInfoWire候选修正这三项，不更改玩法积分与存档。实际Go回包经原生league_info逐字段、整对象、client_dump及on_get_recommend_leagues回调全部通过；图形刷新为夹具。证据学会推荐修补专项.log、学会推荐原生回调验收.log、学会字段类型原生.txt。初查里members/apply_map的列表报错来自JSON导出mobileproto.Map，实际Msgpack为空字典；最终原生测试明确恢复这两种导出形态，未修改服务器正确容器。

已扩展既有真实PG并发建会测试，冷Service从三份真实主档查询推荐，核验三字段类型与内部公告隐藏。私有候选目录%LOCALAPPDATA%/hs-server/release-workspaces/20261010-225602，全仓/vet/双平台/实库正在session99500执行，尚未发布；线上仍223100版本。后续先轮询此session，完成后按既有组合发布流程。还记录到战斗速度推送对已销毁模型调用set_animator_speed的RuntimeError，以及一个统计域名连接错误，未通过吞异常伪称修复。

## 2026-10-10 随机看板与本人名次联合修补已发布

223100私有候选全仓Go、vet、双平台构建及85项真实PG PASS/0 SKIP/5辅助PASS通过；session92071组合发布、独立远端与原生运行时核验均exit0。游戏PID143577，SHA256为4bbb7a78fb8528c11cf658b109be527807c8f4f529fa491045b22e63f7c78fc6；热更PID91509、版本2026101010、SHA256为20706a97a45ae523f8ddfdb77b3a9b46716d64c1db5a13aafc6eea59c4942463。六报告及私有热更备份已同步，归档随机看板与本人名次联合发布/。当前无运行中发布或验收session。

22:43:29新PID完整首窗口2776行，1次lock timeout、0次context deadline、0次idle超时；809角色一轮无需写锁，另一轮4变动角色写锁1.450秒，预读分别25.494秒及20.813秒。未跨23点，仍不可关闭性能故障。证据随机看板与本人名次首窗口.json/log。

腾讯B46随机看板已改D46“已修复”，F46写明整数三模式、冷保存、原版Python2选择逻辑、85项实库及发布证据；E46保持待测试，因为没有Android人工画面验收。已回读自动保存，截图随机看板已修复回填.png。全表40项状态相应为13已修复、18修复中、4无法修复、5待修复；B42 vivo仍修复中，已填入口核验边界。全表目标未完成，不能仅凭本批上线结束。

更新资源索引只读审计进一步确认：按旧生成器优先级读本地文件，14包与清单MD5和大小一致；scene/scenewd2的Documents小包不一致，但APK缓存两包分别完全匹配清单。13个编码偏移包的旧解析全部越界，按既有codec>=62偏移还原后越界均0；script12条无需还原。证据更新资源索引只读审计.json、更新场景源候选核对.json。下一步应以清单哈希选择资源源，并核实原生差量下载及重组对编码偏移的兼容，不能直接运行旧gen_npkfilelist_md5.py跳过逻辑，更不能用Documents小场景包覆盖正式资源。正式清单和资源本轮均未改动。

## 2026-10-10 随机看板与本人名次联合发布执行中

223100私有候选通过全仓Go、vet、双平台构建和85项真实PG PASS/0 SKIP/5辅助PASS；新增随机看板冷保存专项1.15秒、本人名次冷读取且不加载榜单专项0.68秒通过。22:38:06开始组合发布，session92071执行中，尚需独立远端及原生运行时核验、同步六报告与私有热更备份。最终游戏候选SHA256为4bbb7a78fb8528c11cf658b109be527807c8f4f529fa491045b22e63f7c78fc6，热更维持1010。

22:38全表逐格复核至56行，40项非空：12已修复、19修复中、4无法修复、5待修复。证据全表2238复核.json；B42及B46新反馈均已标修复中，历史清空反馈仍在案。用户指定跳过仍仅四项，不扩大无法修复范围。

vivo更新入口进一步只读核验：16个资源在清单端口和原生80端口32次HEAD全部200、长度一致、声明支持分段；两个入口的逐文件校验表长度与MD5均匹配。新工具check_patch_resource_entry.py只读取私有配置、不打印地址、不下载大资源，结果vivo资源入口核验.json。

发现独立风险：现有逐文件校验表只有script，旧gen_npkfilelist_md5.py对大包索引越界采取跳过，并假设玩家本地同源。原生npk_patcher._check_file在差量分支直接索引fileid；test_patch_missing_md5_native.py用原版Python2函数及单条索引夹具复现缺char1表时KeyError，补单条夹具后可进入下载队列。证据更新缺失校验表原生复现.log及更新逐文件校验原生.txt。这不是vivo故障归因，也没有生成正式校验表。既有audit_external_resources.py掌握codec>=62的偏移还原，但需要进一步确认原生差量下载对编码偏移的兼容；不能只填MD5后宣称更新修好。没有修改正式清单或资源。

## 2026-10-10 随机看板整数模式修补及本人名次验收

腾讯表新增B46“幻书无法随机放在界面，提示错误”：原生面板发送0关闭、1全部、2收藏，旧服务端仅接受布尔值且不保存随机范围。候选新增GirlRandomType，兼容旧布尔存档；实际设置同事务保存模式与开关，先回推模式再回推开关与成功回调，冷登录同时投影。拒绝越界、字符串、小数和null。原生选择器保持无收藏时回退全部的原规则，不改变抽卡或奖励。

本地专项覆盖模式切换、属性推送顺序、非法输入不改档、旧布尔兼容；实际Python2 board_girl_mgr选择逻辑验证0/1/2及空收藏回退。证据随机看板模式专项.log、随机看板原生选择验收.log。新增TestPostgresGirlRandomModesColdPersistence待正式实库验证；不是Android画面验收。表D46已改修复中，尚未上线不标已修复。

222217本人名次候选全仓、vet、双平台及84项真实PG PASS/0 SKIP/5辅助PASS已通过，session62932 exit0；未发布且不含随机看板修补。下一正式候选合并两项后重新完整验收。B42 vivo停在检查更新：本机正式健康接口、两条清单入口与热更正文均200且清单JSON有效，仅能证明本机可达，不能推断vivo网络及资源解析正常。证据vivo更新入口只读检查.json，仍待定位。

## 2026-10-10 空周期联合修补已发布

220604私有候选全仓Go、vet、双平台构建及83项真实PG PASS/0 SKIP/5辅助PASS通过；session35421组合发布、独立游戏和原生核验均成功exit0。游戏PID141097、SHA256为6ec3227026048e24a1cb3312e424ba0546e698a6b13722ce62b7f4ded43858d4；热更保持2026101010、SHA256为20706a97a45ae523f8ddfdb77b3a9b46716d64c1db5a13aafc6eea59c4942463。六报告和私有备份已同步，归档空周期联合修补发布/。目前无运行中的发布或验收session。

22:21:08新PID完整首窗口2024行：1次锁超时、0次context deadline、0次idle超时；805角色一轮预读无需加锁，另一轮6角色变动写事务1.729秒。尚未跨下一个整点，不能宣称整点持锁和并发卡顿全部消除。证据空周期联合修补首窗口.json/log。已发布的是竞技空日期及年兽空小时榜修补；根工作树的本人名次读取候选不在此包内，尚待后续完整验收与发布。全表状态本轮未提前关闭。

## 2026-10-10 本人竞技名次读取候选，尚未发布

已把query_sync_pvp_rank与query_sync_pvp_world_rank提前分流：先验证唯一回调参数，再调用原有SyncPvpRank存储接口，仅返回真实本人名次；不再先取1000条榜单及完整玩家进度。本服/世界过滤和同分OID排序仍由原存储负责，榜单列表接口维持原行为。

新增personalRankOnlyStore在读取榜单或SocialAvatars时明确失败，实际Handle验证1102人同分情况下本服第2、世界第1102均正确，超过1000窗口也不截断；非法额外参数拒绝。专项及既有排行榜测试通过（session60387 exit0），证据本人竞技名次读取专项.log。此源码候选不在当前220604发布包内，尚需全仓及真实PG验证，不宣称已上线。

空周期联合220604候选已完成83项真实PG PASS/0 SKIP/5辅助PASS，session71687 exit0。22:14:26开始正式组合发布与独立核验，活跃session35421；仍需轮询该session并归档正式报告、同步私有分发备份、采集新PID完整窗口。当前网页状态未改变。

## 2026-10-10 空年兽小时榜与竞技周期联合候选

22:12只读查询本候选远端test.log确认新增TestPostgresEmptyNianHourLeavesDormantRevisionUnchanged（0.46秒）及TestPostgresAsyncIdleCalendarColdEntryAndNextDayAward（0.94秒）均PASS；整套session71687仍在原生战斗验收，不能仅凭专项提前发布。发布前22:10:26完整31511行累计18锁超时、3次请求期限超时。入馆仍仅11条历史包装异常，末次21:56:29。期限失败包括refresh_assist_use_times两次与query_sync_pvp_rank一次。

已定位下个明确性能缺口：sync_pvp_contract.go querySyncPVPRank所有方法先SyncPvpRankings(1000)，再SocialAvatars加载最多1000人完整存档；query_sync_pvp_rank/world_rank最终只返回SyncPvpRank的本人名次，前述榜单和完整存档无用。后续应先识别个人名次方法、校验唯一callback，再直接调用已有全量名次SQL；保留本服/世界过滤、同分OID排序与错误，不用前1000人估算本人名次。当前尚未改此路径。

早期220239候选session31920已结束exit0：82项真实PG PASS、0 SKIP、5辅助PASS，新增竞技冷入口/离线revision/次日发奖专项1.11秒通过，归档竞技空周期首次实库/，此候选未发布。联合220604候选全仓Go、vet与双平台构建均通过（session39256已结束exit0），最终真实PG已启动session71687，日志私有候选out/empty-cycle-pg.log，仍在运行，先轮询同一session，不重启。当前源码与联合候选已包含防守初始化基线及空年兽小时榜修补，只有这份最终结果可用于发布。

22:03:14对1010进程完整17183行窗口核查发现：22:01:15活动结算801角色全部写入、持锁11.152秒，累计18次lock timeout/1次context deadline/0次idle超时。源码freezeNianHourRanking每小时为不存在于entries的角色写入全零NianTotal且更新Hour，导致空榜也整服改档。

候选保留首次空榜快照；已有空快照且仍无参与记录时不更新Hour。有旧名次离榜仍清零，真实零伤害参与仍在entries内、不能跳过。新增NianEmptyHourlySnapshotNoWriteAndEntryExit验证全进度JSON不变、小时内新成绩不提前入榜、下一整点真实零分入榜、离榜清零及后续空榜不再写。原年兽综合排行/空榜/真实零分专项与竞技/日周奖励联合测试通过，证据空榜与竞技周期联合专项.log。

竞技候选补查到设置防守阵容与离线防守结果也初始化积分，已补齐这些写入口的首次日期基线；只读候选预览不改存档。新增PG专项实际Handle冷进入、离线未参赛revision跨21点不变、首日无补奖/次日一次发奖，以及真实周期维护跨小时空榜revision不变和首次零伤害入榜。第一次220239私有候选仍在session31920实库验收，仅含早期竞技改动，不可拿其结果发布当前联合候选；待完成后生成新候选。当前线上仍1010，尚无联合发布。

## 2026-10-10 未参赛竞技空周期写入修补候选，尚未发布

1010线上21:58:09完整窗口6409行仍为1次lock timeout、1次context deadline、0次idle超时。入馆异常共11条，最后一次21:56:29仍为历史reward_update:1504；不能以短时无新增代替Android打开验收。两项超时分别为refresh_assist_use_times与apply_friend，性能仍在案。

本地候选social_periodic_rewards.go仅对AsyncPvp.Score非零者沿用原日周结算；零积分从未初始化者保留原有奖励日期，不再为跨21点空奖励写整服存档。回拨校验仍先执行，不删除旧日期。async_pvp_business.go两个实际入口先建立日期基线再初始化1000积分；零积分旧基线向当前推进，未来异常日期不覆盖，已有非零积分基线不改，避免补发休眠期奖励或吞掉正常待结算奖励。助战与活动账本不在本次变动中，所以不能宣称消除全部全服锁。

新增async_calendar_idle_test.go覆盖空/旧日期全进度不变、实际Handle进入竞技、当日不补历史奖、次日正常发奖、已有积分重进不推进日期及未来日期拒绝。已有竞技/日周/失败回滚专项通过。首次测试因未初始化助战Active空map及用CloneProgress比较动态类型而失败，原日志未覆盖；修正夹具并按实际JSON存储比较，复验通过。证据未参赛竞技周期检查.log、未参赛竞技周期复验.log、未参赛竞技入口及周期验收.log、未参赛竞技最终专项.log。下一步补真实PG冷存档与首次入口验证，完整候选验收后再发布，当前线上仍1010。全表目标未完成。

## 2026-10-10 入馆字典缓存1010已发布，客户端验收未完成

私有候选214618全仓Go、vet、双平台构建与81项真实PG（0 SKIP、5辅助PASS）通过，session97551组合发布、独立游戏及原生核验均exit0。游戏PID138347，SHA256为294d25f5f2291ee6950a4ff938472e34c1a4d3900563eb90729662dcee6fcbb4；热更PID90992、版本2026101010、SHA256为20706a97a45ae523f8ddfdb77b3a9b46716d64c1db5a13aafc6eea59c4942463。六报告及私有分发备份已同步，归档入馆字典缓存修补发布/。当前没有运行中发布或验收session。

21:55:44首窗口1544行，锁超时1次、context deadline 1次、idle超时0；799角色一轮无写、一轮9角色变动写锁1.704秒。21:56:33完整3732行仍相同超时计数。入馆异常仍有11条上报，reward_update调用点1479/1504，都是历史包装位置；当前1010清单runtime的调用点1516、startup的调用点1868。尚不能证明报错客户端已经执行1010，也不能仅凭旧栈位置断言全部问题消失。继续核对客户端补丁应用及实际打开，不关闭B45。证据入馆字典修补首窗口.json/log、入馆字典修补后续窗口.json/log、入馆1010首窗口异常聚合.json。

修补使用原生box.load().to_list()恢复全领按日回包内层字典，同时迁移旧签到缓存；真实RPC和reward_item.update已复现旧KeyError并验证转换后渲染。图形控件为夹具，无Android手动点击结论。回归人数接口缺真实账本、性能锁竞争和全表其他待修问题仍在案。

## 2026-10-10 入馆字典缓存根因及1010修补候选

1010私有候选214618全仓、vet、双平台构建及真实PG81 PASS/0 SKIP/5辅助PASS通过；21:51:12开始组合发布，session97551执行中，尚未回填已上线。1009至21:50:12完整窗口16071行，4次锁超时；共94条入馆异常上报（包含旧新包装重复上报，不能当94次独立点击），末尾栈均指向奖励缓存取下标0。另发现get_avatar_back_total_num原生为无业务参数call_server，回调(ret,total_num)送activity_returning_total.on_get_total_num；当前服务端尚无回归人数账本，不能伪造零人数完成活动。原生证据回归人数入口原生.txt、回归人数回调原生.txt。

1009私有候选213246全仓、vet、构建、81项真实PG（0 SKIP、5辅助PASS）、组合发布及独立游戏/原生核验均通过；游戏PID136594、SHA256为96fc5d9d5def797559c4a4fe3880e95d1aa7270e2f808a3b64050d07a47e44da，热更PID90776、版本2026101009、SHA256为6493d562221b75abade29c08575255e16b06803f6a820e5d27d86a84861d7901。六报告及私有备份同步，归档入馆末尾定位发布/。21:42:42新进程完整首窗口2024行有4次锁超时，其他两类超时0；790角色预读后5角色变动持锁1.569秒。仍不能关闭性能反馈。

1009末尾栈已定位prepare/beginner_guide.py update:1149的bonus_data[0]为KeyError。实际Python2 rpc.revert_args对全领外层按日字典不递归恢复内层box，确实留下带__custom_type=box.box的普通字典；奖励行缓存取下标0复现同一错误。1010候选只对该已标记类型调用原生box.load().to_list()，同时修复新全领回调及旧缓存，不清奖励、不重抽、不修改已领状态。奖励对象与原列表继续兼容。原生真实奖励渲染14组、字典故障复现、修补渲染、新回包三形态与幂等安装通过，既有UI和展示回归通过。图形控件仍为夹具，未作Android入口验收。首次探针断言误把原生material字符串写成整数1，失败已保留，随后按原生结果修正测试。证据签到字典缓存修补原生验收.log、签到字典缓存既有回归.log、签到字典缓存展示回归.log；正式候选待验收发布。

全表再次核对38项非空，12已修复、17修复中、4无法修复、5待修复；四项跳过原因已逐格核实，历史清空反馈继续在案，B45仍为点不进去且D45修复中。

## 2026-10-10 入馆深层异常定位1009候选

1008仅保留前8层栈，真实KeyError停在widgets.update_panel_info调用奖励行set_info，尚未暴露最内层。1009调整为保留末尾12层，仍只含相对文件、函数、行号和异常类型，不发送局部变量、参数、缓存键或异常文本；幂等安装通过独立版本2与原函数引用，深20层异常原生测试确认保留末尾故障点。现仅定位候选，不宣称修复入口。七天原生reward_item.update与真实bonus_utils/data/gui_utils在已领/未领两状态14组执行通过；图形控件与update_rarity尚为夹具，因此不能作为整个入馆UI通过。证据签到原生奖励渲染初查.py/log、入馆末尾异常栈原生验收.log。未修改奖励或存档。


## 2026-10-10 字段差异及入馆定位1008已发布

211313私有候选全仓Go、vet、双平台构建和真实PG81 PASS/0 SKIP/5辅助PASS通过；session54451组合发布、独立游戏与原生核验均成功。游戏PID134719，SHA256为13fb643e4b6083a32b0559cbe1c25dd19c621d566210f4ec19c609d9d3bf11ce；热更1008，PID90522，SHA256为48493a30a256b936c6c53a0e6d3ff06462bb10c166dbfa6625e9c0325359c6b0。六正式报告及私有分发备份已同步，证据字段差异与入馆定位发布/。首窗口21:23:57共1552行，1次锁超时、另两类超时0，785角色预览后一轮5人变化写锁1.355秒；未跨21点全量结算，不宣称消除性能故障。

关键新证据：1008真实客户端上报beginner_init_error为KeyError，栈为init_show:783→update_select_tab:948→update_checkin_list:820→widgets.set_infos:279→update_list_items:291→update_panel_info:761；8层上限使更深奖励项栈未保留。已静态取证签到奖励列表实际异常原生.txt、签到奖励渲染原生.txt。七天原表预览全部产生存在的材料ID（501、13201、12203、10004、11、501、80002），见签到原生奖励预览初查.log；所以不能未经实际栈证据删除奖励或替换随机卡。仍需定位最内层KeyError，例如保留有界末尾栈/执行真实reward_item.update及缓存路径。1008并未修好入口。

21:28实时B45已改为“入馆指南点不进去，之前点他会导致屏幕变暗，现在是点不进去”。已更新F45明确承认入口未解决和真实KeyError，D45保持修复中，E列未改，回读保存成功；截图入馆指南打不开实际异常回填.png。继续全表与历史移除条目，不能把场景保护或释放遮罩等同完成修复。当前无运行中验收/发布session。


## 2026-10-10 周期字段差异写入候选，尚未发布

1007新进程21:03:14完整窗口6075行累计21次lock timeout、3次context deadline exceeded；21点两轮均变动780角色，写阶段分别14.067秒与13.864秒。此前首窗口只有3人变化的1.34秒不能代表全量跨期负载。证据入馆指南后续完整窗口.json/log。

候选social_patch.go将周期写阶段当前锁内重算结果与原始规范化进度逐顶层JSON字段比较，用state删除已消失键后合并变化键。原生大整数保留RawMessage，不经过浮点数；数组、null与omitempty删除分别保留语义。维持原avatars→progress锁顺序、xmin重读并发变化、跨角色同事务和失败整体回滚，未延长玩家超时、未拆分部分提交。非周期UpdateAllSocial保持整份写入。未知顶层兼容字段由差异写保留。仅减少写入传输/PG解析，仍不能宣称消除全服持锁。

新增本地测试验证大整数、数组清空、null、字段删除及取消；真实PG专项验证周期收据、无关资产、未知字段及重复幂等；隔离24行对照仅报告耗时，断言差异/整份结果一致和显著缩小载荷。现仍待候选验收及实测性能，不据此关闭表内登录/抽卡/战斗卡顿。


首次210711私有候选全仓Go、vet、双平台构建及真实PG81 PASS/0 SKIP/5辅助PASS通过；隔离24行整份写入838.878毫秒、差异670.537毫秒，单行载荷374036→22字节，单行差异生成7.793毫秒。顺序对照仅为此次样本，且写入耗时不含差异生成，不能当作生产端到端加速比例或解决14秒全服锁的证据。首次候选尚未发布，证据周期差异首次候选验收/。

同窗口捕获54次beginner_init_failed，1007未覆盖全部入馆初始化故障。新增独立幂等包装器，只上报异常类型及最多8层相对代码文件/函数/行号，不上报局部变量、入参或异常文本；保留原异常。热更候选升2026101008，入馆原生和既有UI回归通过，证据入馆异常定位候选原生验收.log、入馆定位既有UI回归.log。合并候选需重新验收，当前线上仍1007。

只读聚合统计进一步确认：当时785角色全部未解锁异步竞技、异步正积分为0、年兽记录非空为0；平均存档文本143476字节、磁盘24696字节。21点仍对780角色写入，后续应核对无参赛角色日历/空收据的批量更新与首次参赛基线，避免把“没参加”的全服档案反复改写；不能直接删历史账本或取消有参与者的奖励。证据周期参与范围只读统计.json，无玩家标识。

天体结界原表441002激活规则为is_teammate_or_self、标签441001、数量-1；原生选择和process_active_magic_field在两个同坐标我方结界时分别产生两个激活演出和两次计数，敌方/异标签/范围外被排除。test_tianti_fields_native.py实际执行原生选择与激活函数，但几何和伤害演出为夹具，不是完整伤害或Android验证；不关闭“两结界按一个算”反馈，不改技能平衡。证据天体技能原表.json、天体结界调度原生.txt、天体双结界原生调度验收.log。

完整引擎补充：test_tianti_fields_engine.py使用当前隔离CPython2原生server_battle，通过原生add_battle_magic_field在敌方同坐标布置两个441001结界；原生真实几何/阵营/标签选择同时保留两者，get_magic_field_action_play分别使敌方HP28198→27783→27368，各扣415。原场景另有既存结界，场上合计3个；没有把总数误写成仅2个。该测试绕过客户端连续施法创建路径，仍不证明Android反馈无效。原生宿主输出既有debug_logic/extensions可选模块提示，进程退出0且全部断言通过。证据天体双结界完整引擎验收.log；21:22已按B34描述核对并回填F34，D34待修复/E列未改，截图天体结界核查表格回填.png。54次入馆初始化失败涉及7角色，脱敏聚合入馆初始化失败范围.json。

## 2026-10-10 入馆指南展示生命周期修补1007已发布

原生init_show先显示UI外层并隐藏panel_beginner，0.1秒后的delay_show先读取当前scene.is_main_city_scene，再显示内容；当scene为None时，实际Python2闭包抛AttributeError，留下外层可见、内容隐藏。test_beginner_show_native.py用原生delay_show字节码复现；GUI、场景及调度是夹具，未宣称Android复现用户现场。基线证据入馆展示原生失败复现.log。

ui_callback_repair_script.py新增仅作用于入馆指南原生delay_show的生命周期保护：只在场景为None时按100毫秒重试，最多20次，恢复后执行原闭包；关闭或重新打开会递增代次，旧闭包不得复活旧UI。保持原add_callback句柄及无关定时器行为，未改任务、签到条件或奖励。超过等待上限关闭自身UI并记录beginner_show_scene_timeout；原生初始化或展示其他异常先关闭半开界面，再原样抛出，并分别记录beginner_init_failed/beginner_show_failed，不吞错伪装成功。安装有独立版本标记，不重复包裹。该修补只覆盖已复现的生命周期故障，不证明用户全部暗屏都由此触发。

候选原生验证覆盖缺场景失败基线、场景恢复、主城隐藏模型原动作、关闭/重开隔离、有界等待、安装幂等、无关回调及异常保留；既有test_ui_callback_repair_native回归通过。测试开发中Python2临时unbound method身份断言及缺__builtins__环境各失败一次，原日志已保留，修订的是测试环境。热更bridgeHotfixIndex及合并器升至2026101007，1.0.125/1.0.128冷启动和运行期正文均已合并、清单校验通过。204928私有候选全仓Go、vet、双平台构建通过，真实PG为79项PASS、0 SKIP及4项辅助PASS；session95151组合发布、独立游戏和原生运行时核验全部通过，现正式热更2026101007。游戏PID132083，SHA256为0ae779b31b8e196382ce5684e4f8e268d7f4a6384f34f42facf3a1e592360141；热更PID90261，SHA256为0f938b1f7f8f79a548e449738b44fb58dd4d819171ae48aefc253c75078ddd67。六份正式报告、私有分发备份已同步，归档入馆指南展示发布/。20:59:00采得新进程完整首窗口985行：购物和入场各1次lock timeout，另两类超时为0；779角色周期结算成功、3人变动写事务1.34秒。不能以独立发布核验通过覆盖这2次线上业务失败，登录/卡顿问题继续在案；证据入馆指南发布首窗口.json/log。候选证据入馆展示候选原生验收.log、入馆展示既有UI原生回归.log。MuMu实例选择仍待用户回复，不自行改用其他实例。

21:00腾讯表按描述复核后，B45入馆指南仍为本条反馈；F45已回填1007上线、原生/实库验收及Android待复测边界，D45改为修复中并确认已自动保存，E列未改。截图入馆指南上线表格回填.png。当前38条非空为12已修复、17修复中、4无法修复、5待修复；历史移除条目继续在案。

后续性能定位：social_all.go的UpdatePeriodicSocial在预读发现变化后，写阶段仍先锁全服avatars与avatar_progress，再执行结算和全服编码比较。本次首窗口两条业务锁超时与1.34秒周期写事务同秒，且玩家事务lock_timeout为1秒；这是明确竞争候选，尚未抓到这两次的实时阻塞链，不能写作已证实唯一根因。后续应研究保持跨角色结算原子性及防止旧快照覆盖的缩锁方案，不仅延长玩家超时。妄言新增原生证据为妄言布阵界面原生.txt、妄言场景入口原生.txt、妄言跳过布阵判定原生.txt：is_skip_edit依次检查副本固有skip_edit、活动自动与锁阵容组合、预设键记录，尚未定位用户本次触发分支，未放宽限制或改场景。

## 2026-10-10 关闭契印自动分解入口已发布

启程发布前旧进程126479的20:08—20:26完整窗口共25143行，remove_rune_auto_decompose_stars出现5次未实现拒绝。原生rune_mgr同名方法无业务参数，走call_server，线上参数仅callback_id；成功回调提示关闭并刷新card_rune_list。新增Service分派及removeRuneAutoDecomposeStars：必须Playing且只有有效回调，事务保存非nil空星级列表，先client_prop_changed同步rune_auto_decompose_stars=[]再返回整数RetSuccess。失败回调不伪造成功，不删除或分解任何契印；保留既有1至4星保存接口。nil表示未设置，空列表表示明确关闭，冷登录不能恢复默认开关。

本地TestDisableRuneDecomposePersistsExplicitEmpty与既有偏好回归通过，覆盖保存后关闭、重复关闭、非法参数、资产不变及重登空列表；新增TestPostgresDisableRuneDecomposeColdPersistence已通过实库。203310私有候选全仓Go、vet、双平台构建通过，session83363真实PG已成功结束：79项PASS、0 SKIP及4项辅助PASS，关闭冷存档专项0.57秒通过。session45040组合发布、独立游戏及原生运行时核验已通过；游戏PID130715，热更仍2026101006。正式报告和私有备份已同步，归档关闭自动分解发布/。20:43:58完整新PID首窗口1506行三类超时均0，776角色周期两轮成功，5人变动写锁1.366秒；只证明短窗口，不关闭性能反馈。证据关闭自动分解发布首窗口.json/log。候选游戏SHA256为3fed0184e6fbaa82dd1edc0ec3e8c33f192d2281c82d95f3c61dc68cb63e012a。test_rune_disable_native.py实际执行原生need_auto_decompose，启用指定星级和关闭后所有星级均不命中的断言通过，证据关闭自动分解原生验收.log；不冒充Android操作。本批为日志明确缺口，未找到腾讯表独立同名条目，不据此关闭其他契印反馈。证据关闭自动分解本地检查.log、契印关闭自动分解原生.txt。

战斗反馈初查：原生独立存储battle_type与从dungeons读取的dungeon_type；试炼荒庭活动3与异步竞技模式3重号，但尚未证明该分支造成怪物停滞。妄言活动207、原场景3003，未凭编号猜测修改场景或统一转换全部活动模式。入口、装配、初始化、加载原生反汇编及战斗模式使用位置.txt已保存。旧窗口仅1条DNS解析客户端异常，另2次布阵数量拒绝关联同一角色，但缺上行列表和当时关卡，不能等同2-1反馈；脱敏证据布阵数量拒绝关联.json。新增每日活跃红点原生判定同时检查每日、每周宝箱及完成未领任务；旧阈值回包修补已存在，不能把新反馈自动视为已解决。证据每日活跃红点初查.json。继续全表修复。


第一天探索任务补充：原表new_task105—108对应base_target20101，target_type=6且参数为空，与每日1013相同；现有finishFreeStageSite只在首领节点完成整张探索图时发该事件，单个普通节点不计通关次数。已扩展TestQQBugExploreDailyCompletionClaimAndResultPush，实际走入场、节点结算、全图完成后断言四条第一天任务均累计1，并通过receive_new_task_bonus领取105；专项通过，证据第一天探索任务实际链检查.log。这是本地实际Handle链，尚无该反馈的Android/角色历史证据，不关闭反馈。新增测试在203310快照之后，未包含于该发布的79项实库；本次未改探索生产逻辑。布阵拒绝角色现时只读投影为36700021、未开始，与20:09/20:12失败时间不同，不能据此判定当时关卡或放宽布阵上限。


20:43全表复核为38条非空：12已修复、16修复中、4无法修复、6待修复，快照关闭自动分解期间全表复核.json。42—44行已清空、45行改为入馆指南变暗；历史的试炼怪物、2-1读条、每日红点、第一天探索任务仍在案，不以清空关闭。本轮未改网页。MuMuManager官方info确认实例0/1/2均关闭，Android版本分别15/15/12，ADB无设备；已询问用户要启动哪个现有测试实例，等待选择，不重装或清数据。入馆指南原生init_show先显示外层并隐藏panel_beginner，再以0.1秒回调delay_show读取当前scene.is_main_city_scene后显示面板；scene缺失会在面板显示前中断。当时仅为生命周期风险；后续已执行原生闭包复现并形成1007候选，见顶部，仍缺该反馈Android现场证据。证据入馆指南初始化原生.txt及入馆指南延迟展示原生.txt。

## 2026-10-10 启程一次性保底已发布，历史与客户端验收待补

原表启程池3的1002—1011为第1—10抽一次性指定卡组规则，1007在第6抽提供7种典藏候选；现有抽卡路径未执行这十条规则。新增beginner_promise_catalog.json保留原表哈希与节点/卡种，beginner_promise.go在每抽选卡前检查未完成节点，按本次原池单卡权重筛选，最终卡入账后刷新全部已知规则。原生check_promise_rule、do_promise、get_onetime_promise_card、refresh_record由test_beginner_promise_native.py直接执行，节点、原候选权重、提前命中不重复触发及收据刷新均通过。客户端包没有发现原服务端调度调用者；本次依据可执行原生判定的count+1语义选择先判后刷，不宣称恢复了不可见的服务端源码。

单抽与十连共用PromiseCardsRecord和卡池累计数，与材料扣费、新卡及获取历史在同一进度事务提交；空候选或零权重拒绝整次交易。旧档缺失记录仅为尚未到期节点接续Count，已到期缺失记录维持未知，不从当前持卡推定历史命中、不补造已领取、不自动补偿。已抽完旧启程池的历史争议尚未解决；循环精装规则20033及共享规则1060不属于本次十条一次性规则实现，不将本次补丁宣称全卡池保底均已恢复。

本地TestBeginnerPromise覆盖分次单抽、连续十连、前六抽实际典藏、20抽上限、成本、记录、提前命中、旧档和失败回滚通过；实际Python2专项通过，证据启程保底本地检查.log、启程保底原生验收.log。新增真实PG专项要求五次单抽后新Store/Service/Connection冷登录，第六抽记录与实际典藏同时存在，继续十连及单抽至上限并验证失败请求不改存档。201437私有候选全仓Go、vet、Windows/Linux构建通过；缩短测试昵称后真实PG78项PASS、0 SKIP及4项辅助PASS，新启程冷登录专项1.51秒通过。游戏SHA256为4530f089b3f077be7624d9e62dc7650f64f9a6d0e2ba902791aa78eb7398f6ee，实库测试产物SHA256为c485018539729eef40902b05e26683ce823c00c51f1a106a264f9f525df46dfc。证据归档启程保底发布/；组合发布、独立游戏和原生运行时核验已通过，游戏PID128664，热更仍为2026101006。正式报告与私有分发备份已同步。20:27:06完整新进程首窗口1298行，三类超时均0，771角色两轮周期成功，其中3人变动写锁1.030秒；仅短窗口，不能据此关闭性能反馈。证据启程保底发布首窗口.json/log。仍缺Android点击及旧档历史补偿依据，腾讯表启程保留修复中；20:30回填已上线、78项实库和仍待Android/历史验收说明，回读和截图为启程上线表格回填.txt/png。


腾讯表20:21表尾复核：启程第41行已从待修复改为修复中，F41写明根因、验证中及历史不可追溯边界，回读通过，截图启程保底修复中回填.png。新增42行试炼荒庭怪物不会行动、43行打完2-1退到界面无限90%读条且大退后无副本、44行每日活跃领取红点一直在、45行活动第一天探索任务无法完成，均继续在案，未改已修复。原文与状态保存于启程修补表尾新增.json。首次实库新增用例因昵称超长在取名阶段失败，77项其他实库与4辅助通过；原始报告保存在启程保底首次实库失败/。已只缩短测试昵称并重建，游戏产物不变，实库重跑session49031已成功结束，后续发布见上文。

## 2026-10-10 幻书收集榜已发布，启动锁超时仍在案

日志query_rank_list参数[2,0]对应原表rank[2]幻书收集榜，非山海活动；[6,0]为学会活跃榜。候选在既有ActivityRankAccounts接口内实现类型2，子榜只能为0，原生回推on_query_rank_list("2&0", infos)，score为单元素整数列表；列表上限100。按同服真实获取记录和旧档当前实际拥有卡去重，使用已有profile_catalog的82种可统计幻书，排除disable/not_in_stat、素材、未知及剧情变体。保留已分解但存在历史的卡；旧版已删除且从未记录的历史不凭空恢复。同分采用既有稳定OID次序，原生收集榜显示连续名次。无玩家或无合格卡返回真实空榜。

数据库单条只读SQL在同一快照中计算全部分数、排名和个人名次，再限制页面；读取公开资料时保留已选头像框，已清除自定义头像为空。只传公开资料/分数，不把全服完整存档取回应用层，无行锁、无资产修改、不触发活动开放校验。内存与PG使用相同有效卡目录；查询必须在Playing状态。

新增本地回归覆盖重复卡、历史卡、素材/未知排除、105角色分页和实际Handle的100条上限；PG专项覆盖同服/异服、冷Store、持锁期间可查询、个人名次不受页面截断、同分稳定、头像和版本不变。首次旧测试仍要求类型2必须拒绝，失败日志保存在收集榜旧拒绝断言失败.log；已替换为真实回包与计数断言，并保留类型6未实现拒绝。Rank/Profile本地检查通过。test_collection_rank_native.py实际执行本版Python2图鉴计数与榜单接收函数，证实已分解0份历史仍计种、体验/素材排除、同分连续名次、列表更新及空榜；结果收集榜原生验收.log，不能冒充Android点击。195539私有候选全仓Go、vet和双平台构建通过，77项真实PG、0 SKIP及4项辅助检查通过，组合发布和独立游戏核验通过，游戏PID126479，SHA256为1a65564e1a271cc55ed3393ddbbe524b6b734212b6064b0ae2e13ce3e0b57b60，热更仍为2026101006。首次原生核验因运行日志错误失败，原日志保留；后续原生环境、运行时、哈希及最近100行复核通过，不抹去首次失败。至20:08:19完整新进程窗口4620行有1次锁超时、0次请求期限超时和0次事务空闲超时：20:06:04副本705战斗观察检查点因锁等待延后，同秒周期事务处理768角色、变动4人、持锁1.499秒成功；时间相邻不是已证明的唯一因果。性能与玩法反馈继续修复中。证据归档out/evidence/qq-bugs-20261010/幻书收集榜发布/及收集榜发布首窗口.json/log，正式报告和私有分发备份已同步。本批没有表内单独收集榜行，不关闭其他BUG，也无Android点击验收。线上只读执行计划返回100条，执行572.976毫秒，无行锁，证据收集榜线上只读执行计划.json。

另外核对了broadcast_random_cards为卡UUID列表的无回调上行，尚缺广播接收/频道契约，未用空成功吞掉；unlock_mid_age与set_mid_age_cards属于中世纪活动（不是展厅），后者走call_server回调并提交chapter_2_cards，前者无参、下行on_unlock_mid_age(Bool)。中世纪掉落队列、派驻加成及收取闭环未实现，不以仅补入口宣称活动完成。学会活跃榜缺真实活跃积分来源，继续记录缺口，不伪造排行。原生取证文件集中于qq-bugs-20261010既有证据目录。 最新腾讯表37条：12已修复、15修复中、4无法修复、6待修复。新40行妄言无忌战前不能调阵容且场景异常、新41行启程前十连无典藏却显示已领取，均纳入；历史被清空反馈继续在案。启程池为cards_random[3]，成本材料11×138、上限20，promise_rules含1002—1011；其中1007规定onetime_count=6及7张固定候选，全部rarity=4典藏。现有handleGachaContract除池1首次1001外未执行这些一次性规则，候选池3的guarantee=0，因此仅一般权重不能保证第6抽出典藏。原表证据启程十连保底原表.json，后续优先修此确定缺口。抽卡保底记录原生.txt已确认do_promise按原池权重过滤候选再抽样；check_promise_rule的稀有度、位置和指定卡种条件为或关系，稀有度为相等判断。一次性规则在count+1达到节点且finish_count不足时触发，refresh_record每抽递增计数、命中候选才递增完成次数，节点届满有原生收据完成规则。仍需核实调用者的选卡与刷新顺序后实现，这是收集榜发布时的历史状态；后续启程一次性保底已实现并发布，见上文，不从文案捏造概率。只读范围核查发现1个角色启程计数至少6次；其已有典藏来源未知，不能据此判为保底兑现或自动补偿。现有存档没有逐抽卡池收据，历史追溯边界保留，见启程保底线上只读范围.json。

## 2026-10-10 学会局部事务已发布，继续全表修复

19:06—19:19旧进程窗口的学会创建超时已定位为leagueRPC和雅努斯起战仍调用全服写事务。本批新增LeagueTransaction/LeagueAccounts与dbstore.UpdateLeagueSocial：写操作先获取当前schema的学会专属事务锁，串行维护同服名称和全局UID，再按OID顺序只锁本人、当前/目标学会主档及显式审批或任命目标；全服只读取关系与公开资料投影，完整背包和战斗存档仅加载上述角色。只读搜索/详情不获取行锁或专属锁。展示投影不能返回给连接替换完整存档，回调修改未锁定角色会使整笔事务回滚。沿用5秒请求期限、1秒锁等待和avatars先于progress的顺序，不修改创建成本、冷却、原生拒绝码或数据库全局配置。

新增真实PG专项覆盖无关角色持锁时学会仍能完成、学会持锁时无关普通写入仍能完成、三角色回调失败/越界修改整体回滚、只读零写锁和SQL投影、同名并发仅一次成功且仅成功者扣费、异名并发UID唯一。既有学会审批、退会、移交、雅努斯和冷登录验收继续保留。193030私有候选全仓Go、vet及Windows/Linux正式构建通过；真实PG共76项PASS、0 SKIP，另4项辅助PASS。新增两项学会实库专项及既有审批、移交退会、雅努斯冷恢复均通过。组合发布、独立游戏与原生运行时核验均通过，游戏PID124470，SHA256 `47463ab9524a2cbc30e7ad24f5d2804a6fb50a819c5206c8f2b8a3f64b8ada13`，热更维持1006。归档`out/evidence/qq-bugs-20261010/学会局部事务发布/`，正式报告和私有分发备份已同步。首个完整新PID窗口1153行中三类超时均0，764角色两轮周期成功，其中写锁事务1.59秒、3人变动；这是短窗口，不能宣称高并发或全部卡顿反馈通过。证据`学会局部事务首窗口.json`。截至19:43:13新PID完整窗口5574行仍三类超时0，另存`学会局部事务完整窗口.json`。腾讯表B28登录、B32绿钥匙、B33十连均已补回本批修补与待客户端复测说明；D32/D33由待修复改为修复中，D28维持修复中，未改测试列或提前关闭。回读及截图为`学会修补表格回填.json/png`。生产只读EXPLAIN实测763角色学会投影执行184.016毫秒、规划2.108毫秒，无行锁；证据学会投影线上只读执行计划.json。旧窗口确认绿钥匙池2十连两次锁超时，另有query_rank_list参数[2,0]及[6,0]未实现，脱敏分类保存在学会修补相关旧日志分类.json；不能据此关闭全部抽卡故障。原表状态不提前关闭，用户指定的四项无法修复维持。

## 2026-10-10 七日头像框同步已发布，学会全服锁待修

19:06实时表B5:B43共39条：12已修复、13修复中、4按用户要求无法修复、10待修复；新增第42行“探索和活动剧情关卡进入战斗界面后无法交互”、第43行“登录7天领取出错”，均纳入本轮，不按旧行号回填。快照`腾讯表1906复核.json`。19:06日志另有query_rank_list类型拒绝及query_pass_team_rank未实现，已取证原生通关队伍回包结构；没有据此伪造历史队伍或推断它们就是表中战斗故障的根因。

七日原表链为check_in_bonus[1]第7天→bonus220007→material80002（type9，target1004，永久头像框）。现有单领与一键领取能将1004持久入OwnedHeadBox，却仅下发材料、卡牌、契印、体力、知识和签到记录，遗漏owned_head_box，客户端领取成功时仍持有旧头像框状态。新增实际Handle回归先复现两条路径均失败（`七日头像框修复前.log`），候选在成功领奖回调前补同步原生owned_head_box；不改变七次实际登录条件、原奖励、持久收据和重复领取拒绝。

新增验证覆盖七次日期推进、单领、一键领、帧状态必须先于回调及重复领取；真实PG用例逐日使用新服务/连接登录，领取后再次冷登录核对永久框1004、count=7和仅第七日收据，并检查重复请求明确拒绝。191304正式候选全仓Go、vet、双平台构建通过；74项真实PG零SKIP及4项辅助通过，组合发布和独立游戏、热更、原生核验均通过。游戏PID122419，SHA256 `4104c1440854fd32171d950bedd964ed8a65da919d736beb775919fc48435595`，热更仍1006；归档`七日头像框发布/`，正式报告与私有备份已同步。首窗口952行中周期预览761人，两轮成功，写事务1.28秒，三类超时均0。该遗漏尚未证明是用户“领取出错”的唯一根因，且原反馈已从原43行清空，本轮未改表格状态。无Android实例的验证边界保持。

回填前再次读取发现协作表尾部变化：新39行为“藏书竞斗不能加载大退重进还在这个界面”，40—44行为空；此前入馆指南、中间位契印、3-7、无法交互和七日领取已不在旧位置。证据`七日回填前行变更.json`。它们仍是历史在案条目，不能按表中清空推断已解决；本轮不按旧43行写入状态。藏书竞斗反馈尚无角色标识，不能据该文字重置任意玩家存档。

优先后续：19:06—19:19:32旧PID120329较长窗口有83条锁超时、21条请求期限超时，create_new_league失败16次；同时波及抽卡广播、家具、任务、锁技能、战斗和重连。源码leagueRPC所有方法在验证等级、参数、名称和费用前调用UpdateAllSocial，仍锁全服并读取完整进度；周期预览/并行编码不覆盖该非周期路径。证据`学会创建超时定位.json`及followup-191932/events.log。应为学会操作收窄读取/锁定范围，保证同服名称唯一、全局学会UID唯一、扣费及成员/主档原子性；不能仅通过延长请求超时或关闭学会功能掩盖问题。该次七日发布当时尚无学会修补，不能把当时首窗口零超时当作学会锁已解决。

## 2026-10-10 周期存档并行编码发布与首窗口

上一版本周期结算成功，但两轮全服锁分别持续4.06与3.54秒，仍触发5条前台锁超时。本轮先以既有446份存档约48.7MB规范化JSON做本地测量：串行编码447—488毫秒，四路编码125—153毫秒，独立深比较749—763毫秒，故不采用深比较替代。远端有8个逻辑核，游戏服务未设置CPU配额限制；本地比例不能直接证明远端时延改善。

候选只并行周期纯计算结束后的JSON编码与差异比较，最多4个工作协程且不超过GOMAXPROCS；每个协程独占结果槽位，不修改Progress，全部退出后才能组装数据库批次及修改资料。只保留变化行编码结果，未变化行即释放，避免另存一份全服JSON。非周期操作保持单路编码；avatars→avatar_progress锁顺序、版本重读、全量重算与单事务原子提交不变。任何编码错误或上下文取消都拒绝整批输出并回滚，不部分入账。

新增回归覆盖32角色并行结果顺序、嵌套进度变动、未变动不写入、取消及单行编码错误拒绝整批；真实PG新增8角色锁内编码故障回滚测试。证据存于`out/evidence/qq-bugs-20261010/存档比较基准.log`及周期并行编码日志。185418正式候选已通过全仓Go、vet、双平台构建及73项真实PG（零SKIP）和4项辅助验收，组合发布、独立游戏与原生核验均通过。游戏PID120329，SHA256 `f70d6aff3576431036906741a775eae3866db87208b39d1bccc63d0f0b85694e`；客户端仍为1006。证据归档`out/evidence/qq-bugs-20261010/周期并行编码发布/`，正式报告及私有备份已同步。完整新进程首窗口1271行：活动预览759人17.27秒无需加锁，PVP预览759人16.15秒、锁内1.16秒、2人变动且成功；锁超时、上下文超时、事务空闲超时均0。截至19:04:55完整新进程窗口1962行仍为上述三类超时0，证据`周期并行完整窗口.json`。它是短窗口且变动人数与前版不同，不表示所有高并发性能或卡顿反馈已通过。另已观察到18:50:35副本707胜利结算和客户端完成回调，证明该次3-7可完成，但不能证明调整阵容入口正常，仍不关闭此反馈。新增只读工具`out/tools/observe_periodic_release.py --expected-sha <64位候选哈希> --since "2026-10-10 HH:MM:SS" --label <证据名>`：先核运行进程哈希，再采该PID完整指定窗口，分别保存原始日志与中文统计；不以最近100行替代启动窗口。

## 2026-10-10 后台独占版已发布，启动持锁问题继续修复

116481新进程日志明确：18:33:00第一次全服预读在读取755个角色后，被PostgreSQL idle-in-transaction timeout终止；随后玩家PVP相关请求反复进入refreshPvpAwards，每次5秒解析约200—250个角色后因请求期限取消，18:35前已16条上下文超时。过去仅在总线忙时跳过，后台失败后总线空闲仍会被前台接管，故仅优化异步候选查询不能消除此链。

新候选增加StartPeriodicMaintenance(ctx)，正式main在监听请求前调用，以原子标记一次性确立后台所有权；后台失败、取消或总线暂时空闲时，带玩家连接的refreshPvpAwards均不再启动全服事务。独立运行且未启动后台的测试服务保留同步结算入口，后台tryRefreshPvpAwards仍执行原计算并保留错误重试。周期存储事务只在本事务将idle_in_transaction_session_timeout设为120秒，与已有statement_timeout及外层总期限一致；只读快照加载完即提交，再执行纯计算，避免计算期间占着数据库事务。全局数据库配置不变。

新增回归验证后台失败后连续玩家调用零全服结算、后台错误不吞掉且可重试；真实PG回归新增预览回调前已提交的断言，继续覆盖并发变更、xmin、新角色及整体回滚。183724候选全仓Go、vet、双平台构建通过，真实PG72项PASS、零SKIP及3项辅助通过，已完成组合发布。游戏PID118513，SHA256 `d863d53177aac6eaad45924c15092c1d4e449f9db416313d148bb2e02c871b17`；热更内容仍为1006。证据归档`out/evidence/qq-bugs-20261010/后台独占发布/`，四份正式报告与私有分发备份已同步。

18:46:52与18:47:14两轮周期结算实际成功，各预读757个角色，耗时19.68/18.73秒，锁事务耗时4.06/3.54秒，变动2/4名角色。至18:49:48日志4277行，事务空闲超时0、上下文超时0，未再出现前台每5秒的全服扫描；但启动两次加锁窗口仍有5条锁超时，涉及战斗结果、退出、707检查点、引导及加载完成。独立原生首次核验因此失败，原文已保留；随后最近100行核验通过，不能用后续通过抹去启动失败。18:47:15以后3552行无上述三类超时，只是短窗口证据，性能条目继续修复中。下一步需缩短全服写事务内计算和JSON序列化时间，并保留并发资产及周期幂等语义。

表新增“3-7无法调整阵容后开战，只能直接开战”（读取时第41行），已纳入全量跟踪；原表映射707/10238，未设置skip_edit，disable_team_preset=0，无固定出战角色。当前线上确有707检查点锁超时，但尚不能将它定为调整阵容失败的唯一根因，未回填已修复。新增条目及原表证据保存在同目录腾讯表新增3-7.json、3-7战斗原表.txt。中间位副套装规则选择仍待用户回复，未修改概率。


## 2026-10-10 异步重进发布完成与新增中间位反馈

182513候选已组合发布并通过游戏、独立热更与原生运行时核验：游戏PID116481，SHA256 `c1d1feabf03b29a7dccfb73c81bfe43a13db093bfc83ac711ef7672f706a0900`；热更PID88343，仍为1006且内容SHA256 `4c7e5245ae0803849a434af51fc933515869dffebc40b35ff344023bcdc3d5d3`。72项真实PG零跳过及3辅助通过，归档`out/evidence/qq-bugs-20261010/异步重进发布/`，正式报告与私有分发备份已同步。已保存候选直接重进、未解锁和刷新冷却拒绝均不再扫描全服。修补前统计755份进度约106.5MB文本，异步解锁及防守阵容计数均为0；该样本说明未解锁的入场请求也曾先读取全量。它不证明其他时刻无合格对手。

当前表B5:B40共36条（快照`腾讯表1833复核.json`），新39行入馆指南点开变暗、40行中间位契印仅同套已继续纳入。中间位根因已经定位：568个rune_drops中564个副套装权重全零；shopWeightedSuit(optional=true)返回0，GenerateRuneWithMarks对位置1将ExtraSuit回填为Suit，因此此路径只能同套。4个明确副套装权重仍应保留。现有主套装空池采用已批准的运营池，未找到副套装独立抽取概率证据。已向用户提出具体选择：从现有批准池主副独立抽取（允许相同），或按用户选择说明原因标无法修复。尚未修改概率或历史契印，不提前执行待定规则。

ADB仍无设备，127.0.0.1:7555连接失败；仅有MuMu后台服务不等于可用Android实例。18:22:42—18:30:48周期预读版本窗口11905行，锁超时0，上下文超时4（2次异步入场、1在线租约、1房间读取），不宣称全服无故障。用户指定跳过的4项仍保留无法修复，其他未通过反馈不回填已修复。


## 2026-10-10 周期预读发布完成及异步重进候选

技能锁补充证据：`out/tools/test_qq_skill_habit_native.py`实际执行本版Python2回推和AI选技能函数，技能1/2/3/自动、普通攻击回退、新战斗对象及原生异步全自动例外通过。它不包含Android点击、动画或实际重连装配，不能独自关闭表中两条技能锁反馈。回归输出在`out/evidence/qq-bugs-20261010/技能锁原生执行回归.log`。

181544候选已组合发布并完成游戏、独立热更与原生运行时核验。游戏PID114730，SHA256 `a99d19cd966f708127218fd5811b380bd1e32713f00514e9f05071e71aba29e1`；热更PID88140，热更内容仍为1006、SHA256 `4c7e5245ae0803849a434af51fc933515869dffebc40b35ff344023bcdc3d5d3`。72项真实PG零跳过及3辅助通过，证据`out/evidence/qq-bugs-20261010/周期预读发布/`；正式报告及私有分发备份已同步。

18:22:42—18:24:07新进程日志无锁超时和上下文超时，但没有出现有变动周期结算的耗时日志，不能据此声称已证明高峰持锁改善。进程86秒时RSS约262MiB，CPU为进程启动累计91.4%，不能当作瞬时稳态CPU。保留启动窗口日志，性能条目暂不关闭。后续源码给无变化预览也补汇总日志，便于区分无需加锁与结算尚未完成。

另据18:10:19真实enter_asyn_pvp超时继续定位：原重进入口先读取最多1000名角色完整存档，然后才在自己的行锁内发现已有候选并直接复用。候选修补先读取自身当前行，验证解锁、时钟、刷新冷却与已有列表；仅确需新候选时回滚本次探测，在锁外查询，再在锁内重新校验和生成。对手查询失败不以旧连接快照或机器人代替；新对手规则不变。新增TestAsyncPvpSavedCandidatesAvoidGlobalRead验证首次真实读取、重进零全服读取、冷却拒绝零全服读取、存档清空后读取失败不返回旧列表、恢复后可生成。专项、全仓Go和vet通过；182513私有候选双平台构建及72项真实PG零跳过、3辅助通过，正在组合发布，不包含于181544产物。正式候选为`%LOCALAPPDATA%/hs-server/release-workspaces/20261010-182513`，尚待独立核验。


## 2026-10-10 周期结算锁内全量存档重读修补候选

1006启动日志确认51条锁超时，后续18:03:20—18:12窗口没有锁超时但仍有一次异步竞技超时；性能反馈不能关闭。当前补丁保留avatars→avatar_progress全局顺序行锁及单事务原子结算，只调整完整存档读取位置。周期只读预览保存独立的结算前Progress、规范化原文、revision及PostgreSQL xmin；进入写事务后重新读取并锁定最新角色资料及全体进度版本，版本变化/新角色才重新查询完整state。未变化的原始进度直接复用，再执行原结算回调；预览结果绝不直接入账。未递增业务revision的维护写入仍由xmin识别，当前等级/昵称使用锁内资料。

缓存容器与预览修改完全独立；无变化仍不加锁。非周期UpdateAllSocial保持原完整读取方式。新增中文日志只记录角色总数、预读耗时、加锁事务耗时及成功标记，便于上线量化；仍需发布后验证持锁时间和峰值内存，不能仅凭代码声称性能问题已修复。

候选目录`%LOCALAPPDATA%/hs-server/release-workspaces/20261010-181544`。全仓Go、vet与Windows/Linux构建通过；新增真实PG用例覆盖嵌套容器隔离、并发资料和未递增revision的进度更新、新角色加入，以及锁内只传输版本、仅重读变化和新增角色。最终真实PG72项PASS、零SKIP，另3项辅助PASS；新增预读并发回归及原周期回归均通过。已启动组合发布，尚待独立核验和线上持锁观测。本次仅服务端改动，客户端热更仍为2026101006；不回填性能条目。


## 2026-10-10 用户指定跳过证据不足条目

用户针对补充复现角色、活动节点及时间的请求明确回复“跳过，写明原因，选择无法修复”。据此将实时表中下列4项标为无法修复，并在F列处理说明中逐项写明当前证据不足；这属于用户接受的跳过项，不计入已修复。其他有明确根因或仍可独立推进的反馈继续修复。

- 所有的活动，剧情卡死报错。：历史诊断（本轮恢复全量核查，不再跳过）：反馈未指明具体活动、剧情节点、复现角色和时间；缺少可对应的异常堆栈，无法定位“所有活动剧情卡死”的共同根因。
- 进入同桌的你，活动后会有一个选择男女角色的环节，男角色怎么样不知道，我选了女角色，然后游戏卡住了：历史诊断（本轮恢复全量核查，不再跳过）：已检查同桌的你原生性别选择及剧情奖励入口，尚无对应角色、复现时间和报错堆栈，无法确认选择女角色后停在哪个节点。
- 镇灵台—西关卡难度二90%概率卡死：历史诊断（本轮恢复全量核查，不再跳过）：尚无镇灵台—西难度二对应角色、复现时间及战斗日志，无法定位高概率卡死的触发条件。
- 时空回廊无法进入战斗：历史诊断（本轮恢复全量核查，不再跳过）：缺少时空回廊具体层数、角色、复现时间及入场返回信息，无法区分关卡条件拒绝与客户端加载故障。

本次已回读B/D/F验证，证据为`out/evidence/qq-bugs-20261010/按用户要求跳过条目.json`及同目录截图。实时表原39—42行内容已被其他协作者移除，38行现为抽卡后角色界面黑屏；旧行号不得继续回填，历史反馈仍保留在案。


## 2026-10-10 1006正式发布与继续修复边界

1006已组合发布，游戏与独立热更、原生运行时复核通过。游戏PID112184，热更PID87894；游戏SHA256 `df2217dcacefcdc3dc26a4f85be222353f030428e995f2c5bbbbf261c17fd826`，正式热更SHA256 `4c7e5245ae0803849a434af51fc933515869dffebc40b35ff344023bcdc3d5d3`。最终175423隔离候选全仓Go、vet、双平台构建、真实PostgreSQL71项PASS零SKIP及3项辅助通过，空阵容原生Python2回归通过。正式产物、首次失败和后续复核归档在`out/evidence/qq-bugs-20261010/1006发布/`，私有分发备份已按现网哈希同步。

本批修复普通探索全零阵容锁队卡死、同步排行头像字段遗漏、好友搜索缺_id、活动条件13日期门槛。腾讯表按实时描述核对“探索忘记上角色就锁阵容自动会一直卡死报错”，D12已改为已修复并确认自动保存；测试列保持待测试，本机没有Android设备。回填截图`1006空阵容状态回填.png`。

首次原生独立核验失败原因是启动期间多条数据库锁超时，涉及领取新任务奖励、签到、绿钥匙抽卡、战斗结算等。失败原文保留；随后采样15个业务会话均idle、无阻塞，独立复核通过。这不关闭启动及并发性能问题，不以复核窗口零错误覆盖启动失败。最新38条当前反馈及历史已移除条目继续跟踪，新馆主补给/通关十连、试炼荒庭及妄言无忌已纳入；全量目标尚未完成。


## 2026-10-10 1006修复候选与新增原生证据

上一轮完成1005发布及4条状态回填，本轮继续全量目标，尚未关闭剩余反馈。1006候选包含：

- 普通探索空出战阵容：原生`battle_cards.check`接受全零槽位，锁队自动随后可空队开战。界面热修在调用原生开战前检测出战卡、原生固定NPC和剧情卡；普通探索三者皆空时只显示一次原生布阵页，补齐后继续原开战。援护不冒充出战卡，副本17教学固定角色保留。Python2实际原生函数与热修回归通过，证据`空阵容原生回归.log`。
- 同步排行榜：SQL和内存排行遗漏头像/头像框/自定义头像，通用UI读`head_box_info[0]`崩溃。补查询投影和下发，尊重已选头像及清空自定义头像状态，不读取整份JSONB背包；新增真实PG冷查询及回退回归。
- 好友搜索：原生213行按`info['_id']`构造friend，原返回只有eid。搜索列表补ObjectID类型的_id，其他资料接口不变。新日志已复现缺字段，真实Handle非空搜索回归通过。
- 活动日期门槛：新日志两次进入21000104落雪之森·旅程被未知条件拒绝；原生条件13为活动天数不小于目标。补解析`210#3`，使用显式服务端时钟和已发布客户端相同的北京时间日历日/常驻封顶，保留601主线前置、未来未开/过期拒绝。15个原表副本使用该条件；普通/活动及任务塔入场均传递时钟，未配置时钟不隐式放行。

最终候选在`%LOCALAPPDATA%/hs-server/release-workspaces/20261010-175423`，全仓Go和vet通过；最终构建/真实PG正在执行，尚未发布，不回填上述表格状态。上一份174912候选的71项实库通过不替代本最终候选验收。35条当前表及被移除的历史反馈仍全部跟踪。后续应继续落实家具、活动剧情、试炼荒亭、时空回廊、镇灵台、锁技能和性能问题，不能把没有新异常日志当成已修复。

## 2026-10-10 腾讯表红点与同步对决冷重连契约

日常/周常奖励数据库收据继续保存原表奖励行ID；`basicRewardProperties`仅下发对应`active_scale`阈值到`daily_bonus_active`、`weekly_bonus_active`。本版`mission_daily.check_mission_state`按阈值判断已领，原ID投影导致红点不消失。旧存档无需资产迁移，四连接竞争领取与冷登录去重纳入真实PG基础领奖回归。

`revival_restore_pvp_enemy(infos)`是新增战斗桥下行方法，仅使用服务端已冻结的机器人/真人快照调用原生`Avatar.set_pvp_enemy_info`。顺序为装桥→建立战斗→恢复对手→加载队伍→prepare，修复冷重连跳过选队UI后`loading_pvp.set_sync_pvp_cards`读取缺失`enemy_cards`。不重新匹配，不重新触发选队计时器，不改变技能和计分；单原生权威不变。`out/tools/test_qq_pvp_reconnect.py`在本版Python2执行原生初始化及加载函数，先复现缺字段，再验证恢复和幂等。

卡池计时器revision2检查实际方法标记，模块重建后恢复有界刷新；相同revision只保留一条检查链。此项不改池概率、消耗或开池资格。1005正式构建、全仓、真实PG70项零跳过、组合发布及独立复核已通过。首次启动锁超时失败证据仍保留；详细发布状态及客户端未验收边界见HANDOFF最新章节。

## 2026-10-10 1004推荐好友与实际换装候选

好友推荐增加FriendSummaryOnly只读投影：SQL保留等级、头像选择、头像清除、同步竞技积分与好友/黑名单，省去卡牌、契印、物品、邮件和其他玩法存档的传输及反序列化。候选仍同服UID对应OID顺序窗口1000、最多20条，本人/已有好友/双向黑名单/好友上限过滤保持原契约；摘要不能写回完整存档。实际PG检查摘要没有Cards/Materials，且头像、积分、黑名单仍保留。

腾讯表22“升格解锁装帧”和25“实际换上装帧”分开验收；1002仅前者与语音/日常修补通过。1004由已授权协调聊天补change_card_dress，最终结果须以该版本正式回归与发布证据为准，不能把早期解锁修补当作实际换装已通过。

## 2026-10-10 1002上线与1003周期锁等待修补

1002已组合发布并独立核验通过：游戏PID102170，热更PID86708，游戏SHA256 `854f4f46ed80dc77cdfe90fba5f42a8c22cd3f369dd6e1098a8909aa1ef1a6f2`，正式热更SHA256 `d013a00c10f65e4ab72d29a19c0e68111999c0206ea44d43b8617cac3accf14b`。默认/正式全仓、vet、构建、真实PG68 PASS零SKIP及3辅助通过；证据repair1010/release1002。备份为两项目releases/completion-20261010-150356-930580，回滚须按组合报告逐文件哈希恢复游戏和热更，不能单独恢复二进制或动数据库。

1002新PID首分钟1077行无客户端异常，但53次业务锁超时及真人租约/房间等待，不能关闭启动性能问题。15:08:00—15:09:29窗口1107行无客户端异常和战斗拒绝，仍有推荐好友超时1次、change_card_dress未实现3次及引导门槛拒绝1次。1003针对周期无变化时仍全服持锁：先以REPEATABLE READ只读事务读取一致快照并计算，无变化时直接完成；有变化丢弃预览，原FOR UPDATE事务重新读取、计算和原子保存。不把预览存档提交，不改变奖励规则。仅活动/PVP周期纯计算采用此路径，管理/学会社交操作保持单次事务语义。真实PG新增并发测试验证预览不阻塞玩家、重算不覆盖并发资产、锁内错误全部回滚。1003正在正式验收，尚未发布。

## 2026-10-10 日志修复统一验收与第二轮复核

第一轮runtime2026101001已组合发布：游戏PID98348、热更PID86318，游戏SHA256 `e808e14b2878694e189f221e026a6fb619d9ac8b77c1198c7e9755cb95493999`，正式热更SHA256 `aa1a3661369cf68d6d59d5690aaf35a8db236b8107fef7386e7ab03da30531ce`。默认/正式规则全仓、vet、双平台构建、真实PostgreSQL68项PASS零SKIP及3项辅助PASS、组合发布和独立远端核验通过。第一轮归档在out/evidence/player-bugs-20261009/repair1010/release1001。

第一轮新PID日志14:15:17—14:23:18共5581行，11次登录刷新；仍出现13条客户端异常、27条战斗事件拒绝和启动期数据库锁/语句超时，不能记录为零故障。证据followup-142319。第二轮runtime2026101002修补实际战斗类装配方法、签到旧奖励盒缓存、抽卡random_id投影和聚焦下行；专项已过，最终流水线执行中，尚未发布。具体契约见SERVER.md本日章节。

PVP继续采用服务端单原生权威，普通副本保持Android原生计算。HTTP旧日志缺目标、current/input不一致未复现、启动周期全服事务等待、尚未实现玩法与高层装备部分规则、腾讯55条客户端逐项验收仍不属于已关闭项。本机ADB无设备，本轮没有Android/MuMu实机验收。旧日期章节仅为历史，不代表当前版本。

## 2026-10-10 日志修复接口与热更契约（runtime2026101001、1002）

本轮原始日志为 `out/evidence/player-bugs-20261009/followup-105929/events.log`，分类见同目录 `日志分类.json`：同一旧PID71659共256645行，模型实体id异常34次、成就空控件6次、中文std::string异常2次，签到KeyError0/倍数费用空控件/好友列表越界各1次；另有HTTP错误31次及输入单位不一致2次。日志计数受客户端诊断限流影响，不代表全服实际发生总数。

### 战斗桥和客户端异常

普通桥与原生权威竞技桥在调用原生battle_fighting期间缓存同步产生的回合/输入事件及当时快照。原生成功返回后先发送started，再按原序发送缓存；原生抛错仍抛出，不假报成功或消耗事件序号。普通桥保持revision15并新增开战顺序标记；共享桥检测底层升级后重新包裹现行钩子；竞技工厂revision2升级时取回原生工厂，避免旧闭包重新选择旧子类。握手仍为版本1，不修改战斗规则和胜负。

`battle_performance.create_entity` 模型替换后重新从entities[eid]取得新对象，再沿用原生朝向、属性、回调和eid断言；修复原生销毁旧模型后仍访问旧id。成就/好友/费用UI在控件已经销毁时跳过旧页面绘制，真实奖励仍展示；签到全领每日box先to_list再缓存，避免把奖励对象当列表索引。基础奖励失败消息转UTF-8字节，适配Python2/C++控件。补丁幂等，按需加载页面每5秒检查一次安装。

`export_battle_bridge.py` 现在以现行内嵌脚本为维护源，只更新版本头及SHA；不得再从旧_ref导出覆盖修复。热更同时包含启动和运行期闭包，私有正式地址由prepare_private_release读取本机授权备份，与示例配置分离。

### 原生RPC与持久化

| 入口 | 原生契约与处理 |
| --- | --- |
| search_friend | 参数为回调、昵称、UID、服务器、有效标志；纠正旧服务器/昵称顺序 |
| apply_friend | 接受原生默认hostnum=0、apply_src空字符串；来源长度上限100，同时兼容旧整数夹具。仍核验真实目标、留言、关系和黑名单 |
| set_defence_cards | 回调加卡槽列表，保留0/None空槽位置；至少一张本人卡，重复/外来卡/非法形状拒绝，原归属和解锁校验保留 |
| refuse_challenge | 原生忙碌发送回调、None、1；读取本人持久待处理邀请，在单事务移除双方邀请并写拒绝通知；重放不重复通知，其他空目标原因拒绝 |
| set_auto_battle_habit / set_auto_support_habit | 写本人Card.Habit；下行sync_battle_method使用ObjId和card.card_habit对象，冷登录从每卡habit_mgr恢复，默认自动100、助战true |
| set_card_favorites | 回调、本人card_id、Bool；存储CardFavorites并推送原生card_common_mgr.is_favorite，再回单整数ret |
| set_game_score_flag | taptap/ios及非负来源编号，持久Str2IntListDict并去重；每渠道最多64来源，仅用于避免重复提示，不作为评分或发奖证明 |
| set_focus_target | 目标字符串和整数类型；1002改为sync_battle_method(set_focus_target, 操作者ObjectID, 目标, 类型)，由原生player_input继续验活和禁选；结束后迟到输入忽略，权威竞技输入链未接线仍明确拒绝 |
| request_chat_channel | 无参，恢复已存有效频道或默认1，经既有持久接口回on_change_channel |
| query_phone_info | 无参，回成功码及login_type=0、空mobile_num，真实表示本服没有第三方手机号绑定 |
| get_recommend_leagues | 无参，读取同服真实学会主档，过滤已解散/满员，沿用搜索UID升序最多100条；主档候选窗口1000，不加全服行锁，不返回内部公告、申请者或成员存档 |
| close_asyn_pvp_ui | 无参关闭通知，不改变资产 |
| home_enter_quit_log / share_click_log / notice_share / receive_package_check | 分别3/2/0/1参，只验证和记录方法及参数数，不据此发奖、认证或保存上报正文 |

社交查询新增AssistOnly/LeagueOwnersOnly，SQL在LIMIT前过滤实际助战卡/学会主档，避免无资格角色占用候选窗口。UpdateSocial仍按OID锁定全部参与者并原子提交，仅对规范化Progress发生变化的角色写JSONB与revision；纯读取对手不触发在线全量回载。真实PG回归覆盖只读版本不增加、实际修改持久与失败回滚。

### 第二轮1002的新增证据与兼容修补

1001在线下行on_set_focus_target触发shadow_battle缺方法。反汇编确认真实入口是player_input.set_focus_target(eid,target_eid,target_type)，现经sync_battle_method传递本人ObjectID；24位十六进制目标恢复ObjectID，空字符串取消与原生槽位字符串保持原类型。仍拒绝未接输入日志的权威竞技，不把聚焦请求当战斗结算。

抽卡内部旧档RandomCardRecord.PoolID继续使用pool_id保存，客户端random_card_record的存储字段实际为random_id，pool_id是只读表属性。登录单独投影random_id/random_count，抽卡成功在回调前同步真实计数和保底记录；不改抽卡概率、资产和旧档键。未知卡池旧记录不向客户端投影，内部保留。

签到修补补上历史缓存迁移：reward_item.update绘制前仅把仍为Box的checkin_bonus.day_N转为to_list，不清领取记录、不改资产。BattleEntity异常补上组件装配边界：原生client_battle通过gen_class复制组件方法，补丁在组件之外更新仍指向原生battle_performance.create_entity的实际战斗类；不覆盖其他来源的方法。Python2原生函数与装配复制夹具覆盖旧异常、更新后返回实体、重复安装、旧缓存迁移、聚焦取消/无效目标和random_id零值复现。

### 网络诊断与明确未完成边界

诊断revision2保留原错误链、限频和去重，HTTP原生回调额外记录主机名/方法，不记录路径、查询串、头或正文。历史日志没有目标URL，不能把HTTP读超时/解析失败标为已修复；需新版本对应目标与客户端日志继续定位。两个current/input不一致事件也没有完成因果复现。

尚未实现的回流、通关队伍榜、抽卡广播、外部GM桥凭据、中古、端午、竞猜、明星和勋章/分享奖励入口仍需各自真实规则及原生契约，不用空成功掩盖。资源缺失、部分商店门槛、高层装备原石/幸运规则及腾讯表全部客户端逐项验收保留。当前源码/函数回归与真实PG不等于Android全功能通过；发布与新PID日志结果见统一验收节。

## 2026-10-10 腾讯表修复：装帧、语种和探索日常

需求与全部55条追踪见 `out/REPAIR-TASKS.md` 的腾讯表章节。修改前已读取19份项目/参考Markdown（排除重复工作树），UTF-8校验及文件摘要见 `out/evidence/qq-bugs-20261010/md-read.json`。

### 装帧契约

原生cards表awaken_grade才是觉醒装帧解锁阈值。生成器 `out/tools/generate_card_appearance_catalog.py` 导出该字段；`effectiveCardAwakened` 保留已解锁值，否则根据卡牌Grade与正阈值计算。升格事务持久化Awakened，并在成功回调前推送card_mgr、材料和card_common_mgr；旧存档在登录/外观投影时兼容，不批量改写玩家资产。只解锁该卡固有觉醒装帧，付费/活动装帧仍走原拥有检查。原生4401阈值1，固有装帧440102。

### 语音契约

原生声音后缀表为0→空后缀、1→_ch、2→空后缀，0会选日文事件，不是非法索引；声音管理器先看card_common.card_vo再看global_set_vo。仅保存global_vo却继续投影card_vo=0会覆盖中配。新增Progress.GlobalSetVO，旧档优先沿用有效全局语种，否则按原生默认2。单卡显式设置优先，全局设置其次，并受原表可用语种约束。全局切换1/2保存最近默认并清除旧单卡覆盖；切换0保留最近默认和单卡选择。推送global_vo、global_set_vo及card_common_mgr后才成功回调，让客户端重建声音映射。story_vo保持独立。

证据：`out/evidence/qq-bugs-20261010/voice.log`、`voice-language.log`、`global-voice.log`；`out/tools/test_qq_bug_voice_native.py` 加载真实Python2声音函数，复现旧值覆盖并验证中文、单卡日文、通用及剧情语音。通过函数验证不代表缺失BNK/WEM资源已补齐，也不代表真实设备音频播放通过。

### 每日任务契约

探索最终节点完成原先只推进type6带stageID参数的成就，遗漏每日1013的type6空参数目标；现同时推进通用目标一次。普通battle_observe的结算前后公共任务属性同步被跳过，故在remainingDungeonCompletionPushes加入basicRewardPushes，放在battle_result前，覆盖普通战斗和探索。重复结算与重复领取仍须幂等，未修改每日积分或伪造完成。

### 验证与发布边界

`internal/game/qq_bug_repair_test.go` 覆盖全局/单卡/旧档语种、50余张真实卡阈值和付费隔离、探索完整节点链、结算前推送、每日1013、领奖一次20分及冷序列化；专项PASS。`internal/game/dbstore/qq_bug_repair_test.go` 的TestPostgresQQBugVoiceAndAwakenColdPersistence覆盖真实升格扣费与装帧、语种切换、冷登录和重复升格拒绝；未配置数据库的本地运行不计作真实PG通过，由统一流水线真实执行。原生声音函数PASS，完整Go与真实PG最终结果由统一验收章补充。不会据此宣称全部日常均可满分或整表55条已修复。

## 2026-10-09 全服周期结算调度与CPU修复

### 故障链

游戏传输层每连接100ms调用一次 `Service.Tick`，旧实现又从Tick、`BecomePlayer`及通用活动方法前置钩子进入活动/PVP周期结算。`dbstore.UpdateAllSocial` 会在一个事务内按OID顺序锁定全部 `avatars` 和 `avatar_progress`，对全服JSON进度结算后批量写回。547个角色时单次事务超过原5秒请求期限；取消后周期键不推进，下一连接/下一Tick立即重试，最多16个池连接同时排队。第一条修复上线后继续采样又确认第二条链：`finishLogin` 主动调用 `refresh_assist_use_times`，读取同服最多1000个完整进度构造陌生助战，失败后Tick逐秒重新执行登录收尾。两条链共同形成CPU、锁等待和登录/副本超时放大环。

### 当前调度契约

`Service.RunPeriodicMaintenance(ctx)` 是常驻入口：启动立即执行一次，此后每分钟调用 `RunPeriodicMaintenanceOnce(ctx)`。单轮先活动、成功后再PVP；两类结算共用 `periodicSettlementMu`，同进程最多一个全表事务。各自仍有周期总线锁及持久邮件/收据幂等，只有事务提交后才更新内存周期键；错误返回给调度器并在一分钟后重试。活动失败时本轮不再启动PVP全表事务。

`UpdateAllSocial` 的外层期限和事务内 `statement_timeout` 均为120秒。玩家Tick只执行本角色日界和既有轻量时钟；登录和普通活动请求不启动全表事务。既有PVP业务若发现结算总线忙，立即沿用已提交周期状态，不等待占用玩家连接。后台已完成当前周期后，`refreshPlayerActivityCalendar` 可在登录时为新角色补单行日历；总线未就绪、正在执行或周期已变化则立即跳过，绝不回退到全表扫描。`finishLogin` 不再预热助战候选，只完成重连凭证、剧情/战斗恢复及 `on_refresh_login`；客户端进入助战界面时仍可显式调用 `refresh_assist_use_times` 获取最新名册。真人在线租约失败重试间隔由1秒改为5秒。

### 验证与运行记录

本地 `go test ./...`、`go vet ./...`、正式Windows/Linux构建通过。新增回归直接统计 `UpdateAllSocial`：Tick/登录/通用活动请求为0；后台首轮活动/PVP各1次；同周期不重复；PVP锁忙不排队；新角色登录只写自身；已解锁助战的登录收尾读取同服候选次数为0。真实PostgreSQL最终66 PASS、0 SKIP、3项辅助PASS，dbstore测试SHA256 `42f452b1e50718a5ca6c32eb7085c6b53d932bc8d3288753b8db3d51f0185856`。

生产仅替换并重启 `hs-game`。最终PID `71659`，游戏SHA256 `a390b15eda987c13e5d626069d929ec47775802de50a8d6843de51e18d04cdc1`，备份 `/opt/hs-server/bin/backup-20261009-235414`；本轮修复前备份 `/opt/hs-server/bin/backup-20261009-233632`。启动首轮结算后578/578角色有日历、数据库无锁等待；2分34秒、12个连接时瞬时CPU样本6.7%/11%/6.9%。最近60秒context deadline、statement timeout和登录收尾延后均为0，同时有新登录刷新及10001/10002/505战斗建立。累计 `%CPU` 包含启动结算峰值，验收应使用连续瞬时样本、数据库活动与日志增量，不可只读累计平均。启动首轮结算仍会造成短暂CPU及普通后台查询超时峰值；长时并发和真实跨周期边界仍是独立验收门。

本项目本机Go未加入全局PATH；构建与测试固定使用 `由HS_GO指定的Go工具链`，通过当前命令的 `HS_GO`、`GOCACHE`、`GOTMPDIR` 注入，不修改系统环境。

## 2026-10-09 探索根因：活动枚举与战斗模式混用（0922已正式发布）

0922已组合发布并独立核验通过：游戏PID46432、热更PID81731，游戏SHA256`f74139ab9d2f1fa6cde3a238e0a79bf839ff8b147203eea5b3d20393a757eaf4`、正式热更SHA256`33873fec554a2159d8edf4fc28fa350ea216771bf12df2838a29953a216bcd67`、PG产物SHA256`801b24a7c62d5d8a71ff44d3292cc7face810d200965a299315c52abe5da3435`。最终默认/正式全仓、vet、构建、真实PG66项PASS零SKIP及3辅助、只读预检、组合发布与两项独立核验通过，报告、正式产物和私有备份已同步到four-bugs/release22，旧版本备份保留。

上线证据followup-202400/202541：同一探索会话在20:23:11进入原生on_battle_prepare且layout_visible=true，20:23:24执行开战；20:24:06及20:24:51均收到on_finish_stage_site和battle_result，随后20:25:01在10111开战。对应只读数据库快照完成节点数为2、当前战斗started=true、last_sequence=2。证据live-explore-chain.log和live-explore-session.json已归档。此为真实客户端上报、服务端结果及存档对应的两个节点证据，不是本机ADB或全部46条探索实机通关。

同窗仍有其它业务超时、社交参数拒绝及卡牌成长界面迟到回调card_info为空的异常；不声明全服零异常，不将它们混作本次探索模式问题。以下候选施工和首轮失败记录为历史过程，由本节最终验收覆盖。

0921诊断版本已组合发布并独立核验，PID43606/81398，游戏SHA256`3f5525d61c7ecb5ab62f4a9d1fdfdbfd5b4505ee2ae7e8f9d512a7ae9085d994`、正式热更SHA256`ab3a0f5b76cd921a66bfae0b757d0235fbc0ebfbb375c87fd77762a76661831c`；正式全仓、vet、构建、真实PG66项零SKIP及3辅助均通过。正式报告、产物、私有备份已同步，证据four-bugs/release21。下节0921候选状态由此覆盖。

新PID窗口followup-195356在探索10102/20202捕获scene_loaded=true、pending_models=0、layout_visible=false、battle_status=null、battle_process=2，原生场景方法正常返回且无引导等待。空battle_status是观察值，不单独作为缺失状态指令的证明。随后静态反推确定：原生37C6CA84的ACTIVITY_TYPE_EXPLORE=2，而NORMAL_BATTLE=1、SYNC_PVP_BATTLE=2、ASYN_PVP_BATTLE=3、SYNC_PVP_RECORD=4、CONSIGN_TASK=5、CHALLENGE_BATTLE=6、SYNC_PVP_WATCH=7、GM_PVP_BATTLE=8。这是两组独立枚举。

旧enterDungeon及reopenOrdinaryBattlePushes把dungeonCatalog.Type直接作为start_server_battle_ok第一个参数。探索因此被原生5FBB9C5E battle_mgr.handle_battle_entities_create按同步PVP模式识别，明确跳过start_battle_layout；这解释了实体加载/桥握手完成后仍没有布阵、重登仍恢复到同一卡点。Android目录含46条探索副本定义，本批修补共用普通副本首次进入和断线恢复，场景、battle_id、副本类型目录、胜负与奖励规则不变。

0922两处普通加载仅在活动类型为探索2时转换为NORMAL_BATTLE=1，其余原活动模式保持。原生猜拳等特殊活动另有battle_type检查，未取得全部模式契约前不把所有活动统一成1。真实同步PVP及异步PVP专用入口仍发送2/3，冷登录已有专用匹配仍覆盖普通恢复为对应专用载荷。新诊断按data.dungeons[dungeon_id].dungeon_type辨认探索，不能再用battle_type==2判断；布阵实际入口为on_battle_prepare，修正旧诊断候选init_show的观察名称，升级时恢复所保存原函数、保留RPC元数据且不叠加旧包装。

实际Python2执行原生handle_battle_entities_create已复现：模式2创建人物、怪物和领域后跳过布阵；模式1继续start_battle_layout。Go探索首节点首次进入/恢复模式及资产不变目标回归通过；真实PG原测试补入场模式检查、真实鉴权冷登录模式检查、原扣费与节点授权保持后继续胜利结算。正式流水线最终通过，结果见本节顶部，原生函数夹具本身不作为Android实机通关。

0922首轮真实PG为65项PASS、1项失败、0SKIP，未发布。新增探索冷登录覆盖了旧pgActivityLogin助手未实际走过的分支：旧断言要求未完成战斗refreshes=0，与既有finishLogin和battle_recovery_test契约冲突。现有流程先恢复battle，最后唯一on_refresh_login移除登录遮罩，客户端login_battle_recovery revision10识别已恢复对象并避免重复恢复。实际Python2原生RPC/Asio调用、同UUID幂等及旧残留替换回归通过；助手改为检查恢复模式、唯一收尾及先后顺序，不修改生产登录逻辑。首轮失败报告保留在私有目录out/backups/explore22-first-pg，最终流水线重新绑定源码和产物后验收。

## 2026-10-09 探索仍卡死：节点加载后停住（0921诊断候选）

0920已正式上线并独立核验，游戏PID41274、热更PID81127；正式全仓、vet、构建、真实PG66项PASS零SKIP及3辅助通过。游戏SHA256为`335d43dd16cc619a8df67d891eddca19d738305e872df43f8eebd695d18b7ebd`，正式热更SHA256为`b765ba0e65a0d8fbc4fe639d476e8d23f3ced90dfcc136839287ef027f49e022`，PG产物SHA256为`6d08020d1641668ba872d62073e5a4b62069b08a464a03cffc47b6f6e4eddcda`，证据在four-bugs/release20。下节候选描述由本节正式验收覆盖，但不代表探索已修复。

用户再次反馈探索进入卡死。本轮重新完整读取19份MD。0920全服窗口followup-193534关联到地图10101初始化结束、首节点战斗10102/20202建立、断线及重登恢复；快照followup-193620的54条探索战斗均为loaded=true、bridge_ready=true、started=false、last_sequence=1，其中10101有40条、10102有14条。单次存档只能证明当前停点，不能把全部历史会话认作本轮故障，也不能据此强制开战。

原生B0D07550、5357E5D0、09DD7B8B只在初音入口/自动路线使用208布尔开关；3B69CDD4的布阵返回却会以任意当前副本类型调用set_explore_auto_agent(false)。因此日志中的“非初音自动探索拒绝”并不能证明普通探索入口根因，本批不放开未知自动功能。探索快照中的服务端内部倍率保持既有规则，不改玩家资产、节点、引导或战斗收据。

0921补客户端观察点：Avatar探索进入及节点响应、普通探索场景on_scene_loaded/check_auto_battle_fighting、布阵init_show/start_battle和battle_fighting；记录地图、战斗状态、待加载模型数量、布阵可见性、自动状态及开战标志。异常记录完整末尾栈后继续抛出；原返回值原样保留，数值/布尔结果才写入诊断，不上报卡牌UUID、账号或设备。RPC方法通过原rpcmethod.func注册入口包装，保留装饰器和注册对象；幂等安装，延迟导入的类仍重试，自动检查每对象仅记录前4次，节点15秒快照只观察、不取消等待。探索地图旧版本闭包被精确解包，避免重复套层。

实际Python2回归通过原生on_enter_free_stage函数、RPC注册入口、异常传播、布阵返回值、重复安装及轮询限量。首轮夹具把函数放在类属性触发绑定错误，已改为原注册器实例字段并重跑通过，不计作客户端问题。0921正式流水线进行中；这是补齐定位证据的版本，不声明玩家探索已通过。

## 2026-10-09 失败回调与后台排队修补（候选0920）

0919已组合发布并独立核验，游戏PID38566、热更PID80810，真实PG66项PASS零SKIP及3辅助。上线窗口`followup-185139`没有客户端异常上报，但仍有入场/开战调用超时；后窗`followup-190248`也有超时及一条布阵解析拒绝，不能宣称入口问题全部消失。角色快照已经看到一条低层装备胜利盒包含契印。数据库只读观察12个连接均idle、无事务阻塞，只能排除该观察时刻的持续数据库行锁，不能据此排除连接锁等待或其它时间的数据库负载。

原生C13BDDDA.enter_dungeon的失败回调会调用外层callback(ret)及show_error_tips(ret)。原网关在Service.Handle错误时只记日志并丢弃回包。0920仅对已绑定Playing角色的`enter_dungeon`、`finished_storyline`补验证过的单整数失败回调：沿用正整数callback_id，返回875C5BCF的RET_UNKNOWN_ERROR=13；不回成功，不改资产或通关，不猜其它接口的回调形状。Service层错误和事务回滚契约保留。实际Python2原生入场函数验证ret13会执行外层回调；端到端网关帧验证编号和[13]。

后台Tick此前持有本连接锁，随后排队等待另一个连接的全服周期结奖锁。新增`tryRefreshPvpAwards`只在后台使用TryLock：忙时跳过本轮且不推进周期键，释放后仍走同一串行事务和幂等收据。同步结奖调用保留原锁语义；过期Tick不再开启后台资产或日界事务。不会移除数据库行锁、扩大连接池、改变结奖算法或伪造结算成功。并发回归验证另一个连接持锁时本连接Tick仍可返回，释放后可继续结奖。

探索及1-7/1-11仍缺确切客户端根因；19窗口活跃邮件/契印引导各26条且均有契印库存，“缺契印导致全部卡住”不获该快照支持。高层原石/幸运规则、601布局None及后窗单条布阵拒绝仍需追查。上述补丁不代替实机完整验收。

## 2026-10-09 四项玩家反馈联合修补（正式0919）

0919已发布并独立核验；正式报告归档`out/evidence/player-bugs-20261009/four-bugs/release19`。日志窗口`followup-175404`、原生字节码及核对产物保存在`out/evidence/player-bugs-20261009/four-bugs`。双倍失败体力返还也按冻结倍率翻倍，4401扣20后失败返18，券不由未取证规则额外返还。

装备结算漏读`everyday_bonus`。新增`ordinaryDailyBonus`按北京时间周一至周日选数组，保留既有首通及失败分支。原生Python2核对装备30项共210种星期选择；一至四层12项共84种组合完成实际契印生成与跨日重试。五至十层18项涉及材料15契印原石或未取证幸运规则，仍未完成，不能自行编造保底或计为发奖验收。新增明确日志及完整盒接线检查，对这些未实现的每日盒保持0918常规奖励行为，避免本次接入导致原先可结算的高层变成事务拒绝；高层掉落仍是未修复项。旧奖励无有效可追溯收据，不批量补发。

新增`set_dungeon_bonus_double(callback_id,dungeon_id,enabled)`，返回原生单整数IntRet，支持布尔及0/1开关。已完整接线的普通类型8低层装备、类型9材料选择持久化于`server_dungeon_bonus_double`并投影`dungeon_mgr[id].double`；高层未实现的每日盒拒绝开启双倍，避免多扣费用却无对应奖励。新战斗扣两倍体力及等于本次体力的材料29，与会话同事务提交；倍率冻结，重入不再扣费，中途修改选择不改变已支付战斗，失败奖励不翻倍。探索/竞技/活动独立倍率不由此接口覆盖；不支持1029、未首通解锁1030、目录错误1019、事务错误13均为原生错误码。原生`support_double`不能误作普通倍数的总开关。

2-10战斗10222有第五剧情角色槽，原四槽解析拒绝。`battle_story_slots.json`记录原表SHA及九种五、六槽战斗的`my_avatar_list`；`parseBattleLayoutForDungeon`仅按副本上下文接受额外剧情空槽，第五/六槽放玩家UUID仍拒绝，普通/PVP仍四槽。剧情角色不入库存和自有经验接收列表。相同结构覆盖2-17、旧主线、妄言、初音；不是各关卡实机完整验收。

新增无回调`on_unlock_system([system_name,...])`。服务端按原表验证名称及条件，整笔持久化后同步完整解锁容器；未知或未满足拒绝，客户端不能直接解锁。1-7为507纯剧情、后继508；1-11为511纯剧情、后继512；回归核对剧情提交、后继入口和授权补回，不据此宣称邮件/都市引导根因已确认。

灵感`consume_power_material`失败消息改为UTF8字节数组，保留增量、消息双参，防止Python2 PRINT_ITEM中文编码异常。入口观察新增返回类型、联网状态；assemble返回None不再误记accepted=false，保留原生行为并能从0918包装闭包升级。0918日志存在2-10胜利、准备恢复及四槽解析拒绝，不能把全部None观察值当断网。

探索、1-7/1-11的客户端引导等待仍缺确切异常归因，不强清引导或跳关。布阵None异常样本属于601/10211，不是610/10222，其玩家身份与布局键问题仍待复现。

## 2026-10-09 常驻文案C++绑定兼容修补（正式0918）

真实0917栈确认：原生main_line_chapter.init_show隐藏章节页后构造活动卡片，activity_item.init_show调用现有活动热更set_time_panel，其Unicode参数使uis.set_string抛出`cannot convert argument 1 to std::string`，初始化中断。活动后切回主线及主线入口空白可以共享这一异常，不等于关卡602在服务端锁定。

`activity_schedule_script.py`的活动卡片和新手活动倒计时标题两处中文文案都改用`u'常驻开放'.encode('utf-8')`，满足Android Python2/C++ std::string字节串边界。运营常驻配置、活动日期、奖励和关卡门槛均保留，有限日期路径继续调用原生函数。全内嵌脚本同类set_string调用核对仅这两处。

`test_activity_schedule_script.py`对145活动规则回归采用严格bytes控件夹具；`test_ui_callback_repair_native.py`使用实际Python2编译的日程函数、严格str控件输入夹具，验证Unicode旧错、UTF8中文回读、原生有限日期回退。原始栈与首轮夹具失败/修正后通过日志留在本地stage22。0918已发布、实库65项零跳过及3辅助通过、独立核验通过；后窗主线页面5次打开均结束、异常0次，含此前异常角色。正式哈希及客户端证据边界由交接和进度文件维护。

## 2026-10-09 主线与战斗生命周期断点协议

0916已发布并收到主线初始化失败断点。候选0917将主线`_hs_open_flow_revision`升至2，保存原函数并能从0916确切闭包名取回原函数，异常事件记录最多6000字符Python栈；原异常继续传播。新增`panel_double.update_double_button`空img_on保护：仅省略销毁控件绘制，不阻断原set_callback后续press_callback、不改变倍数/费用/服务端确认。原包方法和真实异常链保存在本地`stage22`，Python2验证正常绘制与空控件分支、主线异常栈通过；此修补与主线空白根因分开验收。

0917已正式发布并核验；原生widgets.pyc的update_double_button复现空控件异常，修补后原生check_cost.set_callback仍执行press_callback。具体生产哈希、PID与实库验收见交接和进度文件；主线空白恢复仍待实际异常栈和UI证据。

`ui_callback_repair_script.py`候选0916扩展既有`client_sa_log('hs_ui_flow', ...)`，增加`mainline_open_begin/end/error`、`enter_dungeon_begin/end/error`、`check_dungeon_begin/end/error`、`raw_clear_battle_begin/end/error`、`on_dungeon_finish_begin/end/error`及`dungeon_after_entry`。新增字段为`dungeon`、`has_battle`、`battle_dungeon`、`battle_process`、`model_ready`、`guide_waiting`、`story_waiting`、`scene_present`；入口/检查结束另有原生`accepted`，快照失败仅标`snapshot_unavailable`。

以原生函数返回值和异常为准，不修改本地授权、结果界面或模型状态；入场被拒绝不安排计时。成功入场后15秒只记录状态，后发入场使旧计时失效，不执行超时修复。方法及主线界面分别版本化幂等安装，已装配类和后续按需加载界面继续由现有调度器安装。Python2回归覆盖保留阻断、返回值、异常、快照失败隔离及重复安装。

主线2-2对应地点602、战斗10220。截图与全服记录尚未关联，0915有成功握手及胜利，不据此断言所有玩家正常。断点候选0916的正式发布信息由交接与进度文件维护；这批观察能力不等同于2-2空白根因已确认或修复。

## 2026-10-09 活动返回主线的原生生命周期证据

原生`common_logic/dungeon_check.check_enter_dungeon`在`Avatar.is_in_battle()`为真时直接返回`RET_DUNGEON_IN_BATTLE`，不会进入RPC。`entities/components/battle.is_in_battle`只判断`self.battle`是否存在；结果处理中`on_dungeon_result`还要调用此对象的`set_battle_result`，不能以服务端Finished为由立即删除客户端模型。原生`Avatar.clear_battle`调用`raw_clear_battle`后才调用`on_dungeon_clear`或指定返回回调。

相关原包SHA256及静态方法证据已保存本地`out/evidence/player-bugs-20261009/activity-mainline`。本轮日志没有与用户反馈对应的主线拒绝；初音20831001未进入地图、引导等待及HTTP超时不可直接归因为本问题。需用活动名称、实际点击表现和角色操作关联确定断点；0915保持现网，本轮未添加强清状态或跳过门槛的补丁。

## 2026-10-09 第十四批：关卡完成同步与重复进入契约

2-3对应原生地点603“半身”，关卡603为纯剧情，604为战斗10231。原生`dungeon_check.check_dungeon_finished`读取客户端`dungeon_mgr[id].finished`。`storyDungeonExtra`现在先返回RPC回调，再推送持久化完成条目，最后发送结算结果；保留原有首次奖励判断，重复剧情进入仍同步保存状态，不创建战斗实体。

`finishLogin`保留剧情桶推送，将助战刷新推送追加到其后，再执行登录刷新。普通及活动准备逻辑对同连接、同UUID重复请求只返回回调，保留已加载/开始标志；学会守护专用重新布阵例外保留，PVP原生权威路线不变。

`out/tools/audit_all_stage_contracts.py`以原生导出表核对服务端目录、地点归属、前置引用和章节索引，输出逐关结构证据：1461关、219地点、11组章节索引，零引用差异。`out/tools/test_all_stage_native_progress.py`直接使用原生Python2解锁函数，复现603仅有内部完成记录时604不可解锁，验证同步finished后的恢复及207项地点前置索引；章节条件为测试夹具，不覆盖真实章节门槛。

`internal/game/all_stage_progress_test.go`逐项验证43个启用主线/支线剧情节点的同步顺序与冷投影、167个战斗节点重复进入保留模型。`internal/game/dbstore/all_stage_progress_test.go`对603/604执行真实持久化、重复进入、退出和助战解锁后的冷登录验证。本批已发布runtime2026100915，真实PG65项PASS零SKIP及3项辅助检查通过，组合发布与独立远端核验通过；完整发布信息由统一交接与进度证据维护。目录和协议回归不等于Android逐关通关。

## 公开源码同步与文档入口（2026-10-09）

本次公开交付基线为runtime2026100914，包含第十二/十三批的材料使用、锁卡、退出结果、夏日回调与引导诊断源码，以及对应测试和运维工具。README已整理为公开项目入口，说明上游来源、main派生分支、技术架构、配置、构建、分层验证、已知限制及反馈要求；GitHub介绍与问题反馈模板同步维护。

本次工作只更新源码仓库和公开资料，不触发新的生产部署。64项真实PG0跳过及3辅助属于0914既有发布记录；本次独立公开树检查另行记录，不将外部依赖SKIP或文档审核当作Android/持续负载验收。上传继续使用经隐私审核的独立公开历史，不含客户端资源、玩家数据、实际部署地址或机器绝对路径。下方上传不部署及0911声明均为历史批次，当前游戏修补基线以0914节为准。

## 2026-10-09：材料本返回、功能引导及探索续查（第十三批）

本批处理用户报告的三处卡住，最终runtime2026100914已组合发布。游戏PID26291、热更PID79026；游戏SHA `4f6bb3d0d09b87ce68aa73abb4782deb79f500c1aa2e317356940a4ea763226a`，正式热更SHA `db92d34014ed8d633d84040748824934c4ff885e393e5a3b05ff15804164ef97`。默认/正式全仓、vet、构建、真实PG64项0跳过及3辅助、Python2原包夹具与独立游戏/原生/双主机核验通过。最终报告在repair13/final14，固定报告和产物已同步out；私有分发备份更新0914并保留0913。原始日志窗口14:14至14:23保存在本机followup-142328，原包方法及SHA证据集中repair13；玩家标识和日志不进入公开源。

已确认的退出协议缺口：普通exit_battle把失败及体力返还持久后只发exit_battle_ok。Android A66FD60A的该方法调用battle.real_battle_end；2CC3F05D继续notify_battle_finish，当前桥上报result。但战斗已Finished，服务端拒绝该结果，缺少battle_result使结果界面无法获得结算盒。修补在退出确认后用已提交SettlementBox发送失败battle_result；重复退出只重发确认，不能重复结算、返还或重复弹结果窗。竞技退出仍沿用权威专用链。Go及真实PG回归检查回包顺序、失败状态、三次退出和冷登录资产。

已确认的客户端异常：原生新手奖励回调调用旧task_item.update，而014B74D8的inner_item.get_info直接读取info[0]；属性同步重建列表后旧行info为空会IndexError。只为beginner_guide.task_item和activity_task_item增加空行返回None保护，使用原update既有隐藏分支；有效行仍调用原get_info，不改变奖励、任务完成或解锁条件。Python2夹具直接加载本版widgets.pyc复现旧异常，并校验空行及有效行。

功能解锁普遍卡住及探索入口尚无确定根因。保留原生trigger_guide和free_stage_map.init_show，新增hs_ui_flow诊断：实际触发的guide_started、同一上下文15秒后的guide_wait，以及explore_open/explore_ready。15秒仅为记录等待，不视为失败，不跳过引导、不自动发奖、不强制解锁。事件只包含步骤、原表编号和等待标记；投递失败不影响原流程，服务端沿用限流。按需加载地图模块时继续安装，避免只在启动窗口尝试。当前本机MuMu ADB连接不可用，不能声称这三处均已Android验收。

0913首次已上线并独立核验；followup-144836（PID24798，14:43:15至14:48:37）中一次普通退出确认后实际下发结果，三类后台刷新超时均为0。集中重连的较早窗口分别出现租约129、快照129、周期结奖125次超时，不能把重连窗口当持续故障或已永久解决。引导观察记录了36次真实启动、2次同一剧情上下文15秒等待；该等待仍可能是正常阅读操作，未关联用户卡点。新手奖励越界未再上报，但0913记录未触发引导检查过多，可能占用诊断限额，不能据此做零异常验收。

0914修正诊断噪声：只记录真正启动的引导；保存原方法属性，并按0913已知闭包字段original_trigger_guide迁移旧包装，避免在线升级继续调用旧噪声包装。Python2实际guide_mgr.pyc夹具核验原生剧情标记、未触发不记录、旧上下文不回报和0913包装迁移。四十文件分批、相对路径gofmt已修复私有长路径造成的Windows参数上限；整仓源码漂移批次已拒绝，随后固定输入重新验证。

0914实测窗口followup-145633包含旧客户端更新过渡：7次退出确认、15次结果推送、一次探索初始化完成；这些计数不能作为全部UI通过。新窗口followup-145839（14:56:35至14:58:39）记录11次实际引导启动和1次15秒等待，未触发引导日志为0。旧行越界未再上报，但出现两名角色的成就总览迟到回调空控件异常（achv_mgr.server_callback→mission_achievement_pandect.on_receive_bonus→update_first_class_tab），与本批修补的beginner_guide任务行不同，单独登记待修。探索初始化完成只证明一名角色一次入口；用户UID和卡住页面未提供，功能解锁普遍卡住及探索具体卡点仍未闭环，不能强制跳过原生引导作为验收。

## 日志驱动修补第十二批（2026-10-09）

本节为现行接续，下方0911及上传章节为历史批次。日志followup-133715覆盖PID18512的12:24至13:37，357角色快照仅留本机；分类包含普通退出误真人路由40次、灵感材料未实现64次、跳过编辑类型拒绝43次、锁卡未实现21次、夏日回调少参2次及签到空对象1次。限流上报不等于实际总故障数。本轮严格UTF-8读取19份项目及参考MD，静态原包及来源SHA、读取清单在out/evidence/player-bugs-20261009/repair12，工具inspect_log_repair_contracts.py。

### 接口、原包与持久化规则

- `consume_power_material(callback_id,material_id,count)`：0EC45730回调两参为实际灵感增量和消息；6562F4CC及37C6CA84限定材料type4/stype9，单件量取原表target_id（902为50）。正数量、余额、溢出检查后，同事务扣料、结算自然恢复并加灵感，材料累计获得量不减少。按原生拒绝增加后大于或等于9999；失败回零增量及失败消息，不造成功。
- `lock_card(callback_id,uuid)/unlock_card(callback_id,uuid)`：BB2B671E单IntRet回调；本人卡lock置1/0并持久化，先同步card_mgr再回调。锁卡仍不能归还，不改变阵容或资产。
- `change_dungeon_skip_edit_state(callback_id,dungeon_id,is_skip)`：014B74D8按钮实际发送整数0/1，C13BDDDA负责转发。兼容0/1及布尔，拒绝null、字符串和其他整数；合法回调均返回Bool，包括类型拒绝，偏好仍按副本保存。
- 无参数`exit_battle/leave_battle/quit_battle`：普通副本按本次PaidPower/PaidMaterials及原表失败奖励/返还提交失败事务，再发原生exit_battle_ok；重复不重返还、不推进通关。已提交胜负保留，暂存真实末态先重试，自动进入下一排队战斗时不误关下一会话。清理未进入的排队请求时回合法失败值，夏日保持两参。匹配UUID的真人房间仍走双角色弃权事务；单端原生竞技退出未接通权威弃权，明确拒绝普通结算，不算已修。
- Type9升格副本统一使用同连接、同UUID已下发准备标记，覆盖4202/4302/4402/4502；重复只回原callback，不再次创建/prepare，不重扣费。真实日志出现4202构造/准备断言，本批补上重复入口保护；是否完全消除该设备异常仍须发布后日志，不把候选原因当完整实机结论。
- `summer_game_enter_node(callback_id,node_id,extra)`：90B7C06A的_callback需要ret、box。战斗节点借用enter_dungeon原先只回ret；现直接与排队入场均回ret、None，拒绝也两参，入场不造奖励；原有事件及战斗结算资产逻辑保持。
- ui_callback_repair_script.py原先等签到、抽卡及Avatar三个模块全部加载才安装；界面按需导入，签到先打开时仍执行旧函数。现按已加载模块分别安装，其他模块继续等待；签到修订2更新旧修订。真实Python2复现原生get_item空列表异常，并验证抽卡模块未导入时已安装签到保护、奖励缓存与迟到展示保留。控件及场景是夹具，不算Android画面验收。

实现log_repair_business.go。新增真实PG用例TestPostgresLogRepairPowerLockIntegerSwitchExitColdPersistence验证扣料加灵感、余额不足回滚、锁定/偏好持久、三次退出只返还一次及冷登录；夏日完整战斗回归改走实际summer_game_enter_node入口核验两参。11个验证工具未定义r__import__修为内置__import__，保留HS_GO覆盖。正式规则回归发现三个单卡测试手工改库存却继承全英雄获得历史，已显式关闭这三个夹具的初始化开关，全英雄独立用例继续覆盖；不更改生产开关。

### 验证与发布

runtime2026100912已组合发布，游戏PID22271、热更PID78471。游戏SHA `7cbf5a21fe610fb8fed734f243d8921ed60af011370853718c8d5ef6dbd900c1`，正式热更SHA `e1d1858802879c484bd0a63da3ad339f2add43c384de3cd855f156b3d79158cc`；报告repair12/completion-release-20261009-135949-741644.json。最终默认/正式规则全仓、vet、正式构建及第二轮真实PG64项、0跳过、3辅助通过，实际验收产物SHA为e8061cc75b5eab21457b21c27b2ddef48be45302b0a06d429db2835e172c91ab。独立游戏/原生核验通过，9000及15432监听、current与规则环境保持正确。

原生独立核验第一次失败，原因是源树夏活宿主缺少与Android脚本分割边界一致的末尾空行。先同步宿主后同源专项仍拒绝；进一步对照并恢复空行，执行AST始终完全相同，末尾空行修复后游戏和数据库产物SHA均未改变。已重新执行真实Linux夏活24项、初音/汪言/原生统计专项以及独立核验，最终通过；失败日志保留，不以注释等价直接替代哈希验收。原生注释同步.json记录备份及每文件SHA，current未切换，未改变原生业务代码。未来必须先用同源生成流程准备宿主，不再手工去掉边界空行。

该末尾边界空行由同源分割契约要求，git diff --check报告新增EOF空行，故保留并明确登记；不能为了消除格式提示再次破坏实际原生同源检查。

prepare_private_release.py在仓库外准备正式目录，分发地址取已授权私有备份并先逐项核对发布前现网0911，新扩展取当前源码。公开树保留示例地址，不写回私有地址；正式热更SHA与公开示例清单SHA因此不同。只读预检、组合备份原子发布及独立核验已完成，正式构建和固定发布报告已复制到本机repair12及out，发布操作必须沿用private-release-workspace.json指向的私有目录，不能直接以公开示例树覆盖现网。仓库外私有分发备份已更新0912并保留0911备份，不上传玩家日志。

发布后按PID22271隔离：followup-140740有26次绑定、26次登录刷新及4次普通退出确认，仍有签到旧get_item空对象1次、后台租约及快照各114次失败，多在集中重连窗口；不据此称签到门已关闭。后续14:08至14:14的followup-141412有11次绑定、11次刷新、2次退出确认，签到空对象和夏日少参均0，但后台租约1次、快照2次失败仍在；该短窗口不是长期稳定证明。

新窗口还发现新手任务领奖回调的IndexError：avatar.receive_new_task_bonus.server_callback第780行→beginner_guide.get_callback→update→widgets.get_info，不能与已修签到get_item混为一项；需继续核对任务列表与界面选择索引。拒绝挑战新增形态均为(callback_id,None,1)，CB7E0654的_on_set_challenge_dict在不能挑战时调用refuse_challenge(reason=CHALLENGE_BUSY)且不传目标，说明真实空目标有原生调用来源；尚未接入此语义，不能仅转为任意OID或声称原玩家标识损坏。collect_player_bugs.py增加--logs-only及--pid，仅采指定进程日志，避免反复读全服存档和混合旧进程错误。

聚焦168次、手机信息93次、双倍76次、聊天60次、回归人数36次、通用排行榜50次和其他分类仍开放。只取得部分调用契约，未确认费用、奖励、聊天实体或回归人口规则，不按名字补成功空回包。拒绝挑战标识74次已补类型/长度诊断，不放宽十二字节归属检查。HTTP失败尚缺URL，后台数据库超时及持续负载继续取证，不猜测改域名，不称全服BUG清零。

## 公开交付隐私边界（2026-10-09）

用户明确要求公开项目不包含其电脑及服务器隐私。公开目标为GMA-Revive-Server-Public，原GMA-Revive-Server私有fork保留；GitHub拒绝私有fork直接改为公开，因此公开版本从已审核源码树建立独立历史，不携带原仓库旧提交、作者私人邮箱、服务器配置和历史密钥。后续必须使用out/tools/publish_public_source.py导出公开版本，禁止把私有Git历史直接推到公开远端。

当前版本已将机器绝对路径改为相对路径/环境变量，服务器地址替换为文档示例网络192.0.2.0/24，旧局域网地址改为本机回环；个人机器工具、客户端打包工具、记忆维护脚本和部署公钥退出上传清单。公开Markdown中的实际玩家UID匿名化。audit_public_source.py检查当前Git文件里的本机授权地址/口令、令牌、私钥、个人目录/邮箱及不应收录的数据目录，仅输出文件名和问题类别；它不证明旧私有历史已被清除。

原生产分发配置备份位于仓库外的当前用户应用数据hs-server/deployment-config；本机SSH授权继续从独立DPAPI配置读取，GitHub授权继续由Git Credential Manager管理。公开配置是示例，部署前必须使用自身环境变量和私有配置渲染并重新完成构建、实库检查与发布预检，不能直接将示例地址发布到现网。本轮不修改生产服务，线上仍0911。

详细公开入口见README；公开推送记录保存在本机out/github-public-upload-report.json，不上传机器路径或授权数据。源树与公开Git提交可因独立历史而具有不同提交号，这是有意隔离私有历史，不是漏传文件。

公开目录独立检查曾发现家具规则测试未兼容CRLF及真实录像样本缺失。已将文本默认换行固定LF，家具测试去除行尾空白；真实录像样本分别作为子用例，缺失明确SKIP且不算验收，SHA/结构验证保留，恶意pickle拒绝及所有权检查始终执行。公开仓库不上传玩家录像。基础Go测试/构建可以仅使用公开树；真实PostgreSQL和Python2原生引擎专项仍须显式配置，SKIP不是PASS。

## GitHub服务端源码交付（2026-10-09）

上传目标为私有仓库SakuraZuk/GMA-Revive-Server的main，保留远端原有Python版本历史，现行主分支整理为Go服务端。只收录cmd、internal业务源码/规则、deploy公开配置/脚本、out/tools及既有详细Markdown；客户端资源、原生运行时、玩家日志/存档、数据库备份、构建产物和私钥均忽略。README为新入口，SERVER与三份核心交接继续维护，未将未完成BUG写成通过。

GitHub授权使用Git Credential Manager的Windows凭据存储，origin URL不含令牌。本机Git配置http.proxy为空以绕过不可用的旧127.0.0.1代理，未修改全局设置。两个原服务器授权已迁移至当前Windows用户的DPAPI加密配置 `%LOCALAPPDATA%/hs-server/operations.dpapi`；三个部署/资源工具改为local_ops_credentials.load_target读取，密文回读与迁移前值一致。环境变量方式支持新电脑及Linux，详见README。仓库和记忆不保存任何明文口令或令牌。

本次仅整理上传、凭据读取与文档，不发布新的生产版本；线上仍为runtime2026100911。提交前扫描所收录文件，确认没有本机授权值、私钥和玩家数据，复核Python读取工具与独立SSH连接，使用远端Git引用确认推送结果；具体提交号以Git历史及本机out/github-upload-report.json为准。不能把源码上传当作Android或持续负载验收。

## 登录自动收尾与迟到界面回调修补（2026-10-09 12:21）

当前发布 runtime2026100911、游戏PID18512。游戏SHA `8dcb0d377fc5d302084f3802cc0295f1e9a2368869c2765979a3eb030ab49273`，热更SHA `cd6182116455ba24625739a0f69a0413ea6d7250a03c294705f1b2577a4c950d`。组合报告 `out/completion-release-20261009-121756-095597.json`，独立远端与原生运行时核验均通过；完整Go测试、vet和正式构建通过，真实PostgreSQL 63项通过、0跳过，另3项辅助通过。

已完成普通副本冷登录仍沿用0910修补；本批补上登录收尾失败后的Tick自动重试、首次普通战斗观察免额外快照读取、无历史排行日历的单角色初始化，以及签到/抽卡/探索迟到回调的原生界面生命周期保护。语音、展示槽、看板随机和初音探索偏好已持久化，归还清理展示槽，商城/画质单向埋点接入。详细接口与验证边界见SERVER.md顶部同名节。

新进程12:21至12:22日志中10次冷登录绑定全部收到on_refresh_login，包含原阻断玩家匿名玩家；证据 `out/evidence/player-bugs-20261009/followup-122203/登录刷新统计.json`。这证明真实服务器登录收尾下发，不等于所有设备大厅画面通过。发布后的集中重连仍有后台数据库超时；体力材料、普通退出路由、聊天频道等实际报错仍开放，跳过编辑仍有参数类型拒绝，不能写成全服BUG已清零。

### 登录、观察与活动接口

`set_reconnect_auth_msg(auth)`保留原客户端触发时机、凭据持久化和战斗恢复链。内部新文件login_finish.go复用同一收尾函数；失败不标记refreshLoginSent，保留原凭据，1秒后由Tick重试。成功才标记，每连接只成功刷新一次。正在收尾时暂停本连接背景日历/快照工作，避免先耗尽请求期限；不放宽鉴权或伪造战斗结算。

`do_command("__battle_event__",[envelope])`首次合法普通观察也可使用已经鉴权绑定的本地战斗快照，UUID必须一致；真人、同步/当前异步和单端原生PVP不走此路径。结果、资产和检查点继续读最新玩家行并校验收据。新回归令快照读取及全服活动接口故意超时，仍接收首次ready/started和300个连续观察事件。

活动日历全服总线日期不变时，对空本连接快照先读自身最新行；没有任何山海/年兽历史排行的玩家只初始化自身日历。任何Ranked、RankAt、RankHard、RankDamage或年兽历史均回原全服结奖事务，跨日/跨周仍走原总线。无虚构名次、奖励或历史清理。

### 本版原生回调与偏好

- 客户端热修ui_callback_repair_script.py嵌入runtime及冷启动，标记HS_UI_CALLBACK_REPAIR。签到get_item继续处理原已到账物品/签到日缓存，先确认界面存活再更新，延迟提示再次读取当前界面。实际原生Python2 get_item对空界面抛AttributeError已复现，新保护覆盖关闭界面和延迟关闭。
- 抽卡结果迟到进入大厅时保留原返回payload，调用原有activate_preload_scene并等待真实抽卡场景；100毫秒检查一次，最多10秒。场景准备后原生处理一次；超时保留payload并释放原等待状态、输出错误。不重复random_cards、不生成新卡或假结算。Android动画未实机验收。
- `set_card_vo(callback_id,card_id,voice)`仅接受拥有图鉴及本版默认装帧cv_info中的配音ID，保存后同步card_common_mgr再回IntRetCallback。card_appearance_catalog.json生成增加原表Voices，生成工具generate_card_appearance_catalog.py。
- `set_show_cards(callback_id,[uuid,None,...])`最多本版3槽，验证归属与重复；属性show_cards和公共资料按实际保存槽投影。归还幻书同事务将对应展示槽清空，保留槽位置，不留已删除卡引用。
- `set_girl_random_enable(callback_id,bool)`保留缺省与明确false区别；`set_explore_auto_agent(callback_id,208,bool)`只保存本版初音探索偏好，以Bool回调应答，不推进探索或发奖。探索回调对已关闭miku_map不再调用update_auto_state。
- 持久字段server_card_voices、server_show_cards、girl_random_enable、activity_explore_agent均随原progress JSONB保存。业务拒绝回合法失败值释放界面等待，并记录UID/原因；不伪装成功。
- `change_dungeon_skip_edit_state(callback_id,dungeon_id,bool)`按C13BDDDA修正为三参及Bool回调，保存本副本选择；发布后真实日志仍有一个类型拒绝，需要继续核对调用端实际值，该门未关闭。
- output_shop_enter_log、output_shop_leave_log、change_graphics_quality按本版原生三参单向埋点接入，不写资产；独立60条/分钟配额，不占客户端错误上报额度，脱敏记录内容。

### 验证、发布与待修证据

检查输出out/log-repair11-all.log、log-repair11-vet.log、log-repair11-build.log、log-repair11-targeted.log、log-repair11-pg.log、log-repair11-hotfix.log、log-repair11-native-ui.log。原生专项test_ui_callback_repair_native.py加载本版原始pyc方法并复现旧签到异常，测试保留物品、迟到场景、只执行一次、显式超时及已关闭探索UI；其中模拟场景超时的ERROR为故障用例预期，非忽略真实错误。控件与场景采用夹具，不能称Android画面验收。数据库报告out/remote-database-verification.json绑定本批dc4d1f81a5a2b437298311d38421846b2e80f98175fb76b3299d15fa9b346843测试产物，无跳过。

发布经过只读预检、备份、组合原子切换；独立日志out/log-repair11-independent-release.log及out/log-repair11-independent-native.log确认游戏18512、热更77808、9000与15432监听、current原生运行时及启动无错误。本轮未修改玩家资产来绕过异常。

followup-122203按新进程18512过滤，10次绑定全部收到登录刷新，没有登录收尾重试错误；匿名玩家的实际记录从12:21:46绑定至12:21:49刷新。该短窗口不证明持续负载稳定：还有enter_dungeon超时1次、普通exit_battle错真人路由4次、consume_power_material未实现2次、request_chat_channel未实现2次、set_focus_target未实现5次、get_avatar_back_total_num未实现1次、set_dungeon_bonus_double未实现1次、跳过编辑类型拒绝1次及refuse_challenge标识拒绝1次；后台竞技/租约/房间超时也保留原始日志。后续按这些证据核对原包参数与资产规则，不能把登记当修复或把旧进程错误统计当新版本退化。

后续短窗口followup-122435（12:22起至12:24:35采集）按PID18512统计9次绑定、9次登录刷新，后台周期结奖/租约/房间读取及收尾失败记录0条；本窗口仍有其他业务拒绝与未实现接口。证据登录刷新统计.json与events.log，不据此声称长期负载或所有客户端正常。

## 已完成副本重登卡大厅修补（2026-10-09 11:50）

0910阶段发布runtime2026100910、游戏PID16268，游戏SHA `fc2e9c860118e6952cd6217e6bf0387b3f22ff8f28b049af19667f8a1bc7f8a3`，热更SHA `65761334476b178d0efb192efe36f6045815b83dcb933d5c351de2dab5783205`。组合报告out/completion-release-20261009-114536-755280.json；完整Go/vet/构建通过，真实PG63项通过、0跳过，另3项辅助通过，独立游戏与原生运行时核验通过。提前在组合报告尚未更新时执行的一次独立核验被旧报告SHA拦下，发布结束后已重新核验通过，未拿失败算通过。

11:08至11:28的真实日志中，21次连接、9名玩家冷登录只收到battle_result，漏发on_refresh_login；涉及514/602/4101/4102/4202/4402。以前只对已完成教学副本跳过无实体旧结算，其他已完成普通副本重登没有大厅收尾，导致客户端留在登录遮罩。现在已完成普通副本只下发原生on_refresh_login，初始角色属性已经含到账资产；保留UUID、奖励和原收据。未完成战斗仍先恢复再收尾；真人、异步和单端原生PVP保持独立恢复分支，不据本批称这三种冷登录画面已验收。

登录收尾refreshLoginSent改为全部恢复成功后才标记；恢复失败保留可重试状态。新增六类完成副本回归、故障后重试回归；真实PG用例实际新建连接quick_login/BecomePlayer/set_reconnect_auth_msg验证4102完成态正常收尾且不重复发料。此前活动PG帮助器错误期待完成态只收到结果，已按冷客户端没有旧battle实体的真实契约修正，不删除恢复或收据断言。

普通非结果战斗观察绕开全服活动日历锁；已缓存的普通结果直接进入自身结算/超时重试事务，日历仍由入场/排行榜/Tick刷新。11:00的真实日志有普通do_command超时后连续78次跳号，新回归令全服结奖接口故意超时仍确认300个观察事件连续接受；非法UUID、序号和胜方校验保留。活动及竞技日历在串行锁内取时刻，避免排队请求旧时间误报回拨；真实回拨仍拒绝。日界失败60秒后重试，竞技后台失败5秒后重试，租约失败1秒后重试，保留资产事务和租约期限。

证据：out/evidence/player-bugs-20261009/登录大厅阻断证据.json、followup-112847，检查输出out/log-repair-all.log、log-repair-vet.log、log-repair-build.log、log-repair-pg.log、log-repair-deploy.log和两个independent日志。旧11:09尚未定位的结论已被本节证据取代。首个09010发布窗口followup-114947还处于服务切换后的集中重连，仍有后台超时和未实现接口，不是持续稳定或Android画面证明。

0910阶段登记的签到/抽卡迟到回调、语音/展示/探索偏好、单向埋点与跳过编辑参数数量已接入0911，细节见顶部；普通退出、通用排行榜及其他真实错误仍开放。公网HTTP解析/超时缺失败URL，不猜测修改域名。

## 升格循环、归还幻书与抽卡修补（2026-10-09）

0909阶段发布为 runtime2026100909、游戏PID13865。游戏SHA `32ddd785507025161d88b911b29ab46ebee670834d2d76a92ae6bae5b65c035a`，热更SHA `1807647165b18d0f447d79f0db2b96ff8d0079a64230b26eac7c6a05b8331eb2`；组合发布报告 `out/completion-release-20261009-105458-810824.json`，已完成备份、原子切换及独立远端/原生运行时核验。完整Go测试、vet、正式构建通过；真实PostgreSQL 63项通过、0跳过，另3项辅助通过。原生Python2实际RPC回调类型专项通过。

### 证据与原因

本轮重新读取项目18份MD，记录 `out/evidence/player-bugs-20261009/ascend-md-read.json`。全角色日志从09:35扩展采集到10:08，保存 `followup-100814/`；抽卡实际请求含2、109、122、501、601，原服务仅配置1、3；decompose_cards实际命中“业务方法尚未实现”。4102为升格试炼二层，日志出现09:40、09:41、09:43等重复恢复重开。

原生908202EE的net_delay在3秒心跳超时后调用client_need_recover_battle；原服务收到该请求便更换UUID、重开准备阶段。普通观察事件原本每条都锁行、解析并重写全部玩家JSONB；实际日志还有观察事务超时后连续跳号。两条链分别造成假延迟恢复丢波次和结果无法结算。旧日志分类未统计“战斗事件拒绝”，本轮分类工具已补入；09:35至10:08原始窗口有150次跳号、169次归属不符及2次超时，混合包含0906/0907进程，不拿此统计当0908上线结果。

### 接口和持久化规则

- 普通PVE观察：仍以do_command("__battle_event__",[envelope])上行。UUID、连续序号、握手、单位结构与显式胜方校验保留；回合诊断进连接缓存，最多128条/2MiB。ready/started/settings、结算及其他玩家事务带入检查点，result超时在同连接Tick每秒重试；收据防止二次发奖。非真人房间/排队连接的背景快照由100毫秒改为1秒；普通观察不再逐条额外读取PG。真人及单端原生PVP分支保持原事务路径。
- client_need_recover_battle：同一连接、同一UUID、已开局且最近60秒仍有合法观察事件时，保留当前波次；有暂存结果则先重试结算。新连接无缓存、长期失联或未开局仍按既有冷恢复重开；不恢复中途HP/AP，不假胜利、不补造掉落。
- connect_server(type=1)/Resume：0908上线跟踪发现匿名玩家的4101在真正TCP重连后从13上报，而持久检查点只到8，后续全部跳号。0909将未完成普通战斗的冷恢复接入实际Resume入口，随connect_reply整批下发新UUID、hs_recover_previous_uuid及原生prepare，复用已鉴权Avatar，不等待旧客户端再次超时请求恢复。真人、异步及单端原生PVP仍由各自恢复链处理。Fixture重连也改读最新角色状态，避免旧凭据快照掩盖这个路径。
- decompose_cards(callback_id,uuids)：按原生2948C748的get_decompose_materials与get_decompose_return_exp，优先speical_decompose_rule，否则稀有度固定材料；经验为当前经验×exp_factor加各历史等级最大经验×对应factor，取整入材料4。固定材料不按补完次数倍增。锁定、禁止归还、委托、防守、进行中战斗、最后一本可战斗卡、重复/外来UUID和溢出拒绝。删卡、返料、累计获得、卸契印、清理阵容空槽/助战及保留获得历史同事务完成。回调只有一个原生bonus.bonus，contain_items可to_list；失败先handle_error_msg，再回空bonus释放原界面等待。
- random_cards(callback_id,pool_id,count)：已接入35个非轮换非GM卡池，玩家报告的2/109/122/501/601均在内；费用、限次、稀有度、活动解锁及逐卡权重来自本版原包表。权重以整数精确保存表中4位小数；仍保留已有首抽/十连保底和计数。所有合法回调编号的业务拒绝均返回五参回调，空card.card_list与box.box保留原生类型，不再只记日志让UI等待。
- 跟进附带属性修正：10:44:10 匿名玩家上报prev_exam_correct_info触发int没有iteritems。127E9B33声明该属性为exam.exam_choose_info，BE4D210F声明其键为Int、值为choose_info；不能发送平铺correct_num/total_num整数。现在按题号投影本角色已提交答案为每题correct_num 0或1/total_num 1，未答题为空字典并由原生get_info生成零统计；原生真实load/get_info专项通过。不把本角色数据称为原厂全服统计，不改变答题或奖励存档。

新增资产目录为card_return_catalog.json、gacha_native_catalog.json，含原表SHA256。生成入口 `out/tools/generate_player_return_draw_catalog.py`；业务入口card_return_business.go/gacha_native_catalog.go，观察缓存与重试在battle_observe.go/progress.go/service.go，背景读取在human_delivery.go/sync_pvp_human.go。JSON目录不是运营猜值。

### 验证与后续边界

`player_feedback_repair_test.go`覆盖模拟三波300条连续事件、开局/结算超时、同连接延迟恢复、不重复发材料、重复归还拒绝、卸印及玩家报告卡池。对应真实PG用例 `TestPostgresPlayerFeedbackAscendReturnAndReportedDrawPools`验证4102实际RPC、观察阶段不逐条重写JSONB、胜利结算、资产和卡池计数重登录持久化。原生 `test_card_callback_native.py`验证实际rpc.revert_args得到card_list、box和bonus.to_list。完整输出为ascend09-go.log、ascend09-vet.log、ascend09-build.log、ascend09-pg-final.log、ascend-native-callback.log，均在上述证据目录。

0908真实玩家窗口额外证据：10:40:00建立4102 U匿名玩家ac854005785786fffb52348，10:42:48同UUID胜利结算，无重开；匿名玩家随后进入其他材料副本，保存于followup-104431。多名玩家同连接延迟恢复保留原UUID，亦已记录。此证据是实际玩家上行与服务器结算日志，不是Android画面验收，也不能据单例认定全部设备或持续负载通过。0909新增真实PG凭据Resume验证，确认断线丢失的观察事件不会续用旧序号。

0909上线后窗口保存于followup-110004：游戏PID13865在10:58:58保留匿名玩家的4102战斗U匿名玩家ac8582325dd3ec26f61b059、序号105，10:59:01同UUID胜利结算。该窗口未见战斗事件跳号或random_cards/decompose_cards业务失败；没有实际请求日志不能当作这两项的玩家操作通过。窗口仍有上线重连时旧UUID事件拒绝、后台日界/租约/房间刷新超时及活动周期时钟回拨，还有其他未实现接口；这些问题保留为未关闭边界，不称全服无错误。

101日轮换池和9999 GM池继续明确拒绝，不拿静态阶段或GM配置当公开卡池；完整原厂promise_rules仍需单独补证。既有HTTP解析、未实现聊天/语音入口和角色UI数值等旧门没有因本轮测试而关闭。

## 全服玩家故障深查与本轮修补（2026-10-09）

09:00访问恢复后直接采集全服08:45至09:00客户端及服务端日志，222角色/17839卡存档结构审计未发现等级、品阶或UUID越界；这不证明UI数值正常。09:00时线上为0903游戏SHA `f7671857f95841c2822cea626d8ca0ee0fdece7d42d9977e117c61df20a5983c`、PID616。原有授权继续有效，工具直接使用已保存连接参数，不需要用户重新授权或提供逐个UID。

修补依据及接口约束：

原生调用补证：0906发布后09:26:08 匿名玩家仍上报505/10205断言；DCE9232F的AsioGateClient.entity_message字节码确认调用method(parameters)，外层包装收到一个位置字典，1F09877D的RpcMethod才转换_0至_3。版本10先识别该单字典，并兼容展开关键字/本地位置参数；test_login_recovery_native_keywords.py使用本版原生RpcMethod和真实bson.ObjectId验证单字典调用、旧残留替换及同UUID幂等。之前仅位置/展开关键字的夹具不覆盖实际网络入口，不作为恢复黑屏关闭依据。

- 抽卡结果：4名玩家上报字典没有rarity，原生BB2B671E回调将cards交给E88E0EDF界面；EB773222调用AFDD4FE7的rpc.revert_args。成功单抽/十连及碎片合成回调补card.card_list与__custom_type尾标记，属性存档仍为原结构，扣料/发卡事务不变。Python2实际原生RPC验证单抽/十连转为card对象并可读取rarity，未以夹具自造类型掩盖故障。
- 连续暖恢复：27名玩家41次新建战斗断言及后续prepare断言。多轮失败客户端残留UUID可能早于服务器上一轮UUID，收到服务器明确hs_recover_previous_uuid恢复授权后调用原生Avatar.raw_clear_battle清理残留，再进入原生新建；同一新UUID不重复新建，无恢复标记不清对象。恢复脚本版本10，兼容Asio单位置字典、原生_2/_3展开关键字及本地位置参数；不调用通关、不发奖、不改已结算状态。
- 计时器：纠正旧0903错误导入为from guis import gui_utils；保留到期/开启/关闭/开服候选与下一次日刷新，恒定卡池索引不再无限搜索日期。实际Android模块目录约束专项通过。
- 活动重复入场：首批发布后09:12:21现场同一4101战斗UUID连续创建三次，次秒客户端上报新建及prepare断言。将已证实重复的4101/4102纳入同连接同UUID幂等准备；只回复调用方callback，保留Loaded、seed、UUID及已扣体力，不重发start_server_battle_ok/prepare。单元用例覆盖两个副本连续三次重复；学会守护专用重新布阵既有用例仍通过。
- 引导失败回包：09:36:14 匿名玩家收到[False,msg]后在原生guide_task_mgr.server_callback→tips.show_tips→set_string触发std::string类型错误。仅将失败msg改为UTF-8字节，使网关原样保留并编码为msgpack bin，避免encoding=utf-8将文本转换为unicode；布尔失败、中文内容和任务校验规则保留。失败日志补UID/OID/task_id及原因，不伪造激活或完成。Go专项验证拒绝不改进度与实际网关bin编码；Python2执行本版原生失败回调，C++文本入口由严格str类型夹具验证，不冒充Android画面验收。
- 在线快照：现场hs-game平均CPU约305%，旧每连接100毫秒调用AdminPlayer，行锁事务读取并解析完整角色JSON。新增HumanAvatarSnapshotAccounts.HumanAvatarSnapshot(ctx, oid, since)返回Avatar、revision、changed、error；PostgreSQL用一条MVCC联表查询，无FOR SHARE，版本未变不传输JSONB，元数据仍每次读取。连接按OID缓存版本，无变化保留原进度，跨进程写入revision变化立即重载；错误不推进缓存，不放宽登录期限。新增实库用例验证首次读取、无变化、独立存储更新、元数据修改和不存在角色拒绝。

证据集中 `out/evidence/player-bugs-20261009/`；09:00日志目录followup-090021，发布后窗口followup-091258及followup-091504；首次切换后重连窗口仍有租约/读取超时，09:13后新窗口无上述超时或rarity上报，不以短窗口称稳定。日志分类工具classify_player_logs.py排除同一次异常参数副日志后按异常/入口聚合，发布后窗口followup-091258及followup-091504；首次切换后重连窗口仍有租约/读取超时，09:13后新窗口无上述超时或rarity上报，不以短窗口称稳定。日志分类工具classify_player_logs.py排除同一次异常参数副日志后按异常/入口聚合，发布后窗口followup-091258及followup-091504；首次切换后重连窗口仍有租约/读取超时，09:13后新窗口无上述超时或rarity上报，不以短窗口称稳定。日志分类工具classify_player_logs.py排除同一次异常参数副日志后按异常/入口聚合，resumed Go及实库输出resume-go-test.log/resume-pg.log，原生静态堆栈、回调类型与恢复专项均保存。HTTP解析失败、缺失语音切换/聊天入口、回归活动及未配置卡池继续逐项取证，不能用假成功回调关闭。角色UI数值、新手全流程和持续负载须发布后新窗口与客户端实际操作复核。

最终发布后复查：游戏PID9448，09:50:00至09:50:58新窗口 `out/evidence/player-bugs-20261009/followup-095058/` 有10002、506、4101、510等真实恢复请求，未收到恢复/prepare断言、rarity或std::string异常，也没有存档读取/在线租约超时。仍有1名玩家HTTP读取错误、10次未实现业务入口、1次普通退出未路由到真人房间和1次社交标识拒绝，不能称全服零错误；这些入口、未配置卡池及引导任务状态拒绝保持开放，后续优先结合新增task_id诊断取证。新快照247角色/19924卡没有等级、品阶或UUID越界，不证明客户端UI数值和完整技能演算通过。此窗口与错误上报去重/限流均有限，持续负载、完整教学、角色界面及普通退出仍需真实客户端闭环。

## 历史自动化复查与访问阻断（2026-10-09 03:04至07:00）

三个只读复查窗口均未取得新日志，游戏SSH首行握手及授权热更主机中转失败；未上传或重启，0904候选未发布。原报告和followup-065914等采集目录保留，不能用02:28历史导入错误代替这些窗口的新异常。09:00访问已恢复，后续以本页顶部实际发布与新窗口为准。采集工具连接前写采集状态.json，分别保存日志/角色快照状态，失败保留已取得文件且仅记录异常类型；避免空目录冒充无异常或保存授权参数。当时实库连接池收窄、复用SFTP降低共享数据库和SSH压力，未删并发业务断言、未放宽3秒登录期限。

## 历史玩家故障批次与0904候选阻断证据（2026-10-09）

当前不能称全部修复完成：0904源码已改正原生导入路径，全仓/vet/构建及专项通过，游戏候选SHA256 `b9ba64d09ac1667fee198092bc9cd299b7c19a9960952c1ce4314d03f53b0611`，热更候选SHA256 `0d4c48e0bd887e299c2a20cf3604877a35675b95dd0ad1f5cb025ed64e355e62`，数据库产物SHA256 `177b0b2f169518af2f0afa60ff1c04fadc292b0b3189cc8925f497ec5dd914bf`。最新真实库目录 `db-20261009-024458` 为59项PG通过/0跳过及3辅助，收藏室冷登录与同步PVP冷登录两项因3秒鉴权期限超时，未完整通过，发布链因此没有执行。其后的完整重试均被SSH首行读取超时、No existing session或Timeout opening channel阻断；已尝试授权热更主机中转，同样失败。已请求可用控制台入口或恢复游戏SSH，不再要求玩家UID。最后成功线上核验是02:28的0903，含下述已知导入缺陷；当前无法重新确认线上状态，不能把0904候选或旧核验报告称现行成功。

核验工具修订：测试管理池1连接、业务池4连接，原并发操作数/断言不变；初次测试争抢48槽数据库后超时，只读审计结束时生产4个业务会话、主机可用约2.5GB，不能认定生产一直占满。SSH工具连接/握手/命令默认60秒；真实库结果轮询复用SFTP并每5秒检查，避免每两秒重复握手。失败报告与原始日志保留在 `out/evidence/player-bugs-20261009/`，不改写失败为通过、不降低业务鉴权期限、不操作其他项目服务。恢复访问后先审计实际进程/哈希和项目锁，再按最终候选完成完整实库核验、预检和组合发布；如必须恢复上一已核验版本，0903报告中的旧备份对应0902，须双端整体恢复并核验，不能只回退JSON而保留0903游戏内错误脚本。

0903发布后真实日志发现02:28:02 匿名玩家 `ImportError: cannot import name gui_utils`：本次新增计时器误从utils导入，夹具也错误提供该路径，导致计时器及后续扩展安装中断。已按E88E0EDF原生模块导入和9930FB45实际文件 `guis/gui_utils.py` 改为 `from guis import gui_utils`；夹具不再在utils伪造此模块，revision索引0904重新核验和组合发布。0903是带此已知缺陷的阶段版本，不能当稳定最终交付。原始栈在 `followup-022818`；该快照123角色/9613卡结构无越界、34角色有界面引导记录，不代表所有UI故障已关闭。

0903阶段发布（之后发现导入故障）已于02:28前组合发布：游戏PID778501，游戏SHA256 `f7671857f95841c2822cea626d8ca0ee0fdece7d42d9977e117c61df20a5983c`，两端热更SHA256 `f757166b12d993464182fa6e508eb262d301c067f9371ab305b7a8b3f7b32195`。全仓回归、最终变更专项、vet、正式构建、客户端诊断/登录恢复/抽卡计时器专项及Python2编译通过；真实PG61项0跳过、3辅助通过，目录 `db-20261009-022132`。预检及组合发布报告 `out/completion-release-20261009-022458-711572.json`，独立游戏/原生核验通过：实际运行SHA、9000、runtime0903及全英雄开关1均一致；报告 `out/remote-release-verification.json`、`out/native-solo-release-verification.json`，发布后窗口 `followup-022818` 保存日志和全角色快照。0901/0902及中途候选说明为取证沿革，不作为最终构建证据。最新发布前快照120角色/9363卡无等级品阶目录及UUID异常，27个角色已有界面引导记录；不代表120个角色均执行了所有修补路径。

复测保留原存档，建议玩家彻底退出客户端后冷登录获取0903，依次核对角色选择/属性/技能/返回、未结10001/10002恢复、502退出确认、1014及2001/2002/2003教学、召唤界面计时器。旧在线进程若保留多次恢复失败以前的更早战斗对象，旧UUID不匹配时不会强制清掉，应先完整退出冷登录。服务器发布与专项不是Android实机验收；尚无本机唯一ADB设备，不能标记全部黑屏/数值异常已关闭。

已只读收集本轮全服93个角色快照及00:49后的业务事件。初次完整日志281,921,633字符保存于 `out/evidence/player-bugs-20261009/game.log`；去除巨量编码推送的事件日志、全角色快照、卡牌原生反汇编和数值审计同目录。日志确认界面引导通知 `finished_guide` 被拒绝246次，全部93个角色的 `finished_guides` 为空，另有41次引导期间副本不匹配；统计以初次快照窗口为准，不能把在线玩家后续变化当原统计。客户端事件 `client_sa_log` 原来同样被拒绝，导致事件丢失；MuMu当前没有唯一在线ADB设备，本批不能称客户端现场复现完成。

本版Android71BB353A `finished_guide(guide_id)` 会本地append并一参上报，合法编号来自 `data.guide` 或 `data.guide_task`；与两参回调的 `guide_task_finished` 是不同接口。本批将原生245条guide目录及来源SHA合并既有 `player_defaults.json`，新增接收通知、玩家行锁去重持久化，冷登录按原字段加载。只记录界面已播放，不改权威任务状态、通关、资产或系统解锁，不补造旧界面播放记录。

修补入场检查：初始1000至1013强制教学仍限制对应副本；501后召唤/邮件/升格/契印的界面教学不再独占所有副本入口，原章节/通关/资产门槛继续执行。已授权且未结算的同副本恢复先于当前教学门槛检查，避免后续界面教学阻断先前已扣费战斗恢复；不重复扣费、不清除已有战斗。本批没有用跳过教学、清存档或伪造胜利来掩盖黑屏。

新增诊断：两参 `client_sa_log(key,info)` 限长16384字节、每连接每分钟60条，按服务器鉴权UID/OID记录并脱敏凭据字段，无资产权威。`client_diagnostic_script.py` 基于原生utils/logger.py确实使用的标准logging增加ERROR处理器，并链式保留sys.excepthook；每分钟最多10种去重异常上报为hs_client_error，投递失败不吞原异常。启动/运行期/游戏内扩展生成器统一组合，索引runtime2026100901。网关默认只记推送名称/字节数/OID，排除心跳巨量输出；需原始编码时配置 `HS_GAME_WIRE_TRACE=1`。错误日志增加UID与有限非鉴权定位参数，绑定角色也记录UID。

验证：Go专项覆盖原生通知零回包、持久化、去重、非法编号拒绝、无玩法副作用和初始/界面教学门槛；真实PG专项 `TestPostgresUIGuideConcurrentColdLogin` 覆盖四独立连接并发去重及冷登录。Python诊断专项覆盖重复安装、重复异常去重、原异常处理保留。全仓/正式构建/真实PG/发布及新线上异常继续核验，角色UI实际黑屏与数值显示尚不能称全部解决。

01:50第一批runtime2026100901已发布，游戏PID774200/SHA5732e4d7e2b7d54df265cc6c899d17bf163c8039ea5636eca05ebf6a4aacb859；报告 `out/completion-release-20261009-014715-655474.json`。新窗口 `followup-015108` 已确认匿名玩家/309/179/290界面引导6/18/62真实保存，诊断也已获得真实Python栈。独立原生核验初版把诊断中的客户端Traceback误作服务器启动错误，已将两类分开报告并保留异常样本，不把玩家故障消音或称已验收。

0902按真实栈继续修补：匿名玩家/309/99/290在 `avatar_grow_up.set_cv_info` 第682行 `KeyError:0`，原生80E484CB明确直接访问 `cards_dress[card_info.dress]`；根因是服务端新卡默认装帧为0，并非card_vo=0（原生合法继承中文语音）。新卡写本卡原表DefaultDress，旧0装帧在卡背包和战斗快照中只投影原默认装帧，不批量改存档或解锁付费外观。由此中断init_show，后续 `init_show_tag` 缺失是初始化失败后的连锁错误，不造占位属性掩盖根因。

匿名玩家/379/342等的游戏内热修栈确认 `_revival_start_authority_bridge` 作用域NameError：原生exec_hotfix_data分别提供globals/locals，原运行期清单已包装闭包，但游戏内 `clientExtensionsScript` 仍推裸正文。本批拆出共用正文，游戏内推送也采用与清单完全一致的 `_hs_runtime_install` 闭包，启动脚本组合不变。Python2专项现使用真实分离字典执行原生RpcMethod夹具，覆盖安装重试函数引用、重复安装及普通/PVP隔离。

匿名玩家出现已有battle上的 `start_server_battle_ok` 断言及重复prepare断言。此次只收窄修补10001/10002/501至510：按连接已下发准备UUID去重，同战斗重复enter只回原成功callback，不重复建客户端battle/prepare、不重扣费用；冷恢复的新UUID仍发完整链。活动专用重新布阵契约保持原样，首轮过宽去重导致雅努斯旧测试失败，已收窄并重新全仓验证，不能沿用失败候选构建。

匿名玩家查看玩家资料出现 `avatar_info.show_cards` 缺失，本版67BCD825/9930FB45直接读取show_cards、show_medals、signature、gender。详情回包补原生默认空展示/签名、真实性别和原有真实图鉴/成就/主线统计，不伪造展示卡或身份认证。专项覆盖必需字段；其余新异常需继续按玩家栈取证，不称全部业务已补齐。

0902全仓、vet、正式构建通过，真实PG61项0跳过及3辅助通过；02:08前组合发布完成，游戏PID775620，游戏SHA256 `ee8dcd72b768333a9a5c4f6b68dc8de3b7ce01355ae5a8ffdc15fc2c4e527cb5`，热更SHA256 `77ef4867326027bae10a3951fccb86e2499e839675ecf322dd6a17dfb5715e5a`，报告 `out/completion-release-20261009-020457-650942.json`。独立远端/原生核验通过，全英雄开关仍为1。邮件模板同时兼容旧零装帧和本卡默认装帧，其他外观继续拒绝；首次回归揭示的邮件不兼容已修正，不能引用失败候选作为验收。发布初始窗口 `followup-020839` 无已上报客户端异常，但观察短且并非全玩家均打开相关界面，不能判定全部黑屏关闭。

0903候选针对真实3名玩家4次 `summon_new_card.start_refresh_timer` 时间越界：E88E0EDF原生函数逐日调用 `compute_card_pool_index` 寻找变化，5AFF472D在缺少活动/开服条件或单项轮转时可一直返回同一索引，最终越过Android时间范围。本批 `summon_timer_script.py` 保留失效时间、开服天数、未来开启和关闭时间候选，轮转池最多安排下一次原生日刷新再调用原生 `ask_refresh`，不搜索无限未来、不改卡池权重、抽卡计数、资产或服务端资格。通过启动/运行期/游戏内共同闭包安装，幂等安装且模块延迟加载时重试。专项 `out/tools/test_summon_timer.py` 覆盖恒定索引不搜索、关闭/未来开启/失效时间择早、旧回调取消及重复安装；全仓、构建、真实PG及发布继续核验。

0903另补暖恢复：0902发布后02:09:52 匿名玩家仍因已有客户端battle而在原生A66FD60A工厂断言，随后prepare再次断言，证据 `followup-021144`。2034D84C原生 `raw_clear_battle` 只隐藏结果、释放旧模型/回调并置空battle，不调用 `on_dungeon_clear`；不同于会推进通关的 `clear_battle`。普通副本恢复回包在原生extra字典添加服务器实际 `RecoveredFrom` 为 `hs_recover_previous_uuid`。登录扩展revision7仅当客户端现有对象id与该值一致时调用原生raw入口，再建立新对象；冷进程无旧对象、未标记的正常入场及不匹配的其他对象不清理。Go验证回包准确旧UUID，Python专项验证匹配替换与不匹配不清理，Python2编译通过；未清数据库战斗、未伪造结算奖励。最终候选需重新构建及真实PG，不能沿用增加此修补前的0903构建证据。

0903补退出确认所需副本实体：02:17:15 匿名玩家在502按返回，BAC02512原生 `get_return_str` 直接索引dungeon_mgr[502]，此前只下发已通关行导致KeyError。普通副本入场及恢复在原加载链后用原生 `client_prop_set([dungeon_mgr,id,row])` 增加已授权的当前副本；未通关finished=0，已有通关保持1。冷登录也包含未结当前副本，不凭此解锁、返还资产或结算。继续沿用原生失败返还显示与服务端真实结算。全仓/最终构建/真实PG须按最后包含此修补的候选重验。

其余未关闭实证：回归活动详情 `total_bonus_list` 缺失伴随尚未实现的回归查询；部分HTTP超时/读取失败仍需URL与来源定位。数值审计只证明初次7097卡存档结构无越界，尚未获得角色属性显示异常的明确数值证据。全部玩家异常按 `collect_player_bugs.py --live --since` 采集新窗口，`summarize_client_errors.py` 汇总末行/人数/时间，无需用户逐个提供UID，不创建未授权的定时任务。

## 新角色全英雄测试开关（2026-10-09）

用户授权测试阶段新角色直接拥有全英雄。正式规则文件 `deploy/data/gameplay-rules.env` 新增 `HS_NEW_AVATAR_ALL_HEROES=1`；改为 `0` 并重启 `hs-game` 可关闭。未配置或无效值按关闭处理，Go初始化也接受标准布尔值。开关仅服务端配置，客户端不能请求开启。

英雄范围复用当前Android `profile_catalog.json.counted_cards`，由原始 `cards` 表按未禁用且参与图鉴统计导出，共82位；禁用卡、素材卡、剧情变体不赠送。每位一张，等级1、品阶0，初始4401保留原UUID并去重。不提升馆主等级、不赠材料、契印或皮肤、不跳教学、不开放系统。共有卡与获得历史随初始实体正常下发；成就仅按真实拥有态重建，不伪造抽卡次数、消费或战斗事件。

内部入口 `game.NewAvatarProgress(level,now)` 只供真正的新建角色使用。PostgreSQL `seedAvatar` 在角色插入成功后，将初始进度与角色放在同一事务提交；重复登录、已有角色、缺失进度的旧角色恢复不触发赠送。开发夹具注册及同账号新服建角使用相同入口。关闭开关只影响后续新角色，已赠卡保存且不回收。本次不新增客户端RPC或数据库表。

验证入口：`TestNewAvatarAllHeroesSwitchAndLogin` 检查开关关闭及非法值、82卡去重、ObjectID、登录共有属性、无教学/解锁/抽卡记录副作用，以及关闭后保留已有英雄；`TestPostgresNewAvatarAllHeroesSwitchColdLogin` 使用隔离数据库验证注册落库、冷登录稳定、旧角色及旧进度恢复不补发、同账号新服建角与关闭不回收。常规数据库用例显式关闭开关，专项独立覆盖开启和关闭。

当前状态：2026-10-09 00:49已组合发布并独立核验通过，全仓测试、vet、正式Windows/Linux构建通过；真实PostgreSQL 60项通过、0跳过，另3项辅助通过。游戏进程772014实际加载 `HS_NEW_AVATAR_ALL_HEROES=1`，9000端口正常，启动日志无错误。游戏SHA256为 `bdf5b5c1fafda945a7b39cef8e7dd0b278bc15e1e162da5c6b7724fd4be44397`，热更保持runtime2026100824。发布报告 `out/completion-release-20261009-004444-813889.json`；独立报告 `out/remote-release-verification.json`、`out/native-solo-release-verification.json`；真实库报告 `out/remote-database-verification.json`。Android新角色界面尚未验收。说明已合并现有文档，不新增零散MD。

玩家分发测试接续：收到反馈时按角色UID、发生时间、操作步骤与截图/错误内容关联服务器日志，分别判断服务端RPC/事务错误与客户端异常，保存原始证据后定向修补和回归；不能仅凭服务器无错误认定客户端无故障。本次没有创建定时监控。

## 全功能兼容复核及新手引导现行边界（2026-10-08 21:22）

### 基础领奖与日常专项（2026-10-08 22:31，已发布、实机继续）

22:45实机收尾：匿名玩家原存档、等级3，新进程16136确认hotfix_suc0824及原生登录收尾。07点补给8号从界面101/102实际变151/102，弹窗原奖50；生产只读收据buff_counts[8]=1、power.value=151，再点已领项资产/收据未变。每日礼盒18号弹窗300致知之华+灵感激发1，生产material_mgr[29].count=300、[902].count=1、buff_counts[18]=1。再次force-stop并新进程18595冷登录，余额151/300/1、两领取收据均保留，活动页两项显示“已领取”。未清数据、未人工加资产；其余12点/19点补给保留可领取。证据目录`out/evidence/tutorial-relogin-recovery-20261008/`：`basic-rewards-supply-claimed.png`、`basic-rewards-supply-disabled.png`、`basic-rewards-daily-gift.png`、`basic-rewards-cold-hall.png`、`basic-rewards-cold-disabled.png`及after/repeat/daily-gift/cold-server.json；日志`basic-rewards-full.log`只筛16136/18595及22:31后，未发现本次新Python异常，第三方mcount遥测错误不冒充游戏回包失败。只关闭补给8/18实际领取/资产/冷登录子门；日常活跃、新手109任务、签到/积分所有UI及跨日实机、替代活动日常唯一选择和完整教学仍保留。

现行发布优先于本节候选沿革：基础奖励组合发布及独立核验通过，游戏SHA`e3427c2009d871dce2e430eb1cdd6816b8565a9a3eae7135645a9462dc5b51a9`、游戏PID768580；报告`out/completion-release-20261008-222656-234227.json`。两端热更未变，runtime2026100824及SHA`514a61a2cd27eb70b12f9d8a446e4a5ef8d8f70857ed258a58120c3f682ec35a`。正式全仓test/vet/构建、真实PG59项0跳过+3辅助通过。上传SSH一次超时已自动续传并校验，不能把中途错误当发布失败，也不能用发布成功替代Android领奖。静态全模块复核最新为229固定下行参数数量0候选、11动态；479真实上行中228未见Go同名字面量仍待分类，不是228项均缺失。新实机日志`basic-rewards-full.log`须只看22:31后新进程。

22:26最终重验：正式全仓test/vet及构建通过，真实PG59项0跳过+3辅助全部通过，目录`/opt/hs-server/data/verification/db-20261008-222350`；两项原跨角色/全服回滚也通过。游戏SHA`e3427c2009d871dce2e430eb1cdd6816b8565a9a3eae7135645a9462dc5b51a9`，热更脚本未改、仍runtime2026100824及SHA`514a61a2cd27eb70b12f9d8a446e4a5ef8d8f70857ed258a58120c3f682ec35a`，组合发布进行中。只读角色快照确认匿名玩家等级3、真实已通关501/502/503；此前停在502前的描述已过期，当前未清数据。尚未完成本批Android领奖/重登验收。

22:23验证更新：首轮真实PG57项+3辅助通过、0跳过，但两项旧跨角色/全服回滚测试失败，已保留`out/backups/basic-rewards-pg-failure-20261008-2219/`。原因是处理业务前单独提交跨日日界，并非领奖并发测试失败。已移除前置提交：领奖/事件在自身事务刷新，在线跨日仅随成功心跳/原有周期事务刷新，失败操作不先变单边日界。新增跨日拒绝无变动及成功心跳刷新回归。重新全仓test/vet通过，正式构建源码一致，游戏候选SHA`e3427c2009d871dce2e430eb1cdd6816b8565a9a3eae7135645a9462dc5b51a9`，重新真实PG执行中；尚未发布，不能沿用首轮结果。体力Power结构体已有网关反射转换，未确认该结构体是到账故障，不以此作为修复根因。

现场服务器765056在21:35:17至21:35:19连续四次拒绝`receive_power_supply`，原因为业务未实现；这不是系统解锁问题。Android71BB353A确认补给及积分箱回调为`box,msg`，日常任务为`bool,msg,count`，新手/每日每周活跃/单签到为`ret,box`，一键签到为`ret,Dict(day,box)`，不能统一回假成功。新增`basic_rewards.go`和来源绑定目录`basic_rewards_catalog.json`，生成入口`out/tools/export_basic_rewards.py`，新增8个实际入口：`receive_power_supply`、`receive_new_task_bonus`、`receive_daily_task_active`、`receive_daily_active_bonus`、`receive_weekly_active_bonus`、`receive_check_in_bonus`、`receive_all_check_in_bonus`、`receive_score_bonus`。

私有`server_basic_rewards`保存UTC+8日界、周一周界、真实签到日、领奖收据及任务计数，登录和在线跨日刷新后投影原生`daily_tasks/daily_active/weekly_active/daily_bonus_active/weekly_bonus_active/activity_buff_count/check_in_records/new_tasks/score_bonus_record`，不将私有账本发客户端。补给仅本版8/9/10/18四条，分别按原表07:00/12:00/19:00及18号07:00至23:59:59窗口、原奖励、每日各一次处理；类型1/101等倍率buff不发奖。奖励、领取标记和资产同一玩家行锁事务提交，奖励展开失败全部回滚，数据库失败不冒充业务拒绝或成功。成功先同步真实资产和领取状态再回调。

原`advanceNewTask`错误地把完成写为status2，Android9EFA3F3A中status>1实际表示已领奖，完成则按目标组计数等于原表阈值判断。本批改为完成保留status1、真实领奖才写2；旧版本从未有此领奖入口，因此仅无新收据的旧status2恢复为可领取，不另补发奖励。109任务按原day与14天窗口开放，未开放的后续日任务可记录本期真实累计事件但不能提前领奖，或目标组求和封顶避免超过原生等值完成条件。真实已有通关/卡等级/品阶/契印等级/四位置佩戴可重建；缺失历史消费与累计胜利不伪造。新手奖励240121实际包含52号群星积分，共享`grantNativeItem`之前拒绝type11，本批按原生material_mgr/check_material支持累计积分保存，积分箱按原score_bonus阈值和独立收据发奖，不消耗积分。

基础日常仅原基础13条及10001按真实解锁/周内开放创建；消费灵感34/1、原生成功成就事件（契印强化、赠礼、委托、探索等）、收藏室真实产出33/gather_produce_material及普通副本真实胜利种类推进。不接受客户端报完成/报活跃。多活动常驻下replace_task_id替代任务不能全部并列发活跃，本批不创建替代版本，其唯一选择规则仍待取证；其他未有服务端原生事件的目标也不称全量完成。签到仅真实登录日+1，离线天数不补造；同日重登不增；月签到`signin_monthly`保持已有独立账本。

测试：`basic_rewards_test.go`验证实际Handle补给/到账/重复/跨日/07点边界、溢出整体回滚、新手状态迁移/原奖/期限、真实100灵感→20活跃/日箱/周阈值/日周刷新、签到离线三天只增一次、积分箱无消费/重复和伪造档位、目标组封顶/未来日不提前领奖、数据库故障无回包无连接资产变动与私有投影/克隆隔离。新增`TestPostgresBasicRewardsConcurrentClaimColdLogin`以四独立存储/服务连接验证補给与新手奖恰好一次成功、真实落库冷登录收据；新手完成前置为明确SQL测试夹具，不冒充Android赚取进度。当前正式构建、真实PG及发布仍待重新绑定，不以专项单测关闭用户实机领奖门。

21:33现行发布：runtime2026100824组合发布及独立核验通过，游戏SHA`a5878a43bbdd75b84d978e8f32e1575fdaa2d4839884a7c676c256d340589294`、两端热更SHA`514a61a2cd27eb70b12f9d8a446e4a5ef8d8f70857ed258a58120c3f682ec35a`，报告`out/completion-release-20261008-212512-520899.json`，游戏PID765056；正式构建、全仓test/vet、真实PG58项0跳过及3辅助通过。下方0823及0824候选描述为发布前沿革，不代表当前仍待发布。MuMu原存档冷登录和完整功能实机验收继续执行，发布通过不关闭验收门。

本节优先于下方历史发布摘要。生产runtime2026100823已组合发布，游戏SHA`a529de656c5733d7acf8506d390e6a8023baa6ac429d952bd639d0bbd8c6dc07`、热更SHA`8f5fd8101df3910bb2195e6590331ff8d20cd30a92bd390ab4aba1584e42bacd`，报告`out/completion-release-20261008-211228-594291.json`；全仓/真实PG58项0跳过+3辅助、独立核验通过，PID763447。候选0824补初音成就下行不对称命名，正式构建及全仓/实库重新绑定验证中。普通桥revision15、登录恢复revision6不变。匿名玩家存档保留，未清数据。

最近MuMu实证为0820：20:46确认hotfix_suc0820，20:47角色界面返回大厅按钮完整，20:48世界入口、20:49实际世界地图1-2和调查入口可见，未再出现空结束时间求和；证据`runtime20-full.log`须筛选PID6669及20:43后，截图`runtime20-home-return.png`、`runtime20-world.png`、`runtime20-chapter.png`、`runtime20-next-stage.png`。19:34的501及1013真实完成仍有效；客户端在1-2开始调查前停止，等待0822核验后冷登录，完整教学和其他功能尚未验收。

### 全功能复核范围与参考实现差异

`python out/tools/audit_client_contracts.py`完整扫描Android的3099模块、55766函数/代码对象，结果集中在`out/client-contract-audit-20261008.json`。74个时间求和消费者是复核候选，不是74个已确认故障。静态未匹配下行`show_error_tips`已结合本版原生注册及参考推送核实：它是普通本地方法，网络入口应为`handle_error_msg(Int,Tuple)`；0822修补5处后下行静态未匹配为0，不代表参数和全部UI已经通过。真正客户端上行去重479个，补剧情后236个未见当前Go同名字符串，须继续区分动态路由、旧版不可达/外部官方服务与真实缺口。参考829个函数名候选含辅助，不等同缺接口。工具只读源码输出证据，不修改角色/生产。

剧情入口补齐（0821已发布、0822保留）：Android71BB353A的`finished_storyline`使用`call_server`，上行为`[callback_id,剧情名]`，885071C3的`new_extra_info`只构造对象、不插入管理器。服务端新增`Progress.PlayedStorylines`持久记录、重复幂等；原生桶为`extra_key=storyline,storylines=[],dungeons=[]`，处理后先`client_prop_set([extra_info_mgr,storyline,桶])`再回调RET_SUCCESS，登录有已保存剧情时先恢复桶再`on_refresh_login`。仅保存客户端观影记录，不授予通关、奖励、系统解锁或修改引导；不补造旧历史。参数必须玩家状态/正整数回调/有效UTF-8非空名≤256字节且无控制字符，容量8192条。`TestStorylineRPCPersistenceAndNoAssetAuthority`覆盖精确参数、去重、存储/克隆隔离、无资产副作用及冷登录顺序；`TestStorylineRejectMalformedBeforeMutation`覆盖非法参数；`TestPostgresStorylineConcurrentColdLogin`验证4个独立Store/连接真实行锁去重及冷登录。源码/夹具不能代替活动剧情的实际播放及重登验收。

失败通知统一（0822已发布）：`nativeErrorPush(code)`只推`Avatar.handle_error_msg(code,空Tuple)`两个位置参数；不改错误码、成功/失败判定或资产事务。覆盖探索进入、探索节点入战、探索设置、克苏鲁命名、残页移动共5处原错误推送；残页原callback内容与顺序保留。`TestNativeErrorRPCExactContractAndFailedBusinessRollback`覆盖精确Int/Tuple形态及探索进入、探索设置、克苏鲁三个真实失败分支的事务回滚。生产真实错误提示UI仍须解锁相关功能后验收。

下行参数全仓检查：以Go AST实际调用和本版RPC签名比较，执行`go run out/tools/audit_push_arity.go`，结果`out/client-push-arity-audit-20261008.json`。0824当前227处固定推送未发现参数数量不匹配，另11处动态推送不得自动视作通过。动态核对确认学会`on_agree_league_apply`必须3参`Int,ObjId,Dict`，`on_refuse_league_apply`必须4参`Int,ObjId,List,Dict`；原来成功/失败只推2参。0823已发布`leagueApprovalPush`，成功带同一事务生成的成员/剩余申请快照，失败带非nil空容器，不伪造成员；原审批权限、成本、上限、行锁和回滚不变。既有创建/申请/审批/退会测试新增精确成功/失败回包、拒绝不入会及快照检查，PG雅努斯链新增审批第三参断言。

动态命名补齐（0824候选）：AndroidB0D07550上行为`receive_miku_achv_bonus`，下行注册却为`on_receive_miku_achv_reward(Int,box.box)`；原`on_+method`会推未注册名称。新增明确分支仅修回包，不改成就判断/原奖励；真实Handle夹具新增1101首领的原表12号材料20000、领取态与精确reward方法，再次领取必须失败且资产/收据JSON不变。该夹具属于真实数据库套件的辅助测试，不冒充SQL或Android成就验收。其他动态11处的命名/展开构造已人工核对（收藏室、潜质、排行、初音终点6参、残页2参等），仍须类型/值/实际UI，不以固定参数0候选关闭功能门。

| 共用契约/功能组 | 本轮核对及修补 | 尚未关闭的验收 |
|---|---|---|
| 活动时间、地图、横幅、商店展示 | 参考`tools/make_hotfix.py`使用`(2145801600,86399)`兼容结束窗口；0820给145项常驻活动API/直接字段非空数值，服务端仍结束0且永久开放；不修改奖励/解锁/周内限制 | 74个消费者逐函数复核、各活动实际界面 |
| 横幅关联和定时刷新 | 补`banner_activity_type.banner_dict`反向关联，避免空`banner_ids`漏处理；常驻不排结束事件，未来开始/有限结束和系统锁沿用原生行为 | 大厅显示、切地图及多次返回MuMu |
| 山海阶段 | 本版两个阶段入口直接调用共用`get_activity_day_num`，现已有原生阶段封顶；不复制参考强制第21天，以免越过开服日/门槛 | 两阶段交互、排名、实际结算 |
| 年兽翻格子 | Android385E6D63引用未导入`gworld`，参考同样补模块全局；0820恢复该导入，不绕过票券/材料/服务端检查 | MuMu翻格、扣券、错误反馈 |
| 新手、战斗与断线恢复 | 已有真实10001/10002/501及1013证据，修补顺序和资产规则见下文 | 502至510、召唤/邮件/升格/契印及中断恢复 |
| 剧情记录、下行错误提示、属性默认值 | 剧情桶持久化0821已发布；5处错误改注册RPC0822已发布；属性默认值仍须核对本版，不照搬资产或吞异常 | 对话去重、冷登录持久化、真实错误路径 |
| 其他A至F/十五组功能 | 既有全仓/真实PG只作为服务器证据；每组仍须原生参数、事务、副作用与UI闭环分别核对 | 社交/PVP双端、各活动/商店/宿舍/抽卡等实机及负载 |

0820活动回归验证145项数值结束时间、超过兼容日期仍永久开放、有限/未配置原生回退、原生阶段封顶、周内/系统锁、反向横幅、倒计时取消、重复安装以及商品其余字段不变。兼容窗口是参考实现要求的客户端展示元数据，不是运营截止日期；既有主线/新手倒计时显示“常驻开放”。不能把扫描、构建或发布成功写成全功能验收。

修补依据与生命周期：

- 10002原生序列续行曾调用空实现，已在客户端桥恢复`trigger_all_sequence_actions`；此前实机已完成10002胜利及结果回调，任务1000至1012完成，1013进入501。
- Android ObjectID上行ExtType42使用24位十六进制文本，服务端现同时接受12字节二进制与24字节合法十六进制，非法长度/文本仍拒绝；这修复501战斗按钮无响应，不能据此称完整战斗已验。
- 连续日志`out/evidence/tutorial-relogin-recovery-20261008/501-full-repro.log`证明：501恢复先加载cityruins01，登录刷新又启动`ud_1-1-0`剧情并切换background场景，销毁战斗模型后持续`Invalid visible object`。revision5在现有battle下只调用原生登录退场/UI清理，不再重新预载剧情或重复`on_scene_loaded`；无battle仍走原生路径，战斗已失效则回退原生完成回调。
- 统计、真人共享输入、录像和PVP权威扩展曾严格要求revision12，导致revision15下不断WAIT；现在仅接受已知兼容12至15，不无界放行未知版本。生成源`authority_bridge_wrapper.py`与导出脚本同步，防止重生成丢失修补。
- Android原生只在本地更新教学激活态，`upload_guide_tasks`不授权覆写服务器进度。服务端通过`ActivateGuideTriggers`仅激活已经存在、状态0、条件2且已有真实通关记录的任务；结算、创建后继与实库登录行锁迁移共用此规则。`AdvanceGuide`允许有已验证通关证据的等待任务完成，重复完成幂等，不新增未知/越级任务，不改资产。
- 连续日志`runtime17-full.log`证明501于19:12:07胜利、结果回调正常，随后`ud_1-1-1`后置剧情完毕又进入501，19:13再次胜利并重复。原生`dungeon_mgr.handle_dungeon_clear.end_callback`直接调用`do_guide_task`，没有无后置剧情分支中的`on_guide_task_condition_happened`；任务1013一直处于执行态。revision6在原生`do_guide_task`入口仅收尾已有通关投影的执行中副本型教学，调用原生`guide_task_finished`保证后继和callback顺序；不改变剧情播放、通关资产或未通关状态。未通关、通关后一次收尾、重复调用均有离线回归。
- 已结算教学副本冷登录不直接补发无实体`battle_result`，避免结果永久缓存在登录界面；资产在角色初始化中加载，原结算收据保留，显式同进程恢复契约不变。该分支仅适用于10001、10002及501至510，不改变探索/塔/活动/PVP原有冷恢复。revision6依赖当前服务器永久恢复入口：有未结battle先恢复再刷新，无battle直接原生登录，不再额外请求旧结果。
- 19:34地图异常链：`main_chapter.update_scene→init_fire_pos→get_mid_age_activity_id→sum(end_time)`抛`TypeError: NoneType object is not iterable`，加载停90%。0819修复最近活动选择及主线/新手倒计时，20:09冷登录进大厅，但20:10至20:12的`main_interface.show_panels→update_banner_panel→get_banner_next_refresh_time`仍对空时间求和，截图`runtime19-map.png`为大厅UI隐藏而非地图验收。故0820改为上述参考兼容窗口及共用横幅修补，不再逐界面仅避开None。Android4555A786的`utils.assemble`复制组件函数到Avatar，必须替换已装配Avatar方法；19:51中断组件单独修补候选，审计确认当时生产仍0818且未覆盖。回归明确验证装配复制、有限原生分支和重复安装。

| 教学 | 服务端真实触发 | 下一项 |
|---|---|---|
| 1000至1013 | 原生无条件串联，1013为501入场 | 1014等待 |
| 1014召唤 | 通关504 | 2001等待 |
| 2001邮件 | 通关506 | 2002等待 |
| 2002升格 | 通关509 | 2003等待 |
| 2003契印 | 通关510 | 主教学链结束 |

回归入口：`out/tools/test_login_battle_recovery_script.py`、`test_activity_metrics_script.py`、`test_native_record_bridge.py`；Go `TestFullTutorialGuidePersistence`及真实PG `TestPostgresFullTutorialRecovery`分别覆盖条件拒绝、状态串联、去重与冷登录迁移。离线/实库测试只验证协议和持久化，不代表动画、战斗或教学UI已通过。完整验收仍需MuMu自然通关501至510、各教学交互及中断重登；禁止清数据、跳置完成态或补造奖励。

> 2026-10-08 15:57 登录恢复与10001引导续行已完成组合发布和MuMu复验。生产游戏SHA256为 `9d08d8e7dc6a72ce7de06d3232263f685aecb90b4ba29d3788448e1436dcd24f`，双端热更SHA256为 `c8aea79abac96af14ded564ecffaeef87f133a746236f64a3df4d57118656ac0`，runtime为2026100811，发布报告为 `out/completion-release-20261008-154533-762608.json`。服务端恢复未结战斗后会继续下发原生 `on_refresh_login`，客户端以服务端已建立的battle直接完成登录收尾，不重复请求恢复。真实PostgreSQL复验56 PASS、0 SKIP，另3项辅助通过。匿名玩家实机确认10001前三段引导依次结束；原生表只给10001配置普通攻击、技能、技能详情三段，之后 `local control wait for master input` 是正常自由操作点。实际选择技能并点目标后，客户端上报 `use_skill`、服务端桥回放成功，剧情推进到 `story/00overture/battle/10001_4`；最终胜利结算仍需继续实机完成。

> 2026-10-08T15:31:12.103772 十五组服务器统一交付已完成。默认及正式规则全仓、vet、最终构建和Linux真实原生全部通过；真实PostgreSQL 56 PASS、0 SKIP，另3项辅助通过；游戏/两端热更/私有原生运行时已统一发布并独立核验。Android完整玩法与持续负载仍待验收。当前游戏SHA `ac5710950710fc0852a50f8f16d5cec423da087944fb35c6adfddcbe40fdab3f`，热更SHA `c9bc1595b4fed5e51f90f0b5b8862f896c3670ead9b097ff7848b2b55371bf38`。

> 2026-10-08 14:56 新进程重登战斗恢复契约已补齐并完成MuMu入场验收：旧逻辑依赖客户端已有本地battle才调用 `client_need_recover_battle`，但进程重启后该对象丢失，`on_refresh_login` 会直接启动下一条引导并被服务端以“已有其他战斗会话”拒绝。runtime2026100810在登录刷新前请求恢复，等待 `gworld.get_battle()` 建立后再透传原生刷新参数以清除遮罩；正常无会话时2.5秒回退原生路径。共享服务端源码同时在 `set_reconnect_auth_msg` 发现 `Progress.Battle` 时优先调用同一恢复事务，不下发 `on_refresh_login`。双端热更SHA256为 `c9bc1595b4fed5e51f90f0b5b8862f896c3670ead9b097ff7848b2b55371bf38`；实机日志与画面确认新UUID、prepare四参、实体加载、battle_fighting及可见10001对话战斗场景，生产游戏二进制尚未包含服务端永久入口。

> 2026-10-08 14:39 教程结算卡住修复已发布、待重播验收：10001战斗已经胜利并执行 `on_dungeon_finish`，但活动统计扩展在原生 `battle_end` 清空 `bid` 后调用 `get_total_extra_statistics()`，触发 `KeyError: None` 并中断结果上报。revision 3改为在 `battle_end_notice` 阶段冻结AP与受伤统计，结果回调只读取快照；runtime提升到2026100807，双端清单SHA256为 `839b86d8c930adf95a97d22785fac6a10f17f9e25e1d7ed61671b3f4b324744a`，本地生命周期回归与完整游戏包测试通过。当前客户端已确认热更执行和 `HS_ACTIVITY_METRICS_READY 3`，等待人工重进恢复战斗后验证结算推进。
>
> 2026-10-08 14:35 教程第二层黑屏已修复并完成 MuMu 入场验收：五个战斗入口曾把 `prepare(players,battle_id,seed,dungeon_id)` 的四个位置参数错误包成单个列表，客户端因 `prepare() takes at least 4 arguments (2 given)` 停在加载黑屏。共享源码已统一改为四个位置参数并加入契约测试；战斗桥同时兼容旧服务端的单列表格式，runtime提升到2026100806并双端发布，清单SHA256为 `4086052ec1a7866ef32c9cf7bdf7fe7a187077f6351e0e3ca715645924370c22`。实机确认 `hotfix_suc 2026100806`、四参数 `prepare`、16/16资源预载、`battle_fighting`、`on_battle_start` 与普通攻击教程画面；未替换生产游戏二进制，源码直发修正仍随A–F总发布上线。
>
> 2026-10-08 教程黑屏修复：`dbstore.UpdateAllSocial` 在全服周期结算中会隔离早期版本遗留的“仅有 `avatars`、没有 `avatar_progress`”角色行，不再让单条不完整历史数据阻断正常角色的 `enter_dungeon`。隔离只排除结算对象，不猜测或删除遗留角色数据；真实 PostgreSQL 回归测试覆盖该边界。为即时恢复当前生产服，已在事务内给确认缺失进度的匿名玩家、2写入标准一级初始进度，备份表为 `avatar_progress_repair_20261008_142200`；当前角色匿名玩家未修改。源码防复发修复尚未随另一个会话的A–F总发布上线。

> 2026-10-08 14:10 runtime延迟回调闭包修复已双端发布：正文不变，索引2026100805，热更SHA `0d64d0a782da2d9ab15fd270be2fec744fd67ae45ddaa6e9166932af0c3001d4`。已冷启动到1.0.128登录页，下一次登录验证runtime重播。

> 2026-10-08 14:01热更修复已经MuMu全新数据验收：1.0.125真实整包更新至1.0.128并进入登录界面，不再显示 `-1.-1.-1`或弹出“下载文件出错”。

> 历史单端PVP发布批次（非本轮十五组）：2026-10-08同步机器人、异步真人防守/机器人已接服务端单原生权威并发布；正式PostgreSQL 44项通过、0跳过及2项辅助通过。同步真人沿用既有单权威；普通副本保持Android原生计算。独立远端核验正常。Android双端/完整玩法和持续负载仍未关闭，整份A–F尚未全部完工。

> 2026-10-08十五组同步实现：G01/G02/G03/G04/G05/G07–G15已补源码，G06纠正为本版无入口的历史规则，不新增虚构配方。本轮31条成就都有真实业务链；学会仅此次6条成就及雅努斯基础，不代表完整学会全功能。默认及正式规则全仓、实库、最终构建和组合发布默认及正式规则全仓、vet、最终构建和Linux真实原生全部通过；真实PostgreSQL 56 PASS、0 SKIP，另3项辅助通过；游戏/两端热更/私有原生运行时已统一发布并独立核验。Android完整玩法与持续负载仍待验收。Android完整玩法、双端与持续负载仍未完成。

更新：2026-10-08，北京时间。本文件描述当前源码契约；源码、生产版本与Android验收按证据分别判断。统一交接见 [HANDOFF](out/HANDOFF.md)，剩余施工见 [REPAIR-TASKS](out/REPAIR-TASKS.md)，分层验证见 [PROGRESS](out/PROGRESS.md)。

## 1. 当前状态与证据来源

正式实现为 Go + PostgreSQL，工作目录 .；Android 1.0.128 导出是协议编号、数值、门槛、概率与资产类型的唯一依据。普通副本由Android原生引擎计算；新建同步真人/机器人和异步PVP由服务端私有Python2原生引擎决定命令与唯一胜方，客户端仅负责播放确认。旧 Go 权威教学引擎只作历史对照，不恢复为生产路线。

此前单端PVP生产基线游戏SHA `9621cde391fff0a0238b417f4668c606dd79a9b91ce06c969f4ec795104e881a`，热更SHA `55d2be8356dca5d9cf5fca2b92b143179f65e3777fca825fe6ef97950aabb2bc`（runtime2026100804正文沿用）；回滚备份 `/opt/hs-server/releases/completion-20261008-121214-742541`。正式PG 44 PASS/0 SKIP，产物SHA绑定最新构建；Linux单端/双端原生专项通过。 证据：out/native-solo-release-verification.json、out/server-release.json、out/remote-release-verification.json。

02:40历史检查：全仓test/vet、正式build及真实PG 39 PASS/0 SKIP通过；当时17池尚未批准，02:41预检没有写生产。09时新批次规则已明确批准，全仓/构建重新完成，真实PG为41 PASS/0 SKIP，组合发布已通过。Android/双端确定性/完整玩法/持续负载尚未验收；当时C3映射223条及31条未接，本轮31条已补真实业务链（见17.1）。统一证据out/continuation-final-verification.json，不能写全部完成。 本轮共享奖励、馆主与卡经验、收藏室、活动、社交、录像、技能潜质、管理与邮件接口按各现行章节记录，用户操作MuMu，AI观察日志；不重做F2。

| 导航 | 用途 |
|---|---|
| [逆向交接](out/HANDOFF-REVERSE.md) | 703数据模块、3099脚本模块、来源SHA与静态恢复边界 |
| [表目录](out/client_catalogs/table-index.json)、[RPC目录](out/client_catalogs/rpc-catalog.json) | Record字段、注册、代理与转发调用点；目录数量不等于协议全部完成 |
| [Gate规格](out/gate-protocol-spec.md)、[登录规格](out/login-flow-spec.md)、[RPC语义](out/rpc-semantics.md) | 历史原始取证；日期相关旧状态不能代替本说明 |
| [真实热更记录](out/hotfix-e2e-2026-10-06.md) | 全新安装下载10文件/1.79GB、249秒、真实推进1.0.128 |
| [历史审计](out/AUDIT-2026-10-05.md) | 旧教学/伤害问题的追溯入口，已停止作为开发任务 |

正式源码在cmd、internal；prototype是研究原型，_ref只供语义核对，不能照搬数字。根目录当前不是Git仓库。每次写项目先读全部Markdown，再重读拟改源码，保留来源SHA、错误和验收边界；资料合并进现有四份核心MD，不增加重复长期文档。

## 2. 进程、数据库与事务

| 服务 | 所属位置与职责 |
|---|---|
| hs-game | 游戏服192.0.2.10:22，/opt/hs-server，TCP9000 |
| hs-sdk / hs-login | 同游戏服，HTTP8080/8081；SDK所属443承担项目TLS |
| hs-postgres | 项目专属PostgreSQL，127.0.0.1:15432，库hs、角色hsgame |
| hs-dns | 游戏服项目DNS，UDP/TCP53，按项目后缀回答 |
| hs-hotfix | 热更服192.0.2.20:44905，/opt/hs-hotfix；资源/opt/hs-res |

AscNet、MongoDB、其他监听与服务不属本项目。每次部署检查实际端口、进程可执行文件、目录归属和备份，不凭历史PID操作。数据库口令仅在远端 /opt/hs-server/data/database.env、权限0600；凭据、令牌、私钥不进MD或日志。

[schema.sql](internal/game/db/schema.sql)是嵌入建表定义。accounts保存bcrypt哈希及时间；avatars保存12字节OID、稳定UID、每账号每服唯一角色与资料；avatar_progress保存JSONB state、revision、updated_at；avatar_reconnect保存设备、角色、凭据SHA256摘要和有效期。静态目录内嵌二进制，不把材料定义表当玩家库存，不添加猜测外键。

配置HS_DATABASE_URL使用 [dbstore](internal/game/dbstore/accounts.go)，缺省为开发内存夹具，重启丢失，不能用于生产。建表/连接失败必须退出。玩家资产通过UpdateProgress行锁重读最新JSONB，一次提交材料、卡、契印、任务、收据和标记，任一失败整体回滚。

馆主真实等级同时保存avatars.level与Progress.AvatarLevel，成长/资格使用事务内最新AvatarLevel，不能读连接旧等级。AdminUpdatePlayer同事务更新资料和进度，提交后通知在线重读。登录初始化、订阅与活动恢复必须返回错误，禁止丢弃错误后假成功。

初始状态来自 [player_defaults.json](internal/game/player_defaults.json)：init_config材料仅首次发放，初始幻书4401、体力100、guide1000执行态；迁移保留已有资产。材料Count为余额、Total为累计获得，消费只减Count。契印迁移保存原件/隔离原因，不猜旧编号，不重登反复补奖。

## 3. 原生传输、登录与重连

帧为4字节小端长度＋2字节小端命令＋protobuf，长度不含自身、包含命令号；默认单帧负载上限1MiB。握手为seed_request/seed_reply、RSA-OAEP/SHA1 SessionKey、已RC4加密的session_key_ok、connect_server/connect_reply、实体创建。收发各有独立连续RC4状态，响应和Tick推送整批加密/写出，防止流顺序错乱。

EntityMessage上行raw原名优先，raw为空才查index；下行用原名。parameters为msgpack字典_0、_1…，零参数{}，不能直接传JSON文本或数组。ObjectID为ExtType42、12字节；Float时间保留小数。Map保留整数键、ObjectID键、tuple键；收藏室解锁二元组与格子五元组不能改为Python repr字符串。initialcreate和运行期wire均递归转换结构体/容器，不能落到fmt.Sprint字符串。

当前关闭zipped_channel；压缩、完整routes/localid优化、全部复杂类型不能仅凭定义存在称验收。业务只能操作当前连接绑定Account/Avatar。完整帧续期120秒读空闲期限、15秒写期限；半帧不续期，持续心跳不再触发旧固定120秒断线。

| 方法或动作 | 当前契约 |
|---|---|
| quick_login(client_info) | register_info代表显式注册；否则先认证既有账号，仅ErrAccountNotFound进入设备快速注册；错密不覆盖口令 |
| register_login(client_info) | 本项目演练扩展，显式注册；原Android主入口仍quick_login |
| sdk_login(client_info,sdk_info) | 网易票据鉴权未接，拒绝9002，不伪造第三方登录/支付 |
| 登录成功 | login_result→on_get_all_avatars→on_hotfix_when_login→创建所选Avatar |
| BecomePlayer | Go内部动作，登记在线连接、推时间；不伪造未装饰on_login_success/on_become_player |
| set_reconnect_auth_msg(authmsg) | 已鉴权Avatar保存设备/角色/凭据摘要，首次绑定收尾后推on_refresh_login |
| query_server_time() / heart_beat(last_send_time) | 前者空参；后者回两个Float：服务时间与原发送时间 |
| guide_task_finished(callback,task_id) | 已激活任务事务完成/激活下一项，重复幂等，callback(Bool,Str)，不假完成 |
| upload_guide_tasks(callback,tasks) | 校验记录任务结构，不覆盖资源或任意越级完成态 |
| set_nickname_gender(callback,nickname,gender) | 创建任务1000首次资料，2—9字/同服唯一/性别1或2，回Int码 |
| set_global_vo / set_story_vo(callback,value) | 原生允许值及持久，先属性同步再callback(Bool,Str) |
| start_speed_check() / speed_check(type,dungeon,fps) | 原生轮次与无回包上报；无回包不是未处理 |
| query_league_message_board() | 未入公会发真实空LeagueMessageBoard，不能称完整公会业务 |

账号最长256字节，密码1—72字节，hostnum正数；bcrypt，登录上下文3秒，事务内无外部网络调用。未知/重复账号相应9012，错密/SDK拒绝9002。sync_server_time时区-28800，经客户端取负为UTC+8，不能传8、+28800或整数化Unix时间。

普通重连ConnectServerRequest type1，deviceid、原Avatar12字节entityid、authmsg必须匹配保存摘要和24小时有效期；成功type2恢复原Avatar，失败type3，不新建角色。历史实机非战斗同OID恢复不代替当前战斗恢复验收。

## 4. 客户端战斗桥、成本与完整奖励

[battle_bridge_script.py](internal/game/battle_bridge_script.py)为revision12、握手1；do_command("__battle_event__",[envelope])携带battle_uuid、从1连续sequence、kind、data。ready前不接受其他事件；started校验单位快照；result必须有显式winner_eids，可选player_eid/outcome兼容旧桥。客户端不能上传奖励盒或材料数量。

事件审计最多128条/2MiB、单事件512KiB，不是完整回放。胜负标记client_authoritative=true、verified=false；会话结构校验不是独立反作弊验证。

enter_dungeon(callback,dungeon_id,extra_info)校验引导、原生停用/解锁/活动资格，仅新会话扣费，准备态同副本重入复用。保存UUID、seed、阵容与实付后，脚本→start_server_battle_ok→set_last_fighting_cards→prepare→速度/自动偏好；send_loading_percent/load_entity_finish/battle_fighting走实际加载链。目录1461条不是1461玩法验收。

基础体力取Android D81B4FE0.get_need_power：副本need_power非None取自身，否则回退activity_type.need_power，缺字段不是免费。初音手册按激活效果ceil(max(0,base*(1-sum)))减免，不猜其他加成。普通need_materials和活动明确消耗在新会话事务扣除，PaidPower/PaidMaterials/活动Spent冻结实付；重入/恢复不重复收费。纯剧情节点也校验资格与成本。

普通胜利发首次资格first_bonus＋普通bonus；失败按冻结实付返还。2026-10-08接续已修普通、同步AI、异步已结束恢复：全部属性和原盒在事务提交后组装，不再推连接旧余额；SettlementBox经JSONB重载恢复材料整数键、卡/契印ObjectID和原FinishedTaskList。坏盒明确拒绝，不重抽/不重复发奖。13项定向及夏活/初音2项恢复测试通过；新增真实PG与Android仍须独立验收。

共享内部API位于 [shop_bonus_native.go](internal/game/shop_bonus_native.go)，必须由调用方放入玩家事务：

    grantNativeBonus(p,id,amount,level,now) -> (box.box,error)
    grantNativeItem(p,itemID,amount,level,now,changes,cardIDs,depth) -> error
    grantAvatarExp(p,amount,now) -> error
    grantCardBattleExp(p,uuids,amount) -> (receiverUUIDs,error)

D44DBBA7 bonus_utils证实先概率、按count_list取整数数量、按资格与权重逐项选物品库，不能整库全发。box含实际materials、新卡cards、新契印runes；严格容量、溢出与嵌套深度。

| 资产 | 实际入账 |
|---|---|
| 1体力 | settlePowerRecovery后增Power.Value，显式超额保留，超额不积累自然回复 |
| 2馆主经验 | Android等级1—60与阈值，真实等级/等级内经验/PowerMax，不凭空加满体力 |
| 3幻书经验 | 原生box.get_card_exps给每个冻结接收UUID完整同额，不分摊、不写库存、不按exp_factor缩放 |
| 4知识储备 | Materials[4].Count唯一余额，exp_pool投影，升级抵扣实例已有exp |
| type6/8/9/13 | 家具/卡/头像框/装束真实所有权，不一律当普通材料 |
| 206001妄言补给 | grantWangyanSupply真实Supply；time_auto_attr.add默认auto=False，显式超Max保留 |

卡经验只给冻结真实拥有UUID，去重，剧情/外部助战不写本玩家卡；受品阶与馆主门槛约束，未解锁下级最多当前阈值-1，满品阶经验归零。box.card_exp_receiver为ObjectID列表，与经验/首通/收据同事务。

client_need_recover_battle：已有结果补原结算；未结束新UUID、保留阵容/seed/上下文及实付，从准备重开。当前源码已补脚本→start_server_battle_ok→阵容→prepare→偏好完整加载，Loaded/Started重置；不恢复中途HP/AP。PVP使用授权恢复分支，详情由社交章节补充。finished_dungeon_event(dungeon_id)为原生单整数、无callback上报，不重复发奖/推进通关。

## 5. B基础玩法接口与好感度

| 原生上行 | 下行与约束 |
|---|---|
| embed_rune(callback,card_uuid,rune_uuid) | callback(ret)，拥有/部位/词条校验，同部位旧契印卸下 |
| unembed_rune(callback,rune_uuid) | callback(ret)，原生只带契印UUID |
| up_level_rune(callback,rune_uuid,count) | callback(ret)，逐级成本/门槛/词条解锁，保存旧因子 |
| decompose_runes(callback,rune_uuids) | callback(box)，不追加ret；锁定/佩戴拒绝，批量原子删除/发奖 |
| lock_rune / unlock_rune(callback,rune_uuid) | callback(ret)，wire.lock为Int0/1 |
| change_rune_extra_attr(callback,rune_uuid,index,attr_id) | callback(ret)，五星表内洗练、真实成本，非法/不变拒绝 |
| set_captain_card_id(card_id,dress) | 无callback，模板ID/外观归属/拥有/觉醒；推common与captain_id |
| set_layout_cards(dungeon_id,battle_cards) | 无callback，fighting/support/storyline三列表与空槽，分副本保存/开战冻结 |
| receive_intimacy_bonus(callback,card_id) | callback(ret)，每次下一原生章节，不传章节号，奖励/援护等级同事务 |
| consume_intimacy_gift(callback,card_id,material_id,count,choice) | callback(ret,box)，choice0普通，非0特殊单份；属性先同步 |
| consume_multi_intimacy_gift(callback,card_id,materials) | callback(ret,box)，严格正整数字典，材料/容量全批校验 |
| signin_monthly() | on_signin_monthly(ret,box)，UTC+8、月天数/跨月、同日不重发 |
| random_cards(callback,pool_id,1或10) | 原生卡池/扣料/稀有度保证/首抽奖励一次，2000卡容量，奖励box |
| compose_cards(callback,material_ids) | callback(ret,cards,first_card_ids)，77配方、至多50卡，全消耗/生成原子提交 |

契印wire含uuid/star/pos/suit/extra_suit/level/base_attrs/extra_attrs_count/extra_attrs/extra_attrs_lib/extra_attrs_factor/card_uuid/lock/create_time；nil列表转空列表，未佩戴null，无装备显式空embed_runes。GenerateRune/GrantRune/GrantRuneOnce及Marks版本生成真实UUID与属性；Once同规格收据重试不补发、冲突拒绝。迁移原件、版本、隔离持久，不猜缺失模板。

B5最高好感32000；523/524每份四位置各一枚五星1901，原生标记词条与自动锁。特殊每日每卡/全卡次数与累计SpecialCount分开，累计剧情序号不清空；UTC+8日界为明确本服策略，回拨拒绝。

首次偏好发现已补：DAE1C2E7.favor_gift按gift.like_tag==cards.tag[1][0]匹配，成功赠礼且旧LikeGift=false才发现，不是“答案正确即解锁”。整笔冻结旧标记算收益，全部材料/回礼成功后永久true，下一笔用偏好收益；特殊包装重读common再累次数，避免覆盖新true。4401匹配749，首次特殊choice3为20*1.2=24；523无标签，正确选项也不解锁。

赠礼小数分支现采用批准本服有理数half-up，真实非好友助战亦已接名册授权与计次；G06本版无随机碎片材料入口。详细现行规则见17.3/17.4，原生UI和跨日验收仍保留。

## 6. C1/C2预设接口

阵容20个稳定ObjectID槽，删除内容保留槽位，名长最多7字符、出战4/援护2；普通、同步PVP、mini_team分别保存，后者按六职责，不误套4+2。

| C1上行 | 回调与保存 |
|---|---|
| cover_preset_record(callback,preset_id,cards) | callback(ret)，三列表/空槽/同名去重/拥有与禁用 |
| del_preset_record(callback,preset_id) | callback(ret)，保留槽位 |
| update_preset_name(callback,preset_id,name) | callback(ret)，名称限制 |
| update_preset_index(callback,src,dest,after) | callback(ret)，方向移位，不是交换 |
| set_sync_pvp_preset_record(callback,index,cards) | callback(ret)，按真实分数槽数，允许空组 |
| activity_set_preset_record(callback,activity_name,cards) | callback(ret)，mini_team六职责、无空槽 |

契印预设最多100组、四位置整数键、名称7字符，原生runes_templates投影；旧异常quarantine，删除/分解清悬空UUID，应用替换整组，空槽卸旧。

| C2上行 | 回调与保存 |
|---|---|
| add_runes_templates(callback,old_uuid,rune_uuids,name,tips) | callback(new_uuid,msg)，新建/覆盖 |
| change_runes_templates_name(callback,uuid,name) | callback(ret,msg) |
| delete_runes_templates(callback,uuid) | callback(ret,msg) |
| update_runes_templates(callback,uuid,rune_uuid,pos) | callback(ret,msg)，空UUID清槽 |
| set_top_runes_templates(callback,uuid,card_id,untop) | callback(ret,msg)，拥有卡与持久top_times |
| embed_rune_by_template(callback,uuid,card_uuid,is_battle_layout) | callback(ret,msg)，兼容最后参数有/无，原子卸旧/装新 |

源码：[formation_preset_business.go](internal/game/formation_preset_business.go)、[rune_preset_business.go](internal/game/rune_preset_business.go)。已有历史发布记录，原生UI与跨模块清理以本轮验收为准。

## 7. C3—C10成长、图鉴、商店与统计

成就254条/104奖励，组内求和、组间取最小，真实历史/标记持久。receive_achv_bonus(callback,id)、receive_all_achv_bonus(callback,ids)为callback(ret,box)，未完成6002、已领6004；批量失败全回滚。登录/副本/抽卡/送礼/成长/收藏室及社交活动以实际接线为准，未知事件不补旧历史，目录数不代表全部目标闭环。

家具/主题图鉴记录历史获得，消耗不取消发现，领奖校验完整性；初音activate_handbook_item与夏日钓鱼手册由活动负责，不重复在收藏室发奖。

头像187/框49为当前核对目录，默认head1/head_box3；change_head(callback,id)、change_head_box(callback,id)、update_head_box_by_client()按真实拥有/期限选择，合法旧选择保留，未获得付费/活动资源不全开。只有框2/4/5有unlock_target，均登录目标1000，按真实登录日解锁。

特殊头像框现按领取起算、原小时、多份续期及合法选择规则处理，80040为1小时；详见17.3。

| 商店原生上行 | 业务/下行 |
|---|---|
| buy_commodity(commodity_id,count,select_material_id) | 无callback；on_buy_commodity(ret,box,id,select_material_id)，末参不是数量 |
| reset_gift_box(material_id) | 无callback；on_reset_gift_box()，原生23项按剩余量/权重抽取 |
| update_recommend_gift_state(id,state) | 点击/状态、展示与到期约束，不凭空生成报价 |

35商店/881商品支持普通、30日订阅、显式推荐报价、随机礼盒、家具、契印、自选、装束。阶梯价按每日购买索引取最后档、折扣截断；日/ISO周/月/总/活动限购、批量、时间/系统/条件严格检查。成本、奖励、次数同事务；原生无请求UUID，同参数不能自动当网络幂等。条件组内或/组间且，kind11用Total、kind12用真实类型/子类型完成次数。

月卡2010001立即发原生200付费币，后29个UTC+8日期发200116邮件，唯一MID/序号/收据防重；refreshShopSubscription供登录/tick，邮箱满回滚，多日补算只发有效邮件，过期应发日推进收据。续购为独立30日任务，是当前明确实现，原服重叠/串联运营规则不在APK。

推荐gift.id/price/show_duration/expired_time与SHOW/CLICK/BUY原生保存，经后台recommendation_issue明确发行、审计、在线同步；外部推荐算法缺失，不猜个性折扣。7/15日商品需HS_SERVER_OPEN_TIME实际开服Unix秒，缺省拒绝，不以玩家创建日/当前日替代。

随机库按原概率/权重/数量区间；1011000原生UI容器购买实际子商品。原全零suite缺原服选择器；用户已批准17组各18有效套装等权1，正式HS_RUNE_FALLBACK_POOLS已绑定批准SHA，默认无配置拒绝，不把候选提案当运行配置。

G06审计纠正：规则2/3是未引用历史表，不是两条现行配方；77条固定配方与无材料入口事实见17.4。商品1070003条件kind3引用章节118，chapters无118的旧目录边界保持，不能偷换副本118。

consume_ring(callback,card_id,material_id)按820→ring1、拥有/禁用/好感6/已领章6，一次扣费、同名common共享；update_level_one(callback,uuid)仅本人品阶5/等级60/已誓约/未突破实例，上限70，均callback(ret)。up_level_card(callback,uuid,fast)检查知识/原exp/品阶/真实馆主；upgrade_card(callback,uuid)按该品阶满级及全项成本；card_enhance(callback,uuid,materials)按rarity成本、同名1+EnhanceCount/通用卡4、最多8阶段、真实移除与解绑，callback(ret,box)。

obtained_card_ids为首次获取历史，新卡共用，删除不撤销；旧已删除且无证据历史不恢复。cards_count为当前启用/not_in_stat允许的唯一模板数，last_main_chapter_dungeon_id仅普通主线最高真实通关节点，achv_value为实际已领奖积分；不拿活动最大编号或假历史填充。

### 7.1 技能升级与潜质升级（接续原生证据）

源码card_skill_talent_business.go、card_skill_talent_catalog.json；完整原生表SHA、调用点、成本、所有34指定节点回归在out/card-skill-talent-report.json。8张原生表合并为1529技能匹配行/54潜质模板，不使用参考服数值。

| 原生接口 | 请求与响应 | 验证及事务 |
|---|---|---|
| upgrade_card_skill | callback整数、card_uuid ObjectID、skill_id整数；先推card_mgr/material_mgr，再call_client_callback(callback,[ret]) | 只能自有可成长卡；原生role_info技能顺序、card_grade、forbin、最大级；skill_upgrade[(upgrade_id,当前级)]扣真实材料后升一级。没有有据成本拒绝，不免费升级 |
| upgrade_talent_node | card_uuid、branch（0/1/2或101）、零基index、currentLevel四参，无callback；on_upgrade_talent_node(ret,branch,index,原请求currentLevel) | grade/level、真实前置、forbid/节点最大级、全树次数额度；额度来自品阶/等级/已领奖好感/补完/誓约。currentLevel必须等于持久has_upgrade_time，重复或过期请求整笔拒绝 |

Card.Skills持久为原生skill_mgr列表（skill_id/level/enhance_level整数）；TalentTree为整型分支键与节点列表（node_id/state/has_upgrade_time）。技能level1/2→skill_grade1、3/4→2、5→3；旧缺失容器只补原生未升级结构，已保存技能与潜质保留，不补造历史成就。skill请求没有幂等UUID或期望等级，连续合法请求会再次升级并收费，callback不能当持久收据。潜质响应末参回传原请求，GUI依据先推card_mgr的新状态刷新。

新建卡此前gradeByLevel错误地返回level；现按原生cards_max_level映射：1–10级→grade0，11–20→1，21–30→2，31–40→3，41–50→4，51及以上→5，仅用于新建，不倒推改旧存档。初始grade0的第三技能仍按原表grade1门槛锁定。邮件初始模板和成长材料夹具须同此原生语义，不允许以兼容旧测试恢复错误品阶。

C3原真实升级仍记type27技能阈值、type24同卡三个战斗技能五级及type59指定34潜质节点；本轮reset_talent_node已按原生重置一级/返额度/不返材料接事务。本轮31条真实业务接线及范围见17.1，完整Android目标验收保留。

## 8. C8收藏室与加工

[collection_catalog.json](internal/game/collection_catalog.json)含3房间/8设施/415家具/24主题与原生平面/空洞/体积/生产/性格规则。初始23项家具不代表免费房间；房间1钥匙809、2钥匙810，3缺原生钥匙来源。解锁/升级真实成本，入住拥有/唯一房间/容量/舒适/装束校验；布局各房总库存、重叠、footprint/体积/空洞、120件门槛。

| 原生上行 | 下行/约束 |
|---|---|
| player_enter_house() / set_restroom_index(room_id) | on_enter_house()；房间选择保存 |
| unlock_dormitory(material_id) | on_unlock_dormitory(ret,material_id)，真实钥匙 |
| unlock_facility(material_id) / upgrade_facility(id) | on_方法(ret,id,box) |
| dormitory_change_house_card(room,infos) / set_restroom_girls(room,card_ids) | on_dormitory_change_house_card(ret)，原生位置/装帧 |
| house_card_change_dress_id(card_id,dress) | on_house_card_change_dress_id(ret,card_id,dress) |
| receive_restroom_exp(callback,room_id) | callback(room_id,exp)，5秒增长/小数余量/1000上限 |
| update_visiting_setting(allow,show,room_id) | 保存真实布尔和已拥有房间 |
| setup_furniture(room,plane,id,orientation,xy) / withdraw_furniture(room,plane,layer,xy) | on_update_restroom_grid_info(ret) |
| update_restroom_grid_info(keys,values,inventory) / update_restroom_grid_wallpapers(keys,values,inventory,walls) | 五元组键/二元值，上传库存不能覆盖真实库存 |
| exchange_furniture(material_id,furniture_id) | on_exchange_furniture(ret) |
| receive_furniture_handbook_bonus(id) / receive_furniture_theme_handbook_bonus(id) | on_方法(ret,box) |
| receive_all_furniture_handbook_bonus(type) / receive_all_furniture_theme_handbook_bonus() | on_方法(box)，原子领取 |
| gather_produce_material(facility_id) | on_gather_produce_material(ret,id,[produce_id,count],box) |
| facility_card_upgrade / get_facility_card_reward / upgrade_house_facility_card | 培育三接口，完整签名见collection_card_facility.go |
| unlock_furniture_compose() / compose_furniture_normal / compose_material | 开放on_unlock_furniture_compose(ret)，家具callback(ret,box)，材料callback(box,msg) |
| get_house_reward() | on_get_house_reward(List ret)，每房最高priority合法公式 |
| set_up_furniture_material_id(callback,id) / reset_up_furniture_material_id(callback) | 真实候选心愿单、默认0，不误用材料35 |
| compose_furniture_special(callback,materials) | 恰3家具/舒适档/实际成本，callback(ret,box) |
| update_last_get_time(room_id) | on_last_get_time_update(room_id)，服务器时刻/拥有房间 |

生产先结算旧速率再升级/入住，保留Keep小数，storage cap停产；入住收益×4，同性格效果取最大不乱叠加。设施2产19、3产20、4产70—73是bonus。培育9按100进度、20/40/60/80/100阶段收据全领再升级轮次；加工7的84配方/原料组/入住手续费floor，已摆放家具不可消耗，图鉴历史不删。4级加工及19*2000000一次开放家具合成。

每日基础get_house_reward按房间/house_daily_reward/已通关最高priority公式入知识，UTC+8每房日收据，重复/溢出回滚。桌游每日get_house_board_game_reward由D8发effect11骰子，收藏室不再重复401；D8是残页梦境骰子小游戏，不是入住。

限定融合保留舒适90/120/150、19×30000费用、98/2或92/6/2和心愿60%；八组组内等概率已明确批准并写正式HS_FURNITURE_FUSION_POOLS，不自动加载提案。现行规则见16.1/17，原始候选提案仅保留来源。

收藏室接口数量不代表Android验收；本轮实际能量、住客、房3与原件9000秒教程均已补，规则/API/持久收据与剩余实机边界见17.5。

- 教程原件已恢复：6004/26004实际图只有facility2/9000秒，一次真实进行任务收据，6005/6006没有节点；不开放任意时间加速（17.5）。
- 住客原生两时窗/79卡/mood条件与奖励项保持，批准本服首次进入真实候选等概率及完整冻结箱已实现；原表第二整数仅保存raw_second，不作倍率（17.5）。
- 房3真实104系统后按批准规则一次ownership，能量按原cost/per式与生命周期持久实现；不造3钥匙或免费额外生产（17.5）。

## 9. 邮件资产、冻结随机结果与在线通知

MID为ObjectID，short_mail_info用ObjectID键/short_mail对象，保留has_attachemnt拼写；标题正文custom_text为%s模板＋args，百分号字面值保留。内部未读/待领/结束wire为1/2/3，保存发件人、创建/修改/有效时间与附件审计。

| 邮件接口 | 契约 |
|---|---|
| query_mail_content(mid) | 无callback，on_query_mail_content(mid,raw_mail) |
| read_mail / receive_attachment / delete_mail(callback,mid) | 状态/已领/过期校验，未领有效附件不能删除 |
| receive_all_attachments(callback) | 有效未领资产同事务，容量/溢出任一失败全回滚 |
| IssueMail(ctx,connection,Mail) | 在线内部连接锁及提交后通知；已持锁/玩家事务不能调用 |
| prepareAndInsertMail(p,Mail) | 事务内发件/月卡，不再次锁连接 |
| POST /admin/mail/issue | avatar_oid/receipt/operator/reason/mail，严格JSON、指纹与并发一次发件 |

标题80字/正文4000字/发件人40字/材料附件20项/邮箱500封，校验有效期、MID/UUID、未领卡/契印UUID冲突。ExpiresAt0兼容旧永久邮件。发件收据/结果/审计一次保存，同收据异内容拒绝，删除后同收据不重发；10000收据容量满需归档，不自动清空。

固定附件已接type1材料1、type2材料2/4、type3/4库存、type6家具、type8初始卡、type9框、type13装束，领取统一grantNativeItem；限时从领取起算。type7随机/动态礼盒需冻结展开规则，未开放任意固定附件；材料3无接收卡不能冒充通用邮件经验。

真实Card/Rune实例附件UUID及card_dict/rune_dict仍支持；卡限定合法初始等级/品阶/技能，无额外觉醒誓约装束；契印Android模板/词条校验。领取同时检查固定物品与实例附件容量/重复，同步真实card/rune/common/collection/框等变化。

BecomePlayer登记在线表、Detach清理，发/读/领/删推进revision；提交后跨连接通知释放c.mu再发布，Tick先short_mail_info再notify_new_mail，领/删不触发新邮件声，旧版本不覆盖新邮箱，离线重登读取。

## 10. 玩家管理API、界面与审计

默认关闭；HS_GAME_ADMIN_BIND须回环，HS_GAME_ADMIN_TOKEN至少32字节，启动/每请求都检查来源，不用转发头放宽。数据/写请求Authorization: Bearer；令牌不进URL/持久浏览器存储。/admin与/admin/ui.js是无玩家数据的回环静态中文页，数据仍认证，响应no-store。

| HTTP接口 | 请求/结果 |
|---|---|
| GET /admin/players?q= | 真实角色检索，至多100行，OID/UID/服/昵称/等级/性别 |
| GET /admin/players/{oid} | 资料/JSONB/管理收据审计/当前截断战斗记录，不返回口令/重连凭据 |
| POST /admin/player/update | avatar_oid/receipt/operator/reason/operation＋操作专用字段 |
| POST /admin/runes/grant | spec(suit,pos,star,level,extra_suit)，幂等真实生成与审计 |
| POST /admin/mail/issue | 多资产邮件/收据指纹/在线通知 |

player/update操作：profile改合法昵称/馆主等级/性别；material_add/material_set普通库存，减Count不倒扣Total、增加差量计Total；card_grant启用卡模板1—50份及2000容量；recommendation_issue发行合法推荐报价。不能混不相关字段；进行中战斗冻结资产，管理变更等待结束。

完整请求、SHA256指纹、operator/reason、时间、操作和结果同角色行锁提交；同收据同内容返回旧结果，不重复发；冲突422、收据容量满拒绝。首次201/重试200，参数400/无认证401/非回环403/业务422/内部500/存储不支持503。operator是共享令牌审计标签，不是独立管理员身份。

中文UI支持检索/详情、资料、材料、卡/契印、邮件、报价、审计、当前事件查看。当前128条记录不是完整二维/全事件回放。源码：[admin_player.go](internal/game/admin_player.go)、[admin_ui.html](internal/game/admin_ui.html)、[admin_ui.js](internal/game/admin_ui.js)。

提交后publishPlayerRefresh登记，在线下一Tick重读最新资料/资产并推原生属性，离线重登，不能推旧连接快照。活动特殊材料的管理语义仍须按类型审查，不能把专门账本扩大当普通库存。

## 11. SDK、热更、演练与配置

SDK GET /v1/route?version=按HS_HOTFIX_VERSIONS精确匹配或HS_HOTFIX_REQUIRED=1返回mode/address/reason；POST /v1/login为五分钟闲置管理会话，无网易票据交接，与原生TCP分开。日志兜底不是支付/实名业务。

| 热更路径 | 响应 |
|---|---|
| /pl/patch_hotfix_data_pub | base64(JSON版本键→Python源码)，禁缓存 |
| /pl/patch_list_pub_android.txt | version/base_version/npk/0patchpath/0hash有效，不空清单 |
| /server_list_public.txt | UTF-8空格列，gate=cols[nettype+8]、ip:port、尾行network=bgp |
| /game_notice/notice_formal | XML |
| /resources/... | GET/HEAD、Range206 |
| /v2/?domain= | status/domain/addrs/ttl |
| /applog、/appdump、/upload_patch_log | 请求上限1MiB，200空响应 |

DNS UDP/TCP、后缀A映射，不公共递归；netease/easebar热更、r18sex.net游戏。TLS校验链，历史root CA/转发不是免root分发。F2闸门包用户已确认完成，禁止重做/清数据/旧root注入覆盖。

两个热修通道都需完整幂等源码；runtime索引单调，90秒query_hotfix，缓存清除重播LAST_SCRIPT。基础1.0.125经官方补丁真实推进1.0.128，不能热修改戳跳下载。0hash、script覆盖/逐文件MD5见真实热更记录，配置启动缓存后需所属服务重启。

2026-10-08修复补充：启动脚本必须把 `orig_import`、导入钩子及所有后置扩展包在同一 `_hs_install()` 闭包中，不得使用exec回调离开后失效的全局 `_hs_import`。补丁清单与资源必须同时校验：`script.npk` 为65484字节/MD5 `0dd6e606a1a8cdacbda00d85ef59f49f`，`scenewd2.npk` 为272852892字节/MD5 `179e9787b0e58596824d6f8dcda19328`。后者曾被39616512字节错误文件覆盖，导致尾段Range请求返回416；错误文件备份为 `/opt/hs-res/patch_pub.android_1.0.128a588/scenewd2.npk.bak-39616512-20261008-1353`，15MB错误script备份为同目录 `script.npk.bak-20261008-1350`。

当前生产runtime.index=2026100807，双端热更SHA256为 `839b86d8c930adf95a97d22785fac6a10f17f9e25e1d7ed61671b3f4b324744a`。启动脚本使用 `_hs_install()`，runtime正文使用 `_hs_runtime_install()`，使延迟回调保留兄弟函数的闭包引用；战斗桥 `prepare` 兼容旧二进制的单列表参数，活动统计revision 3在结果清理前冻结快照。生成器支持 `--startup-only` 和 `--runtime-only`；`deploy_runtime_wrapper.py` 会强制校验发布前SHA/索引、两端字节一致、临时文件哈希，只包装当前生产runtime而不带入本地并行开发正文。本次备份为 `/opt/hs-hotfix/data/hotfix.json.bak-20261008-143813` 与 `/opt/hs-server/data/hotfix-20261008-143826.json`。MuMu全新数据实测10文件/1,789,076,496字节，375秒完成，`update_status=1`、`init.on_patch_finish 1.0.128`；后续实机登录已确认runtime2026100807执行成功。

HS_GAME_DEBUG_BIND仅回环、默认关闭；/debug/session、/debug/rpc、/debug/become-player、DELETE会话为临时内存演练，五分钟闲置与容量，不替代真实客户端。

| 配置 | 默认/规则 |
|---|---|
| HS_SDK_BIND / HS_LOGIN_BIND / HS_GAME_BIND | :8080 / :8081 / 0.0.0.0:9000 |
| HS_GAME_ADDRESS / HS_GAME_DOMAIN | 本地127.0.0.1:9000 / r18sex.net，远端实际配置 |
| HS_GAME_RSA_KEY | PEM私钥，空仅明文SessionKey联调 |
| HS_GAME_DEBUG_BIND / HS_GAME_ADMIN_BIND | 空关闭，启用须回环 |
| HS_GAME_ADMIN_TOKEN | 至少32字节，私密来源 |
| HS_DATABASE_URL / HS_DATABASE_MAX_CONNS | 空夹具，池默认32，远端历史16以env为准 |
| HS_TEST_DATABASE_URL | 独立schema真实库，缺省SKIP不算通过 |
| HS_HOTFIX_BIND / HS_HOTFIX_TLS_BIND / HS_TLS_CERT / HS_TLS_KEY | :8082 / 可选TLS / 显式证书与密钥 |
| HS_DNS_BIND / HS_DNS_ANSWER / HS_GAME_DNS_ANSWER | 可选UDP/TCP、热更/游戏IPv4 |
| HS_HOTFIX_ADDRESS / HS_HOTFIX_VERSIONS / HS_HOTFIX_REQUIRED | 管理路由目标/精确版本/1强制热更 |
| HS_DATA_DIR / HS_MAX_SESSIONS / HS_MAX_FRAME_PAYLOAD | deploy/data / 500 / 1048576 |
| HS_ACTIVITY_SCHEDULE_FILE | 显式运营日程，部署匹配客户端扩展 |
| HS_SERVER_OPEN_TIME | 实际开服Unix秒，缺省不开7/15日商品 |
| HS_RUNE_FALLBACK_POOLS | 全零suite本服池，未批准不得开启 |
| HS_FURNITURE_FUSION_POOLS | 融合组内本服池，未批准不得开启 |

## 12. F3真实证书续期

[renew-hs-certificate.sh](deploy/renew-hs-certificate.sh)、[service](deploy/hs-certificate-renew.service)、[timer](deploy/hs-certificate-renew.timer)已安装，不沿用“仅公共certbot.timer”的旧结论。每日Asia/Shanghai03:30＋最多1800秒随机延迟，Persistent=true，flock防并行；超过30天有效即退出，不停止SDK。

到期只确认所属tls目录/SDK可执行文件/443，备份cert/key后短停hs-sdk；项目lego TLS-ALPN-01续签原24个r18sex.net域名，300秒超时，SAN集合与公钥匹配校验、原子替换、恢复SDK、确认443/TLS链。失败trap恢复旧证书并尝试恢复SDK，systemd与daemon.err留故障；不停止AscNet/Mongo/其他项目服务。

2026-10-07 23:15—23:16已真实verify-renewal ACME验收，新序列0534CD2B11BB4EB23B8C04C7604B76E14BC8，UTC2026-10-07 14:17:27至2027-01-05 14:17:26（北京时间到期2027-01-05 22:17:26）；24SAN不变、SDK恢复、TLS成功、游戏哈希仍e976ee89…。见 [真实报告](out/certificate-real-renewal.json)、[日志](out/certificate-real-renewal.log)、[安装报告](out/certificate-renewal-install.json)。验收临时90天窗口仅真实签发验证，日常timer仍30天；失败告警为本机日志，外部通知渠道未声称完成。

## 13. 验证、发布与回滚

Windows显式系统CMD、login=false、中文UTF-8。用户已撤回旧CET包装器，不恢复它，不将默认PowerShell启动故障当远端问题。长任务保存session_id并等退出，不将“运行中”记成功。

~~~bat
cd /d .
go test ./... -count=1
go vet ./...
python out\tools\build_completion.py
python out\tools\test_remote_database.py
python out\tools\server_ops.py audit
rem 先只读预检；17零池没有明确批准时阻断，无生产写入
python out\tools\deploy_completion_release.py --check
rem 仅在正式批准规则/最新构建/真实PG/预检均满足后发布
python out\tools\deploy_completion_release.py
python out\tools\verify_remote_release.py
python out\tools\verify_battle_bridge_hotfix.py
python out\tools\verify_handoff_docs.py
~~~

build_completion.py包装build-game并绑定源码/配置/产物；build-game仅游戏/数据库测试，build.cmd为四服务。真实PG先校验已完成正式构建，上传dbstore.test及其绑定hotfix.json至私有verification/workspace，工作目录对齐internal/game/dbstore以加载真实配置，双方SHA必须匹配；TestPostgres与非PG夹具PASS分别统计。创建清理hs_test_隔离schema，不写生产玩家；脚本自写database-test.log，不重定向同名。SKIP不是PG通过；缺CGO/GCC的-race拒绝也不是通过。

旧单游戏release遗漏活动和热更配置。deploy_completion_release.py要求当前正式构建对应真实PG退出0/PASS>0/SKIP0及双方产物哈希，绑定游戏/日程/双热更/可选运营文件原始字节；必需全零池编号来自构建绑定的原生shop_catalog，不由proposal决定，套装/权重严格验证。两主机常驻SSH/flock锁互斥，prepare/提交/回滚均复核canonical路径、unit/执行文件归属；全部备份/临时哈希成功才提交。任一提交失败尝试逐文件及两机恢复，现场第三方hash拒绝覆盖；新进程失败不阻止所属旧文件恢复，仅重启hs-game/hs-hotfix。17项正式Batch故障模型通过，报告out/completion-release-safety-report.json；未做真实双主机故障回滚。build_completion.py记录构建前后源码及配置/产物SHA，漂移退出125；未知运营规则不加载，17零池无批准时阻断发布，回滚保留PG/玩家和兼容新增表。

本轮TestIntimacy、Shop、Collection、NativeCraft/Enhance、Knowledge、LimitedFrame、活动/社交/真人/录像/技能潜质等均由最新全仓回归覆盖；正式真实PG 39 PASS/0 SKIP见out/remote-database-verification.json及database-test.log，属于独立schema业务验收。新原生记录、租约、完整排名、邮件周期、品阶/技能等新增用例真实执行；不等于AndroidUI、双端或生产负载通过。


### 13.1 接续验证与维护证据

02:40历史检查：全仓test/vet、正式build及真实PG 39 PASS/0 SKIP通过；当时17池尚未批准，02:41预检没有写生产。09时新批次规则已明确批准，全仓/构建重新完成，真实PG为41 PASS/0 SKIP，组合发布已通过。Android/双端确定性/完整玩法/持续负载尚未验收；当时C3映射223条及31条未接，本轮31条已补真实业务链（见17.1）。统一证据out/continuation-final-verification.json，不能写全部完成。 C盘仅清理Go编译缓存，99.1 MiB可用→9.10 GiB，实际报告out/continuation-cache-clean.json；失败原始日志/字节与备份未删除。两个旧一次性MD重写器已归档out/backups/obsolete-doc-generators-20261008，避免覆盖当前接口和边界。邮件新发Grade0、旧已持久Grade1领取保原品阶，回归见out/grade-mail-local-20261008-015049.log。

## 14. 社交、助战、同步与异步竞技

本节社交、候选/票据、积分和周期定义仍适用。涉及客户端shared_control及双方结果一致的描述属于发布前旧会话契约；新建同步真人/机器人及异步PVP的调度、命令日志、恢复和唯一胜方以16.5/16.6的服务端单原生权威为准，客户端仅确认规范播放。

以下为现行已发布源码契约与原生取证；真实PG/Android与负载门见PROGRESS和REPAIR。

### 14.1 取证入口
原生Android1.0.128模块：CB7E0654好友管理、136799A0好友自定义类型、861CE174图鉴评论、89B6A072评论和评价类型、D7C70F60同步竞技、F8D135CF异步竞技、A66FD60A战斗与记录、3BC9394A同步记录界面、D0F7BB01异步记录类型、362EE013异步记录界面、2948C748幻书类型、875C5BCF错误码。
out/tools/export_social_evidence.py静态解析marshal，不执行原生代码。out/dis/social-*-native.asm保存31个完整模块及源marshal SHA256，out/social-native-export.log记录导出。原18个社交模块以外新增：927728D8客户端战斗、2CC3F05D基础、BAC02512演出、8364763B行动逻辑、E3877551驱动、63350DFB实体、08C6EB3D输入演出、45F2CECD玩家输入、42B11C32客户端控制器/录像、CF8BE798原生命令验证、C01C3E87阵营排序、8AFCE3B9助战界面、0179BEAC异步奖励界面。脚本支持模块别名+函数正则、find-method、find-ref、table 表名 [ID]。
out/tools/generate_social_catalog.py生成internal/game/social_catalog.json，记录common_const/error_code marshal及cards/bonus/mail_template/竞技/助战表源SHA256；好友100、黑名单10、每日每种幻书评论3条/100个Unicode字符，结果类型1同步/2异步防守/8异步攻击为原生常量。
generate_sync_pvp_ai_catalog.py补入Android role_info技能清单、默认外观、稀有度；仍保存全部源表SHA256。

### 14.2 好友接口与跨角色事务
apply_friend(callback,eid,msg,hostnum,apply_src)；agree_apply_friend/refuse_apply_friend/delete_friend/delete_black_list(callback,eid)；agree_all_apply_friend/refuse_all_apply_friend(callback)；add_black_list(callback,avatar_info)；clear_friend_request_flag()；search_friend(callback,hostnum,uid,nickname,valid_flag)；get_recommend_friends(callback,force)；get_player_details(callback,eid,hostnum)。
操作者只能是当前鉴权角色；上传昵称/头像不用于构造对方资料。申请目标服务器必须与真实角色一致。重复申请复用接收方同一申请；同意、删除与加入黑名单双向关系变化在有序双角色行锁事务内完成。任一角色容量不足、黑名单或业务拒绝全部回滚。批量同意按本次读到的申请窗口整批原子处理；期间新申请保留后续处理。好友数量/申请队列上限100，黑名单10。删除和拒绝重复操作不新增资产。
SocialState保存在avatar_progress.state.server_social；原生投影friend_dict、friend_request_dict使用ObjectID键，black_list是avatar_info_dict，friend_request_flag为Int。在线变化通过pendingSocialOIDs于Handle解锁后广播通用玩家刷新，避免连接锁相互等待。
social_presence.go从Service真实在线注册表投影online，不持久写假在线字段；同角色仍有任一连接时保持在线。attach只排队通知好友，Detach先释放注册表和自身连接锁后通知好友，避免双方同时进退服互锁。好友资料推送先friend_dict再all_friend_assist_cards，满足原生助战setter立即查询好友资料的顺序。
好友成就base_target401001，type11/params[1]：按当前真实好友数量投影，完成历史不倒扣；删除后重加不会重复累加。
AuthorizedFriendAssist(requester,provider,uuid)提供双向好友/双向黑名单/指定助战卡/卡归属检查；social_assist.go现已将选择、布阵、普通预设保存和实际开战接入实时双角色授权。set_assist_card/del_assist_card(callback,card_uuid)、select_assist_card(callback,card_uuid,assist_avatar)、refresh_assist_use_times()按原生签名，修复旧单参set_assist_card固定callback0。外部卡和契印只保存助战快照，绝不增加借用方拥有资产。
SocialState.friend_assist保存名册、当前选择、战斗冻结快照、主动每卡使用次数、提供者被动次数、已发主动奖次数和最近消费UUID。all_friend_assist_cards使用提供者EID键/card.card值，current_assist是assist_avatar+assist_card结构；assist_active_use_times使用卡UUID/ObjectID键，assist_passive_use_times和assist_reward_times为Int。原表RELATION_TYPE_FRIEND=3，assist_rule3名册100/每卡10次。副本assist_limit_type0禁用/1任意/2已通关后启用，且assist系统必须解锁。同步/异步竞技禁借好友卡，普通预设只允许已选助战引用。
实际开战updateBattleFormation consume=true将实时授权、卡/契印冻结、双方次数及主动bonus10（material17×1）同事务保存，主动奖上限50；consume=false布阵/普通预设不计次。相同UUID重试不计次/发奖；普通恢复迁移Frozen.BattleUUID与LastBattle到新UUID，Started=false也继承旧消费冻结，提供者升级/改卡/拉黑不影响原场，新局重新授权。日界采用本服UTC+8零点政策。原生mail_template5已证“昨天您的助战被使用了%s次”，全量周期事务先按旧Assist.Day/Passive发邮件再清零；assist_passive_bonus按5–50次数映射bonus11–20，5–9次material17×25，超50按50档，低于5不发。邮箱满或金额失败全部回滚可重试；原厂日界时区未在客户端包含。
真人好友挑战与排位已实现共同房间/共享输入/两结果一致事务链，详见14.9节；实际双Android原生演算是否一致仍需两设备MuMu验证，不能把服务端和桥控制测试写成同帧客户端验收。

### 14.3 评论、点赞、评价与标签
send_card_comment(callback,card_id,content)回调(ret,comment_id)；update_card_comment(callback,card_id,comment_id,content)，remove_card_comment/like_card_comment/unlike_card_comment(callback,card_id,comment_id)回调(ret)；query_card_comments(card_id,start_index,sort_type)无callback，下行on_query_card_comments(card_id,start_index,sort_type,list)。sort_type原生1=时间，2=赞数，同分按时间与UUID稳定排序；单页20，最大查询偏移10000是本服务查询保护上限，不冒充原厂常量。
评论写入新增card_comments表，原文和作者、赞用户集合、删除状态同时持久；点赞/取消/编辑/删除按评论锁→玩家行锁顺序，评论与操作者状态同事务提交。只能修改和删除自己的评论；评论与幻书必须匹配。重复赞14001、无赞取消14002、评论不存在14005、日次数超限14008来自原生错误码。时间日界为北京时间零点，回拨拒绝。评论失败不扣次数、不改赞数；删除保留审计状态，查询过滤删除记录。
like_card_remark/unlike_card_remark(callback,card_id)、select_card_tags(callback,card_id,tags)、query_card_remark(card_id)已实现。标签仅允许该幻书cards表fight_tags/character_tags，不接受重复或其他幻书标签。原生archive_card_2_remark为avatar_remark_dict，仅投影like_flag/tags；评论ID列表为服务端持久字段。查询按真实玩家评价统计tag_2_count/like_count和真实非删除评论数，下行on_query_card_remark(card_id,stats,top_comments)。原生unlike是取消赞，不把它描述为独立踩票。
refreshSocialDaily已经由daily_business.go接入在线Tick跨零点刷新评论次数。用户主动重新登录/发评论时仍有事务日界保护。
新增card_comments表和索引已经合入internal/game/db/schema.sql唯一嵌入式定义，原临时SQL已核对一致并归档。只增表，不删除旧玩家数据；回滚旧二进制可保留该表。

### 14.4 同步竞技与机器人补完
同步start_sync_pvp_match先持久waiting，Tick等待原表min_match_time，优先实际在线排队角色，无人到分段lose_match_time上界才落稳定Android机器人。匹配组合距离公式明示为本服政策，参数来自max_match_score/level_rate。机器人card_runes完整展开、套装候选按稳定模板种子选择，star/position/level/base/extra/词条库保留，UUID不写真实资产；role_info.skill_list完整下发skill_mgr，默认外观取cards.default_dress_id，不把card_skill_level当强化次数。
prepare改用当前sync_pvp_score_rule.dungeon_battle_id（原表21/22/23分段），不对所有分段固定使用副本21的battle_id。己方真实带契印幻书快照在锁定阵容时冻结，匹配保存自己的资料，结果记录保存双方资料、卡、阵容与旧积分。名次仍走真实跨角色存储，不用机器人填榜。
receive_sync_pvp_weekly_win_bonus(callback,count)按原生周胜表bonus_id领取，回调(box,msg)，材料与领取标记同事务，重复领取或溢出不发奖。原生sync_weekly_win_bonus是Int2IntDict，内部布尔收据投影为0/1。
SyncPvpMeta持久当前周期、首次参赛周期、周胜领取及结期分段快照。query_sync_pvp_season_id第二参数修为首次参赛周期，不再把activity_id当begin_season_id。赛季按Android原始日期和持续604800秒计算。跨期正在进行的会话延后切换，避免将未结束胜次误归新周期。
receive_sync_pvp_season_bonus()无callback，已结束真实参赛期的division_bonus_id与冻结RankBonus同事务领取一次；下行on_receive_sync_pvp_season_bonus(box)，推material_mgr/card_common_mgr/profileCosmeticProperties让头像框/外观拥有态可见。全量结期保存Rank/RankBonus，角色本地切期不得覆盖History；旧期缺完整全服历史快照的Rank保持0，不用今天排行造旧名次。不补造本轮之前不存在的历史参赛，积分沿项目明示本地ELO，不能宣称原厂公式。

### 14.5 异步竞技业务链
enter_asyn_pvp()/refresh_asyn_pvp()；set_defence_cards(callback,card_uuids)；get_candidate_cards(callback,eid)回(success,cards)；set_asyn_pvp_auto(callback,bool)。系统门槛panel_async_pvp、合法已持有防守阵容，候选优先真实玩家再补稳定Android机器人。候选服务器保存完整敌方卡和契印快照；界面不能覆盖对手。刷新间隔300秒、候选3、挑战副本1和票据26来源Android。防守阵容及候选持久复登可查。
攻击仍使用原生enter_dungeon(callback,1,{asyn_pvp_eid:eid})，只放行已授权候选；票据一次与战斗UUID同事务扣除，同一未结束会话重入不再扣票。加载走桥安装→type3 start_server_battle_ok→原生prepare，敌方add_fighting_cards带card.card_list标记，己方阵容在battle_fighting后冻结。
新会话的result必须经服务端原生唯一胜方、rev31播放末态和世代验证；攻击者与真实防守者按有序双行锁同时更新积分和双方最近10条记录。机器人不建立假账号。对局双方同时胜利、其他角色胜方、未冻结己方卡均拒绝。
积分算法为本项目明示ELO，Android K/fail_factor/defence_win_factor仅参数来源。攻击期间另有防守变化仍可结算，按冻结匹配积分算变化再加最新余额。已纠正finish_bonus：0179BEAC奖励界面将它作为每日分段奖，asyn_pvp_base_rule与邮件模板3明确每天21点，因此战斗胜利只结积分/记录，不能每次获胜提前领取同一个每日奖。
收据保存result序号/规范JSON摘要/结果/实际奖励盒，重复只接受相同序号及摘要，篡改拒绝；复连补原盒，失败也标RewardGranted防其他路径发奖。异步真实名次用PG全量COUNT，同分OID排序，不受候选1000窗口影响。每日21段位、周日21排名邮件已完成，见第十节。

### 14.6 战斗记录
get_record_result(record_type)严格单参无callback；下行update_record_result(type,list)。同步记录具有原生record_time/pvp_info/winner_eid/score及双方卡快照。异步记录具有defence_record的card_uuids/cards/old_score/new_score/enemy_info/enemy_card_uuids/enemy_cards/enemy_old_score/enemy_new_score/time/version/result。查询只取当前鉴权角色。旧同步记录没有双方快照时不伪造展示内容。
RECORD_TYPE4/16分别为原生异步防守/攻击录像。接续新增独立采集后置扩展、upload_native_battle_record分块事务、start_record_battle(index,version,type)→real_start_record_battle完整二进制文件。文件为zlib+cPickle protocol0，条目(action_time,method,args,kwds)，Go只静态白名单解析，不执行pickle；完整文件归属、SHA、版本和已完成状态校验，结果列表冻结索引UUID防新战报漂移。128观察事件与2048真人命令不充当原生录像，A7仍可选。样本测试与源码不等于Android原场景播放已验。

### 14.7 验证与待交付边界
定向命令分别执行：go test ./internal/game -run TestSocial、TestHumanPvp、TestAsyncPvp、TestSyncPvp、TestSyncPVP -count=1 -v（各前缀分别执行）。本次收拢只再做必要定向编译，不重复全仓运行。
out/social-local-verification.log最新21项PASS：社交10、Human3、Async1、SyncPvp4、SyncPVP3，覆盖上述事务/授权/幂等/日界、1101角色全量结期名次、共享屏障/双result/分歧拒奖与原seed/log恢复。out/tools/verify_human_bridge.py控制层PASS：原生调用、序号/去重、人工输入、日志重放、统一倍速/自动UI、中止、普通场景保留；不模拟Android战斗数值。
真实PG专项5项已编译：TestPostgresSocialPairsLockRollbackAndReload（双角色反向锁/普通与Social等级混合并发）、TestPostgresSocialCommentVotesRollbackAndRank、TestPostgresSocialAssistBattleAtomicReconnectAndPresence、TestPostgresSocialHumanSharedMatchSettlementDivergenceAndReconnect、TestPostgresSocialCalendarGlobalAtomicMailOnceAndSnapshot。真实Handle覆盖助战全链、共同匹配/冻结/开战/EID屏障/旧日志恢复/一致双结果/分歧回滚，以及日21/被动/全量故障/周日21头框邮件。out/social-postgres-local.log本机五项因无HS_TEST_DATABASE_URL明确SKIP，不能写为PG通过；最新远端实际日志才是验收证据。
原生录像与跨进程真人投递已有源码及历史实库发布；本轮十五组服务器实现及统一发布已完成。非好友助战已接真实名册与计次（17.3），契印池已批准并绑定正式配置。Android双端/录像、完整奖励和持续负载仍待实机，不把历史PG或桥mock当本轮通过。

### 14.8 数据库等级与锁顺序补正
UpdateSocial将参与者OID排序，先锁全部avatars，再按同序锁avatar_progress；评论先锁评论再avatars再progress。与UpdateProgress及AdminPlayer的avatars→progress一致。事务内以avatars.level播种Progress.AvatarLevel，保存progress同时更新avatars.level；Fixture同样更新Info.Level，材料2馆主经验不能只更新JSON而留下角色等级旧值。好友/竞技公开资料也投影SelectedHeadID、SelectedHeadBoxID和CustomHeadCleared，显示真实已选头像/框和当前等级。

#### 14.8.1 社交回载与评论写入边界

真实PG发现SocialAvatars/loadSocialAvatar及UpdateAllSocial三处SELECT/Scan遗漏账号、性别、nickname_set与created_at，活动acceptSocialRows会把连接资料覆盖为默认值，造成已取名角色副本门槛错误。现与accounts.loadIdentity完整对齐；不改任何旧资料或绕过创建校验。PG回归走真实新号取名→双人事务→全量日历→教学入场/重登，核对原账号/性别/取名标记/创建时间保留；证据out/pg-social-comment-metadata-report.json，结果以最新真实PG为准。

CardComment.Likes使用omitempty，零票JSON读回可能为nil。查询及UpdateComment写回调统一补可写空表，已有票不重建、不丢失。既有评论的OID/作者/CardID不可被回调改写；新评论保留实际创建所需CardID设置；INSERT/UPDATE影响行数必须为1，否则评论与玩家Progress整事务回滚。真实PG覆盖两独立Store追加到3票、SQL like_count/实际玩家快照、身份篡改与触发器0行拒绝。当前RPC原有nil guard保留，不把Store回调panic误称所有客户端必崩。

### 14.9 共享真人房间、输入与结果详细接线
新增sync_pvp_human.go的HumanPvpRoom完整镜像保存双方server_social.human_room，变更必须双方最新行锁且镜像一致。冻结共同UUID、crypto随机seed、排序的两OID、battle/dungeon、真实资料/积分、本人卡/契印/预设/布局、Loaded/Ready/Started、输入屏障、完整共享命令日志、结果和恢复位置，不让两人分别打一场。
好友RPC：challenge_friend(callback,eid)，agree_challenge(callback,eid)，cancel_challenge(callback,eid)，refuse_challenge(callback,eid,reason0/1)。真实双向好友/黑名单/在线/忙碌校验；7001非好友、7002邀请中、7004离线、7005忙碌。成功回调必须真实持久接收者challenge_dict，同意才建房；原生5秒/3条窗口。过期仅失败挑战分支已删除，真实invite专项替换旧测试。
send_pvp_cards支持原生预设数字索引及已验证容器，只能本人4出战/2援护，禁止外借卡。共同选卡后type2排位/type6挑战，副本21/3，挑战battle13，排位battle21/22/23按分段表。prepare两端相同排序OID/battleID/seed，并同时发送冻结卡/runes。camp排序、base递增EID、entity生成顺序和attrs.master/card_uuid均有原生证据。
pvp_load_complete/battle_fighting逐槽校验冻结布局和本人已加载，两端Ready才共同Started并同时battle_fighting/start/check_on_battle_start。ready允许加载时上报，started必须共同屏障已允许，不能单端伪开战。
human_battle_bridge_script.py为rev12后置扩展，原rev12保持。clientExtensionsScript/热更按rev12→活动日程→activity_metrics→human→native_record组合，外层固定标记由组合生成器包裹。等待rev12实际成功并延迟重试，日志HS_HUMAN_BRIDGE_READY 2；保存原函数/旧版本closure解包避免升级双重套装与重复事件。
桥调用原生server_control.controler.do_command验证move_to/use_skill，不猜技能数值。每端human_input(action,eid,master,units,command_index)校验唯一EID/坐标/数值/冻结归属/当前鉴权控制权；两端action/EID/master/排序快照SHA256相等才广播同一revival_do_shared_command。较快拥有者先提交则持久Pending，另一端确认才消费；非控制者、序号跳跃、已消费屏障不同命令拒绝，相同重试幂等。2048日志上限为服务端保护。
共享战斗清两端本地自动输入和master超时，调用原生on_auto_state_update更新按钮/单位标志，原生set_battle_speed统一1，不造UI按钮。普通战斗沿原逻辑。
human_result须唯一胜方属于两人、完整命令位置、连续桥序号/同UUID。第一份仅保存；第二份胜方/行动号/末态摘要一致才双角色同事务积分、10条记录和不可覆盖收据，沿本服明示ELO。好友挑战没有排位分/物品奖。分歧或原生错误aborted，先revival_abort_shared阻止报假结果，再原生exit_battle_ok退出，不发伪胜负奖/获胜记录。
humanResume保留原UUID/seed/冻结卡/log，只重置恢复端握手；revival_set_shared_replay按每个原生输入行动顺序重放旧命令，用info.attrs.master的原生ObjectID，到日志末尾后解除屏障，另一端不新增共同输入。已结算仅补收据，不重开另一独立场次。实际双Android确定一致仍需MuMu。
Service内部接口：humanChallengeRPC处理四邀请；humanCardsRPC/humanBattleFighting/humanDoCommand/absorbHumanBattleEvent(ctx,c,*battleEnvelope)/humanResume(ctx,c)返回(handled,[]Push,error)，分别先于旧选卡/开战/命令/观察/恢复链；Tick玩家reload后tickHumanPvp与flushHumanPvp。humanBus只投递，room/log/result权威在PG。队列/在线投影不拿其他连接锁；publishPlayerRefresh在Handle/Tick释放c.mu后逐项pendingSocialOIDs通知。

### 14.10 全量周期事务、排名与历史边界
新增SocialAccounts.UpdateAllSocial(ctx,fn)，Fixture与PG全量无1000窗口。PG按OID批量锁全部avatars再同序progress，仅变动JSON使用pgx.Batch，fn失败全量回滚，馆主等级同时保存。无周期schema新增，收据存Progress；1101角色专项证明名次1/1101而非截断窗口。
refreshPvpAwards(ctx,c)在日界/每日21/周日21/原生赛季边界串行一次全量事务，离线也处理。已接BecomePlayer/在线Tick日界前、机器人result前；本模块在助战/领奖/真人匹配与result/异步接口与result前调用，先封存截止奖再改积分/次数，失败不写完成key可重试。
asyn_pvp_base_rule、0179BEAC.init_show及mail_template3/4证明日21分段/周日21排名，finish_bonus是每日档位。AsyncPvpState.RewardDay/RewardWeek持久防重，每日按截止积分分段发模板3，周日按全量真实排名发模板4。首次遇到周期只建基线，缺历史21点快照不能用今天积分造旧奖；长停机不捏造多日补奖，保存最新周期发件与收据。
附件只取bonus.fixed_items，邮箱满/金额无效/未支持随机配置全部参与者回滚。排名1191附件80008为原生type9头像框，邮件领取已用grantNativeItem在clone内整事务领取/刷新材料与外观，框/装束不存材料余额。被动昨日邮件同事务先发再清，模板5与assist_passive_bonus可核对。
SyncPvpMeta.History新增Rank/RankBonus。全量边界先冻结尚有完整数据的结束期排名与原生排名奖，再本地切期；已冻结History不覆盖。旧期存在缺历史数据者则Rank保持0，不拿当前排行榜造旧期。分段+排名奖与Claimed同事务，只有真实参赛期发；跨期在战斗角色切期延后，截止奖快照保持。原厂积分/匹配公式不在Android，政策标记不能删。


### 14.11 接续新增契约与证据（2026-10-08）

- 结算恢复：battle_settlement_wire.go按盒字段白名单恢复typed容器；BattleSession.UnmarshalJSON只对SettlementBox使用UseNumber保住大整数，create_time仍为Float。cmd/gameserver/gate.go直接遍历struct/map/slice，保留整数键、ObjectID键和Ext42，不靠JSON或fmt.Sprint编码。实际msgpack回读测试通过。
- 现有card/rune manager的hex键可由原生CustomDict.load的_key_type.convert及ObjId.load的bytes2id/str2id转换；不把“字符串存在”直接当原生错误。来源out/dis/activity-36B65187.asm及custom-basetypes.asm。
- human_delivery.go将投递描述符与两份共同房间镜像同PG事务保存；Tick读最新整档后按当前连接UUID/序号游标发，事务回滚没有幽灵命令。共同UUID/seed/log、双方输入屏障与双一致结果保持；不让两端各打一次。hs_game_presence是项目PG租约表，20秒有效/5秒刷新是本服在线政策，断开删自己的租约，其他同角色连接保留。
- 原生录像native_battle_records表按真实已结算异步攻击UUID持久，防守方仅凭自己的战报列表查看；只新增表，不删旧进度。49,152字节分块、768KiB压缩/64MiB展开保护上限，超限拒绝，不截断拼造。GLOBAL仅允许ObjectID重建相关三项，方法与__custom_type来自本版白名单。两份真实battle.txt静态样本通过，Android播放仍未验证。
- 本轮31条已接真实业务与结算；原223映射基线和当前范围、逐目标边界见17.1，不代表Android全目标验收。
- 旧权威研究测试原未固定随机种子，100次观察13次主人公死亡；固定夹具123456后100次通过。只修夹具可重现性，未修改生产随机种子或恢复旧权威战斗路线，不能据此声称所有随机教学必胜。

## 15. 常驻活动状态机、奖励与原生统计

用户已批准常驻时间政策，原奖励/解锁保留；以下未知机制仍拒绝或待补，不能扩大为批准未知奖励池。

### 15.1 用户决定、证据边界与文件归属
1. 用户明确所有活动常驻开放，包括原 activity_type 中 disable_activity 的历史活动；这是本复刻服运营策略。底层地图、节点、副本的 _disable/_dungeon_disable、原生解锁条件、材料成本、到达条件不绕过。
2. 数值与协议以本项目 Android 1.0.128 marshal/703表导出为主证据，_ref/GMA-Revive-Server-main 仅用于理解玩法语义。本项目根不是Git仓库。全部命令显式系统CMD、login:false，中文文件UTF-8。
3. 本节说明当前源码；线上版本、真实数据库与实机验收以HANDOFF/PROGRESS证据为准，不能根据接口存在推定全流程完成。
4. 代码入口：internal/game/activity_business.go、activity_metrics.go、activity_schedule_script.py、activity_metrics_script.py、exam_business.go、summer_business.go、cthulhu_business.go、cthulhu_rewards.go、miku_business.go、miku_exploration.go、miku_achievement.go、miku_power.go、mountain_nian_business.go、nian_footprint.go、wangyan_business.go、wangyan_rewards.go、house_frage_business.go。
5. 原生目录 internal/game/activity_catalog.json 由 out/tools/export_activity_evidence.py 生成，当前101表/23组件来源；每表/模块保存原SHA，来源为 out/client_catalogs 与 out/npk_scripts/android_inventory.json。不可用导出表数量替代玩法覆盖率。
6. 原始查证工具 out/tools/read_activity_native.py 支持 table、fn、nativefind、search、schema；反汇编 out/dis/activity-*.asm 为阅读证据，不执行客户端脚本。可重跑生成器复核来源，生成前注意其他代理的目录更新。

### 15.2 常驻运营配置与客户端时间
1. ActivitySchedule 字段 ActivityID、Begin、End、Enabled、Permanent；永久活动 End=0 且 Permanent=true。SetActivitySchedules([]ActivitySchedule) 严格拒绝重复ID/未知ID/非法时间，上限按原生145个 activity_type，而非旧128。DefaultPermanentActivitySchedules(begin) 按ID排序生成完整145项。
2. deploy/data/activity-schedules.json统一 begin=2026-10-07上海00:00、End0、Enabled/Permanenttrue。main启动严格加载 DataDir/activity-schedules.json 或 HS_ACTIVITY_SCHEDULE_FILE；不把未加载的默认函数当部署配置。
3. activityOpen 根据原生 begin/end 原时长计算最后活动日，永久活动日数封顶；已验证山海203封顶21、克苏鲁206封顶21、年兽挑战328封顶14。不会几百天后 day 超表关闭。永久运行不会自动重开新赛季；年兽末日14配置会同时开放三个挑战，这是当前“原生日阶段封顶”政策的结果。
4. activity_schedule_script.py 覆盖全部145项，修 activity_utils 时间/开放/活动日函数与 dungeon_check 开放判断；运营 begin/end 相对迁移活动关联banner、board、shop、commodity历史窗口。所有底层disabled/unlock/weekly门槛仍保留。针对shop end=None补 gui_utils 原生时间判断，避免Python时间比较崩溃。
5. 脚本通过 activity_utils._hs_original_windows 保存原日期，只迁移一次；重复热更不会再次移动窗口。等待模块可用时 game3d.delay_exec(1500,attempt) 重试；成功日志 HS_ACTIVITY_SCHEDULES_READY 145。
6. 原生主证据 activity_utils 2A089B57；schedule测试 out/tools/test_activity_schedule_script.py 覆盖全部145、首日/长时间封顶、周期门槛、未知活动、未开始与重复安装，已PASS。没有添加全局 IGNORE 解锁开关。
7. activityShopAvailable 供商品/所属shop历史日期校验委托；商品动态 InvalidHours 仍有效。当前 activityForShop 遍历原生map，多个活动共用同shop时应进一步检查确定性的选择规则，不能误把无关联限时商品全放行。

### 15.3 存档、登录投影与统一战斗事务
1. Progress.Activities持久键是server_activities；包含summer、wangyan、mountain、nian、north_wind_footprint、exam、miku、cthulhu、house以及last_time，定义见progress.go/activity_business.go。
2. ensureActivityLogin(p *Progress, level int, now time.Time) error：检查真实解锁后初始化各活动容器、妄言自然恢复、夏日每日补给、宿舍每日领取资格。地图随机生成/奖励不是登录附赠。已接service登录与daily_business在线日界；年兽每日挑战进度重置仍缺，跨日UI和失败重试须验收。
3. activityProperties输出原生summer_game、wangyan_game、mountain_game、monster_nian_dungeon、north_wind_footprint_game、exam_game_info、prev_exam_correct_info、storyline_bonus_dict、miku_game、cthulhu_infos、cthulhu_title与houseFrageProperties宿舍字段。Miku奖励盒在JSON重载后由保存的材料、卡UUID、契印UUID收据重建，恢复整数键和真实ObjectID。
4. activityWire 用反射转原生dict，整数键转 mobileproto.Map；二元tuple任务键保留tuple，game.ObjectID 与 mobileproto.ObjectID 必须在string分支前保留Ext42。wire:"-"内部账本不泄漏。不能 fmt.Sprint tuple/intmap 后变string。gate也已改递归typed wire，避免 JSON marshal 丢整数键。
5. prepareActivityDungeon(p,dungeonID,level,now) 仅新建会话调用：核对活动/副本disabled、原生解锁、活动开放日、玩法专属状态，再扣 need_materials，并冻结 ActivityBattleContext。旧会话重入不能重复扣。
6. validateActivityDungeonExtra 验证原生地图/节点/选择物/任务/诅咒上下文，禁止客户端随意上传任务清单、材料奖励或跳过活动状态。通用活动ID203/206/207/208/209/211/328覆盖。
7. context保存 ActivityID/DungeonID/MapID/NodeID/Tasks/PreparedAt、Resource、Cthulhu层/格/周期、Spent实际材料、Angry、NativeSkills、PaidPower真实支付及Statistics、RankCards。来源字段应只由服务端状态写入。
8. activityBattleExtra(ctx,extra) 复制extra后删客户端tower_task_list/add_battle_skills，再从冻结context注入。夏日道具技能原生6tuple(skill_id,1,1,0,1,1)取21400154；初音愤怒技能按miku_event原生camp/default/show规则。源码已接start和prepare。
9. settleActivityDungeon(p,b,win,tasks,now) 同玩家行锁事务，必须DID与context一致，任务必须是冻结任务的无重复子集。普通奖励/首通奖励、专属节点与进度、资产、返回材料/体力、UUID奖励标记一起提交。未知配置/溢出失败整笔回滚。重复结果不再发奖，保存 SettlementBox 供重连回包。
10. grantActivityBonus(p,bonusIDs,now,optionalLevel)调用共享grantNativeBonus(p,bonusID,amount,level,now)。固定/随机材料、随机契印、item_libs原生条件与概率统一解释，实际幻书/契印UUID也合入盒。共用主证据D44DBBA7 bonus_utils.generate_bonus，统一处理随机概率、均匀数量、不放回库选择。
11. mergeActivityBox 已修真正card_list多行及尾标记兼容、materials整数映射、runes UUID字典；丢失首通契印盒的问题已修。家具/外观/头像等由统一奖励的材料结果和真实资产处理，不凭空增加未取证盒字段。
12. 馆主经验material2调用grantAvatarExp，实际经验/等级，不仅普通库存材料2。知识储备material4 type2/sub3维护真实余额。卡经验material3只是奖励盒计量，applyBattleCardExp 对实际拥有/冻结出战卡实例同事务升级；非战斗领奖若出现material3没有接收对象，仍需逐奖励检查并明确拒绝，不能悄悄记入普通材料。
13. 失败返还 return_materials <=ctx.Spent 每项实际付费。return_power 原生 D81B4FE0.get_fail_return_power 仅double乘2，没有初音减免比例；当前返还 min原表/ctx.PaidPower，防减免后刷体力。旧会话没有付费收据不返。普通副本也新增PaidPower/PaidMaterials，已发现并修None need_power→activity_type.need_power fallback导出缺陷，相关主证据 out/dis/dungeon-cost-refund-native.asm；请新会话复核普通/活动一致。

### 15.4 D1 同桌考试
1. ExamState 保存学习历史Study、任务计数/领取阈值Tasks、复习成绩Dungeons、摸底科目组Exams、问题答案Answers、完成问题组Groups、预习选择Prev、剧情收据Storyline。
2. 原生92EDA415 special日程按完成课程次数插入强制剧情/答题组；prepareExamDungeon检查当前剧情、课程学习总数、先完成特殊日程再上课；不把预选课程列表当已完成历史。题目必须属于当前组，选项范围和unlock_condition均检查。
3. 活动任务来自fix_tasks/context；结算实际完成任务计算成绩和学习次数。学习分阶段奖励按exam_task.exam_task_bonus，复习/摸底按challenge_bonus阈值与领取账本；all领取只取实际可领，空可领奖错误。
4. RPC：prev_exam_choose_opt(problemID,opt)直接；receive_exam_study_bonus_all(cb)；receive_exam_study_bonus(cb,taskID,times)；receive_exam_review_bonus(cb,dungeonID,threshold)；receive_exam_test_bonus(cb,testID,threshold)；exam_answer_problem(cb,answersIntDict)；exam_answer_bonus(cb,groupID)；receive_storyline_bonus(cb,storylineID,nodeID,bonusID)。实际原生参数与结果以examRPC及主证据为准，回调不得用任务ID替代cb。
5. activityConditionCount(type,subtype)目前对209按实际Study历史和副本subtype计数，商店kind12严格三段#条件调用。其他不认识的活动条件不伪造次数。
6. 尚需考试全日程、学习上限、阶段补发、题组重登和真实数据库/AndroidUI系统验收；本轮没有新增全流程Exam测试，不称D1全量通过。

### 15.5 D2 夏日地图、道具与钓鱼
1. 原生summer_map/node/item/event/fish/site/handbook目录，四邻接划船BFS、真实可达frontier、当前地图状态、解锁/前置节点/道具门槛；_disable节点仍拒绝。开地图只初始化原生locked=0钓鱼点。
2. SP初始80，雾格消耗4；supply_day保存UTC+8日期，跨日只在不足80时补满80，额外>80保留，不累计离线多天补给；同日不重复，时间回拨拒绝。PRIMARY new_system_tips.activity_summer_map 与 error_data72023 明确“每日不足上限补满，额外可以超过上限”，REFRESH_HMS=(0,0,0)。
3. 事件选项0基，节点奖一次；道具选择从真实未拥有物品中生成选择池，消耗选择次数；进入战斗冻结Map/Node/已拥有选择物及原生技能。战胜后bonus_sp、节点完成与后续解锁同事务。道具material_add_rate对211002奖励叠加，按原生21400154 get_material_addition计算实际额外材料并入box。
4. start_fishing(cb,siteID)必须拥有解锁钓鱼点与鱼饵211001，扣1次，原生权重选鱼，保存待钓鱼site/鱼ID/体长/等待时间；check_fishing_result(cb,successBool)对同一pending结算，成功更新不同鱼/数量/最大体长，失败原生奖励，收据只结一次。fish_handbook奖励按实际阈值与领取列表，无重复领。
5. RPC11项：summer_game_enter_map(cb,mapID)、summer_game_move_to(cb,mapID,nodeID)、summer_game_move_boat(cb,mapID,nodeID)、summer_game_enter_node(cb,nodeID,opt)、summer_game_choose_item(cb,itemID)、summer_game_clear_sp(cb)、start_fishing(cb,siteID)、check_fishing_result(cb,Bool)、receive_fish_handbook_bonus(cb,tier)、receive_all_fish_handbook_bonus(cb)、receive_summer_banner_bonus(cb,bannerID)。参数以上以summerRPC为准。
6. 夏活原生封弊者1.6及冻结鱼长已实现，原生/本服规则区别与验收见17.6。

### 15.6 D3 残页梦境（克苏鲁）
1. CthulhuState保存中文title、map/layer/trunk/branch/circle、线索/物品library、事件状态、初始化账本、固定教学移动。重载后restore恢复同item指针别名，否则已领取物品会重新出现。地图采用真实优先级池、加权未用事件、分支与阻断，不是空地图放行。
2. 修open_layer由list误读为单整数；当前层/已完成层/其它地图层条件检查，初始53SAN100、54..57四项属性4、58点数0仅首次初始化，默认中文称号来自cthulhu206.default_sub_name。移步普通体力5走consumePower自然恢复，定点移动53消耗15，固定骰步1..6。
3. 访问物品状态、强制战斗格、challenge/giveup状态2、初始化/层首/层末物品、完整层要求线索与事件完成后进入后续层；只允许当前真实事件的DID/context，不能直接任意进206副本。
4. grantCthulhuBonus：只在206活动奖励路径把53..57按原生materials上限折实际入账，53上限100、54..57上限12。先统一奖励随机生成一次，再恢复原余额按容量实际增加，box与Total都仅actual delta；通用库存奖励仍保留capacity错误，不把活动属性策略套给所有系统。SAN90+原生circleBonus27实际+10；已满则+0且不增Total，已专项PASS。
5. 克苏鲁check及all-in已按原生骰/属性/SAN与批准极值、访问收据规则实现，详见17.6。
6. 原服完整critical优先级/重试生成未在APK提供；当前check/all-in已按原骰/属性/SAN直接证据与用户批准极值/收据政策实现（17.6）。不把本服补充规则称原服恢复。
7. 完整随机层、线索显示、失败放弃重返及AndroidUI仍须实机验收；真实线索资产入账与check/all-in状态收据现已接，不再以过去专用显示未验断言底层未实现。

### 15.7 D4 初音与C4手册
1. Native miku_map/type/node/event/handbook/story/dungeon/task/achv；四张地图day1/3/6/9，开图起点200/100/300/400，正常/困难探索互斥，frontier可达，事件按原生权重与不放回抽样；Angry/Bless3项原生795CA267。不允许客户端选择任意未生成事件。
2. activate_handbook_item(mapID,itemID)直接无callback，on_activate_handbook_item(Int)下行 PRIMARY B0D07550；只可激活已满足progress/parent/材料条件的条目，真实扣费一次，激活状态2与后续解锁持久。story gate修progress_condition使用其指定mapID的进度，而非当前图代替。
3. 成就实际target：2通关、50run次数、52终点、53访问事件、54进入地图、56地图score、69拥有模板、70真实送礼、71活动战斗用卡。组内求和/组间取最小值，普通与特殊成功送礼源码已接recordMikuGift；领奖标记与真实奖励同事务。
4. enter_miku_node验证Path/frontier/当前事件/normal-hard/angry资格，冻结节点、DID和任务后真实进入enterDungeon，结算推进score cap100/Passed/Path/Run/Visits/Treasure，材料/幻书/契印收据均持久；胜利后源码追加on_finish_miku_node，重复结果不再推进。
5. endMikuRun终点原生10%货币raise取floor，提前leave无raise，on_end_miku_map六参map/coins/raise/box/fromEnd/path；只重置当前探索run，地图总进度/Passed保留。JSON重载后receipt盒重建typed material/card/rune，不泄漏UUID字符串。
6. activityPowerCost(p Progress,dungeonID,base int)(int,error) PRIMARY B0D07550.get_miku_power_addition加总当前副本对应图的已激活battle_power_effect，factor=max(0,1-sum)；C13BDDDA fix_dungeon_need_power=int(ceil(max(0,need_power*factor)))。源码已在新会话/纯剧情consumePower前接；实际PaidPower防返还超付。
7. 初音RPC route已有enter_miku_map、activate_handbook_item、set_miku_auto_path、enter_story_dungeon、enter_curse_abyss_dungeon、enter_miku_node、leave_miku_map、receive_miku_like_song_bonus、receive_miku_task_bonus、receive_miku_surprise_bonus、receive_miku_achv_bonus；寻路只允许生成可达路径，任务领取需冻结DID任务完成收据，点赞一次nativeBonus20801002。
8. 初音惊喜及手册原生buff已实现；惊喜数值为批准本服政策，逐buff冻结与持久收据见17.6。

### 15.8 D5 山海、D7 年兽排行榜与原生统计
1. 山海4法阵充能与guard槽真实解锁、拥有/唯一/槽数量；原第一阶段+50及每日350/700/1000，第二阶段阈值收据保持。守护原生base+process×charge/1000冻结，材料同项合并floor与七日常驻赛季现已按政策实现（17.6）。
2. 新修山海mountain_dungeon[progressID].dungeon_id必须真实DID，旧误填progressID会导致hardlevel UI0。真实战报时记录最佳：hard_level高优先，同hard总行动AP低优先；保存total_action、native5tuple(cardID,level,grade,dress,isSupport)。隐藏server_ranked、server_rank_hard仅用于SQL真实成绩过滤；旧桥无统计保留关卡进度但不填排行成绩。
3. 年兽挑战DID20200001/2/3原表monster_nian_dungeon，day列表与unlock610检查；胜利推进current_progress，领奖threshold来自native reward_progress/monster_nian_bonus，领取位图不能重复。新增原生伤害统计记录单副本max_damage/total_damage、最优阵容，胜敗都可提交真实damage，只在该UUID一次结算。
4. 原生统计主证据 FBBE590A：get_battle_ap_statistics返回total_ap_statistics；get_total_extra_statistics按data.battle_info[bid].battle_extra_statistics中的(campID,roleID)过滤statistics，sum user_data.damaged。C9FDF16E山海UI明确int(get_battle_ap_statistics())，不能用command条数代替行动AP。
5. 独立activity_metrics_script.py保留rev12原文，从notify_finish的report闭包构造扩展，只给同一次result增加total_ap_statistics/total_damaged_statistics。真人包装已安装时从shadow._revival_human_originals取得原notify，并替换human wrapper.old_notify闭包；不另起sequence，不重复上报。等待桥12/模块，幂等，日志HS_ACTIVITY_METRICS_READY1。组合顺序rev12→schedule→metrics→human。
6. battle_observe.go已在settle前调用recordActivityBattleStatistics(b *BattleSession,data map[string]any)error；battle.go已在实际b.Team/Layout冻结后调用freezeActivityRankCards(p,b)error，最新全仓编译与测试通过。统计两个字段必须同传、非Bool、有限非负整数、<=2^53-1、AP<=Int32；缺两项的旧桥没有排行资格。
7. query_mountain_sea_own_rank(callback,progressID)回rank_index一参；query_rank_list(rankID,subID)回on_query_rank_list(finalRankIDStr,avatarInfoList)。山海score=[hard,-AP]与五元cards_info、年兽真实分榜及subID0总榜均已接；总榜政策/小时快照/周奖见17.7。
8. ActivityRankAccounts.ActivityRanking(ctx,kind,progressID,oid,hostnum,limit)已实现Fixture及PG；SQL在同一repeatable-read快照做全服COUNT与显示页，按同服/真实参赛资格过滤，难度高/AP低或伤害高优先、同分OID稳定。显示1000条不是名次候选上限；1105同服玩家本地证明尾部真实名次，真实PG新用例待当次报告。
9. 【纠正总榜假设】new_system_tips.monster_nian_dungeon明确总榜以三副本名次综合，不是最大伤害或总伤害；每小时更新，周三23:59:59周结算并清空，日榜每天23:59:59奖励。get_damage_record最大只属于个人UI记录，不是总榜算法。先前max总榜假设已撤销，不再作为施工政策。综合公式、名次为空的惩罚、同总分tie缺原服务端证据，必须保持待取证/用户明确政策，不能随便sum/min/max。
10. 同一本版new_system_tips证明跨UTC+8零点结束不计成绩；接续已实现独立有效RankDamage/RankAt，跨日仍保留个人max/total但不进榜。每日每DID前3次进度与领取位图独立刷新、封顶3次，旧缺日期只建基线不补造奖励。
11. 同服UpdateAllSocial原子冻结排名/原奖邮件/收据；山海按七日常驻赛季22点，年兽非周三23:59:59日奖、周三总榜原周奖及新周清榜。邮箱满整体回滚、缺历史不伪补，rank8/10及真实COUNT保持，详见17.6/17.7。

### 15.9 D7 北风足迹（与年兽挑战是不同活动）
1. ActivityID327是真正北风足迹，328是年兽排行榜挑战；common_const与27A317D7主证据区分，不能混为同一入口。
2. nian_footprint.go保存current_site/real_site/open_grid_bonuses，以及wire隐藏activity_times/剩余奖励索引池。open_grid(gridID)直接无callback，on_open_grid(ret,box)（385E6D63）；native9格0..8，票5221扣1，已开格不能再扣。
3. 从实际未用池抽索引，不放回；索引0通关并立即下一site，奖励native25行north_wind_footprint_bonus。本表已加入活动生成器。
4. site21之后5轮循环；site25下站26的real_site21。native modifier=float? 表达式是site-1-times/5，NeoX Python2环境整数正数除法；落后<-3池保留0并按差距减少其它样本，领先>3池不含0且最多8项，正常池按原表。相关正整数语义由项目opcode-recovery.md的Python2.7.3以及native表达式说明，不声称本轮重做ARM opcode44 handler验证。
5. 源码已接open_grid route。强制池测试覆盖唯一格、不重复付费、pass下一site、末站循环、落后包含pass、领先无pass；只有夹具测试，未做实际Android九宫格验收。

### 15.10 D6 妄言（汪言）
1. Native wangyan_game/map/node/dungeons/general_args：初始妄言力5，补给1000，上限10000，自然每秒1；真实地图路由/占领/Area数、enemyLevel<=实际Power、nodePower一次及Boss上限，专属context保存已付supply。
2. 纠正胜利奖励错误：win使用supply_addition，lose使用return_supply且<=ctx.Resource实际扣费（材料206001描述、newSystemtips及原字段）；原先win误用return_supply已删除。
3. 【显式补给允许超限 PRIMARY】0FA7FEEC custom_types/time_auto_attr.add(add_value,auto=False)，类体LOAD_NAME False→MAKE_FUNCTION1证默认False；auto分支min(max_limit,value+add)，非auto分支value+=add完全不clamp。达到上限更新LastTime，防止额外积累自然回复。不能从自然恢复封顶推导奖励硬截断。
4. grantWangyanSupply(p *Progress,amount int64,now time.Time)(actual int64,error)先结自然恢复，再显式加，不写普通Materials206001，Int32溢出拒绝。win/lose补给统一调用；grantNativeItem type4/sub20已委托helper，box记录actual与真实wangyan_game推送。已追加超额/恢复/重载/整奖励回滚用例TestShop...，TestShop专项已通过。
5. area_bonus仍按原10/30/50阈值材料加成；战斗相关影响节点/修正/buff在prep冻结，当前战斗与恢复不因占领后重算，成功占领影响后续新战斗，详见17.6。
6. 普通task_type1委托严格扣配对原材料+奖励+Mini[id]完成，旧Tasks迁移到Mini。特殊task25先要求全部first task1完成（2007183C/2BCB4D1C）；按玩家所有stype21物品数×bonus20703025固定2060021（E27E0BCF）并消耗全部对应物品，同事务，可积攒后再次提交，无伪造数量上行。
7. RPC4：enter_wangyan_game(cb)、wangyan_game_enter_map(cb,mapID)、wangyan_game_enter_dungeon(cb,mapID,nodeID,extra)、submit_wangyan_consign_task(taskID)直接。妄言等级修正表/affected_by_nodes战斗buff尚未完整注入；本轮未新增完整妄言真实服务battle测试，真实DB与UI待验收。

### 15.11 D8 宿舍骰子与碎石商店
1. house_frage_business.go依据native house_frage/params/card_house/facility_board_game/card_to_fragment/frage_stone，并委托collection设施6真实等级、已入住幻书。卡池按设施级别允许卡模板、SR/SSR指定标记与概率；63个真实board sites，登录不赠库存每日骰子。
2. enter unfinished board保留随机格/当前位置，结束后再roll；normal dice1/2都消耗骰子401一个、两骰为两个独立d6，ctrl1..6消耗402；403慢行step1持续3回合，404刷新碎石商店。17分支→101、118→24为原生GUI路线，45终点native241001。
3. 卡奖励实际nativeCardToFragment、N数量15、R/SR/SSR概率与双倍、+40%、设施effect15/16/17等原表效果，非空成功。碎石商店使用cloneProgress调用统一random bonus281101生成未发放候选，保存真正materialList/price/count；buy按原单价扣费/减stock，卖完拒绝，整笔回滚。
4. get_house_board_game_reward()直接无callback，每日UTC8/设施effect11发401数量2/4/6/8，隐藏日期ledger同日不能再领；BoardReward表示今日可领取，领取后false；collection不重复自动赠dice。
5. RPC8已接：enter_house_frage(cb)、set_mark_card(cb,rarity,cardID)、house_frage_move(cb,diceMode)、house_frage_ctrl_move(cb,step)、open_game_site(cb,index)、house_frage_use_prop(cb,materialID)、buy_frage_stone(cb,index)、get_house_board_game_reward()。移动原生回调5参moved/pos/finished/bonusList/currentProp。
6. 宿舍移动返回真实材料字典，客户端自建动画；per_count使用批准连续非SSR事件计数仅影响新棋盘，见17.8。

### 15.12 验证与现行增量

00:14七项活动测试、09时及单端PVP44项实库是各历史快照，不能代表十五组最终批次。原本未决的活动/住客/骰子规则已按用户批准政策及进一步原生证据实现；详细规则、协议、状态与尚须验收边界统一见第17节，原始报告/反汇编和失败备份保留。设施6真实入住SSR加成仍沿原式，不乘生产倍率；累计计数现用17.8规则。

## 16. 作者新版对齐与本服正式规则（2026-10-08接续）

更新包SHA256为1bdf673fdfdd188a07272a91e083d410e608b82ba0f7fe8aa1eebdc5af1f28fa。差异及迁入九个原生引擎文件来源保存在out/author-update-alignment.json；仅迁入代码，没有读取或复制作者账号、RSA私钥和会话数据。新版主要变化是PVP单原生权威、双方播放FIFO、坐标/实体映射、主动弃权、重连和管理转移；契印权重、家具融合及若干活动机制没有因此补齐。作者自己记录的测试/实机结果不是本服验收证据。

### 16.1 明确批准的运营规则与正式文件

deploy/data/gameplay-rules.env与deploy/hs-game-gameplay.conf为正式发布输入。out/gameplay-policy-approval.json保存用户原话与正式文件SHA256；发布器验证其与构建绑定一致。家具八组内权重1，保留原生98/2或92/6/2、成本/舒适度和3:2心愿60%；契印17全零池每组18个实际有效套装权重1，保留原部位/星级/数量，原非零套装池仍使用原表。本服运营分布不冒充原厂概率。两项proposal仅作候选来源历史，生产不按文件名自动读取。

商品1071011/1071012/1071013/1071014原15天、2010010原7天均改为永久开放。shop_permanent_policy.go仅放行指定ID且核对原天数，未来未知商品继续拒绝缺开服时间；activity_schedule_script.py同步取消这五项open_server_days/count_down_flag。原价格/折扣/每日及总限购/系统解锁均保留，不设置虚假的HS_SERVER_OPEN_TIME。实际购买RPC限购失败保留整Progress及持久层不变；新增五商品购买/冷重载/零写入拒绝用例已纳入09:25正式41项真实PG。批准家具正式文件已逐个grantNativeItem验证候选可发放，包含6999999真实奖励库，不仅检查JSON格式。

### 16.2 主动退出与断线分界

本版原生out/dis/client-control.asm:1316证明exit_battle不带参数。已接exit_battle、leave_battle、quit_battle真人路由，先刷新数据库房间，再在UpdateSocial双方行锁事务核验两方Battle UUID；真正共同开战后主动退出判本人负/对方胜，积分、战绩、唯一UUID收据和持久投递同事务提交。开战屏障未完成则中断，不给积分。已提交终态不能被晚退出翻转；Android旧路线有单端末态待确认时拒绝用退出覆盖它。TCP Detach仅关闭连接与租约，不判负。

弃权先发送原生set_winner_eid_list(ObjectID列表)与无参real_battle_end，再发送已有积分属性/结果/竞技收据。extra.reason=player_forfeit，both_results_agree=false，不能虚称两端原生结果一致；独立Service从持久投递读取同一决议。两个实际Handle专项通过，重复三个退出别名不会重复战绩或积分；首次测试错误地复用已经Closed的连接，其失败日志085904保留，重新建立连接夹具后090000通过。Android真实退出画面仍未验。

### 16.3 历史09时原生引擎验证（当时未接通，现行接线见16.5/16.6）

用户最新授权PVP优先服务端权威，由本服选择方法；采用隔离CPython2.7.18运行本版Android原生battle_logic.server_battle，普通副本继续客户端原生计算。internal/nativepvp九个源文件来自更新包，引擎资源由prepare_native_pvp.py从本版Android base→patch覆盖索引转换；3098模块无转换失败，输入/输出SHA在out/native-pvp-resource-preparation.json。解释器来自NuGet python2 2.7.18，BSON为校验PyPI源码SHA后提取的纯Python依赖，仅位于项目隔离runtime；未改系统解释器或全局CET。

out/native-pvp-engine-verification.json记录真实双阵容自动原生战斗两次相同seed123456完全相同：39步、282事件、胜方唯一；手动输入窗口实际拒绝非法单位且原生状态不变，真实timeout已执行。这是运行原生代码，并非模拟battle结果。internal/nativeengine/process.go提供Go串行有界JSON进程适配器：8MiB报文、FIFO请求、结构化command_rejected保持输入窗口，其它运行错误/超时废弃并回收该场进程；不读账号或访问数据库。

仍需把适配器接到Go冻结房间、双端实体/坐标映射、播放确认、权威命令日志、唯一积分事务、恢复和AI/异步录像；当前未开启生产权威路线。引擎单独运行不代表这些接入已完成，更不代表双Android播放或整场自动/重连无bug。完成接线后重新冻结源、全仓test/vet、正式构建、真实PG及双主机组合发布；此前39项报告只属于旧源码快照；09:25正式41项绑定09:32已发布产物。后续源码若生成不同测试产物，应重新真实库，不以旧报告代替。

16.3追加实测：原生引擎已在游戏主机Debian12的项目私有验证目录运行通过，两次相同seed39步/282事件、非法输入零状态变化与原生timeout执行均通过。Go冻结共同房间的真实卡/主战援护空槽/技能潜质契印数据也已启动原生引擎通过。解释器三包均核对Debian官方索引SHA，未使用apt全局安装；PYTHONHOME仅在该Python2私有包装入口设置。新正式PG为41 PASS/0 SKIP，包括正式17池主线102首通、不重复结算和冷恢复；远端验收同时绑定二进制、热更、正式env三SHA，发布器拒绝任一漂移，18项故障模型通过。PVP完整房间/播放/恢复切换仍未完成。

### 16.4 历史09:32正式发布与回退

09:32本批游戏、145活动配置、游戏热更、独立热更及正式政策/drop-in统一发布通过；09:34独立远端核验通过，09:35实际game进程确认17个契印池各18套等权及家具8组等权与正式文件一致。游戏SHA `8b3307a23e7912bc06a633c90a989d8f264338606632b186073bc64669167373`，热更SHA `13bd23295f1392d8dded16c731944df56c29fa32183d88e6f5beeb6d8e998f48`，备份 `/opt/hs-server/releases/completion-20261008-092750-779899`。回退应恢复本批原来存在的二进制/热更并移除本批原来不存在的activity-schedules.json、gameplay-rules.env及50-gameplay-rules.conf，然后只重启所属game/hotfix；保留生产数据库定义。完整逐文件原存在状态与两主机回退位置在out/completion-release-20261008-092750-779899.json，不删整个data或systemd目录。源码与正式41项PG绑定；本批没有开启仍未接通的PVP单权威，后续必须另外验证与发布。


### 16.5 单原生权威同步PVP房间、恢复与生产运行时

2026-10-08完成internal/nativeengine与HumanPvpRoom接线：服务端唯一原生引擎决定同步真人PVP命令和胜方，客户端只提交点击并回报规范播放检查点。Journal、双方房间镜像、投递、积分和收据在同一SQL事务内提交；进程缓存可丢弃，断线按新恢复世代重建。该链已发布生产，但双Android实际输入、末态、恢复、邀请和持续负载仍需MuMu验收；异步PVP及机器人现行接线见16.6，已完成本批发布。

| 内部接口 | 持久/调用契约 |
|---|---|
| NewAuthority / Begin | 显式解释器、worker、资源目录与资源SHA；输入只用房间冻结阵容。Begin返回候选Journal和真实启动回复，不自行访问账号/数据库 |
| Advance | 输入已提交Journal，允许drive/timeout/step/set_auto；返回全新候选，不改写调用方日志。候选必须与房间镜像、投递、积分/收据同一SQL事务提交 |
| Restore / Close | 缓存末端与已提交Head不同即废弃进程，按冻结start及全部已提交操作逐条重放核验回复SHA；遇分歧禁止继续计分。Close仅回收本实例进程 |
| BindEntities | 唯一阵营/角色/类别/原生type/场类型建立一对一映射。重复EID、同身份歧义不兜底；召唤物依赖已绑定创造者、创造技能及固定棋盘出生原点 |
| DeriveCoordinateFrame | 至少两个不同初始战斗锚点，证明native=sign×client+offset，sign只允许±1；场/援护不作锚点，双方地图ID不代表棋盘中心，战后移动快照不能重新建立坐标系 |
| MapClick / MapPlayback | 当前鉴权原生回合控制者及已证明实体/坐标才能输入；范围/资源/技能资格交给原生引擎。只广播实际sync_command，保留零参强制命令和援护原回合、目标/攻击两位置 |
| Checkpoint.Track / Observe | 持久FIFO期待命令；log_command只代表组合开始，稍后原生input/turn/round_end安全边界才确认完成。援护须恢复原战斗单位，不能把援护自己的round_end当恢复 |
| Checkpoint.TerminalMatches | 客户端仅确认播放末态，胜方必须传入原生引擎已产生的结果；核对恢复世代、started/command/result序号、已消费总数与胜方一致。后续新权威动作清除抢跑result候选 |

Journal版本1，上限2048项/2MiB；原生进程每行8MiB且拒绝第二个JSON值。日志冻结start、资源SHA、每项输入/回复SHA和链Head；JSON规范化保留大整数。数据库提交失败后丢弃候选，下次用已提交Journal重建，可撤销已经执行却未提交的原生动作。回复未存完整录像，持久完整原生录像仍属E5，不能把此日志当录像文件。缓存/资源版本升级不能绕过重放校验；真实PostgreSQL已验证JSONB冷读、日志重放、双方镜像与唯一结算。链哈希先规范化JSON，数据库重排对象键不会改变Head；候选提交失败仍按已提交Journal恢复。

作者桥实际BRIDGE_REVISION为31，握手仍1。out/tools/export_native_authority_bridge.py核验更新ZIP SHA，再用AST仅读turnclock与目标观察辅助源码，不执行作者server。导出internal/game/native_authority_bridge_script.py及来源out/native-authority-bridge-export.json；包装源在internal/nativepvp/authority_bridge_wrapper.py。导出文件使用明确LF并按磁盘实际字节计算SHA，避免Windows换行转换令报告哈希失真。

新桥只装到authority_shadow子类，并要求extra_info.server_authoritative_pvp严格为True；普通shadow的rev12、原函数字典和local洗牌恢复，三个GUI钩子按当前显式权威场景分流。工厂包装的是本版原生RpcMethod.func，不能只替换组件类方法：本版装饰器注册对象与调用缓存通过同一元数据对象执行。native start_server_battle_ok同步构造shadow_battle，临时模块工厂在finally恢复，即使工厂报错也不留在新类。来源为out/dis/battle-component.asm:427和新增pvp-native-rpc-registration/RpcMethod-call汇编；桥已嵌入clientExtensions并进入runtime.index=2026100804；只有extra_info.server_authoritative_pvp为真时使用authority_shadow，普通rev12场景保持原路径。

Windows专项已实际运行：同一冻结阵容/seed的双手动超时战斗经过30输入窗口、60接收端命令转换、65原生操作日志，完成后关闭进程再冷重建回复相同。非法单位、未提交timeout丢弃/重试、伪造回复SHA、资源/操作/大整数、身份歧义/召唤依赖、坐标、援护与末态屏障均有专项；实体/棋盘13种案例与作者Python3纯函数结果逐项相同。原生算法是真实Python2字节码，接收端坐标/GUI为显式夹具，不是Android双端验收。源码先后专项、Linux报告与最新全仓检查以PROGRESS及out报告实际时间为准。

生产运行时位于/opt/hs-server/native-pvp/releases/20261008-105510-289825，current为所属相对链接；3个Debian组件仅解包到项目目录，没有安装全局Python。60-native-pvp.conf注入四个HS_NATIVE_PVP变量，资源SHA为bed95c22...19ef3。发布前运行时自动对局39步/282事件；正式42项PG/0SKIP，Linux引擎9项、房间3项和Python2桥合同均通过。10:56组合发布后游戏SHA a206a856...bdb519、热更文件SHA 55d2be83...bb2bc，游戏PID715230、独立热更PID66016；11:01独立核验未见启动错误。备份在/opt/hs-server/releases/completion-20261008-105631-389903和/opt/hs-hotfix/releases/completion-20261008-105631-389903。该段为10:56历史发布记录；异步PVP/AI权威化已在16.6完成，现行缺口以REPAIR-TASKS余项复核为准；规范命令日志不等于原生录像文件，E5仍按录像门槛单独验收。


### 16.6 单客户端PVP原生权威、实际调用与恢复

`native_pvp_solo.go`将新建同步机器人、异步真人防守/机器人接入既有私有Python2原生引擎；使用本版type2/type3，不为绕过AI报错改战斗类型。`BattleSession.NativeSolo`保存冻结start、Journal、InitialUnits、单客户端映射/FIFO检查点、Canonical与投递描述；真实异步防守者无需在线，也不注册播放Client。

| 内部接口或原生RPC | 现行契约 |
|---|---|
| markNativeSoloSession / nativeSoloLoadExtra | 新会话事务冻结server_authoritative_pvp；start_server_battle_ok携带server_authority_generation，按标记选择已有rev31子类。缺运行时或非法阵容整事务拒绝，不退回客户端结果 |
| prepare.ready / battle_fighting / beginNativeSolo | 本版prepare先发ready，再上报阵容。冻结前只接受世代1/rev31/握手1的ready；随后从真实拥有卡、主战/援护槽及服务端候选启动原生。双方UUID不能重复，不接受上行属性或奖励 |
| nativeObservePlayback / nativeSoloDoCommand | 单端与真人双端共用实体/坐标映射、FIFO期待与安全边界。move_to/use_skill必须属本人当前原生窗口；未知/非法点击零写入拒绝，播放未完成不允许下一个动作 |
| tickNativeSolo / settings | 手动等待原生窗口超时；单端允许原生settings持久自动偏好，自动时按原生timeout算法推进，仍等待规范播放确认。真人双端继续禁止客户端自行自动输入 |
| absorbNativeSoloEvent / absorbAsyncPvpResult | 原生必须产生房间内唯一胜方，客户端只确认播放末态。同步机器人积分/收据与Journal同玩家事务；异步真人攻防按原有双角色锁同事务计分/记录。机器人不建立假账号，finish_bonus仍为每日21点邮件 |
| resumeNativeSolo | 冻结前及冻结后均保留原UUID/seed/票据；冻结后保留完整start/Journal，换恢复世代，重新加载并从Canonical完整播放；新Service无缓存也可JSONB冷读重建。旧世代Outbound删除防恢复体积膨胀 |
| result重试 / 已结束恢复 | 原序号、正文、世代与唯一收据，篡改重试拒绝；恢复仅补原结果。extra.client_authoritative=false、server_authoritative=true、verified=true及authority_digest；verified表示权威/播放契约通过，不表示Android验收 |

原生宿主补齐依据：`51169E3F.auto_battle_habit`的异步type3分支读取battle.avatar_info.extra.asyn_pvp_auto；`2CC3F05D.set_battle_candidate`赋值avatar_info；`AE9F02E4`构造原生资料对象。宿主据此提供原生对象和冻结偏好，不改原生AI算法；完整指令及marshal SHA在 `out/dis/native-solo-ai-contract.asm`，工具 `export_solo_ai_evidence.py`可重现。当前未设置逐卡战斗习惯时使用原生空表，不伪造历史。

Windows `check_native_solo.py`实际执行三种对局及真人回归，覆盖早握手、非法点击/提前伪胜方/收据正文篡改零写入拒绝、原生动作执行后故意拒绝提交、同UUID冷恢复与结果幂等。Linux `check_native_solo_linux.py`核对上传SHA并在项目私有verification目录执行引擎及单双端专项；游戏PID前后不变。新增正式实库用例为 `TestPostgresNativeSoloAsyncAuthorityAndOfflineDefender`、`TestPostgresNativeSoloRobotAuthorityAndColdRecovery`，绑定本批正式dbstore.test/热更/政策三SHA；旧42项报告在out/backups/before-solo-verification-20261008-112309。

本批沿用runtime2026100804和原权威桥正文；新会话按服务端标志选择权威子类。发布前已经进行的旧rev12会话保留原契约，不假造历史Journal。用户尚未操作双Android，自动按钮、原生场景/录像、断线重放和持续负载仍待验。E5完整录像不由Journal代替。私有运行时只在项目目录，不修改系统Python；回退必须同时恢复本批游戏/配置与私有运行时current原链接。


### 16.7 年兽总榜原服反推历史与周三日奖修正

本节保留原服不能唯一推出公式的历史取证。当前已采用用户批准的名次和/缺席N+1/小时快照/周奖政策实现，见17.7；本节旧“本批不实现”仅指当时未获政策的批次。

用户没有旧服数据，要求继续静态反推。已逐层核查Android基础包及补丁优先覆盖：年兽GUI `F33CCCB6`、通用榜管理器 `98236F87`、`rank_detail_info`类型 `7EFB6B1E`、年兽管理器 `385E6D63`、日奖/总榜/伤害奖及排名表；参考服务器 `monster_nian_logic.py`也没有总榜公式。全包 `sub_ranks` 引用清单在 `out/nian-sub-rank-references.json`，年兽/相关榜模块清单在 `out/nian-rank-module-inventory.json`。所有反汇编均写原始marshal SHA，工具 `extract_native_methods.py`可重现，不执行恢复脚本。

| 反推链及详细资料 | 可确认事实 | 不能据此确认 |
|---|---|---|
| `out/dis/nian-total-rank-chain.asm` | 总榜请求query_rank_list(10,0)，三分榜请求各副本ID；客户端只读取服务端榜单 | 综合分计算函数没有在这个GUI中 |
| `out/dis/nian-rank-score-fields.asm` | 总榜显示服务端sub_ranks[0..2]，0显示“－”；分榜显示score[0]；个人self_ranks也读extra.sub_ranks | 0是显示哨兵，不能当综合得分0或未参赛惩罚值 |
| `out/dis/nian-rank-manager-chain.asm` | cal_pvp_rank_info仅按已有返回顺序编号，并透传score/sub_ranks；非同步/异步PVP（包括年兽）同分仍按idx+1显示，不做并列名次 | 原服同分排序键、权重、名次和/积/其他函数；列表顺序由服务端决定 |
| `out/dis/nian-rank-detail-schema.asm` | rank_detail有score与sub_ranks的客户端属性，update只装载收到的数据 | 类型定义没有综合公式 |
| `out/dis/nian-rank-record-chain.asm` | “伤害记录”是三个dungeon_info.max_damage中的最大值 | 这是另一展示指标，不能用来代替总榜或周累计伤害奖 |
| monster_nian_total_rank / monster_nian_daily_rank / rank表 | 总榜和日榜奖励档次确定；rank10通用名称为“总伤害” | 名称不足以推翻原生“三榜名次综合”提示，奖表也没有计算函数 |

仅凭上述客户端数据结构无法唯一恢复综合公式：相同sub_ranks可由名次和、名次乘积或加权公式生成不同榜序，当前没有旧服总分/榜序样本能排除它们。例如纯数学反例A=(1,10,10)、B=(6,6,6)，若名次和越小越好则B领先，若名次乘积越小越好则A领先；两种都能下发同一套sub_ranks并满足客户端显示，这不是原服样本或拟采用政策。因此本批不实现假公式，不将未参赛显示0视为奖励计分0，周收据仍冻结三分榜并标明缺公式；不再要求用户手动提供其没有的数据。后续需原服务端函数或可验证旧服样本才可关闭总榜/周奖。

已确认并修正独立错误：`monster_nian_rank_reward`原生文本为“每晚23:59:59结算排名奖励,奖励以邮件发放（不含周三）”，而旧实现周三也发日奖。现行ActivityRankReceipt新增可选reason，周三保留已知截止名次、写关闭收据及说明，不设置bonus、不发日奖；周榜三分榜冻结及清榜仍同事务完成。未参榜或缺历史继续保留原零名次/缺快照边界，非周三仍按原生日奖表发邮件。本批不追回旧版本已发邮件，不重造历史名次。

本地测试覆盖周三不发、周四正常发及原有邮件容量事务回滚；真实PG的TestPostgresActivityCalendarAtomicDailyMountainReloadAndRetry扩充周三/周四跨日、原生奖表和重建Store/Service后完整进度幂等。该修正已和单端PVP同批构建、44项PG回归与发布，不把测试夹具当原服公式样本。

这项新增PG断言首轮把首次周四跨日登录与日奖重试混在同一完整进度比较，失败日志已保留。补齐跨日登录后仍失败，递归差异证实仅server_activities/wangyan/supply/last_time由1792684798变1792684799；refreshWangyanSupply在满值登录时合法写当前时间，邮件/收据没有改变。现先在截止同一时刻完成真实周四首次登录并验证achievement_login_day，再以同一时刻重建服务冷登录；仍比较完整进度，不豁免成就、补给或删除断言。诊断工具为inspect_progress_delta.py。数据库产物较大，`upload_database_delta.py`可从此前已核SHA的项目私有db-*普通文件复制到本次新目录，按256KiB块更新并核整文件SHA；超过半数区块变化则用原断线续传。绝不覆盖旧验收目录或生产产物，正式验收仍独立检查测试产物、热更、规则三个SHA。

首次差分上传漏启批量写请求，逐块确认导致耗时偏长。已只停止本次本地Python上传进程（PID6228，工作目录及完整脚本路径核对），保留私有db-20261008-115507目录，再用批量写入新目录验收；没有停止游戏或PG。上传关闭后仍核整文件SHA，不能用write返回替代上传成功。一轮预检在新产物的PG证据未完成时正确拒绝，报告completion-release-20261008-114810-956171.json及115238-198575.json保留，均未连接生产部署或写入在线文件。

本批发布证据：本批游戏SHA `9621cde391fff0a0238b417f4668c606dd79a9b91ce06c969f4ec795104e881a`，热更SHA `55d2be8356dca5d9cf5fca2b92b143179f65e3777fca825fe6ef97950aabb2bc`（runtime2026100804正文沿用）；回滚备份 `/opt/hs-server/releases/completion-20261008-121214-742541`。正式PG 44 PASS/0 SKIP，产物SHA绑定最新构建；Linux单端/双端原生专项通过。 私有运行时：`/opt/hs-server/native-pvp/releases/20261008-120919-961724`；原链接：`releases/20261008-105510-289825`。

### 16.8 余项审计与状态定义（2026-10-08）

12:48审计为本轮施工前历史快照；旧31条未接及G06“两条配方”结论已撤除，现行十五组实现在第17节与remaining-functions-audit-20261008.json。源码、定向、全仓、实库、发布、Android与持续负载各自记证，历史原服公式缺证不等同批准本服政策未实现。
## 17. 十五组同步实现、原生契约与本服规则（2026-10-08）

本节替代此前“31条未接线、两条随机碎片配方、活动直接拒绝、运营政策待答复”的现行结论。历史反汇编仍证明原服缺失公式的边界；用户已明确答复“统一采用推荐本服规则，继续全部完成”。业务规则以原生直接证据优先，缺失原服生成逻辑采用明确批准的本服政策，不把政策写成原厂恢复结论。

统一开关为 `HS_REMAINING_GAMEPLAY_POLICY=local-20261008-v1`，未配置默认关闭；正式政策绑定在 `deploy/data/gameplay-rules.env`、`out/gameplay-policy-approval.json`、`out/remaining-policy-proposal.json`。此前家具八组及契印17组政策保持原批准范围和配置，没有额外增加契印池。全部代码、报告、实库、最终产物和生产发布分层判断。

| 组 | 功能 | 当前源码与规则 | 对应清单 |
|---|---|---|---|
| G01 | 成就及底层业务 | 探索9、委托2、绝密2、住客3、点赞6、演练3、学会及雅努斯6全部接真实状态/结算收据；学会仅本轮六条及雅努斯基础。 | C3,C8,C10 |
| G02 | 非好友助战 | 真实同服候选最多10人；双向黑名单、指定卡、服务端名册授权；实际开战每卡每日1次并冻结、恢复不重计。 | B4,E1 |
| G03 | 送礼小数 | 批准本服有理数最终乘积一次half-up；整数既有分支、32000上限与整笔回滚保持。 | B5 |
| G04 | 潜质重置 | 原生四参数重置一级并返还一点潜质额度，不退升级材料；后置依赖、战斗占用及过期等级拒绝，全事务。 | C3 |
| G05 | 特殊头像框 | 领取起算、limit_days实际小时、多份叠期、真实时间账本；保留合法选择，过期默认3。 | C5 |
| G06 | 碎片合成审计纠正 | 1688材料无type4/stype10入口，77配方全部固定单卡；5条random_frage_rules未引用，规则2/3不是两条有效配方，不造入口/等级保底。 | C7 |
| G07 | 收藏室 | 窗口冻结随机住客奖励、原式能量生命周期、104系统开放后一次房3ownership、真实6004/26004教程9000秒一次。 | C8 |
| G08 | 夏活 | 封弊者直接/间接伤害及治疗1.6原生效果；鱼长按原区间0.01闭区间等概率，钓鱼开始冻结。 | D2 |
| G09 | 克苏鲁 | 2d6+fix检定，2/12优先极值、SAN+20/-10、失败后一次20 SAN all-in；事件访问收据冻结，重复不重掷。 | D3 |
| G10 | 初音 | 逐手册buff不去重冻结；每地图首次完整终点后一次10对应和声材料惊喜，永久收据。 | C4,D4 |
| G11 | 山海 | 同材料槽倍率先合并再floor新增部分；七日常驻赛季、旧榜封存与真实参赛奖励收据。 | D5 |
| G12 | 汪言 | 未占领影响节点原生enemy_level_added及敌buff冻结；占领只影响下一新战斗，恢复沿旧快照。 | D6 |
| G13 | 年兽 | 三榜名次和、缺榜N+1、同分参榜数降序/OID、同服整点快照、周三23:59:59原表周奖及原子邮件。 | D7 |
| G14 | 宿舍骰子 | 新连续非SSR残页事件计数，SSR归零、非残页不计；下一棋盘取原生阈值软概率；移动回真实材料字典，动画候选由客户端生成。 | D8 |
| G15 | 邮件 | type7/stype1随机礼盒签发时隔离规划并冻结真实资产/UUID；领取容量失败整笔回滚，重试不重抽。 | E7 |


### 17.1 真实成就接线与事务

原审计254条成就中223条已有映射，本轮31条均补真实可达链：探索章节9条(type6)、幻书委托2条(type7)、绝密任务2条(type48)、住客3条(type1006)、双向点赞6条(type1009)、特别演练3条(type66)、学会/雅努斯6条(type26/21/29)。详细ID、原目标参数及接线文件见 `out/remaining-functions-audit-20261008.json`。不能把254条有映射等同于全部284目标都独立完成或Android逐项通过；本轮学会活动成就走实际雅努斯type21参数14，未新增学会活动12/16的完整系统。

探索/绝密/演练与普通奖励、成本、节点推进、Frozen结算盒、Finished和成就置同一角色事务；失败或非法任务整笔回滚。真正成功后才推送成就属性，含委托 `achves/achv_value` 的即时下行，不依赖重登。点赞用真实双角色事务；年兽/山海/学会周期用同服全量事务，容量/邮件失败一起回滚。整数键、ObjectID、卡/契印UUID及大于2^53资产通过原生typed wire与JSON冷重建保留，不将内存临时状态当持久收据。既有历史无真实流水不追造。

特别演练ID2103/2303/2503对应任务211200/231200/251200，原表伤害阈值7151129/11037000/14383421。以原生tower_task引擎完成任务为凭据，服务端只接受本副本配置及授权的任务，拒绝跨副本、重复任务；win才推进type66。不会按客户端任意伤害字段或命令数伪判完成。普通副本仍由客户端原生引擎计算，这条任务上报信任边界必须保留。

### 17.2 探索、幻书委托与绝密任务

三者持久化于 `Progress.RemainingGameplay/remaining_gameplay`，包括 `Stages/CurrentStage/PendingSite/PendingDungeon/TaskTowers/PendingTaskDungeon/Consign/PowerTypes/PowerType/AutoList/AutoFight/AutoFighting`；原生登录属性由remainingGameplayProperties投影，隐藏授权字段不下发。源目录 `remaining_gameplay_catalog.json` 与 `remaining_chapter_gates.go` 使用本版原表，不采空目标集当完成。

| 上行接口 | 原生参数及主要作用 |
|---|---|
| enter_free_stage | stageID,refresh（无callback）；9章10101–10901、70实际节点，原表权重初始化 |
| leave_free_stage | 无参数；结束实际探索访问 |
| enter_stage_site | nodeID,delay；delay非负、当前章可达节点，不授权任意副本 |
| set_free_stage_power | callback,powerType（1或2） |
| set_auto_free_stage_power | callback,节点ID→powerType整数字典；键为节点不是副本 |
| set_auto_list / set_auto_state / set_auto_fighting | 当前章节点列表 / bool / bool；实际进入仍要求节点解锁 |
| unlock_chapter | chapterID,state2；原生UI已见标记，必须先满足真实章节门槛 |
| enter_task_tower | callback,towerID,floor,extra；101/102各5层，各15实际任务 |
| start_consign_task | callback,taskID,card模板ID列表；拥有且不重复，不与其他未领委托占用冲突 |
| drop_consign_task | callback,taskID；未开始拒绝，已过结束时刻拒绝，原生恰好结束时刻允许取消 |
| refresh_consign_task_one | callback,taskID；仅未开始，消耗1000委托点并换唯一候选 |
| commit_consign_task | taskID；真实结束后一次结算；success(taskID,cards,box)/failed(taskID,ret) |
| reward_consign_phase_bonus | 无参数；on_reward_consign_phase_bonus(ret,原生bonus.bonus) |

探索节点状态1可用/2锁定/3完成；类型1战斗/2宝箱/3随机/4首领/5精英。随机类型1副本/2奖励/3平移/4双倍，本版后两者权重为0。普通/精英/首领与起终点按原表抽取，完成后原邻接unlock_sites开放，真实首领完成才推进对应章节成就。各章节先满足 `site_unlock_dict` 的主线关卡与activity_type2系统门槛，`chapter.unlock_condition` 类型3为真实 `checkChapterFinished`：以site.chapter_id的1–3章节ID遍历实际chapter_dict，排除支线3/7和活动9/10，要求所有目标副本已Cleared并检查前章；不是拿10101等大ID当章节，也不制造空目标完成。

剧情/宝箱随机奖励实际扣5体力并发原奖；副本powerType按原生倍率付费；BossDouble只乘奖励不乘体力。prepareRemainingDungeon先剥客户端server_free_stage_id/node、server_task_tower、server_free_stage_reward_multiple和tower_task_list，再从实际副本表重填任务、从真实访问状态冻结授权与倍率。普通与纯剧情结算均接真实事务，完成后 `on_finish_stage_site/on_finish_free_stage` 与奖励一并下行，重复result沿Finished原收据不再发。

绝密为原activity_type19，并非每日secret_tower。普通入口不开放7001–7005/7011–7015；专用接口核对实际tower/floor/dungeon后冻结未完成的首批最多3任务，结算任务必须属于这份授权。原30任务no_need_win=0，要求win，每任务只发一次实际bonus，保存Finished与Received时间；各塔实际15次才达成type48。输赢结算都消费该次Pending。原表unlock_condition为空，不擅自加上一层先通门槛。原生finished_task_list判定仍由客户端任务引擎上报，服务端严格约束集合。

委托设施5等级0/1–6并行1/1/2/2/3/3/4，每日任务6/6/9/12/16/20/24。5点日界来自原生consign_base；20:00–次日4:59为9小时池，日池3小时、教程池0.1小时。初始点5000，上限5000，每次刷新1000；空房200点/小时，1–5住客260/320/380/440/500。按秒累计并保存/3600余数；入住或设施变化先结旧速率，新速率只作用未来，满点清余数，回拨拒绝。5点保留已开始及已完成未领任务，仅移除未开始并补当前容量；首轮沿consign_refresh_pool，后轮沿标准池，超出原数组槽位用池3；夜间新增/刷新沿池9。20点仅切新生成池，不改旧计时或凭空发阶段奖。这些生成时点属于本服政策。

委托池按馆主1–60级原累计星权差值和实际任务权重抽，不重复模板ID；耗尽星档权重归零再规范其他档。开工检查真实拥有卡、原teammate数量、占用、并行及每日上限，冻结生成时等级和原Task.Time；时长减免沿设施与已解锁第三characterTag的原生AffectHouse最高效果，不叠加全部标签。领取沿生成时等级的bonus_pool，首队员真实命中hideStoryCards才给hideBonus；阶段基础及匹配tag额外项在实际完成领取时冻结，进入待领奖总账。阶段领取一次取累计未领真实奖励并留时间/历史账本，不重抽，不把未运行任务或20点日切伪造为奖励。完成事务同时推进1次/100次成就；原生phase下行为contain_items/contain_runes/buff_ids/is_double/bonus_id，不能发通用box替代。

### 17.3 非好友助战、赠礼、潜质重置与头像框

非好友沿既有 `refresh_assist_use_times()`、`select_assist_card(callback,UUID,{eid,hostnum})`，不用新造RPC。服务器从同服真实查询窗口最多1000角色中等概率抽至多10候选；行锁事务再验指定卡、归属、禁用、双向黑名单、两边无好友关系及持久 `Social.Assist.Strangers` 名册。不能凭外部UUID选任意角色。选择和布阵不计次，实际battle_fighting同一双角色事务按关系2原限制每卡每日1次，主动/被动计数、借卡runes/skills快照与实际奖励绑定battleUUID；冷恢复沿Frozen/LastBattle不再授权或扣次。日界为本服UTC+8政策；该查询窗口不等于全服绝对均匀抽取。

特殊赠礼仍单份count=1，采用赠礼前偏好发现状态。原未证小数分支现按批准规则以有理数计算最终乘积、只一次正数half-up：20×1.1×1.1=24.2→24，20×1.1×1.2=26.4→26，50×1.1×1.1=60.5→61；32000上限后截断。整数旧规则不变，材料、好感、回礼、每日与累计次数一事务。未配置政策时仍不开放未证小数分支。

`reset_talent_node(cardUUID,branch,index,currentLevel)` 与升级相同四参，无callback参数；回 `on_reset_talent_node(ret,branch,index,currentLevel)`，新树在card_mgr属性。校验卡真实归属、级/品解锁、节点合法/非forbid、当前等级一致；已开始战斗使用的卡拒绝。只降一级并重算资格，已升级后置节点不能失去前置。原生说明仅返还一点潜质额度，不返还升级材料，也不收重置材料；额度由树中当前等级重新计算，不伪造材料退款或独立余额。失败树/材料不变，成功card_mgr/material_mgr及回调一次。

框80040材料type9→1041；limit_days虽名days，本版UI实为小时，该框1小时。所有合法框材料领取才起算，签发邮件/周奖不提前计时。同框expiry=max(now,旧expiry)+份数×原小时×3600，wire保存expiry–原单份时长以沿客户端公式，允许该值在未来。旧永久0保持；当前合法选择保持，多个框不自动抢选，过期回默认3；head_cmp原rank仅界面列表排序。`Progress.HeadFrameGrantTime` 保存真实领取时间防回拨，异常数量/非有限数/超过9999年整体拒绝。

### 17.4 G06原审计纠正

本版materials1688条中type4/stype10随机碎片为0；当前compose目录77条全部固定单卡，unverified_promise实例0。random_frage_rules五条在3099脚本中仅有Record及构造器，无实际消费者；规则2.check_level30、规则3.check_level1并不能证明馆主或卡等级。撤除“两条有效配方被拒绝”的结论；没有新增材料ID、入口、配方、等级保底或等概率卡奖励。若以后明确启用须先取真实材料与资格字段，不把馆主<1的不可达条件冒充完成。77条现行固定配方保留原成本、等级、品阶及资产容量事务，Android全合成UI仍须验收。

### 17.5 收藏室随机礼物、点赞、能量、房3与教程

住客目录79卡，UTC+8窗口1为06:00–13:59:59，2为16:00–23:59:59，每窗random_num1。实际心情=min(mood_max,max(0,舒适度×mood_transform_rate))，阈值取最高达成索引+2，奖励用最高匹配mood_status。窗口首次进入时从真实拥有/已解锁房间实际入住且满足条件的不同card候选等概率抽，跨房全局1个，首次空候选也记规划防重进重抽。保留原表第二整数raw_second而不误当数量，只grantBonus一次；同时冻结原房、卡、bonus、真实材料/卡/契印及UUID。此抽取时点/等概率是本服政策，原native表直接给窗口/条件/奖励项。

`player_get_house_random_reward(window,card)` 回 `on_player_get_house_random_reward(ret,box)`：日期/当前窗口/状态1/拥有且仍在原已解锁房间全部满足才领；搬出不能领，搬回可一次，过窗失效，不追造历史。累计实际领取1/30/100时分别触发210501/502/503，不按raw_second假设事件。随机礼物解锁613，固定日奖解锁604是独立系统。`house_daily_random_reward` 为window→Int2TupleDict(card→Tuple[1可领/0已领])，内部Frozen Mail只是资产规划容器不插邮箱。

`query_friend_dormitory(ObjectID,hostnum)` 回 `on_query_friend_dormitory(ret,ObjectID,restroom.house_total_info)`，同服真实双角色、禁止自访、双向黑名单、隐私与好友关系鉴权，保存VisitOwner/VisitTime；`end_visiting_house()` 清授权。`like_friend_house(ObjectID)` 回 `on_like_friend_house(ObjectID,ret)`，UTC+8同owner每日一次、每天99次，双方累赞/日赞/六成就同一UpdateSocial事务，overflow或资格改变双边回滚。在线对方重载；house_likes、house_likes_map与daily_house_likes投影用原生类型。

能量按设施当前upgrade_effect首个efficiency_improve cost，max=30×住客数、base=cost/unit、per=cost×(1+moodProfit+tagProfit)/unit。本版设施2/3/4所有等级没有efficiency_improve，base/per实际为0，这是原表结果，不虚构消耗或免费产物。机制仍先结旧存档per，真实delta扣到0，30秒更新保留余秒；当前生产设施已解锁且有人才启动，耗尽停，换人/读取不补满，空房后重新入住才重启。参数变化先结旧速率再算未来。能量内部settled_time/resident_count/eligible不投影；start_flag/last_update_time/last_get_time/interval/base_cost/per_value/value按native schema。`update_last_get_time(room)` 无callback参，先持久并同步小数服务器时刻后 `on_last_get_time_update(room)`；原get在start_flag=false仍记now，只有真实shut_down转换清零，空房冷读取保留。未解锁、回拨、损坏旧时间在初始化或重置前拒绝并回滚。

房间3两版system_unlock均104，原材料仅809房1/810房2，无3钥匙；批准政策在真实house_dormitory3系统取得后一次建立ownership，设施4仍803与原费用，不制造钥匙。教程ARM完整恢复两份res共23032索引及压缩块0错误，真正6004/26004图均只有GuideFacilityAcc(设施2,9000秒)，6005/6006无节点。`gather_produce_material_speed_up(facility,seconds)` 无专用回调，只同步属性；仅实际进行Status1的6004或26004、设施2、9000秒允许一次；两任务同时进行/其他参数拒绝。产出Keep遵守Storage，ProduceStart不移未来，按guide保存一次收据。

### 17.6 夏活、克苏鲁、初音、山海与汪言

夏活prep冻结SummerClosedBeta；八角色模板对应原生六个封弊者书名，直接/间接伤害及治疗各沿原生1.6，不改技能轮数/原cap。鱼种/原body_form区间、0.01闭区间等概率鱼长、鱼饵及开始时刻在开始钓鱼冻结，同次重试取Pending，不重复扣饵；精确分布为批准本服规则。

`cthulhu_item_check(itemID,mapID)` 与 `cthulhu_check_all_in(itemID,mapID)` 已实现，回 `on_cthulhu_item_check(ret,res,box)`；res含dice1/2、check_res(1成功/2失败)、big_enable。原2d6与fix_check相加后≤属性成功，负fix如-2也沿加法；批准规则先判断原骰和2大成功、12大失败，SAN+20/-10封100/0。解锁资格可来自不同地图实际finished_unlock_all_in层。初次失败含大失败、尚在该访问事件时一次all-in消耗20 SAN完整重掷，第二轮仍应用极值SAN；离开放弃。按Cycle/Layer/Trunk/Branch/Moves/轮次存CheckReceipt，箱与消耗冻结，同访问重试返回原骰/原奖励，重新真实访问才新一次检定。全部SAN、材料、属性、访问信息、奖励与收据一事务。

初音map激活手册效果逐项冻结camp1原生buff，2080021攻击+10%、2080001生命+10%；重复ID代表原生层叠不去重，user_property=null与role_tag12筛幻书沿原生引擎。`receive_miku_surprise_bonus(mapID)` 回 `on_receive_miku_surprise_bonus(ret,box)`，批准政策为每地图首次完整抵终点后一次10对应和声208095–208098。MikuMap.CompletedRuns/SurpriseClaimed/SurpriseBox持久，不拿旧全局Ends推定每张图首通。

山海新battle冻结MaterialRates，按同一材料合并槽倍率后floor(原奖×合并倍率)发新增部分，不先逐槽舍入；其舍入为本服政策。按原七日与22点截止每挑战开启七日常驻赛季，MountainState.Seasons/ActiveSeasons按季保存ProgressBonusRankAtHardAPCards，旧场跨季只结旧进度不混新榜；Calendar按pid/season截止排名、bonusIssued真实收据只给实际参赛赛季，不补无历史。既有守护/结界原生buff/mf_skill保持。

汪言按2BCB4D1C原生未占领影响节点取combat_power_added/buff_added，战力比映射enemy_level_added=-20/-10/0/45/150；原生on_bid_set非0时max(原级+修正,5)，camp2实际节点buff。ctx指针保留0区别未冻结，新战斗prep冻结占领与战力，capture只影响下一新战斗，恢复沿原快照。剥客户端enemy_level/skill/hp/atk/defence等假字段，不接受自报增益。

Android活动扩展和服务器私有Python2的activity_buffs_native.py使用同一安装函数，重复安装不捕获旧gworld；HS_ACTIVITY_BUFFS_BEGIN/END及_start_hs_activity_buffs rev1并入现行桥。实际24原生用例覆盖1.6、角色过滤、原cap、手册叠加、最低5、+150及敌攻击+50%。这是真实引擎执行证据，不等于Android实际完整战斗验收或最新生产已发布。

### 17.7 年兽综合总榜、小时快照与周奖

`query_rank_list(10,0)` 已返回总榜。本服批准Score=sum(三分榜名次)，升序；缺榜按该榜实际参赛N+1；至少参加一榜入榜，Score相同Joined多者优先、再OID稳定序。1000只是下行列表上限，个人rank走全量实际COUNT；未参赛0仅旧UI哨兵，不拿0当最好名次。真正0伤害但有Ranked/RankAt/小时结果收据仍算参与；旧0且无历史时间不补造参赛。

`NianHistory[did][hour]` 冻结At/Damage/Cards；同服UTC+8每整点从全量真实分榜冻结 `Calendar.NianTotal{Hour,Rank,Score,Joined,SubRanks,Cards}`。周三23:59:59冻结截止总榜，按monster_nian_total_rank原档1–100→20205001、101–1000→20205002、1001–9999999→20205003寄周奖，NianWeeks持久总rank/score/joined/bonusIssued，然后新周清榜；个人MaxDamage/TotalDamage不清。周三仍不发日榜奖，其他日沿既有原奖。邮件容量失败同服全事务回滚，重启重试按周收据一次，不给缺历史周补假奖。本服公式与时点明确标运营规则；16.7的反推仅证明原服不能唯一恢复，并非当前缺实现。

### 17.8 宿舍骰子与随机邮件

移动原生第4参数必须是实际材料奖励整数键字典，失败为空字典；869A57A0转发、1CBBBF23迭代items，候选动画get_random_materials/get_end_materials/get_random_props由原客户端构造。服务器不补十个候选资产、不将动画发奖。

HouseFrageState隐藏PolicyVersion/ConsecutiveWithoutSSR保存连续实际非典藏残页奖励事件数，每次非SSR rarity1–3发奖+1，与数量无关；实际SSR rarity4归0，固定祈愿及open_game_site直接残页也遵此，金币/骰/UP/空奖/商店及其他玩法不计。老存档无顺序不由Total-SSR猜，政策迁移从0，Total/SSR历史统计保留。仅下一新棋盘把该计数传per_count；原严格下一阈值10/15/20/30/40/10000对应倍率0.6/0.7/0.85/1/1.2/1.4，≤0基础，超末档沿1.4。进行中/重入不重抽，失败/回滚不推进。此为软概率调整，不宣称固定次数必SSR；原服务端派生缺失已由批准政策替代。

随机邮件复用SendMail/现行管理与周期发件接口，不增客户端RPC。仅原生type7/stype1即时展开礼盒、合法target和数量≤10000，签发时freezeMailRandomRewards在隔离CloneProgress规划，保留真实通关/等级/累计资格，清空副本背包余额避免当前背包容量阻断签发；真实玩家签发时不获资产。Mail.SourceAttachments保原附件、RewardsFrozen标记及展开实际材料/卡/契印UUID持久；卡/契印与显式附件合计各≤100，冲突/溢出拒绝。外部不能伪送已冻结收据。领取仍检查真实容量并与所有附件一起原子入账，失败不改变邮件或玩家，重试/冷登录永远原箱、不重抽；框在真正领取时才起算。未知非即时礼盒子类型不扩大开放。

### 17.9 学会六条成就与雅努斯基础

此次范围为403001真实入会、403002/403003真实1/30次活动、403004–403006简单/中等/困难雅努斯真实win且50普通怪。本轮不实现完整捐献、学会成长/全部职位/学会PVP系统，不以已有六条成就称全学会功能完工。

| 原生直接RPC（无callback前参） | 下行或主要规则 |
|---|---|
| create_new_league(name,public_notice,cardID) | on_create_new_league(ret)；馆主6级、同服名称唯一、原创建材料费用 |
| apply_league(ObjectID,nil) | on_apply_league(ret,ObjectID)；真实申请或实际自动同意 |
| agree_league_apply / refuse_league_apply(ObjectID) | on_方法(ret,ObjectID)；真实管理员、待审批、原容量及离会4小时 |
| leave_league() | on_leave_league(ret)；会长有其他成员须先转让 |
| appoint_league_member(ObjectID,1) | 仅会长真实转让；旧会长降普通，新会长1 |
| league_setup_auto_agree(bool) | on_方法(ret,bool)；实际管理权限 |
| query_total_league_info(ObjectID,hostnum) / search_league(name,uid) | 原native字典/列表，不额外加ret前缀 |
| league_protect_start(protectID,extra_support_skill_idx) | on_league_protect_start(ret,protectID)及真实battleload |

OwnedLeagues主记录只在真实创建者JSONB中，真实ObjectID/同服唯一UID、Members实际OID字典，全同服UpdateAllSocial原子创建成本/申请/成员/转让/成就；转会长后创建者可离会，主记录不丢，最后成员离开标Dissolved。LeagueProtect隐藏账本存TotalWeekly/CurrentTimes/Contribution/ProtectID/Kills/Unlocked/Updated/LastTime/Week/Pending/Consumed/Results；native league_activity_weekly_info/protect_unlock_map投影真实值。

原提示周二/周四开放，每次开放窗口限实际开战一次，准备/布阵离开不计；battle_fighting在授权/阵容冻结后消费。Week取最近开放日，不用周一周界；下一开放重置当场次数/贡献/kills但保Unlocked/Results。Guardian为原real_role_id=-1/role[-1]，技能91001/91011/91012/91013，援护91012/91013/91011门槛1/30/40，初始guardian1级只index0；不能拿普通4401卡技能冒充守护。恢复从旧UUID迁移Consumed别名，跨窗口已开战只结旧场、不花新次数；旧未开战prepare跨窗口须重新授权；Finished原箱复播不发重复奖励。

原生metrics rev2来自get_statistics()[2]非召唤死亡，普通kill_count0..50与treasure_role_id0/501分开，非法51/小数/假tag全回滚，不按win直接当50。原奖档9–18/19–27/28–39/40–49/50给111011–15、111021–25、111031–35；0–8无档，真实输局仍按kills档奖励。宝藏怪111101/2/3仅实际501一次，不计普通50。贡献为原basic20/30/40×普通kills，宝藏不贡献。实际win+50推进type29[100,protectID]及下级unlock_based_id，不猜未消费unlock_growth200/400的学会成长；每次真实battleend推进type21[14]。结果extra.league_activity_battle_result由原native dungeon_mgr自动调用on_league_protect_end，不重复发两次弹窗。

### 17.10 取证、源码与分层验证

四份短期JSON详细报告归入本节，不新增MD：
- `out/remaining-numeric-report.json`：探索/委托/绝密协议、原表门槛、事务/冷重建、G03/G06/G14，定向Go PASS14.283秒；9完整章节、30绝密任务与100实际委托。
- `out/remaining-activity-report.json`：G08–G13、原生24实际用例、Go12专项、SQL小时/周榜、四个新增实库测试与历史迁移修复。
- `out/remaining-collection-report.json`：G05/G07及9条成就、原件教程23032资源恢复、能量正式规则7专项、两个新增实库测试。
- `out/remaining-league-report.json`：真实成员状态、六成就、原生guardian/死亡统计/原奖、5专项与30跨窗口/UUID恢复的新增实库测试。

这些定向PASS不代替全仓；本地因未设置HS_TEST_DATABASE_URL跳过的PG仅为编译成功。首次55项远端实库跑到40项PASS后在旧初音reconcileMikuAchievements遇Achievements空映射panic，该次不能计全量成功，修复后须重新绑定最终源码与测试二进制。另会话临时legacyprogress文件被删除导致源码漂移预检拦下发布，最终已完整重跑通过；不拿此前44项生产PVP批次报告填本批。无avatar_progress遗留行全服结算隔离及匿名玩家/2生产事务修复见文首最新现场记录。

本轮默认/正式政策全仓test、vet、Linux真实原生、最终构建、56项真实PostgreSQL、生产组合发布：**默认及正式规则全仓、vet、最终构建和Linux真实原生全部通过；真实PostgreSQL 56 PASS、0 SKIP，另3项辅助通过；游戏/两端热更/私有原生运行时已统一发布并独立核验。Android完整玩法与持续负载仍待验收**。最终统一报告为 `out/remaining-completion-verification.json`（已记录实际时间、SHA、PASS/SKIP、回退与运行期清单）。Android新账号自然教学、15组真实UI/战斗、双端PVP/原生录像与持续并发负载仍未完成。14:01整包至登录页仅关闭F2/热更启动子门，14:10闭包登录重播已由14:35 runtime2026100806现场通过，教学10001四参prepare、16/16预载及普通攻击教程入场已验；完整战斗结算仍待现场验收。

原生证据SHA（本次整合读取，完整表/模块与源码SHA保留四报告，发布以后以最终manifest为准）：

| 原件/反汇编 | SHA256 |
|---|---|
| out/client_catalogs/tables/free_stage_nodes.json | f8dba8f8c245eeaecdf2d059dd0ed39d1683de9296a339fe6f88abd4eb088159 |
| out/dis/remaining-chapter-condition3.asm | 391b552a0786642ae85aad353cd16299bdd7c0ddeaebafac31357832afc21b59 |
| out/dis/remaining-consign-state.asm | 88e20d813f43d9cef727b269a19aa154d179d087a9cfd11fd6537cd792904961 |
| out/dis/remaining-task-tower-mgr.asm | add02fd6ce255daf26b7e811c5c79723c0ca3e8651231ed0a296de57fa2ccb04 |
| out/dis/compose-check-native.asm | 2c494356adad039f20d11804e913a60ec7e865620ff047df9e2cae28c1d05458 |
| out/dis/activity-FF2A17AE.asm | 88f6f2e8d789258b9fc80d52ba78718290687444671517116542b6da50adeaf6 |
| out/dis/remaining-guide-acc.asm | 5bd68e0a6fb5eb1f55a24e37d6155ede56099eb124ec21e1a3c1aea7e345399b |
| out/dis/remaining-collection-mood-formulas.asm | 6e05ed0a12239c3ed2fe9aa19183cd66bfdda20452537323cd885c73201ccc5b |
| out/dis/remaining-league-full-chain.asm | 5661b76b40f512da0e6001c23a3b68363fae8b2bf73af9c22f443da09d8a03e5 |

登录初始化遗漏已由实际链修复：正式策略ensureActivityLogin同角色事务调用ensureRemainingGameplay，quick_login及在线日界共同持久委托初批；默认关闭不自动启用新运营状态。remainingFixture已移除手动ensure并走真实hotfix目录/quick_login/BecomePlayer，首次6候选、5000点、生成等级60、冷存与同日重复登录不重抽/默认关闭回归PASS14.234秒。第二轮PG原nil panic不能称仅测试问题，最终远端56项已完整通过，0跳过。

### 17.11 最终统一验证、发布与接续

本轮实际关闭十五组服务器实现或审计纠正：31条成就已接真实业务，12个原有特别演练已按明确授权恢复。G06是本版无入口的审计纠正，不新增虚构配方；学会范围为此次六条成就所需会员与雅努斯基础。

默认及正式规则全仓、vet、最终构建和Linux真实原生全部通过；真实PostgreSQL 56 PASS、0 SKIP，另3项辅助通过；游戏/两端热更/私有原生运行时已统一发布并独立核验。Android完整玩法与持续负载仍待验收。

当前游戏SHA256 `ac5710950710fc0852a50f8f16d5cec423da087944fb35c6adfddcbe40fdab3f`；双端热更SHA256 `c9bc1595b4fed5e51f90f0b5b8862f896c3670ead9b097ff7848b2b55371bf38`，runtime `2026100810`；正式规则SHA256 `f5c21e417a165f9a7861689701f2cb1d8f369a225a7ab352a78c072219bb1d1c`。实际进程游戏PID `732614`、独立热更PID `68154`。私有原生运行时 `/opt/hs-server/native-pvp/releases/20261008-152449-813922`，原链接 `releases/20261008-120919-961724`。

组合发布报告 `.\out\completion-release-20261008-152612-999088.json`；游戏回退目录 `/opt/hs-server/releases/completion-20261008-152612-999088`；热更回退文件 `/opt/hs-hotfix/releases/completion-20261008-152612-999088/hotfix.json`。逐文件原存在状态与SHA以组合报告为准，只恢复所属项目文件/单元；若回退私有运行时，先核当前链接仍属于本批，再恢复原相对链接并重启所属游戏服务。

完整证据见 `out/remaining-completion-verification.json`、`out/remaining-local-verification.json`、`out/native-solo-linux-verification.json`、`out/remote-database-verification.json`、`out/native-solo-release-verification.json`。首次旧初音空映射与第二次委托登录遗漏的失败证据保存于 `out/backups/remaining-pg-failure-*`；修复后完整重跑。克苏鲁按实际回调名/原生整数核验，保留SAN成本、冷恢复、重复及整盒断言；邮件与演练实库用有效昵称进入业务，不放宽生产校验。

本轮构建缓存及临时目录为项目自有 `.\_buildcache`、`.\_buildtmp`：执行Go检查/构建前仅在本次命令环境设置GOCACHE/GOTMPDIR，不改全局环境。私有包已同步活动统计revision 3，归档SHA `a7c3d3fd41b1d96fabd75478f9ecba8d22b1a740903d397c8e747dfcbe31f5aa`；3098个实际Python模块及一项重定向条目维持原资源输入SHA `bed95c22e6e6adf4ccf01cabb5156b4888ccc5744f0e192d24501e6df3c19ef3`。14:56重登恢复的服务端永久入口已随本次游戏二进制上线；既有MuMu热更/登录/教学恢复证据保留，最终胜利结算及十五组Android操作继续单独验收。
## 外部资源核验入口（2026-10-09）

用户提供 `.\com.netease.hsqsl` 供检查后接入热更。21个NPK共143,485项全量解压无错误，通用/中/日音库与现行资源服一致，已在线分发。本轮发现旧内网脚本、res缺项、场景共有项变化和散装缓存清单不一致，未整体发布或改动生产。详细资源矩阵、只读内部工具、逐文件证据和后续版本/客户端验收约束合并到 [热更协议与资源核验](out/hotfix-protocol.md)，避免再新增MD。现行游戏业务和运行期0824热修发布状态仍以原有实际发布报告为准。

自动化03:05补记：第三次在线采集长时间未返回，已发送中断，未获得新线上日志。
