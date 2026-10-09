"""仅按文件和行号读取当前施工源码，统一使用UTF-8输出。"""
import sys
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')
for spec in sys.argv[1:]:
    pieces = spec.split(':')
    path = Path(pieces[0])
    lines = path.read_text(encoding='utf-8-sig').splitlines()
    begin = int(pieces[1]) if len(pieces) > 1 else 1
    end = int(pieces[2]) if len(pieces) > 2 else len(lines)
    print(str(path))
    for n in range(begin, min(end, len(lines)) + 1):
        print(f'{n}: {lines[n-1]}')
