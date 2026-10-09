# NeoX RPC 注册、方向与调用语义

> 2026-10-07 文档整理：本文件保留历史取证，日期相关的“当前/下一任务/未实现/未发布”仅对应原记录时点，不能当作现行状态。统一交接见 [HANDOFF.md](HANDOFF.md)，当前施工见 [REPAIR-TASKS.md](REPAIR-TASKS.md)。全部战斗现走客户端原生计算，F2已完成，不再执行旧权威引擎施工或协议闸门改包。


> 2026-10-05 本轮复核：当前MuMu运行v21；主城/创建引导/首战场景已有实机证据；非战斗重连已恢复原Avatar。最新接口、动态JSONB和边界以 SERVER.md、HANDOFF.md、PROGRESS.md 为准。

更新：2026-10-05。证据为 Android 补丁优先的 marshal 和 dis/ 中的指令；静态结果 client_catalogs/rpc-catalog.json。

## 两层注册

mbengine/common/rpcdecorator.py（1F09877D）声明 CLIENT_ONLY=0、SERVER_ONLY=1、CLIENT_SERVER=2、CLIENT_STUB=3，rpc_method(rpctype,argtypes,pub,…) 把 RpcMethod 绑定到函数。类型对象来自 RpcMethodArgs。expose_to_client/expose_to_server 分别判断角色类型，不能按函数名猜。

utils/rpc.py 的 rpc('Int, Str',options=…) 是实体业务层包装：字符串按逗号拆参数类型，解析选项并转换参数。默认 options='server' 在客户端 parse_options 会生成 CLIENT_STUB；显式 client 项也存在。因此“所有 rpc.rpc 都是下行”是错误结论。目录保存原始选项，只有经调用链核验的条目标明方向，其余保留待核验。

rpc 签名覆盖必传类型，函数可能还有默认参数；应同时看签名、argcount、varnames 与默认值。空签名仍可表示真正 RPC，on_refresh_login/on_quit_login_queue 就属于这种情况。普通 on_login_success/on_become_player 没有装饰，不能因为 on_ 前缀就当推送。

## 调用与转发

engine/common/rpc.py（D7B7FC3C）的 RPCObj 保存 remote_id/name，__getattr__ 追加命名空间，__call__ 调 call(remote_id,name,args)。实体 self.server_proxy.method(…) 是直接代理；AvatarEntity.call_server('方法',…) 还可能生成 callback_id 并追加到参数首位，再通过 call_client_callback 恢复结果。

AFDD4FE7 的 pack_args 先 convert_args，再构造 {_0:参数0,_1:参数1,…}。引擎打包为 msgpack，方法名字由 Md5OrIndex 承载。Go 已连接原生帧、RSA/RC4 与此参数字典，详见 gate-protocol-spec.md；JSON 仅是本机演练或解码中间表示，不能直接放进网络 parameters。

## 静态扫描范围

扫描 3099 模块，识别 302 个显式 rpc 装饰方法（含空签名与选项），269 个直接代理引用（259 个方法名）及 221 个字面方法名 call_server 转发（220 个方法名）。目录保存模块、嵌套函数和字节码位置、默认参数与原始表达式，方便继续核验。

复杂显式注册候选数为零只说明该扫描模式全部解析；动态 getattr、非字面名称、隐式组件及运行时构造不会因此自动覆盖。302 不是“全部协议”的总数；259/220 也不能简单相加为完整上行总数，应看 summary.json 的合并去重及遗漏边界。

## 当前服务对应

已处理登录/显式注册、拒绝未实现SDK、时间/心跳/测速、热修、重连摘要、空公会板、引导推进/上传、取名/语音、教学进入/加载/固定阵容。另有仅10001的首回合启动定时下行；do_command及完整战斗、库存、任务等仍缺。最新逐项结果见SERVER.md/PROGRESS.md；静态、模拟和实机必须分别表述。

4555A786.utils/assemble.py将同名组件函数组为顺序调用，不能按普通Python覆盖规则解释shadow_battle。add_fighting_cards会初始化battle_base玩家字典，再执行客户端控制器录像逻辑。AFDD4FE7.revert_arg按__custom_type标记恢复对象；空列表同样须带类型。18:08出战数据及首回合已实机通过；不代表服务器已实现伤害和技能结算。

复核使用 neox_dis.py、export_client_catalogs.py、verify_client_catalogs.py，不执行恢复的客户端代码。字段语义、类型转换、方向与状态条件应结合真实调用点继续完善。
# 2026-10-07 客户端原生战斗结算补充

`do_command("__battle_event__", [envelope])` 是现行客户端战斗桥接口。`envelope` 的字段为 `battle_uuid`（服务端会话 UUID）、`sequence`（从 1 开始连续整数）、`kind` 和 `data` 字典。服务端接受 `ready/started/settings/turn/input/command/skill_targets/snapshot/round_end/story/result/error`；桥握手前只允许 `ready`，`started` 必须带合法 `units` 快照。`result.data` 至少包含 `winner_eids`，当前 revision 12 同时发送 `player_eid` 与 `outcome`，可选 `finished_task_list`。

服务端记录最近 128 条事件到 `avatar_progress.server_battle.event_log`，并按会话 UUID、序号、快照结构和单事件 512 KiB 上限校验。结果只依据客户端显式胜负和服务端副本目录计算奖励，客户端不能上传奖励盒或材料数量；结算回包 `battle_result(win, bonus, assist, extra)` 的 `extra` 标记 `client_authoritative=true`、`verified=false`。`finished_dungeon_event` 仅为无回包上报，不是奖励凭据。
