# -*- coding: utf-8 -*-
"""部署项目所属证书续期脚本和计时器，首次检查未到期时不停止SDK。"""
from pathlib import Path
import hashlib, json, sys, shlex
from server_ops import connect, run, audit
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
c=connect()
try:
    before=audit(c)
    assert before['项目目录']=='/opt/hs-server'
    assert 'Id=hs-sdk.service\nActiveState=active' in before['服务']
    assert 'sdkserver' in run(c,'ss -lntp sport = :443')
    backup=run(c,'mktemp -d /opt/hs-server/tls/renew-install-XXXXXXXX')
    files={'renew-hs-certificate.sh':'/opt/hs-server/tls/renew-hs-certificate.sh',
           'hs-certificate-renew.service':'/etc/systemd/system/hs-certificate-renew.service',
           'hs-certificate-renew.timer':'/etc/systemd/system/hs-certificate-renew.timer'}
    hashes={}
    with c.open_sftp() as ftp:
        for name,target in files.items():
            run(c,'if [ -f '+shlex.quote(target)+' ]; then cp -a '+shlex.quote(target)+' '+shlex.quote(backup+'/'+name)+"; fi")
            content=(root/'deploy'/name).read_bytes()
            with ftp.open(target+'.new','wb') as f:f.write(content)
            hashes[name]=hashlib.sha256(content).hexdigest()
            assert run(c,'sha256sum '+shlex.quote(target+'.new')).split()[0]==hashes[name]
    run(c,'bash -n /opt/hs-server/tls/renew-hs-certificate.sh.new')
    for name,target in files.items():
        run(c,'chmod '+('700' if name.endswith('.sh') else '644')+' '+shlex.quote(target+'.new')+' && mv '+shlex.quote(target+'.new')+' '+shlex.quote(target))
    run(c,'systemctl daemon-reload && systemd-analyze verify /etc/systemd/system/hs-certificate-renew.service /etc/systemd/system/hs-certificate-renew.timer')
    check=run(c,'/opt/hs-server/tls/renew-hs-certificate.sh check')
    run(c,'systemctl enable --now hs-certificate-renew.timer && systemctl start hs-certificate-renew.service')
    status=run(c,'systemctl show hs-certificate-renew.timer hs-certificate-renew.service -p Id -p ActiveState -p Result -p LastTriggerUSec -p NextElapseUSecRealtime')
    after=audit(c)
    assert before['游戏哈希']==after['游戏哈希']
    result={'状态':'续期任务已安装，首次到期检查通过','备份':backup,'SHA256':hashes,'首次检查':check,'计时器':status,'SDK进程未改变':before['服务'].split('Id=hs-sdk.service')[1].split('Id=hs-login.service')[0]==after['服务'].split('Id=hs-sdk.service')[1].split('Id=hs-login.service')[0],'验收边界':'现证书有效期超过30天，未触发ACME真实续期分支；未来失败写systemd日志并恢复SDK/旧证书'}
    (root/'out/certificate-renewal-install.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(result,ensure_ascii=False,indent=2))
finally:c.close()
