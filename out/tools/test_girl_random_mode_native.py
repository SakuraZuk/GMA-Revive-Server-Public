# -*- coding: utf-8 -*-
"""执行原版随机看板选择函数；候选列表及随机选择器为可核查夹具。"""
from __future__ import print_function
import os,sys,glob,types,marshal,__builtin__
root=os.getcwd();runtime=os.path.join(root,'internal/nativepvp/runtime/native_engine')
os.chdir(runtime);sys.path.extend(glob.glob('deps/*.whl'));sys.path.insert(0,os.path.join(runtime,'script'));sys.path.insert(0,os.path.join(root,'internal/nativepvp'))
from battle_native_host import Host
Host().install()
import common_const
assert [common_const.RANDOM_CAPTAIN_TYPE_NONE,common_const.RANDOM_CAPTAIN_TYPE_ALL,common_const.RANDOM_CAPTAIN_TYPE_FAVORITES]==[0,1,2]
class Obj(object):
 def __init__(self,**kw):self.__dict__.update(kw)
def find(c,name):
 if c.co_name==name:return c
 for v in c.co_consts:
  if isinstance(v,types.CodeType):
   found=find(v,name)
   if found:return found
f=open('script/entities/components/board_girl_mgr.pyc','rb');f.read(8);code=marshal.load(f);f.close()
choices=[]
def choose(ids):choices.append(list(ids));return ids[-1]
scope={'__builtins__':__builtin__,'common_const':common_const,'gui_utils':Obj(get_card_commons_in_order=lambda:[4401,4402]),'random':Obj(choice=choose)}
fn=types.FunctionType(find(code,'get_random_captain_girl'),scope)
p=Obj(girl_random_enable=False,girl_random_type=0,captain_id=4401,temp_disable_random_girl=False,random_captain_girl=None,card_common_mgr={4401:Obj(is_favorite_card=lambda:True),4402:Obj(is_favorite_card=lambda:False)})
assert fn(p,True)==4401 and not choices
p.girl_random_enable=True;p.girl_random_type=1
assert fn(p,True)==4402 and choices[-1]==[4401,4402]
p.girl_random_type=2
assert fn(p,True)==4401 and choices[-1]==[4401]
p.card_common_mgr={}
assert fn(p,True)==4402 and choices[-1]==[4401,4402]
print('原生通过：0关闭、1全部、2仅收藏、无收藏回退全部；未改变原生随机规则，非Android画面验收')
