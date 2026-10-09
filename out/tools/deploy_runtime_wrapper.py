# -*- coding: utf-8 -*-
"""只包装当前生产 runtime 热修正文，不带入本地并行开发内容。"""
import hashlib
import io
import json
import sys
import time

import paramiko

import deploy_hotfix_bridge as bridge_deploy
import deploy_hotfix_catalog as game_deploy
from deploy_game_update import HOST, PORT, USER, PW, REMOTE
from merge_battle_bridge_hotfix import compose_runtime


EXPECTED_SHA256 = "f6269ff6ec23c8fd3fa753f767cac7b8e75d9e01d0e8ad8171aa83ba63cd431c"
EXPECTED_INDEX = 2026100804
INDEPENDENT_ROOT = "/opt/hs-hotfix"
INDEPENDENT_FILE = INDEPENDENT_ROOT + "/data/hotfix.json"
GAME_FILE = REMOTE + "/data/hotfix.json"


def connect_game():
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(HOST, port=PORT, username=USER, password=PW, timeout=20,
                   auth_timeout=20, banner_timeout=20,
                   allow_agent=False, look_for_keys=False)
    return client


def read_bytes(client, path):
    sftp = client.open_sftp()
    try:
        with sftp.open(path, "rb") as stream:
            return stream.read()
    finally:
        sftp.close()


def stage_bytes(client, path, raw):
    temp = path + ".runtime-wrapper.new"
    sftp = client.open_sftp()
    try:
        sftp.putfo(io.BytesIO(raw), temp, file_size=len(raw), confirm=True)
    finally:
        sftp.close()
    return temp


def validate_current(raw, label):
    digest = hashlib.sha256(raw).hexdigest()
    if digest != EXPECTED_SHA256:
        raise RuntimeError("%s生产热修已漂移：%s" % (label, digest))
    catalog = json.loads(raw.decode("utf-8"))
    runtime = catalog.get("runtime") or {}
    if runtime.get("index") != EXPECTED_INDEX:
        raise RuntimeError("%s runtime.index不是%s" % (label, EXPECTED_INDEX))
    script = runtime.get("script") or ""
    if script.startswith("def _hs_runtime_install():"):
        raise RuntimeError("%s runtime已经包装，拒绝重复发布" % label)
    return catalog


def main():
    independent = bridge_deploy.connect()
    game = connect_game()
    stamp = time.strftime("%Y%m%d-%H%M%S")
    independent_backup = INDEPENDENT_FILE + ".bak-runtime-" + stamp
    game_backup = REMOTE + "/data/hotfix-runtime-" + stamp + ".json"
    independent_replaced = False
    game_replaced = False
    try:
        if bridge_deploy.run(independent, "readlink -f " + INDEPENDENT_ROOT) != INDEPENDENT_ROOT:
            raise RuntimeError("独立热更目录不匹配")
        if game_deploy.run(game, "readlink -f " + REMOTE) != REMOTE:
            raise RuntimeError("游戏服目录不匹配")
        if "ActiveState=active" not in bridge_deploy.run(
                independent, "systemctl show hs-hotfix -p ActiveState"):
            raise RuntimeError("独立热更服不是active")
        if "ActiveState=active" not in game_deploy.run(
                game, "systemctl show hs-game -p ActiveState"):
            raise RuntimeError("游戏服不是active")

        independent_raw = read_bytes(independent, INDEPENDENT_FILE)
        game_raw = read_bytes(game, GAME_FILE)
        if independent_raw != game_raw:
            raise RuntimeError("两端生产热更字节不一致")
        catalog = validate_current(independent_raw, "独立热更端")
        validate_current(game_raw, "游戏服")

        catalog["runtime"]["script"] = compose_runtime(catalog["runtime"]["script"])
        catalog["runtime"]["index"] = EXPECTED_INDEX + 1
        target = (json.dumps(catalog, ensure_ascii=False, indent=2) + "\n").encode("utf-8")
        target_sha = hashlib.sha256(target).hexdigest()
        independent_temp = stage_bytes(independent, INDEPENDENT_FILE, target)
        game_temp = stage_bytes(game, GAME_FILE, target)
        if bridge_deploy.run(independent, "sha256sum " + independent_temp).split()[0] != target_sha:
            raise RuntimeError("独立热更临时文件校验失败")
        if game_deploy.run(game, "sha256sum " + game_temp).split()[0] != target_sha:
            raise RuntimeError("游戏服临时文件校验失败")

        bridge_deploy.run(independent, "cp -a %s %s && mv -f %s %s && systemctl restart hs-hotfix && systemctl is-active --quiet hs-hotfix" %
                          (INDEPENDENT_FILE, independent_backup, independent_temp, INDEPENDENT_FILE))
        independent_replaced = True
        game_deploy.run(game, "cp -a %s %s && mv -f %s %s && systemctl restart hs-game && systemctl is-active --quiet hs-game" %
                        (GAME_FILE, game_backup, game_temp, GAME_FILE))
        game_replaced = True

        independent_final = bridge_deploy.run(independent, "sha256sum " + INDEPENDENT_FILE).split()[0]
        game_final = game_deploy.run(game, "sha256sum " + GAME_FILE).split()[0]
        if independent_final != target_sha or game_final != target_sha:
            raise RuntimeError("发布后两端哈希不一致")
        print(json.dumps({
            "状态": "runtime闭包双端发布通过",
            "runtime_index": catalog["runtime"]["index"],
            "SHA256": target_sha,
            "独立热更备份": independent_backup,
            "游戏服备份": game_backup,
        }, ensure_ascii=False))
    except Exception:
        if game_replaced:
            game_deploy.run(game, "cp -a %s %s && systemctl restart hs-game" %
                            (game_backup, GAME_FILE))
        if independent_replaced:
            bridge_deploy.run(independent, "cp -a %s %s && systemctl restart hs-hotfix" %
                              (independent_backup, INDEPENDENT_FILE))
        raise
    finally:
        independent.close()
        game.close()


if __name__ == "__main__":
    sys.stdout.reconfigure(encoding="utf-8")
    main()
