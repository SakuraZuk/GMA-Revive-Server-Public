# -*- coding: utf-8 -*-
"""执行原生图鉴计数和收集榜回包处理；界面组件用记录调用的桩，不冒充Android操作。"""
from __future__ import print_function
import __builtin__
import json
import marshal
import os
import types

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))

class Obj(object):
    def __init__(self, **values): self.__dict__.update(values)

def native(path, name, env):
    def find(code):
        if code.co_name == name: return code
        for child in code.co_consts:
            if isinstance(child, types.CodeType):
                found = find(child)
                if found is not None: return found
    with open(os.path.join(root, 'internal/nativepvp/runtime/native_engine/script', path), 'rb') as source:
        source.read(8)
        code = find(marshal.load(source))
    assert code is not None, name
    return types.FunctionType(code, dict({'__builtins__': __builtin__}, **env))

def table(name):
    with open(os.path.join(root, 'out/client_catalogs/tables', name+'.json'), 'rb') as source:
        return {int(k): Obj(**v) for k, v in json.load(source)[u'数据'].items()}

cards, archives = table('cards'), table('cards_archive')
with open(os.path.join(root, 'internal/game/profile_catalog.json'), 'rb') as source:
    ids = json.load(source)['counted_cards']
assert len(ids) == 82 and all(i in archives and 1 <= archives[i].branch_num <= 5 for i in ids)
# 已分解到0份的真实历史条目仍计一次；素材/剧情变体和体验条目不能计入。
commons = {ids[0]: Obj(is_experience=False, count=2), ids[1]: Obj(is_experience=False, count=0),
           ids[2]: Obj(is_experience=True, count=1), 4: Obj(is_experience=False, count=10)}
count = native('guis/gui_utils.pyc', 'get_avatar_card_all_count', {
    'gworld': Obj(get_player=lambda: Obj(card_common_mgr=commons)),
    'data': Obj(cards=cards, cards_archive=archives)})
assert count()[0][0] == 2
commons[ids[2]].is_experience = False
assert count()[0][0] == 3

class Info(object):
    def load(self, value):
        self.extra = dict(value.get('extra', {}))
        return self

with open(os.path.join(root, 'out/client_catalogs/configs/common_const.json'), 'rb') as source:
    constants = json.load(source)[u'直接常量']
calls = []
class UI(object):
    def __getattr__(self, key):
        if key in ('list', 'player_rank', 'panel_empty_rank'): return self
        return lambda *args: calls.append(key)

env = {'common_const': Obj(**constants), 'avatar_info': Obj(avatar_info=Info),
       'gworld': Obj(testlog=lambda *args: None), 'gui': Obj(get_ui=lambda name: UI())}
convert = native('entities/components/rank_mgr.pyc', 'cal_pvp_rank_info', env)
receive = native('entities/components/rank_mgr.pyc', 'on_query_rank_list', env)
player = Obj(rank_id=2, rank_info_dict={}, logger=Obj(info=lambda *args: None))
player.cal_pvp_rank_info = lambda infos: convert(player, infos)
receive(player, '2&0', [{'score': [3]}, {'score': [3]}, {'score': [1]}])
assert [x.extra['rank'] for x in player.rank_info_dict[(2, 0)]] == [1, 2, 3]
assert [x.extra['score'] for x in player.rank_info_dict[(2, 0)]] == [[3], [3], [1]]
assert 'update_rank_list' in calls
receive(player, '2&0', [])
assert player.rank_info_dict[(2, 0)] == [] and 'show' in calls
print('原生收集榜通过：真实历史计种、体验/素材排除、同分顺序、分数列表、界面刷新及空榜。')
print('边界：执行本版Python2函数，不代表Android实际点击验收。')
