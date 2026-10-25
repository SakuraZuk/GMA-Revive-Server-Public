# -*- coding: utf-8 -*-
"""只读反汇编指定原生函数，核对腾讯表反馈涉及的字段和调用契约。"""
from __future__ import print_function
import dis
import marshal
import os
import sys
import types

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
path = os.path.join(root, 'internal/nativepvp/runtime/native_engine/script', sys.argv[1])
with open(path, 'rb') as source:
    source.read(8)
    code = marshal.load(source)
names = sys.argv[2:]

def visit(code, parents=''):
    name = parents + '/' + code.co_name
    if not names or code.co_name in names:
        print(name, code.co_argcount, code.co_varnames)
        dis.dis(code)
    for value in code.co_consts:
        if isinstance(value, types.CodeType):
            visit(value, name)

visit(code)
