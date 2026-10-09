# 证据清单（Evidence Inventory）

> 2026-10-07 文档整理：本文件保留历史取证，日期相关的“当前/下一任务/未实现/未发布”仅对应原记录时点，不能当作现行状态。统一交接见 [HANDOFF.md](HANDOFF.md)，当前施工见 [REPAIR-TASKS.md](REPAIR-TASKS.md)。全部战斗现走客户端原生计算，F2已完成，不再执行旧权威引擎施工或协议闸门改包。


| # | 证据 | 来源 | 结论 | 置信度 |
|---|------|------|------|--------|
| E01 | 包名 com.netease.hsqsl、签名 H62_KEYS、目录 netease/h62 | APK META-INF + files 目录 | 游戏为幻书启世录，项目代号 h62 | 高（用户确认+文件证据） |
| E02 | log.txt: NeoXRoot / FileLoader npk / python.dll 模块 | files/netease/h62/log.txt | 引擎为网易 NeoX，业务逻辑 Python，资源 NXPK 打包 | 高 |
| E03 | log.txt: patch_hotfix_data_url=h62.update.netease.com/pl/patch_hotfix_data_pub + check network 循环 | log.txt 尾部 | 启动卡死点=热修检查，官方热更服务器不可达 | 高 |
| E04 | log.txt: app_base_version 1.0.8 / patch_version 1.0.128 / engine_version 211635 | log.txt | 客户端版本三元组 | 高 |
| E05 | lib/ 仅 armeabi-v7a 21 个 so；libclient.so 48MB | unzip -l | 32 位 only；NeoX 主引擎库 | 高 |
| E06 | minSdk16/targetSdk26/versionCode83，渠道 TapTap | AndroidManifest 解析 | 系统兼容信息 | 高 |
| E07 | Documents/script.npk 12 条，APK assets/script.npk 3100 条 | 原包头部及全量解包 | Documents 为热修增量，APK 含基础脚本包 | 高 |
| E08 | battle.txt 内容为 Python pickle (set_last_fighting_cards) | cat | 客户端用 pickle 序列化战斗数据（本地缓存） | 高 |
| E09 | neox.xml filesystem 配置：脚本加载顺序 %DOC_DIR%\script → asset script | neox.xml | 脚本来源双层：Documents 优先，回退 APK assets | 高 |
| E10 | 40 块字节统计 | out/tools/npk_analyze.py | 固定密钥流推断已撤回，恒定字节不能证明加密算法 | 作废 |
| E11 | PC 与 Android 同哈希样本字节一致 | 原包对比 | 只能证明该样本共享数据，不能证明共享密钥流 | 结论修正 |
| E12 | 内存残片对齐投票 | out/tools/npk_known_plain.py | 固定 K 和 marshal 明文推断已撤回 | 作废 |
| E13 | 早期自签/空热修实验，后续 Android 明确证书链校验 | 历史 log 与 gate 实机记录 | base64 JSON 格式确认；免证书校验结论撤回，成功环境用了 CA 注入 | 结论修正 |
| E14 | 网易系域名不劫持时 upload_patch_log 死等（applog/appdump） | log.txt 01:23 | sdkserver 必须提供日志兜底接口 | 高 |
| E15 | 完整 files 数据回灌模拟器后补丁检查直接跳过 | log.txt 01:30 起 | 本地状态文件即"已是最新"凭证 | 高 |
| E16 | 取反后字节统计 | 历史 npk_header_scan.py | 全取反加密、E68C 容器推断已撤回；实际原始块为标准 LZ4 | 作废 |
| E17 | PC exe x86 立即数 xref：uncompress 区 6 命中；0xbf6adc 11 路压缩类型表；类型存索引条目+0x18 | out/tools/exe_xref_scan.py、out/ghidra/exe_xref.asm | 解压调用链与压缩类型机制 | 高 |
| E18 | 解码器 RVA 0xf09310 / VA 0x1309310，签名 f(src,dst,源长,输出容量) | 修正后的 capstone 反汇编及 Unicorn | 标准 LZ4；旧地址/变体结论已修正 | 高 |
| E19 | 0x4176de2a 的 F1 token 块含构建路径 | 严格解包器与原函数运行 | 同样为标准 LZ4；“明文直存分支”已撤回 | 高 |
| E20 | 净网+files 回灌后游戏可进登录界面（CCLive 心跳、模块加载量增） | 02:00 后 log.txt | 复活可行性实测 | 高 |
| E21 | PE 映像基址 0x400000，旧扫描器把代码 RVA 当 VA | exe_xref_scan.py、npk_decode_probe.py | 代码地址必须加基址，数据立即数本已是 VA | 高（静态+模拟） |
| E22 | VA 0xbf69a3 以索引 +8 作源长、+12 作输出容量，比较返回值与 +12 | exe_xref.asm、uni_lz4.py | +8=stored_size，+12=decoded_size；旧 size/csize 语义反了 | 高（静态+模拟） |
| E23 | Android 3100+12、PC 3031 全部直接标准 LZ4 解压；12 个代表样本与原函数逐字节相同 | out/npk_decoded/verification.json、各 manifest.json | 外层解压完成，不执行取反 | 高（全量+原函数模拟） |
| E24 | PC 头部计数 3031，索引后有 26 个被引用的追加块 | PC 包索引及 verification.json | 禁止根据 EOF 倒推条目数 | 高 |
| E25 | 外层输出：Android 3098+12 块为 1973；PC 3003 块 1973、26 块 19e1；直接 Loader 只读基础包各 1 个 code | verify_npk.py、verification.json | 外层输出需要脚本层处理，不能仅凭解压就标为 marshal；内层现已由 E26/E27 恢复 | 高（外层观察） |
| E26 | redirect.load_module 引用 rotor/zlib/script_decrypt；C_file 三个绑定返回长密钥、233/99 与实际 Python 源码，Android so 中同字符串逐字节一致 | script_loader_bindings.json/.asm、redirect.json | 内层处理链有直接客户端代码证据，无需猜密钥 | 高（静态+全量恢复） |
| E27 | Android 基础 3099 code+1 清单，热修 12 code，PC 3030 code+1 清单，均解析到 EOF、无失败 | out/npk_scripts 三目录 manifest.json、npk_script_decode.py | 脚本 marshal 结构全量恢复，原包未修改 | 高（全量解析）；非运行验收 |
| E28 | redirect 的 LOAD_CONST 为 0x99，NeoX 使用定制 opcode | redirect.json、opcode-recovery.md | 主要映射随后已恢复；仍不能直接套标准表，少量低频项未确认 | 高（解释器与结构统计） |
| E29 | RPC 装饰器、client_info、登录时序及双通道热修指令流 | out/dis、rpc-semantics.md、login-flow-spec.md | 业务签名与语义可供 Go 实现；不证明原生字节封装或实际客户端已登录 | 高（静态指令） |
| E30 | role_server_item/update 字典索引与错误码直接常量赋值 | 315EAFEB、875C5BCF、37C6CA84、E9F4B35D；out/login-constants.txt | 五个角色 UI 字段、RET_SUCCESS=0、登录失败码和踢人类型数值 | 高（静态指令） |
| E31 | Go 单元检查、四进程编译产物的 HTTP/TLS/DNS/TCP 联调 | SERVER.md、out/server-smoke.json、out/tools/smoke_server.py | 本机框架业务和热更接口可用；非 Android 登录、压测或生产部署验收 | 高（本机运行） |
| E32 | 游戏服 SSH 只读端口/进程检查、systemd 状态和 500 并发种子握手 | 2026-10-05 远端命令输出、`/opt/hs-server` 三服务 | 8080/8081/9000 已在独立目录运行；AscNet/MongoDB 未修改；仅证明传输骨架并发，不证明 Android 业务登录 | 高（远端实测） |
| E33 | 早期 SSH 热更认证失败，后续恢复认证并部署 v6 | 上一会话交接与发布记录 | 仅为历史部署记录；当前重打包客户端使用 v21，见 E48 和 HANDOFF.md | 历史部署记录 |
| E34 | DNS 后缀映射单元测试覆盖 `login.r18sex.net` 与网易别名 | `internal/hotfix/hotfix_test.go`、`internal/hotfix/dns.go` | 游戏域名与热更别名可返回不同 IPv4；非项目域名仍拒绝递归 | 高（代码测试） |
| E35 | PostgreSQL 连接池非法配置快速失败测试 | `internal/database/postgres_test.go`、`internal/database/postgres.go` | 初始化和首次 Ping 有 5 秒上限；尚不证明真实数据库业务或压测 | 高（代码测试） |
| E36 | 早期公网管理检查与认证记录 | 历史记录 | 旧凭据阻塞已解除；当前服务状态以新验收为准，文档不保存凭据 | 历史记录 |
| E37 | 远端部署脚本语法检查和目标端口保护逻辑 | `deploy/remote_install.py`、`python -m py_compile` | 可重复部署 game/hotfix；凭据不落盘，非本项目端口占用时中止；尚未使用新凭据执行 | 高（代码检查） |

| E38 | 原生 handler 调用 PyDict_SetItem/PyNumber_Negative/PySet_New/PyObject_SetAttr | dis/store-opcode-evidence.asm、verify_store_opcodes.py | op8/67/99/132 纠错，支撑数据与时区恢复 | 高（原生静态） |
| E39 | 703 模块、来源 SHA256、Record 构造、表达式与原始字节校验 | client_catalogs/summary.json、verification.json | 全部数据模块静态结构覆盖，255,142 顶层条目；表达式不执行 | 高（静态及完整性） |
| E40 | 302 显式装饰、直接代理与命名转发调用点 | client_catalogs/rpc-catalog.json | 比前缀统计可靠，动态调用和方向仍待补证 | 高（模式内静态） |
| E41 | time_utils 取负、Python2 整分钟约束、Float 与空参检查 | dis/9AD0A456.asm、time-contract-verification.json、gate 测试 | 时区 -28800 得到 UTC+8；后续 E44 和本轮日志无同类时间异常，不等于长期时间行为验收 | 高（契约及后续实机日志） |
| E42 | 真实 PostgreSQL 测试 | database-test.log、dbstore/accounts_test.go | 密码哈希、属性保留、并发唯一、回滚和取消通过 | 高（真实数据库） |
| E43 | 公网登录与游戏服务重启后同 OID，错误密码 9002 | gate-remote-before-restart.json、gate-remote-after-restart.json、gate-remote-auth-rejection.json | DB 业务版本上线，协议模拟验证通过；非 Android 主城 | 高（远端模拟） |
| E44 | 10:09 MuMu TCP 直连、login_result=0、本地 Avatar 收尾、数据库首次注册 | mumu-direct/交接状态.json、交接时客户端日志.log、交接时关键日志.log | 该历史阶段证实原按钮鉴权/注册，无9033/时间异常；后续主城和断线复连进展见 E46，不应沿用当时的未验收结论 | 高（历史实机和数据库） |
| E45 | 等长原生库引导、部署哈希、当时 nat OUTPUT 只有 ACCEPT | mumu-direct/deployment.json、交接状态.json、deploy/mumu_direct_login.py | 历史 root 注入路线；现用重打包 APK 与 v21，禁止直接运行旧工具的 update/restore | 高（历史设备核对） |
| E46 | 17:04—17:18 实机连接、17:18 服务重启后的同 Avatar 复连 | PROGRESS.md、HANDOFF.md、服务及客户端日志 | 心跳连接超过旧 120 秒断开窗口，重连认证已落库；不证明完整战斗快照恢复或持续压测 | 高（实机和远端） |
| E47 | 创建昵称、性别、语音设置和引导任务持久化 | HANDOFF.md、handoff-state.json、数据库与客户端回调 | 当前玩家 匿名玩家，昵称落徒彦；1000/1001 已完成，1002 进行中；数据库位于远端游戏服的 127.0.0.1:15432 | 高（实机和真实数据库） |
| E48 | 18:08 实机首战加载、首回合与普通攻击教学 | handoff-client-full.log、handoff-server-full.log、HANDOFF.md | v21 客户端已出现普通攻击引导；服务器仅启动教学战斗 10001 的第一回合，点击技能请求 do_command 后因业务未实现停住 | 高（实机与服务日志） |
| E49 | assemble 同名方法链、card_list 自定义类型、首回合驱动 | dis/assemble.asm、npk_scripts/android_base/2948C748.marshal、dis/battle-driver.asm、rpc-semantics.md | 初始卡牌映射需由原客户端链建立；空卡牌列表仍需类型标记；不能据此宣称已有完整战斗计算 | 高（客户端指令与回放） |
| E50 | 游戏服二进制哈希、PID、端口、备份与数据库进度只读核验 | handoff-state.json、remote-release-verification.json、out/tools/handoff_audit.py | 当前 gameserver SHA256 为 7a27a407dad2c351fe17c90b0f0b1cc87cedcf2bc2eaab6a8d702d38eca2238b；五级回滚链及危险中间版见 HANDOFF.md | 高（18:12 远端只读核验） |
| E51 | 完整帧发送加锁、Ext42 原生 ObjectID、解码边界与真实数据库六项检查 | gate-protocol-spec.md、SERVER.md、database-test.log | 本地测试及真实数据库六项检查通过；测试后的日志写入曾因重定向同名文件报错，需区分测试结果与工具退出状态 | 高（代码和测试）；非完整业务验收 |
| E52 | 首次普通攻击原请求及服务端原始归一化链 | dis/server-control.asm、dis/battle-player-input.asm、HANDOFF.md | do_command 需验证当前输入实体、技能和目标并驱动权威战斗状态；原参数不能直接回显为结算结果 | 高（静态契约），业务待实现 |

## 当前未解决

- 完整自动 Python 源码反编译及低频 opcode；不可恢复固定 K/取反模型。
- 长期时间行为、持续连接与完整战斗断线恢复；本轮已完成主城进入及服务重启后的 Avatar 复连，不能据此宣称战斗状态完整恢复。
- 动态/隐式 RPC、业务方向、自定义复合类型与完整状态语义；静态注册数不等于协议全集。
- 当前最高优先级是 do_command 与权威战斗状态，随后验证首战结算、引导 1002 后续链及背包、角色等动态数据。三项登录后接口的已实现范围见 SERVER.md；禁止沿用早期的“全部未实现”结论。
- 网易 SDK 正式鉴权、新设备路由与证书信任、完整资源下载及持续真实业务压测。当前重打包 APK 已走 v21 启动路线；历史 root 注入工具不再作为现行操作入口。
