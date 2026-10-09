"""One native server_battle is the authority for a dungeon-21 match.

Clients send click RPCs. Auto flags stay owner-local and never drive the
engine. If the shadow cannot start, both sides lose with no score change.
If the input window elapses with no click, the native timeout command runs.
"""

from __future__ import annotations

import copy
import math
import threading
import time

from battle_native import NativeCommandRejected
from sync_pvp_coordinates import coordinate_transform
from sync_pvp_entities import build_native_eid_map as stable_entity_map


DEFAULT_INPUT_TIMEOUT = 20.0
SKILL_USED_TYPES = ("ck_monster", "ck_skill", "ck_head")
MIN_PLAY_WALL = 0.35
MAX_PLAY_WALL = 6.0
MAX_RESTORE_PAUSE = 120.0
CLIENT_PLAYBACK_TIMEOUT = 120.0


class ShadowStartError(RuntimeError):
    """Native shadow could not start. Both sides lose; score is unchanged."""


def _master_id(value):
    """Normalize native current_master to the 24-hex avatar id."""
    if isinstance(value, dict) and value.get("bytes_hex"):
        raw = str(value.get("bytes_hex") or "")
        try:
            return bytes.fromhex(raw).decode("utf-8")
        except (ValueError, UnicodeDecodeError):
            return raw
    if isinstance(value, (bytes, bytearray)):
        try:
            return bytes(value).decode("utf-8")
        except UnicodeDecodeError:
            return bytes(value).hex()
    return str(value or "")


def native_turn_owner(state, left_id, right_id):
    """Whose native turn it is: pinned left/right avatar, never the other board."""
    left_id = str(left_id or "")
    right_id = str(right_id or "")
    master = _master_id((state or {}).get("current_master"))
    if master and master == left_id:
        return left_id
    if master and master == right_id:
        return right_id
    current = str((state or {}).get("current_input_eid") or "")
    for unit in (state or {}).get("units") or ():
        if not isinstance(unit, dict) or str(unit.get("eid") or "") != current:
            continue
        side = _unit_camp(unit)
        if side == "left":
            return left_id
        if side == "right":
            return right_id
        break
    return ""


def native_command_from_click(command, blob, current_eid, eid_map=None):
    """Turn a client click RPC into a native {name, args} command.

    ``current_eid`` is the native unit whose turn it is. Explicit skill casters
    are preserved so native support can act during that unit's round. When
    ``eid_map`` is a dict, client eids must map onto a native unit; an unmapped
    string target aborts instead of hitting the other player's board.
    """
    if command not in ("move_to", "use_skill"):
        raise ValueError("pvp authority only accepts move_to/use_skill")
    if not current_eid:
        raise ValueError("native battle has no current unit")

    def mapped(value):
        if not isinstance(value, str) or eid_map is None:
            return value
        if not isinstance(eid_map, dict):
            raise ValueError("unmapped skill target:%s" % value)
        native = eid_map.get(value)
        if native:
            return native
        raise ValueError("unmapped skill target:%s" % value)

    args = list(blob or [])
    if args and args[0] in SKILL_USED_TYPES:
        args = args[1:]
    if command == "move_to":
        if not args:
            raise ValueError("move_to requires coordinates")
        return {"name": "move_to", "args": [current_eid, args[-1]]}
    if len(args) < 2:
        raise ValueError("use_skill requires skill_id and target")
    skill_id = args[1] if len(args) > 2 else args[0]
    target = mapped(args[2] if len(args) > 2 else args[-1])
    caster = mapped(args[0]) if len(args) > 2 else current_eid
    return {"name": "use_skill", "args": [caster, skill_id, target]}


def events_have_sync_command(update):
    """True when this native tick emitted a command clients must apply first."""
    events = (update or {}).get("events") if isinstance(update, dict) else None
    if not isinstance(events, list):
        return False
    for event in events:
        if isinstance(event, dict) and event.get("kind") == "sync_command":
            return True
    return False


def _unit_camp(unit):
    kind = unit.get("kind")
    if kind in ("field",) or unit.get("native_type") == 4:
        return "field"
    if kind in ("hero", "ally"):
        return "left"
    if kind in ("monster", "enemy"):
        return "right"
    try:
        camp = int(unit.get("camp") or 0)
    except (TypeError, ValueError):
        camp = 0
    if camp == 1:
        return "left"
    if camp == 2:
        return "right"
    return "field"


def _owned_native_support(state, caster_eid, owner, left_id, right_id):
    """Only native support flags grant the original non-current-caster rule."""
    side = "left" if owner == left_id else "right" if owner == right_id else None
    for unit in (state or {}).get("units") or ():
        if not isinstance(unit, dict) or str(unit.get("eid") or "") != caster_eid:
            continue
        attrs = unit.get("attrs") if isinstance(unit.get("attrs"), dict) else {}
        return (side is not None and _unit_camp(unit) == side
                and bool(unit.get("kind") == "support" or attrs.get("support")
                         or attrs.get("extra_support_skill")))
    return False


def build_native_eid_map(native_units, client_units, *, client_is_left,
                         existing_map=None, coordinate_frame=None):
    """Bind entities by stable identity; ambiguous entities stay unmapped."""
    return stable_entity_map(native_units, client_units, client_is_left=client_is_left,
                             existing_map=existing_map, coordinate_frame=coordinate_frame)


def client_to_native_eids(native_units, client_units, *, client_is_left):
    """Invert ``build_native_eid_map`` for one player's click RPCs."""
    native_to_client = build_native_eid_map(
        native_units, client_units, client_is_left=client_is_left)
    return {client: native for native, client in native_to_client.items()}


class PvpShadowAuthority:
    """Drive one NativeBattleProcess for both players."""

    def __init__(self, *, match_uuid, left_id, right_id, metadata, roster,
                 process_factory, seed=1, input_timeout=None,
                 on_events=None, on_result=None, on_abort=None, playback_ready=None,
                 on_input_ready=None):
        self.match_uuid = match_uuid
        self.left_id = str(left_id)
        self.right_id = str(right_id)
        self.metadata = dict(metadata or {})
        self.metadata["enemy_auto"] = False
        self.roster = list(roster or [])
        self.seed = int(seed or 1)
        self.process_factory = process_factory
        self.input_timeout = None if input_timeout is None else float(input_timeout)
        self.on_events = on_events
        self.on_result = on_result
        self.on_abort = on_abort
        self.on_input_ready = on_input_ready
        # Production advances only after both bound clients reach the next
        # native continuation. Fixtures without clients keep virtual pacing.
        self.playback_ready = playback_ready
        self.playback_wait_needed = False
        self.playback_wait_deadline = None
        self.lock = threading.RLock()
        self.command_event = threading.Event()
        self.pending_command = None
        self.pending_auto = {}
        self.coordinate_transforms = {}
        self.entity_maps = {}
        self.process = None
        self.thread = None
        self.closed = False
        self.stopping = threading.Event()
        self.state = {}
        self.steps = []
        self.history = []
        self.update_sequence = 0
        self.input_deadline = None
        self.input_window = None
        self.support_input_window = None
        self.play_deadline = None
        self.play_wall_min = MIN_PLAY_WALL
        self.play_wall_max = MAX_PLAY_WALL
        self.need_play_wall = False
        self.last_update = {}
        self.restore_condition = threading.Condition(self.lock)
        self.restore_pauses = {}
        self.restore_paused_at = None

    def _open(self):
        if self.process_factory is None:
            raise ShadowStartError("无法启动影子战斗")
        return self.process_factory()

    def start(self):
        """Boot the native engine. Raises ShadowStartError on any failure."""
        if self.closed:
            raise ShadowStartError("无法启动影子战斗")
        try:
            self.process = self._open()
            if self.process is None:
                raise ShadowStartError("无法启动影子战斗")
            start = getattr(self.process, "start", None)
            if not callable(start):
                raise ShadowStartError("无法启动影子战斗")
            update = start(self.metadata, self.roster, self.seed, False)
            terminal = self._apply_update(update or {})
            drive = getattr(self.process, "drive", None)
            if callable(drive) and terminal is None and not self.state.get("awaiting_player"):
                update = drive()
                self._apply_update(update or {})
        except ShadowStartError:
            self.close()
            raise
        except Exception as error:
            self.close()
            message = str(error).strip() or "无法启动影子战斗"
            raise ShadowStartError(message) from error
        if self.closed and (self.last_update or {}).get("result") is None:
            raise ShadowStartError("无法启动影子战斗")
        return self.state

    def run(self):
        if self.closed or self.process is None:
            raise ShadowStartError("无法启动影子战斗")
        self.thread = threading.Thread(
            target=self._pump, name="pvp-shadow-%s" % self.match_uuid[:8],
            daemon=True)
        self.thread.start()
        return self.thread

    def submit_command(self, avatar_id, command, blob, client_units=None,
                       client_is_left=None):
        """Queue one click. Auto/timeout RPCs never call this."""
        if command not in ("move_to", "use_skill"):
            return False
        with self.lock:
            if self.closed or self.stopping.is_set():
                return False
            if self.restore_pauses:
                return False
            if not self.state.get("awaiting_player"):
                return False
            if self.pending_command is not None:
                return False
            owner = native_turn_owner(self.state, self.left_id, self.right_id)
            if owner != str(avatar_id):
                return False
            if self.input_deadline is not None and time.monotonic() >= self.input_deadline:
                return False
            if client_is_left is not None and bool(client_is_left) != (str(avatar_id) == self.left_id):
                return False
            try:
                mapping = {client: native for native, client in
                           self.map_client_entities(avatar_id, client_units or []).items()}
                raw = list(blob or [])
                if raw and raw[0] in SKILL_USED_TYPES:
                    raw = raw[1:]
                if len(raw) < (2 if command == "move_to" else 3):
                    return False
                caster = mapping.get(str(raw[0]))
                if (caster != str(self.state.get("current_input_eid"))
                        and (command != "use_skill" or not _owned_native_support(
                            self.state, caster, owner, self.left_id, self.right_id))):
                    return False
                native = native_command_from_click(command, blob, self.state.get("current_input_eid"), mapping)
                if command == "use_skill" and (type(native["args"][1]) is not int or native["args"][1] <= 0):
                    return False
                target = native["args"][-1]
                if command == "move_to" or isinstance(target, (list, tuple)):
                    if (not isinstance(target, (list, tuple)) or len(target) != 3
                            or any(type(value) is not int for value in target) or sum(target) != 0):
                        return False
                elif not isinstance(target, str):
                    return False
            except (TypeError, ValueError, IndexError):
                return False
            self.pending_command = (
                str(avatar_id), command, list(blob or []),
                copy.deepcopy(client_units or []), client_is_left, self.input_window,
            )
            self.command_event.set()
            return True

    def set_auto(self, avatar_id, enabled):
        """Owner-local auto: native thinks on that master's turns. No flag sync."""
        with self.lock:
            if self.closed or self.stopping.is_set():
                return False
            if str(avatar_id) not in (self.left_id, self.right_id):
                return False
            self.pending_auto[str(avatar_id)] = bool(enabled)
            self.command_event.set()
            return True

    def bind_client_coordinates(self, avatar_id, client_units):
        """Pin one board frame before any player movement; never refit it."""
        owner = str(avatar_id)
        with self.lock:
            if owner not in (self.left_id, self.right_id):
                raise ValueError("coordinate frame owner is not a participant")
            if owner not in self.coordinate_transforms:
                mapping = build_native_eid_map(self.state.get("units") or [], client_units,
                                              client_is_left=(owner == self.left_id))
                self.coordinate_transforms[owner] = coordinate_transform(
                    self.state.get("units") or [], client_units, mapping)
            return self.coordinate_transforms[owner]

    def map_client_entities(self, avatar_id, client_units, native_units=None):
        """Preserve bindings and resolve summons using creator and birth cell."""
        owner = str(avatar_id)
        with self.lock:
            if owner not in (self.left_id, self.right_id):
                raise ValueError("entity map owner is not a participant")
            if native_units is None:
                native_units = self.state.get("units") or []
            cached = self.entity_maps.setdefault(owner, {})
            mapping = build_native_eid_map(
                native_units, client_units, client_is_left=(owner == self.left_id),
                existing_map=cached, coordinate_frame=self.coordinate_transforms.get(owner))
            cached.update(mapping)
            return mapping

    def pause_for_restore(self, avatar_id, timeout=MAX_RESTORE_PAUSE):
        """Request a bounded replay pause without waiting for native work.

        The pump acknowledges only after its in-flight update was broadcast.
        A repeated owner request keeps its original expiry. Callers must release
        application locks before wait_restore_paused because on_events may need
        those locks to finish the update which precedes the pause boundary.
        """
        owner = str(avatar_id)
        try:
            timeout = float(timeout)
        except (TypeError, ValueError):
            return False
        if not math.isfinite(timeout) or timeout <= 0:
            return False
        with self.restore_condition:
            if self.closed or self.stopping.is_set() or owner not in (self.left_id, self.right_id):
                return False
            now = time.monotonic()
            if owner in self.restore_pauses and now >= self.restore_pauses[owner]:
                self.command_event.set()
                self.restore_condition.notify_all()
                return False
            self.restore_pauses.setdefault(owner, now + min(timeout, MAX_RESTORE_PAUSE))
            self.command_event.set()
            self.restore_condition.notify_all()
            return True

    def wait_restore_paused(self, timeout=30.0):
        """Wait for a stable, fully broadcast replay history; close interrupts."""
        try:
            timeout = float(timeout)
        except (TypeError, ValueError):
            return False
        if not math.isfinite(timeout) or timeout < 0:
            return False
        deadline = time.monotonic() + timeout
        with self.restore_condition:
            while not self.closed and not self.stopping.is_set():
                if not self.restore_pauses:
                    return False
                if self.restore_paused_at is not None:
                    return True
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    return False
                self.restore_condition.wait(remaining)
        return False

    def resume_after_restore(self, avatar_id):
        """Release one owner; True means the last live pause was released."""
        with self.restore_condition:
            owner = str(avatar_id)
            if owner not in self.restore_pauses:
                return False
            if time.monotonic() >= self.restore_pauses[owner]:
                # Leave the expired owner visible for the pump's failure path.
                # A late replay completion must not revive an expired pause.
                self.command_event.set()
                self.restore_condition.notify_all()
                return False
            self.restore_pauses.pop(owner)
            released = not self.restore_pauses and not self.closed and not self.stopping.is_set()
            if released and self.restore_paused_at is not None:
                elapsed = max(0.0, time.monotonic() - self.restore_paused_at)
                if self.input_deadline is not None:
                    self.input_deadline += elapsed
                if self.play_deadline is not None:
                    self.play_deadline += elapsed
                if self.playback_wait_deadline is not None:
                    self.playback_wait_deadline += elapsed
                self.restore_paused_at = None
            self.command_event.set()
            self.restore_condition.notify_all()
            return released

    def _restore_requested(self):
        with self.lock:
            return bool(self.restore_pauses)

    def _pause_restore_boundary(self):
        """Pump-only safe boundary, after native callbacks and on_events finish."""
        expired = False
        with self.restore_condition:
            while not self.closed and not self.stopping.is_set():
                if not self.restore_pauses:
                    return True
                now = time.monotonic()
                if self.restore_paused_at is None:
                    self.restore_paused_at = now
                    self.restore_condition.notify_all()
                deadline = min(self.restore_pauses.values())
                if now >= deadline:
                    expired = True
                    break
                self.restore_condition.wait(deadline - now)
        if expired:
            self._fail("PVP 重连恢复超时")
        return False

    def close(self):
        with self.restore_condition:
            self.stopping.set()
            self.closed = True
            self.command_event.set()
            self.restore_condition.notify_all()
            process, self.process = self.process, None
        if process is not None:
            closer = getattr(process, "close", None)
            if callable(closer):
                try:
                    closer()
                except Exception:
                    pass
        thread = self.thread
        if thread is not None and thread is not threading.current_thread():
            thread.join(2.0)

    def _apply_update(self, update):
        update = update if isinstance(update, dict) else {}
        events = update.get("events") if isinstance(update.get("events"), list) else []
        with self.lock:
            self.last_update = copy.deepcopy(update)
            self.state = copy.deepcopy(update.get("state")) if isinstance(update.get("state"), dict) else {}
            self.update_sequence += 1
            self.state["pvp_sequence"] = self.update_sequence
            window = (native_turn_owner(self.state, self.left_id, self.right_id),
                      str(self.state.get("current_input_eid") or ""),
                      self.state.get("action_counter")) if self.state.get("awaiting_player") else None
            # Support temporarily clears the native input while its plays run,
            # then real_continue_old_round returns to the same action. Keep the
            # original wall deadline: animations do not buy another 20 seconds.
            for event in events:
                if not isinstance(event, dict):
                    continue
                data = event.get("data") if isinstance(event.get("data"), dict) else {}
                args = data.get("args")
                raw = args[2] if isinstance(args, (list, tuple)) and len(args) >= 3 else None
                if (event.get("kind") == "sync_command" and isinstance(raw, (list, tuple))
                        and len(raw) == 2 and raw[0] in ("use_support_skill", "use_extra_support_skill")
                        and self.input_window is not None
                        and str(data.get("round_eid") or "") == self.input_window[1]
                        and self.state.get("action_counter") == self.input_window[2]):
                    self.support_input_window = self.input_window
            if self.support_input_window is not None:
                interrupted = self.support_input_window
                same_round = (str(self.state.get("round_eid") or "") == interrupted[1]
                              and self.state.get("action_counter") == interrupted[2])
                if window == interrupted:
                    self.support_input_window = None
                elif window is None and same_round and update.get("result") is None:
                    window = interrupted
                    self.pending_command = None
                else:
                    self.support_input_window = None
            if window != self.input_window:
                self.input_window = window
                self.input_deadline = (time.monotonic() + self._timeout_seconds()) if window else None
                self.pending_command = None
            self.history.append({"events": copy.deepcopy(events), "state": copy.deepcopy(self.state)})
            if events_have_sync_command(update):
                self.need_play_wall = True
                self.play_deadline = max(self.play_deadline or 0.0,
                                         time.monotonic() + self._play_wall(update))
                if callable(self.playback_ready):
                    self.playback_wait_needed = True
                    self.playback_wait_deadline = time.monotonic() + CLIENT_PLAYBACK_TIMEOUT
        if events and callable(self.on_events):
            self.on_events(events, self.state)
        if (update.get("result") is None and self.state.get("awaiting_player")
                and callable(self.on_input_ready)):
            # A client can reach its next manual UI before the slow peer.
            # Grant controls only once the native authority really owns an
            # input window. Keep this callback outside the authority lock.
            self.on_input_ready()
        result = update.get("result")
        if result is not None:
            try:
                if callable(self.on_result):
                    self.on_result(result)
            finally:
                self.close()
            return result
        return None

    def _timeout_seconds(self):
        if self.input_timeout is not None:
            return max(0.0, self.input_timeout) if math.isfinite(self.input_timeout) else DEFAULT_INPUT_TIMEOUT
        value = self.state.get("player_input_time")
        try:
            value = float(value)
        except (TypeError, ValueError):
            value = DEFAULT_INPUT_TIMEOUT
        if not math.isfinite(value) or value <= 0:
            value = DEFAULT_INPUT_TIMEOUT
        return value

    def remaining_wait(self):
        with self.lock:
            deadline = self.input_deadline
            now = self.restore_paused_at if self.restore_paused_at is not None else time.monotonic()
            if deadline is None:
                if self.state.get("awaiting_player"):
                    return self._timeout_seconds()
                return 0.0
            return max(0.0, deadline - now)

    def restore_payload(self):
        with self.lock:
            return {"state": copy.deepcopy(self.state), "history": copy.deepcopy(self.history),
                    "remaining_wait": self.remaining_wait(),
                    "awaiting_player": bool(self.state.get("awaiting_player")),
                    "current_input_eid": self.state.get("current_input_eid")}

    def input_status(self):
        """Current legal input only; granting UI never copies replay history."""
        with self.lock:
            return {"state": copy.deepcopy(self.state), "window": self.input_window,
                    "remaining_wait": self.remaining_wait(),
                    "paused": bool(self.restore_pauses), "closed": self.closed}

    def _take_pending(self):
        with self.lock:
            pending = self.pending_command
            self.pending_command = None
            if not self.pending_auto:
                self.command_event.clear()
            return pending

    def _take_pending_auto(self):
        with self.lock:
            pending = None
            if self.pending_auto:
                avatar_id = next(iter(self.pending_auto))
                pending = (avatar_id, self.pending_auto.pop(avatar_id))
            if self.pending_command is None and not self.pending_auto:
                self.command_event.clear()
            return pending

    def _play_wall(self, update):
        """Legacy pacing estimate; next_delay is the earliest internal timer.

        It cannot confirm a client's full animation or continuation. Production
        uses the ordered client boundary feedback instead of this estimate.
        """
        if not events_have_sync_command(update):
            return 0.0
        delay = None
        state = (update or {}).get("state") if isinstance(update, dict) else None
        if isinstance(state, dict):
            delay = state.get("next_delay")
        try:
            delay = float(delay)
        except (TypeError, ValueError):
            delay = self.play_wall_min
        if not math.isfinite(delay) or delay <= 0:
            delay = self.play_wall_min
        return max(self.play_wall_min, min(self.play_wall_max, delay))

    def _wake_queued(self):
        with self.lock:
            if self.pending_command is not None or self.pending_auto:
                self.command_event.set()

    def _wait_interruptible(self, seconds):
        """Wait play wall. Apply pending_auto. Leave pending_command queued."""
        deadline = time.monotonic() + max(0.0, float(seconds or 0.0))
        while not self.stopping.is_set() and not self.closed:
            if self._restore_requested():
                return "paused"
            leftover = deadline - time.monotonic()
            if leftover <= 0:
                return None
            self.command_event.wait(leftover)
            if self.stopping.is_set() or self.closed:
                return None
            if self._restore_requested():
                return "paused"
            pending_auto = self._take_pending_auto()
            if pending_auto is not None:
                update = self._apply_auto(pending_auto[0], pending_auto[1])
                if self._apply_update(update or {}) is not None:
                    return "result"
                extra = self._play_wall(update)
                if extra > 0:
                    deadline = max(deadline, time.monotonic() + extra)
                continue
            with self.lock:
                has_click = self.pending_command is not None
                self.command_event.clear()
            leftover = deadline - time.monotonic()
            if leftover > 0 and has_click:
                # Keep this wait interruptible by both settings and restores.
                # The queued click stays in place until the play wall ends.
                continue
        return None

    def _run_play_wall(self):
        wall = max(0.0, (self.play_deadline or time.monotonic()) - time.monotonic())
        self.need_play_wall = False
        if wall <= 0:
            return None
        result = self._wait_interruptible(wall)
        if result == "paused":
            self.need_play_wall = True
        self._wake_queued()
        return result

    def notify_playback(self):
        """Wake feedback waiting without consuming a pending click or setting."""
        self.command_event.set()

    def _wait_client_playback(self):
        # Never call the gateway while holding authority.lock: the gateway
        # callback takes ACCOUNT_LOCK, and inbound RPCs take those locks in
        # the opposite order. Closing and restore requests interrupt the wait.
        while not self.stopping.is_set() and not self.closed:
            if self._restore_requested():
                return "paused"
            self.command_event.clear()
            remaining = (self.playback_wait_deadline or time.monotonic()) - time.monotonic()
            if remaining <= 0:
                self._fail("双方战斗播放进度确认超时", reason="playback_progress_timeout")
                return "closed"
            if self.playback_ready():
                with self.lock:
                    self.playback_wait_needed = False
                    self.playback_wait_deadline = None
                    self.need_play_wall = False
                    self.play_deadline = None
                self._wake_queued()
                return None
            self.command_event.wait(min(remaining, 0.5))
        return "closed"

    def _pump(self):
        try:
            while not self.stopping.is_set() and not self.closed:
                if not self._pause_restore_boundary():
                    return
                if self.playback_wait_needed:
                    progress = self._wait_client_playback()
                    if progress == "paused":
                        continue
                    if progress == "closed":
                        return
                pending_auto = self._take_pending_auto()
                if pending_auto is not None:
                    update = self._apply_auto(*pending_auto)
                    if self._apply_update(update or {}) is not None:
                        return
                    continue
                if self.need_play_wall:
                    if self._run_play_wall() == "result":
                        return
                    continue
                if not self.state.get("awaiting_player"):
                    drive = getattr(self.process, "drive", None)
                    if not callable(drive):
                        self._fail("影子战斗无法继续")
                        return
                    update = drive()
                    if self._apply_update(update or {}) is not None:
                        return
                    if self.stopping.is_set() or self.closed:
                        return
                    if self.need_play_wall or self.state.get("awaiting_player"):
                        continue
                    self._fail("影子战斗无法继续")
                    return
                timeout = self._timeout_seconds()
                with self.lock:
                    now = time.monotonic()
                    if self.input_deadline is None:
                        self.input_deadline = now + timeout
                    wait = max(0.0, self.input_deadline - now)
                self.command_event.wait(wait)
                if self.stopping.is_set() or self.closed:
                    return
                if self._restore_requested():
                    continue
                pending_auto = self._take_pending_auto()
                if pending_auto is not None:
                    update = self._apply_auto(pending_auto[0], pending_auto[1])
                    if self._apply_update(update or {}) is not None:
                        return
                    if not self.state.get("awaiting_player") and self.support_input_window is None:
                        with self.lock:
                            self.input_deadline = None
                            self.pending_command = None
                    continue
                pending = self._take_pending()
                if self.stopping.is_set() or self.closed:
                    return
                if pending is None:
                    with self.lock:
                        # Playback observations share this wakeup with clicks
                        # and settings. A wakeup is not an input timeout: keep
                        # the original window/deadline until it actually expires.
                        if (self.input_deadline is not None
                                and time.monotonic() < self.input_deadline):
                            continue
                update = self._step_native(pending)
                if self._apply_update(update or {}) is not None:
                    return
        except Exception as error:
            self._fail(str(error).strip() or "影子战斗无法继续")

    def _apply_auto(self, avatar_id, enabled):
        setter = getattr(self.process, "set_auto", None)
        if not callable(setter):
            return {"state": dict(self.state or {}), "events": []}
        self.steps.append({"auto": str(avatar_id), "enabled": bool(enabled)})
        return setter(str(avatar_id), bool(enabled))

    def _drain_timeout(self):
        """Run native `_timeout` even when other virtual callbacks sit in front."""
        consume = getattr(self.process, "timeout", None)
        if callable(consume):
            return consume()
        collected = []
        update = None
        limit = 10000
        while limit > 0:
            update = self.process.step(None)
            limit -= 1
            events = (update or {}).get("events") if isinstance(update, dict) else None
            if isinstance(events, list):
                collected.extend(events)
            if not isinstance(update, dict):
                break
            if update.get("result") is not None:
                break
            state = update.get("state") if isinstance(update.get("state"), dict) else {}
            if not state.get("awaiting_player"):
                break
        if update is None:
            raise ShadowStartError("影子战斗无法继续")
        if (limit <= 0 and isinstance(update, dict)
                and ((update.get("state") or {}).get("awaiting_player"))
                and update.get("result") is None):
            raise RuntimeError("native timeout did not consume the input window")
        update = dict(update)
        update["events"] = collected
        return update

    def _step_native(self, pending):
        if self.process is None:
            raise ShadowStartError("影子战斗无法继续")
        if pending is None:
            self.steps.append(None)
            return self._drain_timeout()
        avatar_id = str(pending[0])
        command = pending[1]
        blob = pending[2]
        client_units = pending[3] if len(pending) > 3 else None
        client_is_left = pending[4] if len(pending) > 4 else None
        if len(pending) > 5 and pending[5] != self.input_window:
            return {"state": copy.deepcopy(self.state), "events": []}
        owner = native_turn_owner(self.state, self.left_id, self.right_id)
        # One native battle: left = master, right = enemy_master. A click from
        # the other owner is not applied onto the current unit.
        if not owner:
            raise ValueError("native battle turn owner unknown")
        if owner != avatar_id:
            return {"state": dict(self.state or {}), "events": []}
        if client_is_left is None:
            client_is_left = avatar_id == self.left_id
        if bool(client_is_left) != (avatar_id == self.left_id):
            raise ValueError("click side does not match battle owner")
        eid_map = {client: native for native, client in
                   self.map_client_entities(avatar_id, client_units or []).items()}
        current = self.state.get("current_input_eid")
        native = native_command_from_click(command, blob, current, eid_map)
        if command == "move_to" or isinstance(native["args"][-1], (list, tuple)):
            frame = self.bind_client_coordinates(avatar_id, client_units)
            native["args"][-1] = frame.client_to_native(native["args"][-1])
        self.steps.append(native)
        try:
            return self.process.step(native)
        except NativeCommandRejected as error:
            # Native validation rejected the input before executing it. Keep
            # this exact window and its deadline; a bad click is recoverable.
            return {"state": copy.deepcopy(self.state), "events": [{
                "kind": "input_rejected", "data": {
                    "owner": avatar_id, "native_eid": str(current),
                    "remaining_wait": self.remaining_wait(), "message": str(error)[:500],
                }}]}

    def _fail(self, message, reason="shadow_failed"):
        if self.closed:
            return
        self.close()
        if callable(self.on_abort):
            self.on_abort(reason, message)
