import json
from pathlib import Path
base=Path('out/client_catalogs/tables')
for name in ['cards_prob','cards_pool','cards_promise_rule','cards_rarity','bonus','dungeons','cards_enhance','cards_level','cards_level_exp']:
    p=json.loads((base/(name+'.json')).read_text(encoding='utf-8'))
    data=p.get('数据',{})
    print('\n###',name,'entries',len(data))
    for k,v in list(data.items())[:3]: print(k, json.dumps(v,ensure_ascii=False)[:1200])
