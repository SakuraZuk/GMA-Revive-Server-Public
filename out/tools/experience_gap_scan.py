"""列出当前Android商店奖励类型、特殊条件及空掉落套装池，供业务审查。"""
import json,sys,collections
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
c=json.loads((root/'internal/game/shop_catalog.json').read_text(encoding='utf-8'))
m=c['materials'];by=collections.defaultdict(list)
for id,r in c['commodities'].items():
 a=m.get(str(r['item_id']),{})
 by[(a.get('type'),a.get('stype'))].append((id,r['item_id'],r.get('commodity_type')))
print('商品奖励类型',json.dumps({str(k):v[:6] for k,v in by.items()},ensure_ascii=False))
print('每类型总数',{str(k):len(v) for k,v in by.items()})
conditions=collections.defaultdict(list)
for id,r in c['commodities'].items():
 for f in ('unlock_condition','invalid_condition'):
  for group in r.get(f,[]) or []:
   for kind,value in group:
    if kind not in (1,2,11):conditions[kind].append((id,f,value))
print('其余条件',json.dumps(conditions,ensure_ascii=False))
print('开服期限商品',[(id,r['open_server_days']) for id,r in c['commodities'].items() if r.get('open_server_days') is not None])
