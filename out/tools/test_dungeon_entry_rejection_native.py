# -*- coding: utf-8 -*-
"""执行原版入场闭包，核验服务端单ret拒绝能回到原生错误提示与调用者。"""
from __future__ import print_function
import __builtin__, marshal, os, types

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
with open(os.path.join(root, 'internal/nativepvp/runtime/native_engine/script/entities/components/dungeon_mgr.pyc'), 'rb') as stream:
    stream.read(8)
    code = marshal.load(stream)

def find(code, name):
    if code.co_name == name:
        return code
    for child in code.co_consts:
        if isinstance(child, types.CodeType):
            result = find(child, name)
            if result is not None:
                return result

class Obj(object):
    def __init__(self, **values):
        self.__dict__.update(values)

requests, cached, errors, replies = [], [], [], []
player = Obj(check_dungeon=lambda value: True,
             cache_player_info=lambda: cached.append(True),
             call_server=lambda *values: requests.append(values),
             show_error_tips=lambda ret: errors.append(ret))
scope = {'__builtins__': __builtin__, 'False': False, 'True': True,
         'gworld': Obj(network_mgr=Obj(has_connected=True))}
enter = types.FunctionType(find(code, 'enter_dungeon'), scope)
for ret in (1005, 1003, 1004, 1019):
    assert enter(player, 6101, {}, replies.append) is True
    name, callback, dungeon, extra = requests[-1]
    assert name == 'enter_dungeon' and dungeon == 6101
    callback(ret)
    assert replies[-1] == ret and errors[-1] == ret
assert len(cached) == len(replies) == len(errors) == 4
print('原版入场闭包：四种单ret拒绝均结束请求并显示原版错误，通过。')
