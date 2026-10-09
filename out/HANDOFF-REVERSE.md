# 客户端数据、配置与协议逆向交接

> 2026-10-07 文档整理：本文件保留历史取证，日期相关的“当前/下一任务/未实现/未发布”仅对应原记录时点，不能当作现行状态。统一交接见 [HANDOFF.md](HANDOFF.md)，当前施工见 [REPAIR-TASKS.md](REPAIR-TASKS.md)。全部战斗现走客户端原生计算，F2已完成，不再执行旧权威引擎施工或协议闸门改包。


更新：2026-10-05。正式输出为 client_catalogs/；服务器交接为 HANDOFF.md。本轮使用 Android 补丁优先的 3099 模块索引，校验每份 marshal 的 SHA256 并解析至 EOF。数据形态是 NeoX Python 2 定制字节码的常量与 Record 类，并非原生 Lua/JSON 文件。

## 已导出的范围

| 对象 | 结果 | 边界 |
|---|---|---|
| datas 模块 | 703：701 数据定义、2 包初始化 | 全部静态结构通过，255,142 条顶层数据 |
| 运行时表达式 | 1600 处引用 | 保存常量、名称、默认参数、字节码 base64/SHA256，不执行或猜值 |
| 非 UTF-8 原始字节 | 8 处 | 保留 base64 标记，不用替换乱码字符 |
| 配置候选 | 29 模块 | 按 config/conf/const/setting/version 名称筛选；只恢复直接常量，不能声称全部配置 |
| 显式 rpc 装饰方法 | 302 个 | 包括零参数签名、选项与默认值；方向不能只看装饰器默认项 |
| 直接 server_proxy 引用 | 269 处、259 个方法名 | 保存调用位置，不等于每个参数均恢复 |
| 命名 call_server 转发 | 221 处、220 个方法名 | callback_id 首参及运行时转发需要结合实现 |

完整统计及两类上行方法名的合并去重数见 client_catalogs/summary.json。旧“约 3287 个推送”是 on_/sync_/notify_ 前缀统计，包含本地回调；旧 291 方法和 250,782 条导出是早期扫描结果，现以本目录为准，不视为全部协议已完成。

旧 out/datas 与根目录旧 rpc-catalog 导出及旧提取器已删除；删除前校对全部 703 个模块名、模块哈希和新输出覆盖一致，避免后续 AI 误用旧结果。

## 输出格式与入口

- client_catalogs/table-index.json：模块、来源文件及 SHA256、状态、条目数、导出路径。
- client_catalogs/tables/<模块>.json：原始混淆命名空间、Record 字段映射、数据；字段名来自 __init__ 的参数到 STORE_ATTR 映射。
- client_catalogs/rpc-catalog.json：显式装饰方法、上行代理引用、命名转发调用、复杂注册候选。每项保留模块和指令位置，可回到代码复核。
- client_catalogs/config-index.json 与 configs/：配置候选的直接常量及顶层常量树。
- client_catalogs/verification.json：覆盖和数据完整性检查。
- client-catalogs.zip：上述输出与现行逆向规格的可交付副本。

gtext 只保留原始文本；复合/二进制键用带类型的键值条目保存，不能强行转为普通 JSON 字符串键。函数表达式保留证据，不等于其运行结果。id、条件组、字段业务含义须以消费调用点确认，不按短名或元组首项猜测。

## 可重复执行

Windows CMD，当前已生成，无需重复下载或改原包：

```bat
cd /d .
set PYTHONIOENCODING=utf-8
python out\tools\verify_store_opcodes.py
python out\tools\refresh_disassembly.py
python out\tools\export_client_catalogs.py
python out\tools\verify_client_catalogs.py
python out\tools\package_client_catalogs.py
```

export_client_catalogs 是白名单静态解释器，只允许常量、容器、Record 构造、gtext 包装及函数证据；未知调用或控制流失败关闭，不执行客户端 Python。输入索引 out/npk_scripts/android_inventory.json，12 个补丁模块优先于基础包；主解析器 mem_marshal_extract.Loader。

## 原生 opcode 纠错

PC EXE 基址 0x400000；分发表 0xce3eec[op-3]，handler 指针表 0xce3d40[slot*4]。8 的 handler 调 PyDict_SetItem，67 调 PyNumber_Negative，99 调 PySet_New/PySet_Add，132 调 PyObject_SetAttr。现为 STORE_MAP、UNARY_NEGATIVE、BUILD_SET、STORE_ATTR。证据 out/dis/store-opcode-evidence.asm；工具 verify_store_opcodes.py。neox_dis 注释先解码完整 UTF-8 再按字符截断，47 份样本已刷新。

## 后续应补的内容

1. 动态 getattr 和运行时计算方法名，隐式注册及组件组装；目录中“复杂显式注册候选为零”只描述本扫描模式。
2. 自定义类型 ObjId/Callback/Tuple/box 等转换、必选与默认参数、方向和业务状态约束。utils.rpc 的默认 options=server 在客户端可转 CLIENT_STUB，不能统一归为服务端推送。
3. 以真实登录、任务、背包、战斗调用点逐一验证字段语义并接服务逻辑；表导出覆盖不等于玩法实现。
4. 完整自动源码反编译与表达式求值不是本轮结果。原始 APK/EXE/NPK、marshal、常量和字节码仍供复核。

## 2026-10-05 18:12 战斗交接补充

服务端已实机推进教学10001至普通攻击教学，当前卡在do_command未实现；现行模块哈希、调用顺序、源码位置和原始日志见HANDOFF.md第5节。特别核对4555A786同名组件顺序执行和AFDD4FE7自定义类型标记；不能把客户端shadow的空控制方法理解成覆盖了所有共享逻辑。

新增查询工具query_evidence.py的records模式递归查复合键容器，例如`python out/tools/query_evidence.py records skill skill_id 440301 440302 6010701`。skill的键包含技能/等级组/品阶组，普通table模式查440301返回null不证明不存在；需要匹配真实等级/品阶并核对表达式，不能仅取第一个记录。

390玩家属性的当前复核文件为avatar-properties-verified.jsonl；旧avatar-props.json将描述标签误作类型，已删除该错误派生结果，原marshal保留。接手先看取证清单和字段读取点，再补玩家状态，不能把静态表直接当玩家数据。
