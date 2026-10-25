# -*- coding: utf-8 -*-
"""运行原版real_prepare，验证同战斗重复握手不重建已加载对象。"""
from __future__ import print_function
import ast, marshal, os, types, __builtin__
root = os.path.abspath(os.path.join(os.path.dirname(__file__), '../..'))
class Obj(object):
    def __init__(self, **values): self.__dict__.update(values)
with open(os.path.join(root,'internal/nativepvp/runtime/native_engine/script/battle_logic/battle_base.pyc'),'rb') as source:
    source.read(8); code = marshal.load(source)
base = next(c for c in code.co_consts if isinstance(c,types.CodeType) and c.co_name=='battle_base')
real = next(c for c in base.co_consts if isinstance(c,types.CodeType) and c.co_name=='real_prepare')
original = types.FunctionType(real, {'__builtins__':__builtin__, 'data':Obj(battle_info={4408:Obj(start_triggers=[])}),
                                  'common_const':Obj(FRAME_BEGIN_CHECK_TIME=0)}, argdefs=(False, False, None))
class Battle(object):
    def __init__(self):
        self.id = 'same-uuid'; self.bid = None; self.constructed = 0
    def prepare_battle_by_type(self, eids): return eids
    def set_up_base_info(self, eids, bid, seed, dungeon): self.bid = bid; self.constructed += 1
    def set_up_battle_skill(self): pass
    def add_trigger_list(self, triggers): pass
    def add_callback(self, delay, callback): pass
    def create_camp(self): pass
    def set_up_eid_to_camp(self, eids): pass
    def create_fighting_slots(self): pass
    def on_battle_prepare(self, *args): pass
    def set_time_scale(self, scale): pass
    def add_battle_enemy(self): pass
    time_scale = 1
args = (['master'], 4408, 13, 4408)
b = Battle(); original(b, *args)
try: original(b,*args)
except AssertionError as error: assert error.args == (4408,)
else: raise AssertionError('原生重复prepare断言未复现')
path = os.path.join(root,'internal/game/battle_bridge_script.py')
tree = ast.parse(open(path,'rb').read(),path)
install = next(n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name=='_install_revival_battle_bridge')
prepare = next(n for n in install.body if isinstance(n,ast.FunctionDef) and n.name=='prepare')
reports = []
env = {'old_prepare':original, 'ident':str,
       'gworld':Obj(get_player=lambda:Obj(eid='master')),
       'report':lambda b,kind,data:reports.append((kind,data)) or True}
exec compile(ast.Module(body=[prepare]),path,'exec') in env
b = Battle(); env['prepare'](b,*args)
b._revival_result_sent = True
env['prepare'](b,*args)
assert b.constructed == 1 and b._revival_result_sent and len(reports) == 1
# 不同种子不是同一握手，原生冲突必须保留，不能吞掉非法准备。
try: env['prepare'](b,args[0],4408,14,4408)
except AssertionError: pass
else: raise AssertionError('不同prepare被错误接受')
assert b.constructed == 1
print('原生4408重复准备断言复现、同UUID握手幂等及不同准备冲突保留通过')
