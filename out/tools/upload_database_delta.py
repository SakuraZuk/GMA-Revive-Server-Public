# -*- coding: utf-8 -*-
"""复制已验证的私有验收产物后按块更新新目录；最终整文件SHA仍由验收器核查。"""
import hashlib, json, shlex
from server_ops import ROOT, connect, run
def upload_database_delta(local, target):
    previous=json.loads((ROOT/'out/remote-database-verification.json').read_text(encoding='utf-8'))
    base=previous.get('远端目录','')
    if not base.startswith('/opt/hs-server/data/verification/db-') or '..' in base or base==target.rsplit('/',1)[0]:return False
    if not target.startswith('/opt/hs-server/data/verification/db-') or not target.endswith('/dbstore.test') or '..' in target:return False
    source=base+'/dbstore.test'; block=256*1024
    client=connect()
    try:
        program='''import hashlib,json,pathlib
p=pathlib.Path(SOURCE)
if p.is_symlink() or not p.is_file():raise ValueError('旧验收产物必须为私有普通文件')
raw=p.read_bytes()
if hashlib.sha256(raw).hexdigest()!=EXPECTED:raise ValueError('旧验收产物SHA漂移')
print(json.dumps([hashlib.sha256(raw[i:i+BLOCK]).hexdigest() for i in range(0,len(raw),BLOCK)]))'''.replace('SOURCE',repr(source)).replace('EXPECTED',repr(previous['SHA256'])).replace('BLOCK',str(block))
        hashes=json.loads(run(client,'python3 -c '+shlex.quote(program)))
        raw=local.read_bytes()
        changed=[i for i in range((len(raw)+block-1)//block) if i>=len(hashes) or hashlib.sha256(raw[i*block:(i+1)*block]).hexdigest()!=hashes[i]]
        # 多数区块变化时使用原断线续传，避免一块一请求放大开销。
        if len(changed)*block>len(raw)//2:return False
        run(client,'test ! -e '+shlex.quote(target)+' && test ! -L '+shlex.quote(source)+' && cp '+shlex.quote(source)+' '+shlex.quote(target)+' && chmod 600 '+shlex.quote(target))
        with client.open_sftp() as sftp:
            with sftp.open(target,'r+b') as output:
                output.set_pipelined(True)
                for i in changed:
                    output.seek(i*block);output.write(raw[i*block:(i+1)*block])
                output.truncate(len(raw))
        expected=hashlib.sha256(raw).hexdigest()
        assert run(client,'sha256sum '+shlex.quote(target)).split()[0]==expected, '差分产物整SHA不一致'
        print(json.dumps({'上传':'私有新目录区块差分，最终SHA一致','变化区块':len(changed),'总区块':(len(raw)+block-1)//block,'SHA256':expected},ensure_ascii=False))
        return True
    finally:client.close()
