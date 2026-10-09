"""幻书补完目录：仅导出Android1.0.128源表，附原文件哈希。"""
import hashlib,json,sys
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
sources={}
def load(name):
    p=root/'out/client_catalogs/tables'/(name+'.json')
    raw=p.read_bytes()
    sources[name]={'路径':p.relative_to(root).as_posix(),'SHA256':hashlib.sha256(raw).hexdigest()}
    return json.loads(raw)['数据']
cards=load('cards')
result={'cards':{i:{'rarity':r['rarity'],'forbid':r['card_forbid_list']} for i,r in cards.items()},'rarities':load('cards_rarity'),'enhancements':load('cards_enhance'),'来源':sources}
(root/'internal/game/card_growth_catalog.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('幻书补完目录',len(result['cards']),len(result['rarities']))
