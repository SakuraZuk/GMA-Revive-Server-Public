# -*- coding: utf-8 -*-
"""Python2执行本版原生领奖函数及回调修补；图形控件和调度为夹具。"""
from __future__ import print_function
import os, sys, types, marshal
root = os.path.abspath(sys.argv[1])
callbacks, rewards, saved, rendered = [], [], {}, []
class Widget(object):
    def update(self): self.updated = True
    def show(self): self.visible = True
    def hide(self): self.visible = False
class Guide(object):
    def __init__(self):
        self.checkin_reward_list = Widget()
        self.checkin_mask = Widget()
        self.wait_bonus = False
    def update_checkin_list(self): self.updated = True
class Summon(object):
    def on_get_card_result(self, *args): rendered.append(args)
    def on_press_summon_cancel(self, count): self.cancelled = count
active = [None]
current_scene = [object()]
def module(name, **values):
    result = types.ModuleType(name)
    result.__dict__.update(values)
    sys.modules[name] = result
    return result
gui = module('gui', get_ui=lambda name: active[0],
             general_tips_mgr=type('Tips',(object,),{'show_tips':lambda self,kind,value,after: rewards.append((kind,value,after))})())
cache = module('cache', set=lambda key,value:saved.update({key:value}))
module('game3d', delay_exec=lambda ms,fn:callbacks.append(fn))
module('gworld', get_current_scene=lambda:current_scene[0],
       scene_mgr=type('Scenes',(object,),{'activate_preload_scene':lambda self:None})())
module('guis.prepare.beginner_guide', beginner_guide=Guide)
class TaskItem(object):
    def get_info(self): return self.info[0]
sys.modules['guis.prepare.beginner_guide'].task_item = TaskItem
class ExploreMap(object):
    def init_show(self, stage): self.free_stage_id = stage
module('guis.prepare.free_stage_map', free_stage_map=ExploreMap)
module('guis.summon_card.summon_new_card', summon_new_card=Summon)
class Avatar(object):
    def call_server(self, method, callback, *args): self.callback = callback
module('entities.Avatar', Avatar=Avatar)
class Shadow(object):
    def on_scene_loaded(self): return False
    def check_auto_battle_fighting(self): return None
class Layout(object):
    def on_battle_prepare(self): return None
    def start_battle(self): return False
    def set_support_mode(self, mode):
        self.support_skills_panel.set_visible(False)
        return mode
module('battle_logic.client_battle', shadow_battle=Shadow)
module('guis.battle.battle_layout', battle_layout=Layout)
gui.is_visible=lambda name:False

with open(os.path.join(root,'internal/nativepvp/runtime/native_engine/script/guis/prepare/beginner_guide.pyc'),'rb') as stream:
    stream.read(8)
    native = marshal.load(stream)
def find(code, name):
    if code.co_name == name: return code
    for value in code.co_consts:
        if isinstance(value,types.CodeType):
            found = find(value,name)
            if found is not None:return found
original = types.FunctionType(find(native,'get_item'), {'gui':gui,'cache':cache})
with open(os.path.join(root,'internal/nativepvp/runtime/native_engine/script/guis/widgets.pyc'),'rb') as stream:
    stream.read(8)
    native_widgets = marshal.load(stream)
TaskItem.get_info = types.FunctionType(find(native_widgets, 'get_info'), {})
stale_row = TaskItem(); stale_row.info = []
try:
    stale_row.get_info()
    raise AssertionError('原生旧任务行越界未复现')
except IndexError:
    pass
with open(os.path.join(root,'internal/nativepvp/runtime/native_engine/script/entities/components/guide_mgr.pyc'),'rb') as stream:
    stream.read(8)
    native_guide = marshal.load(stream)
guide_definition = type('Definition',(object,),{'version':[1], 'guide_storyline':'guide/test', 'finish_end':True})()
module('data', guide={57:guide_definition})
flow_events = []
proxy = type('Proxy',(object,),{'client_sa_log':lambda self,key,value:flow_events.append(value)})()
player = Avatar(); player.server_proxy=proxy; player.avatar_type=1
player.need_guide_ids=[]; player.skip_guide=False; player.in_guide=False
player.can_trigger=lambda *args,**kwargs:True
player.stop_guide=lambda:None
player.guide_finish=lambda *args:None
sys.modules['gworld'].get_player=lambda:player
sys.modules['gworld'].story_mgr=type('Story',(object,),{'run_story':lambda self,*args,**kwargs:object()})()
Avatar.trigger_guide=types.FunctionType(find(native_guide,'trigger_guide'), {'data':sys.modules['data'], 'gworld':sys.modules['gworld'], '__builtins__':__import__('__builtin__')})
obj = Guide()
obj.checkin_reward_list = None
try:
    original(obj,{1:[[1,100,5]]})
    raise AssertionError('原生空界面异常未复现')
except AttributeError as error:
    assert "update" in str(error)
print('本版原生get_item空列表异常已复现')
script = os.path.join(root,'internal/game/ui_callback_repair_script.py')
scope = {}
with open(os.path.join(root,'internal/nativepvp/runtime/native_engine/script/entities/components/free_stage_mgr.pyc'),'rb') as stream:
    stream.read(8)
    native_stage = marshal.load(stream)
native_entry=types.FunctionType(find(native_stage,'on_enter_free_stage'), {'gui':gui})
stage_rpc=type('RPC',(object,),{})()
stage_rpc.func=native_entry
def entry_dispatch(self,*args): return stage_rpc.func(self,*args)
entry_dispatch.rpcmethod=stage_rpc
Avatar.on_enter_free_stage=entry_dispatch
unloaded_summon = sys.modules.pop('guis.summon_card.summon_new_card')
exec(compile(open(script,'rb').read(),script,'exec'),scope)
assert Guide._hs_safe_checkin_revision == 4
assert not scope['_install_hs_ui_callback_repair']()
sys.modules['guis.summon_card.summon_new_card'] = unloaded_summon
assert scope['_install_hs_ui_callback_repair']()
row = TaskItem(); row.info = []
assert row.get_info() is None
row.info = [(1, 100)]
assert row.get_info() == (1, 100)
explore = ExploreMap(); explore.init_show(10101)
assert explore.free_stage_id == 10101
callbacks[:] = []
assert player.trigger_guide(57) is True
assert flow_events[-1]['story_waiting'] is True
callbacks.pop(0)()
assert flow_events[-1]['step']=='guide_wait'
assert player.trigger_guide(57) is True
player.guide_context=object()
before_events=len(flow_events)
callbacks.pop(0)()
assert len(flow_events)==before_events
player.can_trigger=lambda *args,**kwargs:False
before_events=len(flow_events)
assert player.trigger_guide(57) is None
assert len(flow_events)==before_events
player.can_trigger=lambda *args,**kwargs:True
def legacy_wrapper(original_trigger_guide):
    def trigger_guide(self, guide_id, **extra_info):
        flow_events.append({'step':'legacy_noise'})
        return original_trigger_guide(self,guide_id,**extra_info)
    return trigger_guide
Avatar.trigger_guide=legacy_wrapper(Avatar._hs_guide_flow_original)
del Avatar._hs_guide_flow_original
Avatar._hs_guide_flow_revision=1
scope['_install_hs_ui_callback_repair']()
before_events=len(flow_events)
player.can_trigger=lambda *args,**kwargs:False
assert player.trigger_guide(57) is None
assert len(flow_events)==before_events
player.can_trigger=lambda *args,**kwargs:True
obj.get_item({1:[[1,100,5]]})
callbacks.pop(0)()
assert saved['checkin_bonus.day_1'] == [[1,100,5]]
assert rewards[0][1] == ('reward',[[1,100,5]]) and not obj.wait_bonus
rewards[0][2]()
live = Guide(); active[0] = live
live.get_item({2:[[1,101,10]]})
assert live.checkin_reward_list.updated and live.checkin_mask.visible
active[0] = None
callbacks.pop(0)()
rewards[-1][2]()
assert rewards[-1][1] == ('reward',[[1,101,10]]) and not live.wait_bonus
summon = Summon()
active[0] = summon
payload = ([object()], [4401], False, [])
summon.on_get_card_result(*payload)
assert not rendered
current_scene[0] = type('Scene',(object,),{'form_summon_paper_entity_list':lambda *args:None})()
callbacks.pop(0)()
assert len(rendered)==1 and rendered[0]==payload
scope['_install_hs_ui_callback_repair']()
summon.on_get_card_result(*payload)
assert len(rendered)==2
current_scene[0] = object()
summon.on_get_card_result(*payload)
while callbacks: callbacks.pop(0)()
assert summon.cancelled==1 and summon._hs_pending_summon_result==payload
previous_rendered = len(rendered)
# 等待过程中页面被销毁时，不能在稍后场景就绪时渲染旧控件。
summon.on_get_card_result(*payload)
active[0] = None
current_scene[0] = type('Scene',(object,),{'form_summon_paper_entity_list':lambda *args:None})()
while callbacks: callbacks.pop(0)()
assert len(rendered) == previous_rendered and summon._hs_pending_summon_result == payload
summon.on_get_card_result(*payload)
assert len(rendered) == previous_rendered
active[0]=None
avatar=Avatar()
avatar.set_explore_auto_agent(208,True)
avatar.callback(True)
print('领奖关闭界面、延迟关闭、抽卡场景延后及超时解除等待通过')

# 入口断点必须保留原生阻断、返回值和异常，重复安装不能重复包裹。
def check_entry(self, dungeon):
    if getattr(self, 'fail_entry', False):
        raise RuntimeError('原生入口异常夹具')
    return self.battle is None
def enter_entry(self, dungeon, extra=None, callback=None):
    return self.check_dungeon(dungeon)
def clear_entry(self):
    self.battle = None
    return '原生清理返回值'
Avatar.check_dungeon = check_entry
Avatar.enter_dungeon = enter_entry
Avatar.raw_clear_battle = clear_entry
Avatar.on_dungeon_finish = lambda self, win: self.raw_clear_battle()
class Mainline(object):
    def init_show(self): return '原生主线返回值'
module('guis.prepare.main_line_chapter', main_line_chapter=Mainline)
scope['_install_hs_ui_callback_repair']()
player.battle = type('Battle',(object,),{'dungeon_id':21120011, 'battle_process':2, 'bid':123})()
callbacks[:] = []
assert player.enter_dungeon(602,{}) is False
assert flow_events[-1]['accepted'] is False and flow_events[-1]['has_battle'] is True
assert not callbacks
assert player.on_dungeon_finish(True) == '原生清理返回值'
assert player.battle is None
assert player.enter_dungeon(602,{}) is True
assert flow_events[-1]['accepted'] is True
callbacks.pop(0)()
assert flow_events[-1]['step']=='dungeon_after_entry'
assert Mainline().init_show() == '原生主线返回值'
assert flow_events[-1]['step']=='mainline_open_end'
scope['_install_hs_ui_callback_repair']()
before_events=len(flow_events)
assert player.check_dungeon(602) is True
assert len(flow_events)==before_events+2
player.fail_entry=True
try:
    player.enter_dungeon(602,{})
    raise AssertionError('原生入口异常被吞掉')
except RuntimeError:
    assert flow_events[-1]['step']=='enter_dungeon_error'
player.fail_entry=False
old_scene=sys.modules['gworld'].get_current_scene
def unavailable_scene(): raise RuntimeError('诊断场景快照异常夹具')
sys.modules['gworld'].get_current_scene=unavailable_scene
assert player.check_dungeon(602) is True
assert flow_events[-1]['snapshot_unavailable'] is True
sys.modules['gworld'].get_current_scene=old_scene
print('活动后主线入口诊断保留原生阻断、返回值、异常及幂等安装通过')

class DoublePanel(object):
    def update_double_button(self): return self.img_on.set_visible(self.is_double)
DoublePanel.update_double_button=types.FunctionType(find(native_widgets,'update_double_button'),{})
stale_panel=DoublePanel();stale_panel.img_on=None;stale_panel.is_double=True
try:
    stale_panel.update_double_button()
    raise AssertionError('原生倍数按钮空控件异常未复现')
except AttributeError:
    pass
module('guis.widgets', panel_double=DoublePanel)
scope['_install_hs_ui_callback_repair']()
panel=DoublePanel(); panel.img_on=None; panel.is_double=True
assert panel.update_double_button() is None
assert flow_events[-1]['step']=='double_widget_closed'
visible=[]
panel.img_on=type('Image',(object,),{'set_visible':lambda self,value:visible.append(value)})()
panel.update_double_button()
assert visible==[True]
def make_cell(value): return (lambda:value).func_closure[0]
native_panel=find(native_widgets,'panel_double')
native_cost=find(native_panel,'check_cost')
native_callback=find(native_cost,'set_callback')
confirmed=[]; panel.img_on=None; panel.press_callback=lambda enabled:confirmed.append(enabled)
callback=types.FunctionType(native_callback,{},closure=(make_cell(panel),))
callback()
assert confirmed==[True]
def failed_mainline(self): raise RuntimeError('主线页面异常夹具')
Mainline._hs_open_flow_original=failed_mainline
Mainline._hs_open_flow_revision=1
scope['_install_hs_ui_callback_repair']()
try:
    Mainline().init_show()
    raise AssertionError('主线异常被吞掉')
except RuntimeError:
    assert flow_events[-1]['step']=='mainline_open_error'
    assert 'RuntimeError' in flow_events[-1]['message']
print('空倍数按钮保护及主线异常栈透传通过')
def legacy_mainline(original_main_show):
    def main_show(self, *args, **kwargs):
        flow_events.append({'step':'legacy_mainline_wrapper'})
        return original_main_show(self, *args, **kwargs)
    return main_show
Mainline.init_show=legacy_mainline(failed_mainline)
del Mainline._hs_open_flow_original
Mainline._hs_open_flow_revision=1
scope['_install_hs_ui_callback_repair']()
before_events=len(flow_events)
try:
    Mainline().init_show()
except RuntimeError:
    assert len(flow_events)==before_events+2
    assert flow_events[-1]['step']=='mainline_open_error'
print('0916确切闭包升级不重复包装通过')

# 实际Python2编译本批日程函数，控件夹具严格复刻已观测std::string输入边界。
schedule_path=os.path.join(root,'internal/game/activity_schedule_script.py')
schedule_code=compile(open(schedule_path,'rb').read(),schedule_path,'exec')
byte_labels=[]
def strict_label(value):
    if not isinstance(value,str):
        raise RuntimeError('cannot convert argument 1 to std::string')
    byte_labels.append(value)
try:
    strict_label(u'常驻开放')
    raise AssertionError('Python2 Unicode绑定异常未复现')
except RuntimeError:
    pass
class NativeTimeItem(object):
    def set_time_panel(self): return '原生有限日期'
NativeTimeItem._hs_native_set_time_panel=NativeTimeItem.set_time_panel
native_data=type('Data',(object,),{'chapter':{501:type('Chapter',(object,),{'chapter_type':210})(),999:type('Chapter',(object,),{'chapter_type':999})()}})()
schedule_values={'data':native_data,'activity_item':NativeTimeItem,'schedules':{210:{'permanent':True}}}
time_code=find(schedule_code,'set_time_panel')
time_fn=types.FunctionType(time_code,{'__builtins__':__import__('__builtin__')},closure=tuple(make_cell(schedule_values[k]) for k in time_code.co_freevars))
item=NativeTimeItem();item.chapter_id=501;item.text_limit_time=type('Label',(object,),{'set_string':lambda self,value:strict_label(value)})()
assert time_fn(item) is None and byte_labels[-1].decode('utf-8')==u'常驻开放'
item.chapter_id=999
assert time_fn(item)=='原生有限日期'
print('实际Python2日程文案UTF8字节串及有限日期原生回退通过')

# 用原生探索入场函数和RPC注册入口验证诊断，不改变调用及异常。
gui.free_stage_map=ExploreMap()
stage_rpc.func(player,10101,True)
assert player.free_stage_id==10101 and gui.free_stage_map.free_stage_id==10101
assert flow_events[-1]['step']=='explore_on_enter_free_stage_end'
before=stage_rpc.func
scope['_install_hs_ui_callback_repair']()
assert stage_rpc.func is before
gui.free_stage_map=None
try:
    stage_rpc.func(player,10101,True)
    raise AssertionError('探索原生异常被吞掉')
except AttributeError:
    assert flow_events[-1]['step']=='explore_on_enter_free_stage_error'
    assert 'on_enter_free_stage' in flow_events[-1]['message']
shadow=Shadow();shadow.battle_type=1;shadow.dungeon_id=10102;player.battle=shadow
sys.modules['data'].dungeons={10102:type('Dungeon',(object,),{'dungeon_type':2})(),21:type('Dungeon',(object,),{'dungeon_type':5})()}
for unused in range(20):shadow.check_auto_battle_fighting()
assert len([e for e in flow_events if e['step']=='explore_check_auto_battle_fighting_begin'])==4
assert shadow.on_scene_loaded() is False
layout=Layout()
assert layout.start_battle() is False
assert flow_events[-1]['result'] is False
layout.support_skills_panel = Widget()
layout.support_skills_panel.set_visible = lambda value: None
assert layout.set_support_mode('scene') == 'scene'
assert not hasattr(Layout, '_hs_missing_support_panel_revision')
shadow.battle_type=2;shadow.dungeon_id=21
count=len(flow_events)
shadow.on_scene_loaded();layout.start_battle()
assert len(flow_events)==count
print('探索RPC真实注册入口、异常栈、原生返回值、幂等及轮询限量通过')

with open(os.path.join(root,'internal/nativepvp/runtime/native_engine/script/battle_logic/battle_mgr.pyc'),'rb') as stream:
    stream.read(8)
    native_mgr=marshal.load(stream)
native_handle=types.FunctionType(find(native_mgr,'handle_battle_entities_create'),
    {'gworld':sys.modules['gworld'],
     'const':type('Const',(object,),{'RECONNECT_STATUS_IDLE':0,'RECONNECT_STATUS_PREPARE':1,'RECONNECT_STATUS_FIGHTING':2})(),
     'common_const':type('Common',(object,),{'SYNC_PVP_BATTLE':2,'SYNC_PVP_WATCH':7})()})
player.reconnect_stats=0
class Preparation(object):
    def __init__(self,kind):self.battle_type=kind;self.steps=[]
    def create_avatar_entities(self):self.steps.append('avatars')
    def create_creature_entities(self,status):self.steps.append('creatures')
    def create_magic_fields(self):self.steps.append('fields')
    def start_battle_layout(self):self.steps.append('layout')
wrong=Preparation(2);native_handle(wrong)
assert wrong.steps==['avatars','creatures','fields']
correct=Preparation(1);native_handle(correct)
assert correct.steps==['avatars','creatures','fields','layout']
print('原生战斗模式2跳过布阵已复现，普通模式1接续原生布阵通过')

# 本轮真实错误的完整入口：RPC本体 -> PVP就绪 -> 布阵set_ready。
# 正式模式2上面的原生assemble不会初始化布阵面板和区域；不能补假面板。
def native_function(relative, name, values):
    values.setdefault('__builtins__', __import__('__builtin__').__dict__)
    with open(os.path.join(root,'internal/nativepvp/runtime/native_engine/script',relative),'rb') as stream:
        stream.read(8); code=marshal.load(stream)
    return types.FunctionType(find(code,name), values)
world=sys.modules['gworld']
world.get_battle=lambda:shadow
shadow.is_layout_battle=lambda:False
shadow.extra_info={'sync_pvp':True,'server_authoritative_pvp':True}
player.eid='本人'
class PvpView(object): pass
PvpView.on_player_ready=native_function('guis/battle/battle_pvp.pyc','on_player_ready',{'gworld':world,'gui':gui})
pvp=PvpView();pvp.ready_eids=[]
pvp.enemy_info=type('Info',(object,),{'show_state':lambda self:None})()
pvp.own_info=pvp.enemy_info
gui.battle_layout=type('UnpreparedLayout',(object,),{})()
gui.battle_layout.set_ready=native_function('guis/battle/battle_layout.pyc','set_ready',{}).__get__(gui.battle_layout)
def fail_unprepared(mode): raise AttributeError('support_skills_panel')
gui.battle_layout.switch_editor_mode=fail_unprepared
gui.get_ui=lambda name:pvp if name=='battle_pvp' else None
notify=native_function('entities/components/battle.pyc','notify_battle_fighting',{'gui':gui})
try:
    notify(player,player.eid)
except AttributeError as error:
    assert 'support_skills_panel' in str(error)
else:raise AssertionError('现场原生就绪异常未复现')
module('guis.battle.battle_pvp',battle_pvp=PvpView)
# 模拟1101已安装空面板的热升级，必须恢复原方法，不层层包装。
original_support=Layout.set_support_mode
Layout._hs_missing_support_panel_original=original_support
Layout._hs_missing_support_panel_revision=1
Layout.set_support_mode=lambda self,mode:None
scope['_install_hs_ui_callback_repair']()
assert Layout.set_support_mode.im_func is original_support.im_func
notify(player,player.eid)
notify(player,'对手')
assert flow_events[-1]['step']=='pvp_stale_layout_ready_ignored'
assert len(pvp.ready_eids)==1, '正式场景迟到通知污染旧布阵状态'
installed=PvpView.on_player_ready.im_func
scope['_install_hs_ui_callback_repair']()
assert PvpView.on_player_ready.im_func is installed
# 真正的布阵场景仍走原生就绪，不吞掉原生错误。
shadow.is_layout_battle=lambda:True
try:notify(player,player.eid)
except AttributeError as error:assert 'support_skills_panel' in str(error)
else:raise AssertionError('真正布阵场景原生入口被错误屏蔽')
print('实际Python2原生RPC就绪链复现、正式场景迟到通知隔离、1101热升级及真实布阵回退通过')

# revision1守卫已被诊断器再包一层；热升级必须取回原生函数并刷新诊断缓存。
empty_calls=[]
def native_fighting(self,*args,**kwargs): empty_calls.append(True)
def old_guard(original_explore_fighting):
    def explore_fighting(self,*args,**kwargs): return original_explore_fighting(self,*args,**kwargs)
    return explore_fighting
old_empty=old_guard(native_fighting)
Avatar.battle_fighting=lambda self,*a,**kw:old_empty(self,*a,**kw)
Avatar._hs_empty_explore_revision=1
Avatar._hs_explore_battle_fighting_revision=2
Avatar._hs_explore_battle_fighting_revision_originals={'battle_fighting':old_empty}
scope['_install_hs_ui_callback_repair']()
assert Avatar._hs_empty_team_original.im_func is native_fighting
installed_empty=Avatar.battle_fighting.im_func
scope['_install_hs_ui_callback_repair']()
assert Avatar.battle_fighting.im_func is installed_empty
assert Avatar._hs_explore_battle_fighting_revision_originals['battle_fighting'].func_code.co_name == 'explore_fighting'
print('空阵容revision1守卫加诊断包装的精确热升级及重复安装通过')

# 完成窗尚未加载时先安装提示入口，避免首次异步回包赶在下一次轮询之前。
tip_calls=[]
class NativeTips(object):
    def show_tips(self,kind,param,*args,**kwargs): tip_calls.append((kind,param,args,kwargs))
module('guis.common.general_tips_mgr',general_tips_mgr=NativeTips)
sys.modules.pop('guis.activity.consign_finish_new',None)
world.get_player=lambda:type('Owner',(object,),{'card_mgr':{'fixture-card':type('Card',(object,),{'card_id':2202})()}})()
scope['_install_hs_ui_callback_repair']()
installed_tips=NativeTips.show_tips.im_func
scope['_install_hs_ui_callback_repair']()
assert NativeTips.show_tips.im_func is installed_tips
NativeTips().show_tips('consign_finish',(123,['fixture-card'],'已提交奖励'),None,trigger_guide=False)
assert tip_calls==[('consign_finish',(123,[2202],'已提交奖励'),(None,),{'trigger_guide':False})]
print('完整安装器在委托完成窗尚未加载时保护提示入口，重复安装及原生参数保留通过')

# 战斗管理类按需加载后，下一轮安装补齐所属绑定，重复安装不叠加。
input_calls=[]
class InputManager(object):
    def on_allow_player_input(self,eid,allow):input_calls.append((eid,allow,self.entities[eid].battle))
module('battle_logic.client_battle',shadow_battle=InputManager)
input_manager=InputManager()
input_manager.entities={11:type('BattleEntity',(object,),{})()}
input_manager.entity_infos={11:object()}
world.get_battle=lambda:input_manager
scope['_install_hs_ui_callback_repair']()
installed_input=InputManager.on_allow_player_input.im_func
scope['_install_hs_ui_callback_repair']()
assert InputManager.on_allow_player_input.im_func is installed_input
input_manager.on_allow_player_input(11,False)
assert input_calls==[(11,False,input_manager)]
print('按需加载输入管理类、当前战斗所属补齐及重复安装通过')

# 阵容预设保存回包晚于页面销毁：原生update_team_list访问已释放控件崩溃；
# 热修在team_list为None时跳过纯UI重建，控件存在时保留原生行为。
with open(os.path.join(root,'internal/nativepvp/runtime/native_engine/script/guis/battle/battle_team_preset_list.pyc'),'rb') as stream:
    stream.read(8)
    native_preset=marshal.load(stream)
class TeamPreset(object):
    pass
TeamPreset.update_team_list=types.FunctionType(find(native_preset,'update_team_list'),{'__builtins__':__import__('__builtin__').__dict__})
stale_preset=TeamPreset(); stale_preset.team_items=[]; stale_preset.team_list=None
try:
    stale_preset.update_team_list([])
    raise AssertionError('原生阵容预设列表空控件异常未复现')
except AttributeError as error:
    assert 'remove_all_items' in str(error)
module('guis.battle.battle_team_preset_list', battle_team_preset_list=TeamPreset)
scope['_install_hs_ui_callback_repair']()
assert getattr(TeamPreset,'_hs_team_list_revision',0)==1
stale_preset.update_team_list([])
assert stale_preset.team_items==[]
fresh=TeamPreset(); fresh.team_items=[]
cleared=[]
fresh.team_list=type('PresetList',(object,),{'remove_all_items':lambda self:cleared.append(True)})()
fresh.update_team_list([])
assert cleared==[True] and fresh.team_items==[]
installed_preset=TeamPreset.update_team_list.im_func
scope['_install_hs_ui_callback_repair']()
assert TeamPreset.update_team_list.im_func is installed_preset
print('阵容预设保存回包晚于页面销毁的空控件守卫及原生保留通过')
