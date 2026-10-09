"""Execute original NXS battle scripts in a separate CPython 2.7 process.

This module never imports the live server, reads accounts, or sends client RPCs.
The worker imports translated CPython 2.7 modules from a local extracted tree.
Only the worker executes game code; bytecode/dependency failures are returned
as explicit errors.

Requires the isolated Python 2 runtime, BSON wheel, and extracted ``script/``
tree in runtime/native_engine. Runtime does not open script.npk or a key file.
No APK archive, installed client executable, or remote resource source is read.
The live HTTP hotfix and account state are not loaded here; callers may pass a
detached ``activity_buff_snapshot`` in battle metadata.
"""

import argparse
import base64
import bisect
from collections.abc import Mapping
import dataclasses
import json
import math
import os
from pathlib import Path
import queue
import secrets
import struct
import subprocess
import sys
import threading
import time

from app_paths import ROOT

DEFAULT_BASE = ROOT / "runtime/native_engine"
DEFAULT_PYTHON2 = ROOT / "runtime/native_engine/python2/package/tools/python.exe"
PYC_MAGIC = b"\x03\xf3\r\n"

# The opcode dispatch in the shipped engine, not the standard dis table.
OPCODES = {
    0x07: 1, 0x2e: 2, 0x25: 3, 0x42: 4, 0x0c: 5, 0x34: 9,
    0x23: 10, 0x43: 11, 0x51: 12, 0x20: 13, 0x09: 15,
    0x3f: 19, 0x27: 20, 0x2c: 21, 0x24: 22, 0x46: 23,
    0x39: 24, 0x0a: 25, 0x0b: 26, 0x0d: 27, 0x31: 28, 0x17: 29,
    0x0e: 30, 0x0f: 31, 0x10: 32, 0x11: 33,
    0x18: 40, 0x19: 41, 0x1a: 42, 0x1b: 43,
    0x56: 50, 0x57: 51, 0x58: 52, 0x59: 53,
    0x08: 54, 0x15: 55, 0x37: 56, 0x52: 57, 0x22: 58,
    0x16: 59, 0x41: 60, 0x06: 61, 0x3a: 62, 0x47: 63,
    0x35: 64, 0x1e: 65, 0x13: 66, 0x05: 67, 0x3c: 68,
    0x4b: 70, 0x2b: 71, 0x2a: 72, 0x03: 73, 0x30: 74,
    0x54: 75, 0x4d: 76, 0x4e: 77, 0x55: 78, 0x2f: 79,
    0x33: 80, 0x36: 81, 0x32: 82, 0x53: 83, 0x4a: 84,
    0x40: 85, 0x1f: 86, 0x48: 87, 0x2d: 88, 0x21: 89,
    0x74: 90, 0x9f: 91, 0x7d: 92, 0x95: 93, 0x9d: 94,
    0x84: 95, 0x5f: 96, 0x71: 97, 0x6f: 98, 0x8a: 99,
    0x99: 100, 0x65: 101, 0x87: 102, 0x5a: 103, 0x63: 104,
    0x97: 105, 0x60: 106, 0x72: 107, 0x86: 108, 0x91: 109,
    0x9c: 110, 0x67: 112, 0x69: 111, 0x89: 113, 0x94: 114,
    0xac: 115, 0x9b: 116, 0x82: 119, 0x9e: 120,
    0x80: 121, 0x6e: 122, 0x61: 124, 0x68: 125, 0x76: 126,
    0x5d: 130, 0x83: 131, 0x64: 132, 0x73: 133, 0x88: 134,
    0x78: 135, 0x81: 136, 0x66: 137, 0x8c: 140, 0x8d: 141,
    0x8e: 142, 0x5e: 143, 0xa0: 145, 0x6d: 146, 0x7b: 147,
}
RELATIVE = {93, 110, 120, 121, 122, 143}
ABSOLUTE = {111, 112, 113, 114, 115, 119}
NAME_OPS = {90, 91, 95, 96, 97, 98, 101, 106, 108, 109, 116}


@dataclasses.dataclass
class NativeCode:
    argcount: int
    nlocals: int
    stacksize: int
    flags: int
    code: bytes
    consts: tuple
    names: tuple
    varnames: tuple
    freevars: tuple
    cellvars: tuple
    filename: bytes
    name: bytes
    firstlineno: int
    lnotab: bytes


class BytecodeError(ValueError):
    pass


class MarshalReader:
    """Read Python 2 marshal without losing bytes or code object fields."""

    def __init__(self, body):
        self.body, self.pos, self.interned = body, 0, []

    def take(self, count):
        if count < 0 or self.pos + count > len(self.body):
            raise BytecodeError("truncated marshal at %d" % self.pos)
        data = self.body[self.pos:self.pos + count]
        self.pos += count
        return data

    def integer(self):
        return struct.unpack("<i", self.take(4))[0]

    def read(self):
        tag = self.take(1)
        if tag == b"N":
            return None
        if tag in (b"F", b"T"):
            return tag == b"T"
        if tag == b".":
            return Ellipsis
        if tag == b"S":
            return StopIteration
        if tag == b"i":
            return self.integer()
        if tag == b"I":
            return struct.unpack("<q", self.take(8))[0]
        if tag == b"l":
            count = self.integer()
            if abs(count) > 100000:
                raise BytecodeError("invalid long digit count")
            value = sum(struct.unpack("<H", self.take(2))[0] << (15 * i)
                        for i in range(abs(count)))
            return -value if count < 0 else value
        if tag == b"f":
            return float(self.take(self.take(1)[0]))
        if tag == b"g":
            return struct.unpack("<d", self.take(8))[0]
        if tag == b"y":
            return complex(*struct.unpack("<dd", self.take(16)))
        if tag == b"x":
            return complex(float(self.take(self.take(1)[0])),
                           float(self.take(self.take(1)[0])))
        if tag in (b"s", b"t", b"u"):
            data = self.take(self.integer())
            if tag == b"t":
                self.interned.append(data)
            return data.decode("utf-8") if tag == b"u" else data
        if tag == b"R":
            return self.interned[self.integer()]
        if tag in (b"(", b"[", b"<", b">"):
            count = self.integer()
            if count < 0 or count > 2000000:
                raise BytecodeError("invalid sequence count")
            items = [self.read() for _ in range(count)]
            return {b"(": tuple, b"[": list, b"<": set, b">": frozenset}[tag](items)
        if tag == b"{":
            result = {}
            while self.body[self.pos:self.pos + 1] != b"0":
                key = self.read()
                result[key] = self.read()
            self.take(1)
            return result
        if tag == b"c":
            numbers = [self.integer() for _ in range(4)]
            fields = [self.read() for _ in range(8)]
            firstline, lnotab = self.integer(), self.read()
            return NativeCode(*numbers, *fields, firstline, lnotab)
        raise BytecodeError("unsupported marshal tag %r at %d" % (tag, self.pos - 1))


def marshal_dump(value):
    pack = lambda number: struct.pack("<i", number)
    if value is None:
        return b"N"
    if value is StopIteration:
        return b"S"
    if value is Ellipsis:
        return b"."
    if isinstance(value, bool):
        return b"T" if value else b"F"
    if isinstance(value, int):
        if -(1 << 31) <= value < (1 << 31):
            return b"i" + pack(value)
        digits, magnitude = [], abs(value)
        while magnitude:
            digits.append(struct.pack("<H", magnitude & 0x7fff))
            magnitude >>= 15
        return b"l" + pack(len(digits) * (-1 if value < 0 else 1)) + b"".join(digits)
    if isinstance(value, float):
        return b"g" + struct.pack("<d", value)
    if isinstance(value, complex):
        return b"y" + struct.pack("<dd", value.real, value.imag)
    if isinstance(value, (bytes, str)):
        data = value.encode("utf-8") if isinstance(value, str) else value
        return (b"u" if isinstance(value, str) else b"s") + pack(len(data)) + data
    if isinstance(value, (tuple, list, set, frozenset)):
        tag = {tuple: b"(", list: b"[", set: b"<", frozenset: b">"}[type(value)]
        return tag + pack(len(value)) + b"".join(map(marshal_dump, value))
    if isinstance(value, dict):
        return b"{" + b"".join(marshal_dump(k) + marshal_dump(v)
                                for k, v in value.items()) + b"0"
    if isinstance(value, NativeCode):
        return (b"c" + b"".join(pack(getattr(value, key)) for key in
                ("argcount", "nlocals", "stacksize", "flags")) +
                b"".join(marshal_dump(getattr(value, key)) for key in
                ("code", "consts", "names", "varnames", "freevars", "cellvars", "filename", "name")) +
                pack(value.firstlineno) + marshal_dump(value.lnotab))
    raise BytecodeError("unsupported marshal value %r" % type(value))


def _encode(op, arg):
    if arg is None:
        return bytes([op])
    if arg < 0 or arg > 0xffffffff:
        raise BytecodeError("invalid opcode argument %d" % arg)
    prefix = bytes([145]) + struct.pack("<H", arg >> 16) if arg > 0xffff else b""
    return prefix + bytes([op]) + struct.pack("<H", arg & 0xffff)


def translate_code(co):
    instructions, pos, extension, extension_at = [], 0, 0, None
    while pos < len(co.code):
        at, custom = pos, co.code[pos]
        pos += 1
        arg = None
        if custom >= 90:
            if pos + 2 > len(co.code):
                raise BytecodeError("truncated opcode at %s:%d" % (co.name, at))
            arg = struct.unpack_from("<H", co.code, pos)[0] | extension
            pos += 2
        if custom == 0xa0:
            if extension_at is None:
                extension_at = at
            extension = arg << 16
            continue
        start = at if extension_at is None else extension_at
        extension, extension_at = 0, None
        if custom == 0xad:
            ops = [(124, 0), (100, arg)]
        else:
            if custom not in OPCODES:
                raise BytecodeError("unknown opcode %02x at %s:%d" % (custom, co.name, at))
            ops = [(OPCODES[custom], arg)]
        for op, operand in ops:
            if op == 100 and operand >= len(co.consts):
                raise BytecodeError("constant index at %s:%d" % (co.name, at))
            if op in NAME_OPS and operand >= len(co.names):
                raise BytecodeError("name index at %s:%d" % (co.name, at))
            if op in (124, 125, 126) and operand >= co.nlocals:
                raise BytecodeError("local index at %s:%d" % (co.name, at))
        op, operand = ops[-1]
        target = pos + operand if op in RELATIVE else operand if op in ABSOLUTE else None
        instructions.append([start, pos, ops, target])
    if extension_at is not None:
        raise BytecodeError("unterminated EXTENDED_ARG")
    lengths = [sum(len(_encode(op, arg)) for op, arg in row[2]) for row in instructions]
    for _ in range(10):
        offsets, total = {}, 0
        for row, size in zip(instructions, lengths):
            offsets[row[0]] = total
            total += size
        offsets[len(co.code)] = total
        chunks = []
        for row in instructions:
            start, _, ops, target = row
            if target is not None:
                if target not in offsets:
                    raise BytecodeError("jump into operand at %s:%d -> %d" % (co.name, start, target))
                op, _ = ops[-1]
                arg = offsets[target]
                if op in RELATIVE:
                    arg -= offsets[start] + lengths[len(chunks)]
                ops = ops[:-1] + [(op, arg)]
            chunks.append(b"".join(_encode(op, arg) for op, arg in ops))
        new_lengths = list(map(len, chunks))
        if new_lengths == lengths:
            break
        lengths = new_lengths
    else:
        raise BytecodeError("jump relocation did not converge")
    # Preserve traceback line numbers after compound instructions expand.
    lnotab, old_address, new_address = bytearray(), 0, 0
    boundaries = sorted(offsets)
    for index in range(0, len(co.lnotab), 2):
        old_address += co.lnotab[index]
        # Long lines can contain 255-byte continuation entries inside operands.
        boundary = boundaries[bisect.bisect_right(boundaries, old_address) - 1]
        delta = offsets[boundary] - new_address
        while delta > 255:
            lnotab.extend((255, 0))
            delta -= 255
        lnotab.extend((delta, co.lnotab[index + 1]))
        new_address = offsets[boundary]
    return dataclasses.replace(co, code=b"".join(chunks), lnotab=bytes(lnotab),
        consts=tuple(translate_code(item) if isinstance(item, NativeCode) else item
                     for item in co.consts))


def decode_script(plain):
    failures = []
    for size in (99, 23):
        body = bytearray(plain)
        for i in range(min(size, len(body))):
            body[i] ^= 0xe9
        try:
            reader = MarshalReader(bytes(body[1:-1]))
            code = reader.read()
            if not isinstance(code, NativeCode) or reader.pos != len(reader.body):
                raise BytecodeError("incomplete root code object")
            return translate_code(code)
        except (ValueError, IndexError, TypeError, struct.error) as error:
            failures.append(str(error))
    raise BytecodeError("cannot decode NXS: " + "; ".join(failures))


class NativeResources:
    """Load translated battle scripts from an extracted ``script/`` tree.

    Prefers ``.py`` when both source and bytecode exist.
    """

    def __init__(self, base=DEFAULT_BASE):
        self.base = Path(base)
        self.script_dir = self.base / "script"
        if not self.script_dir.is_dir():
            raise FileNotFoundError("extracted native scripts missing: " + str(self.script_dir))
        self.paths = {}
        self.packages = set()
        for path in self.script_dir.rglob("*"):
            if path.suffix.lower() not in (".py", ".pyc") or not path.is_file():
                continue
            rel = path.relative_to(self.script_dir)
            parts = list(rel.with_suffix("").parts)
            if not parts:
                continue
            if parts[-1] == "__init__":
                module = ".".join(parts[:-1])
                if not module:
                    continue
                self.packages.add(module)
            else:
                module = ".".join(parts)
            existing = self.paths.get(module)
            if existing is not None and existing.suffix.lower() == ".py" and path.suffix.lower() == ".pyc":
                continue
            self.paths[module] = path
            pieces = module.split(".")
            for count in range(1, len(pieces)):
                self.packages.add(".".join(pieces[:count]))
        if "battle_logic.server_battle" not in self.paths:
            raise FileNotFoundError(
                "extracted battle_logic.server_battle missing: " + str(self.script_dir))
        self.source_paths = [self.script_dir]

    def payload(self, module):
        path = self.paths.get(module)
        if path is None:
            if module in self.packages:
                return None
            raise ImportError(module)
        if path.suffix.lower() != ".pyc":
            raise ImportError(module + " is source; worker imports it from disk")
        data = path.read_bytes()
        if data.startswith(PYC_MAGIC) and len(data) >= 8:
            data = data[8:]
        return base64.b64encode(data).decode("ascii")


class NativeBattleError(RuntimeError):
    pass


class NativeCommandRejected(NativeBattleError):
    """Recoverable player input rejection, classified by the worker protocol."""

    error_code = "command_rejected"

    def __init__(self, message, native_traceback=""):
        super().__init__(message)
        self.message = message
        self.native_traceback = native_traceback


def _object_id(value):
    if isinstance(value, (bytes, bytearray)):
        if len(value) != 12:
            raise ValueError("native ObjectId must contain 12 bytes")
        return bytes(value).hex()
    if hasattr(value, "binary"):
        return _object_id(value.binary)
    if not isinstance(value, str) or len(value) != 24:
        raise ValueError("native ObjectId must be a 24-character hex string")
    try:
        bytes.fromhex(value)
    except ValueError as error:
        raise ValueError("invalid native ObjectId") from error
    return value.lower()


def _wire_snapshot(value):
    """Detach server wire values and encode BSON identities without account I/O."""
    if isinstance(value, (bytes, bytearray)) or hasattr(value, "binary"):
        return _object_id(value)
    if value is None or isinstance(value, (bool, int, str)):
        return value
    if isinstance(value, float) and math.isfinite(value):
        return value
    if isinstance(value, (tuple, list)):
        return [_wire_snapshot(item) for item in value]
    if isinstance(value, Mapping):
        result = {}
        for key, item in value.items():
            normalized = _wire_snapshot(key)
            if not isinstance(normalized, (int, str)) or isinstance(normalized, bool):
                raise ValueError("native mapping keys must be strings, integers or ObjectIds")
            normalized = str(normalized)
            if normalized in result:
                raise ValueError("duplicate native mapping key " + normalized)
            result[normalized] = _wire_snapshot(item)
        return result
    raise TypeError("unsupported native roster value: " + type(value).__name__)


def _activity_int(value, field):
    if isinstance(value, bool) or not isinstance(value, int):
        raise ValueError("%s must be an integer" % field)
    return value


def _activity_json_value(value, field):
    """Detach one JSON value accepted by the native activity-buff code."""
    if value is None or isinstance(value, (bool, int, str)):
        return value
    if isinstance(value, float):
        if not math.isfinite(value):
            raise ValueError("%s must be a finite JSON number" % field)
        return value
    if isinstance(value, (list, tuple)):
        return [_activity_json_value(item, "%s[%d]" % (field, index))
                for index, item in enumerate(value)]
    if isinstance(value, Mapping):
        normalized = {}
        for key, item in value.items():
            if not isinstance(key, str):
                raise ValueError("%s object keys must be strings" % field)
            normalized[key] = _activity_json_value(item, "%s.%s" % (field, key))
        return normalized
    raise ValueError("%s must be a JSON value" % field)


def normalize_activity_buff_snapshot(snapshot):
    """Validate and detach the activity-only data consumed by native battle.

    The wire shape deliberately mirrors the original Python 2 battle code:
    ``add_battle_skills`` contains six-tuples and ``change_attr_data.buff``
    maps camp ids to ``[buff_id, user_property]`` pairs.  ``user_property`` is
    any finite JSON value because the native buff id selects its type. JSON
    turns integer mapping keys into strings; the host restores those keys
    before assigning ``server_battle.extra_info``.
    """
    if not isinstance(snapshot, Mapping):
        raise ValueError("activity_buff_snapshot must be a mapping")
    unknown = set(snapshot) - {"add_battle_skills", "change_attr_data", "enemy_level_added", "hs_summer_closed_beta"}
    if unknown:
        raise ValueError("unknown activity buff fields: %s" % sorted(unknown))

    skills = snapshot.get("add_battle_skills", [])
    if not isinstance(skills, (list, tuple)):
        raise ValueError("add_battle_skills must be a list")
    normalized_skills = []
    for index, skill in enumerate(skills):
        if not isinstance(skill, (list, tuple)) or len(skill) != 6:
            raise ValueError("add_battle_skills[%d] must contain six fields" % index)
        skill_id = _activity_int(skill[0], "add_battle_skills[%d].skill_id" % index)
        camp_id = _activity_int(skill[1], "add_battle_skills[%d].camp_id" % index)
        level = _activity_int(skill[2], "add_battle_skills[%d].level" % index)
        enhance_level = _activity_int(skill[3],
                                      "add_battle_skills[%d].enhance_level" % index)
        card_grade = _activity_int(skill[4], "add_battle_skills[%d].card_grade" % index)
        if not isinstance(skill[5], bool):
            raise ValueError("add_battle_skills[%d].show_enable must be boolean" % index)
        normalized_skills.append([skill_id, camp_id, level, enhance_level,
                                  card_grade, skill[5]])

    change = snapshot.get("change_attr_data", {})
    if not isinstance(change, Mapping):
        raise ValueError("change_attr_data must be a mapping")
    if set(change) - {"buff"}:
        raise ValueError("unknown change_attr_data fields: %s" %
                         sorted(set(change) - {"buff"}))
    buff = change.get("buff", {})
    if not isinstance(buff, Mapping):
        raise ValueError("change_attr_data.buff must be a mapping")
    normalized_buff = {}
    for camp_id, entries in buff.items():
        if isinstance(camp_id, bool):
            raise ValueError("change_attr_data.buff camp id must be an integer")
        if isinstance(camp_id, int):
            normalized_camp = camp_id
        elif isinstance(camp_id, str):
            try:
                normalized_camp = int(camp_id)
            except ValueError:
                raise ValueError("change_attr_data.buff camp id must be an integer")
            if str(normalized_camp) != camp_id:
                raise ValueError("change_attr_data.buff camp id must be canonical")
        else:
            raise ValueError("change_attr_data.buff camp id must be an integer")
        if not isinstance(entries, (list, tuple)):
            raise ValueError("change_attr_data.buff[%s] must be a list" % camp_id)
        normalized_entries = []
        for index, entry in enumerate(entries):
            if not isinstance(entry, (list, tuple)) or len(entry) != 2:
                raise ValueError("change_attr_data.buff[%s][%d] must contain two fields" %
                                 (camp_id, index))
            buff_id = _activity_int(entry[0],
                                    "change_attr_data.buff[%s][%d].buff_id" %
                                    (camp_id, index))
            normalized_entries.append([
                buff_id,
                _activity_json_value(
                    entry[1],
                    "change_attr_data.buff[%s][%d].user_property" % (camp_id, index)),
            ])
        if normalized_camp in normalized_buff:
            raise ValueError("duplicate activity buff camp id %s" % normalized_camp)
        normalized_buff[normalized_camp] = normalized_entries

    result = {"add_battle_skills": normalized_skills,
              "change_attr_data": {"buff": normalized_buff}}
    if "enemy_level_added" in snapshot:
        addition = _activity_int(snapshot["enemy_level_added"], "enemy_level_added")
        if addition not in (-20, -10, 0, 45, 150):
            raise ValueError("enemy_level_added is outside the native correction table")
        result["enemy_level_added"] = addition
    if "hs_summer_closed_beta" in snapshot:
        if not isinstance(snapshot["hs_summer_closed_beta"], bool):
            raise ValueError("hs_summer_closed_beta must be boolean")
        result["hs_summer_closed_beta"] = snapshot["hs_summer_closed_beta"]
    return result


def roster_from_manager(card_manager, selected_uuids):
    """Snapshot 0-8 owned cards, including all rune/skill/dress/progression fields.

    Accepts the server's byte-keyed card manager or its JSON representation.
    Call under the account lock; the returned roster shares no mutable state.
    """
    if not isinstance(card_manager, Mapping):
        raise ValueError("card_manager must be a mapping")
    if not isinstance(selected_uuids, (list, tuple)) or not 0 <= len(selected_uuids) <= 8:
        raise ValueError("select zero to eight owned card UUIDs")
    selected = [_object_id(value) for value in selected_uuids]
    if len(set(selected)) != len(selected):
        raise ValueError("duplicate selected card UUID")
    manager = {}
    for key, card in card_manager.items():
        identity = _object_id(key)
        if identity in manager:
            raise ValueError("duplicate card manager UUID")
        manager[identity] = card
    roster = []
    for identity in selected:
        if identity not in manager:
            raise ValueError("selected card is not owned: " + identity)
        card = _wire_snapshot(manager[identity])
        if not isinstance(card, dict) or _object_id(card.get("uuid", identity)) != identity:
            raise ValueError("card manager UUID does not match card UUID")
        card["uuid"] = identity
        roster.append(card)
    return roster


class NativeBattleProcess:
    """One engine process per battle; no shared RNG, modules, or callbacks."""

    def __init__(self, resources=None, python2=DEFAULT_PYTHON2, timeout=30):
        self.resources = resources or NativeResources()
        self.request_timeout, self.lock = timeout, threading.RLock()
        if timeout <= 0:
            raise ValueError("timeout must be positive")
        self.closed = False
        self.output, self.diagnostics = queue.Queue(), []
        env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1", PYTHONIOENCODING="utf-8")
        env.pop("PYTHONPATH", None)
        self.process = subprocess.Popen([str(python2), "-B", "-u", str(ROOT / "battle_native_worker.py")],
            cwd=str(ROOT / "runtime/native_engine"), env=env,
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            text=True, encoding="utf-8", errors="replace")
        def read_output():
            for line in self.process.stdout:
                self.output.put(line)
            self.output.put(None)
        def read_errors():
            for line in self.process.stderr:
                self.diagnostics.append(line.rstrip())
        self.readers = [threading.Thread(target=read_output, daemon=True),
                        threading.Thread(target=read_errors, daemon=True)]
        for reader in self.readers:
            reader.start()
        try:
            self.request("boot", modules=sorted(self.resources.paths), packages=sorted(self.resources.packages))
        except BaseException:
            self.close()
            raise

    def _send(self, value):
        self.process.stdin.write(json.dumps(value, ensure_ascii=True) + "\n")
        self.process.stdin.flush()

    def request(self, operation, **data):
        with self.lock:
            if self.closed:
                raise NativeBattleError("native engine is closed")
            if operation == "inspect":
                data["payload"] = self.resources.payload(data["module"])
            self._send(dict(data, operation=operation))
            deadline = time.monotonic() + self.request_timeout
            while True:
                if time.monotonic() >= deadline:
                    self.close()
                    raise NativeBattleError("native engine timed out during " + operation)
                try:
                    line = self.output.get(timeout=max(0.01, deadline - time.monotonic()))
                except queue.Empty:
                    self.close()
                    raise NativeBattleError("native engine timed out during " + operation)
                if line is None:
                    self.close()
                    raise NativeBattleError("native engine exited (%s): %s" %
                        (self.process.poll(), "\n".join(self.diagnostics[-20:])))
                message = json.loads(line)
                if message.get("request") == "module":
                    try:
                        self._send({"payload": self.resources.payload(message["module"])})
                    except Exception as error:
                        self._send({"error": str(error)})
                    continue
                if "error" in message:
                    if message.get("error_code") == "command_rejected":
                        raise NativeCommandRejected(
                            str(message.get("message") or "native command rejected"),
                            native_traceback=str(message["error"]))
                    raise NativeBattleError(message["error"])
                return message["value"]

    def start(self, metadata, roster, seed=1, auto=True):
        """Start once, loading full native card state and unmodified dungeon data.

        metadata: avatar_id, dungeon_id, optional battle_id/battle_uuid/created_at.
        fighting_card_uuids/support_card_uuids optionally select native slots;
        None preserves an empty slot. Otherwise roster order fills the native
        fighting capacity first, followed by support (usually 4+2 or 6+2).
        Scripted replacements retain the ordinary 4+2 selection allowance.
        roster: 0-8 full dictionaries accepted by custom_types.card.card.load.
        Saved talent_tree/habit_mgr must be supplied by the caller; native load
        applies its defaults when fields are absent. assist_level is preserved
        and overrides combat level when nonzero; ordinary saved cards use zero.
        Empty fighting slots require actual native scripted fighting roles.
        storyline_avatar_list supplies the original storyline override only
        when the native battle enables avatar_list_by_storyline.
        Events contain native RPC arguments and observed native state changes.
        No event from this worker should be forwarded to a live client.
        """
        metadata = _wire_snapshot(dict(metadata))
        roster = _wire_snapshot(roster)
        if "activity_buff_snapshot" in metadata:
            metadata["activity_buff_snapshot"] = normalize_activity_buff_snapshot(
                metadata["activity_buff_snapshot"])
        metadata.setdefault("avatar_id", "000000000000000000000001")
        metadata.setdefault("battle_uuid", secrets.token_hex(12))
        metadata.setdefault("created_at", time.time())
        metadata["avatar_id"] = _object_id(metadata["avatar_id"])
        metadata["battle_uuid"] = _object_id(metadata["battle_uuid"])
        metadata["engine"] = "native-python2"
        metadata["seed"] = seed
        metadata["auto"] = bool(auto)
        metadata["roster"] = roster
        return self.request("start", metadata=metadata, roster=roster, seed=seed, auto=auto)

    def step(self, command=None):
        """Advance one native timer; auto=False allows {name, args} commands.

        use_skill: [current_eid, skill_id, target_eid_or_cube_coordinates].
        move_to: [current_eid, cube_coordinates]. idle: [].
        Rejected commands raise NativeCommandRejected without advancing the clock.
        """
        return self.request("step", command=command)

    def timeout(self):
        """Advance virtual time until native `_timeout` consumes the input window."""
        return self.request("timeout")

    def set_auto(self, avatar_id, enabled=True):
        """Toggle one owner's native auto set; thinks now if it is their window."""
        return self.request("set_auto", avatar_id=str(avatar_id or ""), enabled=bool(enabled))

    def drive(self):
        """Advance field ticks until a player must click, or native result."""
        return self.request("drive")

    def autoplay(self, max_steps=10000):
        """Advance up to max_steps additional callbacks, or until native result.

        Each call has its own budget; response.steps is the cumulative count.
        Budget exhaustion returns running/result=None without settlement.
        """
        return self.request("autoplay", max_steps=max_steps)

    def snapshot(self):
        return self.request("snapshot")

    def close(self):
        with self.lock:
            if self.closed:
                return
            self.closed = True
            if self.process.poll() is None:
                self.process.terminate()
                try:
                    self.process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    self.process.kill()
                    self.process.wait(timeout=5)
            for reader in self.readers:
                reader.join(timeout=5)
            for stream in (self.process.stdin, self.process.stdout, self.process.stderr):
                stream.close()

    def __enter__(self):
        return self

    def __exit__(self, *_):
        self.close()


def persist_update(store, update):
    """Write to an explicitly supplied BattleRecordStore; never open live saves.

    Use a distinct battle_uuid from the client observer. Native events are not
    client telemetry, and client results must not overwrite this engine result.
    A precreated record receives initial_state through set_initial_state because
    an idempotent begin does not replace its initially empty scene.
    """
    metadata = update["metadata"]
    battle_uuid, avatar_id = metadata["battle_uuid"], metadata["avatar_id"]
    initial = next((e["data"] for e in update["events"] if e["kind"] == "initial_state"), None)
    store.begin(metadata, initial)
    if initial is not None:
        store.set_initial_state(battle_uuid, initial, avatar_id=avatar_id)
    for event in update["events"]:
        if event["kind"] != "result":
            store.append(battle_uuid, event["kind"], event["data"], avatar_id=avatar_id,
                         t=event["t"], client_sequence=event["sequence"])
    if update["result"] is not None:
        store.finish(battle_uuid, dict(update["result"], engine=metadata["engine"], t=update["t"]),
                     avatar_id=avatar_id)
    return battle_uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", type=Path, default=DEFAULT_BASE,
                        help="Local resource directory containing extracted script/")
    parser.add_argument("--python2", type=Path, default=DEFAULT_PYTHON2)
    parser.add_argument("--probe")
    parser.add_argument("--disassemble", nargs=2, metavar=("MODULE", "FUNCTION"))
    parser.add_argument("--dungeon-id", type=int)
    parser.add_argument("--battle-id", type=int)
    parser.add_argument("--roster-json", type=Path)
    parser.add_argument("--cards", default="1601,2103,2202,4402")
    parser.add_argument("--level", type=int, default=80)
    parser.add_argument("--grade", type=int, default=5)
    parser.add_argument("--skill-level", type=int, default=5)
    parser.add_argument("--seed", type=int, default=1)
    parser.add_argument("--max-steps", type=int, default=10000)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--records", type=Path, help="Explicit SQLite destination (optional)")
    args = parser.parse_args()
    with NativeBattleProcess(NativeResources(args.base), args.python2) as worker:
        if args.disassemble:
            print(worker.request("disassemble", module=args.disassemble[0], name=args.disassemble[1]))
            return 0
        if args.dungeon_id is None:
            print(json.dumps(worker.request("probe", module=args.probe or "hex_extend"), ensure_ascii=True, indent=2))
            return 0
        if args.roster_json:
            roster = json.loads(args.roster_json.read_text(encoding="utf-8"))
        else:
            cards = [int(item) for item in args.cards.split(",")]
            roles = worker.request("table", table="role_info", keys=cards, fields=["skill_list"])
            roster = [{"card_id": card, "level": args.level, "grade": args.grade, "awakened": 1,
                       "skill_mgr": [{"skill_id": skill, "level": args.skill_level, "enhance_level": 0}
                                     for skill in roles[str(card)]["skill_list"]]} for card in cards]
        metadata = {"dungeon_id": args.dungeon_id}
        if args.battle_id is not None:
            metadata["battle_id"] = args.battle_id
        started = worker.start(metadata, roster, args.seed)
        store = None
        if args.records:
            from battle_records import BattleRecordStore
            store = BattleRecordStore(args.records)
            persist_update(store, started)
        finished = worker.autoplay(args.max_steps)
        if store:
            persist_update(store, finished)
        replay = dict(finished, events=started["events"] + finished["events"],
                      initial_state=next(e["data"] for e in started["events"] if e["kind"] == "initial_state"))
        body = json.dumps(replay, ensure_ascii=True, indent=2)
        if args.output:
            args.output.write_text(body + "\n", encoding="utf-8")
            print(json.dumps({"output": str(args.output.resolve()), "result": finished["result"],
                              "status": finished["status"], "steps": finished["steps"],
                              "events": len(replay["events"])}))
        else:
            print(body)
        return 0 if finished["status"] == "finished" else 2


if __name__ == "__main__":
    sys.exit(main())
