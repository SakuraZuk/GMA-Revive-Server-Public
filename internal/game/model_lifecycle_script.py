# -*- coding: utf-8 -*-
# 已销毁或已离开原生场景的模型，不再接收剧情速度更新。
def _hs_install_model_lifecycle_guard():
    from wrapper import model as model_module
    model_class = getattr(model_module, 'model', None)
    if model_class is None:
        return False
    if getattr(model_class, '_hs_time_scale_lifecycle_revision', 0) == 1:
        return True
    original_speed = model_class.set_animator_speed
    original_move = model_class.set_move_timescale

    def speed(self, value):
        if not self.is_valid():
            # 与原函数保持相同的期望速度缓存，模型重新加载时仍可读取。
            self.animator_speed = value
            return
        return original_speed(self, value)

    def movement(self, value):
        if not self.is_valid():
            return
        return original_move(self, value)

    model_class.set_animator_speed = speed
    model_class.set_move_timescale = movement
    model_class._hs_time_scale_lifecycle_revision = 1
    return True


def _hs_start_model_lifecycle_guard():
    try:
        if _hs_install_model_lifecycle_guard():
            print('HS_MODEL_TIME_SCALE_READY 1')
            return
    except ImportError:
        pass
    # 启动热修早于模型模块加载时，沿用引擎延迟调用。
    import game3d
    game3d.delay_exec(1500, _hs_start_model_lifecycle_guard)


_hs_start_model_lifecycle_guard()
