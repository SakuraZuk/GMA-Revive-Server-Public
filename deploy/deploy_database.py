# -*- coding: utf-8 -*-
"""幻书独立 PostgreSQL 初始化、真实数据库验收与游戏服切换。

Windows CMD 调用，阶段为 prepare、test、release。密码仅写入远端权限 0600
的配置文件，不输出或写入本机文档。运行库由 prepare_postgres.py 解包。
"""
import json
from pathlib import Path
import secrets
import shlex
import sys
import time

import paramiko

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "out/tools"))
from deploy_game_update import HOST, PORT, USER, PW

REMOTE = "/opt/hs-server"
PGROOT = REMOTE + "/pgsql"
BIN = PGROOT + "/runtime/usr/lib/postgresql/15/bin"
LIB = PGROOT + "/runtime/usr/lib/x86_64-linux-gnu"
ENVFILE = REMOTE + "/data/database.env"
sys.stdout.reconfigure(encoding="utf-8")
sys.stderr.reconfigure(encoding="utf-8")


def run(client, command, *, secret=False, timeout=60):
    _, stdout, stderr = client.exec_command(command, timeout=timeout)
    output = stdout.read().decode("utf-8") + stderr.read().decode("utf-8")
    code = stdout.channel.recv_exit_status()
    if not secret:
        print(output, flush=True)
    if code:
        raise RuntimeError("远端操作失败，退出码：" + str(code))
    return output


def put_text(client, path, content, mode=0o600):
    with client.open_sftp() as sftp:
        with sftp.file(path, "w") as file:
            file.write(content.encode("utf-8"))
        sftp.chmod(path, mode)


def prepare(client):
    run(client, "if ss -lnt | grep -q ':15432 '; then test -f " + PGROOT + "/data/PG_VERSION; fi")
    run(client, "if id hspg >/dev/null 2>&1; then test \"$(getent passwd hspg | cut -d: -f6)\" = " + PGROOT + "; else useradd --system --home-dir " + PGROOT + " --shell /usr/sbin/nologin hspg; fi")
    run(client, "mkdir -p " + PGROOT + "/data " + PGROOT + "/socket; chown hspg:hspg " + PGROOT + "/data " + PGROOT + "/socket; chmod 700 " + PGROOT + "/data " + PGROOT + "/socket")
    run(client, "if ! test -f " + PGROOT + "/data/PG_VERSION; then runuser -u hspg -- env LD_LIBRARY_PATH=" + LIB + " " + BIN + "/initdb -D " + PGROOT + "/data -L " + PGROOT + "/runtime/usr/share/postgresql/15 --encoding=UTF8 --locale=C.UTF-8 --auth-local=peer --auth-host=scram-sha-256 --data-checksums; fi")
    put_text(client, PGROOT + "/data/hs.conf", "# 幻书专用数据库，仅监听本机\nlisten_addresses='127.0.0.1'\nport=15432\nunix_socket_directories='" + PGROOT + "/socket'\nmax_connections=48\nshared_buffers='64MB'\nstatement_timeout='5s'\nidle_in_transaction_session_timeout='5s'\npassword_encryption='scram-sha-256'\ntimezone='Asia/Shanghai'\n")
    run(client, "chown hspg:hspg " + PGROOT + "/data/hs.conf; grep -q \"include = 'hs.conf'\" " + PGROOT + "/data/postgresql.conf || printf \"\\ninclude = 'hs.conf'\\n\" >> " + PGROOT + "/data/postgresql.conf")
    unit = "[Unit]\nDescription=幻书独立 PostgreSQL 数据库\nAfter=network.target\n[Service]\nType=simple\nUser=hspg\nGroup=hspg\nEnvironment=LD_LIBRARY_PATH=" + LIB + "\nExecStart=" + BIN + "/postgres -D " + PGROOT + "/data\nRestart=on-failure\nRestartSec=3\nKillSignal=SIGINT\nTimeoutStopSec=60\n[Install]\nWantedBy=multi-user.target\n"
    put_text(client, "/etc/systemd/system/hs-postgres.service", unit, 0o644)
    run(client, "systemctl daemon-reload; systemctl enable --now hs-postgres; systemctl is-active hs-postgres")
    with client.open_sftp() as sftp:
        try:
            sftp.stat(ENVFILE)
            exists = True
        except FileNotFoundError:
            exists = False
    if not exists:
        password = secrets.token_hex(24)
        sql = "CREATE ROLE hsgame LOGIN PASSWORD '" + password + "';\nCREATE DATABASE hs OWNER hsgame ENCODING 'UTF8';\n"
        sqlfile = PGROOT + "/create-role.sql"
        put_text(client, sqlfile, sql)
        try:
            run(client, "chown hspg:hspg " + sqlfile, secret=True)
            run(client, "runuser -u hspg -- env LD_LIBRARY_PATH=" + LIB + " " + BIN + "/psql -h " + PGROOT + "/socket -p 15432 -d postgres -v ON_ERROR_STOP=1 -f " + sqlfile, secret=True)
            put_text(client, ENVFILE, "HS_DATABASE_URL=postgres://hsgame:" + password + "@127.0.0.1:15432/hs?sslmode=disable\nHS_DATABASE_MAX_CONNS=16\n")
        finally:
            run(client, "rm -f " + sqlfile, secret=True)
    run(client, "ss -lntp | grep ':15432 '; runuser -u hspg -- env LD_LIBRARY_PATH=" + LIB + " " + BIN + "/psql -h " + PGROOT + "/socket -p 15432 -d postgres -Atc \"SELECT datname FROM pg_database WHERE datname='hs'\"")


def test(client):
    with client.open_sftp() as sftp:
        sftp.put(str(ROOT / "out/bin/linux-amd64/dbstore.test"), REMOTE + "/bin/dbstore.test")
        sftp.chmod(REMOTE + "/bin/dbstore.test", 0o755)
    output = run(client, "set -a; . " + ENVFILE + "; set +a; HS_TEST_DATABASE_URL=\"$HS_DATABASE_URL\" " + REMOTE + "/bin/dbstore.test -test.v -test.timeout=45s", timeout=55)
    (ROOT / "out/database-test.log").write_text(output, encoding="utf-8")


def release(client):
    stamp = time.strftime("%Y%m%d-%H%M%S")
    backup = REMOTE + "/bin/backup-db-" + stamp
    run(client, "mkdir -p " + backup + "; cp -a " + REMOTE + "/bin/gameserver " + backup + "/gameserver; mkdir -p /etc/systemd/system/hs-game.service.d")
    dropin = "/etc/systemd/system/hs-game.service.d/database.conf"
    with client.open_sftp() as sftp:
        try:
            sftp.stat(dropin)
            existing_dropin = True
        except FileNotFoundError:
            existing_dropin = False
    if existing_dropin:
        run(client, "cp -a " + dropin + " " + backup + "/database.conf")
    with client.open_sftp() as sftp:
        sftp.put(str(ROOT / "out/bin/linux-amd64/gameserver"), REMOTE + "/bin/gameserver.db-new")
        sftp.chmod(REMOTE + "/bin/gameserver.db-new", 0o755)
    put_text(client, dropin, "[Unit]\nAfter=hs-postgres.service\nRequires=hs-postgres.service\n[Service]\nEnvironmentFile=" + ENVFILE + "\n", 0o644)
    run(client, "mv " + REMOTE + "/bin/gameserver.db-new " + REMOTE + "/bin/gameserver; systemctl daemon-reload; systemctl restart hs-game")
    try:
        run(client, "systemctl is-active hs-game hs-postgres; ss -lntp | grep ':9000 '; journalctl -u hs-game -n 4 --no-pager")
    except Exception:
        restore_dropin = "cp -a " + backup + "/database.conf " + dropin if existing_dropin else "mv " + dropin + " " + backup + "/database.conf.failed"
        run(client, "cp -a " + backup + "/gameserver " + REMOTE + "/bin/gameserver; " + restore_dropin + "; systemctl daemon-reload; systemctl restart hs-game")
        raise
    print("数据库业务版本已上线，回滚副本：" + backup)


if __name__ == "__main__":
    phase = sys.argv[1]
    if phase not in ("prepare", "test", "release"):
        raise SystemExit("阶段须为 prepare、test 或 release")
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(HOST, port=PORT, username=USER, password=PW, timeout=20,
                   allow_agent=False, look_for_keys=False)
    try:
        {"prepare": prepare, "test": test, "release": release}[phase](client)
    finally:
        client.close()
