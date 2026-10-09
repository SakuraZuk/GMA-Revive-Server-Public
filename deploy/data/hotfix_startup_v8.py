# -*- coding: utf-8 -*-
# v8 free-root startup hotfix: fully self-contained.
# Patch Hotfix execs this string inside a callback: function __globals__ is the
# http_downloader module dict, NOT this exec scope. Therefore every function
# must import locally and all shared state must ride on __builtin__.

import __builtin__ as _B

_B._HS_GAME_IP = '192.0.2.10'
_B._HS_GAME_PORT = 9000
_B._HS_PATCH_URL = 'http://192.0.2.20'
_B._HS_PUBLIC_KEY = 'ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCkKA0gT0CN7lo7ANY0lFX2PSGKb0EJDrPLwIJigtJ3UV9kztg1TGGbHuM2uZKWC/voPXWcCpf82iU+JumP5zCxxT3VMmxFJpKUhdA3326cOkM5eML5sfpAFYbPL+w45ju0PIlcdMrYndfz7Wp3hQLtc75LbKgWOR87PsNtkV+nps5WBjtnhHfNqzAWEQtql9PChoWF3RPJKS565AlW9aW7kcuJs3UsOrKbvi1IjcHvBHZXDpLBEJDsnr8eqk4MiEmK5+jqPhV0H+nRs6j6BYUVI0J3XGgTuGp6vmR5XXWsnqeirJGOy0drP7iPUdhb/BTt42fx6LJD72L0sc9jmdYJ'


def _hs_identity():
    import cache
    import uuid
    import __builtin__ as B
    account = cache.get('hs_direct_account', is_global=True)
    secret = cache.get('hs_direct_secret', is_global=True)
    if not account:
        account = 'hs_' + uuid.uuid4().hex
        cache.set('hs_direct_account', account, save=True, is_global=True)
    if not secret:
        secret = uuid.uuid4().hex + uuid.uuid4().hex
        cache.set('hs_direct_secret', secret, save=True, is_global=True)
    return account, secret


def _hs_set_server(ui):
    from guis.login import server_list_mgr
    import __builtin__ as B
    if getattr(ui, '_hs_direct_server', None) is getattr(ui, 'selected_server', None) and getattr(ui, '_hs_direct_server', None) is not None:
        ui.is_quick_login = True
        return ui.selected_server
    server = server_list_mgr.server_info()
    server.server_id = 10001
    server.server_name = u'明日生机'
    server.game_area = 'ANDIOSNETEASE'
    server.server_area = 1
    server.server_gate_list = [(B._HS_GAME_IP, B._HS_GAME_PORT)]
    server.channel = 'NETEASE'
    server.enable_guide = True
    ui.is_quick_login = True
    ui.channel_classification = 'NETEASE'
    ui.selected_server = server
    ui._hs_direct_server = server
    ui.selected_server_index = 0
    return server


def _hs_patch_modules():
    import sys
    import __builtin__ as B
    if getattr(B, '_hs_patching', False):
        return
    B._hs_patching = True
    try:
        net = sys.modules.get('client.network_mgr')
        if net is not None and hasattr(net, 'gate_client_config'):
            net.gate_client_config['loginkeycontent'] = B._HS_PUBLIC_KEY
            net.gate_client_config['zipped_channel'] = 0
            net.gate_client_config['proto'] = 'msgpack'
            cls = getattr(net, 'network_mgr', None)
            if cls is not None and not getattr(cls, '_hs_direct_ready', False):
                old_connect = cls.connect_server

                def direct_connect(self, server, username, quick_login=False, get_avatars=False):
                    import __builtin__ as B2
                    server.server_gate_list = [(B2._HS_GAME_IP, B2._HS_GAME_PORT)]
                    self.use_tcp_flag = True
                    print('HS_DIRECT 直连游戏服 %s:%s' % (B2._HS_GAME_IP, B2._HS_GAME_PORT))
                    return old_connect(self, server, username, True, get_avatars)
                cls.connect_server = direct_connect
                cls._hs_direct_ready = True

        download = sys.modules.get('patch_logic.http_downloader')
        if download is not None and hasattr(download, 'simple_get_one_file') and not getattr(download, '_hs_direct_ready', False):
            original_get = download.simple_get_one_file

            def direct_get(url, *args, **kwargs):
                import urlparse
                import __builtin__ as B2
                parsed = urlparse.urlparse(url)
                if parsed.hostname and (parsed.hostname.endswith('.update.netease.com') or parsed.hostname.endswith('.update.easebar.com')):
                    url = B2._HS_PATCH_URL + parsed.path
                    if parsed.query:
                        url += '?' + parsed.query
                if url.startswith(B2._HS_PATCH_URL + '/'):
                    callback = kwargs.get('callback', args[0] if args else None)
                    import threading

                    def fetch():
                        import httplib
                        import urlparse as up
                        import __builtin__ as B3
                        target = up.urlparse(url)
                        conn = httplib.HTTPConnection(target.hostname, target.port or 80, timeout=8)
                        data = None
                        try:
                            path = target.path + ('?' + target.query if target.query else '')
                            conn.request('GET', path)
                            response = conn.getresponse()
                            if response.status == 200:
                                data = response.read()
                            else:
                                print('HS_DIRECT 配置下载状态 %s' % response.status)
                        except Exception as error:
                            print('HS_DIRECT 配置下载失败 %s' % error)
                        finally:
                            conn.close()
                        B3._hs_http_callbacks.append((callback, data))
                    worker = threading.Thread(target=fetch)
                    worker.daemon = True
                    worker.start()
                    return None
                return original_get(url, *args, **kwargs)
            download.simple_get_one_file = direct_get
            download._hs_direct_ready = True

        listing = sys.modules.get('guis.login.server_list_mgr')
        if listing is not None:
            cls = getattr(listing, 'server_list_mgr', None)
            if cls is not None:
                cls.get_server_list_host = lambda self: B._HS_PATCH_URL

        login = sys.modules.get('guis.login.login')
        if login is not None:
            cls = getattr(login, 'login', None)
            if cls is not None and not getattr(cls, '_hs_direct_ready', False):
                original_start = cls.start_game

                def direct_start(self, *args, **kwargs):
                    _hs_set_server(self)
                    return original_start(self, *args, **kwargs)

                def direct_sdk(self, *args, **kwargs):
                    _hs_set_server(self)
                    return self.start_game()
                cls.start_game = direct_start
                cls.sdk_client_login = direct_sdk
                cls.get_quick_login_account = lambda self: _hs_identity()[0]
                cls.is_quick_login = True
                cls._hs_direct_ready = True
                print('HS_DIRECT 原登录按钮已接自建服，首次登录自动注册')

        account = sys.modules.get('entities.Account')
        if account is not None:
            cls = getattr(account, 'Account', None)
            if cls is not None and not getattr(cls, '_hs_direct_ready', False):
                original_quick = cls.quick_login

                def direct_quick(self, account_name, password):
                    account_name, secret = _hs_identity()
                    print('HS_DIRECT 提交自动注册或登录，不记录口令')
                    return original_quick(self, account_name, secret)
                cls.quick_login = direct_quick
                cls._hs_direct_ready = True

        if not getattr(B, '_hs_scheduled', False):
            engine = sys.modules.get('game3d')
            if engine is not None and hasattr(engine, 'delay_exec'):
                B._hs_scheduled = True
                engine.delay_exec(1000, _hs_tick)
    finally:
        B._hs_patching = False


def _hs_tick():
    import game3d
    import __builtin__ as B
    try:
        _hs_patch_modules()
        while B._hs_http_callbacks:
            callback, data = B._hs_http_callbacks.pop(0)
            if callback is not None:
                callback(data)
        import gui
        ui = gui.get_ui('login')
        if ui is not None and hasattr(ui, 'selected_server'):
            _hs_set_server(ui)
            if not getattr(B, '_hs_ready_logged', False):
                B._hs_ready_logged = True
                print('HS_DIRECT 登录界面已就绪')
        from mbengine.common.EntityManager import EntityManager
        import sys
        acc = sys.modules.get('entities.Account')
        avatar = sys.modules.get('entities.Avatar')
        entities = list(EntityManager._entities.items())
        alive = set(id(ent) for eid, ent in entities)
        B._hs_account_entities.intersection_update(alive)
        B._hs_avatar_entities.intersection_update(alive)
        for eid, ent in entities:
            if acc is not None and isinstance(ent, acc.Account) and id(ent) not in B._hs_account_entities:
                B._hs_account_entities.add(id(ent))
                ent.on_become_player()
                print('HS_DIRECT Account 原生登录已提交')
            if avatar is not None and isinstance(ent, avatar.Avatar) and id(ent) not in B._hs_avatar_entities:
                B._hs_avatar_entities.add(id(ent))
                ent.on_become_player()
                ent.on_login_success()
                print('HS_DIRECT Avatar 登录收尾已执行')
    except ImportError:
        pass
    except Exception:
        import traceback
        traceback.print_exc()
    game3d.delay_exec(1000, _hs_tick)


def _hs_import_hook(*args, **kwargs):
    import __builtin__ as B
    result = B._hs_import(*args, **kwargs)
    B._hs_patch_modules()
    return result


_B._hs_http_callbacks = []
_B._hs_account_entities = set()
_B._hs_avatar_entities = set()
_B._hs_patching = False
_B._hs_scheduled = False
_B._hs_ready_logged = False
_B._hs_patch_modules = _hs_patch_modules
_B._hs_tick = _hs_tick

if not getattr(_B, '_hs_direct_bootstrap', False):
    _B._hs_direct_bootstrap = True
    _B._hs_import = _B.__import__
    _B.__import__ = _hs_import_hook
    _hs_patch_modules()
    print('HS_DIRECT v8 免 root 直连引导已加载')
