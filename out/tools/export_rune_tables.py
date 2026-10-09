# -*- coding: utf-8 -*-
"""从 Android 导出表合成服务端内嵌契印表 internal/game/rune_tables.json。

行数据全部来自 out/client_catalogs/tables 的 runes 系列表；runes_level /
limits / errors 三个整理常量沿用 GMA-Revive-Server data/rune_tables.json
（其行数据与本项目 android_base 导出逐行一致，已抽查核对），在脚本内以
字面量形式固化，避免运行时依赖参考项目目录。
"""
import hashlib
import json
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')

# 参考项目整理常量（客户端 datas/runes.py 的模块级常量与错误码表）。
RUNES_LEVEL = {"max": 11, "max_pos": 4, "max_star": 5, "min": 1,
               "unlock_limit": {"0": 3, "20": 5, "30": 7, "40": 9, "50": 11}}
LIMITS = {"MAX_RUNE_COUNT": 6000}
ERRORS = {
    "RET_SUCCESS": 0, "RET_CARD_NOT_EXIST": 3, "RET_MATERIAL_NOT_ENOUGH": 5, "RET_GOLD_NOT_ENOUGH": 7,
    "RET_RUNE_NOT_EXIST": 5101, "RET_RUNE_REACH_MAX_LEVEL": 5102, "RET_RUNE_NOT_EMBED": 5104,
    "RET_RUNE_MAX_COUNT_EXCEED": 5105, "RET_RUNE_TEMPLATE_MAX_COUNT_LIMIT": 5106,
    "RET_RUNE_TEMPLATE_NAME_TOO_LONG": 5107, "RET_RUNE_TEMPLATE_DELETE_SUCCEED": 5110,
    "RET_RUNE_MAX_COUNT_LIMIT": 5111, "RET_RUNE_EMBED_OTHER_CARD": 5112, "RET_RUNE_LOCKED": 5113,
    "RET_RUNE_SELECT_SAME_SUIT": 5115, "RET_RUNE_SELECT_SUIT_MAX": 5116,
    "RET_RUNE_SUIT_INVALID": 5117, "RET_RUNE_STAR_INVALID": 5118, "RET_RUNE_POS_INVALID": 5119,
    "RET_RUNE_ATTR_INVALID": 5120, "RET_RUNE_LOCK_SETTING_REPEAT": 5121,
    "RET_RUNE_LOCK_SETTING_NOT_EXIST": 5122, "RET_RUNE_LOCK_SUIT_EMPTY": 5123,
    "RET_RUNE_LOCK_STAR_EMPTY": 5124, "RET_RUNE_LOCK_POS_EMPTY": 5125, "RET_RUNE_LOCK_ATTR_EMPTY": 5126,
    "RET_RUNE_CHANGE_ATTR_INDEX_ERROR": 5127, "RET_RUNE_ATTR_ID_ERROR": 5128,
    "RET_RUNE_CANNOT_RESET_IT": 5129, "RET_DECOMPOSE_RUNE_SINGLE_COUNT_LIMIT": 5251,
    "RET_DECOMPOSE_RUNE_EMPTY": 5252, "RET_RUNE_SHOP_POLISH_MAX": 36001,
    "RET_RUNE_SHOP_POLISH_FINISH": 36002, "RET_RUNE_SHOP_POLISH_NO_RUNE": 36003,
    "RET_RUNE_SHOP_OPEN_STONE_MAX": 36004,
}

# 期望行数与参考项目导出核对（runes=模板、attrs=词条、levels=强化、suits=套装、
# reset=洗练、unlock=词条解锁）。不一致即中止，防止静默换表。
EXPECTED = {"runes": 20, "runes_attrs": 119, "runes_levels": 55, "runes_suits": 38,
            "runes_reset": 1, "runes_unlock_attrs": 11}


def load_rows(name):
    path = ROOT / 'out/client_catalogs/tables' / (name + '.json')
    raw = path.read_bytes()
    table = json.loads(raw)
    assert table.get('状态') == '静态结构还原通过', name + ' 导出状态异常'
    data = table['数据']
    if isinstance(data, dict) and len(data) == 1 and isinstance(next(iter(data.values())), list):
        items = next(iter(data.values()))  # 复合键表：[{"键":[...],"值":{...}}]
        rows = [item['值'] for item in items]
    elif isinstance(data, dict):
        rows = list(data.values())  # 简单键表：{"1":{...}}
    else:
        rows = list(data)
    assert len(rows) == EXPECTED[name], f'{name} 行数 {len(rows)} 与基线 {EXPECTED[name]} 不符'
    return rows, {'路径': str(path.relative_to(ROOT)), '导出SHA256': hashlib.sha256(raw).hexdigest(),
                  '模块SHA256': table['来源SHA256'], '条目数': len(rows)}


def main():
    tables, evidence = {}, {}
    for name in EXPECTED:
        tables[name], evidence[name] = load_rows(name)
    tables.update({'runes_level': RUNES_LEVEL, 'limits': LIMITS, 'errors': ERRORS})
    # 关键字段完整性抽查：模板复合键、词条权重与互斥、强化材料对。
    assert any(r['position'] == 1 and r['star'] == 1 for r in tables['runes'])
    assert any(r['attr_weight'] > 0 for r in tables['runes_attrs'])
    assert any(r['upgrade_material'] for r in tables['runes_levels'])
    out = {'生成时间': time.strftime('%Y-%m-%d %H:%M:%S'),
           '来源': evidence,
           '常量来源': 'runes_level/limits/errors 为客户端 datas/runes.py 模块常量与错误码的整理值'
                      '（沿用 GMA-Revive-Server 同源导出，行数据全部来自本项目导出表）',
           'tables': tables}
    target = ROOT / 'internal/game/rune_tables.json'
    target.write_text(json.dumps(out, ensure_ascii=False, indent=1, sort_keys=True), encoding='utf-8')
    print('生成 ' + str(target.relative_to(ROOT)) + ' 行数 ' +
          json.dumps({k: len(v) for k, v in tables.items() if isinstance(v, list)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
