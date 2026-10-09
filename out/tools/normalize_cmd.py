# -*- coding: utf-8 -*-
"""将指定项目 CMD 文件统一为 UTF-8、CRLF，避免 cmd 对 LF 文件误解析。"""
from pathlib import Path
import sys

root = Path(__file__).resolve().parents[2]
for name in sys.argv[1:]:
    path = (root/name).resolve()
    if not path.is_relative_to(root) or path.suffix.lower() != '.cmd':
        raise SystemExit('只允许项目内的 CMD 文件')
    text = path.read_text(encoding='utf-8')
    path.write_bytes(text.replace('\r\n','\n').replace('\n','\r\n').encode('utf-8'))
