# 幻书启示录服务端技术说明

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
