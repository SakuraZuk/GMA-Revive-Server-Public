# -*- coding: utf-8 -*-
"""恢复2026-10-11误被公开示例配置覆盖的双端私有热更配置。"""
import hashlib
import json
import shlex
import sys
from datetime import datetime
from pathlib import Path

from deploy_hotfix_bridge import connect as connect_hotfix, run as run_hotfix
from server_ops import ROOT, connect, run

BAD_SHA = "3ad263c078507aba95d59f95c24a55b155601e06df8f9700638f907e389b2a94"
GOOD_SHA = "80fd4c28b86a8f9288833f198438fb682cc6f5a53318c20f9d8c008bf24fb5ca"
STAMP = "20261011-181836-325860"


def restore(client, runner, unit, target, backup):
    current = runner(client, "sha256sum %s" % shlex.quote(target)).split()[0]
    saved = runner(client, "sha256sum %s" % shlex.quote(backup)).split()[0]
    if current not in (BAD_SHA, GOOD_SHA):
        raise RuntimeError(unit + " 当前配置不是本次误覆盖或既有正式版本，拒绝恢复")
    if saved != GOOD_SHA:
        raise RuntimeError(unit + " 私有备份SHA不符")
    temp = target + ".recover-private-20261011"
    runner(client, "test ! -L %s && cp -a %s %s && test $(sha256sum %s | cut -d' ' -f1) = %s && mv -f %s %s" %
           tuple(map(shlex.quote, (target, backup, temp, temp, GOOD_SHA, temp, target))))
    runner(client, "systemctl restart %s && systemctl is-active --quiet %s" % (unit, unit))
    final = runner(client, "sha256sum %s" % shlex.quote(target)).split()[0]
    if final != GOOD_SHA:
        raise RuntimeError(unit + " 恢复后SHA不符")
    return {"服务": unit, "恢复前SHA256": current, "恢复后SHA256": final, "备份": backup}


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    rows = []
    hotfix = connect_hotfix()
    try:
        rows.append(restore(hotfix, run_hotfix, "hs-hotfix", "/opt/hs-hotfix/data/hotfix.json",
                            "/opt/hs-hotfix/releases/completion-%s/hotfix.json" % STAMP))
    finally:
        hotfix.close()
    game = connect()
    try:
        rows.append(restore(game, run, "hs-game", "/opt/hs-server/data/hotfix.json",
                            "/opt/hs-server/releases/completion-%s/hotfix.json" % STAMP))
    finally:
        game.close()
    report = {"时间": datetime.now().isoformat(), "状态": "双端私有热更配置已恢复", "文件": rows,
              "边界": "仅恢复误覆盖前hotfix.json；未回退游戏/热更服务二进制，未修改数据库或玩家数据。"}
    target = Path(ROOT) / "out/private-hotfix-recovery-20261011.json"
    target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
