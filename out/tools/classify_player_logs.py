# -*- coding: utf-8 -*-
"""按异常末行及业务入口聚合全服日志，不把带动态UUID的回包逐条当作新故障。"""
import collections
import json
import re
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding='utf-8')
path = Path(sys.argv[1])
rows = {}
methods = collections.Counter()
backend = collections.Counter()
battle = collections.Counter()
for line in path.read_text(encoding='utf-8').splitlines():
    match = re.search(r'客户端事件 uid=(\d+) oid=[0-9a-f]+ (\{.*\})$', line)
    if match:
        event = json.loads(match[2])
        if event.get('事件') != 'hs_client_error':
            continue
        message = event.get('内容', {}).get('message', '')
        if message.startswith('entity_message:call entity method '):
            continue  # 同一异常的第二条参数日志，不重复统计。
        key = message.strip().splitlines()[-1] if message.strip() else '空异常'
        row = rows.setdefault(key, {'次数': 0, '玩家': set()})
        row['次数'] += 1
        row['玩家'].add(int(match[1]))
    match = re.search(r'entity_message (\w+) 业务失败.*?: (.*)$', line)
    if match:
        methods[match[1]] += 1
        backend[match[2]] += 1
    for marker in ('真人持久房间读取失败', '真人在线租约刷新失败'):
        if marker in line:
            backend[marker] += 1
    if '战斗事件拒绝 ' in line and '原因=' in line:
        battle[line.split('原因=',1)[1]] += 1
result = {
    '日志': str(path),
    '客户端异常': [{'异常': key, '次数': row['次数'], '玩家数': len(row['玩家'])}
                   for key, row in sorted(rows.items(), key=lambda value: -value[1]['次数'])],
    '业务失败入口': dict(methods.most_common()),
    '服务端失败原因': dict(backend.most_common()),
    '战斗事件拒绝原因': dict(battle.most_common()),
    '边界': '排除同一次异常的实体参数副日志；客户端限流上报次数不是实际总故障数',
}
path.with_name('日志分类.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps(result, ensure_ascii=False, indent=2))
