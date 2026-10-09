# 登录、时间与热修契约

> 2026-10-07 文档整理：本文件保留历史取证，日期相关的“当前/下一任务/未实现/未发布”仅对应原记录时点，不能当作现行状态。统一交接见 [HANDOFF.md](HANDOFF.md)，当前施工见 [REPAIR-TASKS.md](REPAIR-TASKS.md)。全部战斗现走客户端原生计算，F2已完成，不再执行旧权威引擎施工或协议闸门改包。


> 2026-10-05 本轮复核：当前MuMu运行v21；主城/创建引导/首战场景已有实机证据；非战斗重连已恢复原Avatar。最新接口、动态JSONB和边界以 SERVER.md、HANDOFF.md、PROGRESS.md 为准。

更新：2026-10-05。客户端指令依据 dis/315EAFEB（登录 UI）、B9F5C696（Account）、2034D84C（Avatar）、9AD0A456（time_utils）、7A97880C（timezone_info）、063CF80D（运行热修）、54971139（启动热修）。PC 包 UI 有“技术升级”拦截，真实验收以 Android 为准。

## 客户端登录时序

选择服务器 → start_game/on_touch_login → network_mgr.connect_server(server,account,is_quick) → Gate 握手 → 创建 Account → quick_login 或 sdk_login → login_result → 角色列表 → 创建/绑定 Avatar → 本地登录收尾 → 时间同步与进入世界。

当前Go鉴权后选择该hostnum唯一角色创建Avatar，没有完整多角色选择协议。18:08复登已到教学10001首回合普通攻击提示，资料/任务保留；点击技能停在do_command未实现。完整战斗、战斗重连和真实账号输入UI仍缺。

## 当前原登录按钮直连实现

用户明确选择保留游戏原登录按钮，点击后以本地设备账号自动注册。deploy/client_direct_login.py 将 sdk_client_login 接到原 start_game；构造真正 server_info，设置 hostnum=10001 和 192.0.2.10:9000，强制 TCP/msgpack、关闭压缩并注入服务器公钥。cache 的 hs_direct_account/hs_direct_secret 分别保存随机账号与64字节随机口令；Account.quick_login 使用这两项，原 client_info 结构不变。它不读取网易账号密码，不走 Mpay 停服账号窗口。

启动引导扫描EntityManager，对新Account/Avatar补本地生命周期；实体离开后清理已处理集合。当前v21实际代码以deploy/data/hotfix_startup_v9.py为准，不能拿旧模板代替线上内容。绑定完成后的set_reconnect_auth_msg触发一次on_refresh_login，避免过早读取Account.card_mgr。当前实机账号匿名玩家、OID6ac32910b2e86acd79b13df0、落徒彦，18:12数据库快照handoff-state.json与18:08日志一致；旧OID只是历史测试账号。

当前加载方式是重打 APK/NPK 与远端启动热修 v21，已实机确认；旧 libclient.so/hs.py 仅历史方案，不能再照旧 update/restore。维护流程和资源注意事项见 HANDOFF.md 第4节。

18:08实机启动请求为http://192.0.2.20/h62/pl/netease/patch_hotfix_data_pub，日志加载HS_DIRECT v21。旧e30=/8082/root注入方案只作历史取证；当前维护方式见HANDOFF.md第4节。运行期query_hotfix通道保持，未来源码仍须记录索引并幂等。

## 上行契约

quick_login(client_info)：hostnum、account、password、hotfix_index、need_guide_ids、conn_type；GM 分支可加 gm_role_enter_type/gm_online_time_left，有 register_info 时追加。sdk_login(client_info,sdk_info) 由 SDK 构造信息，本项目默认拒绝，尚无网易鉴权。

query_hotfix(hfindex) 每 90 秒轮询；query_server_time() 为无参空字典；set_reconnect_auth_msg 保存重连凭据摘要并触发一次 on_refresh_login，guide_tasks 已由实体属性初始化，不推普通本地条件通知。heart_beat(last_send_time) 必须回两个 Float。guide_task_finished/upload_guide_tasks 的首参为 callback_id；前者事务完成后回 Bool/String。set_nickname_gender 回 Int；语音设置回 Bool/String；enter_dungeon 的 callback 仅失败通道，成功由战斗创建及共享准备消息驱动。详细八要素和持久化见 SERVER.md。

AvatarEntity.call_server(method,…,callback=…) 可把 callback_id 放到参数首位，完成后通过 call_client_callback(callback_id,result_args)（Callback, Tuple）恢复回调。不能把所有命名转发与直接代理当作相同参数序列。

## 已证的装饰方法

| 方法 | 签名 | 说明 |
|---|---|---|
| Account.login_result | Int, Str, Int | ret_code、reason、conn_type；time_over_flag/notice 是函数内部局部变量 |
| Account.on_get_all_avatars | List | 每项 hostnum/avatar_info |
| Account.on_hotfix_when_login | Str, Int | 登录期运行热修源码与索引 |
| Account.notice_login_queue | Int, Int | 排队位次/队长 |
| Account.on_quit_login_queue | 空签名 | 已注册，不是普通未注册回调 |
| Account.on_sauth_success | Str, Str, Dict | SDK 授权相关，未实现业务 |
| Account.on_get_realname_info | Bool, Int | 实名标志与年龄段，未实现业务 |
| Avatar.on_query_hotfix_success | Str, Int | 运行期热修回复 |
| Avatar.on_refresh_login | 空签名 | 已注册；set_reconnect_auth_msg 绑定收尾后首次推送，不能在 BecomePlayer 过早推送 |
| login.sync_server_time | Float, Int | Unix 秒与 time.timezone 方向的秒值 |
| login.on_kick_avatar | Int, Str | 踢人类型与原因 |
| AvatarEntity.call_client_callback | Callback, Tuple | 回调编号与结果参数 |

on_login_success() 与 on_become_player() 是普通本地函数；RPC 分发会传 parameters 给包装器，直接伪造这两条下行会参数不匹配。BecomePlayer 是 Go 内部动作，只推时间，不虚构新接口。全部显式签名以 client_catalogs/rpc-catalog.json 为准，方向仍需调用链证据。

## 角色字段与错误码

avatars 每项 {hostnum,avatar_info}；UI 必取 nickname、head_id、custom_head_image_url、head_box_id，level 用 get。head_box_id 必须能索引 head_box_info，1 已确认存在；这些只是 UI 字段，不能替代完整 Avatar 属性。

RET_SUCCESS=0，账号空=9001，鉴权失败=9002，服务器号空=9011。具体值见 login-constants.txt 和 inspect_login_contract.py。失败先 disconnect_server_without_reconnect；IP/设备限制的 reason 作为弹窗参数。kick_type 异地登录=1、封禁=4、防沉迷=5、实名=6。

## sync_server_time 根因证据与修正

login.sync_server_time(t,tz) 调 time_utils.reset_time_function，然后 time_to_str(now_time(),use_server_timezone=True)。reset 在偏移 27 用 op67 对 tz 取负；timezone_info 再用 timedelta(seconds=-tz)。因此服务器必须发 -28800，得到 UTC+8。旧 8 产生 -8 秒偏移；旧 +28800 产生 UTC-8。

Python 2 的 datetime 时区检查要求 utcoffset 是整分钟；旧 8 可触发 fromtimestamp 错误。源码依据 [CPython 2.7 datetimemodule.c](https://raw.githubusercontent.com/python/cpython/v2.7.18/Modules/datetimemodule.c) 的 call_utc_tzinfo_method。NeoX 定制运行时仍需实际日志确认。time_to_str 同时捕获 fromtimestamp 与 strftime 的异常并输出 invalid timestamp，历史记录未保存异常类型，不能断言时区是所有异常的唯一原因。

已修正 Unix 秒 Float 小数、时区 -28800 和空参数字典；测试中保留 1700000000.5 的小数，msgpack 线缆值仍为 Float。静态和 Python3 模型报告 time-contract-verification.json，公网协议通过；10:09 Android 登录新日志无 invalid timestamp，服务端确实发送时间推送，但长期消除和完整客户端时间使用仍未验收。

## 两个热修通道

启动期：HTTPS /pl/patch_hotfix_data_pub 返回 base64(JSON{版本键:Python2源码})；普通键匹配运行时 version.VERSION，b 前缀键匹配引擎版本。基础包 1.0.125 被 1.0.128 补丁覆盖，当前匹配 1.0.128。无命中可放行，但不会执行修复。TLS 需可信链；清单不能为 {}，当前空资源表仅供已有完整 files 的相同版本客户端。

运行期：hotfix_data.data 初始 {index:0,script:"\n"}，进入玩家后每 90 秒 query_hotfix。服务端更大索引回完整源码和新索引，否则回空源码与客户端索引；登录期同规则。客户端模块清缓存后会重放 LAST_SCRIPT，因此源码幂等、跨索引完整修复，不能只发增量。框架不执行这些客户端源码。

数据库位于远端游戏服，本轮主城、创建引导与首战场景已有实机证据；非战斗重连恢复原Avatar已通过。测速和空公会消息板三接口闭环；排队、完整战斗重连、SDK票据和玩法仍需完成。
