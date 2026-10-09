# -*- coding: utf-8 -*-
"""NeoX 定制 Python2.7 opcode 反汇编器。
指令格式(源自 hsqsl.exe PyEval_EvalFrameEx @VA 0xce169e 实测):
  op < 90 : 1 字节,无参数
  op >= 90: 3 字节,op + 16 位小端操作数
opcode 映射:handler 反汇编 + 全量结构统计双重确认,见 opcode-recovery.md。
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from mem_marshal_extract import Loader

OPS = {
    3: 'ROT_THREE', 6: 'ROT_TWO_ALT', 7: 'POP_TOP', 8: 'STORE_MAP',
    10: 'BINARY_SUBSCR', 45: 'END_FINALLY', 57: 'BINARY_SUBTRACT',
    60: 'GET_ITER', 65: 'STORE_SUBSCR', 66: 'DUP_TOP', 67: 'UNARY_NEGATIVE', 72: 'POP_BLOCK',
    81: 'UNARY_NOT',
    33: 'BUILD_CLASS', 36: 'BINARY_MOD', 50: 'LOAD_LOCALS', 70: 'BINARY_ADD',
    52: 'NOP', 83: 'RETURN_VALUE',
    90: 'BUILD_LIST', 94: 'REL_JUMP?', 95: 'DELETE_SUBSCR', 96: 'LOAD_ATTR',
    97: 'LOAD_FAST', 99: 'BUILD_SET', 100: 'MAKE_FUNCTION',
    101: 'LOAD_NAME', 102: 'DEREF?', 103: 'BUILD_LIST?', 104: 'STORE_FAST',
    105: 'GET_ITER?', 110: 'SETUP_FINALLY', 111: 'STORE_GLOBAL',
    113: 'STORE_GLOBAL', 114: 'COMPARE_OP', 116: 'STORE_NAME',
    118: 'DELETE_NAME?', 120: 'LOAD_DEREF', 125: 'UNPACK_SEQUENCE',
    128: 'SETUP_EXCEPT', 129: 'LOAD_DEREF', 131: 'CALL_FUNCTION',
    132: 'STORE_ATTR', 134: 'IMPORT_NAME', 135: 'BUILD_TUPLE',
    136: 'MAKE_CLOSURE', 137: 'JUMP_ABS', 138: 'CALL_*?',
    145: 'IMPORT_FROM', 148: 'POP_JUMP_IF_FALSE', 149: 'FOR_ITER',
    151: 'BUILD_MAP', 153: 'LOAD_CONST', 155: 'LOAD_GLOBAL',
    156: 'JUMP_FORWARD', 157: 'BUILD_SLICE3?', 158: 'SETUP_LOOP',
    159: 'IMPORT_FROM2?', 160: 'EXTENDED_ARG', 172: 'POP_JUMP_IF_TRUE',
    173: 'MAKE_CLOSURE2?',
}
NOPS = {3, 6, 7, 8, 9, 10, 11, 13, 14, 15, 16, 17, 19, 21, 22, 23, 24, 25, 26, 27,
        30, 31, 32, 33, 34, 35, 36, 37, 39, 42, 43, 44, 45, 46, 47, 48, 49, 50,
        51, 53, 54, 55, 57, 58, 60, 63, 64, 65, 66, 67, 70, 71, 72, 74, 77, 78,
        81, 82, 84, 85, 86, 87, 89}

JREL = {94, 110, 128, 149, 156, 158}
JABS = {137, 148, 172}


def disasm(c, prefix='', out=sys.stdout):
    name = c['name'].decode('utf8', 'replace') if isinstance(c['name'], bytes) else c['name']
    fn = c['filename'].decode('utf8', 'replace') if isinstance(c['filename'], bytes) else c['filename']
    full = prefix + '/' + name if prefix else name
    print('code %s  (file=%s argc=%d vars=%s)' % (full, fn, c['argcount'],
          [v.decode('utf8', 'replace') if isinstance(v, bytes) else v for v in c['varnames']]), file=out)
    b = c['bytecode']; n = len(b)
    consts = c['consts']; names = c['names']
    def sval(v):
        if isinstance(v, dict) and v.get('type') == 'code':
            return '<code %s>' % (v['name'].decode('utf8', 'replace') if isinstance(v['name'], bytes) else v['name'])
        if isinstance(v, bytes):
            # 先完整解码再按字符截断，避免截断中文 UTF-8 字节后写入乱码。
            r = v.decode('utf8', 'backslashreplace')
            return repr(r[:40]) + ('...' if len(r) > 40 else '')
        return repr(v)
    i = 0
    extended = 0
    while i < n:
        op = b[i]
        if op < 90:
            print('  %4d  %-3d %-14s' % (i, op, OPS.get(op, 'op%d' % op)), file=out)
            i += 1
        else:
            if i + 3 > n:
                raise ValueError('指令在偏移 %d 截断' % i)
            arg = extended | b[i+1] | (b[i+2] << 8)
            extended = (arg << 16) if op == 160 else 0
            nm = OPS.get(op, 'op%d' % op)
            ann = ''
            if op == 153 and arg < len(consts):
                ann = '  ; ' + sval(consts[arg])
            elif op == 100 and arg:
                ann = '  ; defaults=%d' % arg
            elif (op in (101, 96, 116, 111, 155, 113, 95, 132, 134, 145, 159) and arg < len(names)):
                v = names[arg]
                ann = '  ; ' + (v.decode('utf8', 'replace') if isinstance(v, bytes) else str(v))
            elif op in (97, 104) and arg < len(c['varnames']):
                v = c['varnames'][arg]
                ann = '  ; ' + (v.decode('utf8', 'replace') if isinstance(v, bytes) else str(v))
            tgt = ''
            if op in JREL:
                tgt = '  -> %d' % (i + 3 + arg)
            elif op in JABS:
                tgt = '  -> %d' % arg
            print('  %4d  %-3d %-14s %5d%s%s' % (i, op, nm, arg, tgt, ann), file=out)
            i += 3
    for k in consts:
        if isinstance(k, dict) and k.get('type') == 'code':
            disasm(k, full, out)


if __name__ == '__main__':
    if hasattr(sys.stdout, 'reconfigure'):
        sys.stdout.reconfigure(encoding='utf-8')
        sys.stderr.reconfigure(encoding='utf-8')
    mf = Path(sys.argv[1])
    code = Loader(mf.read_bytes()).r_object()
    disasm(code)
