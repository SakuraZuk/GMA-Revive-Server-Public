# -*- coding: utf-8 -*-
# 复刻服活动运营热更，Python 2.7兼容。配置由服务端同一JSON生成，禁止客户端上传。
# 主证据：Android1.0.128 2A089B57、2B0F3668、DF526314、9930FB45。
# 服务端替换下面表达式占位符为JSON字符串字面量；不是替换单引号内部文本。
def _install_hs_activity_schedules():
    import json
    from common_logic import activity_utils, dungeon_check
    from guis import gui_utils
    import sys
    avatar_module = sys.modules.get('entities.Avatar')
    if avatar_module is None or not hasattr(avatar_module, 'Avatar'):
        raise RuntimeError('entities.Avatar 尚未完成装配')
    avatar_type = avatar_module.Avatar
    # Android385E6D63的check_open_grid引用gworld但模块没有导入。
    # 参考项目同样仅补模块全局，不绕过票券、材料或服务端校验。
    import gworld
    from entities.components import monster_nian_mgr
    monster_nian_mgr.__dict__['gworld'] = gworld
    from guis.prepare.main_line_chapter import activity_item
    import guis.prepare.beginner_guide as beginner_module
    data = activity_utils.data
    # 用户明确批准的旧特别演练恢复，仅12个原有配置，不创造副本或奖励。
    if __HS_SPECIAL_DRILL_REOPEN__:
        reopened = set((2103,2106,2112,2203,2206,2212,2303,2306,2312,2503,2506,2512))
        for item in data.timber_pile_dungeons.itervalues():
            if item.dungeon_id in reopened:
                item.disable_activity = 0
                data.dungeons[item.dungeon_id]._dungeon_disable = False
    time_utils = activity_utils.time_utils
    error_code = activity_utils.error_code
    # 用户于2026-10-08明确指定五个开服限时商品永久开放；原始目录保留在包中。
    # 只改时间展示/检查字段，不改价格、数量、解锁条件、失效条件或每日限购。
    permanent_goods = ((1071011, 15), (1071012, 15), (1071013, 15),
                       (1071014, 15), (2010010, 7))
    for goods_id, expected_days in permanent_goods:
        goods = data.commodity.get(goods_id)
        if goods is None:
            raise ValueError('永久商品配置包含未知编号: %s' % goods_id)
        current_days = goods.open_server_days
        if current_days is not None and current_days != expected_days:
            raise ValueError('永久商品原生期限漂移: %s' % goods_id)
    for goods_id, expected_days in permanent_goods:
        goods = data.commodity[goods_id]
        goods.open_server_days = None
        goods.count_down_flag = 0
    rows = json.loads(__HS_ACTIVITY_SCHEDULES_JSON__)
    schedules = {}
    # 参考项目tools/make_hotfix.py的_archive_time_wrap使用此原生兼容窗口。
    # 这是旧客户端展示/算术所需的非空日期，不是服务端永久政策的截止时间。
    # 服务端仍end_time=0、permanent=true；所有授权由check_activity_open决定。
    archive_end = (2145801600, 86399)
    original = getattr(activity_utils, '_hs_original_activity_times', {})
    for entry in rows:
        activity_id = int(entry['activity_id'])
        info = data.activity_type.get(activity_id)
        if info is None:
            raise ValueError('活动配置包含未知编号: %s' % activity_id)
        if activity_id not in original:
            original[activity_id] = (sum(info.begin_time or ()), sum(info.end_time or ()))
        begin = int(entry['begin_time'])
        end = int(entry['end_time'])
        permanent = bool(entry['permanent'])
        if begin <= 0 or (permanent and end != 0) or (not permanent and end <= begin):
            raise ValueError('活动配置时间无效: %s' % activity_id)
        previous_begin, previous_end = original[activity_id]
        maximum_day = ((previous_end-previous_begin)//86400+1
                       if previous_begin > 0 and previous_end > previous_begin else 0)
        schedules[activity_id] = dict(entry, maximum_day=maximum_day)
        # UI直接读取activity_type的开始/结束时间时同样使用运营日期。
        # 不修改节点_disable、unlock_condition、open_time或activity_open_time。
        info.begin_time = (begin, 0)
        info.end_time = archive_end if permanent else (end, 0)
        info.disable_activity = 0 if entry['enabled'] else 1

    if not hasattr(activity_utils, '_hs_native_get_activity_time'):
        activity_utils._hs_native_get_activity_time = activity_utils.get_activity_time
        activity_utils._hs_native_check_activity_open = activity_utils.check_activity_open
        activity_utils._hs_native_get_activity_day_num = activity_utils.get_activity_day_num
        dungeon_check._hs_native_check_dungeon_type_opened = dungeon_check.check_dungeon_type_opened
        dungeon_check._hs_native_check_banner_type_opened = dungeon_check.check_banner_type_opened
        gui_utils._hs_native_check_shop_time_valid = gui_utils.check_shop_time_valid
    activity_utils._hs_original_activity_times = original
    activity_utils._hs_activity_schedules = schedules

    def get_activity_time(activity_id, hostnum):
        entry = schedules.get(activity_id)
        if entry is None:
            return activity_utils._hs_native_get_activity_time(activity_id, hostnum)
        return ((int(entry['begin_time']), 0),
                archive_end if entry['permanent'] else (int(entry['end_time']), 0))

    def check_activity_open(activity_id, hostnum, ignore_end_time=False):
        entry = schedules.get(activity_id)
        if entry is None:
            return activity_utils._hs_native_check_activity_open(activity_id, hostnum, ignore_end_time)
        now = time_utils.now_time()
        if not entry['enabled'] or now < entry['begin_time']:
            return error_code.RET_LIMIT_ACTIVITY_NO_OPEN
        if not ignore_end_time and not entry['permanent'] and now > entry['end_time']:
            return error_code.RET_ACTIVITY_HAS_ENDED
        return error_code.RET_SUCCESS

    def get_activity_day_num(activity_id, hostnum, ignore_end=False):
        entry = schedules.get(activity_id)
        if entry is None:
            return activity_utils._hs_native_get_activity_day_num(activity_id, hostnum, ignore_end)
        if not ignore_end and check_activity_open(activity_id, hostnum) != error_code.RET_SUCCESS:
            return -1
        # 上海日历日与Go activityOpen一致；常驻只封顶原生活动阶段。
        now = time_utils.now_time()
        day = int((now+28800)//86400-(entry['begin_time']+28800)//86400)+1
        if entry['permanent'] and entry['maximum_day'] > 0:
            day = min(day, entry['maximum_day'])
        return day

    def check_dungeon_type_opened(dungeon_type, hostnum, check_open_time=True,
                                 open_days=None, delta_time=None):
        entry = schedules.get(dungeon_type)
        if entry is None:
            return dungeon_check._hs_native_check_dungeon_type_opened(
                dungeon_type, hostnum, check_open_time, open_days, delta_time)
        info = data.activity_type.get(dungeon_type)
        if info is None or check_activity_open(dungeon_type, hostnum) != error_code.RET_SUCCESS:
            return False
        if info.open_time and not dungeon_check.check_dungeon_open_time(info.open_time):
            return False
        if check_open_time and info.activity_open_time and not dungeon_check.check_activity_open_time(info.activity_open_time):
            return False
        if open_days:
            if get_activity_day_num(dungeon_type, hostnum) not in open_days:
                return False
            threshold = entry['begin_time']+(min(open_days)-1)*86400+(delta_time or 0)
            if time_utils.now_time() < threshold:
                return False
        return True

    def check_banner_type_opened(banner_type):
        activity_id = data.banner_activity_type['banner_dict'].get(banner_type)
        entry = schedules.get(activity_id)
        if entry is None:
            return dungeon_check._hs_native_check_banner_type_opened(banner_type)
        info = data.activity_banner.get(banner_type)
        if info is None or check_activity_open(activity_id, 0) != error_code.RET_SUCCESS:
            return False
        return not info.open_time or dungeon_check.check_dungeon_open_time(info.open_time)

    def get_banner_next_refresh_time():
        # Android2A089B57不检查空结束时间，主界面和场景返回共用此入口。
        # 常驻无结束事件，只保留真实未来开始/有限结束；无事件沿用原生maxint。
        now = time_utils.now_time()
        next_refresh = getattr(sys, 'maxint', 2147483647)
        for banner_id, info in data.activity_banner.items():
            if not info.banner_path:
                continue
            if info.lock_system_name and not dungeon_check.check_system_unlock(info.lock_system_name):
                continue
            if check_banner_type_opened(banner_id):
                activity_id = data.banner_activity_type['banner_dict'].get(banner_id)
                permanent = schedules.get(activity_id, {}).get('permanent', False)
                if not permanent and info.end_time is not None:
                    next_refresh = min(next_refresh, sum(info.end_time)-now)
            elif info.begin_time is not None and sum(info.begin_time) > now:
                next_refresh = min(next_refresh, sum(info.begin_time)-now)
        return abs(next_refresh)

    # 只重排属于已配置活动的历史绝对窗口，保留相对阶段、商品条件和动态到期。
    def relocate_window(info, activity_id):
        entry = schedules.get(activity_id)
        if entry is None:
            return
        old_begin, old_end = original[activity_id]
        windows = getattr(activity_utils, '_hs_original_windows', None)
        if windows is None:
            windows = {}
            activity_utils._hs_original_windows = windows
        key = id(info)
        if key not in windows:
            windows[key] = (getattr(info, 'begin_time', None), getattr(info, 'end_time', None))
        begin, end = windows[key]
        if begin and old_begin:
            offset = max(0, sum(begin)-old_begin)
            info.begin_time = (entry['begin_time']+offset, 0)
        if end and entry['permanent']:
            info.end_time = archive_end
        elif end and old_end:
            info.end_time = (entry['end_time'], 0)

    shop_activities = {}
    def check_shop_time_valid(shop_id, tips_enable=True):
        activity_id = shop_activities.get(shop_id)
        if activity_id is None:
            return gui_utils._hs_native_check_shop_time_valid(shop_id, tips_enable)
        enabled = check_activity_open(activity_id, 0) == error_code.RET_SUCCESS
        if not enabled and tips_enable:
            gui_utils.gworld.get_player().show_error_tips(error_code.RET_SHOP_NO_OPEN)
        return enabled

    for activity_id, entry in schedules.items():
        info = data.activity_type[activity_id]
        for banner_id in info.banner_ids or ():
            banner = data.activity_banner.get(banner_id)
            if banner is not None:
                relocate_window(banner, activity_id)
        board_id = info.activity_board_id
        if board_id:
            board = data.activity_board_info.get(board_id)
            if board is not None:
                relocate_window(board, activity_id)
        for shop_id in info.shop_ids or ():
            shop_activities[shop_id] = activity_id
            shop = data.shop.get(shop_id)
            if shop is not None:
                relocate_window(shop, activity_id)
    for commodity in data.commodity.values():
        activity_id = shop_activities.get(commodity.shop_id)
        if activity_id is not None:
            relocate_window(commodity, activity_id)
    # 部分活动的banner_ids为空，真实关联保存在原生反向banner_dict中。
    # 不能漏掉这些广告窗口，亦不能重排未配置活动的广告。
    for banner_id, activity_id in data.banner_activity_type['banner_dict'].items():
        banner = data.activity_banner.get(banner_id)
        if banner is not None and activity_id in schedules:
            relocate_window(banner, activity_id)
    activity_utils.get_activity_time = get_activity_time
    activity_utils.check_activity_open = check_activity_open
    activity_utils.get_activity_day_num = get_activity_day_num
    # Android4D347DB1直接sum(end_time)；兼容窗口不能改变永久开放规则。
    # 保留中世纪地图原生最近活动选择，不强制解锁或跳到最后阶段。
    # utils.assemble会复制组件方法到Avatar，不能只替换组件类。
    if not hasattr(avatar_type, '_hs_native_get_mid_age_activity_id'):
        avatar_type._hs_native_get_mid_age_activity_id = avatar_type.get_mid_age_activity_id

    def get_mid_age_activity_id(self):
        from common_const import ACTIVITY_TYPE_MID_AGE, ACTIVITY_TYPE_MID_AGE_TWO
        import gworld
        ids = (ACTIVITY_TYPE_MID_AGE, ACTIVITY_TYPE_MID_AGE_TWO)
        if not any(schedules.get(key, {}).get('permanent', False) for key in ids):
            return avatar_type._hs_native_get_mid_age_activity_id(self)
        now = time_utils.now_time()
        closest, selected = None, None
        for activity_id in ids:
            begin, end = get_activity_time(activity_id, gworld.get_player().hostnum)
            boundaries = [sum(value) for value in (begin, end) if value is not None]
            if not boundaries:
                continue
            distance = min(abs(value-now) for value in boundaries)
            if closest is None or distance < closest:
                closest, selected = distance, activity_id
        return selected

    avatar_type.get_mid_age_activity_id = get_mid_age_activity_id
    if not hasattr(activity_item, '_hs_native_set_time_panel'):
        activity_item._hs_native_set_time_panel = activity_item.set_time_panel
    beginner_type = beginner_module.beginner_guide
    if not hasattr(beginner_type, '_hs_native_update_activity_count_down'):
        beginner_type._hs_native_update_activity_count_down = beginner_type.update_activity_count_down

    def set_time_panel(self):
        activity_id = data.chapter[self.chapter_id].chapter_type
        if schedules.get(activity_id, {}).get('permanent', False):
            self.text_limit_time.set_string(u'常驻开放')
            return None
        return activity_item._hs_native_set_time_panel(self)

    def update_activity_count_down(self):
        activity_id = beginner_module.activity_mission_id
        if not schedules.get(activity_id, {}).get('permanent', False):
            return beginner_type._hs_native_update_activity_count_down(self)
        self.title_time.set_string(u'常驻开放')
        if self.count_down_handle:
            self.cancel_callback(self.count_down_handle)
            self.count_down_handle = None
        return True

    activity_item.set_time_panel = set_time_panel
    beginner_type.update_activity_count_down = update_activity_count_down
    dungeon_check.check_dungeon_type_opened = check_dungeon_type_opened
    dungeon_check.check_banner_type_opened = check_banner_type_opened
    dungeon_check.get_banner_next_refresh_time = get_banner_next_refresh_time
    gui_utils.check_shop_time_valid = check_shop_time_valid
    print('HS_ACTIVITY_SCHEDULES_READY %s' % len(schedules))

def _start_hs_activity_schedules(install):
    def attempt():
        try:
            install()
        except Exception as error:
            print('HS_ACTIVITY_SCHEDULES_WAIT %s' % error)
            import game3d
            game3d.delay_exec(1500, attempt)
    attempt()

_start_hs_activity_schedules(_install_hs_activity_schedules)
