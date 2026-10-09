"""从已解码Android表生成归还材料和全卡池精确权重，保留来源哈希。"""
import hashlib
import json
from decimal import Decimal
from pathlib import Path

root = Path(__file__).resolve().parents[2]
tables = root / 'out/client_catalogs/tables'
sources = {}
def load(name):
    path = tables / (name + '.json')
    sources[name] = {'路径': str(path.relative_to(root)), 'SHA256': hashlib.sha256(path.read_bytes()).hexdigest()}
    return json.loads(path.read_text(encoding='utf-8'), parse_float=Decimal)['数据']

cards, rarities, levels = load('cards'), load('cards_rarity'), load('cards_level_exp')
returns = {}
for key, card in cards.items():
    if not isinstance(card, dict):
        continue
    special = card.get('speical_decompose_rule') or []
    returns[key] = {'materials': special or rarities.get(str(card.get('rarity')), {}).get('decompose_material', []),
                    'allowed': bool(special) or 5 not in (card.get('card_forbid_list') or []),
                    'battle_allowed': 1 not in (card.get('card_forbid_list') or []), 'rarity': card.get('rarity', 0)}
return_doc = {'来源': dict(sources), 'cards': returns,
              'levels': {k: {'exp': v['level_max_exp'], 'factor': float(v['exp_factor'])} for k,v in levels.items() if isinstance(v,dict)}}
(root/'internal/game/card_return_catalog.json').write_text(json.dumps(return_doc,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')

randoms, weights, probs = load('cards_random'), load('random_card_pool'), load('cards_prob')['键值条目']
pools = {}
for key, cfg in randoms.items():
    if cfg.get('gm_flag') or cfg.get('cycle_flag'):
        continue  # 轮换101需要原生周期处理，不拿静态阶段冒充轮换。
    exact = {}
    for step, values in weights[key].items():
        exact[step] = {cid:int(Decimal(value)*10000) for cid,value in values.items()}
        assert all(v>=0 for v in exact[step].values()) and sum(exact[step].values())>0
        assert all(cid in cards for cid in values)
    pools[key] = {'cost': cfg['consume_material'], 'limit':cfg.get('limit_count') or 0,
                  'guarantee':cfg.get('guarantee_rarity') or 0,'activity':cfg.get('activity_id') or 0,
                  'weights':exact, 'reset_groups':next((r['值'].get('reset_prob_groups') or [] for r in probs if r['值']['_id']==cfg['prob_id']),[])}
(root/'internal/game/gacha_native_catalog.json').write_text(json.dumps({'来源': sources,'pools':pools},ensure_ascii=False,separators=(',',':'))+'\n',encoding='utf-8')
print('归还目录',len(returns),'精确权重卡池',len(pools))
