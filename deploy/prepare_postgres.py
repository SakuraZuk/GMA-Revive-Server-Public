# -*- coding: utf-8 -*-
"""在游戏服项目目录解包 PostgreSQL，不安装系统服务或改动其他项目。"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "out/tools"))
import paramiko
from deploy_game_update import HOST, PORT, USER, PW

sys.stdout.reconfigure(encoding="utf-8")
client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(HOST, port=PORT, username=USER, password=PW, timeout=20,
               allow_agent=False, look_for_keys=False)
commands = [
    "mkdir -p /opt/hs-server/pgsql/packages /opt/hs-server/pgsql/runtime",
    "cd /opt/hs-server/pgsql/packages && apt-get download postgresql-15 postgresql-client-15 libpq5 libicu72 libldap-2.5-0 libllvm14 libxml2 libxslt1.1 libedit2 libz3-4",
    "cd /opt/hs-server/pgsql/packages && for package in ./*.deb; do dpkg-deb -x \"$package\" /opt/hs-server/pgsql/runtime; done",
    "LD_LIBRARY_PATH=/opt/hs-server/pgsql/runtime/usr/lib/x86_64-linux-gnu ldd /opt/hs-server/pgsql/runtime/usr/lib/postgresql/15/bin/postgres | grep 'not found' || true",
    "LD_LIBRARY_PATH=/opt/hs-server/pgsql/runtime/usr/lib/x86_64-linux-gnu /opt/hs-server/pgsql/runtime/usr/lib/postgresql/15/bin/postgres --version",
]
for command in commands:
    _, stdout, stderr = client.exec_command(command, timeout=180)
    output = stdout.read().decode("utf-8") + stderr.read().decode("utf-8")
    code = stdout.channel.recv_exit_status()
    print(output)
    if code:
        raise SystemExit("准备数据库运行库失败，退出码：" + str(code))
client.close()
