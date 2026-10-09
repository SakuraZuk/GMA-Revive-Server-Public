"""只读核对项目证书与项目续期任务，不读取私钥。"""
import json,sys
from pathlib import Path
from server_ops import connect,run
sys.stdout.reconfigure(encoding='utf-8');sys.stderr.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
c=connect()
try:
 result={'证书':run(c,'openssl x509 -in /opt/hs-server/tls/cert.pem -noout -subject -issuer -dates -ext subjectAltName -fingerprint -sha256'),'30天内到期检查':run(c,'openssl x509 -in /opt/hs-server/tls/cert.pem -noout -checkend 2592000; true'),'项目443监听':run(c,'ss -lntp sport = :443'),'计时器':run(c,'systemctl list-timers --all --no-pager')}
 result['计时器']='\n'.join(line for line in result['计时器'].splitlines() if 'hs-' in line or 'lego' in line or 'certbot' in line)
 (root/'out/certificate-audit.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
 print(json.dumps(result,ensure_ascii=False,indent=2))
finally:c.close()
