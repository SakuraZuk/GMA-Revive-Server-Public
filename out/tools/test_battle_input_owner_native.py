# -*- coding: utf-8 -*-
"""原版输入回调、实体激活及战斗组件整链；图形与玩家为边界夹具。"""
from __future__ import print_function
import ast
import __builtin__
import marshal
import os
import types
root = os.path.abspath(os.path.join(os.path.dirname(__file__), '../..'))
def native(path, trail, env):
    with open(os.path.join(root, 'internal/nativepvp/runtime/native_engine/script', path), 'rb') as source:
        source.read(8); code = marshal.load(source)
    for name in trail:
        code = next(c for c in code.co_consts if isinstance(c, types.CodeType) and c.co_name == name)
    return types.FunctionType(code, env)
class Obj(object):
    def __init__(self, **values): self.__dict__.update(values)
events = []
current = [None]
player = Obj(eid=7, on_mouse_button_left_cancel=lambda:events.append('取消鼠标'))
world = types.ModuleType('gworld')
world.get_battle = lambda:current[0]
world.get_player = lambda:player
world.is_player = lambda master:master == player.eid
import sys
sys.modules['gworld'] = world
gui = Obj(get_ui=lambda name:Obj(pvp_input_end=lambda:events.append('输入结束'), pvp_input_start=lambda entity:events.append('输入开始')))
env = {'__builtins__':__builtin__, 'gworld':world, 'gui':gui}
class Entity(object):
    activate = native('entities/components/fashion.pyc', ['fashion','activate'], env)
    on_activate = native('entities/components/combat.pyc', ['combat','on_activate'], env)
    on_deactivate = native('entities/components/combat.pyc', ['combat','on_deactivate'], env)
    master = 7
    def _on_activate(self): events.append('原生激活')
    def _on_deactivate(self): events.append('原生停用')
class Manager(object):
    on_allow_player_input = native('battle_logic/battle_mgr.pyc', ['battle_mgr','on_allow_player_input'], env)
    def __init__(self):
        self.entities = {11:Entity()}
        self.entity_infos = {11:Obj(get_master=lambda:7)}
    def in_control_all_state(self,eid): return False
    def check_change_auto_fight(self,eid): events.append('自动切换检查')
# 使用原版assemble闭包，证明最终入口修补保留后续组件且不依赖组件类替换。
component_input = Manager.on_allow_player_input.im_func
assemble = native('utils/assemble.pyc',['gen_sequence_function'],env)
Manager.on_allow_player_input = assemble([component_input, lambda *args:events.append('其余组件')])
manager = Manager(); current[0] = manager
try: manager.on_allow_player_input(11, False)
except AttributeError as error: assert 'battle' in str(error)
else: raise AssertionError('原生缺所属战斗异常未复现')
events[:] = []
script = os.path.join(root,'internal/game/ui_callback_repair_script.py')
tree = ast.parse(open(script,'rb').read(),script)
helper = next(n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name=='_hs_wrap_battle_input_owner')
scope = {}
exec compile(ast.Module(body=[helper]),script,'exec') in scope
Manager.on_allow_player_input = scope['_hs_wrap_battle_input_owner'](Manager.on_allow_player_input)
manager.on_allow_player_input(11,False)
assert manager.entities[11].battle is manager
assert events == ['输入结束','原生停用','取消鼠标','自动切换检查','其余组件']
events[:] = []
manager.on_allow_player_input(11,True)
assert events == ['输入开始','原生激活','取消鼠标','自动切换检查','其余组件'] and manager.activate_eid == 11
# 已绑定对象必须保留；旧战斗、未知实体及未登记信息不能获得新所属关系。
other = Manager(); current[0] = other
manager.entities[11] = Entity()
try:manager.on_allow_player_input(11,False)
except AttributeError:pass
else:raise AssertionError('旧战斗被错误接入')
assert not hasattr(manager.entities[11],'battle')
other.on_allow_player_input(99,False)
owner = Obj(in_control_all_state=lambda eid:False)
other.entities[11].battle = owner
other.on_allow_player_input(11,False)
assert other.entities[11].battle is owner
print('原生缺battle异常复现、当前所属补齐、激活/停用完整调用与旧战斗隔离通过')
