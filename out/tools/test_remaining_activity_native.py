"""用私有Python2执行本版原生buff、等级修正和四类夏活收益；不连接生产。"""
from pathlib import Path
import json
import os
import subprocess
import sys

sys.stdout.reconfigure(encoding='utf-8')

root = Path(__file__).resolve().parents[2]
source = r'''
# -*- coding: utf-8 -*-
import sys, glob, json
sys.path.extend(glob.glob(r'.\internal\nativepvp\runtime\native_engine\deps\*.whl'))
sys.path.insert(0, r'.\internal\nativepvp')
sys.path.insert(0, r'.\internal\nativepvp\runtime\native_engine\script')
import battle_native_host as native
from battle_native_host import Host
h = Host(); h.install()
import data, common_const
from common_logic import battle_common
execfile(r'.\internal\game\activity_buffs_script.py')

class User(object):
 create_entity_eid = None
 create_skill_id = None
 def __init__(self, role, camp):
  self.role, self.camp = role, camp
  self.attrs = {'role_id':role,'hp':0,'max_hp':100000}
 def get_camp_id(self): return self.camp
 def is_teammate_or_self(self, other): return self.camp==other.camp
 def get_attr(self, name):
  return self.attrs[name]
class Battle(object): extra_info = {'hs_summer_closed_beta':True}
h.battle = Battle()
rows=[]
for cls in (battle_common.damage_struct, battle_common.cure_struct):
 for effect in (common_const.DIRECT_DAMAGE,common_const.INDIRECT_DAMAGE,common_const.REAL_DAMAGE):
  for role,camp,enabled in ((2202,1,True),(4401,1,True),(2202,2,True),(2202,1,False)):
   h.battle.extra_info={'hs_summer_closed_beta':enabled}
   value=cls(User(role,camp),User(4401,2),100,None)
   value.set_effect_type(effect)
   value.set_effect_type(effect)
   expected=160 if role==2202 and camp==1 and enabled and effect in (common_const.DIRECT_DAMAGE,common_const.INDIRECT_DAMAGE) else 100
   assert value.get_expect_value()==expected,(cls,effect,role,camp,enabled,value.get_expect_value())
   rows.append({'kind':cls.__name__,'effect':effect,'role':role,'camp':camp,'enabled':enabled,'value':expected})
execfile(r'.\internal\game\activity_buffs_script.py')
value=battle_common.damage_struct(User(2202,1),User(4401,2),100,None)
h.battle.extra_info={'hs_summer_closed_beta':True}
value.set_effect_type(common_const.DIRECT_DAMAGE)
assert value.get_expect_value()==160

def started(dungeon,snapshot):
 host=Host();host.install()
 card=dict(card_id=4401,uuid='000000000000000000000010',level=40,grade=4,awakened=1,
   skill_mgr=[dict(skill_id=s,level=1,enhance_level=0) for s in data.role_info[4401].skill_list])
 meta=dict(dungeon_id=dungeon,avatar_id='000000000000000000000001')
 if snapshot: meta['activity_buff_snapshot']=snapshot
 host.start(meta,[card],123456,False)
 return host,host.snapshot()
base_host,base=started(21,{})
buff_host,buff=started(21,{'change_attr_data':{'buff':{1:[[2080021,None],[2080021,None],[2080001,None]]}}})
one=next(x for x in base['units'] if x['role']==4401)
two=next(x for x in buff['units'] if x['role']==4401)
entity=base_host.battle.entity_infos['1']
attack_add=data.buff[2080021].add_modifiers[0][1](entity,None,None,None,property_base=2)
hp_add=data.buff[2080001].add_modifiers[0][1](entity,None,None,None,property_base=2)
assert abs(two['attrs']['atk']-one['attrs']['atk']-2*attack_add)<1e-9
assert two['attrs']['max_hp']>one['attrs']['max_hp']
ordinary_host,ordinary=started(20711101,{})
low_host,low=started(20711101,{'enemy_level_added':-20})
high_host,high=started(20711101,{'enemy_level_added':150,'change_attr_data':{'buff':{2:[[6040174,None]]}}})
before=[x for x in ordinary['units'] if x['camp']==2]
lower=[x for x in low['units'] if x['camp']==2]
higher=[x for x in high['units'] if x['camp']==2]
assert before and lower and higher
assert all(x['attrs']['level']==5 for x in lower)
assert all(x['attrs']['level']==before[i]['attrs']['level']+150 for i,x in enumerate(higher))
neutral_host,neutral=started(20711101,{'enemy_level_added':150})
neutral_units=[x for x in neutral['units'] if x['camp']==2]
assert all(abs(x['attrs']['atk']/neutral_units[i]['attrs']['atk']-1.5)<1e-9 for i,x in enumerate(higher))
# 同版原生performance/statistics方法加真实数据表，GUI报告外壳为隔离夹具。
import types,marshal
# 只执行原生已转换方法code，不导入GUI/场景依赖，也不重写其判定。
performance=marshal.loads(open(r'.\internal\nativepvp\runtime\native_engine\script\battle_logic\battle_performance.pyc','rb').read()[8:])
performance_class=next(c for c in performance.co_consts if isinstance(c,types.CodeType) and c.co_name=='battle_performance')
protect_code=next(c for c in performance_class.co_consts if isinstance(c,types.CodeType) and c.co_name=='is_league_protect')
native_is_protect=types.FunctionType(protect_code,{'data':data,'common_const':common_const})
from battle_logic import component_mgr
assert 'battle_performance.battle_performance' in component_mgr.shadow_battle,component_mgr.shadow_battle
metrics_source=open(r'.\internal\game\activity_metrics_script.py','rb').read()
protect_row=next(iter(data.league_protect.itervalues()))
treasure_role=next(role for role,row in data.role_info.iteritems() if 501 in (row.role_tag or ()))
ordinary_role=next(role for role,row in data.role_info.iteritems() if 501 not in (row.role_tag or ()))
metrics=[]
def metric_fixture(human_first,protected):
  engine,ignored=started(21,{})
  native_battle=engine.battle
  native_battle.dungeon_id=protect_row.dungeon_id if protected else 21
  assert native_is_protect(native_battle)==protected
  if protected:
   native_battle.statistics={2:{'target-a':{'role_id':ordinary_role,'dead':1},'target-b':{'role_id':ordinary_role,'dead':1},'target-summon':{'role_id':ordinary_role,'dead':1,'is_summon':1},'target-live':{'role_id':ordinary_role},'target-treasure':{'role_id':treasure_role,'dead':1}}}
  calls=[]
  def report(battle,kind,values,**kwargs):
   calls.append(dict(values));return True
  def notify(self):
   if self.sent:return
   self.sent=report(self,'result',{'winner_eids':[]},include_state=True)
  class MetricBattle(object):
   _revival_bridge_version=12
   notify_battle_finish=notify
   sent=False
   human=False
   bid=native_battle.bid
   dungeon_id=native_battle.dungeon_id
   def get_battle_ap_statistics(self):return native_battle.get_battle_ap_statistics()
   def get_total_extra_statistics(self):
    if self.bid is None:raise KeyError(None)
    return native_battle.get_total_extra_statistics()
   def get_statistics(self):return native_battle.get_statistics()
   def is_league_protect(self):return native_is_protect(native_battle)
   def battle_end_notice(self,*args,**kwargs):self.bid=None
  if human_first:
   old_notify=notify
   def human_notify(self):
    if self.human:return 'human-only'
    return old_notify(self)
   MetricBattle.notify_battle_finish=human_notify
   MetricBattle._revival_human_originals={'notify_battle_finish':notify}
  import battle_logic
  client=types.ModuleType('battle_logic.client_battle');client.shadow_battle=MetricBattle
  sys.modules['battle_logic.client_battle']=client;battle_logic.client_battle=client
  namespace={};exec(compile(metrics_source,'activity_metrics_script.py','exec'),namespace)
  namespace['_install_hs_activity_metrics']()
  sample=MetricBattle()
  expected_ap=int(native_battle.get_battle_ap_statistics())
  expected_damage=int(native_battle.get_total_extra_statistics())
  sample.battle_end_notice([],1)
  assert sample.bid is None
  sample.notify_battle_finish();sample.notify_battle_finish()
  assert len(calls)==1
  assert 'total_ap_statistics' in calls[0] and 'total_damaged_statistics' in calls[0]
  assert calls[0]['total_ap_statistics']==expected_ap and calls[0]['total_damaged_statistics']==expected_damage
  assert MetricBattle._revival_activity_metrics_revision==3
  if protected:assert calls[0]['league_protect_statistics']=={'kill_count':2,'treasure_role_id':treasure_role},calls
  else:assert 'league_protect_statistics' not in calls[0]
  if human_first:
   shared=MetricBattle();shared.human=True
   assert shared.notify_battle_finish()=='human-only' and len(calls)==1
  return {'protect':protected,'human_first':human_first,'report':calls[0]}
for human_first in (False,True):
 for protected in (False,True):metrics.append(metric_fixture(human_first,protected))
print(json.dumps({'夏活原生数值检查':rows,'重复安装倍率':value.get_expect_value(),'初音基线':one['attrs'],'初音两攻击一生命层':two['attrs'],'汪言基线':before,'汪言最低等级':lower,'汪言修正及敌buff':higher,'原生普通雅努斯方法及统计报告':metrics},ensure_ascii=True))
'''
path = root / 'out/remaining-activity-native-execution.py'
source = source.replace(r'.', root.as_posix()).replace('\\', '/')
expected = (root / 'internal/game/activity_buffs_script.py').read_text(encoding='utf-8').split('def _start_hs_activity_buffs():')[0]
if (root / 'internal/nativepvp/activity_buffs_native.py').read_text(encoding='utf-8') != expected:
    raise RuntimeError('Android夏活与私有原生宿主脚本不同源')
path.write_text(source, encoding='utf8')
python2 = os.environ.get('HS_NATIVE_PVP_PYTHON2', str(root / 'internal/nativepvp/runtime/native_engine/python2/package/tools/python.exe'))
result = subprocess.run([python2,str(path)],cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
body = result.stdout.decode('utf8')
(root / 'out/remaining-activity-native-execution.log').write_text(body, encoding='utf8')
if result.returncode:
    print(body)
else:
    evidence = json.loads(body.splitlines()[-1])
    (root / 'out/remaining-activity-native-execution.json').write_text(json.dumps(evidence, ensure_ascii=False, indent=2),encoding='utf8')
    print('原生夏活24项、初音层叠、汪言等级/buff、普通/雅努斯四种原生统计方法及报告通过')
raise SystemExit(result.returncode)
