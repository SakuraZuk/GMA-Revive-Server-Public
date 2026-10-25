# -*- coding: utf-8 -*-
"""原版输入校验、组件组合及推进链，验证普通手动操作不等待网络回包。"""
from __future__ import print_function
import ast
import __builtin__
import marshal
import os
import sys
import types

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '../..'))
def native(path, trail, env):
    with open(os.path.join(ROOT, 'internal/nativepvp/runtime/native_engine/script', path), 'rb') as source:
        source.read(8)
        code = marshal.load(source)
    for name in trail:
        code = next(c for c in code.co_consts if isinstance(c, types.CodeType) and c.co_name == name)
    return types.FunctionType(code, env)

class Obj(object):
    def __init__(self, **values): self.__dict__.update(values)
sent, actions, rejected = [], [], []
current = [None]
player = Obj(eid='master', server_proxy=Obj(do_command=lambda *args:sent.append(args)))
world = Obj(get_player=lambda:player, get_battle=lambda:current[0])
const = Obj(MOVE_SKILL_ID=1, SKILL_USED_TYPES=(), TARGET_TYPE_CELL=1)
env = {'__builtins__':__builtin__, 'gworld':world, 'common_const':const}
native_ui = native('battle_logic/controlers/client.pyc', ['server_controler','do_command'], env)
validator = native('battle_logic/controlers/server.pyc', ['controler','do_command'], env)
real_input = native('battle_logic/player_input.pyc', ['player_input','real_do_command'], env)
clear_input = native('battle_logic/controlers/client.pyc', ['local_controler','on_player_input'], env)
advance = native('battle_logic/driver.pyc', ['round_robin_driver','on_player_input'], env)
assemble = native('utils/assemble.pyc', ['gen_sequence_function'], env)
class Unit(object):
    eid = 'unit'
    def get_skill(self, skill): return Obj(no_turn=False, target_type=1) if skill == 1 else None
    def is_support(self): return False
    def have_extra_support_skill(self): return False
    def can_use_skill(self, skill, target): return True
    def set_command(self, command, *args):
        self.command = (command, args)
        return True
unit = Unit()
class Battle(object):
    real_do_command = real_input
    on_player_input = assemble([clear_input, advance])
    def __init__(self):
        self.entity_infos = {'unit':unit}
        self.current_input_eid = 'unit'
        self._revival_started = True
        self._revival_result_sent = False
        self.bid = 'battle'
        self.master_input_timer = object()
    def get_entity(self, eid): return self.entity_infos.get(eid)
    def check_pvp_short_time(self, master): pass
    def clear_master_input_timer(self): self.master_input_timer = None
    def do_action_right_now(self, info): actions.append(info.command)
    def can_move_to_hex(self, coord, skill, info):
        assert isinstance(coord, tuple)
        return coord == (1, -1, 0)
    def get_cell_skill_atk_coord(self, *args): return (0, 0, 0)
    def battle_error(self, message): rejected.append(message)
battle = Battle(); current[0] = battle
# 原版只发送网络；没有回包时无法清理输入计时器，也不会开始行动。
native_ui(battle, 'move_to', 'unit', [1, -1, 0])
assert len(sent) == 1 and actions == [] and battle.master_input_timer is not None

path = os.path.join(ROOT, 'internal/game/battle_bridge_script.py')
tree = ast.parse(open(path, 'rb').read(), path)
installer = next(n for n in tree.body if isinstance(n, ast.FunctionDef) and n.name == '_install_revival_battle_bridge')
functions = [n for n in installer.body if isinstance(n, ast.FunctionDef) and n.name in ('do_command', 'ui_command')]
env.update(validate_command=validator, old_ui_command=native_ui)
exec compile(ast.Module(body=functions), path, 'exec') in env
ui = env['ui_command']; sent[:] = []
ui(battle, 'move_to', 'unit', [1, -1, 0])
assert sent == [] and len(actions) == 1 and battle.master_input_timer is None
assert actions[0] == ('move_to', ((1, -1, 0),))
ui(battle, 'use_skill', 'unit', 1, [1, -1, 0])
assert len(actions) == 2 and sent == []
# 旧实体、非法目标和未知技能必须继续被原版校验拒绝。
ui(battle, 'move_to', 'previous', [1, -1, 0])
ui(battle, 'move_to', 'unit', [9, -9, 0])
ui(battle, 'use_skill', 'unit', 999, [1, -1, 0])
assert len(actions) == 2 and len(rejected) == 3
for field, value in (('_revival_started', False), ('_revival_result_sent', True), ('bid', None)):
    previous = getattr(battle, field); setattr(battle, field, value)
    ui(battle, 'move_to', 'unit', [1, -1, 0])
    setattr(battle, field, previous)
current[0] = Battle()
ui(battle, 'move_to', 'unit', [1, -1, 0])
assert len(actions) == 2 and sent == []
print('原生弱网等待复现、本地合法输入立即推进、原生拒绝规则及旧战斗隔离通过')
