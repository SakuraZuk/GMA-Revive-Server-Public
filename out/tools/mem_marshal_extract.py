# -*- coding: utf-8 -*-
"""
mem_marshal_extract.py — 从内存 dump 提取 NeoX marshal code 对象
原理：游戏解压后的 marshal 字节流残片留在堆里；marshal 格式可自校验
（r_object 递归解析，完整走完 = 有效对象）。
类型表依据 prototype/marshal_dump_py3.py（Python2.7 marshal + NeoX 扩展）。
"""
import struct
import sys
import re

TYPE_NULL = ord('0')
TYPE_NONE = ord('N')
TYPE_FALSE = ord('F')
TYPE_TRUE = ord('T')
TYPE_STOPITER = ord('S')
TYPE_ELLIPSIS = ord('.')
TYPE_INT = ord('i')
TYPE_INT64 = ord('I')
TYPE_FLOAT = ord('f')
TYPE_COMPLEX = ord('x')
TYPE_BINARY_FLOAT = ord('g')
TYPE_BINARY_COMPLEX = ord('y')
TYPE_LONG = ord('l')
TYPE_STRING = ord('s')
TYPE_INTERNED = ord('t')
TYPE_STRINGREF = ord('R')
TYPE_TUPLE = ord('(')
TYPE_LIST = ord('[')
TYPE_DICT = ord('{')
TYPE_CODE = ord('c')
TYPE_UNICODE = ord('u')
TYPE_ASCII = ord('a')
TYPE_ASCII_INTERNED = ord('A')
TYPE_SET = ord('<')
TYPE_FROZENSET = ord('>')


class MarshalError(Exception):
    pass


class Loader:
    """NeoX/Python2.7 marshal 解析（带 stringref 表）"""

    def __init__(self, data, pos=0):
        self.d = data
        self.p = pos
        self.refs = []          # stringref 表（按出现序）
        self.depth = 0

    def r_byte(self):
        if self.p >= len(self.d):
            raise MarshalError('eof')
        b = self.d[self.p]
        self.p += 1
        return b

    def r_long(self):
        if self.p + 4 > len(self.d):
            raise MarshalError('eof')
        v = struct.unpack_from('<i', self.d, self.p)[0]
        self.p += 4
        return v

    def r_object(self):
        self.depth += 1
        if self.depth > 400:
            raise MarshalError('too deep')
        try:
            t = self.r_byte()
            if t == TYPE_NULL:
                return ('NULL',)
            if t == TYPE_NONE:
                return None
            if t == TYPE_FALSE:
                return False
            if t == TYPE_TRUE:
                return True
            if t == TYPE_STOPITER:
                return ('STOPITER',)
            if t == TYPE_ELLIPSIS:
                return Ellipsis
            if t == TYPE_INT:
                return self.r_long()
            if t == TYPE_INT64:
                lo = self.r_long()
                hi = self.r_long()
                value = ((hi & 0xFFFFFFFF) << 32) | (lo & 0xFFFFFFFF)
                return value-(1<<64) if value & (1<<63) else value
            if t in (TYPE_FLOAT, TYPE_BINARY_FLOAT):
                if t == TYPE_FLOAT:
                    # Python 2 marshal 文本浮点为 u8 长度 + ASCII，不是换行终止。
                    n = self.r_byte()
                    if self.p+n > len(self.d):
                        raise MarshalError('文本浮点被截断')
                    s = self.d[self.p:self.p+n]
                    self.p += n
                    return float(s)
                if self.p + 8 > len(self.d):
                    raise MarshalError('eof')
                v = struct.unpack_from('<d', self.d, self.p)[0]
                self.p += 8
                return v
            if t in (TYPE_COMPLEX, TYPE_BINARY_COMPLEX):
                if t == TYPE_BINARY_COMPLEX:
                    self.p += 16
                    return 0j
                raise MarshalError('complex-text')
            if t == TYPE_LONG:
                n = self.r_long()
                if abs(n) > 1000 or self.p+abs(n)*2 > len(self.d):
                    raise MarshalError('bad long')
                v = 0
                # 长整数的每个 limb 是 2 字节，只有低 15 位有效；n 的符号表示正负。
                for i in range(abs(n)):
                    limb = struct.unpack_from('<H',self.d,self.p)[0]
                    self.p += 2
                    if limb >= (1<<15):
                        raise MarshalError('非法长整数 limb')
                    v |= limb << (15*i)
                return -v if n<0 else v
            if t in (TYPE_STRING, TYPE_INTERNED):
                n = self.r_long()
                if n < 0 or self.p + n > len(self.d):
                    raise MarshalError('bad str len')
                s = self.d[self.p:self.p + n]
                self.p += n
                if t == TYPE_INTERNED:
                    self.refs.append(s)
                return s
            if t == TYPE_STRINGREF:
                idx = self.r_long()
                if idx < 0 or idx >= len(self.refs):
                    raise MarshalError('bad stringref')
                return self.refs[idx]
            if t == TYPE_UNICODE:
                n = self.r_long()
                if n < 0 or self.p + n > len(self.d):
                    raise MarshalError('bad uni')
                s = self.d[self.p:self.p + n]
                self.p += n
                return s.decode('utf-8', 'replace')
            if t in (TYPE_ASCII, TYPE_ASCII_INTERNED):
                n = self.r_long()
                if n < 0 or self.p + n > len(self.d):
                    raise MarshalError('bad ascii')
                s = self.d[self.p:self.p + n]
                self.p += n
                if t == TYPE_ASCII_INTERNED:
                    self.refs.append(s)
                return s.decode('ascii', 'replace')
            if t in (TYPE_TUPLE, TYPE_LIST, TYPE_SET, TYPE_FROZENSET):
                n = self.r_long()
                if n < 0 or n > 1 << 20:
                    raise MarshalError('bad seq len')
                items = tuple(self.r_object() for _ in range(n))
                if t == TYPE_LIST:
                    return list(items)
                if t == TYPE_SET:
                    return set(items[:8]) if len(items) < 16 else None
                if t == TYPE_FROZENSET:
                    return frozenset(items[:8]) if len(items) < 16 else None
                return items
            if t == TYPE_DICT:
                out = {}
                while True:
                    k = self.r_object()
                    if isinstance(k, tuple) and k == ('NULL',):
                        return out
                    v = self.r_object()
                    if isinstance(v, tuple) and v == ('NULL',):
                        return out
                    out[k if not isinstance(k, tuple) else str(k)] = v
                    if len(out) > 1 << 20:
                        raise MarshalError('dict too big')
            if t == TYPE_CODE:
                return self.r_code()
            raise MarshalError('bad type 0x%02x' % t)
        finally:
            self.depth -= 1

    def r_code(self):
        argcount = self.r_long()
        nlocals = self.r_long()
        stacksize = self.r_long()
        flags = self.r_long()
        if not (0 <= argcount <= 1000 and 0 <= nlocals <= 1000
                and 0 < stacksize <= 100000 and 0 <= flags <= 0xFFFF | 0x10000):
            raise MarshalError('bad code hdr')
        code = self.r_object()
        if not isinstance(code, bytes):
            raise MarshalError('code not bytes')
        consts = self.r_object()
        names = self.r_object()
        varnames = self.r_object()
        freevars = self.r_object()
        cellvars = self.r_object()
        filename = self.r_object()
        name = self.r_object()
        firstlineno = self.r_long()
        lnotab = self.r_object()
        return {
            'type': 'code', 'argcount': argcount, 'nlocals': nlocals,
            'stacksize': stacksize, 'flags': flags, 'bytecode': code,
            'consts': consts, 'names': names, 'varnames': varnames,
            'freevars': freevars, 'cellvars': cellvars,
            'filename': filename, 'name': name,
            'firstlineno': firstlineno, 'lnotab': lnotab,
        }


HEAD_RE = re.compile(
    b'\x63.\x00\x00\x00.\x00\x00\x00.\x00\x00\x00.\x00\x00\x00', re.DOTALL)


def extract(mempath, outdir):
    import os
    import json
    os.makedirs(outdir, exist_ok=True)
    mem = open(mempath, 'rb').read()
    found = 0
    for m in HEAD_RE.finditer(mem):
        p = m.start()
        ld = Loader(mem, p)
        try:
            obj = ld.r_object()
        except MarshalError:
            continue
        if not (isinstance(obj, dict) and obj.get('type') == 'code'):
            continue
        fn = obj.get('filename')
        nm = obj.get('name')
        consumed = ld.p - p
        if consumed < 64:
            continue
        found += 1
        fname = 'mod_%04d_%s_%s_%dB.bin' % (
            found,
            (fn.decode('utf-8', 'replace') if isinstance(fn, (bytes, str)) else 'x')
            .replace('\\', '_').replace('/', '_')[-40:] if fn else 'x',
            (nm.decode('ascii', 'replace') if isinstance(nm, (bytes, str)) else 'x')[:20] if nm else 'x',
            consumed)
        with open(os.path.join(outdir, fname), 'wb') as f:
            f.write(mem[p:ld.p])
        print('[%d] %s  (file=%r name=%r %dB)' % (found, fname, fn, nm, consumed))
    print('extracted %d marshal code objects from %s' % (found, mempath))


if __name__ == '__main__':
    p = sys.argv[1] if len(sys.argv) > 1 else r'.\work\memdump\mem2_engine.bin'
    od = sys.argv[2] if len(sys.argv) > 2 else r'.\out\mem_marshal'
    extract(p, od)
