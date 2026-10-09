# -*- coding: utf-8 -*-
"""原子发布游戏服 data/hotfix.json，范围仅限 /opt/hs-server。"""
import hashlib
import json
import time
from pathlib import Path

import paramiko

from deploy_game_update import HOST, PORT, USER, PW, REMOTE

ROOT = Path(__file__).resolve().parents[2]
LOCAL = ROOT / "deploy/data/hotfix.json"
REMOTE_FILE = REMOTE + "/data/hotfix.json"


def run(client, command):
    _, out, err = client.exec_command(command, timeout=120)
    code = out.channel.recv_exit_status()
    text = out.read().decode("utf-8", "replace") + err.read().decode("utf-8", "replace")
    if code != 0:
        raise RuntimeError("远端命令失败(%d): %s\n%s" % (code, command, text.strip()))
    return text.strip()


def main():
    raw = LOCAL.read_bytes()
    json.loads(raw.decode("utf-8"))
    expected = hashlib.sha256(raw).hexdigest()
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(HOST, port=PORT, username=USER, password=PW, timeout=20,
                   allow_agent=False, look_for_keys=False)
    try:
        project = run(client, "readlink -f %s" % REMOTE)
        if project != REMOTE:
            raise RuntimeError("远端项目目录不匹配：" + project)
        if "ActiveState=active" not in run(client, "systemctl show hs-game -p ActiveState"):
            raise RuntimeError("hs-game 当前不是 active")
        listener = run(client, "ss -lntp")
        if ":9000" not in listener or "gameserver" not in listener:
            raise RuntimeError("9000 不是本项目 gameserver 监听")
        remote_dir = REMOTE + "/data"
        stamp = time.strftime("%Y%m%d-%H%M%S")
        backup = "%s/hotfix-%s.json" % (remote_dir, stamp)
        run(client, "test -f %s && cp -a %s %s || true" % (REMOTE_FILE, REMOTE_FILE, backup))
        temp = REMOTE_FILE + ".new"
        run(client, "rm -f %s" % temp)
        sftp = client.open_sftp()
        try:
            sftp.put(str(LOCAL), temp)
        finally:
            sftp.close()
        remote_hash = run(client, "sha256sum %s" % temp).split()[0]
        if remote_hash != expected:
            raise RuntimeError("临时热修清单哈希不一致：%s != %s" % (remote_hash, expected))
        run(client, "mv %s %s && systemctl restart hs-game && systemctl is-active --quiet hs-game" % (temp, REMOTE_FILE))
        final_hash = run(client, "sha256sum %s" % REMOTE_FILE).split()[0]
        if final_hash != expected:
            raise RuntimeError("原子替换后哈希不一致：%s != %s" % (final_hash, expected))
        print(json.dumps({"状态": "热修清单发布通过", "SHA256": expected, "备份": backup}, ensure_ascii=False))
    finally:
        client.close()


if __name__ == "__main__":
    main()
