# -*- coding: utf-8 -*-
"""实际Python2原生函数验证星期奖励与剧情固定槽，不执行图形客户端。"""
from __future__ import print_function
import json, marshal, os, sys, types
root=os.path.abspath(sys.argv[1])
def load(relative):
    with open(os.path.join(root,'internal/nativepvp/runtime/native_engine/script',relative),'rb') as f:
        f.read(8);return marshal.load(f)
def find(code,name):
    if code.co_name==name:return code
    for item in code.co_consts:
        if isinstance(item,types.CodeType):
            result=find(item,name)
            if result is not None:return result
def table(name):
    with open(os.path.join(root,'out/client_catalogs/tables',name+'.json'),'rb') as f:return json.load(f)[u'数据']
class Row(object):
    def __init__(self,**values):self.__dict__.update(values)
get_bonus=types.FunctionType(find(load('custom_types/dungeon.pyc'),'get_success_bonus_id'),{'__builtins__':__import__('__builtin__'),'time_utils':Row(get_weekday=lambda:1)})
get_return=types.FunctionType(find(load('custom_types/dungeon.pyc'),'get_fail_return_power'),{'__builtins__':__import__('__builtin__')})
assert get_return(Row(double=False,return_power=9))==9
assert get_return(Row(double=True,return_power=9))==18
checked=0
for key,row in table('dungeons').items():
    if row.get('dungeon_type')!=8 or row.get('_dungeon_disable'):continue
    obj=Row(finished=False,first_bonus_id=row.get('first_bonus'),everyday_bonus_id=row['everyday_bonus'],damage_bonus=None,bonus_id=row.get('bonus'))
    for day in range(1,8):assert get_bonus(obj,day)==row['everyday_bonus'][day-1]
    checked+=1
class Cards(object):
    def __init__(self):self.fighting_cards=[];self.support_cards=[];self.storyline_cards=[]
gworld=Row(story_mgr=Row(get_story_battle_cards=lambda:[]),get_player=lambda:Row(battle=Row(set_storyline_avatar_list=lambda value:None)))
get_cards=types.FunctionType(find(load('battle_logic/battle_layout_mgr.pyc'),'get_battle_cards'),{'__builtins__':__import__('__builtin__'),'battle_cards':Row(battle_cards=Cards),'gworld':gworld})
with open(os.path.join(root,'internal/game/battle_story_slots.json'),'rb') as f:slots=json.load(f)['fixed_slots']
for key,fixed in slots.items():
    obj=Row(layout_fighting_cards=['00112233445566778899aabb']+[0]*(len(fixed)+1),is_support_slot=lambda i:i>=len(fixed))
    cards=get_cards(obj)
    assert len(cards.fighting_cards)==len(fixed) and len(cards.support_cards)==2
    assert all(value==0 for value in cards.fighting_cards[4:])
pending=[];completed=[];errors=[]
enter=types.FunctionType(find(load('entities/components/dungeon_mgr.pyc'),'enter_dungeon'),{'__builtins__':__import__('__builtin__'),'gworld':Row(network_mgr=Row(has_connected=True))})
obj=Row(check_dungeon=lambda dungeon:True,cache_player_info=lambda:None,show_error_tips=lambda value:errors.append(value),call_server=lambda name,callback,*args:pending.append(callback))
assert enter(obj,610,{},lambda ret:completed.append(ret)) is True
assert not completed
pending.pop()(13)
assert completed==[13] and errors==[13]
print((u'原生星期奖励%d个装备副本全部七日，以及%d种五六槽剧情布阵通过'%(checked,len(slots))).encode('utf-8'))
print(u'原生入场失败IntRet13执行外层回调与错误提示通过'.encode('utf-8'))
