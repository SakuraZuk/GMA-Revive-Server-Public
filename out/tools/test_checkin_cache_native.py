# -*- coding: utf-8 -*-
from __future__ import print_function
import os,sys,glob,marshal,types,__builtin__
root=os.getcwd();runtime=os.path.join(root,'internal/nativepvp/runtime/native_engine')
os.chdir(runtime);sys.path.extend(glob.glob('deps/*.whl'));sys.path.insert(0,os.path.join(runtime,'script'));sys.path.insert(0,os.path.join(root,'internal/nativepvp'))
from battle_native_host import Host
host=Host();host.install()
import data,common_const
from common_logic import bonus_utils
class Obj(object):
 def __init__(self,**kw): self.__dict__.update(kw)
class Widget(object):
 def __getattr__(self,k): return self
 def __call__(self,*args,**kw): return self

def read(p):
 f=open(os.path.join(runtime,'script',p),'rb');f.read(8);return marshal.load(f)
def find(c,n):
 if c.co_name==n:return c
 for v in c.co_consts:
  if isinstance(v,types.CodeType):
   found=find(v,n)
   if found:return found
native=read('guis/prepare/beginner_guide.pyc');utils=read('guis/gui_utils.pyc')
lang=Obj(get_language_res=lambda x:x,get_language_csb=lambda x:x)
us={'__builtins__':__builtin__,'data':data,'language_utils':lang,'format_material_icon_path':lambda x:x,'get_card_icon_gaine_path':lambda x:x}
gui_utils=Obj(**{n:types.FunctionType(find(utils,n),us) for n in ['get_material_icon','get_card_default_icon','get_avatar_rarity_name','get_checkin_card_bg','get_checkin_day_num']})
record=Obj(can_receive=lambda day:True,received_bonus=[])
world=Obj(get_player=lambda:Obj(check_in_records={1:record}),graphics_mgr=Obj(get_splendor_enable=lambda:False))
cache_values={}
scope={'__builtins__':__builtin__,'data':data,'gworld':world,'bonus_utils':bonus_utils,'check_in_bonus_id':1,'cache':Obj(get=lambda k,default:cache_values.get(k,default)),'gui':Obj(beginner_guide=Obj(reward_days=[])),'gui_utils':gui_utils,'language_utils':lang,'common_const':common_const}
# 取证模块710:947仅为rarity_props_dict/sr_days/ssr_days的直线常量赋值。
const_index=native.co_consts.index(None)
raw=native.co_code[710:947]+'d'+chr(const_index & 255)+chr(const_index >> 8)+'S'
constant_code=types.CodeType(0,native.co_nlocals,native.co_stacksize,native.co_flags,raw,native.co_consts,native.co_names,native.co_varnames,native.co_filename,'rarity_constants',1047,'')
exec(constant_code,scope)
rarity_update=types.FunctionType(find(find(native,'reward_item'),'update_rarity'),scope)
update=types.FunctionType(find(find(native,'reward_item'),'update'),scope)
for received in [False,True]:
 for day,bid in data.check_in_bonus[1].check_in_bonus:
  record.received_bonus=[day] if received else []
  cache_values.clear()
  if received:cache_values['checkin_bonus.day_%d'%day]=bonus_utils.generate_bonus(bid).to_list()
  item=Widget();item.get_info=lambda:(day,bid);item.update_rarity=types.MethodType(rarity_update,item)
  try:update(item);print('通过',day,'已领',received)
  except Exception as error:print('失败',day,'已领',received,type(error).__name__,repr(error));raise

# 使用真实RPC全领回包与原生奖励渲染，复现字典缓存再验证迁移和新回调。
from utils import rpc
from custom_types.box import box
wire={1:{'__custom_type':'box.box','materials':{501:3},'cards':[]}}
received=rpc.revert_args([0,wire])[1]
assert isinstance(received[1],dict)
cache_values['checkin_bonus.day_1']=received[1]
item=Widget();item.get_info=lambda:(1,220001);item.update_rarity=types.MethodType(rarity_update,item)
try:
 update(item)
 raise AssertionError('原生KeyError没有复现')
except KeyError:pass
print('原生全领回包缓存导致奖励行KeyError已复现')
patch={}
body=open(os.path.join(root,'internal/game/ui_callback_repair_script.py'),'rb').read().rsplit('_start_hs_ui_callback_repair()',1)[0]
exec(compile(body,'ui_callback_repair_script.py','exec'),patch)
def module(name,**values):
 result=types.ModuleType(name);result.__dict__.update(values);sys.modules[name]=result;return result
class Guide(object):pass
class Reward(Widget):pass
Reward.update=update
active=[None];callbacks=[];shown=[]
module('game3d',delay_exec=lambda ms,fn:callbacks.append(fn))
module('cache',get=lambda k,d=None:cache_values.get(k,d),set=lambda k,v:cache_values.update({k:v}))
module('gworld',get_player=world.get_player)
module('gui',get_ui=lambda name:active[0],general_tips_mgr=Obj(show_tips=lambda kind,value,after:shown.append(value)))
module('guis.prepare.beginner_guide',beginner_guide=Guide,reward_item=Reward)
patch['_install_hs_ui_callback_repair']()
item=Reward();item.get_info=lambda:(1,220001);item.update_rarity=types.MethodType(rarity_update,item)
item.update()
assert cache_values['checkin_bonus.day_1']==[('material',501,3)]
old=Reward.update.im_func;patch['_install_hs_ui_callback_repair']();assert Reward.update.im_func is old
for payload in [received[1],box.load(wire[1]),[('material',501,3)]]:
 obj=Guide();obj.get_item({1:payload});callbacks.pop(0)()
 assert shown[-1]==('reward',[('material',501,3)])
 assert cache_values['checkin_bonus.day_1']==[('material',501,3)]
assert wire[1]['materials']=={501:3} and '__custom_type' in wire[1]
print('修补通过：旧字典缓存迁移、全领字典/奖励对象/列表、实际奖励行渲染、幂等安装；GUI为夹具，无Android结论')
