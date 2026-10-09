# -*- coding: utf-8 -*-
"""保留已提交的真实奖励；界面销毁或场景切换不能使迟到回调中断。"""

def _install_hs_ui_callback_repair():
    import sys
    import game3d
    import gui
    import cache
    import gworld
    import logging
    guide_module = sys.modules.get('guis.prepare.beginner_guide')
    summon_module = sys.modules.get('guis.summon_card.summon_new_card')
    avatar_module = sys.modules.get('entities.Avatar')
    if guide_module is None or summon_module is None or avatar_module is None:
        return False
    guide = guide_module.beginner_guide
    summon = summon_module.summon_new_card
    avatar = avatar_module.Avatar
    if getattr(avatar, '_hs_explore_callback_revision', 0) < 1:
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
    if getattr(guide, '_hs_safe_checkin_revision', 0) < 1:
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
        guide._hs_safe_checkin_revision = 1
    if getattr(summon, '_hs_safe_result_revision', 0) < 1:
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
    return True

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
