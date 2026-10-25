# -*- coding: utf-8 -*-
"""在隔离原版Python2运行时验证原石容器及阶段数据，不代替Android操作。"""
from __future__ import print_function
import glob
import os
import sys
import marshal
import types
import __builtin__
import StringIO
root = os.path.abspath(os.path.join(os.path.dirname(__file__), '../..'))
runtime = os.path.join(root, 'internal/nativepvp/runtime/native_engine')
os.chdir(runtime)
sys.path.extend(glob.glob('deps/*.whl'))
sys.path.insert(0, os.path.join(runtime, 'script'))
sys.path.insert(0, os.path.join(root, 'internal/nativepvp'))
from battle_native_host import Host
host = Host()
host.install()
from custom_types.rune_stone import rune_stone
from bson import ObjectId
uid = '00112233445566778899aabb'
oid = ObjectId(uid)
stone = rune_stone.load(dict(material_id=40007, time=1791864000, count=1,
    step_0_count=1, rune_states={uid:1}, client_runes={uid:dict(star=4,suit=1101)}))
assert stone.material_id == 40007 and stone.step_0_count == 1
assert stone.rune_states[oid] == 1
assert stone.client_runes[oid]['star'] == 4
assert len(stone.runes) == 0
stone.update_client_runes({oid:dict(star=4,suit=1101,pos=1,extra_suit=0)})
assert stone.client_runes[oid]['pos'] == 1
assert stone.check_step_1([oid])
stone.rune_states[oid] = 2
assert stone.check_step_2([oid])
stone.rune_states[oid] = 3
assert not stone.check_step_2([oid])
class Obj(object):
    def __init__(self, **values): self.__dict__.update(values)
events=[]
player=Obj(rune_mgr={oid:Obj(uuid=oid)})
ui=Obj(on_open_stone=lambda:events.append('open'),on_polish_stone=lambda:events.append('polish'),on_identify_stone=lambda:events.append('identify'))
gui=Obj(activity_void_rune_shop=ui,general_tips_mgr=Obj(show_tips=lambda *args:events.append('reward')))
with open(os.path.join(runtime,'script/entities/components/rune_mgr.pyc'),'rb') as source:
    source.read(8); module=marshal.load(source)
def child(code,name):
    return next(c for c in code.co_consts if isinstance(c,types.CodeType) and c.co_name==name)
def cell(value):
    return (lambda:value).func_closure[0]
for stage,label in ((0,'open'),(1,'polish'),(2,'identify')):
    code=child(child(child(module,'rune_mgr'),'step_%d_rune_stone'%stage),'server_callback')
    assert code.co_argcount==3 and code.co_freevars==('self',)
    callback=types.FunctionType(code,dict(__builtins__=__builtin__,gui=gui,common_const=Obj(BOX_TYPE_RUNE=4)),closure=(cell(player),))
    previous=sys.stdout;sys.stdout=StringIO.StringIO()
    try:
        callback(False,'',{})
        assert events==[]
        callback(True,'',{oid:dict(star=4,suit=1101)})
    finally: sys.stdout=previous
    assert player.rune_info[oid]['star']==4 and events[0]==label
    if stage==2: assert events==['identify','reward']
    events[:]=[]
print('PASS: original RuneStone.load ObjectId dictionaries, privacy, stage checks and three native callbacks including synced inventory reward; Android pending')
