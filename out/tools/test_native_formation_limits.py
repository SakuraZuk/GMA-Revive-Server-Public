# -*- coding: utf-8 -*-
"""运行本版原生槽生成与阵容回包，核对派生特殊槽容量。"""
from __future__ import print_function
import __builtin__, json, marshal, os, types
root=os.path.abspath(os.path.join(os.path.dirname(__file__),'..','..'))
class Obj(object):
    def __init__(self,**kw):self.__dict__.update(kw)
def find(code,name):
    if code.co_name==name:return code
    for child in code.co_consts:
        if isinstance(child,types.CodeType):
            result=find(child,name)
            if result is not None:return result
def native(path,name,env):
    with open(os.path.join(root,'internal/nativepvp/runtime/native_engine/script',path),'rb') as stream:
        stream.read(8);code=find(marshal.load(stream),name)
    scope={'__builtins__':__builtin__,'True':True,'False':False};scope.update(env)
    return types.FunctionType(code,scope)
def read(name):
    with open(os.path.join(root,'out/client_catalogs/tables',name+'.json'),'rb') as stream:return json.load(stream)[u'数据']
formations,battles=read('formation_data'),read('battle_info')
with open(os.path.join(root,'internal/game/battle_story_slots.json'),'rb') as stream:limits=json.load(stream)['formation_limits']
generate=native('battle_logic/map_mgr.pyc','generate_slots',{'data':Obj(formation_data=formations),'hex_utils':Obj(offset_to_cube=lambda x,y:(x,y))})
world=Obj(story_mgr=Obj(get_story_battle_cards=lambda:[]),get_player=lambda:Obj(battle=Obj(set_storyline_avatar_list=lambda values:None)))
get_cards=native('battle_logic/battle_layout_mgr.pyc','get_battle_cards',{'gworld':world,'battle_cards':Obj(battle_cards=lambda:Obj(fighting_cards=[],support_cards=[]))})
for key,sizes in limits.items():
    row=battles[key]
    fight=generate(Obj(),[0,0,0],row['main_formation'])
    support=generate(Obj(),[0,0,0],row['main_support_formation'])
    assert [len(fight),len(support)]==sizes
    owner=Obj(layout_fighting_cards=list(range(1,len(fight)+len(support)+1)),is_support_slot=lambda index:index>=len(fight))
    cards=get_cards(owner)
    assert [len(cards.fighting_cards),len(cards.support_cards)]==sizes
assert limits['367011']==[6,0] and limits['361']==[2,4] and limits['30311']==[6,2]
print('原生generate_slots及get_battle_cards全量特殊阵型容量%d项一致；六人/四援护回包契约通过，图形及坐标为夹具。'%len(limits))
