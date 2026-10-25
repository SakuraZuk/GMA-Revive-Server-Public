# -*- coding: utf-8 -*-
"""Python2原生替换实体复现、开战事件顺序和迟到回调回归；控件为夹具。"""
from __future__ import print_function
import ast, marshal, os, sys, types
root = os.path.abspath(sys.argv[1])
def native(path, name):
    with open(os.path.join(root,'internal/nativepvp/runtime/native_engine/script',path),'rb') as f:
        f.read(8); code=marshal.load(f)
    def find(c):
        if c.co_name==name:return c
        for x in c.co_consts:
            if isinstance(x,types.CodeType):
                y=find(x)
                if y is not None:return y
    return find(code)
def module(name, **kw):
    m=types.ModuleType(name);m.__dict__.update(kw);sys.modules[name]=m;return m
class Obj(object):pass
events=[]
player=Obj();player.server_proxy=Obj()
player.server_proxy.client_sa_log=lambda *args:events.append(args)
module('gworld',get_player=lambda:player,get_entity=lambda eid:None)
timers=[];module('game3d',delay_exec=lambda ms,fn:timers.append((ms,fn)))
rewards=[];tips=Obj();tips.show_tips=lambda *args:rewards.append(args)
module('gui',get_ui=lambda name:None,general_tips_mgr=tips)
saved={};module('cache',set=lambda k,v:saved.update({k:v}),get=lambda k,d=None:saved.get(k,d))
module('data',role_model={})
class Entity(object):
    def __init__(self,eid,model):self.id=eid;self.model_id=model
    @property
    def eid(self):return self.id
    def init_from_dict(self,attrs):self.attrs=attrs
class Performance(object):
    def __init__(self):self.entities={'target':Entity('target',1)};self.camps={1:1};self.time_scale=1
    def get_pos_by_coord(self,c):return c
    def remove_scene_entity(self,eid):del self.entities.pop(eid).id
    def create_entity_enter_world(self,eid,attrs,callback):
        self.entities[eid]=Entity(eid,attrs['model_id'])
        if callback:callback()
Performance.create_entity=types.FunctionType(native('battle_logic/battle_performance.pyc','create_entity'),{'gworld':sys.modules['gworld'],'__builtins__':__import__('__builtin__')})
module('battle_logic.battle_performance',battle_performance=Performance)
class Assembled(Performance):
    create_entity = Performance.create_entity.im_func
module('battle_logic.client_battle',local_battle=Assembled)
info=Obj();info.attrs={'model_id':2,'camp_id':1,'hex_coord':(0,0,0),'appear_action':''}
try:
    Performance().create_entity('target',info,None,None)
    raise AssertionError('旧实体引用异常未复现')
except AttributeError as e:assert 'id' in str(e)
class Achievement(object):
    def on_receive_bonus(self,box):self.list_medal.update()
module('guis.mission.mission_achievement_pandect',mission_achievement_pandect=Achievement)
class Recommend(object):
    def on_get_search_friend(self,friends):self.friend_items[1]=friends
module('guis.social.friend_recommend',friend_recommend=Recommend,recommend_tab=1)
class Double(object):
    def update_cost_text(self,power):self.text_cost.set_string(power)
    def update_double_button(self):self.img_on.set_visible(True)
module('guis.widgets',panel_double=Double)
class Guide(object):pass
class RewardItem(object):
    def get_info(self):return (1,101)
    def update(self):return saved['checkin_bonus.day_1'][0]
module('guis.prepare.beginner_guide',beginner_guide=Guide,reward_item=RewardItem)
scope={}
script=os.path.join(root,'internal/game/ui_callback_repair_script.py')
exec(compile(open(script,'rb').read(),script,'exec'),scope)
p=Performance();old=p.entities['target'];calls=[]
new=p.create_entity('target',info,callback=lambda:calls.append(1))
assert new is p.entities['target'] and new is not old and new.eid=='target' and calls==[1]
assert p.create_entity('target',info) is new
assert Assembled().create_entity('target',info).eid=='target'
class Box(object):
    def to_list(self):return [(1,100,5)]
saved['checkin_bonus.day_1']=Box()
assert RewardItem().update()==(1,100,5)
a=Achievement();a.list_medal=None;a.on_receive_bonus(Box());assert rewards[-1][1][1]==[(1,100,5)]
r=Recommend();r.friend_items=[];r.on_get_search_friend([1]);assert r.friend_items==[]
r.friend_items=[None,None];r.on_get_search_friend([1]);assert r.friend_items[1]==[1]
d=Double();d.text_cost=None;d.update_cost_text(10)
g=Guide();g.get_item({1:Box()});assert saved['checkin_bonus.day_1']==[(1,100,5)]
scope['_install_hs_ui_callback_repair']();assert p.create_entity('target',info) is new
print('原生更换模型异常复现及新实体返回、签到盒、迟到回调和重复安装通过')

# 下行要调用实际存在的player_input方法，并保留原生目标有效性检查。
focus = types.FunctionType(native('battle_logic/player_input.pyc','set_focus_target'), {})
b = Obj(); b.entity_infos = {}; b.master_eid_to_camp_id = {'master': 1}; b.camp_focus_target = {}
shown = []; b.show_focus_target_sfx = lambda *args: shown.append(args)
focus(b, 'master', 'missing', 1); assert shown == []
focus(b, 'master', '', 1); assert b.camp_focus_target[1][1] == '' and shown == [('master', '', 1)]
record_code = native('custom_types/random_cards_record.pyc', 'random_card_record')
data_code = next(c for c in record_code.co_consts if isinstance(c, types.CodeType) and c.co_name == 'data')
catalog = Obj(); catalog.cards_random = {1: '真实卡池'}
data_get = types.FunctionType(data_code, {'data': catalog})
record = Obj(); record.random_id = 0
try: data_get(record); raise AssertionError('旧抽卡零编号未复现')
except KeyError: pass
record.random_id = 1; assert data_get(record) == '真实卡池'
print('原生聚焦目标过滤及抽卡编号零值异常复现通过')

for filename, installer in [('battle_bridge_script.py','_install_revival_battle_bridge'),('native_authority_bridge_script.py','_install_revival_authority_shadow_bridge')]:
    source=open(os.path.join(root,'internal/game', filename),'rb').read()
    tree=ast.parse(source)
    install=next(n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name==installer)
    funcs=[n for n in install.body if isinstance(n,ast.FunctionDef) and n.name in ('emit','report','fighting')]
    sent=[];player.server_proxy.do_command=lambda method,payload:sent.append(payload[0])
    import time
    env={'gworld':sys.modules['gworld'],'time':time,'ident':str,'state':lambda b:{'units':[]},'_revival_is_sync_pvp':lambda b:False}
    exec(compile(ast.Module(body=funcs),'<开战顺序原函数>','exec'),env)
    def start(b):
        env['report'](b,'round_end',{'eid':'a'},True)
        env['report'](b,'turn',{'eid':'b'},True)
        return '原生返回值'
    env['old_fighting']=start
    b=Obj();b.id='battle';b._revival_started=False
    env['report'](b,'ready',{'version':1})
    assert env['fighting'](b)=='原生返回值'
    assert [v['kind'] for v in sent]==['ready','started','round_end','turn']
    assert [v['sequence'] for v in sent]==[1,2,3,4]
    def broken(b):
        env['report'](b,'round_end',{})
        raise ValueError('原生失败')
    env['old_fighting']=broken
    b=Obj();b.id='failed';b._revival_started=False;sent[:]=[]
    try:env['fighting'](b);raise AssertionError('异常未传播')
    except ValueError:pass
    assert sent==[] and not b._revival_started and b._revival_start_events is None
    print('开战重入回合顺序、连续序号及原生失败不假报开战通过')
