# -*- coding: utf-8 -*-
# 包装源用于导出Android Python2热更；不在Python3服务端导入。
def _install_revival_authority_bridge():
    import functools
    import gworld
    from battle_logic import client_battle
    from entities.components import battle as battle_component
    from guis.battle import battle_mode, battle_skill

    base = client_battle.shadow_battle
    if getattr(base, '_revival_bridge_version', 0) not in (12, 13, 14, 15):
        raise RuntimeError('PVP权威需要兼容普通战斗桥版本12至15')
    rpc_method = getattr(battle_component.battle.start_server_battle_ok, 'rpcmethod', None)
    if rpc_method is None or not callable(getattr(rpc_method, 'func', None)):
        raise RuntimeError('PVP native factory RPC metadata missing')
    if getattr(rpc_method.func, '_revival_authority_factory', False):
        return

    def function(method):
        return getattr(method, 'im_func', method)

    class authority_shadow(base):
        _revival_server_authority = True
        _revival_bridge_originals = dict(getattr(base, '_revival_bridge_originals', {}))

    # 作者安装器还写入三个GUI钩子和local洗牌函数；完整保存后再精确恢复。
    surfaces = [(client_battle.local_battle, 'shuffle_random_speed'),
                (battle_mode.battle_mode, 'press_auto'),
                (battle_mode.battle_mode, '_revival_press_auto_original'),
                (battle_skill.skill_button, 'on_click_begin'),
                (battle_skill.skill_button, 'on_click_end'),
                (battle_skill.skill_button, '_revival_click_begin_original'),
                (battle_skill.skill_button, '_revival_click_end_original')]
    saved = [(owner, name, name in owner.__dict__, owner.__dict__.get(name)) for owner, name in surfaces]
    previous_gui = {(owner, name): function(getattr(owner, name)) for owner, name in
                    [(battle_mode.battle_mode, 'press_auto'),
                     (battle_skill.skill_button, 'on_click_begin'),
                     (battle_skill.skill_button, 'on_click_end')]}
    installed = {}
    try:
        client_battle.shadow_battle = authority_shadow
        _install_revival_authority_shadow_bridge()
        for owner, name in surfaces:
            installed[(owner, name)] = getattr(owner, name)
    finally:
        client_battle.shadow_battle = base
        for owner, name, exists, value in saved:
            if exists:
                setattr(owner, name, value)
            elif name in owner.__dict__:
                delattr(owner, name)

    def authoritative_scene():
        scene = gworld.get_battle()
        return (isinstance(scene, authority_shadow) and
                (getattr(scene, 'extra_info', {}) or {}).get('server_authoritative_pvp') is True)

    def gated(original, authority):
        @functools.wraps(original)
        def invoke(self, *args, **kwargs):
            if authoritative_scene():
                return authority(self, *args, **kwargs)
            return original(self, *args, **kwargs)
        return invoke

    # 普通场景仍调用原先已安装的函数。仅当前显式权威场景使用作者UI钩子。
    gui_hooks = [(owner, name, gated(previous, function(installed[(owner, name)])))
                 for (owner, name), previous in previous_gui.items()]

    original_factory = rpc_method.func
    @functools.wraps(original_factory)
    def factory(self, battle_type, dungeon_id, battle_uuid, extra_info):
        if not isinstance(extra_info, dict) or extra_info.get('server_authoritative_pvp') is not True:
            return original_factory(self, battle_type, dungeon_id, battle_uuid, extra_info)
        # 本版原生工厂同步构造shadow_battle；即使原生工厂报错也恢复模块入口。
        previous = client_battle.shadow_battle
        try:
            client_battle.shadow_battle = authority_shadow
            return original_factory(self, battle_type, dungeon_id, battle_uuid, extra_info)
        finally:
            client_battle.shadow_battle = previous
    factory._revival_authority_factory = True
    factory._revival_authority_shadow = authority_shadow
    for owner, name, method in gui_hooks:
        setattr(owner, name, method)
    # 原生注册表与装饰器都通过同一RpcMethod.func调用，不能仅替换类方法。
    rpc_method.func = factory


def _revival_start_authority_bridge():
    try:
        _install_revival_authority_bridge()
        print('REVIVAL_PVP_AUTHORITY_BRIDGE_READY 1')
    except Exception as error:
        print('REVIVAL_PVP_AUTHORITY_BRIDGE_WAIT %s' % error)
        import game3d
        game3d.delay_exec(1500, _revival_start_authority_bridge)


_revival_start_authority_bridge()
