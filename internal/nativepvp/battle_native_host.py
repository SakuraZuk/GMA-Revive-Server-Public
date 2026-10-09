# -*- coding: utf-8 -*-
"""本版Python2原生战斗组件的显式无界面宿主。"""
import heapq
import imp
import logging
import hashlib
import math
import sys
import types
import __builtin__
from bson import ObjectId
from battle_targets import skill_target_hooks


class NativeCommandRejected(ValueError):
    """Invalid player input; native state and its input timer remain usable."""


def json_value(value):
    if isinstance(value, ObjectId):
        return str(value)
    if value is None or isinstance(value, (bool, int, long, float, unicode)):
        return value
    if isinstance(value, str):
        try:
            return value.decode("utf-8")
        except UnicodeDecodeError:
            return {"bytes_hex": value.encode("hex")}
    if isinstance(value, (tuple, list, set, frozenset)):
        return [json_value(v) for v in value]
    if isinstance(value, dict):
        return {str(k): json_value(v) for k, v in value.iteritems()}
    if hasattr(type(value), "__slots__"):
        return {key: json_value(getattr(value, key)) for key in type(value).__slots__
                if hasattr(value, key)}
    if hasattr(value, "__dict__"):
        return json_value(vars(value))
    raise TypeError("cannot serialize native value %s" % type(value).__name__)


def _activity_int(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, long)):
        raise ValueError("%s must be an integer" % field)
    return int(value)


def _activity_bool(value, field):
    if not isinstance(value, bool):
        raise ValueError("%s must be boolean" % field)
    return value


def _activity_json_value(value, field):
    """Detach one JSON value accepted by the native activity-buff code."""
    if value is None or isinstance(value, (bool, int, long, unicode, str)):
        return value
    if isinstance(value, float):
        if math.isnan(value) or math.isinf(value):
            raise ValueError("%s must be a finite JSON number" % field)
        return value
    if isinstance(value, list):
        return [_activity_json_value(item, "%s[%d]" % (field, index))
                for index, item in enumerate(value)]
    if isinstance(value, dict):
        normalized = {}
        for key, item in value.iteritems():
            if not isinstance(key, (unicode, str)):
                raise ValueError("%s object keys must be strings" % field)
            normalized[key] = _activity_json_value(item, "%s.%s" % (field, key))
        return normalized
    raise ValueError("%s must be a JSON value" % field)


def normalize_activity_buff_snapshot(snapshot):
    """Validate JSON activity data and restore integer camp keys."""
    if not isinstance(snapshot, dict):
        raise ValueError("activity_buff_snapshot must be a mapping")
    unknown = set(snapshot) - set(("add_battle_skills", "change_attr_data", "enemy_level_added", "hs_summer_closed_beta"))
    if unknown:
        raise ValueError("unknown activity buff fields: %s" % sorted(unknown))
    skills = snapshot.get("add_battle_skills", [])
    if not isinstance(skills, list):
        raise ValueError("add_battle_skills must be a list")
    normalized_skills = []
    for index, skill in enumerate(skills):
        if not isinstance(skill, list) or len(skill) != 6:
            raise ValueError("add_battle_skills[%d] must contain six fields" % index)
        normalized_skills.append([
            _activity_int(skill[0], "add_battle_skills[%d].skill_id" % index),
            _activity_int(skill[1], "add_battle_skills[%d].camp_id" % index),
            _activity_int(skill[2], "add_battle_skills[%d].level" % index),
            _activity_int(skill[3], "add_battle_skills[%d].enhance_level" % index),
            _activity_int(skill[4], "add_battle_skills[%d].card_grade" % index),
            _activity_bool(skill[5],
                           "add_battle_skills[%d].show_enable" % index)])
    change = snapshot.get("change_attr_data", {})
    if not isinstance(change, dict):
        raise ValueError("change_attr_data must be a mapping")
    unknown_change = set(change) - set(("buff",))
    if unknown_change:
        raise ValueError("unknown change_attr_data fields: %s" % sorted(unknown_change))
    buff = change.get("buff", {})
    if not isinstance(buff, dict):
        raise ValueError("change_attr_data.buff must be a mapping")
    normalized_buff = {}
    for camp_id, entries in buff.iteritems():
        if isinstance(camp_id, bool):
            raise ValueError("change_attr_data.buff camp id must be an integer")
        try:
            normalized_camp = int(camp_id)
        except (TypeError, ValueError):
            raise ValueError("change_attr_data.buff camp id must be an integer")
        if str(normalized_camp) != str(camp_id):
            raise ValueError("change_attr_data.buff camp id must be canonical")
        if not isinstance(entries, list):
            raise ValueError("change_attr_data.buff[%s] must be a list" % camp_id)
        normalized_entries = []
        for index, entry in enumerate(entries):
            if not isinstance(entry, list) or len(entry) != 2:
                raise ValueError("change_attr_data.buff[%s][%d] must contain two fields" %
                                 (camp_id, index))
            normalized_entries.append([
                _activity_int(entry[0],
                              "change_attr_data.buff[%s][%d].buff_id" % (camp_id, index)),
                _activity_json_value(
                    entry[1],
                    "change_attr_data.buff[%s][%d].user_property" % (camp_id, index))])
        if normalized_camp in normalized_buff:
            raise ValueError("duplicate activity buff camp id %s" % normalized_camp)
        normalized_buff[normalized_camp] = normalized_entries
    result = {"add_battle_skills": normalized_skills,
              "change_attr_data": {"buff": normalized_buff}}
    if "enemy_level_added" in snapshot:
        result["enemy_level_added"] = _activity_int(snapshot["enemy_level_added"], "enemy_level_added")
        if result["enemy_level_added"] not in (-20, -10, 0, 45, 150):
            raise ValueError("enemy_level_added is outside the native correction table")
    if "hs_summer_closed_beta" in snapshot:
        result["hs_summer_closed_beta"] = _activity_bool(snapshot["hs_summer_closed_beta"], "hs_summer_closed_beta")
    return result


class NativeProxy(object):
    def __init__(self, host):
        self.host = host
        self.all_clients = self

    def sync_battle_method(self, name, args, kwds):
        payload = {"args": args, "kwds": kwds}
        context = getattr(self.host.battle, '_revival_skill_cast', None)
        if name == 'sync_command' and context is not None:
            payload['cast_id'] = context['cast_id']
        if (name == 'sync_command' and len(args) >= 3 and args[2]
                and args[2][0] in ('use_support_skill', 'use_extra_support_skill')):
            # do_master_action_right_now captured the original driver's round
            # before allow_player_input(False) clears current_input_eid.
            round_eid = getattr(self.host.battle, '_revival_master_round_eid', None)
            if round_eid is None:
                raise RuntimeError('native support command has no original round')
            payload['round_eid'] = round_eid
        self.host.emit(name, payload)

    def before_real_battle_end(self):
        self.host.emit("before_real_battle_end", {})

    def notify_battle_finish(self, winners):
        self.host.result = {"winner_eids": list(winners),
                            "outcome": "win" if self.host.master in winners else "loss"}
        self.host.emit("result", self.host.result)

    def set_wait_story_info(self, *args):
        self.host.emit("story", {"args": args})
        self.host.clock.add(0, lambda: self.host.battle.run_battle_storyline_end(args[0], args[2]))

    def set_wait_guide_info(self, *args):
        self.host.emit("guide", {"args": args})
        self.host.clock.add(0, lambda: self.host.battle.battle_guide_end(args[2]))

    def set_boss_tips(self, *args):
        self.host.emit("boss_tips", {"args": args})


class VirtualClock(object):
    def __init__(self):
        self.now = 0.0
        self.counter = 0
        self.heap = []
        self.active = set()

    def add(self, delay, callback):
        self.counter += 1
        token = self.counter
        heapq.heappush(self.heap, (self.now + max(float(delay), 0.000001), token, callback))
        self.active.add(token)
        return TimerHandle(self, token)

    def cancel(self, token):
        self.active.discard(token.token if isinstance(token, TimerHandle) else token)

    def peek_delay(self):
        """Seconds until the next live callback, or None if the heap is empty."""
        while self.heap:
            when, token, callback = self.heap[0]
            if token in self.active:
                delay = when - self.now
                if delay < 0:
                    delay = 0.0
                return delay
            heapq.heappop(self.heap)
        return None

    def step(self):
        while self.heap:
            when, token, callback = heapq.heappop(self.heap)
            if token in self.active:
                self.active.remove(token)
                self.now = when
                callback()
                return True
        return False


class TimerHandle(object):
    def __init__(self, clock, token):
        self.clock, self.token = clock, token

    def cancel(self):
        self.clock.cancel(self.token)


class NativeData(object):
    def get_language_data_path(self):
        return "datas"

    def __getattr__(self, name):
        module = __import__("datas." + name, fromlist=["data"])
        value = module.data
        setattr(self, name, value)
        return value


class IdManager(object):
    is_id_type = staticmethod(lambda value: isinstance(value, ObjectId))
    bytes2id = staticmethod(lambda value: ObjectId(value))
    str2id = staticmethod(lambda value: ObjectId(value))
    id2bytes = staticmethod(lambda value: value.binary)
    id2str = staticmethod(str)


class Host(object):
    def __init__(self):
        self.clock = VirtualClock()
        self.battle = None
        self.players = {}
        self.events = []
        self.sequence = 0
        self.result = None
        self.master = None
        self.enemy_master = None
        self.id_counter = 0
        self.id_namespace = "native"
        self.last_units = {}
        self.steps = 0
        self.auto = True
        self.failed = False

    def new_id(self):
        self.id_counter += 1
        return hashlib.sha256("%s:%s" % (self.id_namespace, self.id_counter)).hexdigest()

    def emit(self, kind, data):
        self.sequence += 1
        self.events.append({"sequence": self.sequence, "t": self.clock.now,
                            "kind": kind, "data": json_value(data)})

    def start(self, metadata, roster, seed, auto=True):
        if self.battle is not None:
            raise ValueError("one battle per worker; create a new worker")
        if not isinstance(roster, list) or not 0 <= len(roster) <= 8:
            raise ValueError("roster requires zero to eight native card specifications")
        from battle_logic.server_battle import server_battle
        from custom_types.card import card as native_card, card_list
        from utils import callback_mgr
        data = sys.modules["data"]
        metadata = dict(metadata)
        if "activity_buff_snapshot" in metadata:
            metadata["activity_buff_snapshot"] = normalize_activity_buff_snapshot(
                metadata["activity_buff_snapshot"])
        dungeon_id = int(metadata["dungeon_id"])
        dungeon = data.dungeons[dungeon_id]
        metadata["battle_id"] = int(metadata.get("battle_id") or dungeon.dungeon_battle_id)
        if metadata["battle_id"] not in data.battle_info:
            raise ValueError("unknown battle_id")
        callback_mgr.get_time = lambda: self.clock.now
        self.metadata = metadata
        self.id_namespace = "%s:%s:%s" % (metadata.get("avatar_id", "native-player"), metadata["battle_id"], seed)
        self.master = str(metadata.get("avatar_id", "native-player")).encode("utf-8")
        self.enemy_master = None
        self.auto = bool(auto)
        self.battle = b = server_battle()
        b.proxy = NativeProxy(self)
        if int(metadata.get("battle_type") or 2) == 3:
            # 本版51169E3F读取avatar_info.extra.asyn_pvp_auto，2CC3F05D
            # 由set_battle_candidate提供。宿主冻结服务器偏好，不改原生AI算法。
            from custom_types.avatar_info import avatar_info
            asynchronous_auto = metadata.get("asyn_pvp_auto", False)
            if not isinstance(asynchronous_auto, bool):
                raise ValueError("asyn_pvp_auto must be boolean")
            b.avatar_info = avatar_info(ObjectId(metadata["avatar_id"]),
                                        {"extra": {"asyn_pvp_auto": asynchronous_auto}})
            b.auto_battle_habit = {}
        process, resolve = skill_target_hooks(
            b.process_command.im_func, b.skill_result.im_func,
            lambda battle, kind, payload: self.emit(kind, payload))
        b.process_command = types.MethodType(process, b)
        b.skill_result = types.MethodType(resolve, b)
        original_master_action = b.do_master_action_right_now
        def master_action(info, *args, **kwargs):
            # real_continue_old_round uses this exact native round_eid for
            # manual and automatic support; current_input_eid may be absent.
            previous = getattr(b, '_revival_master_round_eid', None)
            b._revival_master_round_eid = b.round_eid
            try:
                return original_master_action(info, *args, **kwargs)
            finally:
                b._revival_master_round_eid = previous
        b.do_master_action_right_now = master_action
        original_wave = b.trigger_action_set_wave_text
        def native_wave(text):
            self.emit("wave", {"text": text})
            return original_wave(text)
        b.trigger_action_set_wave_text = native_wave
        cards = card_list()
        identities = set()
        loaded_roster = []
        for spec in roster:
            if not isinstance(spec, dict):
                raise ValueError("native card specification must be a dictionary")
            if spec.get("card_id") not in data.role_info:
                raise ValueError("invalid card_id %r" % spec.get("card_id"))
            spec = dict(spec)
            spec.setdefault("uuid", self.new_id()[:24])
            card = native_card.load(spec)
            if card.uuid in identities:
                raise ValueError("duplicate card UUID")
            identities.add(card.uuid)
            cards.append(card)
            loaded_roster.append({"card": native_card.client_dump(card),
                                  "dress_id": card.get_dress_id(),
                                  "battle_attrs": {key: card.get_battle_attr(key)
                                                   for key in card.get_battle_attrs()},
                                  "skills": card.get_skill_list()})
        # A synchronous arena has a second owner in the same native battle.
        # Keep its card pool separate so duplicate UUIDs cannot accidentally
        # make one side address the other side's card.
        enemy_master = None
        enemy_cards = card_list()
        enemy_identities = set()
        enemy_loaded_roster = []
        enemy_roster = metadata.get("enemy_roster") or []
        if enemy_roster:
            enemy_master = str(metadata.get("enemy_id") or "native-enemy").encode("utf-8")
            if enemy_master == self.master:
                raise ValueError("enemy_id must differ from avatar_id")
            self.enemy_master = enemy_master
            for spec in enemy_roster:
                if not isinstance(spec, dict):
                    raise ValueError("native enemy card specification must be a dictionary")
                if spec.get("card_id") not in data.role_info:
                    raise ValueError("invalid enemy card_id %r" % spec.get("card_id"))
                spec = dict(spec)
                spec.setdefault("uuid", self.new_id()[:24])
                card = native_card.load(spec)
                if card.uuid in enemy_identities or card.uuid in identities:
                    raise ValueError("duplicate card UUID across arena owners")
                enemy_identities.add(card.uuid)
                enemy_cards.append(card)
                enemy_loaded_roster.append({"card": native_card.client_dump(card),
                                            "dress_id": card.get_dress_id(),
                                            "battle_attrs": {key: card.get_battle_attr(key)
                                                              for key in card.get_battle_attrs()},
                                            "skills": card.get_skill_list()})

        # Prepare establishes the original map's main/support formations. Store
        # both card pools first; defer assigning those slots until capacity is known.
        card_slots = [c.uuid for c in cards]
        b.add_fighting_cards(card_slots, cards, self.master)
        enemy_card_slots = [c.uuid for c in enemy_cards]
        if enemy_master is not None:
            b.set_battle_type(int(metadata.get("battle_type") or 2))
            b.add_fighting_cards(enemy_card_slots, enemy_cards, enemy_master)
        automatic = set()
        if self.auto:
            automatic.add(self.master)
            if enemy_master is not None:
                automatic.add(enemy_master)
        elif enemy_master is not None and metadata.get("enemy_auto", False):
            automatic.add(enemy_master)
        b.set_auto_battle_set(automatic)
        if "activity_buff_snapshot" in metadata:
            # battle_base consumes this field during prepare/battle_fighting;
            # keeping it on the battle object also mirrors server battle data.
            b.extra_info = metadata["activity_buff_snapshot"]
        owners = [self.master] + ([enemy_master] if enemy_master is not None else [])
        if self.enemy_master is not None:
            self._install_lockstep_shuffle(b)
        b.prepare(owners, int(metadata["battle_id"]), int(seed), int(metadata["dungeon_id"]))
        slots = b.fighting_slots[self.master]
        fighting_count = sum(slot not in b.support_slots for slot in slots)
        support_count = len(slots) - fighting_count
        if "storyline_avatar_list" in metadata:
            role_list = metadata["storyline_avatar_list"]
            if (not b.get_battle_data().avatar_list_by_storyline or
                    not isinstance(role_list, list) or len(role_list) > len(slots) or
                    any(type(role) not in (int, long) or role < 0 for role in role_list)):
                raise ValueError("storyline_avatar_list requires native storyline battle and valid roles")
            b.set_storyline_avatar_list(role_list)
        script_roles = b.get_my_avatar_list() or []
        scripted = any(script_roles)
        # Fixed story teams historically accept the ordinary owned 4+2 team,
        # then replace/truncate it with the script's roles (often only 1+0).
        selection_fighting_count = max(4, fighting_count) if scripted else fighting_count
        selection_support_count = max(2, support_count) if scripted else support_count
        explicit_team = ("fighting_card_uuids" in metadata or "support_card_uuids" in metadata)
        if explicit_team:
            def selected(name):
                values = metadata.get(name, [])
                if not isinstance(values, list):
                    raise ValueError("%s requires a list of UUID slots" % name)
                return [ObjectId(value) if value is not None else None for value in values]
            fighting = selected("fighting_card_uuids")
            support = selected("support_card_uuids")
        else:
            fighting = [c.uuid for c in cards[:selection_fighting_count]]
            support = [c.uuid for c in cards[selection_fighting_count:]]
        selected_ids = [value for value in fighting + support if value is not None]
        if len(set(selected_ids)) != len(selected_ids):
            raise ValueError("duplicate selected card UUID")
        if set(selected_ids) != identities:
            raise ValueError("team UUIDs must select every roster card exactly once")
        if (len(fighting) > selection_fighting_count or
                len(support) > selection_support_count):
            raise ValueError("team exceeds native battle slot capacity %s+%s" %
                             (fighting_count, support_count))
        # Scripted maps may expose fewer slots than the owned formation.
        active_fighting, active_support = fighting[:fighting_count], support[:support_count]
        native_slots = active_fighting + ([None] * (fighting_count - len(active_fighting)) +
                                         active_support if active_support else [])
        if native_slots != card_slots:
            b.add_fighting_cards(native_slots, cards, self.master)
        if not any(fighting) and not any(script_roles[:fighting_count]):
            raise ValueError("empty fighting roster requires native scripted fighting roles")
        metadata["fighting_card_uuids"] = json_value(fighting)
        metadata["support_card_uuids"] = json_value(support)
        if enemy_master is not None:
            enemy_slots = b.fighting_slots[enemy_master]
            enemy_fighting_count = sum(slot not in b.support_slots for slot in enemy_slots)
            enemy_support_count = len(enemy_slots) - enemy_fighting_count
            enemy_fighting = [c.uuid for c in enemy_cards[:enemy_fighting_count]]
            enemy_support = [c.uuid for c in enemy_cards[enemy_fighting_count:]]
            explicit_enemy = ("enemy_fighting_card_uuids" in metadata or
                              "enemy_support_card_uuids" in metadata)
            if explicit_enemy:
                def selected_enemy(name):
                    values = metadata.get(name, [])
                    if not isinstance(values, list):
                        raise ValueError("%s requires a list of UUID slots" % name)
                    return [ObjectId(value) if value is not None else None for value in values]
                enemy_fighting = selected_enemy("enemy_fighting_card_uuids")
                enemy_support = selected_enemy("enemy_support_card_uuids")
            enemy_selected = [value for value in enemy_fighting + enemy_support if value is not None]
            if len(set(enemy_selected)) != len(enemy_selected) or set(enemy_selected) != enemy_identities:
                raise ValueError("enemy team UUIDs must select every roster card exactly once")
            if (len(enemy_fighting) > enemy_fighting_count or
                    len(enemy_support) > enemy_support_count):
                raise ValueError("enemy team exceeds native battle slot capacity %s+%s" %
                                 (enemy_fighting_count, enemy_support_count))
            enemy_active_fighting = enemy_fighting[:enemy_fighting_count]
            enemy_active_support = enemy_support[:enemy_support_count]
            enemy_native_slots = (enemy_active_fighting +
                                  ([None] * (enemy_fighting_count - len(enemy_active_fighting)) +
                                   enemy_active_support if enemy_active_support else []))
            if enemy_native_slots != enemy_card_slots:
                b.add_fighting_cards(enemy_native_slots, enemy_cards, enemy_master)
            metadata["enemy_fighting_card_uuids"] = json_value(enemy_fighting)
            metadata["enemy_support_card_uuids"] = json_value(enemy_support)
        self.emit("roster_loaded", {"cards": loaded_roster,
                                   "fighting_card_uuids": fighting,
                                   "support_card_uuids": support,
                                   "native_card_slots": native_slots,
                                   "scripted_roles": script_roles,
                                   "unused_card_uuids": [identity for index, identity in enumerate(native_slots)
                                                         if identity is not None and index < len(script_roles)
                                                         and script_roles[index]] +
                                                        [identity for identity in fighting[fighting_count:] +
                                                         support[support_count:] if identity is not None],
                                   "native_slots": [{"eid": slot[0], "hex": slot[1],
                                                     "support": slot in b.support_slots} for slot in slots],
                                   "enemy_cards": enemy_loaded_roster,
                                   "enemy_slots": ([{"eid": slot[0], "hex": slot[1],
                                                     "support": slot in b.support_slots}
                                                    for slot in b.fighting_slots[enemy_master]]
                                                    if enemy_master is not None else [])})
        b.battle_fighting()
        initial = self.snapshot()
        self.last_units = {u["eid"]: u for u in initial["units"]}
        self.emit("initial_state", initial)
        b.start()
        checker = getattr(b, "check_on_battle_start", None)
        if callable(checker):
            checker()
        self.capture_changes()
        return self.response()

    def _install_lockstep_shuffle(self, battle):
        """Consume RNG by (master, role, mf_id) so both engines share order."""
        def shuffled():
            rows = list(battle.iter_alive_entity_info())
            keyed = []
            for eid, entity in rows:
                master = ""
                role = 0
                mf = 0
                try:
                    value = entity.get_master()
                    if value is not None:
                        master = str(value)
                except Exception:
                    master = ""
                try:
                    role = int(entity.get_role_id() or 0)
                except Exception:
                    role = 0
                try:
                    mf = int(entity.get_attr("mf_id") or 0)
                except Exception:
                    mf = 0
                keyed.append(((master, role, mf), eid, entity))
            keyed.sort(key=lambda item: item[0])
            for _key, _eid, entity in keyed:
                entity.random_speed()
                entity.attrs["init_avatar"] = True
        battle.shuffle_random_speed = shuffled

    def snapshot(self):
        b = self.battle
        if b is None:
            return {"units": []}
        units = []
        for eid, info in sorted(b.entity_infos.items()):
            attrs = {k: info.get_attr(k) for k in (
                "hp", "max_hp", "camp_id", "master_eid", "role_id",
                "hex_coord", "level", "ap_speed", "sp", "atk", "defence",
                "crit_prob", "crit_mul", "effect_accuracy", "effect_evasion",
                "dress_id", "model_id", "support", "extra_support_skill", "skill_enhance_count",
                "suit_effect_num", "rune_min_level", "rune_min_star",
                "mf_id", "mf_radius", "is_summon", "create_entity_eid",
                "create_skill_id", "origin_coord") if k in info.attrs}
            attrs = json_value(attrs)
            unit = {"eid": json_value(eid),
                          "kind": ("field" if info.type == 4 else
                                   "support" if attrs.get("support") else
                                   {1: "hero", 2: "monster"}.get(attrs.get("camp_id"), "entity")),
                          "native_type": info.type,
                          "role": attrs.get("role_id"),
                          "hex": attrs.get("hex_coord"),
                          "hp": attrs.get("hp"), "max_hp": attrs.get("max_hp"),
                          "camp": attrs.get("camp_id"), "attrs": attrs,
                          "is_summon": bool(attrs.get("is_summon")),
                          "dead": bool(getattr(info, "dead", False))}
            for key in ("mf_id", "mf_radius", "create_entity_eid",
                        "create_skill_id", "origin_coord"):
                if key in attrs and attrs[key] is not None:
                    unit[key] = (str(attrs[key]) if key == "create_entity_eid"
                                 and attrs[key] is not None else attrs[key])
            units.append(unit)
        timer = bool(getattr(b, "master_input_timer", None))
        current_master = self._current_master()
        owners = set()
        if self.master is not None:
            owners.add(json_value(self.master))
        if self.enemy_master is not None:
            owners.add(json_value(self.enemy_master))
        raw_master = None
        if getattr(b, "current_input_eid", None) is not None:
            user = b.get_entity(b.current_input_eid)
            if user is not None:
                try:
                    raw_master = user.get_master()
                except Exception:
                    raw_master = None
        awaiting_player = (timer and not self.auto and not self._owner_is_auto(raw_master)
                           and current_master is not None
                           and current_master in owners)
        try:
            input_time = float(getattr(b, "player_input_time", 20) or 20)
        except (TypeError, ValueError):
            input_time = 20.0
        next_delay = self.clock.peek_delay()
        return {"units": units, "battle_status": b.battle_status,
                "action_counter": b.action_counter,
                "round_eid": json_value(b.round_eid),
                "current_input_eid": json_value(b.current_input_eid),
                "current_master": current_master,
                "awaiting_input": timer and not self.auto,
                "awaiting_player": awaiting_player,
                "player_input_time": input_time,
                "next_delay": next_delay,
                "result": self.result}

    def _current_master(self):
        b = self.battle
        if b is None or getattr(b, "current_input_eid", None) is None:
            return None
        user = b.get_entity(b.current_input_eid)
        if user is None:
            return None
        try:
            return json_value(user.get_master())
        except Exception:
            return None

    def capture_changes(self):
        current = {u["eid"]: u for u in self.snapshot()["units"]}
        for eid in sorted(current):
            unit, old = current[eid], self.last_units.get(eid)
            if old != unit:
                self.emit("unit_spawn" if old is None else "unit_state", unit)
        for eid in sorted(set(self.last_units) - set(current)):
            self.emit("unit_removed", {"eid": eid})
        self.last_units = current

    def response(self):
        events, self.events = self.events, []
        return {"metadata": self.metadata, "t": self.clock.now, "steps": self.steps,
                "status": "finished" if self.result is not None else "running",
                "state": self.snapshot(), "events": events, "result": self.result}

    def step(self, command=None):
        if self.battle is None:
            raise ValueError("battle not started")
        if self.failed:
            raise RuntimeError("native battle failed; discard worker")
        if self.result is not None and command is not None:
            raise ValueError("battle already finished")
        if command is not None:
            self.submit_command(command)
        if self.result is None:
            self.advance()
        return self.response()

    def submit_command(self, command):
        if not isinstance(command, dict) or command.get("name") not in ("use_skill", "move_to", "idle"):
            raise NativeCommandRejected("command requires name use_skill/move_to/idle and native args")
        if self.auto:
            raise NativeCommandRejected("manual commands require auto=False at start")
        name, args = command["name"], command.get("args", [])
        arity = {"use_skill": 3, "move_to": 2, "idle": 0}[name]
        if not isinstance(args, list) or len(args) != arity:
            raise NativeCommandRejected("invalid command argument count")
        b = self.battle
        if not getattr(b, "master_input_timer", None):
            raise NativeCommandRejected("native engine is not waiting for input")
        user = b.get_entity(b.current_input_eid)
        owners = [self.master]
        if self.enemy_master is not None:
            owners.append(self.enemy_master)
        if user is None or user.get_master() not in owners:
            raise NativeCommandRejected("current input does not belong to a player in this battle")
        args = list(args)
        if args and args[0] != user.eid:
            caster = b.get_entity(args[0]) if name == "use_skill" else None
            if (caster is None or caster.get_master() != user.get_master()
                    or not (caster.is_support() or caster.have_extra_support_skill())):
                raise NativeCommandRejected("command entity is not current input or an owned native supporter")
        if name == "use_skill" and type(args[1]) not in (int, long):
            raise NativeCommandRejected("skill_id must be an integer")
        if name == "move_to" or (name == "use_skill" and isinstance(args[2], list)):
            coord = args[-1]
            if (not isinstance(coord, list) or len(coord) != 3 or
                    any(type(v) not in (int, long) for v in coord) or sum(coord) != 0):
                raise NativeCommandRejected("target requires three integer cube coordinates summing to zero")
            args[-1] = tuple(coord)
        # Native do_command returns None for both rejection and acceptance.
        accepted = [False]
        executing = [False]
        original_input, original_master = b.on_player_input, b.on_master_input
        original_execute, original_error = b.real_do_command, b.battle_error
        original_master_execute = b.real_do_master_command
        def on_input(eid):
            accepted[0] = True
            return original_input(eid)
        def on_master_input(eid):
            accepted[0] = True
            return original_master(eid)
        def execute(*native_args):
            # real_do_command can mutate PVP bookkeeping before on_input.
            # Validation rejects occur before this execution boundary.
            executing[0] = True
            return original_execute(*native_args)
        def master_execute(*native_args):
            # Master commands can mutate/set the supporter before their
            # acceptance callback, just like ordinary player commands.
            executing[0] = True
            return original_master_execute(*native_args)
        def reject(message):
            if executing[0] or accepted[0]:
                raise RuntimeError("native battle error after command execution began: %s" % message)
            raise NativeCommandRejected("native command rejected: %s" % message)
        b.on_player_input, b.on_master_input = on_input, on_master_input
        b.real_do_command, b.battle_error = execute, reject
        b.real_do_master_command = master_execute
        try:
            b.do_command(user.get_master(), name.encode("ascii"), *args)
        finally:
            b.on_player_input, b.on_master_input = original_input, original_master
            b.real_do_command, b.battle_error = original_execute, original_error
            b.real_do_master_command = original_master_execute
        if not accepted[0]:
            if executing[0]:
                raise RuntimeError("native command execution began without an acceptance callback")
            raise NativeCommandRejected("native command was not accepted")
        self.emit("input_command", command)

    def advance(self):
        if self.failed:
            raise RuntimeError("native battle failed; discard worker")
        try:
            if not self.clock.step():
                raise RuntimeError("native battle has no scheduled callback and no result")
            self.steps += 1
            self.capture_changes()
        except BaseException:
            self.failed = True
            raise

    def _avatar_key(self, value):
        """Normalize ObjectId / utf-8 hex / bytes into the 24-hex avatar id."""
        if value is None:
            return ""
        dumped = json_value(value)
        if isinstance(dumped, dict) and dumped.get("bytes_hex"):
            return str(dumped.get("bytes_hex") or "")
        return str(dumped or "")

    def _owner_is_auto(self, master):
        if master is None or self.battle is None:
            return False
        auto_set = getattr(self.battle, "auto_battle_set", None) or set()
        if master in auto_set:
            return True
        want = self._avatar_key(master)
        if not want:
            return False
        for item in auto_set:
            if self._avatar_key(item) == want:
                return True
        return False

    def _master_for_avatar(self, avatar_id):
        """Return the live entity master object, not just Host.master bytes.

        auto_battle_set membership and real_change_auto_state compare by
        object identity. Host.master is utf-8 hex; entities often store
        ObjectId. Adding the Host bytes never matches need_input().
        """
        want = str(avatar_id or "")
        if not want:
            return None
        b = self.battle
        infos = getattr(b, "entity_infos", None) if b is not None else None
        if infos:
            for info in infos.values():
                candidates = []
                try:
                    candidates.append(info.get_attr("master"))
                except Exception:
                    pass
                try:
                    candidates.append(info.get_master())
                except Exception:
                    pass
                for master in candidates:
                    if master is not None and self._avatar_key(master) == want:
                        return master
        if self.master is not None and self._avatar_key(self.master) == want:
            return self.master
        if self.enemy_master is not None and self._avatar_key(self.enemy_master) == want:
            return self.enemy_master
        return None

    def set_auto(self, avatar_id, enabled):
        """Toggle one owner's native auto_battle_set. Does not wait the clock.

        Official real_change_auto_state thinks immediately when this owner
        currently has the input window. The opponent flag is untouched.
        """
        if self.battle is None:
            raise ValueError("battle not started")
        if self.failed:
            raise RuntimeError("native battle failed; discard worker")
        master = self._master_for_avatar(avatar_id)
        if master is None:
            raise ValueError("unknown auto owner")
        try:
            self.battle.real_change_auto_state(master, bool(enabled))
            # If entity master != Host.master bytes, the official
            # get_attr('master') == master check misses. Think anyway
            # while this owner still holds the input timer.
            if enabled:
                eid = getattr(self.battle, "current_input_eid", None)
                timer = getattr(self.battle, "master_input_timer", None)
                if eid and timer:
                    info = self.battle.get_entity(eid)
                    live = None
                    if info is not None:
                        try:
                            live = info.get_master()
                        except Exception:
                            live = None
                    if live is not None and self._avatar_key(live) == self._avatar_key(master):
                        self.battle.on_player_input(eid)
            self.capture_changes()
        except BaseException:
            self.failed = True
            raise
        return self.response()

    def consume_timeout(self):
        """Fire every virtual callback until native _timeout consumes input.

        The parent already waited wall-clock player_input_time. One
        clock.step() may only pop a buff/field tick; keep stepping so
        master_input / think actually runs and sync_command is emitted.
        """
        if self.battle is None:
            raise ValueError("battle not started")
        if self.failed:
            raise RuntimeError("native battle failed; discard worker")
        collected = []
        limit = 10000
        while self.result is None and limit > 0:
            if not self.snapshot().get("awaiting_player"):
                break
            self.advance()
            events, self.events = self.events, []
            collected.extend(events)
            limit -= 1
            if not self.snapshot().get("awaiting_player"):
                break
        if (limit <= 0 and self.result is None
                and self.snapshot().get("awaiting_player")):
            raise RuntimeError("native timeout did not consume the input window")
        response = self.response()
        response["events"] = collected + response["events"]
        return response

    def drive(self):
        """Step field ticks until one sync_command, a click window, or the result.

        Official real_start_action thinks one unit then delay_call_round_end(play).
        Yielding every command lets the parent wall-wait so clients can apply it
        before the next unit thinks. Bursting two commands in one drive() drops
        the second: do_command requires current_input_eid == caster.
        """
        if self.battle is None:
            raise ValueError("battle not started")
        if self.failed:
            raise RuntimeError("native battle failed; discard worker")
        limit = 10000
        while self.result is None and limit > 0:
            if self.snapshot().get("awaiting_player"):
                break
            self.advance()
            limit -= 1
            saw_command = False
            for event in self.events:
                if event.get("kind") == "sync_command":
                    saw_command = True
                    break
            if saw_command:
                break
        return self.response()

    def autoplay(self, max_steps):
        if self.battle is None:
            raise ValueError("battle not started")
        if not isinstance(max_steps, (int, long)) or not 1 <= max_steps <= 100000:
            raise ValueError("max_steps must be an integer from 1 to 100000")
        if not self.auto:
            raise ValueError("autoplay requires auto=True at start")
        for _ in xrange(int(max_steps)):
            if self.result is not None:
                break
            self.advance()
        value = self.response()
        value["budget_exhausted"] = self.result is None
        return value

    def install(self):
        __builtin__.gtext = lambda value: value
        world = imp.new_module("gworld")
        world.get_logger = logging.getLogger
        world.gen_object_id = lambda: ObjectId(self.new_id()[:24])
        world.id_mgr = IdManager()
        world.gen_uuid = lambda: self.new_id()[:32]
        world.is_server = lambda: True
        world.is_test_server = lambda: False
        world.is_dev_server = lambda: False
        world.check_delay_time = lambda: True
        world.get_battle = lambda: self.battle
        world.get_player = lambda eid=None: self.players.get(eid)
        world.get_entity = lambda eid: self.players.get(eid)
        world.gm_enable = lambda: False
        world.debug = lambda *args: logging.getLogger("native").debug("%r", args)
        def raise_callback_error():
            typ, value, tb = sys.exc_info()
            if value is None:
                raise RuntimeError("native callback failed without exception")
            raise typ, value, tb
        world.log_last_except = raise_callback_error
        world.story_mgr = None
        sys.modules["gworld"] = world
        sys.modules["data"] = NativeData()
        timer = imp.new_module("Timer")
        timer.addTimer = self.clock.add
        timer.delTimer = self.clock.cancel
        sys.modules["Timer"] = timer
        language = imp.new_module("utils.language_utils")
        language.LANGUAGE_TO_LANGUAGE_DATAS_PATH = {}
        language.get_current_language = lambda: "zh"
        sys.modules["utils.language_utils"] = language
        rpc_args = __import__("mbengine.common.RpcMethodArgs", fromlist=["RpcMethodArg"])
        world.rpc_arg_type = rpc_args.RpcMethodArg
        world.RpcMethodArgs = rpc_args
        world.rpcdecorator = __import__("mbengine.common.rpcdecorator", fromlist=["*"])
        from activity_buffs_native import _install_hs_activity_buffs
        _install_hs_activity_buffs()
