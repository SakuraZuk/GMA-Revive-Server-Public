# -*- coding: utf-8 -*-
"""验证导出覆盖、关键客户端契约和原始表达式完整性。"""
import base64
import hashlib
import json
from pathlib import Path
import sys

ROOT=Path(__file__).resolve().parents[2]
DEST=ROOT/'out/client_catalogs'
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')
def load(path):
    text=path.read_text(encoding='utf-8')
    if '\ufffd' in text: raise ValueError('产物含替换乱码字符：'+str(path))
    return json.loads(text)
index=load(DEST/'table-index.json')
assert len(index['数据表'])==703
rows=0
expressions=0
rawbytes=0
def verify(value):
    global expressions,rawbytes
    if isinstance(value,dict):
        if '未执行表达式' in value: expressions+=1
        if '原始字节base64' in value:
            rawbytes+=1
            base64.b64decode(value['原始字节base64'],validate=True)
        if '字节码base64' in value:
            raw=base64.b64decode(value['字节码base64'],validate=True)
            assert hashlib.sha256(raw).hexdigest()==value['字节码SHA256']
        for item in value.values(): verify(item)
    elif isinstance(value,list):
        for item in value: verify(item)
for entry in index['数据表']:
    doc=load(DEST/entry['导出文件'])
    assert doc['来源SHA256']==entry['来源SHA256']
    if entry['状态']=='包初始化模块': continue
    assert entry['状态']=='静态结构还原通过'
    rows+=entry['条目数']
    verify(doc['数据'])
    assert isinstance(doc['模块常量'],dict)
head=load(DEST/'tables/head_box_info.json')
assert head['数据']['1']=={'_id':1,'head_type':1,'icon_id':2001,'limit_days':None,'unlock_target_id':None,'unlock_dress_id':None,'bind_gender':1,'rank':1,'lock_type':0,'desc':None}
hotfix=load(DEST/'tables/hotfix_data.json')
assert hotfix['数据']=={'index':0,'script':'\n'}
catalog=load(DEST/'rpc-catalog.json')
registered={(item['模块'],item['方法']):item for item in catalog['显式装饰方法']}
assert registered[('entities/Account.py','login_result')]['签名']=='Int, Str, Int'
assert registered[('entities/Account.py','login_result')]['默认参数数量']==0
assert registered[('entities/Account.py','login_result')]['参数名']==['self','ret_code','reason','conn_type']
assert registered[('entities/components/login.py','sync_server_time')]['签名']=='Float, Int'
assert registered[('entities/Avatar.py','on_refresh_login')]['签名']==''
assert ('entities/Avatar.py','on_login_success') not in registered
assert registered[('entities/components/anti_dulgence_mgr.py','on_set_aas_reason')]['装饰器选项']=='client'
summary=load(DEST/'summary.json')
assert rows==summary['数据条目总数'] and expressions==summary['未执行表达式引用数'] and rawbytes==summary['原始字节标记数']
assert not catalog['复杂注册候选']
result={'状态':'通过','校验数据模块数':703,'静态数据定义数':701,'数据条目数':rows,
        '表达式证据引用数':expressions,'原始字节证据数':rawbytes,
        '范围':'结构、来源、表达式完整性与代表契约；未执行游戏逻辑'}
(DEST/'verification.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps(result,ensure_ascii=False,indent=2))
