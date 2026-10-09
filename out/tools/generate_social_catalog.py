# -*- coding: utf-8 -*-
"""生成 Android 社交规则、错误码及竞技奖励表，保留来源校验。"""
import hashlib
import json
import sys
from pathlib import Path
from mem_marshal_extract import Loader

ROOT = Path(__file__).resolve().parents[2]

def main():
    sources = {}
    def load(path):
        raw = path.read_bytes()
        sources[str(path.relative_to(ROOT))] = hashlib.sha256(raw).hexdigest()
        return json.loads(raw)
    constants = load(ROOT / "out/client_catalogs/configs/common_const.json")["直接常量"]
    selected = {k: v for k, v in constants.items() if (k.startswith(("MAX_FRIEND", "MAX_BLACK", "ARCHIVE_", "RECORD_TYPE_", "RELATION_TYPE_", "ASSIST_LIMIT_", "CHALLENGE_", "MAX_CHALLENGE_")) or k in ("SYNC_PVP_BATTLE",)) and isinstance(v, int)}
    inventory = load(ROOT / "out/npk_scripts/android_inventory.json")
    row = next(r for r in inventory["modules"] if r["hash"] == "875C5BCF")
    path = ROOT / "out/npk_scripts" / row["marshal_file"]
    sources[str(path.relative_to(ROOT))] = hashlib.sha256(path.read_bytes()).hexdigest()
    code = Loader(path.read_bytes()).r_object()
    b = code["bytecode"]
    errors = {}
    offset = 0
    while offset < len(b):
        if b[offset] == 153 and offset + 6 <= len(b) and b[offset + 3] == 116:
            name = code["names"][int.from_bytes(b[offset+4:offset+6], "little")]
            name = name.decode("utf-8") if isinstance(name, bytes) else name
            if name.startswith(("RET_FRIEND_", "RET_ARCHIVE_", "RET_ASSIST_", "RET_CHALLENGE_")) or name in ("RET_CARD_NOT_EXIST", "RET_FAILED", "RET_SUCCESS"):
                errors[name] = code["consts"][int.from_bytes(b[offset+1:offset+3], "little")]
        offset += 1 if b[offset] < 90 else 3
    tables = {}
    for name in ["cards", "cards_base", "cards_archive", "sync_pvp_weekly_win_bonus", "sync_pvp_rank_bonus", "bonus", "mail_template", "asyn_pvp_base_rule", "asyn_pvp_bonus_rule", "asyn_pvp_score_rule", "assist_rule", "assist_passive_bonus", "assist_active_bonus", "assist_bonus", "dungeons"]:
        tables[name] = load(ROOT / ("out/client_catalogs/tables/" + name + ".json"))["数据"]
    out = ROOT / "internal/game/social_catalog.json"
    out.write_text(json.dumps({"source": sources, "constants": selected, "errors": errors, "tables": tables}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print("已生成社交规则", len(errors), "个错误码")

if __name__ == "__main__":
    sys.stdout.reconfigure(encoding="utf-8")
    main()
