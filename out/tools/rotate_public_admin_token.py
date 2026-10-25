# -*- coding: utf-8 -*-
"""轮换公网管理令牌；不在标准输出、命令行或报告中暴露令牌。"""

import secrets
import sys
import time
from pathlib import Path

import paramiko

from local_ops_credentials import load_target


ROOT = Path(__file__).resolve().parents[2]
DROPIN = "/etc/systemd/system/hs-game.service.d/70-public-admin.conf"


def run(client, command):
    _, stdout, stderr = client.exec_command(command, timeout=30)
    output = stdout.read().decode("utf-8")
    error = stderr.read().decode("utf-8")
    if stdout.channel.recv_exit_status():
        raise RuntimeError("管理令牌轮换命令失败：" + error)
    return output.strip()


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    host, port, user, password = load_target("game")
    token = secrets.token_urlsafe(48)
    body = (
        "[Service]\n"
        "Environment=HS_GAME_ADMIN_BIND=0.0.0.0:18080\n"
        "Environment=HS_GAME_ADMIN_TOKEN=" + token + "\n"
    ).encode("utf-8")
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(host, port=port, username=user, password=password, timeout=20,
                   allow_agent=False, look_for_keys=False)
    try:
        stamp = run(client, "date +%Y%m%d-%H%M%S")
        backup = "/opt/hs-server/releases/admin-token-" + stamp
        run(client, "mkdir -m 700 " + backup + " && cp -a " + DROPIN + " " + backup + "/70-public-admin.conf")
        temporary = DROPIN + ".new"
        with client.open_sftp() as sftp:
            with sftp.open(temporary, "wb") as target:
                target.write(body)
            sftp.chmod(temporary, 0o600)
        run(client, "mv -f " + temporary + " " + DROPIN + " && systemctl daemon-reload && systemctl restart hs-game && systemctl is-active --quiet hs-game")
        page = ""
        for _ in range(20):
            try:
                page = run(client, "curl -fsS -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/admin")
            except RuntimeError:
                page = ""
            if page == "200":
                break
            time.sleep(0.5)
        if page != "200":
            raise RuntimeError("管理页面轮换后未就绪")
        private = ROOT / "out" / "public-admin-token.txt"
        private.write_text(token + "\n", encoding="utf-8")
        print("管理令牌已轮换；新值仅保存到本机私有令牌文件，远端旧配置已备份。")
    finally:
        client.close()


if __name__ == "__main__":
    main()
