# -*- coding: utf-8 -*-
"""将本轮自建备份改为.bak，避免Go和后续MD读取把旧副本当现行源码。"""
from pathlib import Path
import sys, json, hashlib
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
target=(root/'out/backups/completion-20261007-222641').resolve()
assert target==root/'out/backups/completion-20261007-222641' and not target.is_symlink()
manifest=json.loads((target/'manifest.json').read_text(encoding='utf-8'))
for name,digest in manifest.items():
    original=(target/name).resolve()
    assert original.is_relative_to(target) and not original.is_symlink()
    if original.exists():
        assert hashlib.sha256(original.read_bytes()).hexdigest()==digest
        destination=Path(str(original)+'.bak')
        assert not destination.exists()
        original.rename(destination)
print('已保留并规范本轮备份：'+str(target))
