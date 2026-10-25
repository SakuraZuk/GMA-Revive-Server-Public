# -*- coding: utf-8 -*-
"""发布带公网运营后台的 gameserver，并只给 hs-game 增加 18080。"""
import hashlib
import json
import secrets
import sys
from datetime import datetime
from pathlib import Path
from server_ops import HOST, ROOT, connect, reap_holders, run, upload_resumable

PORT = 18080
DROPIN = "/etc/systemd/system/hs-game.service.d/70-public-admin.conf"


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    local = ROOT / "out/bin/linux-amd64/gameserver"
    expected = hashlib.sha256(local.read_bytes()).hexdigest()
    target = "/opt/hs-server/bin/gameserver"
    token = secrets.token_urlsafe(36)
    client = connect()
    try:
        listeners = run(client, "ss -lnt")
        if ":%d " % PORT in listeners:
            raise SystemExit("18080 已被占用，停止发布")
        if "gameserver" not in run(client, "ss -lntp | awk '/:9000 /{print}'"):
            raise SystemExit("9000 不属于 gameserver，停止发布")
        stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
        backup = "/opt/hs-server/bin/backup-" + stamp
        run(client, "mkdir -p %s && cp -a %s %s/gameserver" % (backup, target, backup))
        run(client, "mkdir -p /etc/systemd/system/hs-game.service.d")
        if DROPIN in run(client, "ls /etc/systemd/system/hs-game.service.d || true"):
            run(client, "cp -a %s %s/" % (DROPIN, backup))
    finally:
        client.close()
    reap_holders(connect(), target + ".new")
    uploaded = upload_resumable(connect, local, target + ".new")
    client = connect()
    try:
        remote_hash = run(client, "sha256sum %s.new" % target).split()[0]
        if remote_hash != expected:
            raise SystemExit("上传哈希不一致")
        unit = (
            "[Service]\n"
            "Environment=HS_GAME_ADMIN_BIND=0.0.0.0:%d\n"
            "Environment=HS_GAME_ADMIN_TOKEN=%s\n" % (PORT, token)
        )
        run(client, "cat > %s <<'EOF'\n%sEOF" % (DROPIN, unit))
        run(client, "chmod 600 %s && chmod 755 %s.new && mv %s.new %s && systemctl daemon-reload && systemctl restart hs-game" % (DROPIN, target, target, target))
        run(client, "systemctl is-active --quiet hs-game")
        listening = run(client, "ss -lnt | awk '/%d /{print}'" % PORT)
        page = run(client, "curl -fsS -o /dev/null -w '%%{http_code}' http://127.0.0.1:%d/admin" % PORT)
        if page != "200" or ":%d" % PORT not in listening:
            raise SystemExit("管理端口未就绪 code=%s listen=%s" % (page, listening))
        if run(client, "sha256sum %s" % target).split()[0] != expected:
            raise SystemExit("安装后哈希不一致")
    except Exception:
        reap_holders(client, target)
        run(client, "cp -a %s/gameserver %s.rollback && mv %s.rollback %s && rm -f %s && systemctl daemon-reload && systemctl restart hs-game" % (backup, target, target, target, DROPIN))
        raise
    finally:
        client.close()
    report = {"状态": "公网运营后台已发布", "地址": "http://%s:%d/admin" % (HOST, PORT),
              "账号": "admin", "备份": backup, "SHA256": expected, "上传字节": uploaded}
    path = ROOT / "out/public-admin-release.json"
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    secret = ROOT / "out/public-admin-token.txt"
    secret.write_text(token + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False))


if __name__ == "__main__":
    main()
