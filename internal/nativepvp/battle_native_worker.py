"""CPython 2.7 worker for the original server battle scripts."""
from __future__ import print_function

import base64
import dis
import glob
import imp
import json
import marshal
import os
import sys
import traceback
import types
from StringIO import StringIO


PROTOCOL_OUT = sys.stdout
sys.stdout = sys.stderr


def send(value):
    PROTOCOL_OUT.write(json.dumps(value, ensure_ascii=True) + "\n")
    PROTOCOL_OUT.flush()


def error_text(value):
    """Keep Python 2 Windows path bytes without replacement-character loss.

    Traceback filename lines use mbcs on Windows, while other lines may contain
    UTF-8 exception text. Decode each line independently before JSON escaping.
    """
    if isinstance(value, unicode):
        return value
    if not isinstance(value, str):
        try:
            return unicode(value)
        except UnicodeError:
            value = str(value)
    encodings = ["utf-8", sys.getfilesystemencoding(), "gb18030"]
    lines = []
    for line in value.splitlines(True):
        for encoding in encodings:
            if not encoding:
                continue
            try:
                decoded = line.decode(encoding, "strict")
                break
            except (UnicodeError, LookupError):
                pass
        else:
            # Python 2 backslashreplace only handles encoding errors. Preserve
            # undecodable bytes explicitly instead of inserting U+FFFD.
            decoded = u"".join(unichr(ord(char)) if ord(char) < 128 else
                               u"\\x%02x" % ord(char) for char in line)
        lines.append(decoded)
    return u"".join(lines)


class ResourceImporter(object):
    def __init__(self, modules, packages):
        self.modules, self.packages = set(modules), set(packages)

    def find_module(self, fullname, path=None):
        if fullname in self.modules or fullname in self.packages:
            return self
        return None

    def load_module(self, fullname):
        if fullname in sys.modules:
            return sys.modules[fullname]
        send({"request": "module", "module": fullname})
        response = json.loads(sys.stdin.readline())
        if "error" in response:
            raise ImportError(fullname + ": " + response["error"])
        module = imp.new_module(fullname)
        module.__file__ = "script:" + fullname
        module.__loader__ = self
        module.__package__ = fullname if fullname in self.packages else fullname.rpartition(".")[0]
        if fullname in self.packages:
            module.__path__ = ["script:" + fullname]
        sys.modules[fullname] = module
        try:
            if response["payload"]:
                code = marshal.loads(base64.b64decode(response["payload"]))
                exec(code, module.__dict__)
        except BaseException:
            del sys.modules[fullname]
            raise
        return module


def load(name):
    return __import__(name, fromlist=["*"])


def find_function(module, name):
    if hasattr(module, name):
        return getattr(module, name)
    for value in vars(module).values():
        if isinstance(value, type) and hasattr(value, name):
            return getattr(value, name)
    raise AttributeError(name)


def execute(request):
    operation = request["operation"]
    if operation == "boot":
        sys.path.extend(glob.glob("deps/*.whl"))
        script_dir = os.path.abspath("script")
        if not os.path.isdir(script_dir):
            raise RuntimeError("extracted native scripts missing: " + script_dir)
        sys.path.insert(0, script_dir)
        from battle_native_host import Host
        global HOST
        HOST = Host()
        HOST.install()
        return {"python": sys.version, "modules": len(request["modules"])}
    if operation == "probe":
        module = load(request["module"])
        result = {"module": module.__name__, "names": sorted(name for name in vars(module) if not name.startswith("__"))}
        if request["module"] == "hex_extend":
            result["distance"] = module.distance((0, 0, 0), (1, -1, 0))
        if request["module"] == "battle_logic.server_battle":
            load("utils.callback_mgr").get_time = lambda: HOST.clock.now
            HOST.battle = module.server_battle()
            result["instance"] = type(HOST.battle).__name__
        return result
    if operation == "table":
        table = getattr(sys.modules["data"], request["table"])
        keys = request.get("keys")
        from battle_native_host import json_value
        if request.get("keys_only"):
            return json_value(sorted(table))
        fields = request.get("fields")
        return {str(k): json_value({f: table[k].get(f) for f in fields} if fields else table[k])
                for k in (keys if keys is not None else sorted(table)[:20])}
    if operation == "start":
        return HOST.start(request["metadata"], request["roster"], request.get("seed", 1), request.get("auto", True))
    if operation == "step":
        return HOST.step(request.get("command"))
    if operation == "timeout":
        return HOST.consume_timeout()
    if operation == "set_auto":
        return HOST.set_auto(request.get("avatar_id"), request.get("enabled", True))
    if operation == "snapshot":
        return HOST.snapshot()
    if operation == "autoplay":
        return HOST.autoplay(request.get("max_steps", 10000))
    if operation == "drive":
        return HOST.drive()
    if operation == "inspect":
        root = marshal.loads(base64.b64decode(request["payload"]))
        codes = []
        def walk(code, path):
            path = path + [code.co_name]
            if request.get("name") in (None, code.co_name, ".".join(path[1:])):
                codes.append(code)
            for item in code.co_consts:
                if isinstance(item, types.CodeType):
                    walk(item, path)
        walk(root, [])
        output, previous = StringIO(), sys.stdout
        sys.stdout = output
        try:
            for code in codes:
                print(code.co_name, code.co_varnames, "defaults/constants:",
                    [c for c in code.co_consts if not isinstance(c, types.CodeType)])
                dis.dis(code)
        finally:
            sys.stdout = previous
        return output.getvalue().decode("utf-8", "replace")
    if operation == "imported":
        return sorted(sys.modules)
    if operation == "disassemble":
        function = find_function(load(request["module"]), request["name"])
        output, previous = StringIO(), sys.stdout
        sys.stdout = output
        try:
            dis.dis(function)
        finally:
            sys.stdout = previous
        return output.getvalue()
    raise ValueError("unsupported operation " + operation)


def main():
    while True:
        line = sys.stdin.readline()
        if not line:
            break
        try:
            request = json.loads(line)
            send({"value": execute(request)})
        except BaseException as error:
            rejection = getattr(sys.modules.get("battle_native_host"), "NativeCommandRejected", None)
            code = ("command_rejected" if rejection is not None and isinstance(error, rejection)
                    else "native_error")
            send({"error": error_text(traceback.format_exc()), "error_code": code,
                  "message": error_text(error)})


if __name__ == "__main__":
    main()
