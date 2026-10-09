# -*- coding: utf-8 -*-
"""原生录像扩展的文件/幂等/回调续传模型测试，不执行Android战斗数值。"""
import builtins
import contextlib
import hashlib
import io
import json
import os
import pickle
import sys
import tempfile
import types
import zlib
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
class BinaryCompatible:
    def __init__(self,handle):self.handle=handle
    def write(self,value):return self.handle.write(value.encode('utf-8') if isinstance(value,str) else value)
    def __getattr__(self,name):return getattr(self.handle,name)
def compatible_open(filename,mode):return BinaryCompatible(builtins.open(filename,mode))
class Shadow:
    _revival_bridge_version=15
    def notify_battle_finish(self):self.notified=getattr(self,'notified',0)+1;return '原结果'
def main():
    modules={}
    def module(name):value=types.ModuleType(name);modules[name]=value;return value
    gworld=module('gworld');game3d=module('game3d');version=module('version');cpickle=module('cPickle');battle=module('battle_logic');client=module('battle_logic.client_battle')
    # Android/Linux rename覆盖旧文件；Windows宿主模型用replace复现同一语义。
    platform_os=module('os');platform_os.__dict__.update(vars(os));platform_os.rename=os.replace
    version.VERSION='1.0.128';cpickle.dumps=lambda value:pickle.dumps(value,protocol=0);client.shadow_battle=Shadow;battle.client_battle=client
    scheduled=[];calls=[]
    owner='123456789012345678901234';enemy='234567890123456789012345';uuid='345678901234567890123456'
    player=types.SimpleNamespace(eid=owner,call_server=lambda name,*args,**kw:calls.append((name,args,kw['callback'])))
    gworld.get_player=lambda:player;game3d.delay_exec=lambda ms,fn:scheduled.append((ms,fn))
    old_modules={name:sys.modules.get(name) for name in modules};sys.modules.update(modules)
    try:
        with tempfile.TemporaryDirectory() as temporary:
            game3d.get_doc_dir=lambda:temporary
            scope={'open':compatible_open};source=(ROOT/'internal/game/native_record_bridge_script.py').read_text(encoding='utf-8')
            exec(compile(source,'native_record_bridge_script.py','exec'),scope)
            instance=Shadow();instance.id=uuid;instance.winner_eid_list=[owner];instance.extra_info={'asyn_pvp_eid':enemy}
            records=[(0,'set_last_fighting_cards',[{owner:[]}],{}),(1,'prepare',[[owner,enemy],1,42,1],{}),(2,'add_fighting_cards',[[]],{}),(3,'battle_end_notice',[[owner],0],{})]
            instance.battle_record=types.SimpleNamespace(records=records)
            assert instance.notify_battle_finish()=='原结果'
            directory=Path(temporary)/'hs_native_records';data=(directory/(uuid+'.record')).read_bytes()
            assert pickle.loads(zlib.decompress(data))==records
            meta_path=directory/(uuid+'.meta');meta=json.loads(meta_path.read_text());assert meta['owner']==owner and meta['count']==len(records) and meta['sha256']==hashlib.sha256(data).hexdigest()
            loop=next(fn for ms,fn in scheduled if ms==1500);loop();assert len(calls)==1 and calls[-1][0]=='upload_native_battle_record'
            first=calls[-1];timeout=next(fn for ms,fn in scheduled if ms==10000);timeout();first[2](True,'')
            assert json.loads(meta_path.read_text())['next']==0, '过期回调推进了新请求游标'
            loop();assert len(calls)==2 and calls[-1][1]==first[1]
            calls[-1][2](True,'');loop();assert meta_path.with_suffix('.meta.done').exists() and not meta_path.exists()
            before=len(scheduled);wrapped=Shadow.notify_battle_finish;exec(compile(source,'再次安装','exec'),scope);assert Shadow.notify_battle_finish is wrapped and len(scheduled)==before
            instance.notify_battle_finish();assert (directory/(uuid+'.record')).read_bytes()==data
            assert instance.notified==2
            print('PASS 原生完整四元组文件、SHA、所有者、回调超时续传、原结果与幂等安装；非Android验收')
    finally:
        for name,value in old_modules.items():
            if value is None:sys.modules.pop(name,None)
            else:sys.modules[name]=value
if __name__=='__main__':main()
