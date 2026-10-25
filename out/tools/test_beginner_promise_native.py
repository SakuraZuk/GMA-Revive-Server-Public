# -*- coding: utf-8 -*-
"""执行原生启程保底判定、候选权重过滤与收据刷新，不代替Android抽卡验收。"""
from __future__ import print_function
import __builtin__
import json
import marshal
import os
import types

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))

class Obj(object):
    def __init__(self, **values): self.__dict__.update(values)

def load(path):
    with open(os.path.join(root, path), 'rb') as source: return json.load(source)

def native(name, env):
    def find(code):
        if code.co_name == name: return code
        for child in code.co_consts:
            if isinstance(child, types.CodeType):
                found = find(child)
                if found is not None: return found
    with open(os.path.join(root, 'internal/nativepvp/runtime/native_engine/script/custom_types/random_cards_record.pyc'), 'rb') as source:
        source.read(8)
        code = find(marshal.load(source))
    assert code is not None
    return types.FunctionType(code, dict({'__builtins__': __builtin__}, **env))

cards = {int(k): Obj(**v) for k, v in load('out/client_catalogs/tables/cards.json')[u'数据'].items()}
rules = load('out/client_catalogs/tables/cards_promise_rule.json')[u'数据']
catalog = load('internal/game/beginner_promise_catalog.json')['rules']
weights = {int(k): v for k, v in load('internal/game/gacha_native_catalog.json')['pools']['3']['weights']['1'].items()}
seen = []
def choose(filtered):
    seen.append(dict(filtered))
    return min(filtered)
env = {'data': Obj(cards=cards), 'common_const': Obj(PROMISE_TYPE_REPLACE=2),
       'probability_utils': Obj(weighting_choice_with_dict=choose)}
check = native('check_promise_rule', env)
draw = native('do_promise', env)
once = native('get_onetime_promise_card', env)
refresh = native('refresh_record', env)

for item in catalog:
    r = Obj(**rules[str(item['id'])])
    assert item['at'] == r.onetime_count and item['cards'] == r.promise_card_ids
    r.count, r.finish_count = 0, 0
    r.check_promise_rule = lambda card, r=r: check(r, card)
    r.do_promise = lambda pool, r=r: draw(r, pool)
    r.count = r.onetime_count-1
    selected = once(r, None, weights)
    assert selected in item['cards']
    assert seen[-1] == {k:v for k,v in weights.items() if k in item['cards']}
    refresh(r, selected)
    assert r.count == r.onetime_count and r.finish_count == 1
    assert once(r, None, weights) == 0
    r.count, r.finish_count = 0, 0
    refresh(r, selected)
    r.count = r.onetime_count-1
    assert once(r, None, weights) == 0
    if item['at'] > 1:
        r.count, r.finish_count = item['at']-2, 0
        assert once(r, None, weights) == 0

assert all(cards[c].rarity == 4 for c in catalog[5]['cards'])
print('PASS: ten native promise nodes, exact original candidate weights, early hit, boundary and receipt refresh; no Android UI claim')
