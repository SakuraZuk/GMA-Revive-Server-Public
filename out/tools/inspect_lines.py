# -*- coding: utf-8 -*-
"""以 UTF-8 查看指定行，避免 Windows more 对中文重新解码。"""
import sys
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
lines=Path(sys.argv[1]).read_text(encoding='utf-8').splitlines()
start=int(sys.argv[2]);count=int(sys.argv[3]) if len(sys.argv)>3 else 40
for n,line in enumerate(lines[start-1:start-1+count],start):print(f'{n}: {line}')
