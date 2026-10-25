# -*- coding: utf-8 -*-
"""只读采集指定发布进程的完整窗口；哈希不符时拒绝混用旧进程证据。"""
import argparse
import json
import re
from datetime import datetime
from pathlib import Path

from server_ops import connect, run

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--expected-sha', required=True)
parser.add_argument('--since', required=True)
parser.add_argument('--label', required=True)
args = parser.parse_args()
if not re.fullmatch(r'[0-9a-f]{64}', args.expected_sha):
    raise SystemExit('预期哈希格式无效')
if not re.fullmatch(r'\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}', args.since):
    raise SystemExit('起始时间格式无效')
if not re.fullmatch(r'[\w\-]+', args.label):
    raise SystemExit('证据标签格式无效')
client = connect()
try:
    pid = run(client, 'systemctl show hs-game -p MainPID --value')
    if not pid.isdigit() or int(pid) <= 0:
        raise SystemExit('游戏进程未运行')
    sha = run(client, 'sha256sum /proc/' + pid + '/exe').split()[0]
    if sha != args.expected_sha:
        raise SystemExit('当前运行进程不属于预期候选，未采样')
    raw = run(client, "journalctl -u hs-game _PID=" + pid + " --since '" + args.since + "' --no-pager -o short-iso", timeout=90)
    lines = [line for line in raw.splitlines() if '推送编码' not in line]
    report = {
        '采样时间': datetime.now().isoformat(), '窗口起点': args.since,
        '游戏PID': pid, '运行哈希': sha, '日志行数': len(lines),
        '周期日志': [line for line in lines if '周期存档' in line or '后台周期结算' in line],
        '错误计数': {word: sum(word in line for line in lines) for word in
                  ['lock timeout', 'context deadline exceeded', 'idle-in-transaction timeout']},
        '边界': '完整指定窗口的运行证据；没有错误不等于全部玩法通过',
    }
    root = Path(__file__).resolve().parents[2] / 'out/evidence/qq-bugs-20261010'
    root.mkdir(parents=True, exist_ok=True)
    (root / (args.label + '.log')).write_text('\n'.join(lines) + '\n', encoding='utf-8')
    body = json.dumps(report, ensure_ascii=False, indent=2)
    (root / (args.label + '.json')).write_text(body + '\n', encoding='utf-8')
    print(body)
finally:
    client.close()
