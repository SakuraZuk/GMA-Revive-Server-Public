# -*- coding: utf-8 -*-
"""运行原版房间开放判定；不加载引擎、不连接线上、不修改玩家数据。"""
from __future__ import print_function
import hashlib
import json
import marshal
import os
import types

root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
path = os.path.join(root, 'internal/nativepvp/runtime/native_engine/script/guis/gui_utils.pyc')
raw = open(path, 'rb').read()
code = marshal.loads(raw[8:])


def find(parent, name):
    for item in parent.co_consts:
        if isinstance(item, types.CodeType):
            if item.co_name == name:
                return item
            result = find(item, name)
            if result is not None:
                return result


class Record(object):
    def __init__(self, **values):
        self.__dict__.update(values)


catalog = json.load(open(os.path.join(root, 'internal/game/collection_catalog.json'), 'rb'))
rooms = dict((int(key), Record(room_system_name=value['room_system_name']))
             for key, value in catalog['rooms'].items())
player = Record(unlock_systems={})
native = types.FunctionType(find(code, 'is_area_opened'), {
    'data': Record(dormitory_attrs=rooms),
    'gworld': Record(get_player=lambda: player),
})
assert sorted(rooms) == [1, 2, 3]
checks = 0
for room in sorted(rooms):
    system = rooms[room].room_system_name
    player.unlock_systems = {}
    assert native(room) == 0
    for version in (0, 1, 2):
        player.unlock_systems = {system: version}
        assert native(room) == version
        checks += 1
        for other in rooms:
            if other != room:
                assert native(other) == 0
print((u'通过：原版is_area_opened共%d组房间/版本判定；SHA256=%s' %
       (checks, hashlib.sha256(raw).hexdigest())).encode('utf-8'))
print(u'边界：原版Python2函数与数据/角色夹具；不是Android界面验收。'.encode('utf-8'))
