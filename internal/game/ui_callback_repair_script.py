# -*- coding: utf-8 -*-
"""保留已提交的真实奖励；界面销毁或场景切换不能使迟到回调中断。"""

def _hs_wrap_battle_input_owner(original):
    def on_allow_player_input(self, eid, allow):
        import gworld
        entity = self.entities.get(eid)
        # 只给当前战斗自身登记的实体补所属对象；不把旧战斗实体接入新战斗。
        if (entity is not None and gworld.get_battle() is self
                and getattr(entity, 'battle', None) is None
                and eid in self.entity_infos):
            entity.battle = self
        return original(self, eid, allow)
    return on_allow_player_input


def _hs_checkin_bonus_list(values):
    # 原生RPC只转换顶层自定义类型；全领按日字典中的box仍是普通字典。
    # 用原生load/to_list保留材料、契印和内嵌奖励语义，不重新生成随机奖励。
    if isinstance(values, dict) and values.get('__custom_type') == 'box.box':
        from custom_types.box import box
        values = box.load(values)
    if hasattr(values, 'to_list'):
        values = values.to_list()
    return values

def _hs_wrap_consign_common_lookup(original):
    def lookup(self, card_key):
        import gworld
        player = gworld.get_player()
        # 详情页以UUID查card_mgr，主页面原版却以同一参数查模板编号字典。
        # 保留旧模板编号调用；新回包先映射到真实卡牌的card_id，再沿用原生语音。
        try:
            player.card_common_mgr[card_key]
        except (KeyError, TypeError, ValueError):
            try:
                card_key = player.card_mgr[card_key].card_id
                player.card_common_mgr[card_key]
            except (KeyError, TypeError, ValueError):
                # 迟到演出中的幻书已移除时不阻断已提交奖励。
                return None, None
        return original(self, card_key)
    return lookup

def _hs_consign_finish_param(param):
    import gworld
    task_id, card_keys, bonus = param
    player = gworld.get_player()
    card_ids = []
    for key in card_keys:
        if isinstance(key, (int, long)):
            card_ids.append(key)
        else:
            try:
                card_ids.append(player.card_mgr[key].card_id)
            except KeyError:
                continue
    return task_id, card_ids, bonus

def _hs_wrap_consign_tips(original):
    def tips(self, tips_type, param, *args, **kwargs):
        # 完成窗可能在本次show_tips内首次加载，须在原生提示队列入队前转换。
        if tips_type == 'consign_finish':
            param = _hs_consign_finish_param(param)
            if not param[1]:
                tips_type, param = 'item_get', ('reward', param[2].to_list())
        return original(self, tips_type, param, *args, **kwargs)
    return tips

def _hs_wrap_consign_finish(original):
    def finish(self, param):
        import gui
        task_id, card_ids, bonus = _hs_consign_finish_param(param)
        # 原生完成窗直接取card_ids[0]。没有可展示幻书时仍显示已提交奖励。
        if not card_ids:
            return gui.general_tips_mgr.show_tips('item_get', ('reward', bonus.to_list()))
        return original(self, (task_id, card_ids, bonus))
    return finish

def _hs_wrap_live_task_update(original):
    def update(self, *args, **kwargs):
        # 原生widget销毁后_ins为None。仅跳过旧任务行，领奖回调仍继续刷新当前列表。
        if hasattr(self, '_ins') and self._ins is None:
            return None
        return original(self, *args, **kwargs)
    return update

def _hs_receive_power_supply(self, buff_id, callback=None):
    import data
    import gui
    from common_logic import bonus_buff_utils
    # 保留原生补给资格检查及RPC参数。原版callback形参并未调用，仍保持该契约。
    if buff_id not in data.bonus_buff:
        return None
    buffs = bonus_buff_utils.get_empty_buffs()
    buffs.add_buff(buff_id, self)
    if not buffs.get_bonus_ids():
        return None
    def complete(bonus_box, msg):
        if not bonus_box:
            return None
        activity = gui.get_ui('limit_activity')
        if activity is not None and callable(getattr(activity, 'update_list', None)):
            activity.update_list()
        main = gui.get_ui('main_interface')
        buttons = getattr(main, 'hall_buttons', None)
        # 尚未加载的原生UI返回swallow占位对象，不能按字典索引。
        if isinstance(buttons, dict):
            button = buttons.get('xianshi')
            if button is not None:
                button.update_red_point()
        gui.general_tips_mgr.show_tips('item_get', ('reward', bonus_box.to_list()))
    return self.call_server('receive_power_supply', complete, buff_id)

def _hs_gift_window(callback):
    import gui
    function = getattr(callback, 'im_func', callback)
    code = getattr(function, 'func_code', None)
    # 只恢复原版特殊送礼本次选择的界面；普通赠礼与其他界面不受影响。
    if code is None or not code.co_filename.replace('\\', '/').endswith('guis/prepare/girl_special_gift.py'):
        return
    for name, cell in zip(code.co_freevars, function.func_closure or ()):
        if name == 'self':
            window = cell.cell_contents
            if gui.get_ui('girl_special_gift') is window:
                return window
            return None


def _hs_gift_failed(window, generation):
    import gui
    if (window is not None and gui.get_ui('girl_special_gift') is window
            and getattr(window, '_hs_gift_request_generation', 0) == generation
            and getattr(window, 'in_anim', False)):
        # 原生after_story只返回赠礼页并释放动画锁，不发奖也不播放成功对白。
        window.after_story()


def _hs_consume_intimacy_gift(self, card_id, material_id, material_count=1, choice_type=0, callback=None):
    import data
    import gui
    window = _hs_gift_window(callback)
    generation = 0
    if window is not None:
        generation = getattr(window, '_hs_gift_request_generation', 0) + 1
        window._hs_gift_request_generation = generation
    # 按本版material_mgr原生流程保留资格、特殊次数降级及RPC参数。
    ret = self.check_consume_intimacy_gift(card_id, material_id, material_count)
    if not self.check_ret_success(ret):
        _hs_gift_failed(window, generation)
        self.show_error_tips(ret)
        return None
    def complete(ret, bonus_box):
        if not self.check_ret_success(ret):
            _hs_gift_failed(window, generation)
            self.show_error_tips(ret)
            return None
        if bonus_box:
            # 升级演出或切页可能先销毁普通赠礼页；奖励已经由服务端提交，
            # 不能因旧页面为None而中断特殊剧情回调并永久留下in_anim锁。
            gift_ui = gui.get_ui('girl_gift')
            setter = getattr(gift_ui, 'set_back_bonus', None)
            if callable(setter):
                setter(bonus_box)
        if callback:
            callback()
        if gui.is_visible('restroom_interaction'):
            gui.restroom_interaction.on_consume_gift(card_id, material_id, material_count)
    config = data.special_gift_rule[1]
    material = self.material_mgr.get(material_id)
    gift = data.gift[material.target_id]
    card = self.card_common_mgr.get(card_id)
    if (self.special_gift_card_2_count.get(card_id, 0) >= config.single_card_times
            or sum(self.special_gift_card_2_count.values()) >= config.total_times
            or gift.forbid_special_gift > 0 or card.intimacy >= card.get_max_intimacy()):
        choice_type = 0
    self.call_server('consume_intimacy_gift', complete, card_id, material_id, material_count, choice_type)


def _hs_wrap_recommend_request(original):
    def request(self, force, callback=None):
        function = getattr(callback, 'im_func', callback)
        code = getattr(function, 'func_code', None)
        owner = getattr(callback, 'im_self', None)
        if (owner is not None and code is not None and code.co_name == 'on_get_search_friend'
                and code.co_filename.replace('\\', '/').endswith('guis/social/friend_recommend.py')):
            import sys
            module = sys.modules.get('guis.social.friend_recommend')
            tab = getattr(module, 'recommend_tab', None)
            rows = getattr(owner, 'friend_items', None)
            native_callback = callback
            def complete(friends):
                # 请求时已捕获的旧绑定方法不会随类补丁更新；保护本次列表生命周期。
                current = getattr(owner, 'friend_items', None)
                if current is not rows or not isinstance(current, list) or tab is None or len(current) <= tab:
                    return None
                return native_callback(friends)
            callback = complete
        return original(self, force, callback)
    return request


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
    if (avatar is not None and hasattr(avatar, 'get_recommend_friends')
            and getattr(avatar, '_hs_recommend_request_revision', 0) < 1):
        avatar.get_recommend_friends = _hs_wrap_recommend_request(avatar.get_recommend_friends)
        avatar._hs_recommend_request_revision = 1
    if (avatar is not None and hasattr(avatar, 'consume_intimacy_gift')
            and getattr(avatar, '_hs_gift_failure_revision', 0) < 1):
        _hs_consume_intimacy_gift.func_defaults = getattr(avatar.consume_intimacy_gift, 'func_defaults', None)
        avatar.consume_intimacy_gift = _hs_consume_intimacy_gift
        avatar._hs_gift_failure_revision = 1
    if (avatar is not None and hasattr(avatar, 'receive_power_supply')
            and getattr(avatar, '_hs_power_supply_callback_revision', 0) < 1):
        avatar.receive_power_supply = _hs_receive_power_supply
        avatar._hs_power_supply_callback_revision = 1
    consign_module = sys.modules.get('guis.activity.main_consign_new')
    consign = consign_module.main_consign_new if consign_module is not None else None
    if (consign is not None and getattr(consign, '_hs_consign_common_revision', 0) < 2
            and hasattr(consign, 'get_vo_bubble_by_card_id')):
        original_lookup = getattr(consign, '_hs_consign_common_original', consign.get_vo_bubble_by_card_id)
        # 1201没有保存原函数；从唯一已知包装闭包取回，避免重复包装。
        if getattr(consign, '_hs_consign_common_revision', 0) == 1:
            function = getattr(original_lookup, 'im_func', original_lookup)
            for name, cell in zip(function.func_code.co_freevars, function.func_closure or ()):
                if name == 'original':
                    original_lookup = cell.cell_contents
                    break
        consign.get_vo_bubble_by_card_id = _hs_wrap_consign_common_lookup(original_lookup)
        consign._hs_consign_common_original = original_lookup
        consign._hs_consign_common_revision = 2
    finish_module = sys.modules.get('guis.activity.consign_finish_new')
    finish_type = getattr(finish_module, 'consign_finish_new', None)
    if (finish_type is not None and getattr(finish_type, '_hs_consign_finish_revision', 0) < 1
            and hasattr(finish_type, 'show_consign_finish_tips')):
        finish_type.show_consign_finish_tips = _hs_wrap_consign_finish(finish_type.show_consign_finish_tips)
        finish_type._hs_consign_finish_revision = 1
    tips_module = sys.modules.get('guis.common.general_tips_mgr')
    tips_type = getattr(tips_module, 'general_tips_mgr', None)
    if (tips_type is not None and getattr(tips_type, '_hs_consign_tips_revision', 0) < 1
            and hasattr(tips_type, 'show_tips')):
        tips_type.show_tips = _hs_wrap_consign_tips(tips_type.show_tips)
        tips_type._hs_consign_tips_revision = 1
    if (guide is not None and getattr(guide, '_hs_guide_show_revision', 0) < 1
            and all(hasattr(guide, name) for name in ('init_show', 'hide', 'add_callback'))):
        beginner_native_init_show = guide.init_show
        beginner_native_hide = guide.hide
        beginner_native_add_callback = guide.add_callback

        def beginner_init_show(self, *args, **kwargs):
            self._hs_guide_show_generation = getattr(self, '_hs_guide_show_generation', 0) + 1
            try:
                return beginner_native_init_show(self, *args, **kwargs)
            except Exception:
                self.hide()
                flow_event('beginner_init_failed', {})
                raise

        def beginner_hide(self, *args, **kwargs):
            self._hs_guide_show_generation = getattr(self, '_hs_guide_show_generation', 0) + 1
            return beginner_native_hide(self, *args, **kwargs)

        def beginner_add_callback(self, interval, callback, *args, **kwargs):
            code = getattr(callback, 'func_code', None)
            # 仅原生init_show中的展示闭包需要场景，不改变其他UI计时器。
            if (code is None or code.co_name != 'delay_show'
                    or not code.co_filename.replace('\\', '/').endswith('guis/prepare/beginner_guide.py')):
                return beginner_native_add_callback(self, interval, callback, *args, **kwargs)
            generation = getattr(self, '_hs_guide_show_generation', 0)
            attempts = [0]

            def scene_ready(*callback_args, **callback_kwargs):
                if (gui.get_ui('beginner_guide') is not self
                        or getattr(self, '_hs_guide_show_generation', 0) != generation):
                    return None
                scene = gworld.get_current_scene()
                if scene is None:
                    attempts[0] += 1
                    if attempts[0] >= 20:
                        self.hide()
                        flow_event('beginner_show_scene_timeout', {'attempts': attempts[0]})
                        return None
                    game3d.delay_exec(100, lambda: scene_ready(*callback_args, **callback_kwargs))
                    return None
                try:
                    return callback(*callback_args, **callback_kwargs)
                except Exception:
                    # 保留原异常供日志定位，同时不能留下只有遮罩的半开界面。
                    self.hide()
                    flow_event('beginner_show_failed', {})
                    raise

            return beginner_native_add_callback(self, interval, scene_ready, *args, **kwargs)

        guide.init_show = beginner_init_show
        guide.hide = beginner_hide
        guide.add_callback = beginner_add_callback
        guide._hs_guide_show_revision = 1
    if guide is not None and getattr(guide, '_hs_guide_error_revision', 0) < 2 and hasattr(guide, 'init_show'):
        beginner_traced_init_show = getattr(guide, '_hs_guide_trace_original', guide.init_show)
        def beginner_init_trace(self, *args, **kwargs):
            try:
                return beginner_traced_init_show(self, *args, **kwargs)
            except Exception as error:
                # 只上报码位置与异常类型，不发送局部变量、参数或异常文本中的玩家数据。
                try:
                    frames = []
                    trace = sys.exc_info()[2]
                    while trace is not None:
                        code = trace.tb_frame.f_code
                        filename = '/'.join(code.co_filename.replace('\\', '/').split('/')[-2:])
                        frames.append([filename[-96:], code.co_name[:64], trace.tb_lineno])
                        # 前几层通常只是列表与保护包装器，末尾才是实际失败点。
                        if len(frames) > 12:
                            del frames[0]
                        trace = trace.tb_next
                    flow_event('beginner_init_error', {'exception': type(error).__name__[:64], 'frames': frames})
                except Exception:
                    pass
                raise
        guide.init_show = beginner_init_trace
        guide._hs_guide_trace_original = beginner_traced_init_show
        guide._hs_guide_error_revision = 2
    if avatar is not None and getattr(avatar, '_hs_empty_explore_revision', 0) < 2 and hasattr(avatar, 'battle_fighting'):
        original_explore_fighting = getattr(avatar, '_hs_empty_team_original', avatar.battle_fighting)
        if getattr(avatar, '_hs_empty_explore_revision', 0) == 1:
            # 旧守卫外还有探索诊断包装；其已保存的原函数才是revision1守卫。
            original_explore_fighting = getattr(avatar, '_hs_explore_battle_fighting_revision_originals', {}).get(
                'battle_fighting', original_explore_fighting)
            function = getattr(original_explore_fighting, 'im_func', original_explore_fighting)
            for name, cell in zip(function.func_code.co_freevars, function.func_closure or ()):
                if name == 'original_explore_fighting':
                    original_explore_fighting = cell.cell_contents
                    break
        def explore_fighting(self, *args, **kwargs):
            import data
            battle = getattr(self, 'battle', None)
            dungeon = data.dungeons.get(getattr(battle, 'dungeon_id', None))
            # 原生check只检验槽位列表存在，[0,0,0,0]也会放行。
            # 普通探索及已确认的剧情塔普通入口恢复编辑；固定/剧情角色和竞技沿原版。
            if battle is not None and dungeon is not None and dungeon.dungeon_type in (2, 23):
                definition = data.battle_info.get(dungeon.dungeon_battle_id)
                cards = battle.get_battle_cards()
                fixed = getattr(definition, 'my_avatar_list', None)
                if not any(cards.fighting_cards) and not any(fixed or ()) and not any(cards.storyline_cards or ()):
                    if not getattr(battle, '_hs_empty_explore_edit_shown', False):
                        gui.battle_layout.on_battle_prepare()
                        battle._hs_empty_explore_edit_shown = True
                        flow_event('explore_empty_team_edit', {'dungeon': battle.dungeon_id})
                    return None
                battle._hs_empty_explore_edit_shown = False
            return original_explore_fighting(self, *args, **kwargs)
        avatar.battle_fighting = explore_fighting
        avatar._hs_empty_team_original = original_explore_fighting
        avatar._hs_empty_explore_revision = 2
        # 重新让已有诊断器包住新守卫，不能继续调用它缓存的revision1守卫。
        avatar._hs_explore_battle_fighting_revision = 0
        avatar._hs_explore_battle_fighting_revision_originals = {}
    # 原生替换模型后仍用已销毁的旧局部变量校验eid；只修正该分支的返回引用。
    performance_module = sys.modules.get('battle_logic.battle_performance')
    performance_type = getattr(performance_module, 'battle_performance', None)
    if performance_type is not None and getattr(performance_type, '_hs_entity_replace_revision', 0) < 1:
        def create_entity(self, eid, info, replace_attrs=None, callback=None):
            import data
            attrs = dict(replace_attrs) if replace_attrs else dict(info.attrs)
            attrs['battle'] = self
            attrs['camp'] = self.camps[attrs['camp_id']]
            attrs['pos'] = self.get_pos_by_coord(attrs['hex_coord'])
            attrs['time_scale'] = self.time_scale
            if 'appear_action' not in attrs:
                attrs['appear_action'] = data.role_model[attrs['model_id']].appear_action
            if eid not in self.entities:
                self.create_entity_enter_world(eid, attrs, callback)
            elif self.entities[eid].model_id == attrs['model_id']:
                self.entities[eid].init_from_dict(attrs)
            else:
                self.remove_scene_entity(eid)
                self.create_entity_enter_world(eid, attrs, callback)
            entity = self.entities[eid]
            if attrs.get('is_summon'):
                master = gworld.get_entity(attrs['master'])
                if master:
                    entity.set_yaw(master.get_yaw())
            assert entity.eid == eid
            return entity
        performance_type.create_entity = create_entity
        performance_type._hs_entity_replace_revision = 1
    # gen_class把组件方法复制进战斗类；只改组件不会更新已装配的函数。
    battle_classes = sys.modules.get('battle_logic.client_battle')
    if performance_type is not None and battle_classes is not None:
        for class_name in ('local_battle', 'shadow_battle', 'record_battle', 'watch_battle',
                           'pvp_layout_battle', 'async_pvp_layout_battle',
                           'consign_layout_battle', 'team_preset_layout_battle'):
            owner = getattr(battle_classes, class_name, None)
            method = getattr(owner, 'create_entity', None)
            function = getattr(method, 'im_func', method)
            code = getattr(function, 'func_code', None)
            if code is not None and code.co_filename.replace('\\', '/').endswith('battle_logic/battle_performance.py'):
                owner.create_entity = performance_type.create_entity.im_func
                owner._hs_entity_replace_revision = 1
    reward_type = getattr(guide_module, 'reward_item', None)
    if reward_type is not None and getattr(reward_type, '_hs_cached_box_revision', 0) < 2:
        original_reward_update = reward_type.update
        def reward_update(self):
            info = self.get_info()
            if info is not None:
                key = 'checkin_bonus.day_%d' % info[0]
                old = cache.get(key, None)
                if old is not None:
                    converted = _hs_checkin_bonus_list(old)
                    if converted is not old:
                        cache.set(key, converted)
            return original_reward_update(self)
        reward_type.update = reward_update
        reward_type._hs_cached_box_revision = 2
    achievement_module = sys.modules.get('guis.mission.mission_achievement_pandect')
    achievement_type = getattr(achievement_module, 'mission_achievement_pandect', None)
    if achievement_type is not None and getattr(achievement_type, '_hs_reward_callback_revision', 0) < 1:
        original_achievement_reward = achievement_type.on_receive_bonus
        def achievement_reward(self, bonus_box):
            if getattr(self, 'list_medal', None) is not None:
                return original_achievement_reward(self, bonus_box)
            gui.general_tips_mgr.show_tips('item_get', ('reward', bonus_box.to_list()))
            flow_event('achievement_reward_view_closed', {})
        achievement_type.on_receive_bonus = achievement_reward
        achievement_type._hs_reward_callback_revision = 1
    recommend_module = sys.modules.get('guis.social.friend_recommend')
    recommend_type = getattr(recommend_module, 'friend_recommend', None)
    if recommend_type is not None and getattr(recommend_type, '_hs_recommend_callback_revision', 0) < 1:
        original_recommend_reply = recommend_type.on_get_search_friend
        def recommend_reply(self, friends):
            rows = getattr(self, 'friend_items', None)
            if rows is None or len(rows) <= recommend_module.recommend_tab:
                flow_event('friend_recommend_view_closed', {})
                return None
            return original_recommend_reply(self, friends)
        recommend_type.on_get_search_friend = recommend_reply
        recommend_type._hs_recommend_callback_revision = 1
    # 入口可能在客户端校验阶段被阻断，服务端没有RPC并不等于没有点击。
    # 仅观察原生返回值和生命周期，不强清模型、不跳过剧情或修改门槛。
    def dungeon_state(self, dungeon_id=None):
        try:
            battle = getattr(self, 'battle', None)
            return {'dungeon': dungeon_id, 'has_battle': battle is not None,
                    'battle_dungeon': getattr(battle, 'dungeon_id', None),
                    'battle_process': getattr(battle, 'battle_process', None),
                    'model_ready': getattr(battle, 'bid', None) is not None,
                    'guide_waiting': bool(getattr(self, 'in_guide', False)),
                    'story_waiting': bool(getattr(self, 'in_guide_storyline', False)),
                    'network_connected': bool(getattr(getattr(gworld, 'network_mgr', None), 'has_connected', False)),
                    'scene_present': gworld.get_current_scene() is not None}
        except Exception:
            return {'dungeon': dungeon_id, 'snapshot_unavailable': True}
    if avatar is not None and getattr(avatar, '_hs_dungeon_flow_revision', 0) < 2 and hasattr(avatar, 'enter_dungeon'):
        originals = getattr(avatar, '_hs_dungeon_flow_originals', {})
        def observe_method(name):
            original = originals.get(name, getattr(avatar, name))
            if getattr(avatar, '_hs_dungeon_flow_revision', 0) == 1:
                old_function = getattr(original, 'im_func', original)
                for key, cell in zip(old_function.func_code.co_freevars, old_function.func_closure or ()):
                    if key == 'original':
                        original = cell.cell_contents
                        break
            originals[name] = original
            def observed(self, *args, **kwargs):
                dungeon_id = args[0] if name in ('enter_dungeon', 'check_dungeon') and args else None
                flow_event(name + '_begin', dungeon_state(self, dungeon_id))
                try:
                    result = original(self, *args, **kwargs)
                except Exception:
                    flow_event(name + '_error', dungeon_state(self, dungeon_id))
                    raise
                values = dungeon_state(self, dungeon_id)
                if name in ('enter_dungeon', 'check_dungeon'):
                    # 原生assemble包装可能返回None；不能把已发RPC记成拒绝。
                    values['result_type'] = type(result).__name__
                    if isinstance(result, bool):
                        values['accepted'] = result
                flow_event(name + '_end', values)
                if name == 'enter_dungeon' and result is not False:
                    token = object()
                    self._hs_dungeon_probe_token = token
                    def inspect_entry():
                        if getattr(self, '_hs_dungeon_probe_token', None) is token:
                            flow_event('dungeon_after_entry', dungeon_state(self, dungeon_id))
                    try:
                        game3d.delay_exec(15000, inspect_entry)
                    except Exception:
                        pass
                return result
            return observed
        for name in ('enter_dungeon', 'check_dungeon', 'raw_clear_battle', 'on_dungeon_finish'):
            if hasattr(avatar, name):
                setattr(avatar, name, observe_method(name))
        avatar._hs_dungeon_flow_originals = originals
        avatar._hs_dungeon_flow_revision = 2
    main_module = sys.modules.get('guis.prepare.main_line_chapter')
    main_type = getattr(main_module, 'main_line_chapter', None)
    if main_type is not None and getattr(main_type, '_hs_open_flow_revision', 0) < 2:
        original_main_show = getattr(main_type, '_hs_open_flow_original', main_type.init_show)
        if getattr(main_type, '_hs_open_flow_revision', 0) == 1 and not hasattr(main_type, '_hs_open_flow_original'):
            old_function = getattr(original_main_show, 'im_func', original_main_show)
            for name, cell in zip(old_function.func_code.co_freevars, old_function.func_closure or ()):
                if name == 'original_main_show':
                    original_main_show = cell.cell_contents
                    break
        def main_show(self, *args, **kwargs):
            player = gworld.get_player()
            if player is not None:
                flow_event('mainline_open_begin', dungeon_state(player))
            try:
                result = original_main_show(self, *args, **kwargs)
            except Exception:
                import traceback
                flow_event('mainline_open_error', {'message': traceback.format_exc()[-6000:]})
                raise
            if player is not None:
                flow_event('mainline_open_end', dungeon_state(player))
            return result
        main_type.init_show = main_show
        main_type._hs_open_flow_original = original_main_show
        main_type._hs_open_flow_revision = 2
    widgets_module = sys.modules.get('guis.widgets')
    double_type = getattr(widgets_module, 'panel_double', None)
    if double_type is not None and hasattr(double_type, 'update_cost_text') and getattr(double_type, '_hs_cost_callback_revision', 0) < 1:
        original_cost_update = double_type.update_cost_text
        def cost_update(self, need_power):
            if getattr(self, 'text_cost', None) is None:
                flow_event('double_cost_view_closed', {})
                return None
            return original_cost_update(self, need_power)
        double_type.update_cost_text = cost_update
        double_type._hs_cost_callback_revision = 1
    if double_type is not None and getattr(double_type, '_hs_double_callback_revision', 0) < 1:
        original_double_update = double_type.update_double_button
        def double_update(self):
            # 原生set_callback后续仍需调用press_callback；仅跳过已销毁控件的绘制。
            # 不吞掉回调，不修改倍数、费用或服务端确认结果。
            if getattr(self, 'img_on', None) is None:
                flow_event('double_widget_closed', {})
                return None
            return original_double_update(self)
        double_type.update_double_button = double_update
        double_type._hs_double_callback_revision = 1
    map_module = sys.modules.get('guis.prepare.free_stage_map')
    map_type = getattr(map_module, 'free_stage_map', None)
    if map_type is not None and getattr(map_type, '_hs_flow_revision', 0) < 2:
        original_map_show = getattr(map_type, '_hs_flow_original', map_type.init_show)
        if getattr(map_type, '_hs_flow_revision', 0) == 1:
            old_function = getattr(original_map_show, 'im_func', original_map_show)
            for key, cell in zip(old_function.func_code.co_freevars, old_function.func_closure or ()):
                if key == 'original_map_show':
                    original_map_show = cell.cell_contents
                    break
        def map_show(self, *args, **kwargs):
            flow_event('explore_open', {'stage': args[0] if args else None})
            try:
                result = original_map_show(self, *args, **kwargs)
            except Exception:
                import traceback
                flow_event('explore_open_error', {'message': traceback.format_exc()[-6000:]})
                raise
            flow_event('explore_ready', {'stage': getattr(self, 'free_stage_id', None)})
            return result
        map_type.init_show = map_show
        map_type._hs_flow_original = original_map_show
        map_type._hs_flow_revision = 2
    # 探索全服快照停在ready/loaded之后；观察原生场景与布阵调用，
    # 不自动开战、不取消等待、不改变检查返回值或异常。
    def explore_state():
        try:
            player = gworld.get_player()
            values = dungeon_state(player)
            battle = getattr(player, 'battle', None)
            values.update({'stage': getattr(player, 'cur_free_stage_id', None),
                           'auto_fight': bool(getattr(player, 'auto_fight', False)),
                           'auto_fighting': bool(getattr(player, 'auto_fighting', False)),
                           'fight_flag': bool(getattr(player, 'battle_fight_flag', False)),
                           'scene_loaded': bool(getattr(battle, 'scene_loaded', False)),
                           'battle_status': getattr(battle, 'battle_status', None),
                           'pending_models': len(getattr(battle, 'wait_load_eid_list', []) or []),
                           'layout_visible': bool(gui.is_visible('battle_layout'))})
            return values
        except Exception:
            return {'snapshot_unavailable': True}
    def observe_explore(owner, names, always=False):
        revision_key = '_hs_explore_' + names[0] + '_revision'
        if owner is None or getattr(owner, revision_key, 0) >= 2:
            return
        originals = getattr(owner, revision_key + '_originals', {})
        if not originals and names[0] == 'on_battle_prepare':
            originals = dict(getattr(owner, '_hs_explore_init_show_revision_originals', {}))
        def wrap(name, original):
            def observed(self, *args, **kwargs):
                try:
                    player = gworld.get_player()
                    battle = getattr(player, 'battle', None)
                    data_module = sys.modules.get('data')
                    dungeon = data_module.dungeons.get(getattr(battle, 'dungeon_id', None))
                    live = always or getattr(dungeon, 'dungeon_type', None) == 2
                except Exception:
                    live = always
                if live and name == 'check_auto_battle_fighting':
                    count = getattr(self, '_hs_explore_auto_probe_count', 0)
                    self._hs_explore_auto_probe_count = count + 1
                    live = count < 4
                if live:
                    flow_event('explore_' + name + '_begin', explore_state())
                try:
                    result = original(self, *args, **kwargs)
                except Exception:
                    if live:
                        import traceback
                        values = explore_state()
                        values['message'] = traceback.format_exc()[-6000:]
                        flow_event('explore_' + name + '_error', values)
                    raise
                if live:
                    values = explore_state()
                    values['result_type'] = type(result).__name__
                    if result is None or isinstance(result, (bool, int, long)):
                        values['result'] = result
                    flow_event('explore_' + name + '_end', values)
                    if name == 'enter_stage_site':
                        token = object()
                        self._hs_explore_probe_token = token
                        def inspect_entry():
                            if getattr(self, '_hs_explore_probe_token', None) is token:
                                flow_event('explore_node_after_entry', explore_state())
                        try:
                            game3d.delay_exec(15000, inspect_entry)
                        except Exception:
                            pass
                return result
            return observed
        for name in names:
            if hasattr(owner, name):
                method = getattr(owner, name)
                rpc_method = getattr(method, 'rpcmethod', None)
                if rpc_method is not None and callable(getattr(rpc_method, 'func', None)):
                    originals[name] = originals.get(name, rpc_method.func)
                    rpc_method.func = wrap(name, originals[name])
                else:
                    originals[name] = originals.get(name, method)
                    setattr(owner, name, wrap(name, originals[name]))
        setattr(owner, revision_key + '_originals', originals)
        setattr(owner, revision_key, 2)
    observe_explore(avatar, ('enter_free_stage', 'on_enter_free_stage',
                             'enter_stage_site', 'on_enter_stage_site'), True)
    observe_explore(avatar, ('battle_fighting',))
    battle_module = sys.modules.get('battle_logic.client_battle')
    shadow_type = getattr(battle_module, 'shadow_battle', None)
    observe_explore(shadow_type, ('on_scene_loaded', 'check_auto_battle_fighting'))
    # 原生gen_class提前把组件函数收进assemble闭包；改battle_mgr组件类
    # 不会替换已生成的调用链，必须包装实际客户端战斗类的最终入口。
    for battle_name in ('local_battle', 'shadow_battle', 'record_battle', 'watch_battle'):
        input_type = getattr(battle_module, battle_name, None)
        if (input_type is not None and hasattr(input_type, 'on_allow_player_input')
                and getattr(input_type, '_hs_input_owner_revision', 0) < 1):
            input_type.on_allow_player_input = _hs_wrap_battle_input_owner(input_type.on_allow_player_input)
            input_type._hs_input_owner_revision = 1
    layout_module = sys.modules.get('guis.battle.battle_layout')
    layout_type = getattr(layout_module, 'battle_layout', None)
    # 撤销1101空面板补丁：正式shadow场景没有初始化布阵，补面板会继续
    # 在main_slots_area崩溃。保留真正的布阵函数与实际援护控件。
    if layout_type is not None and getattr(layout_type, '_hs_missing_support_panel_revision', 0) == 1:
        layout_type.set_support_mode = layout_type._hs_missing_support_panel_original
        layout_type._hs_missing_support_panel_revision = 2
    pvp_module = sys.modules.get('guis.battle.battle_pvp')
    pvp_type = getattr(pvp_module, 'battle_pvp', None)
    if pvp_type is not None and getattr(pvp_type, '_hs_formal_ready_revision', 0) < 1:
        native_player_ready = pvp_type.on_player_ready
        def player_ready(self, eid):
            battle = gworld.get_battle()
            extra = getattr(battle, 'extra_info', {}) or {}
            if (battle is not None and not battle.is_layout_battle()
                    and (extra.get('sync_pvp') or extra.get('human_shared')
                         or extra.get('server_authoritative_pvp'))):
                # 仅处理旧服务器迟到的布阵就绪通知；正式开战由加载屏障推进。
                flow_event('pvp_stale_layout_ready_ignored', {})
                return None
            return native_player_ready(self, eid)
        pvp_type.on_player_ready = player_ready
        pvp_type._hs_formal_ready_revision = 1
    observe_explore(layout_type, ('on_battle_prepare', 'start_battle'))
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
        for item_name in ('task_item', 'activity_task_item'):
            item_type = getattr(guide_module, item_name, None)
            if (item_type is not None and hasattr(item_type, 'update')
                    and getattr(item_type, '_hs_live_task_update_revision', 0) < 1):
                item_type.update = _hs_wrap_live_task_update(item_type.update)
                item_type._hs_live_task_update_revision = 1
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
    if guide is not None and getattr(guide, '_hs_safe_checkin_revision', 0) < 4:
        def get_item(self, bonus_dict):
            if not bonus_dict:
                return None
            items = []
            self.reward_days = []
            for day, values in bonus_dict.iteritems():
                # 全领回包按日携带原生奖励盒，单领回调已调用to_list；两者统一后再缓存。
                values = _hs_checkin_bonus_list(values)
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
        guide._hs_safe_checkin_revision = 4
    if summon is not None and getattr(summon, '_hs_safe_result_revision', 0) < 2:
        original = getattr(summon, '_hs_result_original', summon.on_get_card_result)
        if getattr(summon, '_hs_safe_result_revision', 0) == 1:
            function = getattr(original, 'im_func', original)
            for name, cell in zip(function.func_code.co_freevars, function.func_closure or ()):
                if name == 'original':
                    original = cell.cell_contents
                    break
        def live_summon(self):
            return (gui.get_ui('summon_new_card') is self
                    and not (hasattr(self, '_ins') and self._ins is None))
        def on_get_card_result(self, cards, first_card_ids, eureka, extra_bonus_list):
            pending = (cards, first_card_ids, eureka, extra_bonus_list)
            # 原生成功回调绑定旧页面。页面销毁后不能访问main_layer、卡池及模型，
            # 也不能把当前幻书页面强行切回已释放的召唤场景。真实资产已在RPC前同步。
            if not live_summon(self):
                self._hs_pending_summon_result = pending
                flow_event('summon_result_stale_ui', {'result_retained': True})
                return None
            scene = gworld.get_current_scene()
            if scene is not None and hasattr(scene, 'form_summon_paper_entity_list'):
                return original(self, cards, first_card_ids, eureka, extra_bonus_list)
            # 原生init_show第一步就是激活已预载召唤场景；不重发抽卡请求。
            self._hs_pending_summon_result = pending
            if getattr(gworld, 'get_battle', lambda: None)() is None:
                gworld.scene_mgr.activate_preload_scene()
            state = {'attempts': 0}
            def ready():
                if getattr(self, '_hs_pending_summon_result', None) is not pending:
                    return
                if not live_summon(self):
                    flow_event('summon_result_stale_ui', {'result_retained': True})
                    return None
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
        summon._hs_result_original = original
        summon._hs_safe_result_revision = 2
    preset_module = sys.modules.get('guis.battle.battle_team_preset_list')
    preset_list = getattr(preset_module, 'battle_team_preset_list', None)
    if (preset_list is not None and getattr(preset_list, '_hs_team_list_revision', 0) < 1
            and hasattr(preset_list, 'update_team_list')):
        original_team_list = preset_list.update_team_list

        def team_list_safe(self, team_list):
            # 保存阵容预设的回包可能晚于页面销毁；控件已释放时跳过纯UI重建，
            # 预设数据由玩家实体属性同步，不依赖这次列表重建。
            if getattr(self, 'team_list', None) is None:
                return None
            return original_team_list(self, team_list)

        preset_list.update_team_list = team_list_safe
        preset_list._hs_team_list_revision = 1
    return guide is not None and summon is not None and avatar is not None and map_type is not None and shadow_type is not None and layout_type is not None

def _start_hs_ui_callback_repair():
    import game3d
    def attempt():
        try:
            ready = _install_hs_ui_callback_repair()
        except Exception as error:
            print('HS_UI_CALLBACK_WAIT %s' % error)
            ready = False
        # 成就/好友界面按需加载；核心安装成功后仍低频检查，避免首次打开遗漏修补。
        game3d.delay_exec(5000 if ready else 1000, attempt)
    attempt()

_start_hs_ui_callback_repair()
