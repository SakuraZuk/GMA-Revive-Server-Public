"""静态查找日志堆栈涉及的原生模块，不执行代码。"""
import sys
from pathlib import Path
from mem_marshal_extract import Loader
sys.stdout.reconfigure(encoding='utf-8')
root = Path(__file__).resolve().parents[2]
for folder in ('android_base', 'android_patch'):
    for path in (root / 'out/npk_scripts' / folder).glob('*.marshal'):
        raw = path.read_bytes()
        if not any(word.encode() in raw for word in sys.argv[1:]):
            continue
        code = Loader(raw).r_object()
        filename = code.get('filename', '')
        if isinstance(filename, bytes):
            filename = filename.decode('utf-8', 'replace')
        if any(word in filename for word in sys.argv[1:]):
            print(path.stem, filename)
