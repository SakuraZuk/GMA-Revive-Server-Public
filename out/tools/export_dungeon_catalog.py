# -*- coding: utf-8 -*-
"""从 Android 导出表合成全量副本目录 internal/game/dungeon_catalog.json。

客户端权威战斗模式下服务端只需要每副本的最小规则字段：
dungeon_battle_id（0/空=剧情节点）、dungeon_type、need_power、
need_power为空时按D81B4FE0原生get_need_power回退活动类型基础体力；
完整奖励由Android原生bonus解释器负责，目录只保存战斗/类型/基础费用。
"""
import hashlib
import json
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')


def main():
    path = ROOT / 'out/client_catalogs/tables/dungeons.json'
    raw = path.read_bytes()
    table = json.loads(raw)
    activity_path = ROOT / 'out/client_catalogs/tables/activity_type.json'
    activity_raw = activity_path.read_bytes()
    activities = json.loads(activity_raw)['数据']
    assert table.get('状态') == '静态结构还原通过', 'dungeons 导出状态异常'
    catalog = {}
    for key, row in table['数据'].items():
        dungeon_id = int(row['dungeon_id'])
        activity_id = int(row.get('dungeon_type') or 1)
        power = row.get('need_power')
        if power is None:
            power = activities.get(str(activity_id), {}).get('need_power') or 0
        assert isinstance(power, int) and not isinstance(power, bool) and power >= 0, '副本体力配置异常'
        catalog[str(dungeon_id)] = {
            'battle_id': int(row.get('dungeon_battle_id') or 0),
            'type': activity_id,
            'power': int(power) if isinstance(power, (int, float)) else 0,
        }
    out = {'生成时间': time.strftime('%Y-%m-%d %H:%M:%S'),
           '来源': {'路径': str(path.relative_to(ROOT)),
                    '导出SHA256': hashlib.sha256(raw).hexdigest(),
                     '模块SHA256': table['来源SHA256'], '条目数': len(catalog),
                     '活动类型导出SHA256': hashlib.sha256(activity_raw).hexdigest(),
                     '活动类型模块SHA256': json.loads(activity_raw)['来源SHA256'],
                     '费用规则': 'D81B4FE0 get_need_power：need_power非None取副本值，否则活动类型need_power；double另行授权'},
           'dungeons': catalog}
    target = ROOT / 'internal/game/dungeon_catalog.json'
    target.write_text(json.dumps(out, ensure_ascii=False, sort_keys=True), encoding='utf-8')
    battles = sum(1 for v in catalog.values() if v['battle_id'] > 0)
    print('生成 %s 副本 %d（战斗 %d / 剧情 %d）' % (target.relative_to(ROOT), len(catalog), battles,
                                               len(catalog) - battles))


if __name__ == '__main__':
    main()
