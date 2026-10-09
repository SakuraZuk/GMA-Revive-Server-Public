"""Bind battle entities by stable identity, independently of snapshot order.

Initial fighters use unique camp/role/type/category/field identities. Summons
also require a mapped creator, the creating skill and the original spawn cell
in the already pinned board frame. Current positions and numeric EID order
never establish a binding. Keep the returned map on the client's match and
pass it back for later snapshots.
"""

from __future__ import annotations

from collections import Counter
from collections.abc import Mapping
from dataclasses import dataclass


def _integer(value):
    if type(value) is int:
        return value
    if isinstance(value, str):
        try:
            return int(value)
        except ValueError:
            pass
    return None


def _eid(value):
    if isinstance(value, str) and value:
        return value
    if type(value) is int:
        return str(value)
    return None


def _cube(value):
    if (isinstance(value, (list, tuple)) and len(value) == 3
            and all(type(component) is int for component in value)
            and sum(value) == 0):
        return tuple(value)
    return None


def _flag(value):
    if type(value) is bool:
        return value
    if type(value) is int and value in (0, 1):
        return bool(value)
    return None


@dataclass(frozen=True, slots=True)
class _Unit:
    eid: str
    camp: int
    role: int
    category: str
    native_type: int | None
    mf_id: int
    summon: bool
    creator: str | None
    skill: int | None
    origin: tuple[int, int, int] | None


def _read_unit(raw, *, flip_camp):
    if not isinstance(raw, Mapping):
        return None
    attrs = raw.get("attrs")
    if not isinstance(attrs, Mapping):
        attrs = {}

    def value(key, default=None):
        return raw.get(key, attrs.get(key, default))

    eid = _eid(raw.get("eid"))
    role = _integer(raw.get("role"))
    native_type = value("native_type")
    if native_type is not None:
        native_type = _integer(native_type)
        if native_type is None:
            return None
    kind = raw.get("kind")
    support = value("is_support", value("support", False))
    support = _flag(support)
    if support is None:
        return None
    category = ("field" if kind == "field" or native_type == 4 else
                "support" if support or kind == "support" else "fighter")
    camp = value("camp", attrs.get("camp_id"))
    if camp is None:
        camp = {"ally": 1, "hero": 1, "enemy": 2, "monster": 2,
                "field": 0}.get(kind)
    else:
        camp = _integer(camp)
    allowed_camps = (0, 1, 2, 3) if category == "field" else (1, 2)
    if (eid is None or role is None or role < 0 or camp not in allowed_camps
            or category != "field" and camp == 0):
        return None
    if flip_camp and camp in (1, 2):
        camp = 3 - camp
    mf_id = value("mf_id", 0)
    mf_id = 0 if mf_id is None else _integer(mf_id)
    if mf_id is None or mf_id < 0:
        return None

    creator = _eid(value("create_entity_eid"))
    skill = _integer(value("create_skill_id"))
    origin = _cube(value("origin_coord"))
    declared_summon = value("is_summon")
    if declared_summon is None:
        # Incomplete spawn metadata must not silently become an initial unit.
        summon = any(value(key) is not None for key in
                     ("create_entity_eid", "create_skill_id", "origin_coord"))
    else:
        summon = _flag(declared_summon)
        if summon is None:
            return None
    return _Unit(eid, camp, role, category, native_type, mf_id, summon,
                 creator, skill, origin)


def _index(units, *, flip_camp=False):
    indexed = {}
    duplicates = set()
    seen = set()
    for raw in units or ():
        eid = _eid(raw.get("eid")) if isinstance(raw, Mapping) else None
        if eid is not None:
            if eid in seen:
                duplicates.add(eid)
            seen.add(eid)
        unit = _read_unit(raw, flip_camp=flip_camp)
        if unit is None:
            continue
        indexed[unit.eid] = unit
    return {eid: unit for eid, unit in indexed.items() if eid not in duplicates}


def _same_base(native, client):
    return (native.camp == client.camp and native.role == client.role
            and native.category == client.category and native.mf_id == client.mf_id
            and native.summon == client.summon
            and (native.native_type is None or client.native_type is None
                 or native.native_type == client.native_type))


def _spawn_key(unit, *, creator, origin):
    if (not unit.summon or unit.native_type is None or creator is None
            or unit.skill is None or unit.skill <= 0 or origin is None):
        return None
    return (unit.camp, unit.role, unit.category, unit.native_type, unit.mf_id,
            creator, unit.skill, origin)


def build_native_eid_map(native_units, client_units, *, client_is_left,
                         existing_map=None, coordinate_frame=None):
    """Return a partial, injective native-EID -> client-EID mapping.

    ``existing_map`` is a previously proven binding for this client/scene;
    the input is never mutated. Only present entities with compatible stable
    identities retain cached bindings. An initial identity must be unique on
    both sides among unbound entities. A summon requires both snapshots to
    include ``is_summon``, ``native_type``, ``create_entity_eid``,
    ``create_skill_id`` and ``origin_coord``. Its creator must already be
    bound, and ``coordinate_frame.client_to_native(origin_coord)`` must agree
    with the native origin. Parents can themselves be summons.

    Missing snapshots, incomplete identities, duplicate EIDs, cyclic creator
    dependencies and ambiguous identities stay unmapped. The caller decides
    how long to wait for another client snapshot; this pure helper never waits.
    """
    native = _index(native_units)
    client = _index(client_units, flip_camp=not client_is_left)
    mapping = {}
    used_clients = set()
    cached = {}
    if isinstance(existing_map, Mapping):
        normalized = {_eid(n): _eid(c) for n, c in existing_map.items()}
        counts = Counter(normalized.values())
        for native_eid, client_eid in normalized.items():
            if (native_eid in native and client_eid in client
                    and counts[client_eid] == 1
                    and _same_base(native[native_eid], client[client_eid])):
                cached[native_eid] = client_eid

    def bind(native_eid, client_eid):
        mapping[native_eid] = client_eid
        used_clients.add(client_eid)

    # Cached fighters do not depend on their current cell or dictionary order.
    for native_eid, client_eid in cached.items():
        if not native[native_eid].summon:
            bind(native_eid, client_eid)

    def bind_unique(candidates):
        client_counts = Counter(client_eid for matches in candidates.values()
                                for client_eid in matches)
        additions = [(native_eid, matches[0]) for native_eid, matches in candidates.items()
                     if len(matches) == 1 and client_counts[matches[0]] == 1]
        for native_eid, client_eid in additions:
            bind(native_eid, client_eid)
        return bool(additions)

    candidates = {}
    for native_eid, unit in native.items():
        if native_eid not in mapping and not unit.summon:
            candidates[native_eid] = [client_eid for client_eid, observed in client.items()
                                     if client_eid not in used_clients
                                     and _same_base(unit, observed)]
    bind_unique(candidates)

    client_origins = {}
    transform = getattr(coordinate_frame, "client_to_native", None)
    if callable(transform):
        for client_eid, unit in client.items():
            if unit.summon and unit.origin is not None:
                try:
                    client_origins[client_eid] = _cube(transform(unit.origin))
                except (TypeError, ValueError, ArithmeticError):
                    pass

    def native_key(unit):
        return _spawn_key(unit, creator=mapping.get(unit.creator), origin=unit.origin)

    def client_key(unit):
        return _spawn_key(unit, creator=unit.creator,
                          origin=client_origins.get(unit.eid))

    # Each pass resolves at least one generation. No speculative cycle break.
    while True:
        changed = False
        for native_eid, client_eid in cached.items():
            if (native_eid not in mapping and client_eid not in used_clients
                    and native[native_eid].summon):
                key = native_key(native[native_eid])
                if key is not None and key == client_key(client[client_eid]):
                    bind(native_eid, client_eid)
                    changed = True
        candidates = {}
        for native_eid, unit in native.items():
            if native_eid in mapping or not unit.summon:
                continue
            key = native_key(unit)
            if key is not None:
                candidates[native_eid] = [client_eid for client_eid, observed in client.items()
                                         if client_eid not in used_clients
                                         and key == client_key(observed)]
        changed = bind_unique(candidates) or changed
        if not changed:
            break
    return mapping
