# -*- coding: utf-8 -*-
"""生成本项目Android收藏室数值及原生接口证据，拒绝从参考版拷贝数值。"""
import hashlib,json,sys,io
from pathlib import Path
from mem_marshal_extract import Loader
from neox_dis import disasm
from export_client_catalogs import instructions,text
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2];sources={}
def load(name):
 p=root/'out/client_catalogs/tables'/(name+'.json');raw=p.read_bytes();sources[name]={'路径':p.relative_to(root).as_posix(),'SHA256':hashlib.sha256(raw).hexdigest()};return json.loads(raw)['数据']
def select(data,fields):return {k:{f:r.get(f) for f in fields} for k,r in data.items()}
rooms=select(load('dormitory_attrs'),['ground_grids','hollow_infos','extend_infos','house_card_num','limit_furniture_count','room_system_name','cards_num_profit','house_daily_reward_id'])
facilities=select(load('facility_base'),['default_dormitory_id','related_data_sheet','mood_effect_range'])
levels={k:select(load(f['related_data_sheet']),['level','material_id','material_count','upgrade_bonus_id','produce_type','produce_material_id','produce_material_num','unit_time','storage_max','upgrade_effect','unit_bonus_material_num','unit_bonus_id','facility_card_reward']) for k,f in facilities.items()}
furniture=select(load('dormitory_furnitures'),['type','satisfaction','source','collect_bonus_id','exchange_flag','size','center','height','facility_id','theme','material_id'])
themes=select(load('furniture_theme'),['handbook_bonus_id'])
types=select(load('furniture_type'),['furniture_type','satisfaction_counts'])
suits=load('furniture_suits');comfort=load('dormitory_satisfaction');materials=select(load('materials'),['type','stype','target_id','limit_count'])
initial=load('init_config')['1']['init_funitures'];bonuses=load('bonus')
daily={}
for key,row in load('house_daily_reward').items():
 rules=[]
 for dungeon,expr,priority in row['house_daily_reward']:
  if not dungeon:continue
  code=expr['未执行表达式'];consts=code['常量'];sha=code['字节码SHA256']
  if sha=='2bcaa211e1bf9a6597a56cc8f62f73eb953523b0ec63fdd326839501547b3601':formula=['constant',consts[1]]
  elif sha=='871b0370cb20bc12330392467c1692d45e09f3c0f7ac5652b25c7f14d7360b72':formula=['comfort',consts[1]]
  elif sha=='c96bf67138b96a84e5f3579394ba4ed4361724832f9b8d3beabaa99e0fce39dc':formula=['comfort_offset',*consts[1:]]
  else:raise ValueError('未取证每日奖励表达式 '+sha)
  rules.append({'dungeon':dungeon,'priority':priority,'formula':formula,'bytecode_sha256':sha})
 daily[key]={'materials':row['daily_reward_material_type_id'],'rules':rules}
ep=root/'out/npk_scripts/android_base/875C5BCF.marshal';raw=ep.read_bytes();code=Loader(raw).r_object();ops=list(instructions(code));errors={text(code['names'][arg]):code['consts'][ops[i-1][2]] for i,(_,op,arg) in enumerate(ops) if i and op==116 and ops[i-1][1]==153 and isinstance(code['consts'][ops[i-1][2]],int)}
sources['error_code']={'路径':ep.relative_to(root).as_posix(),'SHA256':hashlib.sha256(raw).hexdigest()}
for mod in ['413834D1','F0E40550','5D1B302A','514DD3DE','0483009D','19AAEC42','FB66DE26','855ADC1E','696C3416','7D7F0010']:
 p=root/'out/npk_scripts/android_base'/(mod+'.marshal');raw=p.read_bytes();c=Loader(raw).r_object();buf=io.StringIO();disasm(c,out=buf);(root/'out/dis'/('collection-'+mod+'-native.asm')).write_text(buf.getvalue(),encoding='utf-8');sources[mod]={'路径':p.relative_to(root).as_posix(),'SHA256':hashlib.sha256(raw).hexdigest()}
result={'rooms':rooms,'facilities':facilities,'levels':levels,'furniture':furniture,'themes':themes,'types':types,'suits':suits,'comfort':comfort,'materials':materials,'bonuses':bonuses,'initial':initial,'errors':errors,'exp_interval':5,'cards':select(load('cards'),['tag']),'tags':load('cards_tag'),'tag_effects':load('character_tag_effect'),'daily_rewards':daily,'house_base':load('house_base'),'来源':sources}
(root/'internal/game/collection_catalog.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('已生成收藏室目录：房间',len(rooms),'设施',len(facilities),'家具',len(furniture),'主题',len(themes))
