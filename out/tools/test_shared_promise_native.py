# -*- coding: utf-8 -*-
"""直接执行原版循环和共享保底函数；只生成规则证据，不修改正式抽卡或玩家存档。"""
from __future__ import print_function, unicode_literals
import imp
import json
import os

base = os.path.dirname(__file__)
original = imp.load_source('beginner_native_evidence', os.path.join(base, 'test_beginner_promise_native.py'))
Obj, native, rules, cards, weights = original.Obj, original.native, original.rules, original.cards, original.weights
rolls = []

def randint(low, high):
    rolls.append([low, high])
    assert low == high == 10
    return low

env = dict(original.env)
env['random'] = Obj(randint=randint)
cycle = native('get_promise_card', env)
once = native('get_onetime_promise_card', env)
refresh = native('refresh_record', env)
check = native('check_promise_rule', env)
draw = native('do_promise', env)

def record(rule_id, count=0, finish=0, real=0):
    value = Obj(**rules[str(rule_id)])
    value.count, value.finish_count, value.real_count = count, finish, real
    value.check_promise_rule = lambda card: check(value, card)
    value.do_promise = lambda pool: draw(value, pool)
    return value

def state(value):
    return {'count': value.count, 'finish_count': value.finish_count, 'real_count': value.real_count}

rows = []
purple = next(k for k in sorted(weights) if cards[k].rarity == 3)
gold = next(k for k in sorted(weights) if cards[k].rarity == 4)
low = next(k for k in sorted(weights) if cards[k].rarity < 3)

r = record(20033)
for i in range(9):
    assert cycle(r, None, weights) == 0
    refresh(r, low)
assert state(r) == {'count': 9, 'finish_count': 0, 'real_count': 10}
selected = cycle(r, None, weights)
assert selected and cards[selected].rarity == 3
refresh(r, selected)
assert state(r) == {'count': 0, 'finish_count': 0, 'real_count': 10}
rows.append({'case': '连续九次未命中后第十抽', 'selected_rarity': cards[selected].rarity, 'after': state(r)})

for cid in [purple, gold]:
    r = record(20033, count=4, real=10)
    refresh(r, cid)
    expected = 0 if cid == purple else 5
    assert r.count == expected and r.finish_count == 0
    rows.append({'case': '提前命中是否重置', 'drawn_rarity': cards[cid].rarity, 'after': state(r)})
assert rolls == [[10, 10]]

for finish in [0, 1, 2]:
    r = record(1060, count=58, finish=finish)
    assert once(r, None, weights) == 0
    r.count = 59
    selected = once(r, None, weights)
    assert bool(selected) == (finish < 2)
    if selected:
        assert check(r, cards[selected])
        refresh(r, selected)
        assert r.count == 60 and r.finish_count == 2
    rows.append({'case': '共享一次性第六十抽', 'previous_finish_count': finish, 'selected': selected, 'after': state(r)})

pools = original.load('out/client_catalogs/tables/cards_random.json')[u'数据']
members = sorted(int(k) for k, v in pools.items() if 1060 in (v.get('promise_rules') or []))
assert 3 in members and len(members) > 1
result = {'状态': '原版函数验证通过', '规则1060所属卡池': members, '用例': rows,
          '边界': '不证明服务端多规则选择顺序；不推算旧档已抽卡稀有度；未改正式抽卡实现或玩家存档'}
target = os.path.join(original.root, u'out/evidence/qq-bugs-20261010/循环与共享保底原生验收.json')
with open(target, 'wb') as output:
    output.write((json.dumps(result, ensure_ascii=False, indent=2) + '\n').encode('utf-8'))
print('PASS: native cycle exact-rarity reset, persisted threshold, shared one-time boundaries; no production mutation')
