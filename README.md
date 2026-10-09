# 幻书启世录服务端

面向《幻书启世录》Android 1.0.128 的服务端兼容、协议研究与功能修复项目。当前分支采用 **Go / PostgreSQL** 实现，公开服务端源码、业务规则、测试、部署工具和中文技术资料，供开发者研究、复现与协作维护。

> 项目处于持续开发与验证阶段。当前源码包含运行期热修 **`runtime2026100914`**；功能实现、数据库回归、真实客户端操作和持续负载是分别核验的范围。本仓库不将源码可编译或短窗口日志正常表述为完整游戏验收。

[技术说明](SERVER.md) · [开发交接](out/HANDOFF.md) · [验证进度](out/PROGRESS.md) · [待修清单](out/REPAIR-TASKS.md) · [问题反馈](https://github.com/SakuraZuk/GMA-Revive-Server-Public/issues)

## 项目来源与分支定位

本项目派生自 [ShigemoriHakura/GMA-Revive-Server](https://github.com/ShigemoriHakura/GMA-Revive-Server)，从其 fork `SakuraZuk/GMA-Revive-Server` 的 **`main` 分支**继续开发。在原项目的 Python 服务端实现、协议资料和相关功能基础上，本分支持续进行 Go / PostgreSQL 重构、持久化完善及实际玩家故障修复。

公开仓库 `SakuraZuk/GMA-Revive-Server-Public` 使用经隐私审核的独立提交历史，GitHub 页面因此不显示 fork 标记。该发布方式保留上述来源说明，同时避免公开原私有仓库历史中的配置、密钥和个人信息。上游项目的访问受其仓库权限控制。

## 技术架构

| 组成 | 职责 |
| --- | --- |
| 游戏网关 | TCP 连接、会话鉴权、MobileRPC 编解码、角色绑定及业务调度 |
| 业务服务 | 角色、幻书、材料、阵容、成长、活动、社交及相关接口 |
| PostgreSQL 存储 | 账号与角色进度、资产事务、版本读取、奖励收据及重登恢复 |
| 登录与 SDK 服务 | 客户端登录及 SDK 兼容入口 |
| 热更服务 | 启动期与运行期脚本分发、客户端接口兼容和诊断 |
| 原生 PVP 适配 | 隔离 Python2 工作进程、原生权威日志与回放接口 |
| 验证与运维工具 | 规则生成、原包契约提取、数据库回归及发布核验 |

**普通副本保持 Android 客户端原生计算**，服务端负责事件校验、资产提交、结算收据和恢复。PVP 原生权威路线需要单独配置运行时及对应资源；仅克隆本仓库不会自动具备该运行环境。具体路线、协议边界及验证情况见 [SERVER.md](SERVER.md)。

## 当前功能与近期修补

现有业务涵盖登录与建角、角色持久化、幻书成长与管理、阵容和契印、收藏室、活动副本、任务奖励、邮件及社交/PVP适配等模块。各模块的实际覆盖范围以技术说明与验收清单为准。

截至 `runtime2026100914`，近期源码更新包括：

- **登录与恢复**：修正已完成普通副本重登遗漏大厅登录刷新，增加收尾失败重试；调整普通战斗观察与重连检查点，减少材料副本误重开。
- **材料及幻书管理**：实现灵感材料使用、锁定/解锁幻书、归还返料及展示槽清理，保留归属、余额、溢出及重复操作校验。
- **普通副本退出**：使用持久化的失败结算回应退出，补全退出确认后的结果回包；重复退出不重复返还资产或弹出结算。
- **抽卡与活动回调**：接入已核对的原包卡池、修正原生回调类型及夏日节点回调参数，保护签到和抽卡的迟到界面回调。
- **界面偏好**：保存配音、展示槽、看板随机及已核对的探索偏好，兼容跳过布阵的整数/布尔开关。
- **引导诊断**：保护新手奖励列表的失效行，记录真实引导启动、持续等待及探索入口初始化；诊断不改变引导完成条件或自动解锁系统。

测试配置提供 `HS_NEW_AVATAR_ALL_HEROES` 开关。当前示例规则为 `1`，仅对真正新建的角色初始化本版可用英雄；不会给旧角色补发或在重登时重复赠送。关闭设置为 `0`。具体范围与事务规则见技术说明。

## 目录结构

| 路径 | 内容 |
| --- | --- |
| `cmd/gameserver` | 游戏 TCP 入口、网关与服务启动 |
| `cmd/loginserver` / `cmd/sdkserver` | 登录及 SDK 服务入口 |
| `cmd/hotfixserver` / `cmd/genkey` | 热更服务及会话密钥生成工具 |
| `internal/game` | 业务实现、生成后的规则目录、协议投影及回归测试 |
| `internal/game/dbstore` | PostgreSQL 存储、事务与真实数据库测试 |
| `internal/mobileproto` / `internal/session` | 移动端协议及会话处理 |
| `internal/nativepvp` / `internal/nativeengine` | 原生工作进程适配与权威接口 |
| `deploy` | 构建脚本、公开示例配置及服务部署文件 |
| `out/tools` | 规则提取、验证、部署及公开发布工具 |
| `SERVER.md` / `out/*.md` | 集中维护的中文技术与开发资料 |

## 环境与构建

`go.mod` 声明 Go 1.22，当前发布验证使用 **Go 1.25**。数据库验证使用 PostgreSQL 15。基础源码构建需要 Go；SSH 运维工具另外使用 Python 与 Paramiko，公开发布工具的安全归档提取需要 Python 3.12 或以上。原生 PVP 专项使用单独配置的隔离 Python2 运行时。

```sh
git clone https://github.com/SakuraZuk/GMA-Revive-Server-Public.git
cd GMA-Revive-Server-Public
go mod download
go build ./...
go test ./...
go vet ./...
```

构建游戏服务：

```sh
go build -o out/bin/gameserver ./cmd/gameserver
```

Windows 可运行 `deploy/build.cmd` 构建四类服务，或 `deploy/build-game.cmd` 构建游戏服务及数据库验收程序。脚本默认从 PATH 查找 Go，也支持通过 `HS_GO` 指定工具链。

生成后的业务 JSON 已包含在源码中，基础编译无需客户端原包。重新生成这些目录、执行原包契约验证或运行原生 PVP 专项，需要另外准备相应资源。

## 运行与配置

游戏服务启动前应配置数据库、活动日程、客户端分发地址和会话密钥。缺少数据库配置时会尝试读取本地开发夹具 `accounts.dev.json`；公开仓库不包含账号数据，因此该模式需要开发者自行准备夹具，不能视为生产存储。

| 环境变量 | 用途 |
| --- | --- |
| `HS_DATABASE_URL` | 游戏服务 PostgreSQL 连接字符串 |
| `HS_DATABASE_MAX_CONNS` | 数据库连接池上限 |
| `HS_GAME_BIND` | 游戏服务监听地址 |
| `HS_GAME_ADDRESS` / `HS_HOTFIX_ADDRESS` | 客户端可达的游戏与热更地址 |
| `HS_DNS_ANSWER` / `HS_GAME_DNS_ANSWER` | 对应分发服务的 DNS 地址配置 |
| `HS_DATA_DIR` | 配置目录，默认 `deploy/data` |
| `HS_ACTIVITY_SCHEDULE_FILE` | 活动日程文件；未指定时从配置目录读取 |
| `HS_GAME_RSA_KEY` | 本机保管的登录会话私钥文件 |
| `HS_NEW_AVATAR_ALL_HEROES` | 新角色测试英雄初始化开关 |
| `HS_NATIVE_PVP_PYTHON` / `HS_NATIVE_PVP_WORKER` | 隔离原生工作进程入口 |
| `HS_NATIVE_PVP_DIRECTORY` / `HS_NATIVE_PVP_RESOURCE_SHA256` | 原生资源位置与一致性校验 |

完成自身配置后可使用 `go run ./cmd/gameserver` 启动游戏入口。登录、SDK与热更服务分别由相应 `cmd` 子目录启动。会话密钥生成、公钥匹配、SDK地址与热更组合方式详见 [登录链路](out/login-flow-spec.md) 和 [热更协议](out/hotfix-protocol.md)。

`deploy/data` 中的 `192.0.2.0/24` 地址属于文档示例网络；启动期热更及分发清单也需要使用自身配置渲染。公开示例不能直接覆盖现有生产配置。

## 测试与发布验证

| 验证层次 | 条件与解释 |
| --- | --- |
| 源码构建与单元回归 | `go build ./...`、`go test ./...`、`go vet ./...` |
| 真实 PostgreSQL | 设置 `HS_TEST_DATABASE_URL` 指向隔离测试数据库后执行 `go test ./internal/game/dbstore` |
| 原生 PVP / Python2 | 显式配置隔离运行时、资源及专项测试变量，参见技术说明 |
| Android 实际操作 | 以真实客户端行为和对应日志确认，不能由服务端夹具替代 |
| 持续负载与恢复 | 需要独立长窗口与并发验证，短窗口无异常不作为全面稳定结论 |

真实数据库测试为用例创建隔离命名空间，测试连接与生产运行连接分别配置。缺少数据库、原生运行时或真实录像样本时，相关用例会明确 `SKIP`；跳过项不计为验收通过。恶意录像拒绝及归属校验等不依赖真实玩家数据的安全断言仍执行。

最近一次 `runtime2026100914` 发布记录包含：默认/正式规则回归、vet和构建通过，真实 PostgreSQL **64项通过、0项跳过**，另有3项辅助用例通过，以及原生夹具和独立发布核验。该记录描述对应发布批次；公开示例与生产分发配置的哈希可能不同。详细证据范围见 [验证进度](out/PROGRESS.md)。

部署工具提供预检、备份、组合发布及独立核验流程。私有配置与公开源树分别准备，入口与安全边界见 [SERVER.md](SERVER.md)。本次 GitHub 源码更新不触发新的生产部署。

## 已知限制与待修事项

当前仍需继续验证或修复的项目包括：部分功能解锁引导及探索卡点、成就总览迟到回调、挑战空目标语义、部分聊天/聚焦/排行榜/双倍奖励入口、数据库超时和长期负载。普通退出的结果链已补齐，但真实客户端返回界面的全面验收仍单独保留。

新增诊断只记录实际步骤与等待情况，不跳过教学、强制解锁或补造奖励。具体已实现、待实现及待验证事项持续维护在 [REPAIR-TASKS.md](out/REPAIR-TASKS.md)。

## 问题反馈与参与维护

请通过 [GitHub Issues](https://github.com/SakuraZuk/GMA-Revive-Server-Public/issues) 提交可复现问题，注明源码提交或热修版本、客户端版本、复现步骤、预期结果和实际结果。可附最小必要的已脱敏错误片段，并区分客户端异常、业务拒绝和服务端超时。

公开反馈中不要包含账号口令、令牌、私钥、完整数据库、玩家存档、实际服务器地址或电脑绝对路径。涉及个人信息的截图和日志应先脱敏；本项目的问题反馈模板不要求提交真实玩家标识。

修改代码前应阅读项目 Markdown，尤其是技术说明、交接、进度和待修清单。新增或调整接口时，应在既有资料中记录参数、回调类型、持久化事务、失败语义、测试及验证边界。提交说明应描述可观察行为及对应验证结果，文档统一使用中文 UTF-8。

## 公开发布与隐私边界

本仓库不分发客户端 APK/NPK、音视频、原生运行时、玩家存档与日志、数据库备份、构建产物或部署密钥。运行与迁移所需的私有配置由部署者自行保管；部署工具可从环境变量或当前用户的本机安全存储读取，秘密不写入源码。

维护者发布公开版本时，先运行 `python out/tools/audit_public_source.py` 审核当前 Git 文件，再通过 `python out/tools/publish_public_source.py` 导出已提交源树。该流程只接续公开仓库历史，不携带原私有仓库的提交、分支或标签。公开提交使用 GitHub noreply 邮箱。

## 技术资料索引

| 文档 | 内容 |
| --- | --- |
| [SERVER.md](SERVER.md) | 配置、业务接口、原包契约、事务及修补记录 |
| [HANDOFF.md](out/HANDOFF.md) | 开发接续入口及当前基线 |
| [PROGRESS.md](out/PROGRESS.md) | 验证记录与证据边界 |
| [REPAIR-TASKS.md](out/REPAIR-TASKS.md) | 已完成、待修和待验收清单 |
| [gate-protocol-spec.md](out/gate-protocol-spec.md) | 网关帧、握手与会话协议 |
| [rpc-semantics.md](out/rpc-semantics.md) | RPC类型、方法及回调约定 |
| [login-flow-spec.md](out/login-flow-spec.md) | 登录与角色绑定链路 |
| [hotfix-protocol.md](out/hotfix-protocol.md) | 启动期、运行期热更与分发 |
| [HANDOFF-REVERSE.md](out/HANDOFF-REVERSE.md) | 原包解析与逆向资料接续 |

较早技术文档保留历史取证，版本相关结论只适用于对应记录时点；当前状态以本文及核心资料顶部最新批次为准。
