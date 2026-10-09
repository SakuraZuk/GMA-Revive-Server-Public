# -*- coding: utf-8 -*-
"""通过 Paramiko 部署指定服务；密码只从环境变量读取，不写入文件。"""
from pathlib import Path
import os
import re
import sys
import uuid

import paramiko

ROOT = Path(__file__).resolve().parents[1]
TARGET = os.environ.get("HS_DEPLOY_TARGET", "").strip().lower()
PASSWORD = os.environ.get("HS_DEPLOY_PASSWORD", "")
if TARGET not in ("game", "hotfix"):
    raise SystemExit("请设置 HS_DEPLOY_TARGET=game 或 hotfix")
if not PASSWORD:
    raise SystemExit("请设置 HS_DEPLOY_PASSWORD；脚本不会从文件读取密码")

HOST = os.environ.get("HS_DEPLOY_HOST", "").strip()
if not HOST:
    raise SystemExit("请设置 HS_DEPLOY_HOST，不使用任何发布者服务器地址")
PORT = int(os.environ.get("HS_DEPLOY_PORT", "22"))
REMOTE = "/opt/hs-server"
STAGE = f"{REMOTE}/.stage-{uuid.uuid4().hex}"

GAME_UNIT = """[Unit]
Description=幻书复活游戏 TCP 服务
After=network-online.target
Wants=network-online.target
[Service]
WorkingDirectory=/opt/hs-server
Environment=HS_GAME_BIND=0.0.0.0:9000
Environment=HS_GAME_ADDRESS=192.0.2.10:9000
Environment=HS_DATA_DIR=/opt/hs-server/data
Environment=HS_MAX_SESSIONS=500
Environment=HS_MAX_FRAME_PAYLOAD=1048576
ExecStart=/opt/hs-server/bin/gameserver
Restart=on-failure
RestartSec=2
LimitNOFILE=65535
[Install]
WantedBy=multi-user.target
"""

SDK_UNIT = """[Unit]
Description=幻书复活 SDK 管理服务
After=network-online.target
Wants=network-online.target
[Service]
WorkingDirectory=/opt/hs-server
Environment=HS_SDK_BIND=0.0.0.0:8080
Environment=HS_GAME_ADDRESS=192.0.2.10:9000
Environment=HS_HOTFIX_ADDRESS=192.0.2.20:443
Environment=HS_HOTFIX_VERSIONS=1.0.125
ExecStart=/opt/hs-server/bin/sdkserver
Restart=on-failure
RestartSec=2
LimitNOFILE=65535
[Install]
WantedBy=multi-user.target
"""

LOGIN_UNIT = """[Unit]
Description=幻书复活登录管理服务
After=network-online.target
Wants=network-online.target
[Service]
WorkingDirectory=/opt/hs-server
Environment=HS_LOGIN_BIND=0.0.0.0:8081
Environment=HS_GAME_ADDRESS=192.0.2.10:9000
Environment=HS_MAX_SESSIONS=500
ExecStart=/opt/hs-server/bin/loginserver
Restart=on-failure
RestartSec=2
LimitNOFILE=65535
[Install]
WantedBy=multi-user.target
"""

HOTFIX_UNIT = """[Unit]
Description=幻书复活独立热更服务
After=network-online.target
Wants=network-online.target
[Service]
WorkingDirectory=/opt/hs-server
Environment=HS_HOTFIX_BIND=0.0.0.0:80
Environment=HS_HOTFIX_TLS_BIND=0.0.0.0:443
Environment=HS_TLS_CERT=/opt/hs-server/tls/cert.pem
Environment=HS_TLS_KEY=/opt/hs-server/tls/key.pem
Environment=HS_DNS_BIND=0.0.0.0:53
Environment=HS_DNS_ANSWER=192.0.2.20
Environment=HS_GAME_DNS_ANSWER=192.0.2.10
Environment=HS_GAME_DOMAIN=r18sex.net
Environment=HS_GAME_ADDRESS=192.0.2.10:9000
Environment=HS_DATA_DIR=/opt/hs-server/data
Environment=HS_MAX_SESSIONS=500
ExecStart=/opt/hs-server/bin/hotfixserver
Restart=on-failure
RestartSec=2
LimitNOFILE=65535
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
[Install]
WantedBy=multi-user.target
"""

def mkdir(sftp, path):
    try:
        sftp.mkdir(path)
    except IOError:
        pass

def put_tree(sftp, local, remote):
    mkdir(sftp, remote)
    for item in local.iterdir():
        dst = remote + "/" + item.name
        if item.is_dir():
            put_tree(sftp, item, dst)
        else:
            sftp.put(str(item), dst)

def put_selected_binaries(sftp, local, remote, names):
    mkdir(sftp, remote)
    for name in names:
        sftp.put(str(local / name), remote + "/" + name)

def run(client, command, timeout=30):
    _, out, err = client.exec_command(command, timeout=timeout)
    stdout = out.read().decode("utf-8", "replace")
    stderr = err.read().decode("utf-8", "replace")
    return stdout, stderr

def port_conflicts(listeners, ports, allowed):
    conflicts = []
    for line in listeners.splitlines():
        if not any(re.search(rf"[:.]{{1}}{port}(?:\s|$)", line) for port in ports):
            continue
        if not any(name in line for name in allowed):
            conflicts.append(line.strip())
    return conflicts

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(HOST, port=PORT, username="root", password=PASSWORD,
               timeout=15, banner_timeout=15, auth_timeout=15,
               allow_agent=False, look_for_keys=False)
try:
    listeners, _ = run(client, "ss -lntup 2>/dev/null || netstat -lntup 2>/dev/null")
    print("=== 部署前监听检查 ===")
    print(listeners, end="")
    if TARGET == "game":
        conflicts = port_conflicts(listeners, (8080, 8081, 9000), ("sdkserver", "loginserver", "gameserver"))
        binaries = ("sdkserver", "loginserver", "gameserver")
        units = {"hs-sdk.service": SDK_UNIT, "hs-login.service": LOGIN_UNIT, "hs-game.service": GAME_UNIT}
        services = tuple(units)
    else:
        conflicts = port_conflicts(listeners, (80, 443, 53), ("hotfixserver",))
        binaries = ("hotfixserver",)
        units = {"hs-hotfix.service": HOTFIX_UNIT}
        services = tuple(units)
    if conflicts:
        raise SystemExit("目标端口已有非本项目进程，已中止：\n" + "\n".join(conflicts))

    sftp = client.open_sftp()
    mkdir(sftp, STAGE)
    mkdir(sftp, STAGE + "/bin")
    put_selected_binaries(sftp, ROOT / "out/bin/linux-amd64", STAGE + "/bin", binaries)
    put_tree(sftp, ROOT / "deploy/data", STAGE + "/data")
    for name in binaries:
        sftp.chmod(STAGE + "/bin/" + name, 0o755)
    if TARGET == "hotfix":
        cert = Path(os.environ.get("HS_TLS_CERT_SOURCE", str(ROOT / "work/mock/cert.pem")))
        key = Path(os.environ.get("HS_TLS_KEY_SOURCE", str(ROOT / "work/mock/key.pem")))
        if not cert.is_file() or not key.is_file():
            raise SystemExit("热更部署必须提供 HS_TLS_CERT_SOURCE 和 HS_TLS_KEY_SOURCE")
        mkdir(sftp, STAGE + "/tls")
        sftp.put(str(cert), STAGE + "/tls/cert.pem")
        sftp.put(str(key), STAGE + "/tls/key.pem")
    for name, body in units.items():
        with sftp.file(STAGE + "/" + name, "w") as handle:
            handle.write(body)
    sftp.close()

    _, stderr = run(client, f"""set -e
mkdir -p {REMOTE}/bin {REMOTE}/data {REMOTE}/tls
systemctl stop {' '.join(services)} 2>/dev/null || true
for f in {STAGE}/bin/*; do mv -f "$f" {REMOTE}/bin/; done
cp -a {STAGE}/data/. {REMOTE}/data/
""", timeout=45)
    if stderr:
        print("[stderr] " + stderr, end="")
    if TARGET == "hotfix":
        run(client, f"cp -a {STAGE}/tls/. {REMOTE}/tls/ && chmod 600 {REMOTE}/tls/*", timeout=20)
    for name in units:
        run(client, f"install -m 0644 {STAGE}/{name} /etc/systemd/system/{name}", timeout=20)
    run(client, f"rm -rf {STAGE} && systemctl daemon-reload && systemctl enable --now {' '.join(services)}", timeout=45)
    status, stderr = run(client, f"systemctl --no-pager --full status {' '.join(services)}", timeout=30)
    print(status, end="")
    if stderr:
        print("[stderr] " + stderr, end="")
finally:
    client.close()
