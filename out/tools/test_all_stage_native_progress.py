# -*- coding: utf-8 -*-
"""执行本版原生地点完成/解锁函数，核验剧情进度必须在客户端容器中同步。"""
from __future__ import print_function
import json, marshal, os, sys, types

root=os.path.abspath(sys.argv[1])
path=os.path.join(root,'internal/nativepvp/runtime/native_engine/script/common_logic/dungeon_check.pyc')
with open(path,'rb') as stream:
    stream.read(8);code=marshal.load(stream)
def find(c,name):
    if c.co_name==name:return c
    for x in c.co_consts:
        if isinstance(x,types.CodeType):
            v=find(x,name)
            if v is not None:return v
def table(name):
    with open(os.path.join(root,'out/client_catalogs/tables',name+'.json'),'rb') as stream:
        return json.load(stream)[u'数据']
def row(values):
    return type('Row',(object,),values)()
sites={int(k):row(v) for k,v in table('sites').items()}
index=table('chapter_site_dungeon')
index['site_dict']={int(k):v for k,v in index['site_dict'].items()}
index['site_unlock_dict']={int(k):v for k,v in index['site_unlock_dict'].items()}
data=types.ModuleType('data');data.sites=sites;data.chapter_site_dungeon=index
scope={'__builtins__':__import__('__builtin__'),'data':data,
       'check_chapter_unlock':lambda *args:True,'check_chapter_finished':lambda *args:True}
for name in ('check_dungeon_finished','check_site_finished','check_site_unlock'):
    scope[name]=types.FunctionType(find(code,name),scope)
avatar=row({'dungeon_mgr':{602:row({'finished':1}),603:row({'finished':0})}})
assert not scope['check_site_unlock'](604,avatar)
# 模拟旧服务端只更新内部通关列表；原生世界地图不会读取该字段。
avatar.cleared_dungeons=[602,603]
assert not scope['check_site_unlock'](604,avatar)
avatar.dungeon_mgr[603]=row({'finished':1})
assert scope['check_site_unlock'](604,avatar)
avatar=row({'dungeon_mgr':{int(k):row({'finished':1}) for k in table('dungeons')}})
checked=0
for site,predecessors in index['site_unlock_dict'].items():
    if site not in sites:continue
    assert scope['check_site_unlock'](site,avatar)
    for predecessor in predecessors:
        assert scope['check_site_finished'](predecessor,avatar)
    checked+=1
print('2-3旧内部进度无法解锁后继已复现；同步原生finished后后继解锁通过')
print('原生地点前置索引%d项通过；章节条件为夹具，不代表全部章节门槛或Android通关' % checked)
