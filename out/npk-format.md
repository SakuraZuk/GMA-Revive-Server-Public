# NXPK 包格式与脚本恢复结论（v4，2026-10-05）

当前有效结论：外层是标准 LZ4 block，原始存储块直接解压；内层为 rotor → zlib → 前 99 字节 XOR 233 → 去首尾字节 → marshal。三包全部恢复为完整 code 对象或构建清单。v1 固定密钥流、v2 全取反、LZ4 变体和解压前流层解密均撤回。

## 索引格式

| 位置 | 类型 | 当前含义 |
|---|---|---|
| 头部 +0 | 4 字节 | `NXPK` |
| 头部 +4 | u32 LE | 条目数：Android 基础包 3100、热修包 12、当前 PC 包 3031 |
| 头部 +20 | u32 LE | 索引起点 |
| 条目 +0 | u32 LE | 哈希 |
| 条目 +4 | u32 LE | 存储块偏移 |
| 条目 +8 | u32 LE | `stored_size`：存储长度 |
| 条目 +12 | u32 LE | `decoded_size`：解压后长度 |
| 条目 +16 | u64 LE | 哈希辅助值，进一步语义未确认 |
| 条目 +24 | u32 LE | 压缩类型，三个 script.npk 样本均为 2（LZ4） |

条目固定为 28 字节。旧脚本把 +8 命名为 size、+12 命名为 csize，语义与此前文档相反。新工具统一使用上表名称。

当前 PC 包的索引后有 26 个被索引引用的追加块，因此不能按 `(文件大小-索引起点)/28` 计算条目数。读取头部计数，并验证存储块不与索引区重叠、块尾不超过文件尾。索引后的块并不一定非法。

## 地址基准与调用

PE 映像基址为 `0x400000`。旧 `exe_xref_scan.py` 的代码地址漏加基址；数据立即数本来就是 VA，不能统一再加一次。

| 对象 | RVA | VA |
|---|---|---|
| NPK 读取函数入口 | `0x7f656a` | `0xbf656a` |
| LZ4 调用指令 | `0x7f69a3` | `0xbf69a3` |
| LZ4 解码器 | `0xf09310` | `0x1309310` |
| zlib wrapper | `0xed6a00` | `0x12d6a00` |
| 压缩类型跳转表 | `0x7f6adc` | `0xbf6adc` |

LZ4 的 cdecl 签名为 `f(src, dst, stored_size, decoded_size)`，成功返回实际解压长度。调用点以 +8 作为源长、+12 作为输出容量，并把返回值与 +12 比较。类型表把 1 分配至 zlib、2 分配至 LZ4；其他类型不在本次样本验证范围。

Unicorn 必须实际到达返回哨兵，不能把超时后的 EAX 当返回值；禁止自动映射未知页面掩盖错误。

## 为什么旧模型错误

原始 `F0 FF…终值` 就是标准 LZ4 token 与字面量长度扩展。例如 `F0 FF 5E` 表示字面量长度 `15+255+94=364`，其后 `19 73` 是负载的前两字节。

取反得到 `0F 00… E6 8C` 只是将合法 LZ4 流变成无效数据，不能证明存在加密容器。`F1` 块也属于标准 LZ4，低四位用于匹配长度，不能当作明文分支标记。

## 全量复现结果

| 输入 | 解压成功 | 解压负载前缀分布 | 与 PC 原函数逐字节比对 |
|---|---|---|---|
| `work/apk/assets/script.npk` | 3100/3100 | `1973` 3098；`443a` 1；`6300` 1 | 4 个样本一致 |
| `files/netease/h62/Documents/script.npk` | 12/12 | `1973` 12 | 3 个样本一致 |
| `prototype/neteasehsqsl/script.npk` | 3031/3031 | `1973` 3003；`19e1` 26；`443a` 1；`6300` 1 | 5 个样本一致 |

比对覆盖最小/中间/最大存储块、`F1` 块和 PC 索引后追加块。输入文件、EXE 和输出样本 SHA-256 都在 `out/npk_decoded/verification.json`；每个输出目录还有逐条 `manifest.json`。两个实现均输出同一字节流，但本轮没有运行真实客户端验收。

## 工具与运行（Windows CMD）

```bat
set PYTHONIOENCODING=utf-8
python out\tools\npk_unpack.py work\apk\assets\script.npk out\npk_decoded\android_base
python out\tools\npk_unpack.py files\netease\h62\Documents\script.npk out\npk_decoded\android_patch
python out\tools\npk_unpack.py prototype\neteasehsqsl\script.npk out\npk_decoded\pc_base
python out\tools\test_npk_unpack.py
python out\tools\verify_npk.py
```

解包器仅依赖 Python 标准库；原函数验证还需要 Unicorn 和 Capstone，以及指定的 PC EXE。5 个测试覆盖字面量、扩展长度、重叠复制、末尾 token 和截断/越界输入。

## 脚本层已恢复：直接代码证据与全量校验

唯一外层直接可解析的脚本 `F416002A.bin` 是 `redirect`，完整对象树见 `out/npk_decoded/redirect.json`。`NpkImporter.load_module` 明确引用 `my_rotor.decrypt`、`zlib.decompress`、`script_decrypt`、`marshal.loads`。

| C_file 方法 | 当前 PC EXE 绑定函数 VA | 返回内容来源 |
|---|---|---|
| get_rotor_encrypt_key | `0xa7fcd0` | 明文字符串 VA `0x1bbcda8` |
| get_rs_encrypt_key | `0xa7fce0` | 两个整数常量 `0xe9=233`、`0x63=99` |
| get_rs_encrypt_funtion | `0xa7fd10` | `_decrypt` 源码字符串 VA `0x1bbced8` |

完整密钥、原生绑定表、函数汇编和内嵌源码分别保存于 `script_loader_bindings.json`/`.asm`。Android libclient.so 中找到完全相同的密钥字符串（文件偏移 `0x21e509b`）和源码字符串（`0x21e51c5`），且三包使用同一参数均恢复成功。不能根据邻近字符串就认定密钥；例如另一 AI 提到的 `w5q6^C04SW!@e}ad` 未形成上述绑定与成功解码证据。

内嵌 `_decrypt` 源码要求：对 zlib 输出的前 99 字节逐字节 XOR 233，其他字节保留，再取 `[1:-1]`。工具移植其语义，**不执行**内嵌源码或游戏字节码。

rotor 实现参考 CPython v2.3.7 的 `Modules/rotormodule.c`，原文件及完整版权许可保存在 `work/rotormodule_reference.c`。`out/tools/rotor_compat.py` 移植转子初始化、C 有符号除法、字节溢出和每次 decrypt 重置语义。它是参考实现，真正的兼容性证据是全部 zlib 校验和及完整 marshal 消费。

旧 Loader 的长整数 limb 从错误的 4 字节改为正确的 2 字节/15 位；文本浮点改为 u8 长度前缀；修正 int64 符号。4 个独立标量测试通过。

| 恢复目录 | 完整 code 对象 | 构建清单 | 失败 |
|---|---|---|---|
| `out/npk_scripts/android_base` | 3099 | 1 | 0 |
| `out/npk_scripts/android_patch` | 12 | 0 | 0 |
| `out/npk_scripts/pc_base` | 3030 | 1 | 0 |

每个 `.marshal` 都已完整解析到 EOF，顶层必须为 code；清单包含原始模块名、方法参数、引用名、字节码及 SHA-256。路径 `4176DE2A` 为构建清单，不当作 code。

```bat
python out\tools\inspect_script_loader.py
python out\tools\npk_script_decode.py android_base
python out\tools\npk_script_decode.py android_patch
python out\tools\npk_script_decode.py pc_base
python out\tools\test_marshal_loader.py
python out\tools\build_script_inventory.py
```

`out/npk_scripts/android_inventory.json` 合并基础包与按哈希覆盖的 12 条热修：3099 个模块、55766 个嵌套 code 对象。这里只是方法元数据清单，不能把数量称为 RPC/API 数量。

## 下一步与明确边界

资源解压与内层解码完成；后续已恢复主要 NeoX opcode、RPC 注册、登录与热修指令级语义，见 `opcode-recovery.md`、`rpc-semantics.md` 和 `login-flow-spec.md`。完整自动源码反编译尚未完成，少数指令仍有问号，不能直接套用标准 Python 2.7 指令表。Go 框架实现及真实传输边界见根目录 `SERVER.md`。

优先读取模块：`entities/Account.py`（B9F5C696）、`entities/Avatar.py`（2034D84C）、`entities/components/login.py`（8A6BFA34）、`engine/common/rpc.py`（D7B7FC3C）、`mbengine/common/rpcdecorator.py`（1F09877D）、`guis/login/login.py`（315EAFEB，热修覆盖）。

旧统计/猜密钥脚本只保留作实验过程；作废的 keystream.bin 已移除，不能复用于正式服务端。真实客户端业务登录、2H4G 压测和生产部署仍未在本轮验收。
