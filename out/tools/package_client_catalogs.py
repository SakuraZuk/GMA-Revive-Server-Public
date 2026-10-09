# -*- coding: utf-8 -*-
"""打包静态导出、验证证据、工具和现行规格，校验 ZIP 内容完整性。"""
import hashlib
import json
from pathlib import Path
import sys
import zipfile

ROOT=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')
catalog=ROOT/'out/client_catalogs'
verification=json.loads((catalog/'verification.json').read_text(encoding='utf-8'))
if verification['状态']!='通过': raise ValueError('目录尚未通过校验')
files=sorted(catalog.rglob('*.json'))
files.extend(ROOT/relative for relative in (
    'out/HANDOFF-REVERSE.md','out/gate-protocol-spec.md','out/login-flow-spec.md',
    'out/rpc-semantics.md','out/opcode-recovery.md','out/npk-format.md',
    'out/references/rotor-algorithm.md','out/time-contract-verification.json',
    'out/dis/store-opcode-evidence.asm','out/tools/export_client_catalogs.py',
    'out/tools/verify_client_catalogs.py','out/tools/verify_store_opcodes.py',
    'out/tools/neox_dis.py','out/tools/mem_marshal_extract.py'))
manifest={}
destination=ROOT/'out/client-catalogs.zip'
temporary=destination.with_suffix('.zip.tmp')
with zipfile.ZipFile(temporary,'w',compression=zipfile.ZIP_DEFLATED,compresslevel=6) as archive:
    for path in files:
        relative=path.relative_to(ROOT/'out').as_posix()
        raw=path.read_bytes()
        manifest[relative]={'字节数':len(raw),'SHA256':hashlib.sha256(raw).hexdigest()}
        archive.writestr(relative,raw)
    archive.writestr('文件校验清单.json',json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
with zipfile.ZipFile(temporary) as archive:
    bad=archive.testzip()
    if bad: raise ValueError('ZIP 损坏条目：'+bad)
    for relative,entry in manifest.items():
        if hashlib.sha256(archive.read(relative)).hexdigest()!=entry['SHA256']:
            raise ValueError('ZIP 文件哈希不符：'+relative)
temporary.replace(destination)
print(json.dumps({'状态':'打包并校验通过','路径':str(destination),'文件数':len(manifest),
                 '压缩包字节数':destination.stat().st_size,'SHA256':hashlib.sha256(destination.read_bytes()).hexdigest()},ensure_ascii=False,indent=2))
