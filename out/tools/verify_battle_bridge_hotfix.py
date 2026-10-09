# -*- coding: utf-8 -*-
"""校验启动热修与可选的完整客户端扩展。"""
import argparse
import hashlib
import json
import sys
from pathlib import Path
from merge_battle_bridge_hotfix import (
    BRIDGE_MARKER,
    RUNTIME_WRAPPER,
    compose_startup,
    compose_runtime,
    extension_script,
    extract_startup_extension,
    startup_script,
)

ROOT = Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding="utf-8")
catalog = json.loads((ROOT / "deploy/data/hotfix.json").read_text(encoding="utf-8"))
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--startup-only", action="store_true",
                    help="只校验生产启动前缀，不要求并行开发中的扩展正文已合并")
args = parser.parse_args()
bridge = (ROOT / "internal/game/battle_bridge_script.py").read_text(encoding="utf-8")
source = bridge.split("\n", 4)[4]
digest = hashlib.sha256(source.encode("utf-8")).hexdigest()
for fragment in (
    "def finish_instance_sequence(self, method, *args):",
    "activate_type == common_const.EVENT_ACTION_INSTANCE",
    "and not self.sequence_actions",
    "and self.wait_event_signal is not None",
    "self.event_all_triggered()",
    "shadow.run_battle_storyline_end = storyline_end",
    "shadow.battle_guide_end = guide_end",
    "def real_story(self, *args, **kwargs):",
    "shadow.real_run_battle_storyline = real_story",
    "if (not self.storyline_contents",
    "def trigger_sequence_actions(self):",
    "return self.trigger_all_sequence_actions()",
    "shadow.client_trigger_sequence_action = trigger_sequence_actions",
):
    assert fragment in source, fragment
prefix = startup_script()
assert sorted(catalog["startup_scripts"]) == ["1.0.125", "1.0.128"]
for version, script in catalog["startup_scripts"].items():
    assert script == compose_startup(extract_startup_extension(script)), version
    assert script.startswith(prefix[:-len("\n_hs_install()")].rstrip()), version
    assert "\n    " + BRIDGE_MARKER in script, version
    assert script.endswith("\n_hs_install()"), version
    assert "orig_import = B.__import__" in script, version
    assert "_hs_import = __builtin__.__import__" not in script, version
    assert script.count(BRIDGE_MARKER) == 1, (version, BRIDGE_MARKER)

if args.startup_only:
    print("启动热修校验通过 版本=%s runtime.index=%s" %
          (sorted(catalog["startup_scripts"]), catalog["runtime"]["index"]))
    raise SystemExit(0)

combined = extension_script()
assert catalog["runtime"]["script"] == compose_runtime(combined)
assert catalog["runtime"]["index"] >= 2026100805
assert catalog["runtime"]["script"].startswith(RUNTIME_WRAPPER + "\n")
assert catalog["runtime"]["script"].endswith("\n_hs_runtime_install()")
for version, script in catalog["startup_scripts"].items():
    assert script == compose_startup(combined), version
    for marker in ('# REVIVAL_BATTLE_BRIDGE', '# HS_ACTIVITY_SCHEDULES_BEGIN', '# HS_ACTIVITY_METRICS_BEGIN', '# HS_ACTIVITY_BUFFS_BEGIN', '# HS_LOGIN_BATTLE_RECOVERY_BEGIN', '# HS_HUMAN_BRIDGE_BEGIN', '# HS_NATIVE_RECORD_BEGIN'):
        assert script.count(marker) == 1, (version, marker)
assert ("# sha256=" + digest) in bridge.split("\n", 4)[0:4]
print("战斗桥热修清单校验通过 sha256=%s runtime.index=%s" % (digest, catalog["runtime"]["index"]))
