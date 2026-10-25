# -*- coding: utf-8 -*-
"""用本版原生RpcMethod关键字解码验证恢复包装层；场景清理为夹具。"""
from __future__ import print_function
import glob
import marshal
import os
import sys
import types

root = os.path.abspath(sys.argv[1])
sys.path.insert(0, os.path.join(root, 'internal', 'nativepvp'))
runtime = os.path.join(root, 'internal', 'nativepvp', 'runtime', 'native_engine')
sys.path.extend(glob.glob(os.path.join(runtime, 'deps', '*.whl')))
sys.path.insert(0, os.path.join(runtime, 'script'))
from battle_native_host import Host
host = Host()
host.install()
from mbengine.common.rpcdecorator import RpcMethod, CLIENT_ONLY
from bson.objectid import ObjectId

class Arg(object):
    def __init__(self, index): self.index = index
    def getname(self): return '_%d' % self.index
    def get_type(self): return 'Any'
    def default_val(self): return None
    def convert(self, value): return value

class Shadow(object):
    def __init__(self, uid=None): self.id = uid

with open(os.path.join(runtime, 'script/entities/components/battle.pyc'), 'rb') as source:
    source.read(8)
    code = marshal.load(source)
component = next(c for c in code.co_consts if isinstance(c, types.CodeType) and c.co_name == 'battle')
native_code = next(c for c in component.co_consts if isinstance(c, types.CodeType) and c.co_name == 'start_server_battle_ok')
native_function = types.FunctionType(native_code, {'client_battle': types.ModuleType('client_battle'), '__builtins__': __import__('__builtin__')})
native_function.func_globals['client_battle'].shadow_battle = Shadow

def native_start(self, battle_type, dungeon_id, battle_uuid, extra):
    native_function(self, battle_type, dungeon_id, battle_uuid, extra)
    self.starts += 1

meta = RpcMethod(native_start, CLIENT_ONLY, tuple(Arg(i) for i in range(4)), True)
def registered(self, *args, **parameters):
    if args:
        assert len(args) == 1 and isinstance(args[0], dict)
        parameters = args[0]
    return meta.call_with_key(self, None, parameters)
registered.rpcmethod = meta
def noop(self, *args, **kwargs): pass
class Avatar(object):
    start_server_battle_ok = registered
    on_refresh_login = preload_by_refresh_login = load_scene_finished = do_guide_task = noop
    def __init__(self): self.battle, self.clears, self.starts = Shadow('retained'), 0, 0
    def raw_clear_battle(self): self.battle = None; self.clears += 1
    def refresh_assist_cards(self, dungeon): pass

avatar_module = types.ModuleType('entities.Avatar')
avatar_module.Avatar = Avatar
sys.modules['entities.Avatar'] = avatar_module
gworld = types.ModuleType('gworld')
gworld.get_battle = lambda: None
sys.modules['gworld'] = gworld
game3d = types.ModuleType('game3d')
game3d.delay_exec = lambda *args: None
sys.modules['game3d'] = game3d
script = os.path.join(root, 'internal', 'game', 'login_battle_recovery_script.py')
scope = {}
exec(compile(open(script, 'rb').read(), script, 'exec'), scope)
av = Avatar()
args = dict(_0=1, _1=10001, _2='current', _3={'hs_recover_previous_uuid': 'server_previous'})
av.start_server_battle_ok(**args)
assert av.clears == 1 and av.starts == 1 and av.battle.id == 'current'
av.start_server_battle_ok(**args)
assert av.clears == 1 and av.starts == 1
av.battle = Shadow('older')
args['_2'] = 'next'
av.start_server_battle_ok(**args)
assert av.clears == 2 and av.starts == 2 and av.battle.id == 'next'
av.battle = Shadow('retained_packet')
args['_2'] = 'packet_current'
av.start_server_battle_ok(args)
assert av.clears == 3 and av.starts == 3 and av.battle.id == 'packet_current'
av.start_server_battle_ok(args)
assert av.clears == 3 and av.starts == 3
av.battle = Shadow(ObjectId('10112233445566778899aabb'))
args['_2'] = ObjectId('20112233445566778899aabb')
av.start_server_battle_ok(args)
assert av.clears == 4 and av.starts == 4 and av.battle.id == args['_2']
av.start_server_battle_ok(args)
assert av.clears == 4 and av.starts == 4
assert registered.rpcmethod is meta
args['_3']['hs_recover_rebuild'] = True
old = av.battle
av.start_server_battle_ok(args)
assert av.clears == 5 and av.starts == 5 and av.battle is not old and av.battle.id == args['_2']
assert av.battle.battle_type == 1 and av.battle.extra_info['hs_recover_rebuild']
av.start_server_battle_ok(**args)
assert av.clears == 6 and av.starts == 6
print('原生Asio单字典调用及RpcMethod解码、旧残留替换和同UUID幂等验证通过')
print('原版战斗创建函数同UUID恢复重建、连续恢复与关键字入口通过；模型清理为夹具')

# 普通入场没有恢复标记时，同UUID重复包同样不能再次调用原生assert入口。
args['_3'] = {}
starts, clears, current = av.starts, av.clears, av.battle
av.start_server_battle_ok(args)
assert av.starts == starts and av.clears == clears and av.battle is current
args['_2'] = ObjectId('30112233445566778899aabb')
try:
    av.start_server_battle_ok(args)
    raise AssertionError('未授权不同UUID覆盖了当前战斗')
except AssertionError:
    assert av.battle is current and av.starts == starts
print('普通入场重复握手幂等、不同UUID未授权冲突仍拒绝通过')
