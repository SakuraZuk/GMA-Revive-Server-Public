# -*- coding: utf-8 -*-
"""执行原生入馆指南展示闭包，验证场景切换造成的遮罩残留及候选恢复。"""
from __future__ import print_function
import marshal
import __builtin__
import os
import sys
import types

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
class Obj(object):
    def __init__(self, **values): self.__dict__.update(values)
def module(name, **values):
    result = types.ModuleType(name)
    result.__dict__.update(values)
    sys.modules[name] = result
    return result
scene, active, timers, events = [None], [None], [], []
module('game3d', delay_exec=lambda delay, callback: timers.append((delay, callback)))
module('gui', get_ui=lambda name: active[0])
module('cache')
world = module('gworld', get_current_scene=lambda: scene[0],
               sound_mgr=Obj(play_ui_sound=lambda name: None),
               get_player=lambda: Obj(server_proxy=Obj(client_sa_log=lambda name, value: events.append(value))))
with open(os.path.join(root, 'internal/nativepvp/runtime/native_engine/script/guis/prepare/beginner_guide.pyc'), 'rb') as source:
    source.read(8)
    native = marshal.load(source)
def find(code, name):
    if code.co_name == name: return code
    for child in code.co_consts:
        if isinstance(child, types.CodeType):
            result = find(child, name)
            if result is not None: return result
delay_code = find(find(native, 'init_show'), 'delay_show')
def cell(value):
    return (lambda: value).func_closure[0]
def delay_callback(self):
    values = {'self': self, 'init_tab': 0}
    return types.FunctionType(delay_code, {'__builtins__': __builtin__, 'gworld': world, 'first_tab_task': 1},
                              closure=tuple(cell(values[name]) for name in delay_code.co_freevars))
class Panel(object):
    def show(self): self.visible = True
    def hide(self): self.visible = False
class Guide(object):
    def __init__(self): self.panel_beginner = Panel(); self.visible = False
    def show(self): self.visible = True
    def hide(self): self.visible = False; self.panel_beginner.hide()
    def add_callback(self, delay, callback): timers.append((delay, callback)); return 'native-handle'
    def init_show(self, tab_type=None):
        # 原生init_show已取证的展示顺序；实际delay_show字节码原样执行。
        self.show(); self.panel_beginner.hide()
        return self.add_callback(0.1, delay_callback(self))
module('guis.prepare.beginner_guide', beginner_guide=Guide)

guide = Guide(); active[0] = guide; guide.init_show()
try:
    timers.pop(0)[1]()
    raise AssertionError('原生场景缺失中断未复现')
except AttributeError:
    assert guide.visible and not guide.panel_beginner.visible
print('原生复现：场景缺失使delay_show中断，外层仍显示而内容面板隐藏。')
if '--baseline' in sys.argv:
    raise SystemExit(0)

scope = {}
path = os.path.join(root, 'internal/game/ui_callback_repair_script.py')
body = open(path, 'rb').read().rsplit('_start_hs_ui_callback_repair()', 1)[0]
exec(compile(body, path, 'exec'), scope)
scope['_install_hs_ui_callback_repair']()
assert getattr(Guide, '_hs_guide_show_revision', 0) == 1, '缺少入馆展示生命周期修补'

guide = Guide(); active[0] = guide
assert guide.init_show() == 'native-handle'
timers.pop(0)[1]()
assert guide.visible and not guide.panel_beginner.visible and len(timers) == 1
scene[0] = Obj(is_main_city_scene=lambda: False)
timers.pop(0)[1]()
assert guide.panel_beginner.visible and not timers

# 关闭或重新打开后，旧闭包不能复活已关闭的面板或覆盖新页面。
scene[0] = None; guide.init_show(); timers.pop(0)[1](); guide.hide()
scene[0] = Obj(is_main_city_scene=lambda: False); timers.pop(0)[1]()
assert not guide.visible and not guide.panel_beginner.visible
scene[0] = None; guide.init_show(); timers.pop(0)[1]()
guide.init_show(); old = timers.pop(0)[1]; old()
assert not guide.panel_beginner.visible
scene[0] = Obj(is_main_city_scene=lambda: False); timers.pop(0)[1]()
assert guide.panel_beginner.visible

scene[0] = None; guide.init_show()
for attempt in range(25):
    if not timers: break
    timers.pop(0)[1]()
assert not timers and not guide.visible and not guide.panel_beginner.visible
assert any(event.get('step') == 'beginner_show_scene_timeout' for event in events)
original_init = Guide.init_show.im_func
scope['_install_hs_ui_callback_repair']()
assert Guide.init_show.im_func is original_init
girls = []
scene[0] = Obj(is_main_city_scene=lambda: True, set_all_girls_visible=lambda value: girls.append(value))
guide.init_show(); timers.pop(0)[1]()
assert guide.panel_beginner.visible and girls == [False]
unrelated = []
guide.add_callback(1.5, lambda: unrelated.append(True))
assert timers[0][0] == 1.5
timers.pop(0)[1]()
assert unrelated == [True]
def failed_scene(): raise ValueError('原生展示异常')
scene[0] = Obj(is_main_city_scene=failed_scene)
guide.init_show()
try:
    timers.pop(0)[1]()
    raise AssertionError('展示异常被吞掉')
except ValueError:
    assert not guide.visible and not guide.panel_beginner.visible
def failed_init(*args, **kwargs): raise ValueError('不得进入遥测的私有错误详情')
saved_show = guide.show
guide.show = failed_init
try:
    guide.init_show()
    raise AssertionError('初始化异常被吞掉')
except ValueError:
    records = [event for event in events if event.get('step') == 'beginner_init_error']
    assert records and records[-1]['exception'] == 'ValueError'
    assert records[-1]['frames'] and len(records[-1]['frames']) <= 12
    assert all(len(frame) == 3 for frame in records[-1]['frames'])
    assert '私有错误详情' not in repr(records[-1])
guide.show = saved_show
def deeply_failed_show(depth=20):
    if depth: return deeply_failed_show(depth-1)
    raise KeyError('不得上报的缓存键')
guide.show = deeply_failed_show
try:
    guide.init_show()
    raise AssertionError('深层异常被吞掉')
except KeyError:
    record = [event for event in events if event.get('step') == 'beginner_init_error'][-1]
    assert len(record['frames']) == 12 and record['frames'][-1][1] == 'deeply_failed_show'
    assert '缓存键' not in repr(record)
guide.show = saved_show
print('候选通过：场景就绪后展示、关闭与重开隔离、有界等待释放遮罩；没有Android实机结论。')
