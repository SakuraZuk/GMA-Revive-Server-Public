"""导出Android送礼规则和回礼配置，保留来源哈希供审查。"""
import hashlib
import json
import sys
from pathlib import Path
from fractions import Fraction
from mem_marshal_extract import Loader
from export_client_catalogs import instructions, text

sys.stdout.reconfigure(encoding="utf-8")
sys.stderr.reconfigure(encoding="utf-8")
ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / "out/client_catalogs"
sources = {}


def load(name, category="tables", key="数据"):
    path = BASE / category / (name + ".json")
    raw = path.read_bytes()
    doc = json.loads(raw)
    if doc["来源包"] != "android_base":
        raise ValueError("必须使用Android原始表：" + name)
    sources[name] = {"路径": str(path.relative_to(ROOT)), "文件SHA256": hashlib.sha256(raw).hexdigest(), "模块SHA256": doc["来源SHA256"]}
    return doc[key]


constants = load("common_const", "configs", "直接常量")
materials = load("materials")
gifts = load("gift")
cards = load("cards")
levels = load("cards_intimacy")
raises = load("intimacy_raise")
special = load("special_gift_rule")
presents = load("special_gift_present")
bonuses = load("bonus")
drops = load("runes_drops")
head_boxes = load("head_box_info")
system_hide = load("system_hide")
fragments = load("card_to_fragment")
error_path = ROOT / "out/npk_scripts/android_base/875C5BCF.marshal"
error_raw = error_path.read_bytes()
error_code = Loader(error_raw).r_object()
ops = list(instructions(error_code))
errors = {text(error_code["names"][arg]): error_code["consts"][ops[i-1][2]]
          for i, (_, op, arg) in enumerate(ops)
          if i and op == 116 and ops[i-1][1] == 153 and isinstance(error_code["consts"][ops[i-1][2]], int)}
sources["error_code"] = {"路径": str(error_path.relative_to(ROOT)), "文件SHA256": hashlib.sha256(error_raw).hexdigest()}
exported = {}
for mid, mat in materials.items():
    if mat.get("type") != constants["MATERIAL_TYPE_CONSUME"] or mat.get("stype") != constants["MATERIAL_STYPE_CONSUME_INTIMACY_GIFT"]:
        continue
    target = str(mat["target_id"])
    if target not in gifts:
        raise ValueError("礼物关联配置缺失：" + mid)
    gift = gifts[target]
    favored = Fraction(mat.get("intimacy_value") or 0) * (1 + Fraction(str(raises["1"]["increase_value"])))
    if favored.denominator != 1:
        raise ValueError("偏好礼物增量存在未取证的舍入规则：" + mid)
    exported[mid] = {"gift_id": int(target), "intimacy": mat.get("intimacy_value") or 0,
                     "favored_intimacy": favored.numerator,
                     "like_tag": gift.get("like_tag"), "return_bonus_id": gift.get("return_bonus_id") or 0,
                     "forbid_special_gift": gift.get("forbid_special_gift") or 0}
card_rules = {}
for cid, card in cards.items():
    tags = card.get("tag") or []
    favored = tags[1][0] if len(tags) > 1 and tags[1] else None
    card_rules[cid] = {"favored_tag": favored, "bonuses": card.get("card_intimacy_bonus") or [], "rarity": card["rarity"], "disabled": card.get("disable") or 0, "not_in_stat": card.get("not_in_stat") or 0}
needed = {v["return_bonus_id"] for v in exported.values() if v["return_bonus_id"]}
needed.update(bid for card in card_rules.values() for bid in card["bonuses"])
bonus_rows = {str(bid): bonuses[str(bid)] for bid in needed}
material_ids = {int(row[0]) for bonus in bonus_rows.values() for row in bonus.get("fixed_items", [])}
material_ids.update(fragments.values())
material_ids.update(int(cid) for cid in cards if cid in materials)
reward_materials = {str(mid): materials[str(mid)] for mid in material_ids}
drop_ids = {int(row[0]) for bonus in bonus_rows.values() for row in bonus.get("random_runes", [])}
drop_rows = {str(did): drops[str(did)] for did in drop_ids}
for gift in exported.values():
    gift["return_runes"] = []
    bid = gift["return_bonus_id"]
    if not bid:
        continue
    bonus = bonus_rows[str(bid)]
    if any(bonus.get(key) for key in ("fixed_items", "inner_bonus_id", "random_items", "random_item_libs")):
        raise ValueError("送礼回礼含未实现奖励类型：" + str(bid))
    for drop_id, counts, chance in bonus["random_runes"]:
        if counts != [1] or chance != 1:
            raise ValueError("回礼数量或概率需要新增原生取证：" + str(drop_id))
        drop = drop_rows[str(drop_id)]
        def fixed_index(weights):
            positive = [i + 1 for i, weight in enumerate(weights) if weight > 0]
            if len(positive) != 1 or any(weight < 0 for weight in weights):
                raise ValueError("回礼含非固定掉落：" + str(drop_id))
            return positive[0]
        suits = [sid for sid, weight in drop["suit_id_weight"] if weight > 0]
        if len(suits) != 1 or any(weight for _, weight in drop["extra_suit_id_weight"]):
            raise ValueError("回礼含未取证额外套装：" + str(drop_id))
        gift["return_runes"].append({"spec": {"suit": suits[0], "pos": fixed_index(drop["pos_weight"]),
                                                "star": fixed_index(drop["star_weight"]), "level": 1, "extra_suit": 0},
                                     "marks": drop["mark_base_attrs"]})
for gift in exported.values():
    # 精确整数可以直接入账；非整数保留缺口，不引入没有原生证据的舍入。
    gift["special_amounts"] = {}
    gift["special_favored_amounts"] = {}
    gift["unverified_special_amounts"] = {}
    for choice, multiplier in special["1"]["choice_params"].items():
        for field, value in (("special_amounts", gift["intimacy"]), ("special_favored_amounts", gift["favored_intimacy"])):
            amount = Fraction(value) * Fraction(str(multiplier))
            if amount.denominator != 1:
                gift["unverified_special_amounts"][field + ":" + str(choice)] = str(amount)
                continue
            gift[field][choice] = amount.numerator
maximum_level = levels[str(max(map(int, levels)))]
maximum = maximum_level.get("max_value") or maximum_level["min_value"]
result = {"来源": sources, "gifts": exported, "cards": card_rules, "levels": levels,
          "favor_ratio": raises["1"]["increase_value"], "special_gift_rule": special,
          "special_presents": presents,
          "bonuses": bonus_rows, "rune_drops": drop_rows, "max_intimacy": maximum,
          "materials": reward_materials, "head_boxes": head_boxes,
          "card_to_fragment": fragments,
          "constants": {name: constants[name] for name in ("RARITY_N", "MATERIAL_TYPE_HEADBOX", "MATERIAL_TYPE_SPECIAL_FRAGE")},
          "support_skill_hidden": system_hide["support_skill_level"]["hide_enable"], "errors": errors}
target = ROOT / "internal/game/intimacy_catalog.json"
target.write_text(json.dumps(result, ensure_ascii=False, sort_keys=True, indent=2) + "\n", encoding="utf-8")
print("Android送礼材料：%d，回礼配置：%d，上限：%d" % (len(exported), len(bonus_rows), maximum))
print("章节材料配置：%d，错误码：%d" % (len(reward_materials), len(errors)))
for bid, row in bonus_rows.items():
    if any(row.get(key) for key in ("inner_bonus_id", "random_items", "random_runes", "random_item_libs")):
        print("非固定奖励", bid, json.dumps(row, ensure_ascii=False))
