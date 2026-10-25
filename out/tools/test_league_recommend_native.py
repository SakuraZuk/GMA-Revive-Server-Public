# -*- coding: utf-8 -*-
"""真实Go推荐回包经原生league_info及原生回调；仅UI刷新控件为夹具。"""
from __future__ import print_function
import __builtin__, glob, json, marshal, os, sys, types

root = os.getcwd()
runtime = os.path.join(root, 'internal/nativepvp/runtime/native_engine')
os.chdir(runtime)
sys.path.extend(glob.glob('deps/*.whl'))
sys.path.insert(0, os.path.join(runtime, 'script'))
sys.path.insert(0, os.path.join(root, 'internal/nativepvp'))
from battle_native_host import Host
Host().install()
from bson import ObjectId
from custom_types import league

class Obj(object):
    def __init__(self, **values):
        self.__dict__.update(values)

def find(code, name):
    if code.co_name == name:
        return code
    for item in code.co_consts:
        if isinstance(item, types.CodeType):
            result = find(item, name)
            if result:
                return result

def read_fixture(name):
    path = os.path.join(root, 'out/evidence/qq-bugs-20261010', name)
    info = json.load(open(path, 'rb'))
    info['eid'] = ObjectId(info['eid'])
    # Go mobileproto.Map的空值经JSON导出为[]，实际Msgpack编码为空字典。
    # 此处只恢复这两个已由gate.wireValue确定的容器，不替服务端修正错误字段。
    assert info['members'] == [] and info['apply_map'] == []
    info['members'], info['apply_map'] = {}, {}
    return info

old = read_fixture(u'学会推荐修补前夹具.json')
failures = []
for name, value in old.items():
    try:
        league.league_info.load({name: value})
    except (TypeError, AttributeError):
        failures.append(name)
assert sorted(failures) == ['boss_challenge_hps', 'boss_max_hps', 'trial_score']
new = read_fixture(u'学会推荐修补后夹具.json')
for name, value in new.items():
    league.league_info.load({name: value})
loaded = league.league_info.load(new)
assert loaded.trial_score == 0 and not loaded.boss_challenge_hps and not loaded.boss_max_hps
with open('script/entities/components/league_mgr.pyc', 'rb') as source:
    source.read(8)
    code = marshal.load(source)
updates = []
scope = {'__builtins__': __builtin__, 'league': league,
         'gui': Obj(get_ui=lambda name: Obj(update_league_list=lambda: updates.append(name)))}
callback = types.FunctionType(find(code, 'on_get_recommend_leagues'), scope)
player = Obj(logger=Obj(info=lambda *args: None))
callback(player, [new])
assert len(player.recommend_leagues) == 1 and updates == ['league_add']
print('原生通过：旧回包3字段失败已复现；新回包全部字段与整对象加载、client_dump及推荐列表回调通过。图形刷新为夹具，不是Android人工验收。')
