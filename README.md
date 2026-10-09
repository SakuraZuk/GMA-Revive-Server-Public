# 幻书启示录服务端

## 项目来源

本项目是 [ShigemoriHakura/GMA-Revive-Server](https://github.com/ShigemoriHakura/GMA-Revive-Server) 的派生版本，从其 fork `SakuraZuk/GMA-Revive-Server` 的 `main` 分支继续开发。在原项目的 Python 服务端实现及协议资料基础上，本分支继续进行 Go / PostgreSQL 重构、功能完善和玩家反馈故障修补。

公开仓库 `SakuraZuk/GMA-Revive-Server-Public` 使用经过隐私清理的独立提交历史，因此 GitHub 页面不显示 fork 标记；项目来源与派生关系仍按上述说明保留。上游项目的访问取决于其仓库权限。

现行实现为 Go / PostgreSQL 服务端，适配 Android 1.0.128。当前交付包含 `runtime2026100911` 的登录大厅收尾、升格材料副本恢复、幻书归还、抽卡回调及界面偏好修补。普通副本保持 Android 原生计算；PVP 使用服务端单原生权威，具体已验收范围与未完成项以交接资料为准。

本仓库只保存服务端源码、生成后的规则目录、部署工具、测试和详细资料。客户端 APK/NPK/音视频、玩家存档与日志、数据库备份、构建产物、登录私钥和原生 Python2 运行时均不上传。原有Python版保留在原私有fork；公开仓库只接收审核后的现行Go源码树，使用独立提交历史，不携带原仓库历史配置或密钥。

## 资料入口

- [服务端技术、配置、接口与近期修补](SERVER.md)
- [统一交接与接续要求](out/HANDOFF.md)
- [进度及验证证据边界](out/PROGRESS.md)
- [剩余施工和验收清单](out/REPAIR-TASKS.md)
- [登录链路](out/login-flow-spec.md)、[网关协议](out/gate-protocol-spec.md)、[RPC 类型与回调](out/rpc-semantics.md)
- [热更协议](out/hotfix-protocol.md)、[逆向交接](out/HANDOFF-REVERSE.md)

## 目录

| 目录 | 用途 |
| --- | --- |
| `cmd/gameserver` | TCP 游戏入口与网关 |
| `cmd/loginserver`、`cmd/sdkserver`、`cmd/hotfixserver` | 登录、SDK及热更入口 |
| `internal/game` | 业务、规则目录、持久化模型和测试 |
| `internal/game/dbstore` | PostgreSQL存储、事务及真实数据库回归 |
| `internal/nativepvp` | PVP原生权威适配、工作进程与校验工具 |
| `deploy` | 构建脚本、systemd配置、部署及公开数据配置 |
| `out/tools` | 规则生成、原包提取、验收、部署和运维工具 |
| `out/*.md` | 已整合的项目详细资料 |

## 构建与测试

使用 Go 1.25 工具链。在项目根目录执行：

```powershell
go test ./...
go vet ./...
go build -o out/bin/windows/gameserver.exe ./cmd/gameserver
```

Windows正式跨平台构建可设置 `HS_GO` 为本机go.exe绝对路径，再运行 `deploy/build-game.cmd`。Linux直接使用 `go build -o out/bin/linux-amd64/gameserver ./cmd/gameserver`。生成后的业务JSON已在仓库，基础编译不需要客户端原包。

实际PostgreSQL验收需要隔离测试数据库；未配置数据库出现SKIP不能称通过。PVP原生验收及原包规则重新生成还需要单独配置原生运行时与原始资源，仓库不伪造或下载这些依赖。详细方法及目录见SERVER与交接资料。

## 配置与部署

`deploy/data/gameplay-rules.env` 保存公开业务开关，测试阶段 `HS_NEW_AVATAR_ALL_HEROES=1` 仅对真正新建角色生效。数据库连接、服务器密码和登录私钥必须另行配置，不能提交。部署前完成正式构建、真实数据库检查和 `out/tools/deploy_completion_release.py --check`；发布后执行独立远端与原生核验，具体发布链及服务目录见SERVER。

部署工具通过 `out/tools/local_ops_credentials.py` 读取授权。Windows当前用户可使用 `%LOCALAPPDATA%/hs-server/operations.dpapi` 中的DPAPI密文，文件不在项目内；其他电脑和Linux使用环境变量 `HS_GAME_DEPLOY_HOST/PORT/USER/PASSWORD`、`HS_HOTFIX_DEPLOY_HOST/PORT/USER/PASSWORD`。DPAPI文件绑定Windows当前用户，不能当作跨机器备份直接复制。GitHub令牌保存在Git Credential Manager的Windows凭据存储，Git远端URL不含令牌。

运维工具中存在原本地资源或目录依赖，新电脑请按资料配置；只克隆本仓库不等于拥有玩家数据库、登录私钥或原生运行时。不得用清空存档、补造奖励或跳过断言掩盖故障。

## 当前验证范围

0911发布时完整Go测试、vet、正式构建通过，真实PostgreSQL63项通过、0跳过，另3项辅助通过；独立线上哈希、服务与监听核验通过。上线后两个短日志窗口19次冷登录全部收到登录刷新。此证据不是所有Android设备画面或长期负载通过；体力材料、普通退出、聊天等仍开放的真实报错详见SERVER顶部。

后续修改先阅读项目全部Markdown，再把新接口、配置、持久化规则、测试结果和未完成边界整合进既有资料。文档使用中文UTF-8，不重复堆积交接文件。

## 公开交付与隐私要求

公开仓库为 `SakuraZuk/GMA-Revive-Server-Public`，原私有fork保留。禁止收录发布者电脑绝对路径、服务器实际地址/账号/口令、私人邮箱、令牌、玩家数据与密钥；配置中的192.0.2.0/24是文档示例网络，不是可用生产地址。路径从项目位置、PATH或环境变量解析，构建工具可用HS_GO覆盖。

公开前运行 `python out/tools/audit_public_source.py`，先提交审核过的工作区，再运行 `python out/tools/publish_public_source.py`。该工具在仓库外的独立检出中发布当前源码树，只接续公开版本历史；禁止直接将原私有仓库分支或标签推送到公开仓库。Git作者邮箱使用GitHub noreply地址。

完整生产分发配置留在仓库外当前用户应用数据目录的hs-server/deployment-config。后续部署必须另行渲染私有地址/密钥并重新验收；公开源码上传不改变线上服务，不证明旧私有提交或GitHub缓存已删除。

仅源码克隆时，真实客户端录像样本子用例会明确SKIP；恶意pickle及归属安全校验继续执行。真实PG/Python2/原始录像专项需要另配依赖，跳过项不算验收。
