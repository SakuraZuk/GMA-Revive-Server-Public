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
exec(compile(open(script,'rb').read(),script,'exec'),scope)
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
