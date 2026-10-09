# -*- coding: utf-8 -*-
"""执行本版原生引导失败回调，C++文本ABI以严格字节类型夹具验证。"""
from __future__ import print_function
import marshal
import os
import sys
import types

root = os.path.abspath(sys.argv[1])
path = os.path.join(root, 'internal', 'nativepvp', 'runtime', 'native_engine', 'script', 'entities', 'components', 'guide_task_mgr.pyc')
with open(path, 'rb') as source:
    source.read(8)
    code = marshal.load(source)

def find(value, names):
    if not names: return value
    for child in value.co_consts:
        if isinstance(child, types.CodeType) and child.co_name == names[0]:
            return find(child, names[1:])
    raise AssertionError('未找到原生回调')

callback_code = find(code, ['guide_task_mgr', 'guide_task_finished', 'server_callback'])
assert callback_code.co_freevars == ('task_id',)
def cell(value):
    def capture(): return value
    return capture.func_closure[0]
seen = []
class Tips(object):
    def show_tips(self, message):
        assert isinstance(message, str), 'std::string需要UTF-8字节'
        seen.append(message.decode('utf-8'))
gui = types.ModuleType('gui')
gui.tips = Tips()
callback = types.FunctionType(callback_code, {'gui': gui}, closure=(cell(1000),))
message = u'引导任务不存在或未激活'
try:
    callback(False, message)
except AssertionError:
    pass
else:
    raise AssertionError('旧Unicode回包错误通过ABI夹具')
callback(False, message.encode('utf-8'))
assert seen == [message]
print('原生引导失败回调UTF-8字节、中文完整性及ABI类型验证通过；非Android画面验收')
