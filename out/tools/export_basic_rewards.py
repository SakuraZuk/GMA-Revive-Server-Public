"""导出本版基础任务、活跃度、签到和补给规则，保留逐文件来源哈希。"""
import hashlib
import json
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding='utf-8')

ROOT = Path(__file__).resolve().parents[2]
tables, sources = {}, {}
for name in ('new_task', 'new_task_params', 'daily_task', 'daily_task_bonus',
             'weekly_task_bonus', 'check_in_bonus', 'bonus_buff', 'base_target', 'score_bonus'):
    path = ROOT / ('out/client_catalogs/tables/' + name + '.json')
    raw = path.read_bytes()
    tables[name] = json.loads(raw)['数据']
    sources[str(path.relative_to(ROOT))] = hashlib.sha256(raw).hexdigest()
target = ROOT / 'internal/game/basic_rewards_catalog.json'
target.write_text(json.dumps({'表': tables, '来源SHA256': sources}, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print('已导出本版基础奖励目录：' + str(target))
