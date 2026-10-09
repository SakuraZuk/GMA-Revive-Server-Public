# 热更协议与发布现状

## 外部全资源目录核验与发布判定（2026-10-09）

本次用户提供来源为 `.\com.netease.hsqsl`，要求先查验，确认没问题再接入热更。完整读取项目现有18份MD，核验记录为 `out/external-resource-audit-20261009/markdown-read-inventory.json`。本次只新增只读核验工具与证据，不修改生产清单、资源文件、游戏程序或玩家存档，也没有操作MuMu。

**结论：资源量较全，但不是可直接整体发布的干净资源集。三份通用/中文/日文音库已在线，来源与现行分发完全一致；整目录尚不满足直接发布条件，不能标为“全语音、全场景资源已验收”。**

### 文件与完整性证据

- `files/netease/h62/Documents`有21个NPK，共10,164,180,878字节。按本版ARM已证索引公式解码，逐项核对重复ID、存储边界、索引重叠、压缩类型及解压长度；143,485项全部解压通过，错误0，存储重叠0。原生公式来源为 `out/dis/remaining-collection-npk-native.asm`，不会再用旧工具的原始偏移误判大包损坏。
- 同时读取 `files/netease/h62/res` 与 `Documents/res`，共1,274个散装文件、985,157,327字节；包含62个MP4、440个WEM、679个NFXO、89个PNG及清单/缓存压缩包。WEM全量RIFF/WAVE长度核验通过；440份文件仅212种不同SHA256内容，存在重复，不能按文件个数推定全部语音覆盖。
- 三份音库 `wwise.npk`、`wwisech.npk`、`wwisejp.npk`共591,836,496字节，分别有199、152、125个BKHD音库条目。目录、现行补丁清单、远端资源文件三方MD5和大小一致，已通过现行热更分发，不需要重复替换。包含音库不等于已确认每个剧情、角色和语言的全部语音依赖齐全。
- 远端只读核对 `/opt/hs-res` 真实路径、hs-hotfix服务与监听、16个现行包MD5和清单SHA256。远端补丁清单与本地字节一致，SHA256为 `010a02c8b15485341c1c418557f8e1aa8f65cd6362aec00fd79d0444722c69aa`；运行期热修SHA256仍为 `514a61a2cd27eb70b12f9d8a446e4a5ef8d8f70857ed258a58120c3f682ec35a`。本轮未重启任何生产服务。
- 中文音库公网实际Range请求返回206，`Content-Range: bytes 0-63/232290528`，64字节与来源一致，报告 `http-range-verification.json`。该检查只证明本次分块请求；整文件一致性由远端MD5核对证明，不扩大为Android播放验收。主报告条目数、7项非官方脚本、线上清单及散装WEM已交叉校验；新增工具语法和修改文档UTF-8检查通过。

### 与现行热更的差异及处理

| 来源资源 | 核验结果 | 本次处理 |
|---|---|---|
| char1/2/3/4、effect、scenewd1、uiicon、wwise/wwisech/wwisejp | 10包与现行清单及远端文件一致 | 已在线，保持现行分发 |
| char5 | 2,269项可读，Android KTX资源存在；现行清单没有此包 | 保留为待接入候选，尚未证明版本依赖及跨包路径覆盖兼容 |
| char6 | 原596项内容全部相同，新增170项，无缺少 | 保留候选；不能以新增条目数量代替客户端验收 |
| ui | 原13,845项内容全部相同，新增109项，无缺少 | 保留候选 |
| res | 共有11,515项内容相同，但比现行基础包少2项 | 不覆盖现行包；缺少ID为1447422837、4220706976 |
| scene | 共有6,389项，其中2项内容变化，新增1,238项 | 需确认变更资源、来源和场景依赖后才能合并 |
| scenewd2 | 共有10,189项，其中1项内容变化，新增93项 | 同上，不能直接覆盖当前已验资源 |
| reslow、reslow_char、reslow_scene、reslow_ui | 共6,456,714,358字节，全量可解压；客户端存在不支持ASTC时选择低兼容资源的分支 | 是Android兼容资源候选，不按DDS后缀误判PC包；仍需核对资源集合和低兼容设备实际加载 |
| script | 3,100项完整合并包；相对原APK19模块变化，12项与官方1.0.128补丁一致，另外7项是非官方改动 | 禁止整体替换现行65,484字节官方覆盖包 |
| 主资源散装清单 | 246项全部存在，大小和MD5全部匹配 | 可作为后续视频/音频接入来源，尚未接入生产散装下载 |
| Documents散装清单 | 680项存在，679项实际大小或MD5与所附清单不一致 | 不能原样发布清单；着色器缓存需要确认生成平台及校验语义，不把所有文件直接判为损坏 |
| cache、日志、原件恢复前备份 | 设备缓存、用户数据或历史追溯文件 | 不作为热更下载文件，不删除用户原件 |

### 明确的脚本发布阻断

非官方7个模块为 `guis/login/server_list_mgr.py`、`patch_logic/patch_utils.py`、`patch_logic/data_receiver.py`、`patch_logic/patch_hotfix.py`、`utils/const_utils.py`、`client/network_mgr.py`、`patch_logic/http_downloader.py`。其中server_list_mgr含 `http://192.168.233.53:8443/%sXXXXX`，data_receiver和http_downloader含 `http://192.168.233.53:8443/patch`，patch_hotfix含旧内网模板。以上均为实际脚本常量，解密、marshal完整消费后读取，没有执行来源字节码。不能把这份完整脚本当作纯资源替换，否则会带入另一环境的更新行为。详细模块及地址见 `script-route-constants.json` 与主报告的脚本核验部分。

### 接续入口与验收限制

工具为 `out/tools/audit_external_resources.py` 与 `out/tools/finalize_external_resource_audit.py`，前者全量解压、保存逐项存储MD5与内容SHA256、对比现行包；后者核对散装清单、WEM结构、脚本差异和线上实际文件。依赖本机现有Python的lz4、paramiko及项目已有rotor/marshal工具；远端连接从现有已授权脚本导入，不在报告或文档复制口令。

复核命令：

```powershell
$env:PYTHONIOENCODING='utf-8'
python out\tools\audit_external_resources.py
python out\tools\finalize_external_resource_audit.py
```

证据统一存于 `out/external-resource-audit-20261009/`：`report.json`为主结论，`*-entries.json`为逐项包哈希，`all-loose-files.json`为散装文件完整SHA256清单，`remote-audit.json`为线上只读证据，`remote-patch-list.json`为本次线上原始清单。历史报告下方保持对应原日期，不应用来覆盖本次判定。

后续接入应保留现行官方脚本和更完整的res包，对新增资源单独核查后生成新清单及完整逐文件MD5表，再执行项目目录内的备份、上传校验、原子切换与独立下载验证。当前客户端 `patch_mgr.app_version_compare` 对1.0.128同版本设置 `no_patch=True`；仅在同版本清单增加文件，不能证明已安装玩家会收到增量。版本推进、散装下载和低兼容设备选择必须分别有实际客户端证据，不能编造版本号或用上传成功代替验收。

## 以下为历史协议与发布记录

> 2026-10-07 文档整理：本文件保留历史取证，日期相关的“当前/下一任务/未实现/未发布”仅对应原记录时点，不能当作现行状态。统一交接见 [HANDOFF.md](HANDOFF.md)，当前施工见 [REPAIR-TASKS.md](REPAIR-TASKS.md)。全部战斗现走客户端原生计算，F2已完成，不再执行旧权威引擎施工或协议闸门改包。


> 2026-10-05 本轮复核：当前MuMu运行v21；主城/创建引导/首战场景已有实机证据；非战斗重连已恢复原Avatar。最新接口、动态JSONB和边界以 SERVER.md、HANDOFF.md、PROGRESS.md 为准。

更新：2026-10-05 18:12。原始地址h62.update.netease.com，专用热更192.0.2.20。当前MuMu为重打APK/NPK+启动v21，18:08实机再次确认；本轮服务端推进未改热更服。数据库在远端游戏服192.0.2.10。详见../SERVER.md与HANDOFF.md。

## 启动请求

| 路径 | 响应 |
|---|---|
| /pl/patch_hotfix_data_pub | base64(JSON{版本键:Python2源码}) |
| /pl/patch_list_pub_android.txt | Android 补丁 JSON 清单 |
| /server_list_public.txt | UTF-8 空格分列；四组网关地址，末行 network=bgp |
| /game_notice/notice_formal | XML 公告 |
| /v2/?domain=… | HttpDNS status/domain/addrs/ttl |

清单需要 0hash，android 下至少 version/base_version/npk/0patchpath；空 {} 会走异常重试。空 npk 表仅为历史基线；2026-10-06真实整包更新证据见hotfix-e2e-2026-10-06.md。资源下载支持 Range，缺资源客户端需要真正资源及校验清单。

server_list gate 取 cols[nettype+8]；地址用 ip:port，两段即可。旧 Tab 固定格式、ip:port:16 和缺少 network 尾行的样本不能用作当前发布模板。

启动热修按运行时version.VERSION匹配；基础1.0.125、补丁后1.0.128，现行键1.0.128；b前缀按引擎版本。当前v21包含公钥替换、压缩关闭、版本戳、原手动按钮、生命周期和签名白名单；日志执行证据与部署状态分开。deploy/data/hotfix_startup_v9.py文件名历史遗留、内容为v21。

## TLS 与路由验收边界

历史“自签证书不校验”结论已经撤回：后续 Android 日志证明会校验证书链。成功联调用 root iptables 把 443/80 转到项目服务，并向系统 CA 注入多 SAN 证书。MuMu 环境已由上一会话清理，不能把 root 转发到公网称为免 root 直连。

项目DNS支持UDP/TCP，仅回答配置后缀、不公共递归；netease/easebar→热更、r18sex.net→游戏。当前重打客户端已实机运行；全新真机签名/分发/资源及证书仍需独立验收，不能仅用当前MuMu推定全部设备可用。

## 日志与运行期热修

/applog、/appdump、/upload_patch_log 提供 200 空响应、限制 1 MiB；未知非根路径目前返回 {} 兜底，只为避免 SDK 重试，不代表所有接口业务已实现。

运行期 Account.on_hotfix_when_login、Avatar.on_query_hotfix_success 使用 Str, Int；每 90 秒 query_hotfix，索引独立单调增加，客户端重放 LAST_SCRIPT。源码需幂等及完整修复，配置启动读取后重启生效。详见 login-flow-spec.md。
