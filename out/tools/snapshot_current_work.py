# -*- coding: utf-8 -*-
"""全量施工前保留源码与现行文档，只读取项目文本，不复制凭据。"""
from pathlib import Path
from datetime import datetime
import hashlib, json, shutil, sys

sys.stdout.reconfigure(encoding='utf-8')

root = Path(__file__).resolve().parents[2]
target = root / 'out/backups' / ('completion-' + datetime.now().strftime('%Y%m%d-%H%M%S'))
files = list((root/'internal').rglob('*.go')) + list((root/'internal').rglob('*.sql'))
files += list((root/'internal/game').glob('admin_ui.*'))
files += [root/'SERVER.md', root/'out/HANDOFF.md', root/'out/REPAIR-TASKS.md', root/'out/PROGRESS.md']
manifest = {}
for file in files:
    name = file.relative_to(root)
    dst = target/(str(name)+'.bak')
    dst.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(file, dst)
    manifest[str(name)] = hashlib.sha256(file.read_bytes()).hexdigest()
(target/'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
print('源码备份：'+str(target))
