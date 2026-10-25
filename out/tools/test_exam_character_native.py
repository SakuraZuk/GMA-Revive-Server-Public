# -*- coding: utf-8 -*-
"""原版同桌活动角色选择：资格、双参回调、男女材料判定；图形剧情不在此验收。"""
from __future__ import print_function
import json
import glob
import marshal
import os
import sys
import types

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
scripts = os.path.join(root, 'internal/nativepvp/runtime/native_engine/script')
sys.path.insert(0, scripts)
sys.path.insert(0, os.path.join(root, 'internal/nativepvp'))
sys.path.extend(glob.glob(os.path.join(root, 'internal/nativepvp/runtime/native_engine/deps/*.whl')))
from battle_native_host import Host
Host().install()
import common_const
import error_code


class Obj(object):
    def __init__(self, **values):
        self.__dict__.update(values)


path = os.path.join(scripts, 'entities/components/exam_mgr.pyc')
with open(path, 'rb') as stream:
    stream.read(8)
    code = marshal.load(stream)


def find(parent, name):
    if parent.co_name == name:
        return parent
    for child in parent.co_consts:
        if isinstance(child, types.CodeType):
            value = find(child, name)
            if value is not None:
                return value


table = json.load(open(os.path.join(root, 'out/client_catalogs/tables/storyline_bonus.json'), 'rb'))[u'数据'][u'键值条目']
rows = dict((tuple(row[u'键']), Obj(**row[u'值'])) for row in table)
scope = {'data': Obj(storyline_bonus=rows), 'common_const': common_const,
         'error_code': error_code, 'gworld': Obj(get_player=lambda: Obj(hostnum=1)),
         'activity_utils': Obj(check_activity_open=lambda activity, host: error_code.RET_SUCCESS)}
check = types.FunctionType(find(code, 'check_receive_storyline_bonus'), scope)
receive = types.FunctionType(find(code, 'receive_storyline_bonus'), scope)
gender = types.FunctionType(find(code, 'get_gaokao_character_gender'), scope)
file_name = 'story/gaokao/story/gk1_1_1b'
node = 'choose_character_53'
for option, material, expected in [('A', 209011, common_const.BOY_ROLE),
                                    ('B', 209012, common_const.GIRL_ROLE)]:
    errors, calls = [], []
    owner = Obj(storyline_bonus_dict={}, get_material_count=lambda mid: int(mid == material),
                check_ret_success=lambda ret: ret == error_code.RET_SUCCESS,
                show_error_tips=lambda ret: errors.append(ret),
                call_server=lambda method, callback, *args: calls.append((method, callback, args)))
    owner.check_receive_storyline_bonus = lambda f, n, o: check(owner, f, n, o)
    assert check(owner, file_name, node, option) == error_code.RET_SUCCESS
    receive(owner, file_name, node, option)
    assert len(calls) == 1 and calls[0][0] == 'receive_storyline_bonus'
    assert calls[0][2] == (file_name, node, option)
    calls[0][1](error_code.RET_SUCCESS, {})
    assert errors == [error_code.RET_SUCCESS]
    assert gender(owner) == expected
    owner.storyline_bonus_dict[(file_name+'_'+node).replace('.', '')] = option
    assert check(owner, file_name, node, option) == error_code.RET_EXAM_RECV_BONUS_REPEAT
    assert check(owner, file_name, node, 'invalid') == error_code.RET_EXAM_NO_BONUS
print('原版男女选项资格、实际双参closure回调、角色材料判定与重复拒绝通过；未验证图形剧情。')
