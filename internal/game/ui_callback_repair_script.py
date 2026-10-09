# -*- coding: utf-8 -*-
"""保留已提交的真实奖励；界面销毁或场景切换不能使迟到回调中断。"""

def _install_hs_ui_callback_repair():
    import sys
    import game3d
    import gui
    import cache
    import gworld
    import logging
    def flow_event(name, values):
        # 只记录界面步骤与等待状态，诊断失败不得打断原生流程。
        try:
            player = gworld.get_player()
            if player is not None:
                player.server_proxy.client_sa_log('hs_ui_flow', dict(values, step=name))
        except Exception:
            pass
    guide_module = sys.modules.get('guis.prepare.beginner_guide')
    summon_module = sys.modules.get('guis.summon_card.summon_new_card')
    avatar_module = sys.modules.get('entities.Avatar')
    guide = guide_module.beginner_guide if guide_module is not None else None
    summon = summon_module.summon_new_card if summon_module is not None else None
    avatar = avatar_module.Avatar if avatar_module is not None else None
    map_module = sys.modules.get('guis.prepare.free_stage_map')
    map_type = getattr(map_module, 'free_stage_map', None)
    if map_type is not None and getattr(map_type, '_hs_flow_revision', 0) < 1:
        original_map_show = map_type.init_show
        def map_show(self, *args, **kwargs):
            flow_event('explore_open', {'stage': args[0] if args else None})
            result = original_map_show(self, *args, **kwargs)
            flow_event('explore_ready', {'stage': getattr(self, 'free_stage_id', None)})
            return result
        map_type.init_show = map_show
        map_type._hs_flow_revision = 1
    if avatar is not None and getattr(avatar, '_hs_guide_flow_revision', 0) < 2 and hasattr(avatar, 'trigger_guide'):
        original_trigger_guide = getattr(avatar, '_hs_guide_flow_original', avatar.trigger_guide)
        if getattr(avatar, '_hs_guide_flow_revision', 0) == 1 and not hasattr(avatar, '_hs_guide_flow_original'):
            # 0913包装未保存原方法属性；按该包装的确切闭包名取回原方法，
            # 热更新已有进程时不能把旧噪声包装再套在新包装里面。
            old_function = getattr(original_trigger_guide, 'im_func', original_trigger_guide)
            for name, cell in zip(old_function.func_code.co_freevars, old_function.func_closure or ()):
                if name == 'original_trigger_guide':
                    original_trigger_guide = cell.cell_contents
                    break
        def trigger_guide(self, guide_id, **extra_info):
            result = original_trigger_guide(self, guide_id, **extra_info)
            context = getattr(self, 'guide_context', None)
            if result:
                flow_event('guide_started', {'guide': guide_id, 'accepted': True,
                                            'waiting': bool(getattr(self, 'in_guide', False)),
                                            'story_waiting': bool(getattr(self, 'in_guide_storyline', False))})
                def inspect_wait():
                    if getattr(self, 'guide_context', None) is context and (getattr(self, 'in_guide', False) or getattr(self, 'in_guide_storyline', False)):
                        flow_event('guide_wait', {'guide': guide_id, 'seconds': 15})
                game3d.delay_exec(15000, inspect_wait)
            return result
        avatar.trigger_guide = trigger_guide
        avatar._hs_guide_flow_original = original_trigger_guide
        avatar._hs_guide_flow_revision = 2
    # 原包inner_item.get_info直接取info[0]。领奖后属性推送重建任务列表，
    # 原回调仍持有旧行；只保护任务行，原update已有None分支负责隐藏旧行。
    if guide_module is not None:
        for item_name in ('task_item', 'activity_task_item'):
            item_type = getattr(guide_module, item_name, None)
            if item_type is None or getattr(item_type, '_hs_task_row_revision', 0) >= 1:
                continue
            def protect_row(original):
                def get_info(self):
                    if not getattr(self, 'info', None):
                        return None
                    return original(self)
                return get_info
            item_type.get_info = protect_row(item_type.get_info)
            item_type._hs_task_row_revision = 1
    # 各界面按需导入，签到不能等待玩家先打开抽卡界面才安装保护。
    if avatar is not None and getattr(avatar, '_hs_explore_callback_revision', 0) < 1:
        def set_explore_auto_agent(self, dungeon_type, enabled, callback=None):
            def complete(success):
                if callback is not None:
                    callback(success)
                active = gui.get_ui('miku_map')
                if active is not None:
                    active.update_auto_state()
            return self.call_server('set_explore_auto_agent', complete, dungeon_type, enabled)
        avatar.set_explore_auto_agent = set_explore_auto_agent
        avatar._hs_explore_callback_revision = 1
    if guide is not None and getattr(guide, '_hs_safe_checkin_revision', 0) < 2:
        def get_item(self, bonus_dict):
            if not bonus_dict:
                return None
            items = []
            self.reward_days = []
            for day, values in bonus_dict.iteritems():
                cache.set('checkin_bonus.day_%d' % day, values)
                items.extend(values)
                self.reward_days.append(day)
            live = gui.get_ui('beginner_guide') is self
            panel = getattr(self, 'checkin_reward_list', None)
            if live and panel is not None:
                panel.update()
            mask = getattr(self, 'checkin_mask', None)
            self.wait_bonus = True
            if live and mask is not None:
                mask.show()
            def after_show():
                active = gui.get_ui('beginner_guide')
                if active is not None and getattr(active, 'checkin_reward_list', None) is not None:
                    active.update_checkin_list()
            def show_tips():
                self.reward_days = []
                try:
                    gui.general_tips_mgr.show_tips('item_get', ('reward', items), after_show)
                finally:
                    self.wait_bonus = False
                    if gui.get_ui('beginner_guide') is self:
                        active_mask = getattr(self, 'checkin_mask', None)
                        if active_mask is not None:
                            active_mask.hide()
            # 原生1.5秒展示延迟；独立调度避免关闭原UI时取消真实领奖展示。
            game3d.delay_exec(1500, show_tips)
        guide.get_item = get_item
        guide._hs_safe_checkin_revision = 2
    if summon is not None and getattr(summon, '_hs_safe_result_revision', 0) < 1:
        original = summon.on_get_card_result
        def on_get_card_result(self, cards, first_card_ids, eureka, extra_bonus_list):
            scene = gworld.get_current_scene()
            if scene is not None and hasattr(scene, 'form_summon_paper_entity_list'):
                return original(self, cards, first_card_ids, eureka, extra_bonus_list)
            # 原生init_show第一步就是激活已预载召唤场景；不重发抽卡请求。
            pending = (cards, first_card_ids, eureka, extra_bonus_list)
            self._hs_pending_summon_result = pending
            if getattr(gworld, 'get_battle', lambda: None)() is None:
                gworld.scene_mgr.activate_preload_scene()
            state = {'attempts': 0}
            def ready():
                if getattr(self, '_hs_pending_summon_result', None) is not pending:
                    return
                current = gworld.get_current_scene()
                if current is not None and hasattr(current, 'form_summon_paper_entity_list'):
                    self._hs_pending_summon_result = None
                    return original(self, *pending)
                state['attempts'] += 1
                if state['attempts'] >= 100:
                    # 保留结果供诊断，释放输入等待；资产已经由服务器持久，不再抽一次。
                    self.on_press_summon_cancel(len(cards))
                    logging.error('HS_SUMMON_SCENE_NOT_READY result_retained=1')
                    return
                game3d.delay_exec(100, ready)
            game3d.delay_exec(100, ready)
        summon.on_get_card_result = on_get_card_result
        summon._hs_safe_result_revision = 1
    return guide is not None and summon is not None and avatar is not None and map_type is not None

def _start_hs_ui_callback_repair():
    import game3d
    def attempt():
        try:
            ready = _install_hs_ui_callback_repair()
        except Exception as error:
            print('HS_UI_CALLBACK_WAIT %s' % error)
            ready = False
        if not ready:
            game3d.delay_exec(1000, attempt)
    attempt()

_start_hs_ui_callback_repair()
