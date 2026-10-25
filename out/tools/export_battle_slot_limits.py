# -*- coding: utf-8 -*-
"""合并本版原生阵型容量进既有剧情槽目录，保留固定角色语义。"""
import hashlib, json
from pathlib import Path
root = Path(__file__).resolve().parents[2]
folder = root / 'out/client_catalogs/tables'
paths = [folder / (name + '.json') for name in ('battle_info', 'formation_data')]
battles, formations = [json.loads(path.read_text(encoding='utf-8'))['数据'] for path in paths]
target = root / 'internal/game/battle_story_slots.json'
doc = json.loads(target.read_text(encoding='utf-8'))
limits = {}
for key, row in battles.items():
    sizes = [len(formations.get(row[name], [])) if row[name] else 0 for name in ('main_formation', 'main_support_formation')]
    if sizes[0] > 4 or sizes[1] > 2:
        assert 0 < sizes[0] <= 7 and 0 <= sizes[1] <= 4
        limits[key] = sizes
doc['原生阵型来源'] = {path.name: hashlib.sha256(path.read_bytes()).hexdigest() for path in paths}
doc['formation_limits'] = limits
target.write_text(json.dumps(doc, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print('已合并原生特殊阵型容量：' + str(len(limits)) + '项；没有修改原版固定角色槽。')
