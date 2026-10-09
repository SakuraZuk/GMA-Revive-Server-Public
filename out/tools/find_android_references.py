"""从原生脚本清单查找字段引用，不执行客户端代码。"""
import argparse
import json
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8")
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("names", nargs="+")
args = parser.parse_args()
root = Path(__file__).resolve().parents[2]
manifest = json.loads((root / "out/npk_scripts/android_base/manifest.json").read_text(encoding="utf-8"))
for entry in manifest["entries"]:
    for method in entry.get("methods", []):
        matched = sorted(set(args.names) & set(method.get("referenced_names", [])))
        if matched:
            print(json.dumps({"模块": entry["filename"], "文件": entry["file"],
                              "方法": method["name"], "引用": matched}, ensure_ascii=False))
