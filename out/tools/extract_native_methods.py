# -*- coding: utf-8 -*-
"""静态提取 Android 方法字节码，保留源哈希，不执行恢复代码。"""
import argparse
import hashlib
import io
import json
import sys
from pathlib import Path
from mem_marshal_extract import Loader
from neox_dis import disasm

sys.stdout.reconfigure(encoding="utf-8")
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("module")
parser.add_argument("methods", nargs="+")
parser.add_argument("--output")
args = parser.parse_args()
root = Path(__file__).resolve().parents[2]
path = root / "out/npk_scripts/android_base" / (args.module + ".marshal")
patch = root / "out/npk_scripts/android_patch" / path.name
if patch.exists():
    path = patch
raw = path.read_bytes()
stream = io.StringIO()
stream.write("源文件：" + str(path.relative_to(root)) + "\nSHA256：" + hashlib.sha256(raw).hexdigest() + "\n")

def label(value):
    return value.decode("utf-8") if isinstance(value, bytes) else str(value)

found = []
def visit(code, prefix=""):
    name = label(code["name"])
    full = prefix + "." + name if prefix else name
    if name in args.methods or full in args.methods:
        found.append(full)
        disasm(code, prefix=prefix, out=stream)
        return
    for child in code.get("consts", []):
        if isinstance(child, dict) and child.get("type") == "code":
            visit(child, full)

visit(Loader(raw).r_object())
if not found:
    raise SystemExit("未找到所选原生方法")
content = stream.getvalue()
if args.output:
    destination = root / args.output
    destination.write_text(content, encoding="utf-8")
    print(json.dumps({"输出": str(destination.relative_to(root)), "方法": found}, ensure_ascii=False))
else:
    print(content)
