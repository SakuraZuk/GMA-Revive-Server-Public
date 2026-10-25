# -*- coding: utf-8 -*-
"""将原生Python异常和ERROR日志关联到服务器角色；保留原错误处理。"""

def _install_hs_client_diagnostic():
    import sys
    import time
    import logging
    import traceback
    import gworld
    revision = getattr(logging, '_hs_client_diagnostic_revision', 0)
    if revision >= 2:
        return
    original = sys.excepthook
    if revision == 1:
        # 只替换本项目旧诊断器；恢复其保存的原异常链，避免热升级重复上报。
        for handler in list(logging.getLogger().handlers):
            if handler.__class__.__name__ == 'DiagnosticHandler':
                logging.getLogger().removeHandler(handler)
        code = getattr(original, 'func_code', getattr(original, '__code__', None))
        cells = getattr(original, 'func_closure', getattr(original, '__closure__', None))
        if code is not None:
            for name, cell in zip(code.co_freevars, cells or ()):
                if name == 'original':
                    original = cell.cell_contents
                    break
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
                message = self.format(record)
                if 'HTTPClient - callback: err=' in message:
                    # 原生HTTPClient只打印错误枚举；handler为partial(_callback,key,request)。
                    # 只采集主机名和方法，不采集路径、查询串、请求头或正文中的身份信息。
                    frame = sys._getframe()
                    try:
                        for unused in range(16):
                            if frame is None:
                                break
                            owner = frame.f_locals.get('self')
                            args = getattr(getattr(owner, 'handler', None), 'args', ())
                            if frame.f_code.co_name == 'callback' and len(args) >= 2:
                                request = args[1]
                                host = getattr(request, 'host', '')
                                method = getattr(request, 'method', '')
                                import re
                                if re.match(r'^[a-zA-Z0-9.:-]{1,253}$', host) and method in ('GET','POST','HEAD','PUT','DELETE','OPTIONS'):
                                    message = 'HTTP目标 host=%s method=%s\n%s' % (host, method, message)
                                break
                            frame = frame.f_back
                    finally:
                        del frame
                report(message)
            except Exception:
                pass

    handler = DiagnosticHandler(logging.ERROR)
    logging.getLogger().addHandler(handler)
    def exception_hook(kind, value, trace):
        try:
            report(''.join(traceback.format_exception(kind, value, trace)))
        except Exception:
            pass
        return original(kind, value, trace)
    sys.excepthook = exception_hook
    logging._hs_client_diagnostic_revision = 2
    print('HS_CLIENT_DIAGNOSTIC_READY 2')

_install_hs_client_diagnostic()
