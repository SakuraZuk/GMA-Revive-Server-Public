# -*- coding: utf-8 -*-
# 作者新版PVP桥仅安装到专用子类；普通rev12桥与握手1保留。
def _install_revival_authority_shadow_bridge():
    import sys
    import time
    import gworld
    import common_const
    from battle_logic import client_battle
    from battle_logic.controlers import server as server_control
    from guis.battle import battle_mode, battle_skill
    shadow = client_battle.shadow_battle
    local = client_battle.local_battle
    mode = battle_mode.battle_mode
    button_type = battle_skill.skill_button
    if getattr(shadow, '_revival_bridge_version', 0) == 31:
        return

    def skill_target_hooks(process_command, skill_result, report):
        # Keep this function self-contained and Python 2 compatible for hotfix embedding.
        def process(self, user):
            previous = getattr(self, '_revival_skill_cast', None)
            context = None
            try:
                name, args = user.command
                if name in ('use_skill', 'use_support_skill', 'use_extra_support_skill',
                            'charm_use_skill', 'taunt_use_skill'):
                    self._revival_cast_sequence = getattr(self, '_revival_cast_sequence', 0) + 1
                    context = {'cast_id': self._revival_cast_sequence, 'eid': str(user.eid),
                               'skill_id': args[0], 'target_eids': [], 'target_roles': {}}
            except Exception:
                pass
            self._revival_skill_cast = context
            try:
                result = process_command(self, user)
            finally:
                self._revival_skill_cast = previous
            if context is not None and not context.get('incomplete'):
                try:
                    report(self, 'skill_targets', context)
                except Exception:
                    pass
            return result

        def resolve(self, effect_struct, effect_play, result_struct):
            context = getattr(self, '_revival_skill_cast', None)
            target_id = role = None
            try:
                # The native result is created after range/random/target confirmation.
                # Exclude counterattacks and passive skills executed inside this cast.
                if (context is not None and str(result_struct.user.eid) == context['eid']
                        and result_struct.skill is not None
                        and result_struct.skill.skill_id == context['skill_id']):
                    target_id = str(result_struct.target.eid)
                    role = int(result_struct.target.get_role_id())
            except Exception:
                if context is not None:
                    context['incomplete'] = True
            result = skill_result(self, effect_struct, effect_play, result_struct)
            if target_id is not None and target_id not in context['target_eids']:
                context['target_eids'].append(target_id)
                context['target_roles'][target_id] = role
            return result

        return process, resolve
    def rpc_from_native_command(eid, raw):
        """Turn native user.command into a lockstep RPC blob.

        Timeout auto-play stores ['use_skill', [skill_id, target_eid, coord, atk]]
        or ['move_to', [coord]]. Clicks send use_skill/move_to with a caster eid
        and an optional ck_* prefix. Peer remap needs the click shape.
        """
        if not isinstance(raw, (list, tuple)) or not raw:
            return (None, None)
        name = raw[0]
        payload = raw[1] if len(raw) > 1 else None
        caster = str(eid)
        if name in ("move_to", b"move_to"):
            coord = payload[0] if isinstance(payload, (list, tuple)) and payload else payload
            if isinstance(coord, tuple):
                coord = list(coord)
            return ("move_to", [caster, coord])
        if name in ("use_skill", b"use_skill"):
            if not isinstance(payload, (list, tuple)) or len(payload) < 2:
                return (None, None)
            skill_id = payload[0]
            target = payload[1]
            if target is None and len(payload) > 2:
                target = payload[2]
            if isinstance(target, tuple):
                target = list(target)
            elif target is not None and not isinstance(target, list):
                target = str(target)
            return ("use_skill", ["ck_monster", caster, skill_id, target])
        return (None, None)
    def native_control_command(raw):
        """Accept only original zero-argument forced/pass commands for playback.

        These consume the native action or move AP through different command_*
        handlers. Preserve the name; asking the client to think again is not an
        authoritative replay. Field commands remain with the local field driver.
        """
        if not isinstance(raw, (list, tuple)) or len(raw) != 2:
            return None
        name, args = raw
        if isinstance(name, bytes):
            try:
                name = name.decode("ascii")
            except UnicodeDecodeError:
                return None
        if (name not in ("stun", "pass_action", "idle", "no_target_idle", "no_move_idle")
                or not isinstance(args, (list, tuple)) or args):
            return None
        return [name, []]
    def native_support_command(raw):
        """Preserve the original master-input command and its two cube positions.

        The native controller resolves an ordinary click to this four-argument
        form. Support actions interrupt and resume the fighter's round; reducing
        them to a use_skill RPC loses that boundary and recomputes attack positions.
        """
        if not isinstance(raw, (list, tuple)) or len(raw) != 2:
            return None
        name, args = raw
        if isinstance(name, bytes):
            try:
                name = name.decode("ascii")
            except UnicodeDecodeError:
                return None
        if (name not in ("use_support_skill", "use_extra_support_skill")
                or not isinstance(args, (list, tuple)) or len(args) != 4):
            return None
        skill_id, target, target_coord, atk_coord = args
        if type(skill_id) not in (int, long) or skill_id <= 0:
            return None
        if target is not None and (not isinstance(target, basestring) or not target):
            return None
        for coord in (target_coord, atk_coord):
            if (not isinstance(coord, (list, tuple)) or len(coord) != 3
                    or any(type(value) not in (int, long) for value in coord) or sum(coord) != 0):
                return None
        return [name, [skill_id, target, list(target_coord), list(atk_coord)]]

    def function(method):
        return getattr(method, 'im_func', method)

    originals = getattr(shadow, '_revival_bridge_originals', {})

    def original(name, captured):
        method = function(getattr(shadow, name))
        if name in originals:
            return originals[name]
        # Revision 1 stored originals in closures; unwrap before upgrading.
        code = getattr(method, 'func_code', getattr(method, '__code__', None))
        closure = getattr(method, 'func_closure', getattr(method, '__closure__', None))
        if code is not None and closure:
            for freevar, cell in zip(code.co_freevars, closure):
                if freevar == captured:
                    return cell.cell_contents
        return method

    def ident(value):
        return str(value)

    def _revival_cube(value):
        if (not isinstance(value, (list, tuple)) or len(value) != 3
                or any(isinstance(item, bool) or not isinstance(item, (int, long))
                       for item in value) or sum(value) != 0):
            return None
        return tuple(int(item) for item in value)

    def _revival_latch_spawn_origin(battle, eid, info, replace_attrs=None):
        if (info is None or not info.get_attr('is_summon')
                or not _revival_is_sync_pvp(battle)
                or getattr(battle, '_revival_preparing', False)
                or not getattr(battle, 'scene_loaded', True)):
            return
        origins = getattr(battle, '_revival_spawn_origins', None)
        if origins is None:
            origins = battle._revival_spawn_origins = {}
        previous = origins.get(eid)
        if previous is not None and previous[0] is info:
            return
        origin = _revival_cube(info.get_attr('origin_coord'))
        # do_summon passes the summon play's creation attrs as replace_attrs.
        # Capture before native on_add consumes born_result and changes state.
        if origin is None and isinstance(replace_attrs, dict):
            origin = (_revival_cube(replace_attrs.get('origin_coord'))
                      or _revival_cube(replace_attrs.get('hex_coord')))
        if origin is None:
            origin = _revival_cube(info.get_attr('hex_coord'))
        if origin is not None:
            origins[eid] = (info, origin)

    def state(battle):
        units = []
        for eid, info in battle.entity_infos.items():
            coord = info.get_attr('hex_coord')
            if not isinstance(coord, (list, tuple)) or len(coord) != 3:
                continue
            camp = info.get_camp_id()
            native_type = getattr(info, 'type', None)
            is_field = native_type == common_const.INFO_TYPE_MAGIC_FIELD
            kind = 'field' if is_field else ('support' if info.is_support() else ('ally' if camp == 1 else ('enemy' if camp == 2 else 'neutral')))
            unit = {'eid': str(eid), 'role': int(info.get_role_id()), 'kind': kind,
                    'hex': list(coord), 'alive': not info.is_dead(),
                    'camp': int(camp), 'is_summon': bool(info.get_attr('is_summon'))}
            if native_type is not None:
                unit['native_type'] = native_type
            if is_field:
                for key in ('mf_id', 'mf_radius'):
                    value = info.get_attr(key)
                    if isinstance(value, (int, float)):
                        unit[key] = value
            creator = info.get_attr('create_entity_eid')
            if creator is not None:
                unit['create_entity_eid'] = str(creator)
            skill_id = info.get_attr('create_skill_id')
            if isinstance(skill_id, (int, long)) and not isinstance(skill_id, bool):
                unit['create_skill_id'] = int(skill_id)
            origin = _revival_cube(info.get_attr('origin_coord'))
            if origin is None:
                saved = getattr(battle, '_revival_spawn_origins', {}).get(eid)
                if saved is not None and saved[0] is info:
                    origin = saved[1]
            if origin is not None:
                unit['origin_coord'] = list(origin)
            for key in ('hp', 'max_hp', 'ap'):
                value = info.get_attr(key)
                if isinstance(value, (int, float)):
                    unit[key] = value
            units.append(unit)
        return {'units': units}

    def emit(battle, kind, data):
        battle_id = ident(battle.id)
        if getattr(battle, '_revival_event_uuid', None) != battle_id:
            battle._revival_event_uuid = battle_id
            battle._revival_event_sequence = 0
            battle._revival_event_begin = time.time()
        sequence = battle._revival_event_sequence + 1
        payload = {'battle_uuid': battle_id, 'sequence': sequence,
                   't': max(0.0, time.time() - battle._revival_event_begin), 'kind': kind, 'data': data}
        payload['generation'] = int((getattr(battle, 'extra_info', {}) or {}).get('server_authority_generation') or 0)
        gworld.get_player().server_proxy.do_command('__battle_event__', [payload])
        battle._revival_event_sequence = sequence

    def report(battle, kind, data=None, include_state=False):
        # Recording must never interrupt a native play/round callback.
        try:
            # Skill composition has already created logical summon infos;
            # do not wait for their appearance animation or the next turn.
            if (kind == 'skill_targets' and _revival_is_sync_pvp(battle)
                    and getattr(battle, '_revival_ready', False)
                    and not getattr(battle, '_revival_preparing', False)):
                include_state = True
            payload = state(battle) if include_state else {}
            payload.update(data or {})
            emit(battle, kind, payload)
            return True
        except Exception as error:
            print('REVIVAL_BATTLE_REPORT_ERROR %s: %s' % (kind, error))
            return False

    native = {}
    for name in ('start_next_round', 'round_end', 'server_round_end', 'delay_call_round_end',
                 'server_after_pre_play', 'continue_old_round', 'wait_for_master_input',
                 'on_player_input', 'on_master_input', 'do_uninterrupted_action',
                 'do_master_action', 'client_trigger_sequence_action',
                 'run_battle_storyline_end', 'battle_guide_end', 'battle_end',
                 'change_auto_state'):
        native[name] = function(getattr(local, name))
    old_prepare = original('prepare', 'old_prepare')
    old_check_auto_fighting = original('check_auto_battle_fighting', 'old_check_auto_fighting')
    old_fighting = original('battle_fighting', 'old_fighting')
    old_wait = original('wait_for_player_input', 'old_wait')
    old_need_input = original('need_input', 'old_need_input')
    old_shuffle = original('shuffle_random_speed', 'old_shuffle')
    old_log_command = original('log_command', 'old_log_command')
    old_process_command = original('process_command', 'old_process_command')
    old_skill_result = original('skill_result', 'old_skill_result')
    old_start_action = original('real_start_action', 'old_start_action')
    old_trigger_end = original('trigger_battle_end', 'old_trigger_end')
    old_dispatch_plays = original('dispatch_plays', 'old_dispatch_plays')
    old_end_notice = original('battle_end_notice', 'old_end_notice')
    old_on_add_entity = original('on_add_entity', 'old_on_add_entity')
    old_ui_command = original('do_command', 'old_ui_command')
    old_click_begin = function(getattr(button_type, '_revival_click_begin_original',
                                      button_type.on_click_begin))
    old_click_end = function(getattr(button_type, '_revival_click_end_original',
                                    button_type.on_click_end))
    old_press_auto = function(getattr(mode, '_revival_press_auto_original',
                                     mode.press_auto))
    validate_command = function(server_control.controler.do_command)

    def prepare(self, *args, **kwargs):
        # Native preparation can update the mode UI before the handshake.
        self._revival_input_grant = None
        self._revival_spawn_origins = {}
        self._revival_ready = False
        self._revival_started = False
        self._revival_preparing = True
        self._revival_deferred_auto_fighting = False
        # Scenes reuse the battle object. Old EIDs can be allocated again;
        # never play a previous match's queued RPC in the new match.
        self._revival_cmd_queue = []
        self._revival_queue_flushing = False
        self._revival_rpc_command = False
        self._revival_pvp_failed = False
        try:
            result = old_prepare(self, *args, **kwargs)
        except:
            self._revival_deferred_auto_fighting = False
            raise
        finally:
            self._revival_preparing = False
        self._revival_result_sent = False
        self._revival_finished_tasks = None
        self._revival_end_scheduled = False
        self._revival_play_tokens = set()
        self._revival_wait_end = None
        self._revival_ready = report(self, 'ready', {'version': 1, 'bridge_revision': 31})
        deferred = self._revival_deferred_auto_fighting
        self._revival_deferred_auto_fighting = False
        if deferred and self._revival_ready:
            old_check_auto_fighting(self)
        return result

    def check_auto_battle_fighting(self):
        # Reusing a scene can finish its load inside native prepare. Defer
        # the request until ready is sent, before Avatar sets battle_fight_flag.
        if getattr(self, '_revival_preparing', False):
            self._revival_deferred_auto_fighting = True
            return
        return old_check_auto_fighting(self)

    def change_auto_state(self, state):
        was_auto = (_revival_is_sync_pvp(self)
            and gworld.get_player().eid in self.auto_battle_set)
        result = native['change_auto_state'](self, state)
        # Native auto-off only changes flags. Reopen a parked own window only
        # if the authority has already granted this exact local continuation.
        if (was_auto and gworld.get_player().eid not in self.auto_battle_set
                and not getattr(self, '_revival_pvp_failed', False)
                and getattr(self, '_revival_ready', False)
                and getattr(self, '_revival_started', False)
                and not getattr(self, '_revival_preparing', False)
                and not getattr(self, '_revival_result_sent', False)
                and getattr(self, 'scene_loaded', True)
                and self.bid is not None
                and getattr(self, '_revival_event_uuid', None) == ident(self.id)
                and self.winner_eid_list is None and gworld.get_battle() is self):
            info = self.entity_infos.get(getattr(self, 'current_input_eid', None))
            if (_revival_has_input_grant(self, info) and self.master_is_player(info)
                    and old_need_input(self, info)):
                wait_for_input(self, info)
        # Report the accepted native state, including unlock checks, only for
        # the prepared current battle. Preference restoration stays silent.
        try:
            caller = sys._getframe(1).f_code
            # Guess-PVP's native confirmation callback runs after press_auto
            # returns. Cancelling is still an explicit user choice.
            user_change = (getattr(self, '_revival_user_auto_change', False)
                or (caller.co_name == 'cancel_change' and caller.co_filename.replace('/', '\\')
                    .endswith('guis\\battle\\battle_mode.py')))
            if (getattr(self, '_revival_ready', False)
                    and user_change
                    and not getattr(self, '_revival_restoring_preferences', False)
                    and not getattr(self, '_revival_result_sent', False)
                    and self.bid is not None and gworld.get_battle() is self
                    and getattr(self, '_revival_event_uuid', None) == ident(self.id)):
                report(self, 'settings', {
                    'auto_battle': gworld.get_player().eid in self.auto_battle_set})
        except Exception as error:
            print('REVIVAL_BATTLE_REPORT_ERROR settings: %s' % error)
        return result

    def set_battle_preferences(self, speed, automatic):
        restoring = getattr(self, '_revival_restoring_preferences', False)
        self._revival_restoring_preferences = True
        try:
            self.set_battle_speed(speed)
            return native['change_auto_state'](self, automatic)
        finally:
            self._revival_restoring_preferences = restoring

    def press_auto(self, *args, **kwargs):
        # Persist explicit button choices. Native battle rules also change
        # automatic mode, but those temporary changes must stay battle-local.
        battle = gworld.get_battle()
        if battle is None:
            return old_press_auto(self, *args, **kwargs)
        user_change = getattr(battle, '_revival_user_auto_change', False)
        battle._revival_user_auto_change = True
        try:
            return old_press_auto(self, *args, **kwargs)
        finally:
            battle._revival_user_auto_change = user_change

    def _revival_input_ui_probe(battle, phase, button=None, command=None, args=None,
                               synthetic=False, has_touch=None):
        # Observation only: never repair a gate or interrupt its native callback.
        try:
            current_battle = gworld.get_battle()
            if (battle is None or battle is not current_battle
                    or not _revival_is_sync_pvp(battle)
                    or not getattr(battle, '_revival_ready', False)
                    or not getattr(battle, '_revival_started', False)
                    or getattr(battle, '_revival_preparing', False)
                    or getattr(battle, '_revival_result_sent', False)
                    or getattr(battle, '_revival_pvp_failed', False)
                    or getattr(battle, 'winner_eid_list', None) is not None
                    or getattr(battle, 'bid', None) is None
                    or getattr(battle, '_revival_event_uuid', None) != ident(battle.id)):
                return
            import gui
            panel = gui.battle_skill
            def scalar(read):
                try:
                    value = read()
                    if value is None or isinstance(value, (bool, int, long, str)):
                        return value
                except Exception:
                    pass
                return None
            def skill_id(item):
                return scalar(lambda: getattr(getattr(item, 'skill', None), 'skill_id', None))
            picked = getattr(panel, 'selected_button', None)
            last_pick = getattr(panel, 'last_selected_button', None)
            if button is None:
                button = picked
            payload = {
                'phase': phase,
                'current_eid': scalar(lambda: getattr(battle, 'current_input_eid', None)),
                'active_eid': scalar(lambda: getattr(panel, 'active_eid', None)),
                'auto': scalar(lambda: gworld.get_player().eid in battle.auto_battle_set),
                'anim': scalar(lambda: getattr(panel, 'skill_anim_playing', None)),
                'ready': scalar(lambda: getattr(button, 'ready', None)),
                'avail': scalar(lambda: bool(panel.avail_button(button))),
                'active': scalar(lambda: getattr(button, 'active', None)),
                'skill_id': skill_id(button),
                'picked': skill_id(picked),
                'last_pick': skill_id(last_pick),
                'synthetic': synthetic,
                'has_touch': has_touch,
            }
            if command is not None:
                payload['command'] = scalar(lambda: command)
                raw = list(args or [])
                if raw and raw[0] in ('ck_monster', 'ck_skill', 'ck_head'):
                    raw = raw[1:]
                if command == 'use_skill' and len(raw) > 1:
                    payload['skill_id'] = scalar(lambda: raw[1])
            report(battle, 'snapshot', {'input_ui': payload}, include_state=True)
        except Exception:
            # Diagnostic failures cannot replace a native result or exception.
            pass

    def _revival_input_ui_battle():
        try:
            return gworld.get_battle()
        except Exception:
            return None

    def _revival_input_ui_click(args):
        synthetic = False
        try:
            import sys
            # Native GUI auto-selection calls the same begin/end callbacks.
            synthetic = sys._getframe(2).f_code.co_name == 'select_first_skill'
        except Exception:
            pass
        return synthetic, bool(args and args[0] is not None)

    def click_begin(self, *args, **kwargs):
        battle = _revival_input_ui_battle()
        synthetic, has_touch = _revival_input_ui_click(args)
        _revival_input_ui_probe(battle, 'begin0', self,
                               synthetic=synthetic, has_touch=has_touch)
        try:
            return old_click_begin(self, *args, **kwargs)
        finally:
            _revival_input_ui_probe(battle, 'begin1', self,
                                   synthetic=synthetic, has_touch=has_touch)

    def click_end(self, *args, **kwargs):
        battle = _revival_input_ui_battle()
        synthetic, has_touch = _revival_input_ui_click(args)
        _revival_input_ui_probe(battle, 'end0', self,
                               synthetic=synthetic, has_touch=has_touch)
        try:
            return old_click_end(self, *args, **kwargs)
        finally:
            _revival_input_ui_probe(battle, 'end1', self,
                                   synthetic=synthetic, has_touch=has_touch)

    def do_ui_command(self, command, *args, **kwargs):
        _revival_input_ui_probe(self, 'rpc', command=command, args=args)
        if _revival_is_sync_pvp(self) and command in ('move_to', 'use_skill'):
            info = self.entity_infos.get(getattr(self, 'current_input_eid', None))
            # A deferred native GUI callback can restore ready/selection after
            # its animation was closed. Validate the window again at RPC entry.
            if (gworld.get_battle() is not self
                    or not getattr(self, '_revival_ready', False)
                    or not getattr(self, '_revival_started', False)
                    or getattr(self, '_revival_preparing', False)
                    or getattr(self, '_revival_result_sent', False)
                    or getattr(self, '_revival_pvp_failed', False)
                    or not getattr(self, 'scene_loaded', True)
                    or self.bid is None
                    or getattr(self, '_revival_event_uuid', None) != ident(self.id)
                    or self.winner_eid_list is not None
                    or not _revival_has_input_grant(self, info)
                    or not self.master_is_player(info)):
                return
        return old_ui_command(self, command, *args, **kwargs)

    def fighting(self, *args, **kwargs):
        result = old_fighting(self, *args, **kwargs)
        if not self._revival_started:
            self._revival_started = True
            report(self, 'started', include_state=True)
        return result

    def after_pre_play(self, user, preplay_duration, play_delay):
        report(self, 'turn', {'eid': str(user.eid)}, include_state=True)
        return native['server_after_pre_play'](self, user, preplay_duration, play_delay)

    def on_add_entity(self, eid, *args, **kwargs):
        replace_attrs = kwargs.get('replace_attrs', args[0] if args else None)
        _revival_latch_spawn_origin(self, eid, self.entity_infos.get(eid), replace_attrs)
        result = old_on_add_entity(self, eid, *args, **kwargs)
        # on_add_entity runs during native appearance playback. Preserve its
        # callback and result, then publish all logical entities atomically.
        # Initial appearance runs inside fighting after ready but before started.
        # The started report includes that full roster; only later appearances
        # can publish observations without breaking the handshake sequence.
        if (_revival_is_sync_pvp(self) and getattr(self, '_revival_ready', False)
                and getattr(self, '_revival_started', False)
                and not getattr(self, '_revival_preparing', False)
                and getattr(self, 'scene_loaded', True)
                and not getattr(self, '_revival_result_sent', False)
                and self.bid is not None and eid in self.entity_infos):
            report(self, 'snapshot', {'spawn_eid': str(eid)}, include_state=True)
        return result

    def _revival_is_sync_pvp(battle):
        return bool(getattr(battle, '_revival_server_authority', False) and
                    (getattr(battle, 'extra_info', {}) or {}).get('server_authoritative_pvp') is True)

    def _revival_unit_lockstep_key(entity):
        master = ''
        role = 0
        mf = 0
        try:
            value = entity.get_master()
            if value is None:
                value = entity.get_attr('master')
            if value is not None:
                master = str(value)
        except Exception:
            master = ''
        try:
            role = int(entity.get_role_id() or 0)
        except Exception:
            role = 0
        try:
            mf = int(entity.get_attr('mf_id') or 0)
        except Exception:
            mf = 0
        return (master, role, mf)

    def shuffle_random_speed(self):
        # Native walks iter_alive_entity_info sorted by eid. Dual engines
        # mint different eids for the same card, so the shared seed assigns
        # random_speed_rate to different units and 4401/4410 (both 110)
        # swap order. Consume RNG by (master, role) which both engines share.
        if not _revival_is_sync_pvp(self):
            return old_shuffle(self)
        try:
            rows = list(self.iter_alive_entity_info())
        except Exception as error:
            print('REVIVAL_PVP_SHUFFLE_ERROR %s' % error)
            return old_shuffle(self)
        keyed = []
        for eid, entity in rows:
            keyed.append((_revival_unit_lockstep_key(entity), eid, entity))
        keyed.sort(key=lambda item: item[0])
        print('REVIVAL_PVP_SHUFFLE n=%s order=%s' % (
            len(keyed), [(item[0][1], str(item[1])) for item in keyed]))
        for _key, _eid, entity in keyed:
            entity.random_speed()
            entity.attrs['init_avatar'] = True

    def need_input(self, info):
        # Official: pass_action False, other-control True, auto_battle_set
        # False, else info.need_input(). Auto False hides skill select.
        # HOLD of local think lives in real_start_action, not here.
        return old_need_input(self, info)

    def _revival_command_caster(command, args):
        args = list(args or [])
        if args and args[0] in ('ck_monster', 'ck_skill', 'ck_head'):
            args = args[1:]
        if args:
            return args[0]
        return None

    def _revival_pvp_fail(self, code, message, caster=None):
        if getattr(self, '_revival_pvp_failed', False):
            return False
        self._revival_pvp_failed = True
        try:
            self.clear_master_input_timer()
        except Exception:
            pass
        print('REVIVAL_PVP_BRIDGE_ERROR %s: %s' % (code, message))
        # The server aborts this match explicitly. Keeping the queue intact
        # makes the lost boundary diagnosable instead of dropping an action.
        report(self, 'error', {'code': code, 'message': message,
            'queued': len(getattr(self, '_revival_cmd_queue', None) or []),
            'current_eid': str(getattr(self, 'current_input_eid', None)),
            'caster_eid': None if caster is None else str(caster)})
        return False

    def _revival_queue_put(self, item):
        queue = getattr(self, '_revival_cmd_queue', None)
        if queue is None:
            queue = []
            self._revival_cmd_queue = queue
        if len(queue) >= 16:
            return _revival_pvp_fail(self, 'pvp_bridge_queue_overflow',
                'authoritative actions exceeded the client playback queue', item[0])
        queue.append(item)
        print('REVIVAL_PVP_QUEUE n=%s caster=%s kind=%s current=%s' % (
            len(queue), item[0], item[1],
            getattr(self, 'current_input_eid', None)))
        # A parked boundary can already match the oldest action. Always append
        # the new arrival first, then consume the head rather than that arrival.
        current = getattr(self, 'current_input_eid', None)
        if (_revival_is_sync_pvp(self) and current is not None
                and str(queue[0][0]) == str(current)
                and not getattr(self, '_revival_rpc_command', False)
                and not getattr(self, '_revival_queue_flushing', False)):
            info = getattr(self, 'entity_infos', {}).get(current)
            if info is not None:
                start_action(self, info, 0, 0)
        return True

    def _revival_queue_take(self, eid):
        queue = getattr(self, '_revival_cmd_queue', None) or []
        if not queue:
            return None
        # The authoritative sequence is global, including master support
        # actions. A later matching actor must never jump an older queue entry.
        if str(queue[0][0]) != str(eid):
            return None
        return queue.pop(0)

    def _revival_has_input_grant(self, info):
        grant = getattr(self, '_revival_input_grant', None)
        return (info is not None and grant is not None
                and grant[0] == ident(self.id) and grant[1] is info
                and grant[2] == getattr(self, 'action_counter', None))

    def wait_for_input(self, info):
        if _revival_is_sync_pvp(self) and not _revival_has_input_grant(self, info):
            # A faster client can reach the next local turn while its peer is
            # still playing. Park without opening controls until restore_input
            # grants the authority's real awaiting_player window.
            # A master/support action disables its caster, so the old fighter
            # controls can survive that animation. Close those controls too.
            self.allow_player_input(info, False)
            self.current_input_eid = info.eid
            self.clear_master_input_timer()
            report(self, 'input', {'eid': str(info.eid)}, include_state=True)
            return
        result = old_wait(self, info)
        # Server shadow is the only clock. Clear every local input timer,
        # including own units and the camp-3 field.
        try:
            if _revival_is_sync_pvp(self) and info is not None:
                self.clear_master_input_timer()
                print('REVIVAL_PVP_HOLD_INPUT eid=%s master=%s' % (
                    info.eid, info.get_master()))
        except Exception as error:
            print('REVIVAL_PVP_HOLD_INPUT_ERROR %s' % error)
        report(self, 'input', {'eid': str(info.eid)}, include_state=True)
        return result

    def on_player_input(self, eid):
        if not _revival_is_sync_pvp(self):
            return native['on_player_input'](self, eid)
        if not getattr(self, '_revival_rpc_command', False):
            print('REVIVAL_PVP_SKIP_LOCAL_INPUT eid=%s' % eid)
            return
        print('REVIVAL_PVP_NATIVE_INPUT eid=%s rpc=1' % eid)
        return native['on_player_input'](self, eid)

    def wait_for_master_input(self, master):
        if not _revival_is_sync_pvp(self):
            return native['wait_for_master_input'](self, master)
        if self.master_input_timer:
            self.clear_master_input_timer()

    def log_command(self, user):
        if _revival_is_sync_pvp(self):
            self._revival_input_grant = None
        result = old_log_command(self, user)
        context = getattr(self, '_revival_skill_cast', None)
        report(self, 'command', {'eid': str(user.eid), 'command': user.command,
                                'cast_id': context['cast_id'] if context else None})
        return result

    def round_end(self, eid):
        report(self, 'round_end', {'eid': None if eid is None else str(eid)}, include_state=True)
        return native['round_end'](self, eid)

    def start_action(self, info, delay, play_delay):
        # A scripted victory can arrive while after_pre_play is processing
        # events. Its saved continuation must finish instead of opening a turn.
        if self.winner_eid_list is not None:
            return self.trigger_battle_end(max(0, delay + play_delay), info)
        if not _revival_is_sync_pvp(self):
            return old_start_action(self, info, delay, play_delay)
        # Even the same EID can begin another turn. A queued restore grants
        # after this reset; an earlier window's grant cannot activate this one.
        self._revival_input_grant = None
        if getattr(self, '_revival_pvp_failed', False):
            return
        if getattr(self, '_revival_rpc_command', False):
            return old_start_action(self, info, delay, play_delay)
        try:
            if info is not None and info.is_dead():
                return old_start_action(self, info, delay, play_delay)
        except Exception:
            pass
        queued = None
        try:
            if info is not None:
                queued = _revival_queue_take(self, info.eid)
        except Exception as error:
            print('REVIVAL_PVP_QUEUE_TAKE_ERROR %s' % error)
            queued = None
        if queued is not None:
            print('REVIVAL_PVP_FLUSH caster=%s kind=%s' % (queued[0], queued[1]))
            try:
                self.current_input_eid = info.eid
            except Exception:
                pass
            kind = queued[1]
            flushing = getattr(self, '_revival_queue_flushing', False)
            self._revival_queue_flushing = True
            try:
                if kind == 'peer':
                    return do_peer_command(self, queued[4], queued[2], queued[3])
                if kind == 'continue':
                    return continue_input(self, queued[0])
                if kind == 'restore':
                    result = restore_input(self, queued[0], queued[2])
                elif kind == 'native':
                    return play_native_command(self, queued[0], queued[2])
                elif kind == 'support':
                    return play_support_command(self, queued[0], queued[2], queued[3])
                else:
                    return do_command(self, queued[2], queued[3])
            finally:
                self._revival_queue_flushing = flushing
            # Restore only reopens a window. It has no animation callback that
            # could consume another queued action in that same window.
            queue = getattr(self, '_revival_cmd_queue', None) or []
            current = getattr(self, 'current_input_eid', None)
            if queue and current is not None and str(queue[0][0]) == str(current):
                start_action(self, info, 0, 0)
            return result
        wants_input = True
        try:
            wants_input = old_need_input(self, info)
        except Exception as error:
            print('REVIVAL_PVP_NEED_INPUT_ERROR %s' % error)
            wants_input = True
        is_player = False
        try:
            is_player = self.master_is_player(info)
        except Exception:
            is_player = False
        is_field = False
        try:
            is_field = getattr(info, 'type', None) == common_const.INFO_TYPE_MAGIC_FIELD
        except Exception:
            is_field = False
        if wants_input and is_player:
            # Own manual unit: skill select, no local timer. Click RPC echoes.
            try:
                info.clear_command()
            except Exception:
                pass
            return wait_for_input(self, info)
        if not is_field:
            # Own auto and every enemy card: park eid so the RPC matches
            # official do_command. No allow_player_input (no skill-select UI).
            # Magic field still thinks locally; server does not sync field idle.
            print('REVIVAL_PVP_HOLD_AUTO eid=%s player=%s' % (
                getattr(info, 'eid', None), int(bool(is_player))))
            try:
                self.current_input_eid = info.eid
            except Exception:
                pass
            return
        return old_start_action(self, info, delay, play_delay)

    def trigger_end(self, delay, info):
        if self.winner_eid_list is None:
            return old_trigger_end(self, delay, info)
        if getattr(self, '_revival_end_scheduled', False):
            return
        self._revival_end_scheduled = True
        def finish():
            try:
                return old_trigger_end(self, delay, info)
            except Exception:
                self._revival_end_scheduled = False
                raise
        if getattr(self, '_revival_play_tokens', None):
            self._revival_wait_end = finish
            return
        return finish()

    def dispatch_plays(self, plays):
        # The renderer also waits p.delay; calc_plays_time excludes that delay.
        # Capture it before do_play can consume/reset a play's delay field.
        play_duration = sum(max(0, p.get_action_time() + p.delay) for p in plays)
        duration = old_dispatch_plays(self, plays)
        if play_duration > 0:
            tokens = getattr(self, '_revival_play_tokens', None)
            if tokens is None:
                tokens = self._revival_play_tokens = set()
            token = object()
            tokens.add(token)
            def played():
                # A callback left over from a prior battle must not finish a new one.
                if getattr(self, '_revival_play_tokens', None) is not tokens:
                    return
                tokens.discard(token)
                if not tokens:
                    finish = getattr(self, '_revival_wait_end', None)
                    self._revival_wait_end = None
                    if finish is not None and self.bid is not None:
                        finish()
            self.add_callback(play_duration, played)
        return duration

    def end_notice(self, winners, action_counter):
        result = old_end_notice(self, winners, action_counter)
        battle_id = self.id
        tokens = getattr(self, '_revival_play_tokens', None)
        if tokens is None:
            tokens = self._revival_play_tokens = set()
        print('REVIVAL_BATTLE_END_NOTICE %s action=%s plays=%s' %
              (ident(battle_id), action_counter, len(tokens or ())))
        def finish_notice():
            if (self.id != battle_id or self.bid is None
                    or getattr(self, '_revival_play_tokens', None) is not tokens
                    or getattr(self, '_revival_result_sent', False)):
                return
            if self.winner_eid_list is not None:
                infos = self.action_list
                self.trigger_battle_end(0, infos[0] if infos else None)
        # Victory may be calculated inside compose_play, before dispatch_plays.
        # Let that calculation finish, then wait for every native play duration.
        self.add_callback(0, finish_notice)
        return result

    def _revival_tuple_cubes(args):
        args = list(args or [])
        for i, item in enumerate(args):
            if isinstance(item, list) and len(item) == 3:
                args[i] = tuple(item)
        return args

    def do_command(self, command, args):
        # RPC arrays decode as lists; the native hex map uses tuple keys.
        # Both move_to and cell-target use_skill carry the coordinate last,
        # including use_skill requests prefixed with a ck_* counter tag.
        # Mark RPC playback so log_command does not emit a second timeout_command.
        if _revival_is_sync_pvp(self) and getattr(self, '_revival_pvp_failed', False):
            return
        args = _revival_tuple_cubes(args)
        caster = _revival_command_caster(command, args)
        current = getattr(self, 'current_input_eid', None)
        # Official controler.do_command: user = entity_infos.get(current);
        # if not user: return. After a command, allow_player_input(False)
        # clears current. The next RPC can arrive in that gap, before
        # start_action parks the next eid. Queue it (current is None or
        # caster mismatch) and flush when that unit starts.
        queued = (_revival_is_sync_pvp(self) and bool(getattr(self, '_revival_cmd_queue', None))
                  and not getattr(self, '_revival_queue_flushing', False))
        if caster is not None and (queued or current is None or str(caster) != str(current)):
            _revival_queue_put(self, (caster, 'self', command, args, None))
            return
        self._revival_rpc_command = True
        try:
            return validate_command(self, gworld.get_player().eid, command, *args)
        finally:
            self._revival_rpc_command = False

    def do_peer_command(self, master, command, args):
        # Opponent lockstep: the acting avatar is master, not get_player().
        if _revival_is_sync_pvp(self) and getattr(self, '_revival_pvp_failed', False):
            return
        args = _revival_tuple_cubes(args)
        caster = _revival_command_caster(command, args)
        current = getattr(self, 'current_input_eid', None)
        queued = (_revival_is_sync_pvp(self) and bool(getattr(self, '_revival_cmd_queue', None))
                  and not getattr(self, '_revival_queue_flushing', False))
        if caster is not None and (queued or current is None or str(caster) != str(current)):
            _revival_queue_put(self, (caster, 'peer', command, args, master))
            return
        self._revival_rpc_command = True
        try:
            return validate_command(self, master, command, *args)
        finally:
            self._revival_rpc_command = False

    def continue_input(self, eid=None):
        # Field ticks and server-generated continues. Local auto/timeout never
        # reach on_player_input unless this RPC (or do_command) set the flag.
        target = self.current_input_eid if eid is None else eid
        if _revival_is_sync_pvp(self):
            if getattr(self, '_revival_pvp_failed', False):
                return
            if target is None:
                return _revival_pvp_fail(self, 'pvp_bridge_missing_caster',
                    'authoritative continue has no acting entity')
            current = getattr(self, 'current_input_eid', None)
            # Native driver.on_player_input acts directly on entity_infos[eid].
            # Unlike do_command it does not check the current input boundary.
            # Idle/timeout continues must therefore wait through animations too.
            queued = (bool(getattr(self, '_revival_cmd_queue', None))
                      and not getattr(self, '_revival_queue_flushing', False))
            if queued or current is None or str(target) != str(current):
                return _revival_queue_put(self, (target, 'continue', None, None, None))
        self._revival_rpc_command = True
        try:
            print('REVIVAL_PVP_CONTINUE eid=%s' % target)
            return native['on_player_input'](self, target)
        finally:
            self._revival_rpc_command = False

    def play_native_command(self, eid, raw):
        # The authoritative engine already selected this zero-argument control
        # command. Keep its native AP handler and do not recalculate AI/input.
        if (not _revival_is_sync_pvp(self)
                or getattr(self, '_revival_pvp_failed', False)
                or getattr(self, 'bid', None) is None
                or not getattr(self, '_revival_ready', False)
                or not getattr(self, '_revival_started', False)
                or getattr(self, '_revival_preparing', False)
                or getattr(self, '_revival_result_sent', False)
                or not getattr(self, 'scene_loaded', True)
                or getattr(self, 'winner_eid_list', None) is not None
                or getattr(self, '_revival_event_uuid', None) != ident(self.id)):
            return
        try:
            if gworld.get_battle() is not self:
                return
        except Exception:
            return
        command = native_control_command(raw)
        if command is None:
            return _revival_pvp_fail(self, 'pvp_bridge_invalid_native_command',
                'unsupported authoritative control command', eid)
        if eid is None:
            return _revival_pvp_fail(self, 'pvp_bridge_missing_caster',
                'authoritative control command has no acting entity')
        current = getattr(self, 'current_input_eid', None)
        queued = (bool(getattr(self, '_revival_cmd_queue', None))
                  and not getattr(self, '_revival_queue_flushing', False))
        if queued or current is None or str(eid) != str(current):
            return _revival_queue_put(self, (eid, 'native', command, None, None))
        info = self.entity_infos.get(eid)
        if info is None:
            return _revival_pvp_fail(self, 'pvp_bridge_missing_entity',
                'authoritative control entity is absent from the client', eid)
        previous_rpc = getattr(self, '_revival_rpc_command', False)
        self._revival_rpc_command = True
        try:
            info.set_command(command[0], *command[1])
            return native['on_player_input'](self, eid)
        finally:
            self._revival_rpc_command = previous_rpc

    def play_support_command(self, round_eid, caster_eid, raw):
        # Support is a master action inside the fighter's input window. Its
        # caster never takes an ordinary turn, so wait for the fighter instead.
        if (not _revival_is_sync_pvp(self)
                or getattr(self, '_revival_pvp_failed', False)
                or getattr(self, 'bid', None) is None
                or not getattr(self, '_revival_ready', False)
                or not getattr(self, '_revival_started', False)
                or getattr(self, '_revival_preparing', False)
                or getattr(self, '_revival_result_sent', False)
                or not getattr(self, 'scene_loaded', True)
                or getattr(self, 'winner_eid_list', None) is not None
                or getattr(self, '_revival_event_uuid', None) != ident(self.id)):
            return
        try:
            if gworld.get_battle() is not self:
                return
        except Exception:
            return
        command = native_support_command(raw)
        if command is None:
            return _revival_pvp_fail(self, 'pvp_bridge_invalid_support_command',
                'unsupported authoritative support command', caster_eid)
        if round_eid is None or caster_eid is None:
            return _revival_pvp_fail(self, 'pvp_bridge_missing_caster',
                'authoritative support requires fighter and caster', caster_eid)
        current = getattr(self, 'current_input_eid', None)
        queued = (bool(getattr(self, '_revival_cmd_queue', None))
                  and not getattr(self, '_revival_queue_flushing', False))
        if queued or current is None or str(round_eid) != str(current):
            return _revival_queue_put(self, (round_eid, 'support', caster_eid, command, None))
        fighter = self.entity_infos.get(round_eid)
        caster = self.entity_infos.get(caster_eid)
        if fighter is None or caster is None:
            return _revival_pvp_fail(self, 'pvp_bridge_missing_entity',
                'authoritative support fighter or caster is absent', caster_eid)
        if (fighter.get_master() != caster.get_master()
                or str(getattr(self, 'round_eid', round_eid)) != str(round_eid)):
            return _revival_pvp_fail(self, 'pvp_bridge_support_window_mismatch',
                'support caster and fighter do not share the active round', caster_eid)
        previous_rpc = getattr(self, '_revival_rpc_command', False)
        self._revival_rpc_command = True
        try:
            args = _revival_tuple_cubes(command[1])
            if not caster.set_command(command[0], *args):
                return _revival_pvp_fail(self, 'pvp_bridge_support_rejected',
                    'native client did not accept the support command', caster_eid)
            # The original master consumer clears the input timer, composes
            # support plays and schedules continue_old_round after animation.
            return native['on_master_input'](self, caster_eid)
        finally:
            self._revival_rpc_command = previous_rpc

    def restore_input(self, eid=None, remaining=None):
        # Reconnect: reopen the current input window with leftover wait seconds.
        target = self.current_input_eid if eid is None else eid
        if _revival_is_sync_pvp(self):
            if getattr(self, '_revival_pvp_failed', False):
                return
            if target is None:
                return _revival_pvp_fail(self, 'pvp_bridge_missing_caster',
                    'restored input window has no acting entity')
            current = getattr(self, 'current_input_eid', None)
            queued = (bool(getattr(self, '_revival_cmd_queue', None))
                      and not getattr(self, '_revival_queue_flushing', False))
            if queued or current is None or str(target) != str(current):
                return _revival_queue_put(self, (target, 'restore', remaining, None, None))
            info = self.entity_infos.get(target)
            if info is None:
                return _revival_pvp_fail(self, 'pvp_bridge_missing_entity',
                    'restored input entity is absent from the client', target)
            try:
                leftover = float(remaining)
            except (TypeError, ValueError):
                leftover = None
            if leftover is not None and leftover >= 0:
                self.player_input_time = leftover
            self._revival_input_grant = (ident(self.id), info,
                getattr(self, 'action_counter', None))
            # Native on_player_input means "execute now", not "reopen".
            # Only the owning manual player receives controls. Opponent/auto
            # units stay parked until the next authoritative action RPC.
            self.clear_master_input_timer()
            if self.master_is_player(info) and old_need_input(self, info):
                info.clear_command()
                return wait_for_input(self, info)
            return
        self._revival_rpc_command = True
        try:
            print('REVIVAL_PVP_RESTORE eid=%s wait=%s' % (target, remaining))
            result = native['on_player_input'](self, target)
            try:
                leftover = float(remaining)
            except (TypeError, ValueError):
                leftover = None
            if leftover is not None and leftover >= 0:
                try:
                    self.clear_master_input_timer()
                except Exception:
                    pass
                armed = False
                for name in ('add_master_input_timer', 'start_master_input_timer',
                             'set_master_input_timer'):
                    setter = getattr(self, name, None)
                    if callable(setter):
                        try:
                            setter(leftover)
                            armed = True
                            break
                        except Exception:
                            pass
                if not armed:
                    try:
                        self.master_input_timer = leftover
                    except Exception:
                        pass
                    try:
                        self.player_input_time = leftover
                    except Exception:
                        pass
            return result
        finally:
            self._revival_rpc_command = False

    def notify_finish(self):
        winners = self.winner_eid_list
        if winners is None or self._revival_result_sent:
            return
        if self._revival_finished_tasks is None:
            try:
                tasks = self.get_tower_task_statistics()
                if not isinstance(tasks, (list, tuple)):
                    raise ValueError('native task statistics must be a list')
                unique = []
                for task in tasks:
                    if isinstance(task, bool) or not isinstance(task, int) or task <= 0:
                        raise ValueError('invalid native task id')
                    if task not in unique:
                        unique.append(task)
                self._revival_finished_tasks = unique
            except Exception as error:
                print('REVIVAL_BATTLE_TASK_ERROR %s' % error)
                return
        self._revival_result_sent = report(self, 'result',
            {'winner_eids': [ident(value) for value in winners],
             'finished_task_list': list(self._revival_finished_tasks)}, include_state=True)

    for name, method in native.items():
        setattr(shadow, name, method)
    shadow.prepare = prepare
    shadow.check_auto_battle_fighting = check_auto_battle_fighting
    shadow.battle_fighting = fighting
    shadow.server_after_pre_play = after_pre_play
    shadow.on_add_entity = on_add_entity
    shadow.wait_for_player_input = wait_for_input
    shadow.need_input = need_input
    shadow.wait_for_master_input = wait_for_master_input
    shadow.on_player_input = on_player_input
    shadow.shuffle_random_speed = shuffle_random_speed
    local.shuffle_random_speed = shuffle_random_speed
    shadow.log_command = log_command
    shadow.process_command, shadow.skill_result = skill_target_hooks(
        old_process_command, old_skill_result, report)
    shadow.round_end = round_end
    shadow.real_start_action = start_action
    shadow.trigger_battle_end = trigger_end
    shadow.dispatch_plays = dispatch_plays
    shadow.battle_end_notice = end_notice
    shadow.change_auto_state = change_auto_state
    shadow.revival_set_battle_preferences = set_battle_preferences
    mode.press_auto = press_auto
    mode._revival_press_auto_original = old_press_auto
    button_type.on_click_begin = click_begin
    button_type.on_click_end = click_end
    button_type._revival_click_begin_original = old_click_begin
    button_type._revival_click_end_original = old_click_end
    shadow.do_command = do_ui_command
    shadow.revival_do_command = do_command
    shadow.revival_do_peer_command = do_peer_command
    shadow.revival_play_native_command = play_native_command
    shadow.revival_play_support_command = play_support_command
    shadow.revival_continue_input = continue_input
    shadow.revival_restore_input = restore_input
    shadow.notify_battle_finish = notify_finish
    shadow._revival_bridge_originals = {'prepare': old_prepare,
        'check_auto_battle_fighting': old_check_auto_fighting, 'battle_fighting': old_fighting,
        'wait_for_player_input': old_wait, 'need_input': old_need_input,
        'shuffle_random_speed': old_shuffle,
        'wait_for_master_input': native['wait_for_master_input'],
        'on_player_input': native['on_player_input'],
        'log_command': old_log_command,
        'process_command': old_process_command, 'skill_result': old_skill_result,
        'real_start_action': old_start_action, 'trigger_battle_end': old_trigger_end,
        'dispatch_plays': old_dispatch_plays, 'battle_end_notice': old_end_notice}
    shadow._revival_bridge_originals['on_add_entity'] = old_on_add_entity
    shadow._revival_bridge_originals['do_command'] = old_ui_command
    shadow._revival_bridge_version = 31

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
