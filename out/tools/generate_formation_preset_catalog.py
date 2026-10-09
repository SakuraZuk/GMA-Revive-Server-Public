# -*- coding: utf-8 -*-
"""从 Android 1.0.128 导出阵容预设所需的最小契约目录。"""
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
TABLES = ROOT / "out/client_catalogs/tables"
OUT = ROOT / "internal/game/formation_preset_catalog.json"


def load(name):
    path = TABLES / name
    raw = path.read_bytes()
    return json.loads(raw), hashlib.sha256(raw).hexdigest(), path


def main():
    cards, cards_sha, cards_path = load("cards.json")
    duties, duties_sha, duties_path = load("mini_duty.json")
    score, score_sha, score_path = load("sync_pvp_score_rule.json")
    card_rules = {}
    for key, value in cards["数据"].items():
        if not isinstance(value, dict):
            continue
        card_rules[str(key)] = {
            "position": value.get("card_position"),
            "disable": value.get("disable", 0),
            "gm_flag": value.get("gm_flag", 0),
            "forbid": value.get("card_forbid_list") or [],
        }
    data = {
        "constants": {
            "preset_record_num": 20,
            "preset_record_name_len": 7,
            "fighting_cards_num": 4,
            "support_cards_num": 2,
            "support_unlock": "support",
            "battle_forbid": 1,
        },
        "cards": card_rules,
        "mini_duty": duties.get("数据", {}),
        "sync_pvp_score_rule": score.get("数据", {}),
        "source": {
            "cards": {"path": str(cards_path.relative_to(ROOT)), "sha256": cards_sha},
            "mini_duty": {"path": str(duties_path.relative_to(ROOT)), "sha256": duties_sha},
            "sync_pvp_score_rule": {"path": str(score_path.relative_to(ROOT)), "sha256": score_sha},
        },
    }
    OUT.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print("已生成 %s cards=%d duties=%d score_rules=%d" % (
        OUT.relative_to(ROOT), len(card_rules), len(data["mini_duty"]), len(data["sync_pvp_score_rule"])))


if __name__ == "__main__":
    main()
