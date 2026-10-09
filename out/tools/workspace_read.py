# -*- coding: utf-8 -*-
"""按 UTF-8 读取文档与取证文件，支持分段和完整 Markdown 清单。"""
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8")
root = Path(__file__).resolve().parents[2]
if sys.argv[1] == "--markdown":
    for path in sorted(root.rglob("*")):
        if path.is_file() and path.suffix.lower() == ".md":
            print(f"{path.relative_to(root)}\t{len(path.read_text(encoding='utf-8-sig').splitlines())}行")
else:
    path = root / sys.argv[1]
    lines = path.read_text(encoding="utf-8-sig").splitlines()
    start = int(sys.argv[2]) if len(sys.argv) > 2 else 1
    count = int(sys.argv[3]) if len(sys.argv) > 3 else len(lines)
    for number, line in enumerate(lines[start - 1:start - 1 + count], start):
        print(f"{number}: {line}")
