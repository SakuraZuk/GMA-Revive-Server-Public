# -*- coding: utf-8 -*-
"""domain_scan.py — 扫描全部模块 marshal consts 树,收集域名/IP/URL 硬编码点。
输出 out/domain-scan.json:每条(模块, hash, 字符串, 常量路径)。
"""
import sys
import json
import io
import re
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from mem_marshal_extract import Loader

ROOT = Path(r'./out/npk_scripts')

RE_URL = re.compile(
    r'(https?://[^\s\'"<>]+|[A-Za-z0-9.-]{4,}\.(?:com|cn|net|org)(?::\d+)?|'
    r'\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(?::\d+)?)')


def dec(v):
    return v.decode('utf-8', 'replace') if isinstance(v, bytes) else v


def walk(v, path, hits):
    if isinstance(v, dict):
        if v.get('type') == 'code':
            nm = dec(v.get('name'))
            walk(v.get('consts'), path + '/<%s>' % nm, hits)
        return
    if isinstance(v, (bytes, str)):
        if isinstance(v, bytes):
            try:
                s_u = v.decode('utf-8')
            except UnicodeDecodeError:
                return
        else:
            s_u = v
        for m in RE_URL.finditer(s_u):
            hits.append((m.group(0), path))
    elif isinstance(v, (tuple, list)):
        for i, x in enumerate(v):
            walk(x, path + '/%d' % i, hits)


def main():
    inv = json.load(io.open(ROOT / 'android_inventory.json', encoding='utf-8'))
    out = []
    for i, m in enumerate(inv['modules']):
        mf = ROOT / m['marshal_file']
        try:
            code = Loader(mf.read_bytes()).r_object()
        except Exception:
            continue
        hits = []
        walk(code.get('consts'), '', hits)
        for s, path in hits:
            out.append({'module': m['filename'].replace('\\', '/'),
                        'hash': m['hash'], 'value': s, 'path': path[:120]})
        if (i + 1) % 500 == 0:
            print('  ...%d/%d, hits=%d' % (i + 1, len(inv['modules']), len(out)))
    with io.open(r'./out/domain-scan.json', 'w', encoding='utf-8') as f:
        json.dump(out, f, ensure_ascii=False, indent=1)
    # 汇总
    from collections import Counter
    doms = Counter(h['value'] for h in out)
    print('total hits:', len(out), 'distinct:', len(doms))
    for d, c in doms.most_common(60):
        print('%5d  %s' % (c, d))


if __name__ == '__main__':
    main()
