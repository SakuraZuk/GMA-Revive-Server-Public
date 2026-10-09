# -*- coding: utf-8 -*-
"""Python2合同检查：隔离子类、原生RPC元数据、异常恢复及普通场景保留。

GUI与场景为最小夹具，RpcMethod来自本版原生模块；不冒充Android实机验收。
"""
from __future__ import print_function
import glob, json, os, re, sys, types

root = os.path.abspath(sys.argv[1])
sys.path.insert(0, os.path.join(root, 'internal', 'nativepvp'))
runtime = os.path.join(root, 'internal', 'nativepvp', 'runtime', 'native_engine')
sys.path.extend(glob.glob(os.path.join(runtime, 'deps', '*.whl')))
sys.path.insert(0, os.path.join(runtime, 'script'))
from battle_native_host import Host
host = Host()
host.install()
from mbengine.common.rpcdecorator import RpcMethod, CLIENT_ONLY

def module(name):
    result = types.ModuleType(name)
    result.__path__ = []
    sys.modules[name] = result
    if '.' in name:
        parent, child = name.rsplit('.', 1)
        setattr(sys.modules[parent], child, result)
    return result

gworld = module('gworld')
scene = [None]
gworld.get_battle = lambda: scene[0]
gworld.get_player = lambda: None
module('common_const').SYNC_PVP_BATTLE = 8
module('battle_logic')
client = module('battle_logic.client_battle')
module('battle_logic.controlers')
server = module('battle_logic.controlers.server')
module('guis')
module('guis.battle')
mode_module = module('guis.battle.battle_mode')
skill_module = module('guis.battle.battle_skill')
module('entities')
module('entities.components')
component = module('entities.components.battle')
delays = []
module('game3d').delay_exec = lambda *args: delays.append(args)

calls = []
def generic(self, *args, **kwds):
    calls.append((self, args))
    return 'ordinary'

source_path = os.path.join(root, 'internal', 'game', 'native_authority_bridge_script.py')
source = open(source_path, 'rb').read()
compile(source, source_path, 'exec')
class Ordinary(object):
    _revival_bridge_version = 15
    _revival_human_bridge = 2
    _revival_bridge_originals = {}
    def __init__(self): self.extra_info = {}

names = set(re.findall(r"original\('([^']+)'", source))
names.update(['start_next_round','round_end','server_round_end','delay_call_round_end',
              'server_after_pre_play','continue_old_round','wait_for_master_input',
              'on_player_input','on_master_input','do_uninterrupted_action',
              'do_master_action','client_trigger_sequence_action',
              'run_battle_storyline_end','battle_guide_end','battle_end',
              'change_auto_state','set_battle_speed','revival_set_battle_preferences'])
for name in names: setattr(Ordinary, name, generic)
class Local(Ordinary): pass
class Controller(object): pass
Controller.do_command = generic
server.controler = Controller
client.shadow_battle, client.local_battle = Ordinary, Local
class Mode(object): press_auto = generic
class Button(object):
    on_click_begin = generic
    on_click_end = generic
mode_module.battle_mode, skill_module.skill_button = Mode, Button

def native_factory(self, battle_type, dungeon_id, battle_uuid, extra_info):
    if self.fail: raise ValueError('factory fault')
    assert self.battle is None
    self.battle = client.shadow_battle()
    self.battle.id = battle_uuid
    self.battle.battle_type = battle_type
    self.battle.extra_info = extra_info

meta = RpcMethod(native_factory, CLIENT_ONLY, (), True)
def registered(*args): return meta.func(*args)
registered.rpcmethod = meta
class Component(object):
    start_server_battle_ok = staticmethod(registered)
    def __init__(self): self.battle, self.fail = None, False
component.battle = Component

base_dict = dict(Ordinary.__dict__)
originals = dict(Ordinary._revival_bridge_originals)
original_shuffle = Local.shuffle_random_speed.im_func
original_meta = (meta.rpctype, meta.argtypes, meta.pub)
scope = {}
assert "payload['generation']" in source and 'server_authority_generation' in source
# 真实exec_hotfix_data的globals/locals分离；运行期闭包必须仍能引用辅助函数。
wrapped = 'def _hs_runtime_install():\n' + '\n'.join('    '+line if line.strip() else line for line in source.rstrip().splitlines()) + '\n    return _install_revival_authority_bridge\n\n_hs_installer = _hs_runtime_install()'
exec(compile(wrapped, source_path, 'exec'), {}, scope)
assert not delays, 'installer scheduled a retry'
factory = meta.func
assert getattr(factory, '_revival_authority_factory', False)
assert client.shadow_battle is Ordinary
assert dict(Ordinary.__dict__) == base_dict
assert Ordinary._revival_bridge_originals == originals
assert Local.shuffle_random_speed.im_func is original_shuffle
assert original_meta == (meta.rpctype, meta.argtypes, meta.pub)

ordinary = Component()
registered(ordinary, 1, 101, 'ordinary', {})
assert type(ordinary.battle) is Ordinary
assert ordinary.battle._revival_bridge_version == 15
scene[0] = ordinary.battle
assert Mode().press_auto() == 'ordinary'
assert not hasattr(ordinary.battle, '_revival_user_auto_change')
assert Button().on_click_begin() == 'ordinary'
assert Button().on_click_end() == 'ordinary'

authority = Component()
registered(authority, 8, 21, 'authority', {'server_authoritative_pvp': True})
assert type(authority.battle) is factory._revival_authority_shadow
assert authority.battle._revival_bridge_version == 31
assert authority.battle._revival_server_authority
assert client.shadow_battle is Ordinary
scene[0] = authority.battle
assert Mode().press_auto() == 'ordinary'
assert authority.battle._revival_user_auto_change is False

broken = Component()
broken.fail = True
try: registered(broken, 8, 21, 'broken', {'server_authoritative_pvp': True})
except ValueError: pass
else: raise AssertionError('factory did not fail')
assert client.shadow_battle is Ordinary
scope['_hs_installer']()
assert meta.func is factory, 'repeated install wrapped the factory again'
assert dict(Ordinary.__dict__) == base_dict

# 缺少原桥时应正确排重试，不因globals/locals分离抛NameError。
client.shadow_battle = type('NotReady', (object,), {})
retry_scope = {}
exec(compile(wrapped, source_path, 'exec'), {}, retry_scope)
assert len(delays) == 1 and callable(delays[0][1])
client.shadow_battle = Ordinary
delays.pop()[1]()
assert meta.func is factory

print(json.dumps({'python2_compile': True, 'native_rpc_metadata_preserved': True,
                  'ordinary_revision15_preserved': True, 'explicit_pvp_subclass': True,
                  'factory_fault_restores_global': True, 'repeat_install_idempotent': True,
                  'recovery_generation_in_envelope': True,
                  'scope': 'contract fixture; not Android acceptance'}))
