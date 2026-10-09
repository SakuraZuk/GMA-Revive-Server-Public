# -*- coding: utf-8 -*-
"""从 Android 1.0.128 表生成同步 PVP 机器人兜底目录。"""
import hashlib
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
TABLES = ROOT / "out/client_catalogs/tables"
OUT = ROOT / "internal/game/sync_pvp_ai_catalog.json"


def load(name):
    path = TABLES / name
    raw = path.read_bytes()
    value = json.loads(raw)
    return value.get("数据", {}), hashlib.sha256(raw).hexdigest()


def main():
    names = ["sync_pvp_rule.json", "sync_pvp_score_rule.json",
             "sync_pvp_robot_rule.json", "sync_pvp_robot_match.json",
             "sync_pvp_robot_time.json", "robot_config.json",
             "robot_card_config.json", "robot_rune_config.json",
             "sync_pvp_ghost_scores.json"]
    data = {}
    source = {}
    for name in names:
        data[name[:-5]] , digest = load(name)
        source[name] = {"path": str((TABLES / name).relative_to(ROOT)), "sha256": digest}
    roles, digest = load("role_info.json")
    source["role_info.json"] = {"path": str((TABLES / "role_info.json").relative_to(ROOT)), "sha256": digest}
    cards, digest = load("cards.json")
    source["cards.json"] = {"path": str((TABLES / "cards.json").relative_to(ROOT)), "sha256": digest}
    data["robot_card_skills"] = {str(row["card_id"]): {"skills": roles[str(row["card_id"])]["skill_list"], "support_skills": roles[str(row["card_id"])]["support_skill_list"], "default_dress": cards[str(row["card_id"])]["default_dress_id"], "rarity": cards[str(row["card_id"])]["rarity"]} for row in data["robot_card_config"].values()}
    OUT.write_text(json.dumps({"source": source, "tables": data}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print("已生成", OUT.relative_to(ROOT), "源表", len(names))


if __name__ == "__main__":
    sys.stdout.reconfigure(encoding="utf-8")
    main()
