# -*- coding: utf-8 -*-
"""将原生Python异常和ERROR日志关联到服务器角色；保留原错误处理。"""

def _install_hs_client_diagnostic():
    import sys
    import time
    import logging
    import traceback
    import gworld
    if getattr(logging, '_hs_client_diagnostic_revision', 0) >= 1:
        return
    state = {'sending': False, 'window': 0, 'count': 0, 'seen': {}}

    def report(message):
        if state['sending']:
            return
        now = time.time()
        if now - state['window'] >= 60:
            state['window'], state['count'], state['seen'] = now, 0, {}
        message = message[:6000]
        if state['count'] >= 10 or message in state['seen']:
            return
        player = gworld.get_player()
        if player is None or getattr(player, 'server_proxy', None) is None:
            return
        state['sending'] = True
        try:
            player.server_proxy.client_sa_log('hs_client_error', {'message': message})
            state['seen'][message] = True
            state['count'] += 1
        except Exception:
            # 诊断投递失败不改变或递归调用原生错误处理。
            pass
        finally:
            state['sending'] = False

    class DiagnosticHandler(logging.Handler):
        def emit(self, record):
            try:
                report(self.format(record))
            except Exception:
                pass

    handler = DiagnosticHandler(logging.ERROR)
    logging.getLogger().addHandler(handler)
    original = sys.excepthook
    def exception_hook(kind, value, trace):
        try:
            report(''.join(traceback.format_exception(kind, value, trace)))
        except Exception:
            pass
        return original(kind, value, trace)
    sys.excepthook = exception_hook
    logging._hs_client_diagnostic_revision = 1
    print('HS_CLIENT_DIAGNOSTIC_READY 1')

_install_hs_client_diagnostic()
