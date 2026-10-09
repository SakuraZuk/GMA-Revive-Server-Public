# -*- coding: utf-8 -*-
"""从已校验客户端表生成服务端内嵌基线；保留输入哈希和原始字段。"""
import hashlib
import json
import sys
from pathlib import Path
from mem_marshal_extract import Loader
from export_client_catalogs import instructions, text

ROOT=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')
def direct_constants(module):
    code=Loader((ROOT/'out/npk_scripts/android_base'/module).read_bytes()).r_object()
    ops=list(instructions(code))
    return {text(code['names'][arg]):(text(code['consts'][ops[i-1][2]]) if isinstance(code['consts'][ops[i-1][2]],(bytes,str)) else code['consts'][ops[i-1][2]]) for i,(offset,op,arg) in enumerate(ops) if i and op==116 and ops[i-1][1]==153}
names=['init_config','avatar_level_power','guide_task','guide','materials','nickname_spec','dungeons','battle_info','instance_event','instance_trigger','role_info','skill','skill_effect','new_task','system_unlock']
tables={}
evidence={}
for name in names:
    path=ROOT/'out/client_catalogs/tables'/(name+'.json')
    raw=path.read_bytes()
    table=json.loads(raw)
    assert table['状态']=='静态结构还原通过'
    tables[name]=table['数据']
    evidence[name]={'路径':str(path.relative_to(ROOT)), '导出SHA256':hashlib.sha256(raw).hexdigest(), '模块SHA256':table['来源SHA256']}
config=json.loads((ROOT/'out/client_catalogs/configs/common_const.json').read_text(encoding='utf-8'))
print('配置结构：'+str(list(config)))
constants=config.get('直接常量',config.get('常量',{}))
assert constants['AVATAR_POWER_INTERVAL']>0
value={'来源':evidence,'初始化':tables['init_config']['1'],'等级体力':tables['avatar_level_power'],'引导任务':tables['guide_task'],
       '界面引导':tables['guide'],
       '材料定义':{str(mid):tables['materials'][str(mid)] for mid,count in tables['init_config']['1']['init_materials']},
       '体力恢复间隔':constants['AVATAR_POWER_INTERVAL'],'每次恢复体力':constants['AVATAR_POWER_PER_INTERVAL']}
const=direct_constants('B41789AA.marshal')
direct=direct_constants('C1E7CE13.marshal')
value['取名规则']={'min':const['NICKNAME_MIN_LEN'],'max':const['NICKNAME_MAX_LEN'],'words':direct['VALID_WORD'],
                 'special':''.join(row['special_word'] for row in tables['nickname_spec'].values())}
guide_dungeons={str(int(row['do_type_params'][0])) for row in tables['guide_task'].values() if row['do_type']==2}
value['教学副本']={key:tables['dungeons'][key] for key in guide_dungeons if key in tables['dungeons']}
value['教学战斗']={str(row['dungeon_battle_id']):tables['battle_info'][str(row['dungeon_battle_id'])] for row in value['教学副本'].values()}
# 触发器/事件闭包收集：从每场战斗的 start_triggers 出发，
# 沿 trigger_events → instance_event、trigger_ids/extra_trigger_events → instance_trigger 递归展开。
def collect_battle_triggers(battle):
    triggers={}
    events={}
    pending_t=[int(t) for t in (battle.get('start_triggers') or [])]
    while pending_t:
        tid=pending_t.pop()
        key=str(tid)
        if key in triggers or key not in tables['instance_trigger']:
            continue
        row=tables['instance_trigger'][key]
        triggers[key]=row
        pending_t.extend(int(e) for e in (row.get('trigger_events') or []))
        for eid in (row.get('trigger_events') or []):
            ekey=str(eid)
            if ekey in events or ekey not in tables['instance_event']:
                continue
            erow=tables['instance_event'][ekey]
            events[ekey]=erow
            pending_t.extend(int(t) for t in (erow.get('trigger_ids') or []))
            pending_t.extend(int(t) for t in (erow.get('extra_trigger_events') or []) if t in tables['instance_trigger'])
            for sub in (erow.get('extra_trigger_events') or []):
                if str(sub) in tables['instance_event'] and str(sub) not in events:
                    events[str(sub)]=tables['instance_event'][str(sub)]
    return triggers,events
value['教学波次触发']={}
value['教学波次事件']={}
wave_role_ids=set()
for bid,battle in value['教学战斗'].items():
    triggers,events=collect_battle_triggers(battle)
    value['教学波次触发'].update(triggers)
    value['教学波次事件'].update(events)
    battle['_start_triggers']=[int(t) for t in (battle.get('start_triggers') or [])]
    # 开场演出时长：battle_start 触发器事件链（含 extra_trigger_events）的 event_time 最大值
    # （10001 为 EV11(0)→extra EV13(1.5)，演出串行累计约 1.5 秒）。
    enter=None
    for tid in battle['_start_triggers']:
        row=triggers.get(str(tid)) or {}
        if row.get('trigger_type')!='battle_start':
            continue
        for eid in (row.get('trigger_events') or []):
            erow=events.get(str(eid)) or {}
            for t in (erow.get('event_time'),*(tables['instance_event'].get(str(s),{}).get('event_time') for s in (erow.get('extra_trigger_events') or []))):
                if t is not None and (enter is None or t>enter):
                    enter=t
    battle['_enter_show_time']=enter
for key,row in value['教学波次事件'].items():
    for rid in (row.get('role_ids') or []):
        wave_role_ids.add(int(rid))
teach_roles=set()
teach_factors={}
for bid,battle in value['教学战斗'].items():
    teach_roles.update(battle.get('my_avatar_list') or [])
    teach_roles.update(battle.get('enemy_avater_list') or [])
    teach_factors[bid]={'hp':battle.get('enemy_hp_factor'),'atk':battle.get('enemy_atk_factor'),'defence':battle.get('enemy_defence_factor')}
value['教学战斗系数']=teach_factors
role_rows={}
for rid in sorted(teach_roles|wave_role_ids):
    row=tables['role_info'].get(str(rid))
    assert row, '缺少角色属性 %d'%rid
    role_rows[str(rid)]={'role_id':rid,'hp':row['hp'],'atk':row['atk'],'defence':row['defence'],'ap_speed':row['ap_speed'],'skill_list':row['skill_list']}
value['战斗角色']=role_rows
def find_records(table,field,wanted):
    """复合键容器递归检索：按字段值唯一取行（skill 与 skill_effect 记录均以 skill_id 自标识）。"""
    unique={}
    def walk(item):
        if isinstance(item,dict):
            value=item.get(field)
            if isinstance(value,int) and value in wanted and value not in unique:
                unique[value]=item
            for child in item.values():walk(child)
        elif isinstance(item,list):
            for child in item:walk(child)
    walk(table)
    return unique
all_skill_ids={sid for row in role_rows.values() for sid in row['skill_list']}
skill_rows=find_records(tables['skill'],'skill_id',all_skill_ids)
effect_wanted=set()
for sid in all_skill_ids:
    effect_wanted.update(skill_rows[sid].get('skill_effect') or [])
effect_rows=find_records(tables['skill_effect'],'skill_id',effect_wanted)
def first_effect_rate(skill_id):
    row=skill_rows.get(skill_id)
    assert row, '缺少技能 %s'%skill_id
    rates=[];act=float(row.get('skill_act_time') or 0)
    for eid in (row.get('skill_effect') or []):
        effect=effect_rows.get(eid)
        if not effect: continue
        groups=effect.get('effect_rates')
        if not groups: continue
        group=groups[0]
        percent=effect.get('effect_rate_percent')
        rates.append({'rate':group[0],'grow':group[1] if len(group)>1 else None,'percent':percent if percent is not None else 1.0})
    return {'act_time':act,'effects':rates}
value['技能效果']={}
for rid,row in role_rows.items():
    entries=[first_effect_rate(sid) for sid in row['skill_list']]
    entries=[e for e in entries if e['effects']]
    assert entries, '角色 %s 无可用伤害技能'%rid
    value['技能效果'][rid]=entries
# new_task 全量（成就/累计任务计数的服务端表驱动基线）。
value['新任务']={key:row for key,row in tables['new_task'].items()}
# system_unlock（键值条目 → 扁平列表：system/version/unlock_condition；条件类型 1=通关副本）。
value['系统解锁']=[{'system':r['值']['system_name'],'version':r['值']['version'],'conditions':r['值']['unlock_condition'] or []}
                  for r in (tables['system_unlock'].get('键值条目') or [])]
(ROOT/'internal/game/player_defaults.json').write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('已生成客户端基线，初始体力=%s 触发器=%d 事件=%d 新任务=%d'%(value['初始化']['init_power'],len(value['教学波次触发']),len(value['教学波次事件']),len(value['新任务'])))
