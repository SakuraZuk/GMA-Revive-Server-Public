"""Bounded gateway stages for a real two-player match (monotonic time).

Selection uses the shipped 22-second rule plus delivery grace. Scene/bridge
loading and native startup have separate local budgets: neither a duplicate
RPC nor loading-percent traffic extends them. Combat input is timed by the
native authority, never by this watchdog.
"""

import time


SELECTION_GRACE = 5.0
LOAD_TIMEOUT = 60.0
START_TIMEOUT = 90.0  # boot, start and drive each have a 30-second IPC limit
RECONNECT_TIMEOUT = 120.0


class MatchLifecycle:
    TRANSITIONS = {
        "selecting": {"loading", "terminal"},
        "loading": {"starting", "terminal"},
        "starting": {"fighting", "terminal"},
        "fighting": {"terminal"},
        "terminal": set(),
    }

    def __init__(self, selection_seconds=22, *, clock=time.monotonic):
        self.clock = clock
        self.stage = "selecting"
        self.deadline = clock() + float(selection_seconds) + SELECTION_GRACE
        self.disconnected = {}

    def advance(self, stage):
        if stage == self.stage or self.stage == "terminal":
            return False
        if stage not in self.TRANSITIONS[self.stage]:
            raise ValueError("invalid PvP stage: %s -> %s" % (self.stage, stage))
        self.stage = stage
        budget = {"loading": LOAD_TIMEOUT, "starting": START_TIMEOUT}.get(stage)
        self.deadline = None if budget is None else self.clock() + budget
        if stage == "terminal":
            self.disconnected.clear()
        return True

    def disconnect(self, avatar):
        if self.stage != "terminal":
            self.disconnected.setdefault(str(avatar), self.clock() + RECONNECT_TIMEOUT)

    def reconnect(self, avatar):
        self.disconnected.pop(str(avatar), None)

    def expired(self, now=None):
        if self.stage == "terminal":
            return None
        now = self.clock() if now is None else now
        if self.deadline is not None and now >= self.deadline:
            return "%s_timeout" % self.stage
        if any(now >= deadline for deadline in self.disconnected.values()):
            return "reconnect_timeout"
        return None
