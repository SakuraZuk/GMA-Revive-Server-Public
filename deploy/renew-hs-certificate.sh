#!/bin/bash
# 项目专属续期：只有30天内到期才短暂停止所属SDK；任何失败都恢复旧证书和SDK。
set -Eeuo pipefail
umask 077
tls=/opt/hs-server/tls
mode=${1:-renew}
case "$mode" in check|renew|verify-renewal) ;; *) echo '参数只允许check、renew或verify-renewal'; exit 64;; esac
exec 9>"$tls/renew.lock"
flock -n 9 || { echo '证书续期已有任务运行'; exit 0; }
[ -f "$tls/cert.pem" ] && [ -f "$tls/key.pem" ] && [ -x "$tls/lego" ]
openssl x509 -in "$tls/cert.pem" -noout -dates
if [ "$mode" != verify-renewal ] && openssl x509 -in "$tls/cert.pem" -noout -checkend 2592000; then
  echo '证书超过30天有效，无需释放443或续期'
  exit 0
fi
if [ "$mode" = check ]; then echo '证书需要续期'; exit 2; fi
renew_days=30
# 仅手工验收真实签发流程时允许90天窗口；日常计时器始终使用30天。
[ "$mode" != verify-renewal ] || renew_days=90
[ "$(readlink -f "$tls")" = "$tls" ]
systemctl is-active --quiet hs-sdk
pid=$(systemctl show hs-sdk -p MainPID --value)
[ "$(readlink -f "/proc/$pid/exe")" = /opt/hs-server/bin/sdkserver ]
listener=$(ss -H -lntp 'sport = :443')
[[ "$listener" = *"pid=$pid,"* && "$listener" = *sdkserver* ]]
stamp=$(date -u +%Y%m%d-%H%M%S)
backup="$tls/backup-renew-$stamp"
mkdir "$backup"
cp -p "$tls/cert.pem" "$backup/cert.pem"
cp -p "$tls/key.pem" "$backup/key.pem"
stopped=0
installed=0
finished=0
cleanup() {
  rc=$?
  trap - EXIT
  if [ "$finished" = 0 ] && [ "$installed" = 1 ]; then
    cp -p "$backup/cert.pem" "$tls/cert.pem.rollback"
    cp -p "$backup/key.pem" "$tls/key.pem.rollback"
    mv "$tls/cert.pem.rollback" "$tls/cert.pem"
    mv "$tls/key.pem.rollback" "$tls/key.pem"
  fi
  if [ "$stopped" = 1 ]; then
    if [ "$installed" = 1 ] && [ "$finished" = 0 ]; then
      systemctl restart hs-sdk || rc=1
    else
      systemctl start hs-sdk || rc=1
    fi
    systemctl is-active --quiet hs-sdk || rc=1
  fi
  if [ "$rc" != 0 ]; then
    echo "证书续期失败，旧证书备份=$backup；SDK已尝试恢复" >&2
    logger -p daemon.err -t hs-certificate "证书续期失败，备份=$backup"
  fi
  exit "$rc"
}
trap cleanup EXIT
mapfile -t domains < <(openssl x509 -in "$tls/cert.pem" -noout -ext subjectAltName | tr ',' '\n' | sed -n 's/.*DNS:\([a-z0-9.-]*\).*/\1/p' | sort -u)
[ "${#domains[@]}" = 24 ]
args=()
for domain in "${domains[@]}"; do
  [[ "$domain" =~ ^[a-z0-9.-]+\.r18sex\.net$ ]]
  args+=(--domains "$domain")
done
email=$(python3 - "$tls/lego-data" <<'PY'
import json, sys
from pathlib import Path
paths=list((Path(sys.argv[1])/'accounts').rglob('account.json'))
assert len(paths)==1
email=json.loads(paths[0].read_text())['email']
assert '\n' not in email and '@' in email
print(email)
PY
)
# 保持原lego主域名，证书文件路径由该首域决定。
args=(--domains service0000002.r18sex.net)
for domain in "${domains[@]}"; do
  [ "$domain" = service0000002.r18sex.net ] || args+=(--domains "$domain")
done
stopped=1
systemctl stop hs-sdk
[ -z "$(ss -H -lntp 'sport = :443')" ]
timeout 300 "$tls/lego" --accept-tos --email "$email" --path "$tls/lego-data" --tls "${args[@]}" renew --days "$renew_days" --ari-disable --no-random-sleep
new_cert="$tls/lego-data/certificates/service0000002.r18sex.net.crt"
new_key="$tls/lego-data/certificates/service0000002.r18sex.net.key"
openssl x509 -in "$new_cert" -noout -checkend 2592000
old_sans=$(printf '%s\n' "${domains[@]}")
new_sans=$(openssl x509 -in "$new_cert" -noout -ext subjectAltName | tr ',' '\n' | sed -n 's/.*DNS:\([a-z0-9.-]*\).*/\1/p' | sort -u)
[ "$old_sans" = "$new_sans" ]
cert_pub=$(openssl x509 -in "$new_cert" -pubkey -noout | openssl pkey -pubin -outform DER | sha256sum)
key_pub=$(openssl pkey -in "$new_key" -pubout -outform DER | sha256sum)
[ "$cert_pub" = "$key_pub" ]
install -m 644 "$new_cert" "$tls/cert.pem.new"
install -m 600 "$new_key" "$tls/key.pem.new"
installed=1
mv "$tls/cert.pem.new" "$tls/cert.pem"
mv "$tls/key.pem.new" "$tls/key.pem"
systemctl start hs-sdk
systemctl is-active --quiet hs-sdk
[ "$(readlink -f "/proc/$(systemctl show hs-sdk -p MainPID --value)/exe")" = /opt/hs-server/bin/sdkserver ]
ss -H -lntp 'sport = :443' | grep -q sdkserver
openssl s_client -connect 127.0.0.1:443 -servername service0000002.r18sex.net -verify_return_error </dev/null >/dev/null 2>&1
finished=1
stopped=0
echo "证书续期和TLS验证通过，回滚备份=$backup"
