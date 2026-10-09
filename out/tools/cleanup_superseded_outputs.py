# -*- coding: utf-8 -*-
"""已确认同源覆盖和校验后，删除指定过期导出与本轮临时工具。"""
import json
from pathlib import Path
import shutil
import sys

ROOT=Path(__file__).resolve().parents[2]
OUT=(ROOT/'out').resolve()
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')
current=OUT/'client_catalogs'
assert json.loads((current/'verification.json').read_text(encoding='utf-8'))['状态']=='通过'
index=json.loads((current/'table-index.json').read_text(encoding='utf-8'))
modules={item['模块']:item for item in index['数据表']}
old=(OUT/'datas').resolve()
if old.exists():
    # 递归删除前验证最终绝对路径，以及全部旧模块的来源与现行输出对应。
    assert old==OUT/'datas' and old.is_relative_to(OUT) and old.is_dir() and not old.is_symlink()
    files=[path for path in old.rglob('*.json') if path.name!='_index.json']
    assert len(files)==703
    for path in files:
        value=json.loads(path.read_text(encoding='utf-8-sig'))
        module=value['module'].replace('\\','/')
        assert module in modules and value['hash']==modules[module]['模块哈希']
        assert (current/modules[module]['导出文件']).exists()
    shutil.rmtree(old)
    print('已删除同源覆盖的旧 out/datas 导出目录。')
targets=(
    'rpc-catalog.json','tools/rpc_catalog.py','tools/data_tables.py',
    'inspect_pg_remote.sh','restart_game_verify.sh','read_project_docs.py',
    'tools/inspect_reverse_tables.py','tools/inspect_catalog_results.py',
    'tools/inspect_export_checks.py','tools/consolidate_docs.py',
)
for relative in targets:
    path=(OUT/relative).resolve()
    assert path.is_relative_to(OUT) and path!=OUT and not path.is_dir()
    if path.exists():
        path.unlink()
        print('已删除过期或临时文件：'+relative)
