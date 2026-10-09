# 幻书启示录进度与验证证据

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

后续短窗口followup-122435（12:22起至12:24:35采集）按PID18512统计9次绑定、9次登录刷新，后台周期结奖/租约/房间读取及收尾失败记录0条；本窗口仍有其他业务拒绝与未实现接口。证据登录刷新统计.json与events.log，不据此声称长期负载或所有客户端正常。

## 已完成副本重登卡大厅修补（2026-10-09 11:50）

0910阶段runtime2026100910、游戏PID16268；已修复已完成普通副本重登只补旧结算、不进入大厅的错误，实际日志21次连接/9名玩家命中。完整Go/vet/构建和真实PG63项0跳过通过，独立线上核验通过。未完成战斗和原收据保留。后台重试/观察阻塞同批修补，其他日志错误仍在继续处理；详细接口、证据和边界见SERVER.md同名节。 游戏SHA `fc2e9c860118e6952cd6217e6bf0387b3f22ff8f28b049af19667f8a1bc7f8a3`，热更SHA `65761334476b178d0efb192efe36f6045815b83dcb933d5c351de2dab5783205`。

## 升格循环、归还幻书与抽卡修补（2026-10-09）

0909阶段发布为 runtime2026100909、游戏PID13865。游戏SHA `32ddd785507025161d88b911b29ab46ebee670834d2d76a92ae6bae5b65c035a`，热更SHA `1807647165b18d0f447d79f0db2b96ff8d0079a64230b26eac7c6a05b8331eb2`；组合发布报告 `out/completion-release-20261009-105458-810824.json`，已完成备份、原子切换及独立远端/原生运行时核验。完整Go测试、vet、正式构建通过；真实PostgreSQL 63项通过、0跳过，另3项辅助通过。原生Python2实际RPC回调类型专项通过。

三项业务已实现并发布；详细原包依据、接口、缓存与回调规则见SERVER顶部。升格4102长事件流、归还资产与报告卡池已通过真实PG验证；Android第三波及持续玩家窗口仍以实际日志复核，不将模拟波次当实机验收。

## 玩家分发故障修补进展（2026-10-09）

09:00访问恢复后直接采集全服08:45至09:00客户端及服务端日志，222角色/17839卡存档结构审计未发现等级、品阶或UUID越界；这不证明UI数值正常。09:00时线上为0903游戏SHA `f7671857f95841c2822cea626d8ca0ee0fdece7d42d9977e117c61df20a5983c`、PID616。原有授权继续有效，工具直接使用已保存连接参数，不需要用户重新授权或提供逐个UID。

本轮新增抽卡/合成原生回调转型、连续失败暖恢复清理和按进度版本读取的在线快照，并纳入0904原生gui_utils导入修正。具体源模块、接口与测试见SERVER顶部。0901至0903为修补沿革，02:28旧验证与03:00至07:00访问阻断不作为当前状态；历史报告保留。未取得新版本客户端复测前，角色UI/数值、教学、回归活动及HTTP相关门继续开放。

## 新角色全英雄测试开关（2026-10-09）

新增仅建角生效的 `HS_NEW_AVATAR_ALL_HEROES`，测试配置为1；范围为本版82位可用图鉴英雄，每位一张1级，保留原初始卡。角色与初始资产同事务，不改旧存档、教学或系统解锁；关闭不回收。内部接口、配置方式、具体测试及证据边界已整合SERVER顶部专节。本地专项、全仓测试、vet、正式Windows/Linux构建通过；真实数据库60项通过、0跳过，另3辅助通过。00:49本批组合发布与两项独立核验通过，进程772014实际加载开启配置，启动日志无错误。报告及SHA见SERVER顶部；Android新建角色界面待用户操作验收，后续结合玩家反馈与服务器日志修补。

## 全功能复核现行进展（2026-10-08 21:24）

22:45实机：原账号8号补给101→151、收据1、已领再点不增；18号礼盒300致知之华+灵感激发1真实落库，收据1。第二新进程18595冷登录余额151/300/1保留，活动页两项仍“已领取”；原存档未清、未人工加资产。证据与精确验收边界统一SERVER基础领奖节。本批实际只关闭两补给到账/领取态/重登子门，其他完整UI/跨日和教学保留。

22:31基础奖励现行生产：组合发布及独立核验通过，游戏SHA`e3427c2009d871dce2e430eb1cdd6816b8565a9a3eae7135645a9462dc5b51a9`、PID768580，报告`completion-release-20261008-222656-234227.json`。两端热更不变0824。正式全仓test/vet/构建和真实PG59项0跳过+3辅助通过。以下候选/执行中为沿革；现行实机领奖取证继续，不以服务器通过关闭完整Android门。

22:14奖励/日常专项：实际日志确认`receive_power_supply`未实现；8入口与日周收据、原生异型回调、积分type11、完成/领奖状态、签到真实登录次数已补源码。专项测试通过，新增真实PG四连接并发领取/冷登录用例待构建执行；正式全仓/构建/实库/发布需绑定最终源码。当前生产仍0824；不称所有领奖、活动替代任务、完整教学已验收。

22:26最终重验：基础奖励全仓test/vet及正式构建通过，真实PG59项0跳过+3辅助通过，目录`db-20261008-222350`；首轮两项旧回滚失败修复后重验通过。游戏候选SHA`e3427c2009d871dce2e430eb1cdd6816b8565a9a3eae7135645a9462dc5b51a9`组合发布进行中。原存档只读确认等级3、通关501/502/503；完整教学和本批Android领奖仍未完成。

21:33现行0824组合发布及独立核验通过，报告`completion-release-20261008-212512-520899.json`，游戏PID765056；正式构建/全仓test/vet/真实PG58项0跳过+3辅助通过。下面0824候选状态为发布前沿革，当前进入保留存档冷登录和后续实机测试，未关闭完整教学或其他功能验收门。

生产0823组合发布及独立核验通过，真实PG58项0跳过+3辅助。0824候选补初音成就下行reward名称，正式构建、全仓test/vet及真实PG58项0跳过+3辅助通过，目录`/opt/hs-server/data/verification/db-20261008-212156`；游戏SHA`a5878a43bbdd75b84d978e8f32e1575fdaa2d4839884a7c676c256d340589294`、热更SHA`514a61a2cd27eb70b12f9d8a446e4a5ef8d8f70857ed258a58120c3f682ec35a`，组合发布进行中。0820实机20:47大厅完整、20:49地图1-2/调查入口可见，原90%及横幅异常未再出现；后续剧情/错误提示/学会/初音及完整教学/其他功能仍须实机，矩阵集中SERVER。

20:40修订：0820全仓test/vet及正式构建通过；真实PG57项/0跳过及3辅助通过，独立目录`/opt/hs-server/data/verification/db-20261008-203718`，游戏候选SHA`ed86e6e055567a5adf9e31d6724fc66c0069f447671cf382160c18ea2ee366c4`、热更SHA`4316c751286d36e38d290a5aba8fc598d3adadbbaf74111f4b1d497b34695b72`，组合发布进行中。全功能静态上行479个/237个未见同名Go字符串是待核对候选，不能当作全部已证实缺失或通过。0818首次实库影响旧探索冷恢复后已收窄教学编号并通过，失败目录`/opt/hs-server/data/verification/db-20261008-191031`保留。

> 2026-10-08 15:57 runtime2026100811与永久登录恢复入口已组合发布并实机通过。生产游戏SHA256 `9d08d8e7dc6a72ce7de06d3232263f685aecb90b4ba29d3788448e1436dcd24f`，热更SHA256 `c8aea79abac96af14ded564ecffaeef87f133a746236f64a3df4d57118656ac0`；全仓测试、vet、最终构建、Linux原生核验通过，真实PostgreSQL 56 PASS、0 SKIP，另3项辅助通过。MuMu日志确认 `start_server_battle_ok`、四参prepare、`HS_LOGIN_BATTLE_RECOVERY_SERVER_STARTED` 与 `on_refresh_login` 顺序正确，10001可见。随后 `guide_normal_attack`、`guide_skill`、`guide_skill_detail` 三段均产生真实 `use_skill` 与桥回放；原生 `guide.json` 只将这三段绑定10001，第四项绑定10002。15:54后的 `local control wait for master input` 不是缺引导；15:57实际自由选择技能和目标后进入 `story/00overture/battle/10001_4`。截图见 `out/evidence/tutorial-relogin-recovery-20261008/runtime11-free-input-action1-result.png`。10001最终胜利、结果落库及下一关仍保留为实机验收门。

> 2026-10-08T15:31:12.103772 十五组服务器统一交付已完成。默认及正式规则全仓、vet、最终构建和Linux真实原生全部通过；真实PostgreSQL 56 PASS、0 SKIP，另3项辅助通过；游戏/两端热更/私有原生运行时已统一发布并独立核验。Android完整玩法与持续负载仍待验收。当前游戏SHA `ac5710950710fc0852a50f8f16d5cec423da087944fb35c6adfddcbe40fdab3f`，热更SHA `c9bc1595b4fed5e51f90f0b5b8862f896c3670ead9b097ff7848b2b55371bf38`。

> 2026-10-08 14:56 重登黑屏已完成实机修复：客户端日志证明runtime2026100807正常，服务端同一时刻拒绝下一条 `enter_dungeon` 为“已有其他战斗会话”；生产只读快照确认匿名玩家仍保存10001未结会话。新增登录恢复扩展在 `on_refresh_login` 前请求 `client_need_recover_battle`，待battle建立后再执行原生登录收尾，避免下一关抢跑且正确移除黑色遮罩；永久服务端入口同步在 `set_reconnect_auth_msg` 检测未结战斗并优先复用恢复事务。runtime2026100810已双端发布，SHA256为 `c9bc1595b4fed5e51f90f0b5b8862f896c3670ead9b097ff7848b2b55371bf38`。MuMu确认 `HS_LOGIN_BATTLE_RECOVERY_REQUESTED/STARTED`、新UUID、四参prepare、教学实体加载及可见10001对话战斗画面；本轮未替换生产游戏二进制，最终胜利结算与引导继续仍以本次人工完成后的日志为准。

> 2026-10-08 14:39 教程结算第三层已定位并发布修复：战斗胜利日志已有 `REVIVAL_BATTLE_END_NOTICE` 与 `on_dungeon_finish dungeon_id:10001 win:True`，随后活动统计因原生已清空 `bid` 而在 `get_total_extra_statistics()` 抛出 `KeyError: None`。revision 3在 `battle_end_notice` 预先冻结统计，结果阶段只读快照；离线生命周期夹具覆盖“通知后bid清空”，runtime2026100807双端SHA为 `839b86d8c930adf95a97d22785fac6a10f17f9e25e1d7ed61671b3f4b324744a`。客户端已显示 `hotfix_suc 2026100807` 与 `HS_ACTIVITY_METRICS_READY 3`，残留异常场景已通过不清数据重启退出；恢复战斗后的最终结算与引导推进仍待本次人工重播确认。
>
> 2026-10-08 14:35 教程黑屏第二层已关闭：Android日志证明旧生产二进制把 `prepare` 四参错误下发为单个嵌套列表并触发 `TypeError`。共享源码五处调用已修正并增加精确参数形状测试；runtime2026100806增加旧格式展开兼容，独立热更服与游戏服清单均已发布为SHA `4086052ec1a7866ef32c9cf7bdf7fe7a187077f6351e0e3ca715645924370c22`。MuMu重启后确认 `account:on_hotfix_when_login 2026100806`、`hotfix_suc`、四参数 `prepare`、16/16预载、`battle_fighting`、`on_battle_start`，画面已进入普通攻击教程。证据截图在 `out/evidence/tutorial-prepare-20261008/tutorial-battle-after-fix.png`；本轮只发热更清单，没有覆盖另一个会话的服务端二进制。
>
> 2026-10-08 教程实机定位：角色“理渐离”运行期热更2026100805成功，跳过开场动画后客户端于14:14:20请求 `enter_dungeon`，服务端因两条早期无 `avatar_progress` 的遗留角色行返回“周期结算角色缺少进度”，造成黑屏。生产数据库已在事务内给匿名玩家、2补标准一级初始进度，备份表 `avatar_progress_repair_20261008_142200`，匿名玩家未改；源码同时在全服结算读取层隔离无进度遗留行并增加真实PostgreSQL回归测试，但需等待另一个会话A–F总发布后才上线。共享源码的隔离实库套件已通过40项后，被旧初音reconcileMikuAchievements空映射panic阻断，因此本轮没有覆盖发布半成品二进制。

> 2026-10-08 14:10 runtime重播闭包修复已双端发布：首次登录角色成功，但 `account:on_hotfix_when_login 2026100804` 暴露延迟回调 `NameError`。现保留生产正文不变，仅用 `_hs_runtime_install()` 闭包包装并递增到2026100805；独立热更与游戏服清单SHA均为 `0d64d0a782da2d9ab15fd270be2fec744fd67ae45ddaa6e9166932af0c3001d4`。发布后冷启动到1.0.128登录页无启动异常；下一次人工登录的runtime重播实机仍需现场确认。
> 本次修复前的固定发布报告仍以热更清单SHA `f6269ff6ec23c8fd3fa753f767cac7b8e75d9e01d0e8ad8171aa83ba63cd431c` 为审计基线；该值仅用于对应 `out/hotfix-bridge-release.json`，不代表当前线上SHA。

> 2026-10-08 MuMu全新数据热更实机通过：用户明确要求的一次 `pm clear com.netease.hsqsl` 后，客户端从1.0.125真实下载10文件/1,789,076,496字节，375秒后 `update_status=1`、`init.on_patch_finish 1.0.128`，正常进入登录界面。同时修复启动热修全局 `_hs_import` 失效与资源服 `script.npk`/`scenewd2.npk` 漂移；完成后无 `PatchHotfixTraceback`、`NameError`、HTTP 416或“下载文件出错”。

> 2026-10-08十五组同步实现：G01/G02/G03/G04/G05/G07–G15已补源码，G06纠正为本版无入口的历史规则，不新增虚构配方。本轮31条成就都有真实业务链；学会仅此次6条成就及雅努斯基础，不代表完整学会全功能。默认及正式规则全仓、实库、最终构建和组合发布默认及正式规则全仓、vet、最终构建和Linux真实原生全部通过；真实PostgreSQL 56 PASS、0 SKIP，另3项辅助通过；游戏/两端热更/私有原生运行时已统一发布并独立核验。Android完整玩法与持续负载仍待验收。Android完整玩法、双端与持续负载仍未完成。

> 历史单端PVP发布批次（非本轮十五组）：2026-10-08同步机器人、异步真人防守/机器人已接服务端单原生权威并发布；正式PostgreSQL 44项通过、0跳过及2项辅助通过。同步真人沿用既有单权威；普通副本保持Android原生计算。独立远端核验正常。Android双端/完整玩法和持续负载仍未关闭，整份A–F尚未全部完工。

更新时间：2026-10-08。当前源码、本批已发布生产、真实数据库和Android验收分开记录；接手以[HANDOFF](HANDOFF.md)和[全量余项](REPAIR-TASKS.md)为准，接口已整合[SERVER](../SERVER.md)。接续会话已继续施工，此前单端PVP业务已发布，本轮十五组服务器实现及统一发布已完成，正式构建/真实PG以接续当次报告为准。

## 当前证据边界

此前源码全部战斗采用客户端原生计算，rev12/握手1；新建同步真人/机器人及异步PVP均为服务端单原生权威，普通副本路线保留；共同真人命令与真实统计为后置扩展，原桥正文未改。旧Go权威引擎不是生产路线。用户确认145活动常驻且保留奖励/门槛；家具八组、契印17组等概率运营规则已获明确批准并生成正式文件；五商品永久开放已接。F2已完成。新账号自然引导/大厅、全玩法MuMu、双端确定性与持续负载仍待验收。

| 层级 | 本轮结果 | 证据与限制 |
|---|---|---|
| 本轮十五组当前源码全仓 | 默认及正式规则全仓、vet、最终构建和Linux真实原生全部通过；真实PostgreSQL 56 PASS、0 SKIP，另3项辅助通过；游戏/两端热更/私有原生运行时已统一发布并独立核验。Android完整玩法与持续负载仍待验收 | 本机默认不启用PG/Python2；明确原生专项另记，SKIP不计入实库或原生PASS |
| 活动专项 | 00:14 TestActivity 7 PASS，6.353秒；夏活/初音真实Handle链和宿舍日界/回滚、北风池/SAN/常驻wire | `internal/game/activity_business_test.go`及最终`out/completion-all-test.log`；定向结果来源于本轮实际命令输出，未另存独立活动日志；夹具账号，不模拟实际Android数值 |
| 社交/PVP专项 | 最新21项定向PASS；桥共享控制/幂等/恢复mock PASS | `out/social-local-verification.log`、`out/tools/verify_human_bridge.py`；两端Android未验 |
| 体验专项 | TestCollection/Shop/Intimacy/Native等专项PASS；对应数据库用例已纳入当次39项PG PASS | 具体接口/原生SHA见SERVER和源码，实际用例名见database-test.log；Android完整UI仍待验收 |
| 历史0811热更证据（非当前生产） | runtime2026100811；兼容旧服务端嵌套 `prepare`、活动统计revision 3及服务端已恢复战斗的登录收尾 | 双端SHA `c8aea79abac96af14ded564ecffaeef87f133a746236f64a3df4d57118656ac0`；当时验收止于10001_4，当前501已结算及后续验收门见本文顶部 |
| 管理页浏览器 | 本机独立夹具端口19901检查中文页面、401拒绝及推荐表单；临时预览已停止 | 未做认证UI全部写操作/真实生产管理；API事务专项已有。令牌临时文件已清理，不暴露公网 |
| 历史单端PVP已发布批次PG（非十五组） | 44 PASS/0 SKIP、2辅助PASS；SHA `7083d898b18fbbabbbec0a9d1a688a79e024732cf6911394b2ff793c556b2515` | out/remote-database-verification.json；新增单端真实PG原生/冷恢复/防守事务，不证明Android |
| 十五组发布前历史生产核验 | 游戏SHA `9621cde391fff0a0238b417f4668c606dd79a9b91ce06c969f4ec795104e881a`；PID721448，私有运行时、环境、监听和日志通过 | out/native-solo-release-verification.json；共享服务未改 |
| 历史0807独立热更发布（非当前生产） | SHA `839b86d8c930adf95a97d22785fac6a10f17f9e25e1d7ed61671b3f4b324744a`，runtime2026100807 | 独立热更备份 `hotfix.json.bak-20261008-143813`；游戏服清单备份 `hotfix-20261008-143826.json`；当前发布及回退以本文顶部组合报告为准 |
| F3 | 10-07 23:15:53–23:16:00真实续期PASS、24SAN、443恢复 | `out/certificate-real-renewal.json`和.log；新证书有效至2027-01-05 22:17:26北京时间，game/共享进程未改 |

09:23正式Windows/Linux游戏与Linux dbstore.test已构建并09:32发布；产物SHA及正式配置绑定见completion-build.json/组合发布报告。02:41因当时17池未批准而未写生产只是历史，不能继续作为当前状态。PVP基础新增后重新冻结与构建，是否沿用正式PG必须核对测试二进制、热更及env三SHA。

## 本轮源码实际完成范围

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

全部原生接口、内部状态、成本/奖励/持久收据/冻结与事务边界已合并[SERVER第17节](../SERVER.md#17-十五组同步实现原生契约与本服规则2026-10-08)。G06为审计纠正而非新增配方；学会范围仅本轮六条与雅努斯基础。

| 分层 | 当前证据 | 边界 |
|---|---|---|
| numeric定向 | PASS14.283秒；实际9完整探索章、30绝密任务、100委托；原生参数/推送与失败回滚 | remaining-numeric-report.json；不是Android |
| activity定向 | Go12项及24个真实Python2原生场景通过 | remaining-activity-report.json；不替代生产与Android |
| collection定向 | 点赞双角色与旧行为专项、正式能量7项通过 | remaining-collection-report.json；两个新PG本地仅编译SKIP |
| league定向 | 5项真实成员/30窗口/恢复/六成就通过 | remaining-league-report.json；实库与Android另层 |
| 默认/正式全仓、vet、Linux原生、最终构建 | 默认及正式规则全仓、vet、最终构建和Linux真实原生全部通过；真实PostgreSQL 56 PASS、0 SKIP，另3项辅助通过；游戏/两端热更/私有原生运行时已统一发布并独立核验。Android完整玩法与持续负载仍待验收 | 已绑定最终同一源码，修复前快照保留历史证据 |
| 56项真实PG及组合发布 | 默认及正式规则全仓、vet、最终构建和Linux真实原生全部通过；真实PostgreSQL 56 PASS、0 SKIP，另3项辅助通过；游戏/两端热更/私有原生运行时已统一发布并独立核验。Android完整玩法与持续负载仍待验收 | 首次40PASS后初音panic保留失败记录；修复后完整56 PASS |
| Android/双端/持续负载 | 未完成 | 14:01登录页是热更启动子门，非十五组业务验收 |

最终统一报告out/remaining-completion-verification.json已记录实际时间、SHA、退出码、PASS/SKIP、备份与回退。

## 已知失败与修复范围

10-08 00:10全仓曾失败：`TestLayoutInvalidOrStartedChangesRollback`将首次全量PVP周期基线事务误归非法布阵改动；已先刷新基线再取before，仍保留每次整Progress深比较。旧权威`TestGuideBattleFullLoopToVictory`随机战斗死亡随后100次复现13失败，固定研究夹具123456后100次通过，未改生产原生战斗。原失败及复现日志保留在`out/completion-known-failures.json`索引，不称所有随机结果已修。

旧SSH隧道并发登录超时与旧测试初始化Catalog缺失的失败保存在 `out/backups`；后续远端回环基线通过，不将旧失败隐去，也不把今天本地PASS替代真实PG。当前未执行CGO/race，不能记竞态检测通过。

历史02:40的5项社交PG、真人租约/投递/录像3项、恢复/C3/技能3项、活动排名/邮件/入住3项及已有体验/管理/成长用例纳入当时39项实际PASS；原报告/日志按日期在备份中追溯。历史单端PVP快照为44项真实PG；当前database-test.log由主代理新55项核验更新，原专项本机SKIP记录仍保留，不能把旧报告改成当时通过。

## 历史基线及可回退位置

| 时间 | 已验事实 | 历史证据边界 |
|---|---|---|
| 10-05 | 老账号置完成后重登大厅、旧权威教学攻击/波次 | `handoff-client-full.log`、`handoff-server-full.log`、AUDIT；不代表新账号自然引导 |
| 10-06 | 用户确认协议闸门、真实整包热更10文件/1.79GB/249秒、运行1.0.128 | `out/hotfix-e2e-2026-10-06.md`、`out/e2e-20261006`；F2不重做 |
| 10-07 04:20/05:00/10:45 | 基础契印/队长/布阵/签到、阵容预设/契印预设历史发布 | 历史hash与backup在旧报告/备份；均已被21:46基线替代，不作为当前线上 |
| 10-07 20:43 | PVP/送礼/邮件/抽卡/合成/成长/头像基线发布 | 旧hash4f234f41…，后来21:46替代 |
| 10-07 21:46 | 邮件/商城/成就最新基线发布及19项PG | 当前game hash/备份见上，PG目录 `/opt/hs-server/data/verification/db-20261007-214118`；测试二进制SHA `da74851e2cca67000e6e065bdc0521078df18fb4ab786c78bd6176d4225ab841` |

F3脚本SHA `fe9f158bf30650a1f353c623c9d7dbb7048d22eda4d11f1fc0b04cd1a41374f1`；项目timer每日北京时间03:30后随机延迟最多30分钟，即03:30—04:00，≤30天续；verify-renewal一次90天阈值实测。脚本安装备份 `/opt/hs-server/tls/renew-install-Kamp1J5F`，真实旧cert/key备份 `/opt/hs-server/tls/backup-renew-20261007-151553`，失败自动复原并journal/logger；未配置外部告警平台。

## 文档、记忆与继续施工

用户授权远程连接记忆已保存，报告 `out/authorized-memory-update.json`，记忆增量 `<本机配置路径>`；地址/端口/用户名/目录/单元/数据库/本机凭据来源已记录，未复制口令。旧CET包装器被用户撤回，所有命令显式系统CMD，不恢复故障包装器。

四份原MD整合，MD总数18，无新增资料负担；UTF-8/链接/46编号校验见handoff-doc-check.json，当前源码清单见completion-source-manifest.json。旧一次性重写器已归档，原生证据/失败/备份保留；Go缓存清理使C盘99.1 MiB→9.10 GiB，未扩删其他资源。

## 02:40批次历史进展（当时未发布，保留失败与原始证据）

开工18份MD全部读取，401项completion-source-manifest哈希差异0。00:37游戏audit及01:00独立热更audit均确认旧生产哈希未变、所属进程正常，未改共享AscNet/Mongo/nginx/其他数据库。新增源码备份out/backups/completion-20261008-003718及各代理.bak。

普通结算恢复13项、夏活/初音恢复2项通过；C3新21顶层/23含子用例通过；真人两Service持久投递/128屏障及完整录像静态样本已测。各专项当时真实PG本机SKIP记录保留，对应新增用例随后已在当次远端39项PG中执行通过。旧研究夹具随机100次13失败日志continuation-guide-reproduce-20261008-003810.log保留；固定123456后100次PASS见continuation-guide-fixed-20261008-004845.log。typed容器真实msgpack回读PASS见continuation-wire-native-20261008-004951.log。

中途全仓因跨代理helper签名、录像索引字段、组合热更索引差异出现失败，continuation-all-midpoint-20261008-005608.log及continuation-integration-20261008-005815.log保留；修复后最终全仓/正式build/PG另记，不能用专项覆盖这些失败。双主机组合发布故障注入4项PASS，工具尚未实际发布，policy未获答复不得自行加载等概率。

真实PG曾分别2项、27项、37项PASS后失败，原报告/日志保存在out/backups/pg-first-failure-20261008、pg-failure-20261008-021419、pg-failure-20261008-022747。另一次根将构建与上传并行导致产物SHA拒绝，未执行测试，证据continuation-pg-artifact-race-failure.json保留；最终严格等待检查→构建完成→上传/实库退出。实际业务修复为社交三条SQL回载账号/性别/取名/创建时间遗漏，以及评论JSON空Likes、身份不可改和影响0行整事务回滚；测试夹具遵守quick_login→BecomePlayer→真实guide_task_finished，不跳过引导。Tick允许且核对已提交属性推送，严格拒绝旧权威调度/自动输入，完整战斗和资源保持不变。对应报告pg-social-comment-metadata-report.json、pg-social-metadata-guide-fix-report.json及pg-native-tick-compile-20261008-023156.json；最终39数据库PASS、2辅助PASS、0SKIP，不能把辅助PASS算进数据库数量。

发布工具最终17项本地故障模型通过，日志continuation-release-tools-final-20261008-022124.log；包含产物/绑定热更漂移、认证不重试、SFTP关闭错误、锁/逐文件双机回滚边界。completion-release-safety-report.json为此前工具快照，当前工具哈希以最终源清单为准。真实双主机故障回滚未执行；02:41只读预检记录completion-release-20261008-024136-844968.json，17零池错误、主机列表为空。

接续专项冻结快照：out/activity-rules-final-report.json、out/activity-all-local-test.log，16项PASS/退出0；out/human-record-local-verification.json，17项PASS/退出0及两桥模型通过。这些快照里的PG本机SKIP是当时事实；最终远端39项已包含活动排名/周期/入住、human/录像、恢复/C3/技能及社交元数据用例。模板/概率缺服务端来源和当前原型假设显式保留，不因源码与实库通过称活动全量完成。

02:40历史检查：全仓test/vet、正式build及真实PG 39 PASS/0 SKIP通过；当时17池尚未批准，02:41预检没有写生产。09时新批次规则已明确批准，全仓/构建重新完成，真实PG为41 PASS/0 SKIP，组合发布已通过。Android/双端确定性/完整玩法/持续负载尚未验收；当时C3映射223已有/31未接，现本轮31条已补真实链。统一证据out/continuation-final-verification.json，不能写全部完成。 当次总证据out/continuation-final-verification.json；新版Linux游戏SHA `035d1a7d8ad00de52946110159388b3d833c9520c7dd91e2a9cf81a46669a2da`，尚未部署。

新增品阶/邮件兼容、成就初始目录与旧首事件原子性修复；失败日志continuation-all-final-20261008-013442.log、continuation-all-acceptance-20261008-015159.log及grade-mail首轮夹具失败保留。好感/洗练测试仍严格验证原资产/回调序列，额外成就状态/分数成对、与已提交进度相符且早于回调；没有删除资产回滚断言。CMD构建包装器错误输出保留completion-build-20261008-015517.log/.raw，改系统CMD路径后正式构建通过，未修改全局CET设置。

## 作者新版对齐接续验证

家具正式八池逐候选发放/三档选择PASS，out/continuation-approved-fusion-20261008-085958.log；主动退出两个实际RPC专项PASS，out/continuation-author-forfeit-20261008-090000.log；失败085904保留。原生资源3098模块转换成功，真实Python2 server_battle启动和双阵容确定性/无效点击零状态变化/timeout通过，out/native-pvp-engine-verification.json。Go进程适配器尚在专项检查，权威房间和客户端接线未完成。正式规则批准不再待用户回答；原39项PG不得覆盖此后修改。

## 历史09时正式运营规则批次证据

09:21全仓test/vet、09:23构建前后源码一致；Linux游戏SHA 8b3307a23e7912bc06a633c90a989d8f264338606632b186073bc64669167373，dbstore.test SHA 09346cd72b01894c335b23cabebefde1d2a917b3c25a3226be9d5c27826de986。09:25真实PG41 PASS/0 SKIP、2辅助PASS，目录/opt/hs-server/data/verification/db-20261008-092330，热更/正式env分别SHA 13bd23295f1392d8dded16c731944df56c29fa32183d88e6f5beeb6d8e998f48 / bf54024d044eb2a0e9ffc3d7224bc49021f40b0b381ba0af85c6de4f1b552d3c。首轮40PASS后的失败仅为新夹具昵称超长，尚未进入102结算，原报告保留在out/backups/pg-failure-20261008-092103；修为合法六字昵称后同正式17池完整首通/重试/冷恢复通过。原39项通过快照保存在out/backups/pg-failure-20261008-091452（目录名沿用旧工具，内容实际是通过报告）。18项发布故障模型通过，新增正式规则与实库SHA失配拒绝；09:27只读预检通过。

隔离Linux原生验证也已通过，out/native-pvp-linux-verification.json：Debian官方索引SHA校验三个包后只在项目验收目录解包，未安装全局包或重启服务；两次39步/282事件及非法输入不改状态/真实timeout通过。首轮PYTHONHOME污染Python3启动的失败保存out/backups/native-linux-first-failure-20261008-092012，改为仅Python2包装入口设置后通过。Go真实进程专项090801、真实冻结共同房间原生启动091217通过；091205只因JSON克隆将nil容器规范化后与原对象直接DeepEqual导致夹具失败，改为比较完整持久字节后091242通过，不放宽跨双方重复UUID拒绝。PVP仍未激活权威房间接线，不称全部对齐完成。

## 历史09:32组合发布及独立核验（现行见本批接续结果）

游戏SHA `8b3307a23e7912bc06a633c90a989d8f264338606632b186073bc64669167373`、独立热更清单SHA `13bd23295f1392d8dded16c731944df56c29fa32183d88e6f5beeb6d8e998f48`，正式规则SHA `bf54024d044eb2a0e9ffc3d7224bc49021f40b0b381ba0af85c6de4f1b552d3c`；发布目录 `/opt/hs-server/releases/completion-20261008-092750-779899`，报告out/completion-release-20261008-092750-779899.json。09:34独立检查game/sdk/login/postgres正常，PG回环15432、game9000；实际PID709648。SDK690363/login599422/PG595042/DNS592232及共享AscNet563565/Mongo1903没有改变；独立热更PID65495，其他共享服务未改。09:35读取所属进程，仅核验两个政策变量，确认17×18等权契印与8组等权家具实际加载，未打印其它进程环境或数据库口令。日志确认145活动日程、PostgreSQL及9000启动；当前发布不是Android新账号、完整玩法、双端PVP或持续负载验收。PVP单权威完整接线仍施工。


## 09:40后PVP单权威实现、真实库与10:56生产发布

Windows明确启用原生专项9 PASS/0 SKIP，见out/continuation-native-pvp-go-20261008-100645.log；包括真实双手动超时对局30窗口/60接收端命令/65日志、完整终态冷重建、未提交动作丢弃与重试、作者13映射案例对照、援护及播放末态屏障。桥合同夹具见out/continuation-authority-bridge-contract-20261008-100328.log，使用本版原生RpcMethod元数据，普通rev12、子类选择、异常工厂恢复和重复安装均通过；GUI/客户端场景是夹具，不是Android。Linux专项报告native-authority-linux-go-verification.json绑定独立测试二进制/桥磁盘字节SHA，游戏PID前后不变；实际项数以其当次日志为准。

原生工厂/注册元数据汇编新证据在out/dis/pvp-native-*.asm。一次探测误用inspect.payload而接口要求module，错误已修且源脚本未变；导出桥首次报告按内存LF算SHA与Windows实际CRLF不同，随后固定LF并以磁盘字节计算。新导出SHA在native-authority-bridge-export.json。详细接口已整合SERVER16.5。HumanPvpRoom已接Journal、双端实体/坐标映射、规范播放、恢复世代、超时推进和唯一原生胜方结算；真实PG修复并验证jsonb键重排后的链哈希。10:53全仓/vet/正式构建通过，10:55生产私有运行时实际自动对局通过，10:56组合发布，11:01独立核验游戏、两端热更、drop-in、进程环境和日志通过。报告为out/remote-database-verification.json、out/native-pvp-production-runtime.json、out/completion-release-20261008-105631-389903.json及out/native-pvp-release-verification.json。用户尚未操作MuMu 102或双端PVP；实机门保持未完成。

当前固定发布完整哈希：游戏 `9621cde391fff0a0238b417f4668c606dd79a9b91ce06c969f4ec795104e881a`，热更清单 `55d2be8356dca5d9cf5fca2b92b143179f65e3777fca825fe6ef97950aabb2bc`。同步真人、同步机器人与异步PVP新会话使用服务端单原生权威；普通副本保持Android原生计算。

## 单端PVP权威本批实际验证

本批游戏SHA `9621cde391fff0a0238b417f4668c606dd79a9b91ce06c969f4ec795104e881a`，热更SHA `55d2be8356dca5d9cf5fca2b92b143179f65e3777fca825fe6ef97950aabb2bc`（runtime2026100804正文沿用）；回滚备份 `/opt/hs-server/releases/completion-20261008-121214-742541`。正式PG 44 PASS/0 SKIP，产物SHA绑定最新构建；Linux单端/双端原生专项通过。 Windows三条单端对局与双端回归见out/native-solo-local.log；Linux真实原生/单双端专项见out/native-solo-linux-verification.json，生产游戏PID前后均715230。正式PG两项新增完整Handle对局通过。中途夹具null援护列表、首次超时只建立deadline而尚未推进、Python2新增中文未声明编码，以及异步宿主缺avatar_info的失败日志均保留out/native-solo-local-*.log；修复后重新验证，不将失败覆盖为PASS。单客户端场景及映射/播放确认使用显式夹具，原生计算是真实Python2，不是Android界面验收。

异步PG首次完整进度幂等断言曾失败：首次胜利的派生成就积分未在双角色事务内同步，重试才由通用投影补算（目标101101对应四个成就）。已将reconcileAchievementState移入首次双角色结算事务，保持完整进度/修订号断言，重新构建后44项PG通过；失败证据在out/backups/solo-pg-failure-*。周三日奖修正前已通过的44项/原生报告另保留out/backups/before-nian-daily-20261008-114612，本次发布使用修正后新产物和重新验证报告。

年兽反推已扩展至通用榜管理器/条目类型与全包sub_ranks引用；三榜名次只由服务端下发，未参赛0仅为显示哨兵，不能唯一恢复综合公式。已据原生明确文本修正周三不发日榜奖，非周三正常发放，实库覆盖重建服务后幂等；详见SERVER16.7及out/dis/nian-rank-*.asm。原服总榜公式反推当时未闭合，当前按批准本服政策已实现（SERVER17.7），不要求用户手动提供其没有的数据。

登录初始化遗漏已由实际链修复：正式策略ensureActivityLogin同角色事务调用ensureRemainingGameplay，quick_login及在线日界共同持久委托初批；默认关闭不自动启用新运营状态。remainingFixture已移除手动ensure并走真实hotfix目录/quick_login/BecomePlayer，首次6候选、5000点、生成等级60、冷存与同日重复登录不重抽/默认关闭回归PASS14.234秒。第二轮PG原nil panic不能称仅测试问题，最终远端56项已完整通过，0跳过。

## 十五组最终交付记录

本轮实际关闭十五组服务器实现或审计纠正：31条成就已接真实业务，12个原有特别演练已按明确授权恢复。G06是本版无入口的审计纠正，不新增虚构配方；学会范围为此次六条成就所需会员与雅努斯基础。

默认及正式规则全仓、vet、最终构建和Linux真实原生全部通过；真实PostgreSQL 56 PASS、0 SKIP，另3项辅助通过；游戏/两端热更/私有原生运行时已统一发布并独立核验。Android完整玩法与持续负载仍待验收。

当前游戏SHA256 `ac5710950710fc0852a50f8f16d5cec423da087944fb35c6adfddcbe40fdab3f`；双端热更SHA256 `c9bc1595b4fed5e51f90f0b5b8862f896c3670ead9b097ff7848b2b55371bf38`，runtime `2026100810`；正式规则SHA256 `f5c21e417a165f9a7861689701f2cb1d8f369a225a7ab352a78c072219bb1d1c`。实际进程游戏PID `732614`、独立热更PID `68154`。私有原生运行时 `/opt/hs-server/native-pvp/releases/20261008-152449-813922`，原链接 `releases/20261008-120919-961724`。

组合发布报告 `.\out\completion-release-20261008-152612-999088.json`；游戏回退目录 `/opt/hs-server/releases/completion-20261008-152612-999088`；热更回退文件 `/opt/hs-hotfix/releases/completion-20261008-152612-999088/hotfix.json`。逐文件原存在状态与SHA以组合报告为准，只恢复所属项目文件/单元；若回退私有运行时，先核当前链接仍属于本批，再恢复原相对链接并重启所属游戏服务。

完整证据见 `out/remaining-completion-verification.json`、`out/remaining-local-verification.json`、`out/native-solo-linux-verification.json`、`out/remote-database-verification.json`、`out/native-solo-release-verification.json`。首次旧初音空映射与第二次委托登录遗漏的失败证据保存于 `out/backups/remaining-pg-failure-*`；修复后完整重跑。克苏鲁按实际回调名/原生整数核验，保留SAN成本、冷恢复、重复及整盒断言；邮件与演练实库用有效昵称进入业务，不放宽生产校验。

本轮构建缓存及临时目录为项目自有 `.\_buildcache`、`.\_buildtmp`：执行Go检查/构建前仅在本次命令环境设置GOCACHE/GOTMPDIR，不改全局环境。私有包已同步活动统计revision 3，归档SHA `a7c3d3fd41b1d96fabd75478f9ecba8d22b1a740903d397c8e747dfcbe31f5aa`；3098个实际Python模块及一项重定向条目维持原资源输入SHA `bed95c22e6e6adf4ccf01cabb5156b4888ccc5744f0e192d24501e6df3c19ef3`。14:56重登恢复的服务端永久入口已随本次游戏二进制上线；既有MuMu热更/登录/教学恢复证据保留，最终胜利结算及十五组Android操作继续单独验收。
## 外部资源核验补记（2026-10-09）

`com.netease.hsqsl`的21个NPK共143,485项全量解压通过，10包与现行分发一致；三份通用/中文/日文音库已经在线。新增资源不能直接整体发布：来源脚本带7个非官方模块改动和旧内网更新地址，res缺2项，两个场景包有共有项变更，Documents散装清单679项不一致。仅新增只读核验工具、JSON证据及中文文档；生产清单/资源/程序/玩家存档未变，无MuMu操作。完整资料统一见 `hotfix-protocol.md` 的2026-10-09章节，证据在 `out/external-resource-audit-20261009/`。全量解压成功不等于所有资源齐全或Android验收。
