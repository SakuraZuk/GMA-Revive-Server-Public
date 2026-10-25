# -*- coding: utf-8 -*-
"""用真实16包索引执行原版补丁器；候选表和偏移修正均不发布。"""
from __future__ import print_function
import __builtin__, json, marshal, os, struct, time, types

root = os.getcwd()
evidence = os.path.join(root, 'out/evidence/qq-bugs-20261010')
with open(os.path.join(root, 'internal/nativepvp/runtime/native_engine/script/patch_logic/patcher.pyc'), 'rb') as source:
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

table_path = os.path.join(evidence, u'完整逐文件校验候选.json')
table = json.load(open(table_path, 'rb'))
report = json.load(open(os.path.join(evidence, u'完整逐文件校验候选.report.json'), 'rb'))
old = json.load(open(os.path.join(root, 'deploy/data/npk_filelist_md5.txt'), 'rb'))
assert table['script'] == old['script']
scope = {'__builtins__': __builtin__, 'struct': struct, 'time': time,
         'patch_utils': Obj(NPK_INDEX_SIZE=28), 'patch_log': lambda *args: None}
native_class = find(code, 'npk_patcher')
set_info = types.FunctionType(find(native_class, 'set_npkfilelist_md5_info'), scope)
check = types.FunctionType(find(native_class, '_check_file'), scope)
results = []
for row in report[u'资源核验']:
    path = os.path.join(root, row[u'源'])
    with open(path, 'rb') as source:
        header = source.read(24)
        count = struct.unpack_from('<I', header, 4)[0]
        index = struct.unpack_from('<I', header, 20)[0]
        source.seek(index)
        raw = source.read(count * 28)
    p = Obj(npk_name=row[u'资源'], name=row[u'资源'], can_download_full_npk=False,
            index_count=count, full_indices=raw, npk_app=None, npk_old=None)
    set_info(p, table)
    check(p, False)
    assert len(p.download_filelist) == row[u'校验条目数']
    invalid = sum(offset + size > row[u'大小'] for fid, offset, size, h, md in p.download_filelist)
    assert invalid == row[u'编码偏移条目']
    restored = []
    for i in range(count):
        values = list(struct.unpack_from('<IIIIIII', raw, i * 28))
        if values[6] >= 62:
            values[1] ^= ((-100 - values[3]) & 0xffffffff) ^ 0x7a090d89
            values[6] -= 62
        restored.append(struct.pack('<IIIIIII', *values))
    p.full_indices = b''.join(restored)
    check(p, False)
    assert len(p.download_filelist) == row[u'校验条目数']
    assert all(24 <= offset and offset + size <= row[u'大小'] for fid, offset, size, h, md in p.download_filelist)
    results.append({'npk': row[u'资源'], 'entries': len(p.download_filelist), 'raw_offset_outside': invalid, 'restored_offset_outside': 0})
assert len(results) == 16
print(json.dumps({'results': results, 'entries': sum(r['entries'] for r in results)}, ensure_ascii=True))
print('原生通过：完整表覆盖16包105734条；15包105722条原始编码偏移在下载队列越界，还原索引后队列偏移全部有效。尚未验证原生写包、HTTP实际Range和Android启动，不可据此发布。')
