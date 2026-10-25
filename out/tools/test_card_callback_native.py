# -*- coding: utf-8 -*-
"""使用本版原生RPC与card类型验证回调转型，不代替Android画面验收。"""
from __future__ import print_function
import glob
import os
import sys

root = os.path.abspath(sys.argv[1])
sys.path.insert(0, os.path.join(root, 'internal', 'nativepvp'))
runtime = os.path.join(root, 'internal', 'nativepvp', 'runtime', 'native_engine')
sys.path.extend(glob.glob(os.path.join(runtime, 'deps', '*.whl')))
sys.path.insert(0, os.path.join(runtime, 'script'))
from battle_native_host import Host
host = Host()
host.install()
from utils import rpc
from custom_types import card

row = {'uuid': '00112233445566778899aabb', 'card_id': 4401,
       'level': 1, 'grade': 0, 'dress': 440101}
plain = rpc.revert_args([0, [dict(row)], [], False])[1]
assert isinstance(plain[0], dict)
for count in (1, 10):
    payload = [dict(row) for _ in range(count)] + ['card.card_list', '__custom_type']
    result = rpc.revert_args([0, payload, [], False])[1]
    assert isinstance(result, card.card_list)
    assert len(result) == count
    for value in result:
        assert isinstance(value, card.card)
        assert value.card_id == 4401 and value.level == 1
        assert value.rarity > 0
print('原生RPC单抽、十连card_list与rarity读取验证通过')
from custom_types import bonus, box
empty = rpc.revert_args([5, ['card.card_list', '__custom_type'], [], False,
    {'__custom_type': 'box.box', 'materials': {}}])
assert isinstance(empty[1], card.card_list) and not empty[1]
assert isinstance(empty[4], box.box) and empty[4].to_list() == []
returned = rpc.revert_args([{'__custom_type':'bonus.bonus',
                           'contain_items':{100:10,101:10,4:200}}])[0]
assert isinstance(returned, bonus.bonus)
assert sorted((item[1],item[2]) for item in returned.to_list()) == [(4,200),(100,10),(101,10)]
print('原生归还材料bonus.to_list与抽卡失败空类型回调验证通过')
from custom_types import exam
previous = exam.exam_choose_info.load({1:{'correct_num':1,'total_num':1},2:{'correct_num':0,'total_num':1}})
assert previous.get_info(1).correct_num==1 and previous.get_info(2).total_num==1
assert previous.get_info(3).total_num==0
print('原生往届答题统计题号字典与空题默认值验证通过')

# 首次抽到新模板后，原生共享信息查询及收藏选人不能依赖重登补齐。
import marshal, types
def native_function(path, trail, env):
    env = dict(env, __builtins__=__import__('__builtin__'))
    with open(os.path.join(runtime, 'script', path), 'rb') as source:
        source.read(8); code = marshal.load(source)
    for name in trail:
        code = next(c for c in code.co_consts if isinstance(c, types.CodeType) and c.co_name == name)
    return types.FunctionType(code, env)
class Obj(object):
    def __init__(self, **values): self.__dict__.update(values)
from custom_types import card_common
player = Obj(card_common_mgr={}, get_can_used_cards_in_used_type=lambda kind:[4401],
             check_card_in_task=lambda cid:True)
common_get = native_function('entities/components/card_mgr.pyc', ['card_mgr','get_card_common_info'], {})
player.get_card_common_info = lambda cid:common_get(player, cid)
select = native_function('guis/gui_utils.pyc', ['get_all_card_id_in_dormitory_task'],
                         {'gworld':Obj(get_player=lambda:player)})
try:
    select(1)
    raise AssertionError('缺失新幻书共享状态的原生异常未复现')
except AttributeError:
    pass
player.card_common_mgr[4401] = card_common.card_common.load({'card_id':4401,'count':1,'level':1,'grade':0,'card_vo':0})
assert select(1) == [4401]
print('新幻书缺共享状态原生异常复现、补齐共享属性后无需重登选择通过')
