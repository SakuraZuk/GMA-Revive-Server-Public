# -*- coding: utf-8 -*-
"""执行本版原生技能锁回推与AI选择，并检查新战斗对象；不改技能规则。"""
from __future__ import print_function
import __builtin__
import marshal
import os
import types

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))

class Obj(object):
    def __init__(self, **kw): self.__dict__.update(kw)

def find(code, name):
    if code.co_name == name: return code
    for child in code.co_consts:
        if isinstance(child, types.CodeType):
            result = find(child, name)
            if result is not None: return result

def native(path, name, env):
    with open(os.path.join(root, 'internal/nativepvp/runtime/native_engine/script', path), 'rb') as source:
        source.read(8)
        code = find(marshal.load(source), name)
    assert code is not None, name
    return types.FunctionType(code, dict({'__builtins__': __builtin__}, **env))

# 数值已经由common_const原始字节码核对。
const = Obj(SUPPORT_HABIT=1, BATTLE_HABIT=2, FIRST_SKILL_ATTTACK=1, AI_BATTLE=100, ASYN_PVP_BATTLE=3)
env = {'common_const': const}
update = native('battle_logic/player_input.pyc', 'on_set_auto_battle_habit', env)
allow = native('battle_logic/ai_nodes/entity_status.pyc', 'auto_battle_habit', env)
judge = native('battle_logic/ai_nodes/entity_status.pyc', 'ai_judge_skill_disable', env)
can_use = native('battle_logic/ai_nodes/entity_status.pyc', 'ai_can_use_skill_by_index', env)
refresh = []
battle = Obj(battle_type=1, auto_battle_habit={}, avatar_info=Obj(extra={}),
             on_set_auto_battle_habit_performance=lambda: refresh.append(True))
entity = Obj(battle=battle, attrs={'card_uuid': '已拥有卡牌'},
             is_support=lambda: False, is_creature=lambda: False,
             get_skill_by_index=lambda index: Obj(index=index),
             ai_can_use_skill=lambda *args, **kw: True)
entity.auto_battle_habit = lambda index: allow(entity, index)
chosen = {}
context = Obj(get_info=lambda key, default: default,
              set_info=lambda key, value: chosen.update({key: value}))

for selected in (1, 2, 3, 100):
    update(battle, '已拥有卡牌', Obj(support_habit=False, battle_habit=selected))
    assert battle.auto_battle_habit['已拥有卡牌'][1] is False
    for index in (1, 2, 3):
        expected = selected == 100 or index == 1 or index == selected
        assert allow(entity, index) == expected
        assert judge(entity, context, Obj(index=index)) == (not expected)
        assert can_use(entity, context, Obj(index=index)) == expected
    # 连续战斗/重连的新对象使用已保存设置，不能靠旧对象偶然缓存通过。
    next_battle = Obj(battle_type=1, auto_battle_habit={}, avatar_info=Obj(extra={}),
                      on_set_auto_battle_habit_performance=lambda: None)
    update(next_battle, '已拥有卡牌', Obj(support_habit=False, battle_habit=selected))
    entity.battle = next_battle
    assert [allow(entity, index) for index in (1, 2, 3)] == [selected == 100 or index == 1 or index == selected for index in (1, 2, 3)]
    entity.battle = battle

assert len(refresh) == 4
update(battle, '已拥有卡牌', Obj(support_habit=True, battle_habit=2))
battle.battle_type = 3
battle.avatar_info.extra['asyn_pvp_auto'] = True
assert all(allow(entity, index) for index in (1, 2, 3)), '原生异步全自动例外被改变'
print('原生技能锁回推与AI选择通过：技能1/2/3/自动、普通攻击回退、新战斗对象、异步全自动原规则。')
print('边界：未执行Android点击、动画或实际重连装配。')
