# -*- coding: utf-8 -*-
"""在游戏服临时目录编译并测试运营后台，不替换、不重启线上服务。"""
import sys
import tarfile
import tempfile
from pathlib import Path
import paramiko
from local_ops_credentials import load_target

ROOT = Path(__file__).resolve().parents[2]
HOST, PORT, USER, PW = load_target("game")
REMOTE = "/tmp/hs-admin-ops-check"
SKIP = {".git", "out", "work", ".claude", "_ref", "node_modules"}


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(HOST, port=PORT, username=USER, password=PW, timeout=30,
                   allow_agent=False, look_for_keys=False)
    try:
        _, out, err = client.exec_command("command -v go || ls /usr/local/go/bin/go /usr/lib/go*/bin/go 2>/dev/null; go version", timeout=30)
        print(out.read().decode() + err.read().decode())
        blob = tempfile.NamedTemporaryFile(delete=False, suffix=".tar")
        blob.close()
        with tarfile.open(blob.name, "w") as tar:
            for folder in ("cmd", "internal", "deploy"):
                tar.add(ROOT / folder, folder, filter=lambda info: None if any(part in SKIP for part in Path(info.name).parts) else info)
            tar.add(ROOT / "go.mod", "go.mod")
            tar.add(ROOT / "go.sum", "go.sum")
        sftp = client.open_sftp()
        sftp.put(blob.name, "/tmp/hs-admin-ops.tar")
        sftp.close()
        command = (
            "set -e; rm -rf %s; mkdir -p %s; tar -xf /tmp/hs-admin-ops.tar -C %s; "
            "cd %s; export PATH=/usr/local/go/bin:$PATH; gofmt -w internal/game/admin.go internal/game/admin_ops.go internal/game/admin_ops_test.go internal/game/dbstore/admin.go; "
            "go test -count=1 -timeout 180s ./internal/game/ -run 'TestOpsAdmin|TestAdminStatic|TestAdminRune|TestPlayerAdmin'"
        ) % (REMOTE, REMOTE, REMOTE, REMOTE)
        _, out, err = client.exec_command(command, timeout=180)
        print(out.read().decode("utf-8", "replace"))
        print(err.read().decode("utf-8", "replace"))
        print("exit", out.channel.recv_exit_status())
    finally:
        client.close()


if __name__ == "__main__":
    main()
