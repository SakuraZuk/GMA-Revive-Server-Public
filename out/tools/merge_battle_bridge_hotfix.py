# -*- coding: utf-8 -*-
"""幂等生成启动热修并合并客户端扩展。"""
import argparse
import json
import sys
import shlex
import textwrap
from pathlib import Path
ROOT = Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding="utf-8")
CATALOG = ROOT / "deploy" / "data" / "hotfix.json"
STARTUP = ROOT / "deploy" / "data" / "hotfix_startup_v10.py"
BRIDGE = ROOT / "internal" / "game" / "battle_bridge_script.py"
BRIDGE_MARKER = "# REVIVAL_BATTLE_BRIDGE"
RUNTIME_WRAPPER = "def _hs_runtime_install():"


def startup_script():
    script = STARTUP.read_text(encoding="utf-8").rstrip()
    if "orig_import = B.__import__" not in script:
        raise RuntimeError("启动热修没有使用闭包保存原始 import")
    if "_hs_import = __builtin__.__import__" in script:
        raise RuntimeError("启动热修仍包含失效的全局 import 引用")
    return script


def extract_startup_extension(script):
    marker_at = script.find(BRIDGE_MARKER)
    if marker_at < 0:
        raise RuntimeError("启动热修缺少扩展边界")
    line_at = script.rfind("\n", 0, marker_at) + 1
    extension = script[line_at:]
    if extension.rstrip().endswith("_hs_install()"):
        extension = extension.rstrip()[:-len("_hs_install()")].rstrip()
    return textwrap.dedent(extension).rstrip()


def compose_startup(extension):
    base = startup_script()
    call = "\n_hs_install()"
    if not base.endswith(call):
        raise RuntimeError("启动热修缺少最终安装调用")
    body = base[:-len(call)].rstrip()
    wrapped = textwrap.indent(extension.rstrip(), "    ")
    return body + "\n\n" + wrapped + "\n\n_hs_install()"


def extract_runtime_extension(script):
    script = script.rstrip()
    if not script.startswith(RUNTIME_WRAPPER):
        return script
    lines = script.splitlines()
    if lines[0] != RUNTIME_WRAPPER or lines[-1] != "_hs_runtime_install()":
        raise RuntimeError("运行期热修闭包结构无效")
    return textwrap.dedent("\n".join(lines[1:-1])).strip()


def compose_runtime(extension):
    extension = extract_runtime_extension(extension)
    if not extension:
        raise RuntimeError("运行期热修正文为空")
    return (RUNTIME_WRAPPER + "\n" + textwrap.indent(extension, "    ")
            + "\n\n_hs_runtime_install()")


def extension_script():
    bridge = BRIDGE.read_text(encoding='utf-8')
    rows = json.loads((ROOT / 'deploy/data/activity-schedules.json').read_text(encoding='utf-8'))
    keys = ('activity_id', 'begin_time', 'end_time', 'enabled', 'permanent')
    rows = [{key: row[key] for key in keys} for row in sorted(rows, key=lambda row: row['activity_id'])]
    raw = json.dumps(rows, ensure_ascii=False, separators=(',', ':'))
    template = (ROOT / 'internal/game/activity_schedule_script.py').read_text(encoding='utf-8')
    activity = template.replace('__HS_ACTIVITY_SCHEDULES_JSON__', json.dumps(raw, ensure_ascii=False))
    env={}
    for line in (ROOT/'deploy/data/gameplay-rules.env').read_text(encoding='utf-8').splitlines():
        if line.strip() and not line.lstrip().startswith('#'):
            key,_,value=line.partition('=');words=shlex.split(value)
            if len(words)!=1:raise ValueError('正式活动规则环境值无效')
            env[key]=words[0]
    enabled=env.get('HS_REMAINING_GAMEPLAY_POLICY')=='local-20261008-v1' and env.get('HS_SPECIAL_DRILL_REOPEN')=='timber-12-20261008'
    activity=activity.replace('__HS_SPECIAL_DRILL_REOPEN__','True' if enabled else 'False')
    human = (ROOT / 'internal/game/human_battle_bridge_script.py').read_text(encoding='utf-8')
    metrics = (ROOT / 'internal/game/activity_metrics_script.py').read_text(encoding='utf-8')
    buffs = (ROOT / 'internal/game/activity_buffs_script.py').read_text(encoding='utf-8')
    login_recovery = (ROOT / 'internal/game/login_battle_recovery_script.py').read_text(encoding='utf-8')
    diagnostic = (ROOT / 'internal/game/client_diagnostic_script.py').read_text(encoding='utf-8')
    summon_timer = (ROOT / 'internal/game/summon_timer_script.py').read_text(encoding='utf-8')
    ui_repair = (ROOT / 'internal/game/ui_callback_repair_script.py').read_text(encoding='utf-8')
    record = (ROOT / 'internal/game/native_record_bridge_script.py').read_text(encoding='utf-8')
    authority = (ROOT / 'internal/game/native_authority_bridge_script.py').read_text(encoding='utf-8')
    return (bridge + '\n\n# HS_ACTIVITY_SCHEDULES_BEGIN\n' + activity + '\n# HS_ACTIVITY_SCHEDULES_END\n'
            + '\n\n# HS_ACTIVITY_METRICS_BEGIN\n' + metrics + '\n# HS_ACTIVITY_METRICS_END\n'
            + '\n\n# HS_ACTIVITY_BUFFS_BEGIN\n' + buffs + '\n# HS_ACTIVITY_BUFFS_END\n'
            + '\n\n# HS_LOGIN_BATTLE_RECOVERY_BEGIN\n' + login_recovery + '\n# HS_LOGIN_BATTLE_RECOVERY_END\n'
            + '\n\n# HS_CLIENT_DIAGNOSTIC_BEGIN\n' + diagnostic + '\n# HS_CLIENT_DIAGNOSTIC_END\n'
            + '\n\n# HS_SUMMON_TIMER_BEGIN\n' + summon_timer + '\n# HS_SUMMON_TIMER_END\n'
            + '\n\n# HS_UI_CALLBACK_REPAIR_BEGIN\n' + ui_repair + '\n# HS_UI_CALLBACK_REPAIR_END\n'
            + '\n\n# HS_HUMAN_BRIDGE_BEGIN\n' + human + '\n# HS_HUMAN_BRIDGE_END\n'
            + '\n\n# HS_NATIVE_RECORD_BEGIN\n' + record + '\n# HS_NATIVE_RECORD_END\n'
            + '\n\n# HS_NATIVE_PVP_AUTHORITY_BEGIN\n' + authority + '\n# HS_NATIVE_PVP_AUTHORITY_END\n')


def main(startup_only=False, runtime_only=False):
    if startup_only and runtime_only:
        raise RuntimeError("不能同时指定仅启动与仅运行期模式")
    catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
    startup = catalog.setdefault("startup_scripts", {})
    if not startup:
        raise RuntimeError("热修清单缺少 startup_scripts")
    if startup_only:
        for version, script in list(startup.items()):
            startup[version] = compose_startup(extract_startup_extension(script))
        CATALOG.write_text(json.dumps(catalog, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print("已仅替换启动热修：版本=%s runtime.index=%s" %
              (sorted(startup), catalog.get("runtime", {}).get("index")))
        return

    if runtime_only:
        runtime = catalog.setdefault("runtime", {})
        runtime["script"] = compose_runtime(runtime.get("script", ""))
        runtime["index"] = int(runtime.get("index", 0)) + 1
        CATALOG.write_text(json.dumps(catalog, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print("已仅修复运行期热修闭包：runtime.index=%s" % runtime["index"])
        return

    bridge = extension_script()
    for version, script in list(startup.items()):
        startup[version] = compose_startup(bridge)
    runtime = catalog.setdefault("runtime", {})
    runtime["index"] = max(int(runtime.get("index", 0)), 2026100911)
    runtime["script"] = compose_runtime(bridge)
    CATALOG.write_text(json.dumps(catalog, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print("已合并战斗桥：版本=%s runtime.index=%s" % (sorted(startup), runtime["index"]))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--startup-only", action="store_true",
                        help="仅替换启动前缀，保留当前扩展正文与 runtime index")
    parser.add_argument("--runtime-only", action="store_true",
                        help="仅包装现有运行期正文并递增 runtime index")
    args = parser.parse_args()
    main(args.startup_only, args.runtime_only)
