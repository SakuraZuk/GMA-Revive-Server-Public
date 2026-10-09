# -*- coding: utf-8 -*-
"""验证活动统计借用原桥单次结果、重播幂等和真人包装闭包，不启动客户端。"""
import pathlib
import sys
import types
sys.stdout.reconfigure(encoding='utf-8')

source = pathlib.Path('internal/game/activity_metrics_script.py').read_text(encoding='utf-8')

def fixture(human_first, revision):
    calls = []
    def report(battle, kind, data, **kwargs):
        calls.append((kind, data, kwargs))
        return True
    def native_notify(self):
        if self.sent:
            return
        self.sent = report(self, 'result', {'winner_eids': ['本玩家']}, include_state=True)
    class Battle:
        _revival_bridge_version = revision
        notify_battle_finish = native_notify
        bid = 10001
        sent = False
        def get_battle_ap_statistics(self): return 7.9
        def get_total_extra_statistics(self):
            if self.bid is None:
                raise KeyError(None)
            return 135792
        def is_league_protect(self): return False
        def battle_end_notice(self, *args): self.bid = None
    client_battle = types.ModuleType('battle_logic.client_battle')
    client_battle.shadow_battle = Battle
    package = types.ModuleType('battle_logic')
    package.client_battle = client_battle
    sys.modules['battle_logic'] = package
    sys.modules['battle_logic.client_battle'] = client_battle
    if human_first:
        old_notify = native_notify
        def human_notify(self):
            if self.human: return '真人专用结果'
            return old_notify(self)
        Battle.notify_battle_finish = human_notify
        Battle._revival_human_originals = {'notify_battle_finish': native_notify}
    namespace = {}
    exec(compile(source, '<活动统计>', 'exec'), namespace)
    namespace['_install_hs_activity_metrics']()
    battle = Battle()
    battle.human = False
    battle.battle_end_notice(['本玩家'], 1)
    assert battle.bid is None, '夹具必须模拟原生结算前清空bid'
    battle.notify_battle_finish()
    battle.notify_battle_finish()
    assert len(calls) == 1, '统计扩展重复发出战斗结果'
    assert calls[0][1]['total_ap_statistics'] == 7
    assert calls[0][1]['total_damaged_statistics'] == 135792
    assert calls[0][2]['include_state'] is True
    if human_first:
        shared = Battle()
        shared.human = True
        assert shared.notify_battle_finish() == '真人专用结果'
        assert len(calls) == 1

for revision in (12, 13, 14, 15):
    fixture(False, revision)
    fixture(True, revision)
print('活动原生统计扩展：原桥、真人闭包、重复安装、单次结果全部通过')
