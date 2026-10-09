import json
p=json.load(open('out/client_catalogs/tables/dungeons.json',encoding='utf-8'))['数据']
for k,v in p.items():
    if isinstance(v,dict) and v.get('dungeon_type')==1 and (v.get('first_bonus') or v.get('bonus')):
        print(k, v.get('dungeon_battle_id'), v.get('first_bonus'), v.get('bonus'), v.get('need_power'))
