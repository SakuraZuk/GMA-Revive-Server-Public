# -*- coding: utf-8 -*-
import json, pathlib
m = json.loads(pathlib.Path(r'..\npk_decoded\pc_base\manifest.json').read_text(encoding='utf-8'))
for e in m['entries']:
    fn = str(e.get('filename', ''))
    if fn.replace('\\', '/').endswith('entities/Account.py'):
        print(e['hash'] + '.marshal')
        break
