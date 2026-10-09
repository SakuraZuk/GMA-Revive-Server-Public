# -*- coding: utf-8 -*-
"""新进程登录时优先恢复服务端未结战斗，避免引导直接进入下一关而黑屏。"""


def _install_hs_login_battle_recovery():
    import sys
    import game3d
    import gworld

    avatar_module = sys.modules.get('entities.Avatar')
    if avatar_module is None:
        raise RuntimeError('entities.Avatar 尚未加载')
    avatar_type = avatar_module.Avatar
    if getattr(avatar_type, '_hs_login_battle_recovery_revision', 0) >= 10:
        return

    original_refresh = getattr(avatar_type, '_hs_login_battle_recovery_original', avatar_type.on_refresh_login)
    original_start_battle = getattr(avatar_type, '_hs_login_battle_recovery_start_original', avatar_type.start_server_battle_ok)
    original_preload = getattr(avatar_type, '_hs_login_battle_recovery_preload_original', avatar_type.preload_by_refresh_login)
    original_load_finished = getattr(avatar_type, '_hs_login_battle_recovery_load_original', avatar_type.load_scene_finished)
    original_do_guide = getattr(avatar_type, '_hs_login_battle_recovery_guide_original', avatar_type.do_guide_task)

    def do_guide_task(self, callback=None, pre_callback=None):
        # 原生带通关后剧情的handle_dungeon_clear，其end_callback直接调用
        # do_guide_task，漏掉无剧情分支的胜利条件通知，导致同一副本反复进入。
        # 这里只收尾已经存在、执行中且dungeon_mgr证实通关的副本型教学；
        # 剧情与教学交互仍交给原生guide_task_finished，资产由服务端校验。
        import data
        for task_id in list(self.guide_tasks.keys()):
            task = self.guide_tasks.get(task_id)
            if task is None:
                continue
            if not task.is_doing():
                continue
            definition = data.guide_task[task_id]
            if definition.do_type != 2 or not definition.do_type_params:
                continue
            dungeon_id = int(definition.do_type_params[0])
            dungeon = self.dungeon_mgr.get(dungeon_id)
            if dungeon is not None and dungeon.finished:
                print('HS_GUIDE_DUNGEON_COMPLETED %s %s' % (task_id, dungeon_id))
                return self.guide_task_finished(task_id, callback)
        return original_do_guide(self, callback=callback, pre_callback=pre_callback)

    def preload_by_refresh_login(self):
        battle = gworld.get_battle()
        if battle is None:
            return original_preload(self)
        # 原生刷新会重播活动引导的开场剧情；剧情切换场景会销毁已恢复的
        # 战斗模型。战斗已由prepare自行加载，只复用原生登录退场与UI清理。
        self._hs_login_recovery_scene = battle
        print('HS_LOGIN_BATTLE_RECOVERY_LEAVE_ONLY')
        return self.preload_scene_finished(True)

    def load_scene_finished(self, in_battle):
        battle = getattr(self, '_hs_login_recovery_scene', None)
        if battle is not None:
            self._hs_login_recovery_scene = None
            if gworld.get_battle() is battle:
                # prepare已经负责on_scene_loaded，不能重复生成实体或重启剧情。
                print('HS_LOGIN_BATTLE_RECOVERY_SCENE_PRESERVED')
                return None
        return original_load_finished(self, in_battle)

    def start_server_battle_ok(self, *args, **kwargs):
        # AsioGateClient.entity_message实际调用method(parameters)，即单个
        # 位置字典；兼容RpcMethod展开关键字及本地四个位置参数调用。
        parameters = args[0] if len(args) == 1 and isinstance(args[0], dict) else kwargs
        extra = args[3] if len(args) >= 4 else parameters.get('_3', parameters.get('extra_info', {}))
        previous_uuid = extra.get('hs_recover_previous_uuid') if isinstance(extra, dict) else None
        current = getattr(self, 'battle', None)
        new_uuid = args[2] if len(args) >= 3 else parameters.get('_2', parameters.get('battle_uuid'))
        if previous_uuid and current is not None:
            if str(getattr(current, 'id', '')) == str(new_uuid):
                return None
            # 原生2034D84C入口释放模型/回调并置空battle；不会调用通关或发奖。
            # 服务器恢复标记授权替换当前未结会话；多次失败重连后，客户端
            # 可能仍保留更早的UUID，不能要求它恰好等于服务器上一轮UUID。
            old_uuid = str(getattr(current, 'id', ''))
            self.raw_clear_battle()
            print('HS_LOGIN_BATTLE_RECOVERY_REPLACED %s %s' % (old_uuid, previous_uuid))
        # 服务端登录恢复链先创建战斗、最后再推on_refresh_login。记录该事实，
        # 让刷新只清理登录遮罩，不重复请求第二次恢复。
        self._hs_login_server_battle_started = True
        return original_start_battle(self, *args, **kwargs)

    def on_refresh_login(self, *args, **kwargs):
        if getattr(self, '_hs_login_server_battle_started', False):
            self._hs_login_server_battle_started = False
            print('HS_LOGIN_BATTLE_RECOVERY_SERVER_STARTED')
            return original_refresh(self, *args, **kwargs)
        try:
            if gworld.get_battle() is not None:
                print('HS_LOGIN_BATTLE_RECOVERY_ALREADY_STARTED')
                return original_refresh(self, *args, **kwargs)
        except Exception as error:
            print('HS_LOGIN_BATTLE_RECOVERY_PRECHECK_ERROR %s' % error)
        # 本版服务端永久入口保证未结战斗先恢复、后刷新。无battle意味着
        # 没有未结会话或已提交结算；不能再请求恢复把旧结果缓存到无实体场景。
        print('HS_LOGIN_BATTLE_RECOVERY_NATIVE_FINISH')
        return original_refresh(self, *args, **kwargs)

    avatar_type.start_server_battle_ok = start_server_battle_ok
    avatar_type.on_refresh_login = on_refresh_login
    avatar_type.preload_by_refresh_login = preload_by_refresh_login
    avatar_type.load_scene_finished = load_scene_finished
    avatar_type.do_guide_task = do_guide_task
    avatar_type._hs_login_battle_recovery_start_original = original_start_battle
    avatar_type._hs_login_battle_recovery_original = original_refresh
    avatar_type._hs_login_battle_recovery_preload_original = original_preload
    avatar_type._hs_login_battle_recovery_load_original = original_load_finished
    avatar_type._hs_login_battle_recovery_guide_original = original_do_guide
    avatar_type._hs_login_battle_recovery_revision = 10


def _start_hs_login_battle_recovery():
    def attempt():
        try:
            _install_hs_login_battle_recovery()
            print('HS_LOGIN_BATTLE_RECOVERY_READY 1')
        except Exception as error:
            print('HS_LOGIN_BATTLE_RECOVERY_WAIT %s' % error)
            try:
                import game3d
                game3d.delay_exec(1000, attempt)
            except Exception as retry_error:
                print('HS_LOGIN_BATTLE_RECOVERY_RETRY_ERROR %s' % retry_error)
    attempt()


_start_hs_login_battle_recovery()
