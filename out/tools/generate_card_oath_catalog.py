"""从当前 Android 表生成誓约及知识升级规则，保留来源哈希。"""
import hashlib
import json
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8")
sys.stderr.reconfigure(encoding="utf-8")
root = Path(__file__).resolve().parents[2]
sources = {}
def load(name):
    path = root / "out/client_catalogs/tables" / (name + ".json")
    raw = path.read_bytes()
    sources[name] = {"路径": path.relative_to(root).as_posix(), "SHA256": hashlib.sha256(raw).hexdigest()}
    return json.loads(raw)["数据"]

cards = load("cards")
rings = load("rings")
materials = load("materials")
caps = load("cards_max_level")
levels = load("cards_level_exp")
intimacy = load("cards_intimacy")
limits = load("cards_level")
for key, row in levels.items():
    if row["level_max_exp"] < 0 or (row["level_max_exp"] == 0 and int(key) != limits["max"]):
        raise ValueError("等级经验必须为正数")
    for group in row.get("unlock_condition") or []:
        if not group or any(len(c) != 2 or c[0] != 2 or not isinstance(c[1], int) for c in group):
            raise ValueError("存在未取证的升级门槛")
result = {
    "来源": sources,
    "常量来源": "37C6CA84: MATERIAL_STYPE_CONSUME_RING=30 / MATERIAL_ID_EXP_POOL=4 / CARD_FORBID_GROWUP=2",
    "cards": {key: {"forbid": row.get("card_forbid_list") or [], "upgrade": row.get("upgrade_material") or []} for key, row in cards.items()},
    "rings": {key: row["max_level"] for key, row in rings.items()},
    "materials": {key: row["target_id"] for key, row in materials.items() if row["type"] == 4 and row["stype"] == 30},
    "caps": {key: row["max_level"] for key, row in caps.items()},
    "levels": {key: {"exp": row["level_max_exp"], "conditions": row.get("unlock_condition") or []} for key, row in levels.items()},
    "ring_intimacy_level": min(int(k) for k, row in intimacy.items() if row.get("can_give_ring")),
    "max_grade": max(map(int, caps)),
    "normal_max_level": limits["grade_max_level"],
}
if any(str(rid) not in rings for rid in result["materials"].values()):
    raise ValueError("戒指材料引用缺失")
(root / "internal/game/card_oath_catalog.json").write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print("已生成誓约目录与70级知识升级门槛")
