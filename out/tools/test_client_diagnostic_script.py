"""验证客户端诊断去重限频、重复安装及保留原异常处理。"""
import logging
import sys
import types
from pathlib import Path

root = logging.getLogger()
before_handlers = list(root.handlers)
old_hook = sys.excepthook
sent, forwarded = [], []
module = types.ModuleType('gworld')
module.get_player = lambda: types.SimpleNamespace(server_proxy=types.SimpleNamespace(client_sa_log=lambda *args: sent.append(args)))
sys.modules['gworld'] = module
sys.excepthook = lambda *args: forwarded.append(args)
code = (Path(__file__).resolve().parents[2] / 'internal/game/client_diagnostic_script.py').read_text(encoding='utf-8')
try:
    namespace = {}
    exec(compile(code, 'client_diagnostic_script.py', 'exec'), namespace)
    count = len(root.handlers)
    exec(compile(code, 'client_diagnostic_script.py', 'exec'), {})
    assert len(root.handlers) == count
    handler = root.handlers[-1]
    record = logging.LogRecord('验收', logging.ERROR, 'card.py', 1, '角色异常', (), None)
    handler.emit(record)
    handler.emit(record)
    assert len(sent) == 1 and sent[0][0] == 'hs_client_error'
    try:
        raise ValueError('原生错误保留')
    except ValueError:
        sys.excepthook(*sys.exc_info())
    assert len(forwarded) == 1 and 'ValueError' in sent[-1][1]['message']
    print('客户端诊断重复安装、去重与原异常链验证通过')
finally:
    root.handlers = before_handlers
    sys.excepthook = old_hook
    if hasattr(logging, '_hs_client_diagnostic_revision'):
        del logging._hs_client_diagnostic_revision
