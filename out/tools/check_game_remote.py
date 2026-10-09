# -*- coding: utf-8 -*-
"""只读检查幻书游戏服部署和数据库环境，不输出连接密码。"""
import sys
from pathlib import Path
import paramiko
from deploy_game_update import HOST, PORT, USER, PW

sys.stdout.reconfigure(encoding="utf-8")
client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(HOST, port=PORT, username=USER, password=PW, timeout=20,
               allow_agent=False, look_for_keys=False)
commands = (
    "cat /etc/os-release; command -v apt-get; command -v runuser; df -h /opt; apt-cache policy postgresql-15 postgresql-16; ps -eo comm | grep -E 'postgres|pg_ctl'",
    "systemctl is-active hs-game hs-sdk hs-login; ss -lntup | head -40",
    "command -v psql; command -v postgres; command -v docker; ls /usr/lib/postgresql 2>/dev/null; systemctl list-units --type=service --all | grep -E 'postgres|hs-'",
    "systemctl cat hs-game | sed -E 's#(HS_DATABASE_URL=).*#\\1[已隐藏]#'; ls -l /opt/hs-server/bin /opt/hs-server/data",
    "journalctl -u hs-game -n 12 --no-pager",
)
if len(sys.argv)>1:
    commands = [Path(sys.argv[1]).read_text(encoding="utf-8")]
for command in commands:
    _, stdout, stderr = client.exec_command(command, timeout=30)
    print(stdout.read().decode("utf-8") + stderr.read().decode("utf-8"))
client.close()
