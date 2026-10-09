"""从Android表生成碎片合成目录，保留来源哈希；未知保底语义不猜测。"""
import hashlib
import json
import sys
from pathlib import Path

root=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding="utf-8")
source={}
def load(name):
    path=root/"out/client_catalogs/tables"/(name+".json")
    raw=path.read_bytes()
    source[name]={"路径":str(path.relative_to(root)),"SHA256":hashlib.sha256(raw).hexdigest()}
    return json.loads(raw)["数据"]
materials=load("materials");cards=load("cards");rarity=load("cards_rarity")
rules=load("random_frage_rules");bonus=load("bonus");libs=load("item_libs");weights=load("item_libs_data")
recipes={}
for mid,m in materials.items():
    if not isinstance(m,dict) or m.get("type")!=4:continue
    target=str(m.get("target_id"))
    if m.get("stype")==1:
        card=cards.get(target,{})
        if not card or card.get("disable") or not card.get("can_compose"):continue
        need=rarity[str(card["rarity"])]["fragment_count"]
        recipes[mid]={"need":need,"cards":[{"card_id":int(target),"weight":1}]}
    elif m.get("stype")==10:
        rule=rules.get(target)
        if not rule:continue
        b=bonus[str(rule["bonus_id"])]
        candidates=[]
        rows=b.get("random_item_libs") or []
        if len(rows)!=1 or rows[0][1:]!=[1,1.0,1] or any(b.get(key) for key in ("fixed_items","random_items","random_runes","inner_bonus_id")):
            raise ValueError("随机碎片奖励结构未支持")
        for key,pair in weights[str(rows[0][0])].items():
            item=libs[key]
            if pair[1] or item.get("unlock_condition") or item.get("item_lib") or item["item_count"]!=[1]:raise ValueError("随机碎片有未支持条件")
            output=materials[str(item["item_id"])]
            cid=str(output["target_id"])
            if output["type"]!=5 or cid not in cards:raise ValueError("奖励不是幻书材料")
            if cards[cid].get("disable"):continue
            candidates.append({"card_id":int(cid),"weight":int(pair[0])})
        recipes[mid]={"need":rule["fragment_count"],"cards":candidates,"unverified_promise":bool(rule.get("check_level") or rule.get("promise_cards"))}
result={"来源":source,"recipes":recipes,"max_cards":2000,"max_compose":50,"常量来源":"37C6CA84 common_const.py MAX_CARD_COUNT/CARD_COMPOSE_MAX_NUMBER"}
(root/"internal/game/card_compose_catalog.json").write_text(json.dumps(result,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
print("已生成合成配方",len(recipes))
