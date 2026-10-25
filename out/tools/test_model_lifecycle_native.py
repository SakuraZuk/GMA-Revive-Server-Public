# -*- coding: utf-8 -*-
"""原版驱动、复合模型分发和底层模型函数的失效速度回归；引擎对象为边界夹具。"""
from __future__ import print_function
import __builtin__
import marshal
import os
import sys
import types

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))

def code_at(path, trail):
    with open(os.path.join(root, 'internal/nativepvp/runtime/native_engine/script', path), 'rb') as source:
        source.read(8)
        code = marshal.load(source)
    for name in trail:
        code = next(c for c in code.co_consts if isinstance(c, types.CodeType) and c.co_name == name)
    return code

class Obj(object):
    def __init__(self, **values): self.__dict__.update(values)

env = {'__builtins__': __builtin__, 'debug_utils': Obj(is_engine_test=lambda: False)}
def native(path, trail): return types.FunctionType(code_at(path, trail), env)

class EngineModel(object):
    def __init__(self, valid=True):
        self.valid, self.movements, self.fail = valid, [], False
        self.sfx = Obj(frame_rate=1, anim_rate=1)
    def get_socket_count(self):
        if not self.valid: raise RuntimeError('Invalid model which has beed destroy!')
        if self.fail: raise RuntimeError('live model unrelated failure')
        return 1
    def get_socket_objects(self, index): return [self.sfx]
    def set_move_timescale(self, value):
        if not self.valid: raise RuntimeError('Invalid model which has beed destroy!')
        self.movements.append(value)

class Model(object):
    is_valid = native('wrapper/model.pyc', ['model', 'is_valid'])
    set_animator_speed = native('wrapper/model.pyc', ['model', 'set_animator_speed'])
    set_move_timescale = native('wrapper/model.pyc', ['model', 'set_move_timescale'])
    def __init__(self, valid=True):
        self._ins, self.pause, self.speeds = EngineModel(valid), False, []
        self._animator = Obj(set_speed=self.speeds.append)

call_code = code_at('wrapper/super_model.pyc', ['super_model', '__getattr__', '_call'])
def cell(value): return (lambda: value).func_closure[0]
class Super(object):
    def __init__(self, models): self.models = models
    def __getattr__(self, name):
        values = {'self': self, 'name': name}
        return types.FunctionType(call_code, env, closure=tuple(cell(values[n]) for n in call_code.co_freevars))

class Driver(object):
    set_time_scale = native('client/model_driver.pyc', ['model_driver', 'set_time_scale'])
    set_animator_speed = native('client/model_driver.pyc', ['model_driver', 'set_animator_speed'])
    def __init__(self, models):
        self.model, self.is_battle, self.extra_speed_rate = Super(models), False, 1

def must_raise(call, expected):
    try: call()
    except RuntimeError as error:
        assert str(error) == expected
    else: raise AssertionError('expected original runtime failure')

old = Driver([Model(True), Model(False)])
must_raise(lambda: old.set_time_scale(1.75), 'Invalid model which has beed destroy!')

package = types.ModuleType('wrapper')
module = types.ModuleType('wrapper.model')
module.model, package.model = Model, module
sys.modules['wrapper'], sys.modules['wrapper.model'] = package, module
scope = {'__builtins__': __builtin__}
script = os.path.join(root, 'internal/game/model_lifecycle_script.py')
execfile(script, scope)
installed = Model.set_animator_speed.im_func
scope['_hs_install_model_lifecycle_guard']()
assert Model.set_animator_speed.im_func is installed

for reverse in [False, True]:
    live, dead = Model(True), Model(False)
    driver = Driver([dead, live] if reverse else [live, dead])
    driver.set_time_scale(1.75)
    assert live.speeds == [1.75] and live._ins.movements == [1.75]
    assert live._ins.sfx.frame_rate == 1.75 and live._ins.sfx.anim_rate == 1.75
    assert dead.speeds == [] and dead._ins.movements == [] and dead.animator_speed == 1.75
    assert driver.time_scale == 1.75
    # 模型加载恢复后仍经过原函数，不能永久屏蔽同一个Python对象。
    dead._ins.valid = True
    driver.set_time_scale(1.0)
    assert dead.speeds == [1.0] and dead._ins.movements == [1.0]

live = Model(True)
live.pause = True
live.set_animator_speed(2)
assert live.animator_speed == 2 and live.speeds == []
live.pause, live._ins.fail = False, True
must_raise(lambda: live.set_animator_speed(1), 'live model unrelated failure')
scheduled = []
engine_module = types.ModuleType('game3d')
engine_module.delay_exec = lambda delay, callback: scheduled.append((delay, callback))
sys.modules['game3d'] = engine_module
del module.model
scope['_hs_start_model_lifecycle_guard']()
assert len(scheduled) == 1 and scheduled[0][0] == 1500
module.model = Model
scheduled[0][1]()
assert Model.set_animator_speed.im_func is installed
print('PASS: original failure reproduced; native driver and super-model dispatch; both member orders, cache, recovery, pause, idempotence and live-error propagation')
