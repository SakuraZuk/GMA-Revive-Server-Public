# -*- coding: utf-8 -*-
"""以原生Python2函数复现非整包更新遇到缺失校验表；索引为单条夹具。"""
from __future__ import print_function
import __builtin__, json, marshal, os, struct, time, types

root = os.getcwd()
path = os.path.join(root, 'internal/nativepvp/runtime/native_engine/script/patch_logic/patcher.pyc')
with open(path, 'rb') as source:
    source.read(8)
    code = marshal.load(source)

def find(parent, name):
    for item in parent.co_consts:
        if isinstance(item, types.CodeType):
            if item.co_name == name:
                return item
            found = find(item, name)
            if found:
                return found

class Obj(object):
    def __init__(self, **values):
        self.__dict__.update(values)

native_class = find(code, 'npk_patcher')
scope = {'__builtins__': __builtin__, 'struct': struct, 'time': time,
         'patch_utils': Obj(NPK_INDEX_SIZE=28), 'patch_log': lambda *args: None}
set_info = types.FunctionType(find(native_class, 'set_npkfilelist_md5_info'), scope)
check = types.FunctionType(find(native_class, '_check_file'), scope)
table = json.load(open(os.path.join(root, 'deploy/data/npk_filelist_md5.txt')))
assert 'char1' not in table
patcher = Obj(npk_name='char1', name='char1', can_download_full_npk=False,
              index_count=1, full_indices=struct.pack('IIIIIII', 123, 24, 8, 8, 1, 1, 0),
              npk_app=None, npk_old=None)
set_info(patcher, table)
try:
    check(patcher, False)
except KeyError as error:
    assert error.args == (123,)
else:
    raise AssertionError('缺少校验表时没有复现原生KeyError')
set_info(patcher, {'char1': [123, '0123456789abcdef0123456789abcdef']})
check(patcher, False)
assert len(patcher.download_filelist) == 1 and patcher.patch_size == 8
print('原生通过：缺失char1逐文件表可复现KeyError；完整单条夹具可进入下载队列。不是vivo故障归因，也不是正式资源校验表。')
