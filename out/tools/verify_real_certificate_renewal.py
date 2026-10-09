"""执行一次项目专属真实续期验收，保存证书/SDK恢复与共享服务证据。"""
import hashlib,json,sys,time
from pathlib import Path
from server_ops import connect,run,audit
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')
ROOT=Path(__file__).resolve().parents[2]
c=connect()
result={}
try:
    before=audit(c)
    cert_before=run(c,'openssl x509 -in /opt/hs-server/tls/cert.pem -noout -dates -serial -ext subjectAltName')
    assert 'Id=hs-sdk.service\nActiveState=active' in before['服务']
    assert run(c,'systemctl is-active hs-certificate-renew.timer')=='active'
    script=ROOT/'deploy/renew-hs-certificate.sh'
    assert run(c,'sha256sum /opt/hs-server/tls/renew-hs-certificate.sh').split()[0]==hashlib.sha256(script.read_bytes()).hexdigest()
    run(c,'systemd-run --unit=hs-certificate-verify --description=hs-real-certificate-renewal /opt/hs-server/tls/renew-hs-certificate.sh verify-renewal')
    print('24域名真实续期验收已启动，任务hs-certificate-verify；任何失败自动恢复旧证书与SDK',flush=True)
    for attempt in range(90):
        state=run(c,'systemctl show hs-certificate-verify -p ActiveState -p Result -p ExecMainStatus')
        if 'ActiveState=active' not in state and 'ActiveState=activating' not in state:break
        time.sleep(4)
    logs=run(c,'journalctl -u hs-certificate-verify --no-pager -n 200')
    after=audit(c)
    cert_after=run(c,'openssl x509 -in /opt/hs-server/tls/cert.pem -noout -dates -serial -ext subjectAltName')
    result={'状态':state,'续期日志':logs,'发布前':before,'发布后':after,'旧证书':cert_before,'新证书':cert_after,'验收时间':time.strftime('%Y-%m-%d %H:%M:%S')}
    (ROOT/'out/certificate-real-renewal.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    (ROOT/'out/certificate-real-renewal.log').write_text(logs+'\n',encoding='utf-8')
    assert 'Result=success' in state and 'ExecMainStatus=0' in state
    assert '证书续期和TLS验证通过' in logs
    assert cert_after!=cert_before,'未得到新签发证书'
    assert before['游戏哈希']==after['游戏哈希']
    assert 'Id=hs-sdk.service\nActiveState=active' in after['服务']
    print('真实续期、24域名SAN和SDK恢复通过；证据out/certificate-real-renewal.json')
finally:c.close()
