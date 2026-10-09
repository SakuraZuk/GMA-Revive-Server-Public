# -*- coding: utf-8 -*-
"""对全部 opcode 找自洽的"未知 3 字节指令"插入方案:
- 跳转指令的 arg(或相对量)必须 100% 落在指令边界
- 逐个候选独立验证(在某 opcode 位置插入候选规则,不与其他未知冲突)
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

JUMPS_ABS = {110, 111, 112, 114, 115, 119, 121, 122, 141, 142}
JUMPS_REL = {93, 120}
# 已按 3 字节非跳转解释的 opcode(handler 或统计已确认)
KNOWN_OPS = {90, 95, 96, 97, 99, 100, 101, 102, 103, 104, 105, 113, 116,
             118, 123, 125, 128, 129, 130, 131, 132, 134, 135, 136, 137,
             138, 145, 148, 149, 151, 153, 155, 156, 157, 158, 159, 160, 172, 173}

def main():
    codes = []
    for mf in sorted(Path(r'.\out\npk_scripts\android_base').glob('*.marshal'))[:400]:
        try:
            code = Loader(mf.read_bytes()).r_object()
        except Exception:
            continue
        codes.extend(walk(code))
    print('codes', len(codes))

    def instr_positions(c, unknown=None, ukrel=None):
        """返回 (pos_set, ok)。unknown: 按跳转处理(尝试 abs)或 ukrel(相对)。"""
        b = c['bytecode']; n = len(b)
        pos = set(); i = 0
        while i < n:
            op = b[i]
            pos.add(i)
            if op < 90:
                i += 1
            elif unknown is not None and op == unknown:
                i += 3
            else:
                i += 3
        pos.add(n)
        return pos, b

    base_positions = []
    for c in codes:
        pos, b = instr_positions(c)
        base_positions.append((pos, b))

    freqs = Counter()
    for pos, b in base_positions:
        for p in pos:
            if p < len(b):
                freqs[b[p]] += 1
    # 未识别 opcode:既非本方案已知 op,且不在跳转候选中
    unknown_ops = [op for op in freqs if op >= 90 and op not in KNOWN_OPS and op not in JUMPS_ABS and op not in JUMPS_REL]
    print('未知 opcode:', unknown_ops)
    for op in unknown_ops:
        absok = relok = total = 0
        for pos, b in base_positions:
            if op not in [b[p] for p in pos if p < len(b)]:
                continue
            i = 0; n = len(b)
            while i < n:
                o = b[i]
                if o < 90:
                    i += 1; continue
                arg = b[i+1] | (b[i+2] << 8)
                if o == op:
                    total += 1
                    if arg in pos: absok += 1
                    if i + 3 + arg in pos or (i + arg) in pos: relok += 1
                    i += 3
                else:
                    i += 3
        print('op %3d freq %d  abs对齐 %d/%d  rel(i+3+a/i+a)对齐 %d' % (op, freqs[op], absok, total, relok))

if __name__ == '__main__':
    main()
