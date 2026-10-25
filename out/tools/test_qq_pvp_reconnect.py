# -*- coding: utf-8 -*-
"""在本版 Python2 中执行原生对手恢复和加载界面函数，覆盖冷重连缺字段。"""
from __future__ import print_function
import marshal
import os
import types
import __builtin__

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
native = os.path.join(root, 'internal/nativepvp/runtime/native_engine/script')

def find(code, name):
    if code.co_name == name:
        return code
    for child in code.co_consts:
        if isinstance(child, types.CodeType):
            result = find(child, name)
            if result is not None:
                return result

def read(path, name):
    with open(os.path.join(native, path), 'rb') as source:
        source.read(8)
        return find(marshal.load(source), name)

class Obj(object):
    def __init__(self, **values):
        self.__dict__.update(values)
    def load(self, values):
        self.__dict__.update(values)
        return self

player = Obj(eid='own', card_mgr={'ours': Obj(card_id=4401)})
player.set_pvp_enemy_info = types.MethodType(types.FunctionType(
    read('entities/components/sync_pvp_mgr.pyc', 'set_pvp_enemy_info'),
    {'__builtins__': __builtin__, 'avatar_info': Obj(avatar_info=Obj), 'card': Obj(card=Obj(load=lambda value: Obj(**value)))}), player)
player.get_pvp_enemy_info = lambda: player.enemy_info
gworld = Obj(get_player=lambda: player)
with open(os.path.join(root, 'internal/game/battle_bridge_script.py'), 'rb') as source:
    bridge = compile(source.read(), '实际战斗桥', 'exec')
def cell(value):
    return (lambda: value).func_closure[0]
restore_code = find(bridge, 'restore_pvp_enemy')
assert restore_code.co_freevars == ('gworld',)
restore = types.FunctionType(restore_code, {}, closure=(cell(gworld),))
infos = {'own': {}, 'enemy': {'avatar_info': {'nickname': '对手'},
    'preset_ids': ['preset'], 'cards_record': {'preset': {'fighting_cards': ['enemy-card']}},
    'cards': [{'uuid': 'enemy-card', 'card_id': 3202}]}}
player.select_pvp_teams = {'own': {'fighting_cards': ['ours'], 'support_cards': []},
    'enemy': {'fighting_cards': ['enemy-card'], 'support_cards': []}}
received = []
view = Obj(own=Obj(update_info=lambda *args: received.append(args)),
    enemy=Obj(update_info=lambda *args: received.append(args)))
loading = types.FunctionType(read('guis/loading/loading_pvp.pyc', 'set_sync_pvp_cards'),
    {'gworld': gworld, 'gui': Obj(get_ui=lambda name: Obj(hide=lambda: None))})
try:
    loading(view)
except AttributeError as error:
    assert 'enemy_cards' in str(error)
else:
    raise AssertionError('冷客户端未复现缺少对手卡牌')
received[:] = []
restore(None, infos)
loading(view)
assert received[1][0][0].card_id == 3202
assert received[1][2].nickname == '对手'
restore(None, infos)
assert len(player.enemy_cards) == 1
print('原生冷重连缺字段复现、冻结对手快照恢复、加载界面及重复恢复通过')
