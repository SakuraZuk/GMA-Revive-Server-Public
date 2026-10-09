"""只读核对已保存全服快照的卡牌目录、等级品阶和引导记录。"""
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
destination = ROOT / 'out/evidence/player-bugs-20261009'
if len(sys.argv) > 1:
    destination = Path(sys.argv[1]).resolve()
players = json.loads((destination / 'players.json').read_text(encoding='utf-8'))
rules = json.loads((ROOT / 'internal/game/card_oath_catalog.json').read_text(encoding='utf-8'))
issues, count = [], 0
for player in players:
    seen = set()
    state = player.get('state') or {}
    for card in state.get('cards', []):
        count += 1
        if (str(card['card_id']) not in rules['cards'] or card['level'] < 1
                or card['level'] > rules['caps'].get(str(card['grade']), 0)
                or card['exp'] < 0 or card['uuid'] in seen):
            issues.append({'uid': player['uid'], 'card': card})
        seen.add(card['uuid'])
result = {'角色数量': len(players), '核对卡牌数量': count, '等级品阶目录及UUID异常': issues,
          '界面引导空记录角色数量': sum(not (player.get('state') or {}).get('finished_guides') for player in players),
          '边界': '服务端存档数值；不代表客户端UI数值和完整技能演算已验收'}
(destination / 'state-audit.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps(result, ensure_ascii=False))
