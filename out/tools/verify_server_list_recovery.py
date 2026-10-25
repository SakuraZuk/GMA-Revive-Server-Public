# -*- coding: utf-8 -*-
"""只读验证独立热更的客户端服务器列表链路，不输出真实地址。"""
import json
import re
import shlex
import sys
from datetime import datetime
from pathlib import Path

from deploy_hotfix_bridge import connect, run
from server_ops import ROOT


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    client = connect()
    try:
        checks = {}
        commands = {
            "后端直连": "curl -fsS http://127.0.0.1:8082/server_list_public.txt",
            "客户端HTTPS": "curl -kfsS --resolve h62.update.netease.com:443:127.0.0.1 https://h62.update.netease.com/h62/server_list_public.txt",
            "客户端HTTP": "curl -fsS --resolve h62.update.netease.com:80:127.0.0.1 http://h62.update.netease.com/h62/server_list_public.txt",
        }
        for name, command in commands.items():
            body = run(client, command)
            endpoints = re.findall(r"(?<![\d.])(?:\d{1,3}\.){3}\d{1,3}:\d+", body)
            checks[name] = bool("ANDIOSNETEASE" in body and "network=bgp" in body and len(endpoints) >= 4 and
                                len(set(endpoints[:4])) == 1 and "192.0.2." not in body)
        sha = run(client, "sha256sum /opt/hs-hotfix/data/hotfix.json").split()[0]
        pid = run(client, "systemctl show hs-hotfix -p MainPID --value")
        active = run(client, "systemctl is-active hs-hotfix") == "active"
    finally:
        client.close()
    report = {"时间": datetime.now().isoformat(), "状态": "通过" if active and all(checks.values()) else "失败",
              "独立热更PID": pid, "私有配置SHA256": sha, "服务器列表链路": checks,
              "边界": "仅验证格式、路由和非示例地址；不输出真实服务器地址。"}
    (Path(ROOT) / "out/server-list-recovery-verification.json").write_text(
        json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False, indent=2))
    if report["状态"] != "通过":
        raise SystemExit(1)


if __name__ == "__main__":
    main()
