# -*- coding: utf-8 -*-
"""只验证共享桥的调用/屏障/重放控制，不模拟Android战斗数值或宣称MuMu验收。"""
import sys
import types
from pathlib import Path


def main():
    events = []
    native_calls = []
    retries = []

    class Info:
        eid = "1"
        def get_attr(self, key):
            return {"hex_coord": (0, 0, 0), "master": "123456789012345678901234", "card_uuid": "234567890123456789012345", "hp": 100, "max_hp": 100, "ap": 0}.get(key)
        def get_camp_id(self): return 1
        def get_role_id(self): return 4401
        def is_support(self): return False
        def is_dead(self): return False

    class Shadow:
        _revival_bridge_version = 12
        def __init__(self):
            self.extra_info = {"human_shared": True}
            self.id = "345678901234567890123456"
            self.action_counter = 1
            self.entity_infos = {"1": Info()}
            self._revival_event_sequence = 0
            self._revival_event_begin = 0
            self._revival_result_sent = False
            self.winner_eid_list = None
            self.timer_cleared = 0
            self.auto_battle_set = {"enemy"}
        def wait_for_player_input(self, info): return "原生等待"
        def wait_for_master_input(self, master): return "原生计时器"
        def change_auto_state(self, state): return "原生自动"
        def notify_battle_finish(self): return "普通原生结果"
        def prepare(self): self._revival_result_sent = False
        def revival_set_battle_preferences(self, speed, automatic): native_calls.append(("偏好", speed, automatic))
        def clear_master_input_timer(self): self.timer_cleared += 1
        def set_battle_speed(self, value): native_calls.append(("倍速", value))
        def on_auto_state_update(self, master): native_calls.append(("自动状态界面", master))

    class Control:
        def do_command(self, master, name, *args): native_calls.append((master, name, args))

    gworld = types.ModuleType("gworld")
    gworld.get_player = lambda: types.SimpleNamespace(eid="123456789012345678901234",server_proxy=types.SimpleNamespace(do_command=lambda command, args: events.append((command, args))))
    battle_logic = types.ModuleType("battle_logic")
    battle_logic.client_battle = types.SimpleNamespace(shadow_battle=Shadow)
    controllers = types.ModuleType("battle_logic.controlers")
    controllers.server = types.SimpleNamespace(controler=Control)
    game3d = types.ModuleType("game3d")
    game3d.delay_exec = lambda delay, fn: retries.append((delay, fn))
    for module in (gworld, battle_logic, controllers, game3d): sys.modules[module.__name__] = module
    source = (Path(__file__).resolve().parents[2] / "internal/game/human_battle_bridge_script.py").read_text(encoding="utf-8")
    scope = {}
    exec(compile(source, "真人桥", "exec"), scope)
    b = Shadow()
    b.prepare()
    b.revival_set_battle_preferences(1.75, True)
    assert ("偏好", 1, False) in native_calls and b.auto_battle_set == set()
    b.set_battle_speed(1.75)
    assert native_calls[-1] == ("倍速",1)
    b.wait_for_master_input("好友")
    assert b.timer_cleared == 2
    assert b.wait_for_player_input(Info()) == "原生等待"
    assert events[-1][1][0]["kind"] == "human_input" and events[-1][1][0]["data"]["command_index"] == 0
    b.revival_do_shared_command(Info().get_attr("master"), "move_to", ["1", [1, -1, 0]], 1, b.id)
    assert native_calls[-1][2][-1] == (1, -1, 0)
    count = len(native_calls)
    b.revival_do_shared_command(Info().get_attr("master"), "move_to", ["1", [1, -1, 0]], 1, b.id)
    assert len(native_calls) == count
    try: b.revival_do_shared_command(Info().get_attr("master"), "move_to", ["1", [1, -1, 0]], 3, b.id)
    except ValueError: pass
    else: raise AssertionError("序号跳跃被接受")
    b.prepare()
    assert b._revival_human_command_index == 0
    b.revival_set_shared_replay([{"index": 1, "action": 1, "eid": "1", "master": Info().get_attr("master"), "name": "move_to", "args": ["1", [1, -1, 0]]}], b.id)
    b.wait_for_player_input(Info())
    assert b._revival_human_command_index == 1 and not b._revival_human_replay
    b.winner_eid_list = [Info().get_attr("master")]
    b.notify_battle_finish()
    assert events[-1][1][0]["kind"] == "human_result" and events[-1][1][0]["data"]["command_index"] == 1
    count = len(events)
    b.notify_battle_finish()
    assert len(events) == count
    b.prepare()
    b.revival_abort_shared()
    b.notify_battle_finish()
    assert len(events) == count
    b.extra_info = {}
    assert b.notify_battle_finish() == "普通原生结果"
    print("真人共享桥控制验证通过：原生调用、输入序号、同UUID去重、重放、人工输入、分歧中止；Android双端演算仍待MuMu实测。")


if __name__ == "__main__":
    sys.stdout.reconfigure(encoding="utf-8")
    main()
