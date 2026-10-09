# -*- coding: utf-8 -*-
"""从 Android 1.0.128 monthly_signin/bonus 表生成签到固定奖励。"""
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
TABLE = ROOT / "out/client_catalogs/tables"


def rows(path):
    raw = json.loads(path.read_text(encoding="utf-8"))
    return raw["数据"]


def main():
    monthly = rows(TABLE / "monthly_signin.json")
    bonuses = rows(TABLE / "bonus.json")
    out = []
    for day in range(1, 32):
        row = monthly[str(day)]
        fixed = bonuses[str(row["bonus_id"])].get("fixed_items") or []
        out.append((day, fixed))
    target = ROOT / "internal/game/signin_catalog_generated.go"
    lines = ["package game", "", "// 由 generate_signin_catalog.py 从 Android 1.0.128 表生成。", "var androidMonthlySignin = map[int]map[int]int{"]
    for day, fixed in out:
        values = ", ".join("%d: %d" % (int(item[0]), int(item[1])) for item in fixed)
        lines.append("\t%d: {%s}," % (day, values))
    lines += ["}", ""]
    target.write_text("\n".join(lines), encoding="utf-8")
    print("生成签到表 %s 天" % len(out))


if __name__ == "__main__":
    main()
