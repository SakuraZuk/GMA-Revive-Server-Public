# -*- coding: utf-8 -*-
"""核对全部原表关卡、地点、章节索引及服务端派生目录，不执行或跳过剧情。"""
import hashlib
import json
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding='utf-8')
root = Path(__file__).resolve().parents[2]
destination = root / 'out/evidence/player-bugs-20261009/all-stages'
destination.mkdir(parents=True, exist_ok=True)
sources = {}
def read_table(name):
    path = root / 'out/client_catalogs/tables' / (name + '.json')
    value = json.loads(path.read_text(encoding='utf-8'))
    sources[name] = {'原包SHA256': value['来源SHA256'], '导出SHA256': hashlib.sha256(path.read_bytes()).hexdigest()}
    return value['数据']

dungeons = read_table('dungeons')
sites = read_table('sites')
index = read_table('chapter_site_dungeon')
derived = json.loads((root/'internal/game/dungeon_catalog.json').read_text(encoding='utf-8'))['dungeons']
issues, entries = [], []
for key, row in sorted(dungeons.items(), key=lambda pair: int(pair[0])):
    output = {'副本': int(key), '类型': row['dungeon_type'], '地点': row['site_id'],
              '停用': bool(row['_dungeon_disable']), '战斗': row['dungeon_battle_id'] or 0,
              '开场剧情': row['dungeon_event'], '后续剧情': row['map_post_event_id']}
    expected = derived.get(key)
    if expected is None or expected['battle_id'] != output['战斗'] or expected['type'] != output['类型']:
        issues.append({'副本': int(key), '问题': '服务端派生战斗编号或类型与原表不同'})
    output['形态'] = '战斗' if output['战斗'] > 0 else '无战斗节点'
    output['地点存在'] = str(row['site_id']) in sites
    entries.append(output)
for key, values in index['site_dict'].items():
    for dungeon in values:
        row = dungeons.get(str(dungeon))
        if row is None or str(row['site_id']) != key:
            issues.append({'地点': int(key), '副本': dungeon, '问题': '地点索引缺失副本或地点归属不一致'})
for key, values in index['site_unlock_dict'].items():
    if key not in sites or any(str(value) not in sites for value in values):
        issues.append({'地点': int(key), '问题': '原表地点解锁索引引用不存在地点'})
for key, chapters in index['chapter_dict'].items():
    for site, values in chapters.items():
        if site not in sites or int(sites[site]['chapter_id']) != int(key):
            issues.append({'章节': int(key), '地点': int(site), '问题': '原表章节索引地点归属不一致'})
story = [v['副本'] for v in entries if not v['停用'] and v['类型'] in (1, 17) and v['战斗'] <= 0]
report = {'状态': '目录核对完成', '关卡数': len(entries), '地点数': len(sites), '章节索引数': len(index['chapter_dict']),
          '主线支线纯剧情数': len(story), '主线支线纯剧情': story, '来源': sources, '问题': issues, '关卡': entries,
          '边界': '覆盖全部原表与派生编号、地点和章节引用；不是全部资源渲染、剧情交互、活动规则或Android通关验收'}
(destination/'全部关卡目录核对.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({key: report[key] for key in ('状态','关卡数','地点数','章节索引数','主线支线纯剧情数','问题')},ensure_ascii=False))
