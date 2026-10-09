# -*- coding: utf-8 -*-
"""只读取续期所需公开上下文，不输出账户密钥或私钥。"""
from pathlib import Path
import json, sys
from server_ops import connect, run
sys.stdout.reconfigure(encoding='utf-8')
c=connect()
try:
    report={
        '证书':run(c,'openssl x509 -in /opt/hs-server/tls/cert.pem -noout -ext subjectAltName -dates'),
        '服务配置':run(c,'systemctl show hs-sdk -p FragmentPath -p User -p ExecStart -p EnvironmentFiles'),
        '续期帮助':run(c,'/opt/hs-server/tls/lego renew --help'),
        '账户邮箱':run(c,"python3 - <<'PY'\nimport json\nfrom pathlib import Path\nfor p in Path('/opt/hs-server/tls/lego-data/accounts').rglob('account.json'):\n d=json.loads(p.read_text());print(json.dumps({'email':d.get('email'),'contact':d.get('registration',{}).get('body',{}).get('contact',[])}))\nPY"),
        '443归属':run(c,'ss -lntp sport = :443'),
    }
    Path('out/certificate-renewal-context.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(report,ensure_ascii=False,indent=2))
finally:c.close()
