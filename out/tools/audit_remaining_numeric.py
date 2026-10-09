"""扫描现行 Android marshal，保留数值缺口的精确引用与反汇编。"""
import hashlib
import io
import json
import sys
from pathlib import Path
from mem_marshal_extract import Loader
from neox_dis import disasm

ROOT = Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding="utf-8")
WORDS = {"promise_cards", "check_level", "house_frage_total_count", "house_frage_total_ssr_count", "per_count", "choice_params", "house_frage_move", "house_frage_ctrl_move"}
inventory = json.loads((ROOT / "out/npk_scripts/android_inventory.json").read_text(encoding="utf-8"))
hits = []
errors = []
for module in inventory["modules"]:
    path = ROOT / "out/npk_scripts" / module["marshal_file"]
    raw = path.read_bytes()
    try:
        code = Loader(raw).r_object()
    except Exception as exc:
        errors.append({"来源": str(path.relative_to(ROOT)), "错误": str(exc)})
        continue
    def visit(c, parent=""):
        name = c["name"].decode("utf-8") if isinstance(c["name"], bytes) else c["name"]
        fullname = parent + "/" + name
        names = {v.decode("utf-8") if isinstance(v, bytes) else str(v) for v in c["names"]}
        constants = {v.decode("utf-8", "backslashreplace") if isinstance(v, bytes) else v for v in c["consts"] if isinstance(v, (str, bytes))}
        variables = {v.decode("utf-8") if isinstance(v, bytes) else str(v) for v in c["varnames"]}
        matched = (names | constants | variables) & WORDS
        if matched:
            buf = io.StringIO()
            shallow = dict(c)
            shallow["consts"] = [v if not isinstance(v, dict) or v.get("type") != "code" else {"引用函数": str(v.get("name"))} for v in c["consts"]]
            disasm(shallow, out=buf)
            hits.append({"来源": str(path.relative_to(ROOT)), "SHA256": hashlib.sha256(raw).hexdigest(), "函数": fullname, "命中": sorted(matched), "反汇编": buf.getvalue()})
        for child in c["consts"]:
            if isinstance(child, dict) and child.get("type") == "code":
                visit(child, fullname)
    visit(code)
report = {"模块数": len(inventory["modules"]), "解析错误": errors, "命中函数": hits}
materials_path = ROOT / "out/client_catalogs/tables/materials.json"
materials_raw = materials_path.read_bytes()
materials = json.loads(materials_raw)["数据"]
random_materials = {mid: row for mid, row in materials.items() if row.get("type") == 4 and row.get("stype") == 10}
compose_path = ROOT / "internal/game/card_compose_catalog.json"
compose_raw = compose_path.read_bytes()
recipes = json.loads(compose_raw)["recipes"]
report["随机碎片可达性"] = {
    "材料表条目数": len(materials), "随机碎片材料数": len(random_materials),
    "随机碎片材料": random_materials, "实际合成目录数": len(recipes),
    "保底待确认目录数": sum(bool(r.get("unverified_promise")) for r in recipes.values()),
    "固定单卡目录数": sum(len(r["cards"]) == 1 for r in recipes.values()),
    "材料文件SHA256": hashlib.sha256(materials_raw).hexdigest(),
    "合成文件SHA256": hashlib.sha256(compose_raw).hexdigest(),
}
(ROOT / "out/remaining-numeric-native-scan.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
animation_sources = []
for module_id, method_names in {
    "FF2A17AE": {"get_random_materials", "get_end_materials", "get_random_props"},
    "1CBBBF23": {"on_roll_real_callback", "show_single_reward", "show_random_bonus"},
    "869A57A0": {"house_frage_move", "house_frage_ctrl_move"},
}.items():
    module = next(m for m in inventory["modules"] if m["hash"] == module_id)
    path = ROOT / "out/npk_scripts" / module["marshal_file"]
    raw = path.read_bytes()
    def selected(c, parent=""):
        name = c["name"].decode("utf-8") if isinstance(c["name"], bytes) else c["name"]
        fullname = parent + "/" + name
        if name in method_names:
            buf = io.StringIO()
            disasm(c, out=buf)
            animation_sources.append({"来源": str(path.relative_to(ROOT)), "SHA256": hashlib.sha256(raw).hexdigest(), "函数": fullname, "反汇编": buf.getvalue()})
        for child in c["consts"]:
            if isinstance(child, dict) and child.get("type") == "code":
                selected(child, fullname)
    selected(Loader(raw).r_object())
report["动画奖励协议取证"] = animation_sources
(ROOT / "out/remaining-numeric-native-scan.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps({"模块数": report["模块数"], "解析错误数": len(errors), "命中函数数": len(hits), "随机碎片可达性": report["随机碎片可达性"], "动画奖励协议函数数": len(animation_sources)}, ensure_ascii=False, indent=2))
