# -*- coding: utf-8 -*-
"""只读核对远端二进制哈希、服务与数据库监听，不读取或打印数据库口令。"""
import hashlib
import json
from pathlib import Path
import sys
import paramiko
from deploy_game_update import HOST,PORT,USER,PW

ROOT=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')
client=paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(HOST,port=PORT,username=USER,password=PW,timeout=20,allow_agent=False,look_for_keys=False)
def read(command):
    _,out,err=client.exec_command(command,timeout=15)
    raw=out.read().decode('utf-8')
    error=err.read().decode('utf-8')
    if out.channel.recv_exit_status(): raise RuntimeError('远端只读检查失败：'+error)
    return raw.strip()
try:
    local=hashlib.sha256((ROOT/'out/bin/linux-amd64/gameserver').read_bytes()).hexdigest()
    remote=read('sha256sum /opt/hs-server/bin/gameserver').split()[0]
    assert local==remote,'部署产物与本机最终构建不一致'
    services=read('systemctl is-active hs-game hs-postgres hs-sdk hs-login').splitlines()
    assert services==['active']*4
    listeners=read("ss -lntp | grep -E ':9000 |:15432 '").splitlines()
    assert any('127.0.0.1:15432' in item for item in listeners)
    assert any(':9000' in item for item in listeners)
    report={'状态':'通过','服务器':HOST,'游戏二进制SHA256':remote,'服务状态':dict(zip(['hs-game','hs-postgres','hs-sdk','hs-login'],services)),
            '监听证据':listeners,'验收边界':'二进制、服务与监听；非 Android 主城验收'}
    (ROOT/'out/remote-release-verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(report,ensure_ascii=False,indent=2))
finally:
    client.close()
