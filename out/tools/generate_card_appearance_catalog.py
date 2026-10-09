"""从 Android 1.0.128 原生表导出队长与外观校验数据，不读取参考项目。"""
import json
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8")

ROOT = Path(__file__).resolve().parents[2]
CATALOG = ROOT / "out/client_catalogs"


def source(relative, key):
    document = json.loads((CATALOG / relative).read_text(encoding="utf-8"))
    if document["来源包"] != "android_base":
        raise ValueError("外观目录必须来自 Android 原始包")
    return document[key]


cards = source("tables/cards.json", "数据")
dresses = source("tables/cards_dress.json", "数据")
constants = source("configs/common_const.json", "直接常量")
lines = ["package game", "", "// 由 generate_card_appearance_catalog.py 从 Android 1.0.128 表生成，禁止手工修改。",
         "type cardAppearanceRule struct { DefaultDress, AwakenedDress int; Dresses, Forbid, Voices []int }",
         "var androidShowCardSlots = %d" % constants["MAX_SHOW_CARD_COUNT"],
         "var androidMikuActivityType = %d" % constants["ACTIVITY_TYPE_MIKU"],
         "var androidCaptainForbidden = %d" % constants["CARD_FORBID_CAPTAIN"],
         "var androidBattleForbidden = %d" % constants["CARD_FORBID_BATTLE"],
         "var androidFightingSlots = %d" % constants["FIGHTING_CARDS_NUM"],
         "var androidSupportSlots = %d" % constants["SUPPORT_CARDS_NUM"],
         "var androidCardAppearances = map[int]cardAppearanceRule{"]


def ints(values):
    return "[]int{" + ",".join(str(int(value)) for value in (values or [])) + "}"


for key, row in sorted(cards.items(), key=lambda pair: int(pair[0])):
    default = int(row.get("default_dress_id") or 0)
    allowed = row.get("dresses") or []
    if default and (default not in allowed or str(default) not in dresses):
        raise ValueError("默认外观与归属表不一致: " + key)
    for dress in allowed:
        if str(dress) not in dresses:
            raise ValueError("外观记录缺失: " + str(dress))
    voices = [entry[0] for entry in dresses.get(str(default), {}).get("cv_info", [])]
    lines.append("%s: {DefaultDress:%d, AwakenedDress:%d, Dresses:%s, Forbid:%s, Voices:%s}," %
                 (key, default, int(row.get("awakened_dress_id") or 0), ints(allowed), ints(row.get("card_forbid_list")), ints(voices)))
lines.extend(["}", "var androidDressNeedsAwakened = map[int]bool{"])
for key, row in sorted(dresses.items(), key=lambda pair: int(pair[0])):
    lines.append("%s: %s," % (key, "true" if row.get("need_awakened") else "false"))
lines.extend(["}", ""])
(ROOT / "internal/game/card_appearance_catalog_generated.go").write_text("\n".join(lines), encoding="utf-8")
print("Android 卡牌外观导出完成：%d 张卡牌，%d 种外观" % (len(cards), len(dresses)))
