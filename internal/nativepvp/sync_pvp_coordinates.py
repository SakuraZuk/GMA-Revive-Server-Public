"""Derive one client's arena coordinate frame from matched initial units.

Entity IDs are paired by the caller's camp/role mapping. Ordinary fighting
units must agree on either a translation or a reflected translation. Field
spawns and support slots have separate placement rules and are not anchors.
Store the returned immutable transform for this client's match/scene; do not
derive a new board frame from units that have already moved independently.
"""

from __future__ import annotations

from dataclasses import dataclass
from collections.abc import Mapping


class CoordinateMappingError(ValueError):
    """The observed unit pairs do not establish one arena coordinate frame."""


def _cube(value):
    if (not isinstance(value, (list, tuple)) or len(value) != 3
            or any(type(item) is not int for item in value) or sum(value) != 0):
        raise CoordinateMappingError("coordinate must be three integers summing to zero")
    return tuple(value)


@dataclass(frozen=True, slots=True)
class CoordinateTransform:
    """native = sign * client + offset, established by initial fighting units."""

    sign: int
    offset: tuple[int, int, int]
    anchor_count: int

    def client_to_native(self, coord):
        cube = _cube(coord)
        return [self.sign * item + delta for item, delta in zip(cube, self.offset)]

    def native_to_client(self, coord):
        cube = _cube(coord)
        return [self.sign * (item - delta) for item, delta in zip(cube, self.offset)]


def _units_by_eid(units):
    indexed = {}
    for unit in units or ():
        if not isinstance(unit, Mapping) or unit.get("eid") is None:
            continue
        eid = str(unit["eid"])
        if eid in indexed:
            raise CoordinateMappingError("duplicate unit eid in coordinate snapshot")
        indexed[eid] = unit
    return indexed


def _fighting_unit(unit):
    attrs = unit.get("attrs") or {}
    if not isinstance(attrs, Mapping):
        attrs = {}
    if (unit.get("kind") in ("field", "support") or unit.get("native_type") == 4
            or attrs.get("support") or unit.get("support")):
        return False
    return (unit.get("kind") in ("ally", "enemy", "hero", "monster")
            or unit.get("camp") in (1, 2) or attrs.get("camp_id") in (1, 2))


def coordinate_transform(native_units, client_units, native_to_client_eid_map):
    """Establish a frame from at least two distinct, consistent fighting pairs.

    ``native_to_client_eid_map`` is the existing native-EID -> client-EID map
    for this recipient. Neither player side nor map ID implies a coordinate
    frame. Missing, ambiguous, malformed or contradictory evidence raises
    CoordinateMappingError; callers must wait for initial snapshots or fail
    explicitly instead of guessing a center.
    """
    if not isinstance(native_to_client_eid_map, Mapping):
        raise CoordinateMappingError("coordinate mapping requires matched entity ids")
    native = _units_by_eid(native_units)
    client = _units_by_eid(client_units)
    pairs = []
    seen_clients = set()
    for native_eid, client_eid in native_to_client_eid_map.items():
        native_unit = native.get(str(native_eid))
        client_unit = client.get(str(client_eid))
        if native_unit is None or client_unit is None:
            raise CoordinateMappingError("matched entity is absent from coordinate snapshot")
        native_fights = _fighting_unit(native_unit)
        client_fights = _fighting_unit(client_unit)
        if native_fights != client_fights:
            raise CoordinateMappingError("matched entities have different coordinate roles")
        if not native_fights:
            continue
        if str(client_eid) in seen_clients:
            raise CoordinateMappingError("fighting entities share one client eid")
        seen_clients.add(str(client_eid))
        if native_unit.get("role") != client_unit.get("role"):
            raise CoordinateMappingError("matched fighting entities have different roles")
        pairs.append((_cube(native_unit.get("hex")), _cube(client_unit.get("hex"))))
    if (len(pairs) < 2 or len({pair[0] for pair in pairs}) < 2
            or len({pair[1] for pair in pairs}) < 2):
        raise CoordinateMappingError("two distinct fighting anchors are required")

    candidates = []
    for sign in (1, -1):
        offset = tuple(n - sign * c for n, c in zip(*pairs[0]))
        if all(tuple(n - sign * c for n, c in zip(native_hex, client_hex)) == offset
               for native_hex, client_hex in pairs):
            candidates.append(CoordinateTransform(sign, offset, len(pairs)))
    if len(candidates) != 1:
        raise CoordinateMappingError("fighting anchors do not establish one consistent board transform")
    return candidates[0]
