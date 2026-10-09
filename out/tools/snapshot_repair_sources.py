# -*- coding: utf-8 -*-
"""无 Git 工作区的修复前快照；不读取密钥或环境凭据。"""
import hashlib
import json
import shutil
import sys
from datetime import datetime
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8")
root = Path(__file__).resolve().parents[2]
destination = root / "out/backups" / datetime.now().strftime("full-repair-%Y%m%d-%H%M%S")
paths = list((root / "internal/game").rglob("*.go"))
paths += [root / name for name in ("SERVER.md", "out/HANDOFF.md", "out/PROGRESS.md", "out/REPAIR-TASKS.md")]
manifest = {}
for path in paths:
    relative = path.relative_to(root)
    target = destination / (str(relative) + ".bak")
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(path, target)
    manifest[str(relative)] = hashlib.sha256(path.read_bytes()).hexdigest()
(destination / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8")
print("修复前快照：" + str(destination))
