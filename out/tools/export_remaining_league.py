# -*- coding: utf-8 -*-
"""导出本版学会规则，保留原生来源，不执行游戏业务字节码。"""
import hashlib
import json
import sys
from pathlib import Path
from mem_marshal_extract import Loader

ROOT = Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
sources = {}

def load(path):
    raw = path.read_bytes()
    sources[str(path.relative_to(ROOT))] = hashlib.sha256(raw).hexdigest()
    return json.loads(raw)

tables = {name: load(ROOT / ('out/client_catalogs/tables/' + name + '.json'))['数据'] for name in (
    'league_base', 'league_level', 'league_protect', 'league_power', 'league_card_base', 'league_card_grade', 'league_card_exp',
    'league_card_enhance', 'league_card_skill', 'league_card_skill_unlock', 'cards', 'cards_rarity', 'league_explore',
    'new_league_challenge', 'new_league_challenge_damage_bonus', 'league_spacetime',
)}
constants = load(ROOT / 'out/client_catalogs/configs/const.json')['直接常量']
constants = {k: v for k, v in constants.items() if k.startswith('MAX_LEAGUE')}
common = load(ROOT / 'out/client_catalogs/configs/common_const.json')['直接常量']
constants.update({k: v for k, v in common.items() if k.startswith('LEAGUE_MEMBER_TYPE_')})
inv = load(ROOT / 'out/npk_scripts/android_inventory.json')
path = ROOT / 'out/npk_scripts' / next(r['marshal_file'] for r in inv['modules'] if r['hash'] == '875C5BCF')
raw = path.read_bytes()
sources[str(path.relative_to(ROOT))] = hashlib.sha256(raw).hexdigest()
code = Loader(raw).r_object()
errors = {}
b = code['bytecode']
offset = 0
while offset < len(b):
    if b[offset] == 153 and offset + 6 <= len(b) and b[offset + 3] == 116:
        name = code['names'][int.from_bytes(b[offset+4:offset+6], 'little')].decode('utf-8')
        if name.startswith('RET_LEAGUE') or name in ('RET_SUCCESS', 'RET_FAILED'):
            errors[name] = code['consts'][int.from_bytes(b[offset+1:offset+3], 'little')]
    offset += 1 if b[offset] < 90 else 3
out = ROOT / 'internal/game/league_catalog.json'
out.write_text(json.dumps({'来源': sources, '表': tables, '常量': constants, '错误码': errors}, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print('已导出学会规则和来源SHA')
