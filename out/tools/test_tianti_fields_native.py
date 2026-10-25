# -*- coding: utf-8 -*-
"""执行原生结界选择及激活调度；几何、伤害演出为夹具，不宣称Android伤害验收。"""
from __future__ import print_function
import __builtin__
import json
import marshal
import os
import types

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
base = os.path.join(root, 'internal/nativepvp/runtime/native_engine/script')

class Obj(object):
    def __init__(self, **values): self.__dict__.update(values)
    def get_attr(self, key): return self.attrs.get(key)
    def is_teammate_or_self(self, other): return self.camp == other.camp

def read(relative):
    with open(os.path.join(base, relative), 'rb') as source:
        source.read(8)
        return marshal.load(source)

def find(code, name):
    if code.co_name == name: return code
    for child in code.co_consts:
        if isinstance(child, types.CodeType):
            found = find(child, name)
            if found is not None: return found

logic = read('battle_logic/magic_field_logic.pyc')
events = read('battle_logic/skill_events/magic_field.pyc')
scope = {'__builtins__': __builtin__,
         # 原生37C6CA84取证的SELF=4、MAIN_TARGET=6。
         'common_const': Obj(TARGET_TYPE_SELF=4, TARGET_TYPE_MAIN_TARGET=6),
         # 所有测试点都在同一轴，绝对差等于六边格距离；只验证范围筛选与重叠。
         'hex_utils': Obj(distance=lambda a, b: abs(a[0] - b[0])),
         'battle_common': Obj(AFTER_ACTIVE_MAGIC_FIELD='after', ANY_FIELD_ACTIVED='active', ANY_FIELD_ACTIVED_COUNT='count')}

class Battle(object): pass
for name in ['get_circle_range_magic_fields', 'get_effect_magic_field_targets', 'get_all_satisfied_targets']:
    setattr(Battle, name, types.FunctionType(find(logic, name), scope))
for name in ['process_active_magic_field', 'active_one_magic_field']:
    setattr(Battle, name, types.FunctionType(find(events, name), scope))

with open(os.path.join(root, 'out/client_catalogs/tables/skill_effect.json'), 'rb') as source:
    effect_row = json.load(source)[u'数据']['441002']
assert effect_row['active_magic_fields'] == [['is_teammate_or_self', 441001, -1]]
effect = Obj(**effect_row)
user = Obj(camp=1, attrs={'hex_coord': (1, 0), 'coord_radius': 0})
target = Obj(camp=2, attrs={'hex_coord': (1, 0), 'coord_radius': 0})
fields = []
for eid, coord, camp, tag in [(101, (1, 0), 1, 441001), (102, (1, 0), 1, 441001),
                               (103, (1, 0), 2, 441001), (104, (1, 0), 1, 999),
                               (105, (10, 0), 1, 441001)]:
    fields.append(Obj(eid=eid, camp=camp, attrs={'hex_coord': coord, 'mf_radius': 3, 'mf_tags': [tag]}))
battle = Battle()
battle.magic_fields = dict((field.eid, (field, None)) for field in fields)
effect_struct = Obj(user=user, main_target=target, target_coord=(1, 0), effect=effect, atk_coord=None, ori_coord=None)
selected = battle.get_all_satisfied_targets(effect_struct, effect.active_magic_fields)
assert [eid for eid, field in selected] == [101, 102]
plays, notifications = [], []
battle.get_magic_field_action_play = lambda field: Obj(eid=field.eid, delay=0)
battle.get_effect_delay = lambda play: 0.2
battle.trigger_skill_event = lambda event, *args: notifications.append(event)
effect_play = Obj(append_combo=lambda play: plays.append(play))
battle.process_active_magic_field(effect_struct, effect_play)
assert [play.eid for play in plays] == [101, 102]
assert all(play.delay == 0.2 for play in plays)
assert notifications.count('count') == 2 and notifications.count('active') == 2
print('原生选择与激活调度通过：两个重叠的我方星辰结界分别生成激活演出和计数；排除敌方、异标签及范围外结界。')
print('边界：伤害演出及几何为夹具；没有验证实际伤害、结界创建时序或Android画面，不关闭原反馈。')
