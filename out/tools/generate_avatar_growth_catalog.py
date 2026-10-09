"""导出馆主等级经验及体力上限，保留Android原表文件哈希。"""
import hashlib
import json
import sys
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
ROOT = Path(__file__).resolve().parents[2]
sources = {}
def load(name):
    path = ROOT / 'out/client_catalogs/tables' / (name + '.json')
    raw = path.read_bytes()
    doc = json.loads(raw)
    if doc['来源包'] != 'android_base':
        raise ValueError('等级规则必须来自Android原表')
    sources[name] = {'路径': str(path.relative_to(ROOT)), '文件SHA256': hashlib.sha256(raw).hexdigest(), '模块SHA256': doc['来源SHA256']}
    return doc['数据']
limits = load('avatar_level')
levels = load('avatar_level_exp')
powers = load('avatar_level_power')
if any(row['level_up_bonus_id'] for row in levels.values()):
    raise ValueError('等级奖励需要新增原表解释，不能静默略过')
result = {'来源': sources, 'limits': limits, 'levels': levels, 'powers': powers}
(ROOT/'internal/game/avatar_growth_catalog.json').write_text(json.dumps(result, ensure_ascii=False, sort_keys=True, indent=2)+'\n', encoding='utf-8')
print('馆主等级经验已导出：%d条，原表等级奖励均为空' % len(levels))
