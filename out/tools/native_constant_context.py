"""静态打印模块字段附近的字节码，不执行客户端代码。"""
import io
import sys
from pathlib import Path
from mem_marshal_extract import Loader
from neox_dis import disasm

sys.stdout.reconfigure(encoding="utf-8")
root = Path(__file__).resolve().parents[2]
path = root / "out/npk_scripts/android_base" / (sys.argv[1] + ".marshal")
patched = root / "out/npk_scripts/android_patch" / path.name
if patched.exists():
    path = patched
stream = io.StringIO()
disasm(Loader(path.read_bytes()).r_object(), out=stream)
lines = stream.getvalue().splitlines()
hits = [i for i, line in enumerate(lines) if any(word in line for word in sys.argv[2:])]
selected = set()
for hit in hits:
    selected.update(range(max(0, hit - 22), min(len(lines), hit + 4)))
for i in sorted(selected):
    print(lines[i])
