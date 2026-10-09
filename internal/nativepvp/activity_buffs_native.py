# -*- coding: utf-8 -*-
# 本版夏日原生提示六种书名的四类收益+60%；公式和舍入继续走原生。
def _install_hs_activity_buffs():
    from common_logic import battle_common
    import common_const
    import gworld
    cls = battle_common.base_hp_change_struct
    old = cls.set_effect_type
    if getattr(old, '_hs_activity_buff_revision', 0) == 1:
        return
    # role_info.book_name 对照六种书名。2207/3412为同书另一角色模板。
    roles = frozenset((2102, 2202, 2207, 3403, 3404, 3412, 3502, 5222))

    def set_effect_type(self, effect_type):
        import gworld
        result = old(self, effect_type)
        if effect_type not in (common_const.DIRECT_DAMAGE, common_const.INDIRECT_DAMAGE):
            return result
        battle = gworld.get_battle()
        if battle is None or not getattr(battle, 'extra_info', {}).get('hs_summer_closed_beta', False):
            return result
        user = self.user
        if user is None or user.get_camp_id() != 1 or user.get_attr('role_id') not in roles:
            return result
        # init_rate 对同名已经存在的倍率幂等；set_rate使原生缓存重新求值。
        # damage_struct与cure_struct共同经过此点，间接治疗也走原类型与上限。
        self.init_rate('hs_summer_closed_beta', 1.6)
        return result

    set_effect_type._hs_activity_buff_revision = 1
    cls.set_effect_type = set_effect_type
