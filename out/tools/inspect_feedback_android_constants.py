# -*- coding: utf-8 -*-
"""只读检索Android原资源常量，包含正式补丁覆盖；不执行字节码。"""
import json
import sys
from pathlib import Path
from mem_marshal_extract import Loader
sys.stdout.reconfigure(encoding='utf-8')
root = Path(__file__).resolve().parents[2]
terms = sys.argv[1:]
paths = {}
for folder in ('android_base', 'android_patch'):
    for path in (root/'out/npk_scripts'/folder).glob('*.marshal'):
        paths[path.name] = path

def visit(value, location):
    if isinstance(value, dict):
        if 'filename' in value:
            location = str(value['filename']) + '/' + str(value.get('name', ''))
        for key, child in value.items():
            if key not in ('code', 'lnotab'):
                visit(child, location)
    elif isinstance(value, (tuple, list)):
        for child in value:
            visit(child, location)
    elif isinstance(value, (str, bytes)):
        text = value.decode('utf-8', 'replace') if isinstance(value, bytes) else value
        if any(term in text for term in terms):
            print(json.dumps({'位置':location,'常量':text[:600]},ensure_ascii=False))

for name, path in sorted(paths.items()):
    raw = path.read_bytes()
    if any(term.encode('utf-8') in raw for term in terms):
        visit(Loader(raw).r_object(), path.stem)
