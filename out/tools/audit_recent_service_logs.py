# -*- coding: utf-8 -*-
"""匿名统计发布后各服务日志，并验证独立热更兜底不记录请求正文。"""
import argparse
import json
import re
import shlex
import sys
from collections import OrderedDict
from datetime import datetime
from pathlib import Path

from deploy_hotfix_bridge import connect as connect_hotfix, run as run_hotfix
from server_ops import ROOT, connect, run


def count_patterns(body, patterns):
    # Go临时gctrace中的毫秒/堆大小不是HTTP状态或业务日志。
    business = "\n".join(line for line in body.splitlines() if not line.startswith("gc "))
    return {name: len(re.findall(pattern, business, re.I)) for name, pattern in patterns.items()}


def anonymous_client_errors(body):
    """只保留异常文本类型；移除日志前缀、玩家标识、地址及长数字。"""
    summaries = []
    for line in body.splitlines():
        if not re.search(r"hs_client_error|traceback|attributeerror", line, re.I):
            continue
        # RPC参数摘要可能含昵称、库存或其他玩家资料；这里只保留栈与错误类型。
        if ' has exception' in line:
            continue
        start = line.find('{"事件"')
        text = line[start:] if start >= 0 else "客户端异常（结构不可解析）"
        text = re.sub(r"(?i)(uid|oid|account|token|ticket|password)[=: \"]+[^, }\"]+", r"\1=[已脱敏]", text)
        text = re.sub(r"\b(?:\d{1,3}\.){3}\d{1,3}\b", "[地址已脱敏]", text)
        text = re.sub(r"\b[0-9a-fA-F]{16,}\b", "[标识已脱敏]", text)
        text = re.sub(r"\b\d{6,}\b", "[数字已脱敏]", text)
        summaries.append(text[:8000])
    return list(dict.fromkeys(summaries))[:12]


def anonymous_signal_lines(body, pattern):
    summaries = []
    for line in body.splitlines():
        if not re.search(pattern, line, re.I):
            continue
        text = re.sub(r"(?i)(uid|oid|account|token|ticket|password)[=: \"]+[^, }\"]+", r"\1=[已脱敏]", line)
        text = re.sub(r"\b(?:\d{1,3}\.){3}\d{1,3}\b", "[地址已脱敏]", text)
        text = re.sub(r"\b[0-9a-fA-F]{16,}\b", "[标识已脱敏]", text)
        text = re.sub(r"\b\d{6,}\b", "[数字已脱敏]", text)
        summaries.append(text[-800:])
    return summaries[:5]


def service_window(client, runner, unit, since):
    pid = runner(client, "systemctl show %s -p MainPID --value" % shlex.quote(unit))
    if not pid.isdigit() or int(pid) <= 0:
        raise RuntimeError("服务未运行：" + unit)
    # 必须覆盖整个服务窗口：进程被OOM终止后，仅过滤当前PID会漏掉异常及重启。
    body = runner(client, "journalctl -u %s --since %s --no-pager -o cat" %
                  (shlex.quote(unit), shlex.quote(since)))
    return pid, body


def timeout_operations(body):
    # 只输出源码固定方法名与次数，不保存错误行中的玩家标识和RPC参数。
    known = set()
    for source in (Path(ROOT) / 'internal/game').glob('*.go'):
        known.update(re.findall(r'"([a-z][a-z0-9_]{2,80})"', source.read_text(encoding='utf-8')))
    counts = {}
    for line in body.splitlines():
        if not re.search(r'context deadline exceeded|lock timeout|idle-in-transaction timeout', line, re.I):
            continue
        method = re.search(r'entity_message ([a-z][a-z0-9_]{2,80}) 业务失败', line)
        name = method[1] if method and method[1] in known else '未归类'
        if '后台周期结算失败' in line:
            name = '后台活动排行' if '活动排行结算' in line else '后台竞技与助战'
        elif '成为玩家失败' in line:
            name = '登录成为玩家'
        counts[name] = counts.get(name, 0) + 1
    return counts


def failed_rank_types(body):
    # gate只在业务失败后记录这两个目录编号；不关联或保存UID、账号与其他参数。
    counts = {}
    for line in body.splitlines():
        match = re.search(r'业务定位 method=query_rank_list 参数=(\[[^\]]*\])', line)
        if not match:
            continue
        try:
            args = json.loads(match[1])
        except ValueError:
            continue
        if len(args) != 2 or any(type(value) is not int for value in args):
            continue
        if args[0] not in (2, 4, 8, 10) or not 0 <= args[1] <= 99999999:
            continue
        label = '%d/%d' % tuple(args)
        counts[label] = counts.get(label, 0) + 1
    return counts


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--since", required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}", args.since):
        raise SystemExit("日志起点格式无效")

    common = OrderedDict([
        ("panic", r"(?:^|\s)panic(?:\s|:|$)"),
        ("fatal", r"(?:^|\s)fatal(?:\s|:|$)"),
        ("OOM", r"out of memory|oom-kill|oom_reaper"),
        ("HTTP5xx", r"(?:^|\s)5\d\d(?:\s|$)"),
        ("数据库超时", r"context deadline exceeded|lock timeout|idle-in-transaction timeout"),
        ("服务异常退出", r"Main process exited|Failed with result"),
        ("自动重启", r"Scheduled restart job"),
    ])
    game_patterns = OrderedDict(list(common.items()) + [
        ("登录结果", r"login_result"),
        ("登录完成", r"on_refresh_login"),
        ("用户验证失败", r"用户验证失败|user verification failed"),
        ("客户端异常", r"hs_client_error|traceback|attributeerror"),
        ("缺少援护面板", r"support_skills_panel|pvp_missing_support_panel"),
        ("PVP加载屏障完成", r"同步PVP加载屏障完成"),
        ("PVP已开始", r"PVP客户端原生开战已核验"),
        ("PVP已结算", r"PVP客户端权威末态已核验"),
        ("PVP原生推进重试", r"单端PVP原生推进失败|PVP单原生超时推进失败"),
        ("PVP推进因上下文取消", r"(?:单端PVP原生推进失败|PVP单原生超时推进失败)[^\n]*context canceled"),
        ("PVP播放确认拒绝", r"PVP播放确认拒绝|该原生命令没有播放确认规则|播放命令与服务端FIFO日志不符"),
        ("战斗事件序号故障", r"战斗事件序号跳号|战斗事件序号回退"),
        ("真人PVP已就绪", r"真人PVP实际加载与桥握手完成"),
        ("阵容形状拒绝", r"battle_fighting[^\n]{0,120}(?:阵容|形状|类型)"),
        ("空契印拒绝", r"(?:空[^\n]{0,30}契印|契印[^\n]{0,30}空)[^\n]{0,60}拒绝"),
        ("学会旧服号拒绝", r"(?:学会[^\n]{0,50}服编号|query_total_league_info[^\n]{0,80}拒绝)"),
    ])

    report = {"采样时间": datetime.now().isoformat(), "窗口起点": args.since, "服务": {}}
    game_client = connect()
    try:
        for unit in ("hs-game", "hs-sdk", "hs-login", "hs-postgres", "hs-dns"):
            pid, body = service_window(game_client, run, unit, args.since)
            row = {
                "PID": pid, "日志行数": len(body.splitlines()),
                "计数": count_patterns(body, game_patterns if unit == "hs-game" else common),
            }
            if unit == "hs-game":
                row["数据库超时操作计数"] = timeout_operations(body)
                row["排行榜失败目录编号计数"] = failed_rank_types(body)
                row["客户端异常匿名摘要"] = anonymous_client_errors(body)
                row["阵容拒绝匿名摘要"] = anonymous_signal_lines(
                    body, r"battle_fighting[^\n]{0,120}(?:阵容|形状|类型)")
                row["PVP推进重试匿名摘要"] = anonymous_signal_lines(
                    body, r"单端PVP原生推进失败|PVP单原生超时推进失败")
                row["PVP播放拒绝匿名摘要"] = anonymous_signal_lines(
                    body, r"PVP播放确认拒绝|该原生命令没有播放确认规则|播放命令与服务端FIFO日志不符")
                row["计数"]["PVP推进非取消失败"] = (
                    row["计数"]["PVP原生推进重试"] - row["计数"]["PVP推进因上下文取消"])
            report["服务"][unit] = row
    finally:
        game_client.close()

    marker = "HS_LOG_REDACTION_PROBE_20261011"
    hotfix_client = connect_hotfix()
    try:
        pid = run_hotfix(hotfix_client, "systemctl show hs-hotfix -p MainPID --value")
        run_hotfix(hotfix_client,
                   "curl -fsS -X POST --data-binary %s http://127.0.0.1:8082/index.php >/dev/null" % shlex.quote(marker))
        _, body = service_window(hotfix_client, run_hotfix, "hs-hotfix", args.since)
        counts = count_patterns(body, common)
        counts.update({"兜底脱敏摘要": body.count("内容不记录"), "探针正文泄露": body.count(marker)})
        report["服务"]["hs-hotfix"] = {"PID": pid, "日志行数": len(body.splitlines()), "计数": counts}
    finally:
        hotfix_client.close()

    leaked = report["服务"]["hs-hotfix"]["计数"]["探针正文泄露"]
    redacted = report["服务"]["hs-hotfix"]["计数"]["兜底脱敏摘要"]
    report["热更正文脱敏"] = "通过" if leaked == 0 and redacted > 0 else "失败"
    counts = report["服务"]["hs-game"]["计数"]
    if any(counts[name] for name in ("客户端异常", "数据库超时", "panic", "fatal", "OOM", "服务异常退出", "自动重启", "PVP推进非取消失败", "PVP播放确认拒绝", "战斗事件序号故障")):
        report["状态"] = "发现待处理异常"
    elif counts["PVP推进因上下文取消"]:
        report["状态"] = "存在PVP上下文取消，须结合断线与恢复证据判断"
    else:
        report["状态"] = "当前窗口未发现异常"
    report["边界"] = "仅匿名计数；不保存原始日志、玩家标识或请求正文；PVP实机通过仍需真实玩家完成一局。"
    target = Path(ROOT) / "out/recent-service-log-audit.json"
    target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False, indent=2))
    if report["热更正文脱敏"] != "通过":
        raise SystemExit(1)


if __name__ == "__main__":
    main()
