# -*- coding: utf-8 -*-
# v9 free-root startup hotfix: wrap everything in _hs_install() so that every
# shared name is a CLOSURE cell, independent of the exec globals (Patch Hotfix
# execs this inside a callback whose globals is the http_downloader module).
# The import hook itself must contain no import statement (it would recurse).

def _hs_install():
    import sys
    import __builtin__ as B

    GAME_IP = '192.0.2.10'
    GAME_PORT = 9000
    PATCH_URL = 'http://192.0.2.20'
    # 渠道版本戳:服务端清单 version 与全套 npk 内容均为 1.0.128,但 APK 内置
    # script.npk 的 version.py 还是基础版 1.0.125。若不戳,客户端每次都判定
    # client_patch < server_patch -> need_update,且版本永不推进,导致
    # check_need_restart_game 恒真 -> "更新完成需重启"死循环。
    SERVER_VERSION = '1.0.128'
    # 反作弊白名单:client/cheat_check.py 导入时会把
    # patch_utils.AndroidSignatureWhiteListFromPatch extend 进
    # android_signature_sha1_whitelist(官方预留热修扩展点)。
    # 本 APK 自签名 SHA1(抓日志 c_sig),白名单格式为 59 字符无尾冒号。
    # 实机 c_sig 原始形态带尾冒号;反汇编显示比较值经 sig[0:-1] 再 [::-1](整串反转),
    # 四种形态全塞进白名单,保证真实比较值必命中。
    _SIG0 = '0C:FB:23:F4:3A:79:B2:4D:84:68:DF:64:80:99:83:BD:5A:8D:B8:71:'
    HS_SIGS = [_SIG0, _SIG0[0:-1], _SIG0[::-1], _SIG0[0:-1][::-1]]
    PUBLIC_KEY = 'ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCkKA0gT0CN7lo7ANY0lFX2PSGKb0EJDrPLwIJigtJ3UV9kztg1TGGbHuM2uZKWC/voPXWcCpf82iU+JumP5zCxxT3VMmxFJpKUhdA3326cOkM5eML5sfpAFYbPL+w45ju0PIlcdMrYndfz7Wp3hQLtc75LbKgWOR87PsNtkV+nps5WBjtnhHfNqzAWEQtql9PChoWF3RPJKS565AlW9aW7kcuJs3UsOrKbvi1IjcHvBHZXDpLBEJDsnr8eqk4MiEmK5+jqPhV0H+nRs6j6BYUVI0J3XGgTuGp6vmR5XXWsnqeirJGOy0drP7iPUdhb/BTt42fx6LJD72L0sc9jmdYJ'

    B._hs_http_callbacks = []
    B._hs_account_entities = set()
    B._hs_avatar_entities = set()
    B._hs_patching = False
    B._hs_scheduled = False

    def _hs_identity():
        import cache
        import uuid
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
        if getattr(ui, '_hs_direct_server', None) is getattr(ui, 'selected_server', None) and getattr(ui, '_hs_direct_server', None) is not None:
            ui.is_quick_login = True
            return ui.selected_server
        server = server_list_mgr.server_info()
        server.server_id = 10001
        server.server_name = '明日生机'  # 必须是 utf-8 str(源文件 utf-8);u'' 会让 C++ set_string 抛 cannot convert to std::string
        server.game_area = 'ANDIOSNETEASE'
        server.server_area = 1
        server.server_gate_list = [(GAME_IP, GAME_PORT)]
        server.channel = 'NETEASE'
        server.enable_guide = True
        # 用户要求先关自动登录(手动点击走 start_game/sdk_client_login 链路)
        ui.channel_classification = 'NETEASE'
        ui.selected_server = server
        ui._hs_direct_server = server
        ui.selected_server_index = 0
        return server

    def _hs_patch_modules():
        if B._hs_patching:
            return
        B._hs_patching = True
        try:
            # 版本戳:不能按模块名找(--实机证实 version 不在 sys.modules['version']
            # 名下,tick 也未必在跑);直接扫描 sys.modules,凡 VERSION 属性等于基础
            # 版'1.0.125'的模块一律改成服务端清单声明的 1.0.128。
            # 热修脚本在补丁解析之前执行,故此戳对 app_version_compare 必生效。
            # 反作弊签名白名单:两条路都补(模块已导入则直接追加;未导入则靠
            # 官方扩展点,cheat_check 导入时自动 extend)。
            _pu = sys.modules.get('patch_logic.patch_utils')
            if _pu is not None and '_hs_sig_ready' not in _pu.__dict__:
                _cur = _pu.__dict__.get('AndroidSignatureWhiteListFromPatch') or []
                _cur = list(_cur) if isinstance(_cur, (list, tuple)) else []
                for _s in HS_SIGS:
                    if _s not in _cur:
                        _cur.append(_s)
                _pu.__dict__['AndroidSignatureWhiteListFromPatch'] = _cur
                _pu._hs_sig_ready = True
                print('HS_DIRECT 反作弊签名白名单扩展点已注入(%d 条)' % len(_cur))
            _cc = sys.modules.get('client.cheat_check')
            if _cc is not None:
                _wl = _cc.__dict__.get('android_signature_sha1_whitelist')
                if isinstance(_wl, list):
                    for _s in HS_SIGS:
                        if _s not in _wl:
                            _wl.append(_s)
                            print('HS_DIRECT cheat_check 白名单追加 %s' % _s)
                # v17 诊断:包装 check_signature 类方法(v16 错包了模块级名字),直看判定与命中
                if '_hs_sig_wrapped' not in _cc.__dict__:
                    _cls_cc = _cc.__dict__.get('cheat_check')
                    _orig_cs = getattr(_cls_cc, 'check_signature', None) if _cls_cc is not None else None
                    if _orig_cs is not None:
                        def _hs_cs_wrap(self, *a, **k):
                            r = _orig_cs(self, *a, **k)
                            try:
                                _gl = None
                                _g3 = sys.modules.get('game3d')
                                if _g3 is not None:
                                    _gl = _g3.get_signature_info()
                                _w = _cc.__dict__.get('android_signature_sha1_whitelist') or []
                                _hits = [(s[:8], (s in _w), (s[0:-1] in _w), (s[::-1] in _w), (s[0:-1][::-1] in _w)) for s in (_gl or [])]
                                print('HS_DIRECT 签名检查结果=%s 请求签名=%r 白名单%d条 命中=%r' % (r, _gl, len(_w), _hits))
                            except Exception as _e:
                                print('HS_DIRECT 签名检查诊断异常 %s' % _e)
                            return r
                        _cls_cc.check_signature = _hs_cs_wrap
                        _cc._hs_sig_wrapped = True
                        print('HS_DIRECT check_signature 已包装诊断(类方法)')

            for _ver_nm in list(sys.modules.keys()):
                _ver_mod = sys.modules.get(_ver_nm)
                if _ver_mod is None:
                    continue
                try:
                    # 必须走 __dict__:utils.data_mgr 之类模块有自定义 __getattr__,
                    # 普通 getattr 会触发其 lazy import(实机曾炸出 No module named VERSION)。
                    _ver_val = _ver_mod.__dict__.get('VERSION')
                    if _ver_val == '1.0.125':
                        _ver_mod.__dict__['VERSION'] = SERVER_VERSION
                        print('HS_DIRECT 版本戳命中模块 %s -> %s' % (_ver_nm, SERVER_VERSION))
                except Exception:
                    continue

            net = sys.modules.get('client.network_mgr')
            if net is not None and hasattr(net, 'gate_client_config'):
                net.gate_client_config['loginkeycontent'] = PUBLIC_KEY
                net.gate_client_config['zipped_channel'] = 0
                net.gate_client_config['proto'] = 'msgpack'
                cls = getattr(net, 'network_mgr', None)
                if cls is not None and not getattr(cls, '_hs_direct_ready', False):
                    old_connect = cls.connect_server

                    def direct_connect(self, server, username, quick_login=False, get_avatars=False):
                        server.server_gate_list = [(GAME_IP, GAME_PORT)]
                        self.use_tcp_flag = True
                        print('HS_DIRECT 直连游戏服 %s:%s' % (GAME_IP, GAME_PORT))
                        import traceback as _tb
                        _tb.print_stack()
                        return old_connect(self, server, username, True, get_avatars)
                    cls.connect_server = direct_connect
                    cls._hs_direct_ready = True

            download = sys.modules.get('patch_logic.http_downloader')
            if download is not None and hasattr(download, 'simple_get_one_file') and not getattr(download, '_hs_direct_ready', False):
                original_get = download.simple_get_one_file

                def direct_get(url, *args, **kwargs):
                    import urlparse
                    import threading
                    parsed = urlparse.urlparse(url)
                    if parsed.hostname and (parsed.hostname.endswith('.update.netease.com') or parsed.hostname.endswith('.update.easebar.com')):
                        url = PATCH_URL + parsed.path
                        if parsed.query:
                            url += '?' + parsed.query
                    if url.startswith(PATCH_URL + '/'):
                        callback = kwargs.get('callback', args[0] if args else None)

                        def fetch():
                            import httplib
                            import urlparse as up
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
                            B._hs_http_callbacks.append((callback, data))
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
                    cls.get_server_list_host = lambda self: PATCH_URL

            login = sys.modules.get('guis.login.login')
            if login is not None:
                # 渠道页签真源:模块级 server_area_name_dict {显示名: 分类},页签按它建。
                # get_area_name 只是另一处(不动)。把 'NETEASE' 对应的键改成公益服。
                _d = getattr(login, '__dict__', None) or {}
                _gt = _d.get('gtext')
                _sd = _d.get('server_area_name_dict')
                if not _d.get('_hs_area_dbg'):
                    login._hs_area_dbg = True
                    _cands = [k for k in _d.keys() if not k.startswith('__')]
                    print('HS_DIRECT login模块顶层名字(%d)=%r' % (len(_cands), sorted(_cands)))
                    for _k2 in _cands:
                        try:
                            print('HS_DIRECT %s = %r' % (_k2, _d.get(_k2)))
                        except Exception:
                            print('HS_DIRECT %s <不可打印>' % _k2)
                if isinstance(_sd, dict) and not _d.get('_hs_area_ready'):
                    # 结构是 {分类: 显示名},如 'NETEASE': gtext('网易全平台')。
                    # 把 NETEASE/ALL 的显示值换成公益服。
                    _changed = False
                    for _k in ('NETEASE', 'ALL'):
                        if _k in _sd:
                            try:
                                _nv = _gt('公益服') if _gt is not None else '公益服'
                            except Exception:
                                _nv = '公益服'
                            _sd[_k] = _nv
                            _changed = True
                            print('HS_DIRECT 渠道页签 %s -> %s' % (_k, _nv))
                    if _changed:
                        login._hs_area_ready = True
                cls = getattr(login, 'login', None)
                if cls is not None and not getattr(cls, '_hs_direct_ready', False):
                    original_start = cls.start_game

                    def direct_start(self, *args, **kwargs):
                        # 手动链路入口:置手动标记(登录收尾与 sdk 入口都靠它放行)
                        B._hs_login_go = True
                        _hs_set_server(self)
                        # 诊断:谁调用了 start_game(定位残余自动登录触发源)
                        import traceback as _tb
                        print('HS_DIRECT start_game 被调用,调用栈:')
                        _tb.print_stack()
                        return original_start(self, *args, **kwargs)

                    def direct_sdk(self, *args, **kwargs):
                        # on_notice_received 公告到达后会自动触发 sdk_client_login(原版弹
                        # 网易 SDK 登录窗处)。按用户要求先关自动登录:未走手动链路直接忽略。
                        if not getattr(B, '_hs_login_go', False):
                            print('HS_DIRECT 自动登录已暂停(未手动点击),忽略 sdk_client_login')
                            return None
                        _hs_set_server(self)
                        return self.start_game()
                    cls.start_game = direct_start
                    cls.sdk_client_login = direct_sdk
                    cls.get_quick_login_account = lambda self: _hs_identity()[0]
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
                        import traceback as _tb2
                        _tb2.print_stack()
                        return original_quick(self, account_name, secret)
                    cls.quick_login = direct_quick
                    cls._hs_direct_ready = True

            if not B._hs_scheduled:
                engine = sys.modules.get('game3d')
                if engine is not None and hasattr(engine, 'delay_exec'):
                    B._hs_scheduled = True
                    print('HS_DIRECT tick 调度尝试 game3d=%s delay_exec=%s' % (engine is not None, getattr(engine, 'delay_exec', None)))
                    engine.delay_exec(1000, _hs_tick)
        finally:
            B._hs_patching = False

    def _hs_tick():
        import game3d
        try:
            _hs_patch_modules()
            # ---- v13 诊断:观察 version 值与 patch_mgr 决策(值变化时打印一次) ----
            try:
                diag_ver = sys.modules.get('patch_logic.version') or sys.modules.get('version')
                diag_cur = diag_ver.__dict__.get('VERSION') if diag_ver is not None else None
                if diag_cur != getattr(B, '_hs_ver_seen', None):
                    B._hs_ver_seen = diag_cur
                    print('HS_DIRECT 观察 version.VERSION=%s' % diag_cur)
                diag_pm = getattr(B, '_hs_pm_mod', None)
                if diag_pm is None:
                    for diag_nm in list(sys.modules.keys()):
                        _m = sys.modules.get(diag_nm)
                        if diag_nm.endswith('patch_mgr') and _m is not None and 'PATCH_MGR' in _m.__dict__:
                            B._hs_pm_mod = diag_pm = _m
                            break
                if diag_pm is not None:
                    diag_mgr = diag_pm.__dict__.get('PATCH_MGR')
                    if diag_mgr is not None:
                        diag_vals = (diag_mgr.__dict__.get('need_update'), diag_mgr.__dict__.get('no_patch'), diag_mgr.__dict__.get('patch_version_before_update'))
                        if diag_vals != getattr(B, '_hs_pm_seen', None):
                            B._hs_pm_seen = diag_vals
                            print('HS_DIRECT patch_mgr need_update=%s no_patch=%s before=%s' % diag_vals)
            except Exception:
                pass
            # ---- 诊断结束 ----
            while B._hs_http_callbacks:
                callback, data = B._hs_http_callbacks.pop(0)
                if callback is not None:
                    callback(data)
            ui = None
            try:
                import gui
                ui = gui.get_ui('login')
            except ImportError:
                pass
            if ui is not None and not getattr(B, '_hs_login_seen', False):
                B._hs_login_seen = True
                print('HS_DIRECT 登录界面已就绪(不自动选服,等待手动操作)')
            # 用户要求先不自动登录:仅当手动链路(direct_start)触发过才做登录收尾
            if getattr(B, '_hs_login_go', False):
                from mbengine.common.EntityManager import EntityManager
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
        except Exception:
            import traceback
            traceback.print_exc()
        game3d.delay_exec(1000, _hs_tick)

    orig_import = B.__import__

    def _hs_import_hook(*args, **kwargs):
        result = orig_import(*args, **kwargs)
        _hs_patch_modules()
        return result

    if not getattr(B, '_hs_direct_bootstrap', False):
        B._hs_direct_bootstrap = True
        B.__import__ = _hs_import_hook
        _hs_patch_modules()
        _g3d = sys.modules.get('game3d')
        print('HS_DIRECT v21 免 root 直连引导已加载(版本戳 %s)|game3d=%s delay_exec=%s' % (
            SERVER_VERSION, _g3d is not None, hasattr(_g3d, 'delay_exec')))


_hs_install()
