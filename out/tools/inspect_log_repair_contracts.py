# -*- coding: utf-8 -*-
"""汇集日志入口的原生函数、调用点和文档读取校验，不执行客户端代码。"""
import hashlib
import io
import json
import sys
from pathlib import Path
from mem_marshal_extract import Loader
from neox_dis import disasm

sys.stdout.reconfigure(encoding='utf-8')
root = Path(__file__).resolve().parents[2]
destination = root / 'out/evidence/player-bugs-20261009/repair12'
destination.mkdir(parents=True, exist_ok=True)
names = {'change_dungeon_skip_edit_state', 'set_dungeon_power_type',
         'set_auto_battle_habit', 'on_set_auto_battle_habit', 'set_asyn_pvp_auto',
         'set_focus_target', 'on_set_focus_target', 'exit_battle',
         'consume_power_material', 'is_power_material', 'get_avatar_back_total_num',
         'query_phone_info', 'request_chat_channel', 'lock_card', 'unlock_card',
         'summer_game_enter_node', 'summer_game_clear_sp', 'receive_summer_banner_bonus',
         'refuse_challenge'}
paths = {p.name: p for p in (root/'out/npk_scripts/android_base').glob('*.marshal')}
paths.update({p.name: p for p in (root/'out/npk_scripts/android_patch').glob('*.marshal')})
hits = []
two_arg_callbacks = []
for path in sorted(paths.values()):
    raw = path.read_bytes()
    if not any(name.encode() in raw for name in names) and b'_callback' not in raw:
        continue
    stream = io.StringIO()
    def visit(code, prefix=''):
        name = code['name'].decode('utf-8') if isinstance(code['name'], bytes) else code['name']
        full = prefix + '.' + name
        if name == '_callback' and code['argcount'] == 2:
            two_arg_callbacks.append({'模块':path.stem, '函数':full,
                                      '形参':[v.decode('utf-8') if isinstance(v, bytes) else v for v in code['varnames'][:2]]})
        # 方法本身及跳过开关实际调用点；不打印根模块的全量字节码。
        caller_names = code.get('names', [])
        caller = any(word in caller_names or word.encode() in caller_names for word in ('change_dungeon_skip_edit_state','refuse_challenge'))
        definitions = [child.get('name') for child in code.get('consts', []) if isinstance(child, dict) and child.get('type') == 'code']
        if any(word in definitions or word.encode() in definitions for word in ('change_dungeon_skip_edit_state','refuse_challenge')):
            caller = False  # 类体定义方法不等于实际调用点。
        if name in names or name != '<module>' and caller:
            disasm(code, prefix=prefix, out=stream)
            return
        for child in code.get('consts', []):
            if isinstance(child, dict) and child.get('type') == 'code':
                visit(child, full)
    visit(Loader(raw).r_object())
    if not stream.getvalue():
        continue
    output = destination / (path.stem + '.log')
    output.write_text('来源：'+path.relative_to(root).as_posix()+'\nSHA256：'+hashlib.sha256(raw).hexdigest()+'\n'+stream.getvalue(), encoding='utf-8')
    hits.append({'模块': path.stem, '证据': output.relative_to(root).as_posix()})
mds = []
for path in sorted(root.rglob('*.md')):
    if '.git' in path.parts or '_buildcache' in path.parts:
        continue
    raw = path.read_bytes()
    text = raw.decode('utf-8-sig')
    mds.append({'文件':path.relative_to(root).as_posix(), '行数':len(text.splitlines()),
                'SHA256':hashlib.sha256(raw).hexdigest(), '替换字符':text.count('\ufffd')})
(destination/'文档与原包读取.json').write_text(json.dumps({'文档':mds, '原生证据':hits, '二参数回调':two_arg_callbacks},ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'文档数':len(mds), '证据模块':hits},ensure_ascii=False))
print(json.dumps({'二参数回调':two_arg_callbacks},ensure_ascii=False))
