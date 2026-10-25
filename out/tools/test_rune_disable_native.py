# -*- coding: utf-8 -*-
"""执行原生自动分解条件，验证关闭后的显式空列表不再匹配任何星级。"""
from __future__ import print_function
import marshal
import os
import types

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
with open(os.path.join(root, 'internal/nativepvp/runtime/native_engine/script/entities/components/rune_mgr.pyc'), 'rb') as source:
    source.read(8)
    module = marshal.load(source)

def find(code):
    if code.co_name == 'need_auto_decompose': return code
    for child in code.co_consts:
        if isinstance(child, types.CodeType):
            found = find(child)
            if found is not None: return found

class Obj(object):
    pass

code = find(module)
assert code is not None
check = types.FunctionType(code, {})
player, rune = Obj(), Obj()
player.rune_auto_decompose_stars = [1, 4]
for star in range(1, 7):
    rune.star = star
    assert check(player, rune) == (star in (1, 4))
player.rune_auto_decompose_stars = []
for star in range(1, 7):
    rune.star = star
    assert not check(player, rune)
print('PASS: native need_auto_decompose enabled star selection and explicit empty list; no Android UI claim')
