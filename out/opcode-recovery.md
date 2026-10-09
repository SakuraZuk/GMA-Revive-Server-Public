# NeoX 定制 Python 2.7.3 opcode 映射恢复(2026-10-05)

## 指令格式(PyEval_EvalFrameEx 实测)

来源:`prototype/neteasehsqsl/hsqsl.exe` 导出函数 `PyEval_EvalFrameEx`,RVA `0x8e13a0`,主分发点 VA `0xce16d7`:

```
movzx ecx,[edx]        ; op
cmp ecx,0x5a
jb  no_arg             ; op < 90 → 1 字节
; op >= 90 → 3 字节:op + 16 位小端操作数
lea eax,[ecx-3]        ; switch: opcode 值域 3..173
movzx eax,[eax+0xce3eec]   ; range 表(压缩索引)
jmp [eax*4+0xce3d40]   ; 主跳转表
```

- **op < 90:1 字节无参数;op ≥ 90:3 字节,op + arg16(小端)**
- opcode 稀疏编码,值域 3..173,大量值未实现(default @`0xce394b`)
- EXE 内嵌 Python 2.7.3 NeoX 定制(路径串 `...\engine\python27\Objects\stringobject.c`)

## 已确认映射(handler 反汇编 + 全量结构统计双重验证)

### 无参区(op<90)
| op | 名称 | 证据 |
|---|---|---|
| 3 | ROT_THREE | 模式推断 |
| 7 | POP_TOP | import 尾部/调用丢弃返回值;freq 9 万 |
| 8 | STORE_MAP | handler 0xce3039 调 PyDict_SetItem；弹 key/value，保留 dict |
| 10 | BINARY_SUBSCR | role_server_item 中 avatar['nickname'] 与 data.head_box_info[avatar['head_box_id']] 的栈序 |
| 33 | BUILD_CLASS | rpc.py 模块体 `MAKE_FUNCTION; CALL; BUILD_CLASS; STORE_NAME` |
| 36 | BINARY_MOD | '%s' % t 格式化 |
| 42/43 | PROFILER_POP/PUSH(名称待定) | 每个实体方法首尾固定埋点 `LOAD_CONST 'funcname'; op43; LOAD self; LOAD_ATTR uid; op43; op42` |
| 45 | END_FINALLY | except 块收尾,freq≈except 数 |
| 50 | LOAD_LOCALS | 类体末尾 `LOAD_LOCALS; RETURN_VALUE` |
| 65 | STORE_SUBSCR | `client_info['hotfix_index'] = v` 三连 |
| 67 | UNARY_NEGATIVE | handler 0xce1d85 调 PyNumber_Negative；time_utils.reset 取负 |
| 66 | DUP_TOP | except 匹配前复制异常元组 |
| 70 | BINARY_ADD | `'.' + name` 字符串拼接 |
| 72 | POP_BLOCK | for/with 结束,freq≈SETUP_LOOP |
| 81 | UNARY_NOT | `self['today_first_login'] = not check_same_day(...)` |
| 83 | RETURN_VALUE | **全部 5634 个 code 对象末字节 100%=83**;handler 弹栈+why=8 |
| 93 | RAISE_VARARGS | assert/raise;handler 目标 PyErr 设置 |

### 有参区(op≥90)
| op | 名称 | 证据 |
|---|---|---|
| 90 | BUILD_LIST | handler `PyList_New(oparg)`+逐项填栈 |
| 94 | 相对跳转(SETUP_LOOP 类) | 29/29 目标对齐 i+3+arg |
| 95 | DELETE_SUBSCR | SetItem(container,name,v) 第三变体 |
| 99 | BUILD_SET | handler 0xce2fa6 调 PySet_New/PySet_Add，oparg 为元素数 |
| 96 | LOAD_ATTR | handler `PyObject_GetAttr(v,name)`;freq 31 万 |
| 97 | LOAD_FAST | handler localsplus[oparg]+判空+INCREF+PUSH;var:100% |
| 100 | MAKE_FUNCTION | `make_function(f_globals, code)`;def 语句模式;defaults=arg |
| 101 | LOAD_NAME | 错误串 `'no locals when loading %s'`(ceval.c 原文);freq 205 万 |
| 103 | BUILD_LIST(变体) | freq 1376 |
| 104 | STORE_FAST | handler localsplus[oparg]=POP()+DECREF;var:100% |
| 105 | GET_ITER(变体) | handler 调 PyObject_GetIter |
| 110/128 | SETUP_FINALLY/SETUP_EXCEPT(组) | push_block 类调用 0xd07ab0/0xce3334;rel 目标 |
| 111/113 | STORE_GLOBAL | `PyObject_SetItem(f_globals,name,v)` 系 |
| 114 | COMPARE_OP | handler 内 `cmp ebx,9` + PyCmp 十路跳转表 @0xce3f98;arg=0..10 标准 2.7 序:LT,LE,EQ,NE,GT,GE,**IN=6,NOT_IN=7,IS=8,IS_NOT=9**,EXC_MATCH=10(login-flow 实例验证:6=in、7=not in、10=异常匹配) |
| 116 | STORE_NAME | names[oparg]+f_locals 判 dict(type 0x2a83798)快速 PyDict_SetItem |
| 120 | LOAD_DEREF(基址版) | [ebp-0x38] cell 数组直接读取;MAKE_CLOSURE 前构 cell 元组 |
| 125 | UNPACK_SEQUENCE | 元组参数 `lambda (a,b,c,d):` 模式,arg=数量 |
| 129 | LOAD_DEREF(带检查版) | free:100%;闭包工厂内读捕获变量 |
| 131 | CALL_FUNCTION | handler `call_function(&sp,oparg)`;位置参数数=arg & 255，关键字参数数=arg >> 8（低/高 8 位）；257=1pos+1kw；freq 40 万 |
| 132 | STORE_ATTR | handler 0xce2b84 调 PyObject_SetAttr；Record.__init__ 参数映射 |
| 134 | IMPORT_NAME | builtins 查 `__import__`(frame+0x14) |
| 135 | BUILD_TUPLE | handler `PyTuple_New(oparg)`+栈填充;freq 30 万 |
| 136 | MAKE_CLOSURE | LOAD_CLOSURE×n + BUILD_TUPLE + MAKE_CLOSURE 模式;free:91% |
| 137 | JUMP_ABSOLUTE | 目标=arg(绝对);rpc_method 循环回跳验证 |
| 140 | SETUP_WITH(疑似) | freq 71 |
| 141/142 | CALL_FUNCTION_KW/VAR_KW(待精确配对) | |
| 145 | IMPORT_FROM | from X import y 逐名导入 |
| 148 | POP_JUMP_IF_FALSE | int/str 快速真值,目标=arg 绝对;`if not log_rpc:` 模式 |
| 149 | FOR_ITER | handler 调 tp_iternext(type+0x70) |
| 151 | BUILD_MAP | `PyDict_NewPresized(oparg)`;dict 字面量 6 键验证 |
| 153 | LOAD_CONST | handler `consts[oparg]`(ob_item+0xc)+INCREF+PUSH;const:100%,freq 229 万 |
| 155 | LOAD_GLOBAL | names[oparg] str 哈希(0x2a7a4c0=str)+f_globals 查找;不弹栈 |
| 156 | JUMP_FORWARD | 目标=i+3+arg |
| 158 | SETUP_LOOP | push_block;目标 rel |
| 159 | IMPORT_FROM(变体) | freq 730 |
| 160 | EXTENDED_ARG | handler:`arg16<<16` 合并下一条 |
| 172 | POP_JUMP_IF_TRUE(NOT_NONE 快速) | int/None 检查;`if _logger is not None` 懒初始化;目标绝对 |
| 173 | MAKE_CLOSURE(变体)/BUILD_SLICE | freq 2083 待定 |

### 跳转寻址
- 114/131/135 等:非跳转
- 绝对目标:137、148、172
- 相对目标(i+3+arg):94、156;SETUP 类(110/128/140/158)记录块起点

2026-10-05 工具纠错：7=POP_TOP，8=STORE_MAP（旧 ROT_TWO 注记撤回），67=UNARY_NEGATIVE，99=BUILD_SET，132=STORE_ATTR（旧 STORE_SUBSCR 注记撤回）。原生证据见 `out/dis/store-opcode-evidence.asm`，运行 `python out/tools/verify_store_opcodes.py` 可复核。MAKE_FUNCTION 先弹 code 再弹默认参数；EXTENDED_ARG 合并下一条高位。反汇编器先完整解码 UTF-8 再按字符截断，47 份样本按 Android 补丁优先刷新，避免旧按字节截断的乱码。703 个数据模块静态恢复与来源校验通过，但不是完整源码反编译。172 的 None 快速路径等未确认细节继续保留限制。

## 全量自洽验证

`opcode_stats.py` 对 android_base 3099 文件/55766 code 对象按宽度规则解析:
**truncated=0**,全部指令流精确消费到末尾,越界率为 0。同一张表适用于 android_patch(热修 login UI 315EAFEB)与 pc_base(同哈希 B9F5C696 Account.py)。

## 工具

- `out/tools/neox_dis.py <marshal>` — 反汇编器,带 consts/names/varnames 注记
- `out/tools/extract_switch.py` — EXE 跳转表提取
- `out/tools/opcode_stats.py` — 全量结构统计
- `out/tools/op2handler.json` / `handler_features.json` — opcode→handler 映射与特征

## 未竟事项

- 无参区仍有低频项未逐一命名(UNARY_*/BINARY_*/INPLACE_* /SLICE 系)，已确认的 10/57/60 不再列为未知；需要具体方法时逐项核验。
- op42/43(profiler 埋点)与 op141/142 的 KW/VAR 精确配对可从 handler 特征补齐
- ndis 输出已足够人工还原业务逻辑;如需 xdis/uncompyle6 级反编译,需按本表重写字节码再喂反编译器(映射回标准 2.7 opcode 即可,格式差异:3 字节有参指令需改写)
