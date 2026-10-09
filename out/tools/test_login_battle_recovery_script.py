# -*- coding: utf-8 -*-
"""离线验证登录恢复、原生收尾与场景互斥；不代替Android验收。"""
import sys
import types
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "internal" / "game" / "login_battle_recovery_script.py"


def install_case(proxy_factory):
    delayed = []
    state = {"battle": None, "refresh": 0, "start": 0, "preload": 0, "load": 0, "leave": 0}
    game3d = types.ModuleType("game3d")
    game3d.delay_exec = lambda delay, callback: delayed.append((delay, callback))
    gworld = types.ModuleType("gworld")
    gworld.get_battle = lambda: state["battle"]
    avatar_module = types.ModuleType("entities.Avatar")
    data = types.ModuleType('data')
    data.guide_task = {1013: types.SimpleNamespace(do_type=2, do_type_params=[501]), 1014: types.SimpleNamespace(do_type=1, do_type_params=[])}
    sys.modules['data'] = data

    class Avatar:
        def __init__(self):
            self.server_proxy = proxy_factory()
            self.guide_tasks = {}
            self.dungeon_mgr = {}
            self.completed = []
            self.guide_runs = 0

        def do_guide_task(self, callback=None, pre_callback=None):
            self.guide_runs += 1

        def guide_task_finished(self, task_id, callback=None):
            self.completed.append(task_id)
            self.guide_tasks.pop(task_id)
            if callback: callback()

        def on_refresh_login(self, *args, **kwargs):
            state["refresh"] += 1
            self.preload_by_refresh_login()

        def preload_by_refresh_login(self):
            state["preload"] += 1

        def preload_scene_finished(self, in_battle):
            state["leave"] += 1
            self.load_scene_finished(in_battle)

        def load_scene_finished(self, in_battle):
            state["load"] += 1

        def start_server_battle_ok(self, *args, **kwargs):
            state["start"] += 1

        def raw_clear_battle(self):
            state["cleared"] = state.get("cleared", 0) + 1
            self.battle = None

    avatar_module.Avatar = Avatar
    sys.modules["game3d"] = game3d
    sys.modules["gworld"] = gworld
    sys.modules["entities.Avatar"] = avatar_module
    namespace = {"__name__": "__hotfix_test__"}
    exec(compile(SCRIPT.read_text(encoding="utf-8"), str(SCRIPT), "exec"), namespace)
    return Avatar, delayed, state


class CountingProxy:
    def __init__(self):
        self.calls = 0

    def client_need_recover_battle(self):
        self.calls += 1


class FailingProxy:
    def client_need_recover_battle(self):
        raise RuntimeError("无恢复会话")


def main():
    sys.stdout.reconfigure(encoding='utf-8')
    avatar_type, delayed, state = install_case(CountingProxy)
    avatar = avatar_type()
    avatar.start_server_battle_ok(1, 10001, "uuid", {})
    state["battle"] = object()
    avatar.on_refresh_login({})
    assert state["start"] == 1
    assert state["refresh"] == 1
    assert avatar.server_proxy.calls == 0
    assert not delayed
    assert state["leave"] == 1 and state["preload"] == 0 and state["load"] == 0

    avatar_type, delayed, state = install_case(CountingProxy)
    avatar = avatar_type()
    avatar.on_refresh_login({})
    assert avatar.server_proxy.calls == 0
    assert not delayed
    assert state["refresh"] == 1
    assert state["preload"] == 1 and state["leave"] == 0

    avatar_type, delayed, state = install_case(CountingProxy)
    avatar = avatar_type()
    state["battle"] = object()
    avatar.on_refresh_login({})
    assert avatar.server_proxy.calls == 0
    assert state["refresh"] == 1
    assert state["leave"] == 1
    assert state["preload"] == 0
    assert state["load"] == 0

    avatar_type, delayed, state = install_case(FailingProxy)
    avatar = avatar_type()
    avatar.on_refresh_login({})
    assert state["refresh"] == 1
    assert not delayed
    assert state["preload"] == 1

    avatar_type, delayed, state = install_case(CountingProxy)
    avatar = avatar_type()
    state["battle"] = object()
    avatar.on_refresh_login({})
    assert avatar.server_proxy.calls == 0 and not delayed
    assert state["refresh"] == 1 and state["leave"] == 1
    assert state["preload"] == 0 and state["load"] == 0
    # 后续正常非战斗场景完成仍必须交回原生流程，不吞掉引导推进。
    state["battle"] = None
    avatar.load_scene_finished(False)
    assert state["load"] == 1

    # 登录退场期间战斗已失效时，不能误吞原生完成回调。
    avatar._hs_login_recovery_scene = object()
    avatar.load_scene_finished(True)
    assert state["load"] == 2
    avatar.guide_tasks[1013] = types.SimpleNamespace(is_doing=lambda: True)
    avatar.dungeon_mgr[501] = types.SimpleNamespace(finished=0)
    avatar.do_guide_task()
    assert avatar.guide_runs == 1 and not avatar.completed
    avatar.dungeon_mgr[501].finished = 1
    called = []
    avatar.do_guide_task(callback=lambda: called.append(True))
    assert avatar.completed == [1013] and called == [True]
    avatar.do_guide_task()
    assert avatar.guide_runs == 2 and avatar.completed == [1013]
    # 服务器明确恢复授权可释放多轮失败重连后残留的更早对象。
    avatar.battle = types.SimpleNamespace(id='旧战斗')
    avatar.start_server_battle_ok(1, 10001, '新战斗', {'hs_recover_previous_uuid': '旧战斗'})
    assert avatar.battle is None and state['cleared'] == 1
    avatar.battle = types.SimpleNamespace(id='更早旧战斗')
    avatar.start_server_battle_ok(1, 10001, '新战斗', {'hs_recover_previous_uuid': '旧战斗'})
    assert avatar.battle is None and state['cleared'] == 2
    avatar.battle = types.SimpleNamespace(id='新战斗')
    start_count = state['start']
    avatar.start_server_battle_ok(1, 10001, '新战斗', {'hs_recover_previous_uuid': '旧战斗'})
    assert avatar.battle.id == '新战斗' and state['cleared'] == 2 and state['start'] == start_count
    avatar.start_server_battle_ok(1, 10001, '新战斗', {})
    assert state['cleared'] == 2 and avatar.battle.id == '新战斗'
    # 原生网络调用在装饰器转换前传_0至_3，不能只覆盖位置参数。
    avatar.battle = types.SimpleNamespace(id='更早残留')
    avatar.start_server_battle_ok(_0=1, _1=10001, _2='关键字新战斗', _3={'hs_recover_previous_uuid': '上一轮'})
    assert avatar.battle is None and state['cleared'] == 3
    avatar.battle = types.SimpleNamespace(id='关键字新战斗')
    start_count = state['start']
    avatar.start_server_battle_ok(_0=1, _1=10001, _2='关键字新战斗', _3={'hs_recover_previous_uuid': '上一轮'})
    assert state['cleared'] == 3 and state['start'] == start_count
    avatar.battle = types.SimpleNamespace(id='网络旧对象')
    packet = {'_0': 1, '_1': 505, '_2': '网络新对象', '_3': {'hs_recover_previous_uuid': '网络上一轮'}}
    avatar.start_server_battle_ok(packet)
    assert state['cleared'] == 4 and avatar.battle is None
    avatar.battle = types.SimpleNamespace(id='网络新对象')
    start_count = state['start']
    avatar.start_server_battle_ok(packet)
    assert state['cleared'] == 4 and state['start'] == start_count
    print("登录战斗恢复热更离线验证通过")


if __name__ == "__main__":
    main()
