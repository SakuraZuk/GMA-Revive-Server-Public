"""归并探索、幻书委托与绝密任务的当前 Android 表，保留来源哈希。"""
import hashlib
import json
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8")
ROOT = Path(__file__).resolve().parents[2]
names = ["free_stage_nodes", "free_stage_site", "free_stage_site_event", "sites", "chapter", "chapter_site_dungeon", "task_tower_info", "task_tower_dungeon", "tower_task", "consign_base", "consign_task", "consign_refresh_pool", "consign_task_pool", "consign_bonus_pool", "consign_level", "facility_consign", "house_base"]
sources, tables = {}, {}
for name in names:
    path = ROOT / "out/client_catalogs/tables" / (name + ".json")
    raw = path.read_bytes()
    doc = json.loads(raw)
    if doc["来源包"] not in ("android_base", "android_patch"):
        raise ValueError("必须使用现行Android表")
    sources[name] = {"路径": str(path.relative_to(ROOT)), "文件SHA256": hashlib.sha256(raw).hexdigest(), "模块SHA256": doc["来源SHA256"]}
    tables[name] = doc["数据"]
target = ROOT / "internal/game/remaining_gameplay_catalog.json"
target.write_text(json.dumps({"来源": sources, "表": tables}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print("当前Android底层玩法目录生成：%d张表" % len(tables))
