# -*- coding: utf-8 -*-
# Android FBBE590A 战斗统计；只给原桥的同一次 result 增补原生值。
def _install_hs_activity_metrics():
    import math
    import types
    from battle_logic import client_battle
    shadow = client_battle.shadow_battle
    if getattr(shadow, '_revival_bridge_version', 0) not in (12, 13, 14, 15):
        raise ValueError('活动统计等待兼容战斗桥版本12至15')

    def function(value):
        return getattr(value, 'im_func', getattr(value, '__func__', value))

    def code(value):
        return getattr(value, 'func_code', getattr(value, '__code__', None))

    def closure(value):
        return getattr(value, 'func_closure', getattr(value, '__closure__', None))

    def new_cell(value):
        def holder():
            return value
        return closure(holder)[0]

    def replace_cell(value, name, replacement):
        value = function(value)
        names = code(value).co_freevars
        cells = list(closure(value) or ())
        if name not in names:
            raise ValueError('活动统计未找到原生桥闭包 ' + name)
        cells[names.index(name)] = new_cell(replacement)
        result = types.FunctionType(code(value),
            getattr(value, 'func_globals', getattr(value, '__globals__', None)),
            getattr(value, 'func_name', getattr(value, '__name__', 'notify_finish')),
            getattr(value, 'func_defaults', getattr(value, '__defaults__', None)), tuple(cells))
        return result

    current = function(shadow.notify_battle_finish)
    human = getattr(shadow, '_revival_human_originals', {})
    original = function(human.get('notify_battle_finish', current))
    if getattr(original, '_hs_activity_metrics_revision', 0) == 3:
        return
    names = code(original).co_freevars
    cells = closure(original) or ()
    if 'report' not in names:
        raise ValueError('活动统计等待原战斗结果回调')
    report = cells[names.index('report')].cell_contents

    def native_integer(value):
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            # Python2 的 long 也属于合法原生整数。
            try:
                is_long = isinstance(value, long)
            except NameError:
                is_long = False
            if not is_long:
                raise ValueError('活动统计必须为原生数字')
        if math.isnan(float(value)) or math.isinf(float(value)) or value < 0:
            raise ValueError('活动统计必须为有限非负数')
        result = int(value)
        if result > 9007199254740991:
            raise ValueError('活动统计超过准确传输范围')
        return result

    old_end_notice = function(shadow.battle_end_notice)

    def capture_statistics(self):
        # 原生 battle_end 会先清空 bid，再进入 notify_battle_finish。
        # 因此必须在 battle_end_notice 阶段冻结统计，结果回调只读快照。
        self._hs_activity_total_ap_statistics = native_integer(
            self.get_battle_ap_statistics())
        try:
            damaged = self.get_total_extra_statistics()
        except KeyError:
            # 只兼容已被原生生命周期清空的 bid；有效 bid 查表失败仍上抛。
            if getattr(self, 'bid', None) is not None:
                raise
            damaged = 0
        self._hs_activity_total_damaged_statistics = native_integer(damaged)

    def end_notice(self, *args, **kwargs):
        capture_statistics(self)
        return old_end_notice(self, *args, **kwargs)

    def metrics_report(self, kind, values, *args, **kwargs):
        if kind == 'result':
            values = dict(values)
            if not hasattr(self, '_hs_activity_total_ap_statistics'):
                capture_statistics(self)
            # C9FDF16E 原生UI同样使用 int(get_battle_ap_statistics())。
            values['total_ap_statistics'] = self._hs_activity_total_ap_statistics
            values['total_damaged_statistics'] = self._hs_activity_total_damaged_statistics
            if self.is_league_protect():
                # CF38A2E4原生雅努斯面板：死亡、非召唤，501独计不算50守门怪。
                import data
                protect = next(row for row in data.league_protect.itervalues()
                               if row.dungeon_id == self.dungeon_id)
                kills, treasure = 0, 0
                for info in self.get_statistics().get(2, {}).itervalues():
                    role = info.get('role_id', 0)
                    if not info.get('dead') or not role or info.get('is_summon'):
                        continue
                    tags = data.role_info[role].role_tag or ()
                    if any(tag in tags for tag in protect.alone_count_tag):
                        if treasure and treasure != role:
                            raise ValueError('雅努斯宝藏角色统计不唯一')
                        treasure = role
                    else:
                        kills += 1
                values['league_protect_statistics'] = {'kill_count': kills,
                                                       'treasure_role_id': treasure}
        return report(self, kind, values, *args, **kwargs)

    patched = replace_cell(original, 'report', metrics_report)
    patched._hs_activity_metrics_revision = 3
    if 'notify_battle_finish' in human:
        # 热更已装真人包装时，保留其胜负协议，只更新 old_notify 闭包。
        shadow.notify_battle_finish = replace_cell(current, 'old_notify', patched)
        human['notify_battle_finish'] = patched
    else:
        shadow.notify_battle_finish = patched
    shadow.battle_end_notice = end_notice
    shadow._revival_activity_metrics_revision = 3

def _start_hs_activity_metrics():
    try:
        _install_hs_activity_metrics()
        print('HS_ACTIVITY_METRICS_READY 3')
    except Exception as error:
        print('HS_ACTIVITY_METRICS_WAIT %s' % error)
        try:
            import game3d
            game3d.delay_exec(1500, _start_hs_activity_metrics)
        except Exception as retry_error:
            print('HS_ACTIVITY_METRICS_RETRY_ERROR %s' % retry_error)

_start_hs_activity_metrics()
