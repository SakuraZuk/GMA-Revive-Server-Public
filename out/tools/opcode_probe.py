# -*- coding: utf-8 -*-
"""验证假设:NeoX 脚字节码是否就是标准 CPython 2.7 opcode。

方法:用标准 2.7 opcode 表反汇编若干 marshal code 对象,
统计非法操作数(索引越界、跳转落在指令边界外)比例。
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from mem_marshal_extract import Loader

# 标准 CPython 2.7 opcode(公开定义,opcode.h)
OPNAME = {}
def _d(op, name): OPNAME[op] = name
_std = """STOP_CODE0 POP_TOP1 ROT_TWO2 ROT_THREE3 DUP_TOP4 ROT_FOUR5
NOP9 UNARY_POSITIVE10 UNARY_NEGATIVE11 UNARY_NOT12 UNARY_CONVERT13
UNARY_INVERT15 BINARY_POWER19 GET_ITER68
BINARY_MULTIPLY20 BINARY_DIVIDE21 BINARY_MODULO22 BINARY_ADD23
BINARY_SUBTRACT24 BINARY_SUBSCR25 BINARY_FLOOR_DIVIDE26 BINARY_TRUE_DIVIDE27
INPLACE_FLOOR_DIVIDE28 INPLACE_TRUE_DIVIDE29 SLICE+0 30 SLICE+1 31
SLICE+2 32 SLICE+3 33 STORE_SLICE+0 40 STORE_SLICE+1 41
STORE_SLICE+2 42 STORE_SLICE+3 43 INPLACE_ADD55 INPLACE_SUBTRACT56
INPLACE_MULTIPLY57 INPLACE_DIVIDE58 INPLACE_MODULO59 STORE_SUBSCR60
DELETE_SUBSCR61 BINARY_LSHIFT62 BINARY_RSHIFT63 BINARY_AND64 BINARY_XOR65
BINARY_OR66 INPLACE_POWER67 GET_ITER68 PRINT_ITEM71 PRINT_NEWLINE72
INPLACE_LSHIFT75 INPLACE_RSHIFT76 INPLACE_AND77 INPLACE_XOR78
INPLACE_OR79 BREAK_LOOP80 WITH_CLEANUP81 RETURN_VALUE82 IMPORT_STAR83
EXEC_STMT85 YIELD_VALUE86 POP_BLOCK87 END_FINALLY88 BUILD_CLASS89"""
for tok in _std.replace('\n',' ').split():
    if tok[0].isalpha():
        name=''.join(ch for ch in tok if ch.isalpha() or ch in '+')
        num=int(''.join(ch for ch in tok if ch.isdigit()) or 0)
        _d(num,name)

_have=90
_named = {
 90:'STORE_NAME',91:'DELETE_NAME',92:'UNPACK_SEQUENCE',93:'FOR_ITER',
 94:'LIST_APPEND',95:'STORE_ATTR',96:'DELETE_ATTR',97:'STORE_GLOBAL',
 98:'DELETE_GLOBAL',100:'LOAD_CONST',101:'LOAD_NAME',102:'BUILD_TUPLE',
 103:'BUILD_LIST',104:'BUILD_SET',105:'BUILD_MAP',106:'LOAD_ATTR',
 107:'COMPARE_OP',108:'IMPORT_NAME',109:'IMPORT_FROM',110:'JUMP_FORWARD',
 111:'JUMP_IF_FALSE_OR_POP',112:'JUMP_IF_TRUE_OR_POP',113:'JUMP_ABSOLUTE',
 114:'POP_JUMP_IF_FALSE',115:'POP_JUMP_IF_TRUE',116:'LOAD_GLOBAL',
 119:'CONTINUE_LOOP',122:'SETUP_LOOP',120:'SETUP_EXCEPT',121:'SETUP_FINALLY',
 124:'LOAD_FAST',125:'STORE_FAST',126:'DELETE_FAST',130:'RAISE_VARARGS',
 131:'CALL_FUNCTION',132:'MAKE_FUNCTION',133:'BUILD_SLICE',134:'MAKE_CLOSURE',
 135:'LOAD_CLOSURE',136:'LOAD_DEREF',137:'STORE_DEREF',138:'CALL_FUNCTION_VAR',
 141:'CALL_FUNCTION_KW',140:'CALL_FUNCTION_VAR_KW',142:'SETUP_WITH',
 143:'EXTENDED_ARG',145:'LIST_APPEND_OLD'}
for k,v in _named.items(): _d(k,v)

HASJREL={93,110,111,112,114,115,120,121,122,119,142}
HASJABS={113,111,112,114,115,119}
HASNAME={90,91,95,96,97,98,101,106,108,109,116}
HASLOCAL={124,125,126}
HASCONST={100}
HASFREE={135,136,137,134}


def analyze(code, path='<top>'):
    """返回 (指令数, 非法操作数数, 非法列表)"""
    bc = code['bytecode']
    n = len(bc)
    consts = code['consts']
    names = code['names']
    varnames = code['varnames']
    freevars = code.get('freevars') or []
    instrs = 0
    bad = []
    ext = 0
    i = 0
    targets = set()
    while i < n:
        op = bc[i]
        if op == 143:
            ext = (ext | bc[i+1]) << 8
            i += 2
            continue
        arg = bc[i+1] | ext
        ext = 0
        name = OPNAME.get(op)
        if name is None:
            bad.append((path, i, '未知opcode %d' % op))
            i += 2
            continue
        instrs += 1
        # 边界检查
        if op in HASCONST and arg >= len(consts):
            bad.append((path, i, '%s arg=%d 越界 consts=%d' % (name, arg, len(consts))))
        if op in HASNAME and arg >= len(names):
            bad.append((path, i, '%s arg=%d 越界 names=%d' % (name, arg, len(names))))
        if op in HASLOCAL and arg >= len(varnames):
            bad.append((path, i, '%s arg=%d 越界 varnames=%d' % (name, arg, len(varnames))))
        if op in HASFREE and arg >= len(freevars):
            bad.append((path, i, '%s arg=%d 越界 freevars=%d' % (name, arg, len(freevars))))
        if op in (131,138,141,140):  # CALL_FUNCTION: arg 分低/高字节,高字节为 kw 数,不可能超 nargs+kw 总量太多
            kwcnt = arg >> 8
            if kwcnt > 50:
                bad.append((path, i, 'CALL_FUNCTION kw=%d 可疑' % kwcnt))
        i += 2
        if op in HASJREL or op in (110,111,112,114,115,120,121,122,93,142):
            targets.add((i + arg) if op in HASJREL else arg)
        else:
            targets.add(arg if op in HASJABS else None)
    # 跳转目标必须落在指令边界
    return instrs, bad, bc, names, consts, varnames


def walk(code, prefix=''):
    name = code['name']
    full = prefix + '/' + (name if isinstance(name, str) else name.decode('utf8', 'replace'))
    yield full, code
    for c in code['consts']:
        if isinstance(c, dict) and c.get('type') == 'code':
            yield from walk(c, full)


def main():
    total_i = 0
    total_bad = 0
    bad_samples = []
    files = 0
    p = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(r'.\out\npk_scripts\android_base')
    marshal_files = sorted(p.glob('*.marshal'))[:150]
    for mf in marshal_files:
        try:
            code = Loader(mf.read_bytes()).r_object()
        except Exception as exc:
            continue
        files += 1
        for path, c in walk(code):
            instrs, bad, *_ = analyze(c, path)
            total_i += instrs
            total_bad += len(bad)
            bad_samples.extend(bad[:3])
    print('文件=%d 指令=%d 非法=%d (%.4f%%)' % (files, total_i, total_bad, 100.0*total_bad/max(total_i,1)))
    for b in bad_samples[:30]:
        print(' ', b)

if __name__ == '__main__':
    main()
