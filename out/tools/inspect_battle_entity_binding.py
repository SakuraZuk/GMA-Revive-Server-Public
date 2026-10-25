# -*- coding: utf-8 -*-
"""只读定位原生战斗实体所属战斗的绑定与销毁函数。"""
from __future__ import print_function
import dis
import marshal
import os
import types
root = os.path.abspath(os.path.join(os.path.dirname(__file__), '../..'))
base = os.path.join(root, 'internal/nativepvp/runtime/native_engine/script')
def visit(code, path, trail):
    raw = code.co_code
    i = 0
    found = False
    while i < len(raw):
        op = ord(raw[i]); i += 1
        if op >= dis.HAVE_ARGUMENT:
            arg = ord(raw[i]) + 256 * ord(raw[i + 1]); i += 2
            if op in (dis.opmap['STORE_ATTR'], dis.opmap['DELETE_ATTR']) and code.co_names[arg] == 'battle':
                found = True
    if found:
        print(path, trail + '/' + code.co_name)
        dis.dis(code)
    for child in code.co_consts:
        if isinstance(child, types.CodeType): visit(child, path, trail + '/' + code.co_name)
for directory, _, names in os.walk(base):
    for name in names:
        if name.endswith('.pyc'):
            path = os.path.join(directory, name)
            try:
                with open(path, 'rb') as source:
                    source.read(8); code = marshal.load(source)
                visit(code, os.path.relpath(path, base), '')
            except (ValueError, EOFError): pass
