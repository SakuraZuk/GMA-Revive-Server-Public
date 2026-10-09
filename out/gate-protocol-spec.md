# MobileRPC Gate 当前协议规格

> 2026-10-07 文档整理：本文件保留历史取证，日期相关的“当前/下一任务/未实现/未发布”仅对应原记录时点，不能当作现行状态。统一交接见 [HANDOFF.md](HANDOFF.md)，当前施工见 [REPAIR-TASKS.md](REPAIR-TASKS.md)。全部战斗现走客户端原生计算，F2已完成，不再执行旧权威引擎施工或协议闸门改包。


> 2026-10-05 本轮复核：当前MuMu运行v21；主城/创建引导/首战场景已有实机证据；非战斗重连已恢复原Avatar。最新接口、动态JSONB和边界以 SERVER.md、HANDOFF.md、PROGRESS.md 为准。

更新：2026-10-05。依据 proto_desc/client_gate.proto、common.proto、dis/DCE9232F（AsioGateClient）、947AC97C（ServerProxy）、FB4E9EB4（RpcChannel）、0D8641E6（加密）、ABBB4C0C（方法编码）、17C141AB（索引）、7BEE0C97（扩展类型）及历史 Android 日志。当前 Go 实现和公网模拟检查已接通，Android已显示主城和教学场景，完整玩法仍未验收。

## 帧与握手

帧：uint32 小端长度（两字节命令号 + protobuf 负载的总长）+ uint16 小端命令号 + protobuf。四字节长度不包含自身；当前负载上限由 HS_MAX_FRAME_PAYLOAD 控制，默认 1 MiB，不是固定 65535。

| 顺序 | 方向与命令 | 说明 |
|---|---|---|
| 1 | C→S seed_request=0 | Void |
| 2 | S→C seed_reply=0 | SessionSeed.seed |
| 3 | C→S session_key=1 | EncryptString.encryptstr：RSA-OAEP 密文 |
| 4 | S→C session_key_ok=1 | Void；此帧已经 RC4 加密 |
| 5 | C→S connect_server=2 | ConnectServerRequest |
| 6 | S→C connect_reply=2 | CONNECTED 与 Account 实体 ID |
| 7 | S→C create_entity=3 | Account；鉴权后再创建 Avatar |
| 8 | C→S entity_message=3 / S→C entity_message=5 | 业务调用与推送 |

SessionKey 内含随机前后填充、session_key=SHA1(随机32字节)、回传 seed。RSA 为 PKCS1_OAEP/SHA1、空标签；Go 私钥由 HS_GAME_RSA_KEY 指定。当前代码仅解析并记录回传 seed，尚未校验与该连接 seed 一致，不把此握手称为完整安全实现。

libclient.so 的 async::arc4_crypter/algorithm_arc4 确认 ARC4；两方向各维护独立连续状态，对整帧含长度头加密。历史 Android 明文 ok 会导致 parse bad size，已改成密文。当前热修 zipped_channel=0，未实现/验收压缩协商；zlib_compressor 存在不代表当前服务支持压缩客户端。

## 方法、参数与实体

EntityMessage：1=routes、2=id、3=method(Md5OrIndex)、4=parameters、5=reliable、6=localid。method 子字段 1 是 raw 名字（字段虽叫 md5），2 是 index。上行 raw 非空必须优先，避免动态 salt 索引撞静态索引；raw 为空才查 31 项静态表。下行用 raw 原名、index 缺省零。未知 index 不猜方法。

parameters 是 msgpack map：{_0:第一个参数,_1:第二个参数,…}；零参数为 {}。不能发送 JSON 文本、BSON 或普通参数数组。客户端 bin 编码的字符串/键在 Go 解码时归一成 UTF-8 字符串。sync_server_time 的第一个参数保留 Float；其余 UI 整数字段恢复 Int。ExtType 42 表示 ObjectId，43 表示 datetime；当前最小业务未全面验证自定义扩展类型、Callback/Tuple 和复合类型。

实体ID使用十二字节ObjectId兼容字节串。EntityInfo：1=routes、2=type(Md5OrIndex)、3=id、4=info；type raw为Account/Avatar，info有效msgpack字典。Avatar下发持久OID/UID、资料、初始材料/体力和guide_tasks，完整库存等仍缺；Account是连接实体，不是账号数据库主键。

客户端创建实体后发送set_send_rpc_salt；当前下行不用索引，静默忽略。type1重连用设备/角色/凭据摘要恢复原Avatar，成功回复type2已有实机证据；**localid优化阈值、完整routes、BIND_AVATAR及完整战斗重连没有验收**。

## 登录最小闭环

create_entity(Account) → quick_login → login_result → on_get_all_avatars → on_hotfix_when_login → create_entity(Avatar) → sync_server_time。on_login_success 与 on_become_player 是普通本地函数，不可当作无参服务端 RPC；由启动热修本地触发。完整调用与错误码见 login-flow-spec.md。

## 静态方法表

mobilecommon.py FE413035 的 STATIC_RPC_INDEX_UPPER=31；Go StaticRPCIndex 保存以下名字，登录链路通常走原名。表存在不等于战斗业务已实现。

```
1 sync_souls_pos_v2       2 sync_monster_cmds       3 sync_props
4 sync_space_props       5 sync_stop_move_cmd      6 on_create_space_entity
7 notify_use_skill_or_normal_atk  8 sync_other_add_state  9 sync_other_remove_state
10 on_destroy_space_entity 11 face_to              12 sync_refresh_state
13 on_create_sub_mfs      14 sync_own_props         15 on_show_simple_hit_and_calc
16 on_create_sub_es       17 on_es_created_success  18 on_ping_cb_space
19 sync_other_skill_cd    20 update_other_client_cash 21 update_other_client_exp
22 sync_other_dead        23 update_other_client_total_cash 24 on_show_calc_result
25 on_sync_sub_es_destroy 26 sync_combat_props      27 on_create_fly_sfx
28 sync_start_move_cmd    29 on_show_hit_and_calc   30 sync_add_state
31 sync_remove_state
```

## 版本、地址和 TLS

基础 network_mgr 配置和早期分析曾给出 BSON、压缩和明文回执结论，已被补丁/热修/实机覆盖，禁止据此回退当前实现。Android 运行时版本 1.0.128；server_list 空格分列，gate=cols[nettype+8]，地址为 ip:port，末行 network=bgp。客户端校验证书链；历史成功环境用了 root 转发和系统 CA 注入，不是免 root 结果。

## 验证与未完成项

run_local_gate.py/smoke_gate.py验证RSA/RC4、帧/msgpack、登录/时间/热修；公网模拟与真实数据库结果见PROGRESS.md。当前v21实机已登录、主城历史显示、取名/创建引导/复登、超过两分钟存活、type2原Avatar重连、教学10001首回合普通攻击提示；技能do_command未实现。完整玩法、压缩、localid/routes、多角色绑定、战斗断线快照和长时间业务仍未验收。

2026-10-05战斗验证：嵌套ObjectId用ExtType42十二字节，cmd/gameserver/gate.go.wireValue递归保留类型；传整体JSON会丢对象。sync_battle_method中的空card.card_list要带["card.card_list","__custom_type"]让revert_arg构造对象。定时战斗推送与响应按整批加密/写入持同一锁，保证RC4顺序。ExtType43、对象键字典及全部复杂类型仍未完成。
