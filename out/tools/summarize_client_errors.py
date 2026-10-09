"""汇总全玩家异常末行、人数和时间，避免输出庞大的堆栈。"""
import json
import re
import sys
from collections import defaultdict
from pathlib import Path

sys.stdout.reconfigure(encoding='utf-8')
groups = defaultdict(list)
for line in Path(sys.argv[1]).read_text(encoding='utf-8').splitlines():
    if '客户端事件 ' not in line or 'hs_client_error' not in line:
        continue
    try:
        event = json.loads(line[line.index('{'):])
        message = event['内容']['message']
        key = message.strip().splitlines()[-1]
        groups[key].append((line[:25], re.search(r'uid=(\d+)', line).group(1)))
    except (ValueError, KeyError, AttributeError):
        continue
for key, records in sorted(groups.items(), key=lambda row: -len(row[1])):
    print(json.dumps({'异常': key, '次数': len(records), '玩家数': len(set(r[1] for r in records)), '最早': records[0][0], '最后': records[-1][0]}, ensure_ascii=False))
