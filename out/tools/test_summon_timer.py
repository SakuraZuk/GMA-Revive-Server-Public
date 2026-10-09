"""验证真实故障中的恒定轮转索引不会导致无限日期搜索，并保留到期候选。"""
import sys
import types
import ast
import json
from pathlib import Path

now = 1791482400
scheduled = []
utils = types.ModuleType('utils')
utils.time_utils = types.SimpleNamespace(now_time=lambda: now, REFRESH_HMS=(5, 0, 0),
    next_daily_timestamp=lambda *args: now + 86400,
    next_interval_day_refresh_time=lambda *args: now + 172800)
windows = {1: ([now - 100], [now + 300]), 2: ([now + 100], [now + 500])}
guis = types.ModuleType('guis')
guis.gui_utils = types.SimpleNamespace(get_pool_open_close_time=lambda pool: windows[pool])
sys.modules['guis'] = guis
sys.modules['utils'] = utils
sys.modules['game3d'] = types.SimpleNamespace(delay_exec=lambda *args: None)
player = types.SimpleNamespace(get_card_pool_time=lambda pool: 0, server_open_time=now - 100000,
    compute_card_pool_index=lambda *args: (_ for _ in ()).throw(AssertionError('不能搜索恒定索引')))
sys.modules['gworld'] = types.SimpleNamespace(get_player=lambda: player)
sys.modules['data'] = types.SimpleNamespace(cards_random={
    1: types.SimpleNamespace(invalid_time=0, open_server_days=0, cycle_flag=1),
    2: types.SimpleNamespace(invalid_time=0, open_server_days=0, cycle_flag=1)})
class UI:
    refresh_callback = None
    pools = [1]
    def get_valid_card_pool(self): return self.pools
    def add_callback(self, seconds, callback):
        scheduled.append(seconds)
        return types.SimpleNamespace(cancel=lambda: scheduled.append('取消'))
    def ask_refresh(self): pass
sys.modules['guis.summon_card.summon_new_card'] = types.SimpleNamespace(summon_new_card=UI)
code = Path(sys.argv[1]).read_text(encoding='utf-8')
# 真实本版模块路径，不能让夹具在错误的utils包下造出gui_utils。
assert not hasattr(utils, 'gui_utils')
inventory_path = Path(__file__).resolve().parents[2] / 'out/npk_scripts/android_inventory.json'
inventory = json.loads(inventory_path.read_text(encoding='utf-8'))
def filenames(value):
    if isinstance(value, dict):
        if isinstance(value.get('filename'), str):
            yield value['filename'].replace('\\', '/')
        for child in value.values():
            for item in filenames(child): yield item
    elif isinstance(value, list):
        for child in value:
            for item in filenames(child): yield item
native_paths = set(filenames(inventory))
for node in ast.walk(ast.parse(code)):
    if isinstance(node, ast.ImportFrom):
        for name in node.names:
            target = (node.module + '.' + name.name).replace('.', '/') + '.py'
            assert target in native_paths, '原生不存在导入路径: ' + target
wrapped = 'def install():\n' + '\n'.join('    ' + line for line in code.splitlines()) + '\ninstall()'
exec(compile(wrapped, '热更闭包', 'exec'), {}, {})
ui = UI()
ui.start_refresh_timer()
assert scheduled == [300], scheduled
windows[1] = ([now - 100], None)
ui.start_refresh_timer()
assert scheduled[-2:] == ['取消', 86400], scheduled
ui.pools = [1, 2]
ui.start_refresh_timer()
assert scheduled[-1] == 100, scheduled
player.get_card_pool_time = lambda pool: now - 86350
sys.modules['data'].cards_random[1].invalid_time = 1
ui.start_refresh_timer()
assert scheduled[-1] == 50, scheduled
windows[1] = (None, None)
ui.pools = [1]
ui.start_refresh_timer()
assert scheduled[-1] == 50, scheduled
original = UI.start_refresh_timer
exec(compile(wrapped, '重复热更', 'exec'), {}, {})
assert UI.start_refresh_timer is original
print('卡池恒定索引、关闭时间、未来开启、失效时间、取消及重复安装验证通过')
