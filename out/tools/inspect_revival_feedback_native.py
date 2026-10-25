# -*- coding: utf-8 -*-
"""只读检索停服客户端的反馈相关代码；不读取玩家数据或连接服务器。"""
from __future__ import print_function
import argparse
import dis
import marshal
import os
import types

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '../..'))
BASE = os.path.join(ROOT, 'internal/nativepvp/runtime/native_engine/script')

def constant_strings(values):
    for value in values:
        if isinstance(value, basestring):
            yield value.decode('utf-8', 'replace') if isinstance(value, str) else value
        elif isinstance(value, tuple):
            for item in constant_strings(value):
                yield item

def visit(code, path, trail, terms, show):
    location = trail + '/' + code.co_name
    strings = list(constant_strings(code.co_consts))
    if any(term in code.co_name or any(term in name for name in code.co_names)
           or any(term in value for value in strings) for term in terms):
        print(path, location, 'args=', code.co_varnames[:code.co_argcount])
        if show:
            dis.dis(code)
        else:
            print('names=', code.co_names if len(code.co_names) < 80 else code.co_names[:20])
        print('strings=', [v for v in strings if any(t in v for t in terms)])
    for child in code.co_consts:
        if isinstance(child, types.CodeType):
            visit(child, path, location, terms, show)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('terms', nargs='+')
    parser.add_argument('--module')
    parser.add_argument('--disassemble', action='store_true')
    args = parser.parse_args()
    def decode_term(value):
        if not isinstance(value, str):
            return value
        try:
            return value.decode('utf-8')
        except UnicodeDecodeError:
            return value.decode('mbcs')
    args.terms = [decode_term(v) for v in args.terms]
    for directory, _, names in os.walk(BASE):
        for name in sorted(names):
            path = os.path.join(directory, name)
            relative = os.path.relpath(path, BASE).replace('\\', '/')
            if not name.endswith('.pyc') or (args.module and args.module not in relative):
                continue
            with open(path, 'rb') as source:
                source.read(8)
                code = marshal.load(source)
            visit(code, relative, '', args.terms, args.disassemble)
