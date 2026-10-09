# -*- coding: utf-8 -*-
"""NeoX 字节码结构统计:用已证实的宽度规则(op<90:1B, op>=90:3B)
解析全部 marshal code 对象,按 oparg 与各符号表长度的关系分类 opcode。
"""
import sys, json
from pathlib import Path
from collections import defaultdict, Counter

sys.path.insert(0, str(Path(__file__).parent))
from mem_marshal_extract import Loader

def walk(c):
    yield c
    for k in c['consts']:
        if isinstance(k, dict) and k.get('type') == 'code':
            yield from walk(k)

def main():
    stats = defaultdict(Counter)   # op -> Counter('n','fit_const','fit_name','fit_var','fit_free','oob')
    freq = Counter()
    files = codes = 0
    p = Path(r'.\out\npk_scripts\android_base')
    for mf in sorted(p.glob('*.marshal')):
        try:
            code = Loader(mf.read_bytes()).r_object()
        except Exception:
            continue
        files += 1
        for c in walk(code):
            codes += 1
            b = c['bytecode']; n = len(b)
            lc = len(c['consts']); ln = len(c['names'])
            lv = len(c['varnames']); lf = len(c['freevars']) + len(c['cellvars'])
            i = 0
            while i < n:
                op = b[i]
                if op < 90:
                    freq[op] += 1
                    i += 1
                    continue
                if i + 3 > n:
                    stats[op]['truncated'] += 1
                    break
                arg = b[i+1] | (b[i+2] << 8)
                freq[op] += 1
                s = stats[op]
                s['n'] += 1
                s['fit_const' if arg < lc else 'x_const'] += 1
                s['fit_name' if arg < ln else 'x_name'] += 1
                s['fit_var' if arg < lv else 'x_var'] += 1
                s['fit_free' if arg < lf else 'x_free'] += 1
                i += 3
    print('files=%d codes=%d' % (files, codes))
    rows = []
    for op in sorted(freq):
        s = stats.get(op)
        if op < 90:
            rows.append((op, freq[op], None))
        else:
            rows.append((op, freq[op], dict(s) if s else {}))
    for op, f, s in rows:
        if s is None:
            print('op %3d  freq %8d  (无参)' % (op, f))
        else:
            n = s.get('n', 0)
            if n == 0:
                continue
            print('op %3d  freq %8d  const:%3d%% name:%3d%% var:%3d%% free:%3d%% trunc:%d' % (
                op, f,
                100*s.get('fit_const',0)//n, 100*s.get('fit_name',0)//n,
                100*s.get('fit_var',0)//n, 100*s.get('fit_free',0)//n,
                s.get('truncated',0)))

if __name__ == '__main__':
    main()
