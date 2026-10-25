# -*- coding: utf-8 -*-
"""保留原生卡池时间候选，将无终点的逐日搜索改为次日重新查询。"""


def _install_hs_summon_timer():
    import sys
    import game3d
    import gworld
    import data
    from utils import time_utils
    from guis import gui_utils
    module = sys.modules.get('guis.summon_card.summon_new_card')
    if module is None:
        return False
    cls = module.summon_new_card
    if getattr(cls.start_refresh_timer, '_hs_summon_timer_revision', 0) >= 2:
        return True

    def start_refresh_timer(self):
        if self.refresh_callback:
            self.refresh_callback.cancel()
            self.refresh_callback = None
        now_time = time_utils.now_time()
        target_time = None

        def candidate(value):
            if value > now_time:
                return value
            return None

        for pool_id in self.get_valid_card_pool():
            pool_data = data.cards_random[pool_id]
            player = gworld.get_player()
            begin_time = player.get_card_pool_time(pool_id)
            times = []
            if begin_time > 0 and pool_data.invalid_time:
                times.append(candidate(begin_time + pool_data.invalid_time * 86400))
            if pool_data.open_server_days:
                times.append(candidate(time_utils.next_interval_day_refresh_time(
                    player.server_open_time, None, pool_data.open_server_days)))
            open_time, close_time = gui_utils.get_pool_open_close_time(pool_id)
            if not open_time and not close_time:
                # 原生在检查开启窗口前已计算失效/开服候选，不能丢弃。
                for value in times:
                    if value is not None and (target_time is None or value < target_time):
                        target_time = value
                continue
            open_timestamp = sum(open_time) if open_time else None
            if open_timestamp and open_timestamp > now_time:
                times.append(open_timestamp)
            else:
                if close_time:
                    times.append(candidate(sum(close_time)))
                # 原生索引可能恒为-1或只有一个轮转项，不能循环到2038年。
                # 次日询问原生ask_refresh即可重新读取实际卡池，不改变抽卡权威。
                if pool_data.cycle_flag:
                    times.append(candidate(time_utils.next_daily_timestamp(
                        now_time, time_utils.REFRESH_HMS)))
            for value in times:
                if value is not None and (target_time is None or value < target_time):
                    target_time = value
        if target_time is not None:
            self.refresh_callback = self.add_callback(target_time - now_time, self.ask_refresh)

    start_refresh_timer._hs_summon_timer_revision = 2
    cls.start_refresh_timer = start_refresh_timer
    cls._hs_summon_timer_revision = 2
    print('HS_SUMMON_TIMER_READY 2')
    return True


def _start_hs_summon_timer():
    import game3d
    # UI按需加载或模块重建后可能换回原生类；成功一次不能停止安装检查。
    # 同版本热更重复执行仅复用一条检查链，避免每次入场新建无限计时器。
    if getattr(game3d, '_hs_summon_watch_revision', 0) >= 2:
        _install_hs_summon_timer()
        return
    game3d._hs_summon_watch_revision = 2
    def attempt():
        ready = _install_hs_summon_timer()
        game3d.delay_exec(5000 if ready else 1000, attempt)
    attempt()


_start_hs_summon_timer()
