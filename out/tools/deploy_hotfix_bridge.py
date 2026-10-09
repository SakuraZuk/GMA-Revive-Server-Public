# -*- coding: utf-8 -*-
"""审计并原子发布客户端桥热修到独立热更服。"""
import hashlib
import json
import time
from pathlib import Path

import paramiko

ROOT = Path(__file__).resolve().parents[2]
LOCAL = ROOT / "deploy/data/hotfix.json"
from local_ops_credentials import load_target
HOST, PORT, USER, PASSWORD = load_target('hotfix')
REMOTE = "/opt/hs-hotfix"


def run(client, command):
    _, out, err = client.exec_command(command, timeout=60)
    code = out.channel.recv_exit_status()
    text = out.read().decode("utf-8", "replace") + err.read().decode("utf-8", "replace")
    if code:
        raise RuntimeError("远端命令失败(%s): %s" % (code, text.strip()))
    return text.strip()


def connect():
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(HOST, port=PORT, username=USER, password=PASSWORD, timeout=30,
                   allow_agent=False, look_for_keys=False)
    return client


def audit(client):
    project = run(client, "readlink -f %s" % REMOTE)
    listeners = run(client, "ss -lntup")
    service = run(client, "systemctl show hs-hotfix -p Id -p ActiveState -p MainPID -p FragmentPath")
    if project != REMOTE:
        raise RuntimeError("热更项目目录不匹配: " + project)
    if "ActiveState=active" not in service:
        raise RuntimeError("hs-hotfix 当前不是 active")
    if ":8082" not in listeners and ":443" not in listeners:
        raise RuntimeError("未发现热更 HTTP/HTTPS 监听")
    return {"项目目录": project, "监听": listeners, "服务": service}


def main():
    raw = LOCAL.read_bytes()
    json.loads(raw.decode("utf-8"))
    expected = hashlib.sha256(raw).hexdigest()
    client = connect()
    try:
        before = audit(client)
        remote_file = REMOTE + "/data/hotfix.json"
        stamp = time.strftime("%Y%m%d-%H%M%S")
        backup = REMOTE + "/data/hotfix.json.bak-" + stamp
        run(client, "test -f %s && cp -a %s %s || true" % (remote_file, remote_file, backup))
        temp = remote_file + ".new"
        run(client, "rm -f %s" % temp)
        sftp = client.open_sftp()
        try:
            sftp.put(str(LOCAL), temp)
        finally:
            sftp.close()
        remote_hash = run(client, "sha256sum %s" % temp).split()[0]
        if remote_hash != expected:
            raise RuntimeError("临时清单哈希不一致: %s != %s" % (remote_hash, expected))
        run(client, "mv %s %s && systemctl restart hs-hotfix && systemctl is-active --quiet hs-hotfix" % (temp, remote_file))
        final_hash = run(client, "sha256sum %s" % remote_file).split()[0]
        if final_hash != expected:
            raise RuntimeError("原子替换后哈希不一致: %s != %s" % (final_hash, expected))
        after = audit(client)
        report = {"状态": "热更桥清单发布通过", "服务器": HOST, "SHA256": expected,
                  "备份": backup, "发布前": before, "发布后": after}
        (ROOT / "out/hotfix-bridge-release.json").write_text(
            json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(report, ensure_ascii=False, indent=2))
    finally:
        client.close()


if __name__ == "__main__":
    main()
