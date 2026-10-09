# -*- coding: utf-8 -*-
"""从本版原始表生成已批准收藏室政策所需目录；不改原始表。"""
import json
import sys
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
root = Path(__file__).resolve().parents[2]
def table(name):
    return json.loads((root / 'out/client_catalogs/tables' / (name + '.json')).read_text(encoding='utf-8'))['数据']
rules = {}
for cid,row in table('house_daily_random_reward_rule').items():
    rules[cid] = [{'mood':int(condition['mood_status']), 'bonus':reward[0], 'raw_second':reward[1]} for condition,reward in row['house_random_reward']]
cards = {}
for cid,row in table('cards').items():
    if cid in rules:
        cards[cid] = {k:row[k] for k in ['mood_max','mood_transform_rate','mood_threshold']}
costs = {}
for facility,name in [(2,'facility_production1'),(3,'facility_production2'),(4,'facility_production3')]:
    costs[str(facility)] = {}
    for level,row in table(name).items():
        costs[str(facility)][level] = next((float(data) for effect,data in row['upgrade_effect'] if effect == 'efficiency_improve'),0)
data = {'windows':table('house_daily_random_reward'), 'rules':rules, 'cards':cards, 'energy_costs':costs, 'energy_per_card':30, 'energy_interval':30, 'tutorial_seconds':{'6004':9000,'26004':9000}}
(root/'internal/game/collection_remaining_catalog.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('目录完成：住客',len(rules),'张；能量设施',len(costs))
