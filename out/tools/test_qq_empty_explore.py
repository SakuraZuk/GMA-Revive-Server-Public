# -*- coding: utf-8 -*-
"""执行本版原生阵容检查与实际热修函数，验证空探索恢复布阵。"""
from __future__ import print_function
import __builtin__
import marshal
import json
import os
import sys
import types

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
def find(code, name):
    if code.co_name == name:
        return code
    for child in code.co_consts:
        if isinstance(child, types.CodeType):
            result = find(child, name)
            if result is not None:
                return result
class Obj(object):
    def __init__(self, **values): self.__dict__.update(values)
def cell(value): return (lambda: value).func_closure[0]

data = types.ModuleType('data')
data.dungeons = {10101: Obj(dungeon_type=2, dungeon_battle_id=20201),
                 17: Obj(dungeon_type=2, dungeon_battle_id=10002),
                 21: Obj(dungeon_type=3, dungeon_battle_id=21)}
data.battle_info = {20201: Obj(my_avatar_list=None), 10002: Obj(my_avatar_list=[4401])}
with open(os.path.join(root, 'out/client_catalogs/tables/dungeons.json'), 'rb') as source:
    dungeon_rows = json.load(source)[u'数据']
with open(os.path.join(root, 'out/client_catalogs/tables/battle_info.json'), 'rb') as source:
    battle_rows = json.load(source)[u'数据']
for dungeon_id in (2301102, 2302102):
    row = dungeon_rows[str(dungeon_id)]
    definition = battle_rows[str(row['dungeon_battle_id'])]
    assert row['dungeon_type'] == 23 and not definition['my_avatar_list']
    data.dungeons[dungeon_id] = Obj(**row)
    data.battle_info[row['dungeon_battle_id']] = Obj(**definition)
sys.modules['data'] = data
cards = Obj(fighting_cards=[0, 0, 0, 0], support_cards=[], storyline_cards=[])
with open(os.path.join(root, 'internal/nativepvp/runtime/native_engine/script/custom_types/battle_cards.pyc'), 'rb') as source:
    source.read(8)
    native = marshal.load(source)
check = types.FunctionType(find(native, 'check'), {'__builtins__': __builtin__, 'data': data,
    'common_const': Obj(ACTIVITY_TYPE_GUESS_PVP=999, ACTIVITY_TYPE_LEAGUE_EXPLORE=998),
    'error_code': Obj(RET_SUCCESS=0, RET_BATTLE_CARD_ERROR=1)})
assert check(cards, 10101, Obj()) == 0, '未复现原生空槽放行'
calls, shown = [], []
with open(os.path.join(root, 'internal/game/ui_callback_repair_script.py'), 'rb') as source:
    script = compile(source.read(), '实际界面热修', 'exec')
code = find(script, 'explore_fighting')
env = {'gui': Obj(battle_layout=Obj(on_battle_prepare=lambda: shown.append(True))),
       'flow_event': lambda *args: None,
       'original_explore_fighting': lambda *args, **kwargs: calls.append(True)}
repair = types.FunctionType(code, {'__builtins__': __builtin__}, closure=tuple(cell(env[n]) for n in code.co_freevars))
player = Obj(battle=Obj(dungeon_id=10101, get_battle_cards=lambda: cards))
repair(player); repair(player)
assert not calls and len(shown) == 1, '空阵容仍开战或反复刷新布阵'
cards.support_cards = ['援护']
repair(player)
assert not calls, '援护错误充当出战角色'
cards.fighting_cards[1] = '真实出战卡'
repair(player)
assert len(calls) == 1, '补齐阵容后无法正常开战'
cards.fighting_cards = [0, 0, 0, 0]
repair(player)
assert len(shown) == 2 and len(calls) == 1
player.battle.dungeon_id = 17
repair(player)
assert len(calls) == 2, '教学固定角色被拦截'
player.battle.dungeon_id = 10101
cards.storyline_cards = ['剧情卡']
repair(player)
assert len(calls) == 3, '剧情角色被拦截'
cards.storyline_cards = []
player.battle.dungeon_id = 21
repair(player)
assert len(calls) == 4, '非探索流程被改写'
for dungeon_id in (2301102, 2302102):
    cards.support_cards = []
    assert check(cards, dungeon_id, Obj()) == 0, '未复现剧情塔全零槽放行'
    player.battle.dungeon_id = dungeon_id
    player.battle._hs_empty_explore_edit_shown = False
    before = (len(calls), len(shown))
    repair(player); repair(player)
    assert len(calls) == before[0] and len(shown) == before[1] + 1
    cards.fighting_cards[0] = '真实出战卡'
    repair(player)
    assert len(calls) == before[0] + 1
    cards.fighting_cards[0] = 0
print('原生空槽放行复现、恢复布阵、重复触发、援护隔离及教学剧情边界通过')
print('原表2301102/2302102剧情塔空锁阵容回归通过；界面为夹具，未代替Android验收')
