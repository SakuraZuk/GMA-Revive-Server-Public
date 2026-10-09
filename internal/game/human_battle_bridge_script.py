# -*- coding: utf-8 -*-
# 真人共享输入桥；只扩展兼容的rev12至15，不替换其原生技能演算。
def _install_revival_human_bridge():
    import time
    import gworld
    from battle_logic import client_battle
    from battle_logic.controlers import server as server_control
    shadow = client_battle.shadow_battle
    if getattr(shadow, '_revival_bridge_version', 0) not in (12, 13, 14, 15):
        raise RuntimeError('共享输入桥需要兼容战斗桥版本12至15')
    if getattr(shadow, '_revival_human_bridge', 0) == 2:
        return
    def function(method):
        return getattr(method, 'im_func', method)
    originals = getattr(shadow, '_revival_human_originals', {})
    def original(name, captured):
        method = function(getattr(shadow, name))
        if name in originals:
            return originals[name]
        if getattr(shadow, '_revival_human_bridge', 0):
            code = getattr(method, 'func_code', getattr(method, '__code__', None))
            closure = getattr(method, 'func_closure', getattr(method, '__closure__', None))
            if code is not None and closure:
                for freevar, cell in zip(code.co_freevars, closure):
                    if freevar == captured:
                        return cell.cell_contents
        return method
    old_wait = original('wait_for_player_input', 'old_wait')
    old_master_wait = original('wait_for_master_input', 'old_master_wait')
    old_auto = original('change_auto_state', 'old_auto')
    old_notify = original('notify_battle_finish', 'old_notify')
    old_prepare = original('prepare', 'old_prepare')
    old_preferences = original('revival_set_battle_preferences', 'old_preferences')
    old_speed = original('set_battle_speed', 'old_speed')
    validate = function(server_control.controler.do_command)
    def shared(self):
        return bool(getattr(self, 'extra_info', {}).get('human_shared'))
    def snapshot(self):
        units = []
        for eid, info in self.entity_infos.items():
            coord = info.get_attr('hex_coord')
            if not isinstance(coord, (list, tuple)) or len(coord) != 3:
                continue
            camp = info.get_camp_id()
            row = {'eid': str(eid), 'role': int(info.get_role_id()),
                   'kind': 'support' if info.is_support() else ('ally' if camp == 1 else ('enemy' if camp == 2 else 'neutral')),
                   'hex': list(coord), 'alive': not info.is_dead(), 'camp': int(camp)}
            master = info.get_attr('master')
            row['master'] = None if master is None else str(master)
            card = info.get_attr('card_uuid')
            row['card_uuid'] = None if card is None else str(card)
            for key in ('hp', 'max_hp', 'ap'):
                value = info.get_attr(key)
                if isinstance(value, (int, float)):
                    row[key] = value
            units.append(row)
        return units
    def emit(self, kind, data):
        sequence = self._revival_event_sequence + 1
        payload = {'battle_uuid': str(self.id), 'sequence': sequence,
                   't': max(0.0, time.time() - self._revival_event_begin), 'kind': kind, 'data': data}
        gworld.get_player().server_proxy.do_command('__battle_event__', [payload])
        self._revival_event_sequence = sequence
    def wait(self, info):
        result = old_wait(self, info)
        if shared(self):
            index = getattr(self, '_revival_human_command_index', 0)
            emit(self, 'human_input', {'action': self.action_counter, 'eid': str(info.eid),
                 'master': str(info.get_attr('master')), 'units': snapshot(self), 'command_index': index})
            replay = getattr(self, '_revival_human_replay', [])
            if replay:
                item = replay[0]
                if item['index'] == index + 1 and item['action'] == self.action_counter and str(info.eid) == item['eid']:
                    replay.pop(0)
                    master = info.get_attr('master')
                    if str(master) != item['master']:
                        raise ValueError('shared replay master mismatch')
                    command(self, master, item['name'], item['args'], item['index'], str(self.id))
        return result
    def master_wait(self, master):
        if shared(self):
            self.clear_master_input_timer()
            return
        return old_master_wait(self, master)
    def automatic(self, state):
        if shared(self):
            self.auto_battle_set = set()
            self.on_auto_state_update(gworld.get_player().eid)
            return
        return old_auto(self, state)
    def command(self, master, name, args, index, uuid):
        if not shared(self) or str(self.id) != str(uuid):
            return
        previous = getattr(self, '_revival_human_command_index', 0)
        if index <= previous:
            return
        if index != previous + 1:
            raise ValueError('shared command sequence gap')
        args = list(args)
        if name in ('move_to', 'use_skill') and args and isinstance(args[-1], list):
            args[-1] = tuple(args[-1])
        self._revival_human_command_index = index
        try:
            validate(self, master, name, *args)
        except:
            self._revival_human_command_index = previous
            raise
    def prepare(self, *args, **kwargs):
        self._revival_human_command_index = 0
        self._revival_human_replay = []
        return old_prepare(self, *args, **kwargs)
    def preferences(self, speed, automatic_state):
        result = old_preferences(self, 1 if shared(self) else speed, False if shared(self) else automatic_state)
        if shared(self):
            self.auto_battle_set = set()
            self.clear_master_input_timer()
            self.on_auto_state_update(gworld.get_player().eid)
        return result
    def speed(self, value):
        return old_speed(self, 1 if shared(self) else value)
    def replay(self, commands, uuid):
        if shared(self) and str(self.id) == str(uuid):
            self._revival_human_replay = list(commands)
    def abort(self):
        if shared(self):
            self._revival_result_sent = True
            self._revival_human_replay = []
            self.clear_master_input_timer()
    def finish(self):
        if not shared(self):
            return old_notify(self)
        if self.winner_eid_list is None or getattr(self, '_revival_result_sent', False):
            return
        emit(self, 'human_result', {'winner_eids': [str(eid) for eid in self.winner_eid_list],
             'action': self.action_counter, 'units': snapshot(self), 'finished_task_list': [],
             'command_index': getattr(self, '_revival_human_command_index', 0)})
        self._revival_result_sent = True
    shadow.wait_for_player_input = wait
    shadow.wait_for_master_input = master_wait
    shadow.change_auto_state = automatic
    shadow.revival_do_shared_command = command
    shadow.notify_battle_finish = finish
    shadow.prepare = prepare
    shadow.revival_set_battle_preferences = preferences
    shadow.revival_set_shared_replay = replay
    shadow.revival_abort_shared = abort
    shadow.set_battle_speed = speed
    shadow._revival_human_originals = {'wait_for_player_input': old_wait,
        'wait_for_master_input': old_master_wait, 'change_auto_state': old_auto,
        'notify_battle_finish': old_notify, 'prepare': old_prepare,
        'revival_set_battle_preferences': old_preferences, 'set_battle_speed': old_speed}
    shadow._revival_human_bridge = 2
def _revival_start_human_bridge():
    try:
        _install_revival_human_bridge()
        print('HS_HUMAN_BRIDGE_READY 2')
    except Exception as error:
        print('HS_HUMAN_BRIDGE_WAIT %s' % error)
        try:
            import game3d
            game3d.delay_exec(1500, _revival_start_human_bridge)
        except Exception as retry_error:
            print('HS_HUMAN_BRIDGE_RETRY_ERROR %s' % retry_error)
_revival_start_human_bridge()
