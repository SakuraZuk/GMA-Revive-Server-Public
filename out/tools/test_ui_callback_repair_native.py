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
unloaded_summon = sys.modules.pop('guis.summon_card.summon_new_card')
exec(compile(open(script,'rb').read(),script,'exec'),scope)
assert Guide._hs_safe_checkin_revision == 2
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
active[0]=None
avatar=Avatar()
avatar.set_explore_auto_agent(208,True)
avatar.callback(True)
print('领奖关闭界面、延迟关闭、抽卡场景延后及超时解除等待通过')
