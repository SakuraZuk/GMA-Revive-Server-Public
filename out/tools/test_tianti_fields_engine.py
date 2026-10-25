# -*- coding: utf-8 -*-
"""隔离原生引擎中核验同点双星辰结界及真实伤害，不能代替Android验收。"""
from __future__ import print_function
import glob
import json
import os
import sys

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
runtime = os.path.join(root, 'internal/nativepvp/runtime/native_engine')
os.chdir(runtime)
sys.path.extend(glob.glob('deps/*.whl'))
sys.path.insert(0, os.path.join(runtime, 'script'))
sys.path.insert(0, os.path.join(root, 'internal/nativepvp'))
from battle_native_host import Host, json_value
host = Host()
host.install()
data = sys.modules['data']

def roster(role, side):
    return [dict(card_id=role, uuid=('%024x' % side), level=60, grade=5, awakened=1,
                 skill_mgr=[dict(skill_id=s, level=1, enhance_level=0) for s in data.role_info[role].skill_list])]

metadata = dict(dungeon_id=21, avatar_id='000000000000000000000001', enemy_id='000000000000000000000002',
                battle_uuid='000000000000000000000003', enemy_roster=roster(4402, 2), battle_type=2, enemy_auto=False)
host.start(metadata, roster(4410, 1), 123456, False)
battle = host.battle
enemy = [info for info in battle.entity_infos.values() if info.get_attr('camp_id') == 2 and not info.get_attr('support')][0]
coord = enemy.get_attr('hex_coord')
first = battle.add_battle_magic_field(4410, 441001, 1, coord, 60, 1, 0, None)
second = battle.add_battle_magic_field(4410, 441001, 1, coord, 60, 1, 0, None)
assert first.eid != second.eid and first.eid in battle.magic_fields and second.eid in battle.magic_fields
class Obj(object):
    def __init__(self, **values): self.__dict__.update(values)
user = [info for info in battle.entity_infos.values() if info.get_attr('camp_id') == 1 and info.get_attr('role_id') == 4410][0]
effect = data.skill_effect[441002]
effect_struct = Obj(user=user, main_target=enemy, target_coord=coord, effect=effect, atk_coord=None, ori_coord=None)
selected = battle.get_all_satisfied_targets(effect_struct, effect.active_magic_fields)
assert set([first.eid, second.eid]).issubset(set(eid for eid, info in selected))
print('同点双结界均存在且通过原生范围/阵营/标签选择；场上结界总数', len(battle.magic_fields))
for field in [first, second]:
    before = enemy.get_attr('hp')
    play = battle.get_magic_field_action_play(field)
    after = enemy.get_attr('hp')
    assert after < before, '原生结界伤害没有独立扣血'
    print('结界伤害构造', json.dumps({'before': before, 'after': after, 'play_type': type(play).__name__}, ensure_ascii=True))
print('隔离引擎双结界各自实际扣血通过；通过原生接口布置测试结界，尚未验证客户端连续施法创建与渲染')
