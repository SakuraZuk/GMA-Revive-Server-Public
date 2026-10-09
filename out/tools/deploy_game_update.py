# -*- coding: utf-8 -*-
"""游戏服 192.0.2.10 更新部署：备份→上传→RSA 配置→重启→验证。

操作范围仅限 /opt/hs-server 与 hs-* systemd 单元。
"""
import sys
import time

import paramiko

from local_ops_credentials import load_target
HOST, PORT, USER, PW = load_target('game')
LOCAL = r'.\out\bin\linux-amd64'
LOCAL_KEY = r'.\deploy\data\login_key.pem'
REMOTE = '/opt/hs-server'


def run(c, cmd, check=True):
    _, out, err = c.exec_command(cmd, timeout=120)
    code = out.channel.recv_exit_status()
    text = out.read().decode('utf-8', 'replace') + err.read().decode('utf-8', 'replace')
    print('$ %s\n%s' % (cmd, text.strip()))
    if check and code != 0:
        raise SystemExit('命令失败(%d): %s' % (code, cmd))
    return text


def main():
    c = paramiko.SSHClient()
    c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    c.connect(HOST, port=PORT, username=USER, password=PW, timeout=20, allow_agent=False, look_for_keys=False)

    # 1. 备份与目录
    stamp = time.strftime('%Y%m%d-%H%M%S')
    run(c, 'mkdir -p %s/bin/backup-%s && cp -a %s/bin/*.server %s/bin/backup-%s/ 2>/dev/null; '
           'cp -a %s/bin/gameserver %s/bin/hotfixserver %s/bin/loginserver %s/bin/sdkserver %s/bin/backup-%s/ 2>/dev/null; ls %s/bin/backup-%s'
        % (REMOTE, stamp, REMOTE, REMOTE, stamp, REMOTE, REMOTE, REMOTE, REMOTE, REMOTE, stamp, REMOTE, stamp))

    # 2. 上传新二进制与私钥
    sftp = c.open_sftp()
    for name in ('gameserver', 'loginserver', 'sdkserver', 'hotfixserver'):
        sftp.put('%s/%s' % (LOCAL, name), '%s/bin/%s.new' % (REMOTE, name))
    sftp.put(LOCAL_KEY, '%s/bin/login_key.pem' % REMOTE)
    sftp.close()
    for name in ('gameserver', 'loginserver', 'sdkserver', 'hotfixserver'):
        run(c, 'chmod 755 {r}/bin/{n}.new && mv {r}/bin/{n}.new {r}/bin/{n}'.format(r=REMOTE, n=name))
    run(c, 'chmod 600 %s/bin/login_key.pem' % REMOTE)

    # 3. hs-game 单元增加 RSA 私钥配置（幂等）
    run(c, "grep -q HS_GAME_RSA_KEY /etc/systemd/system/hs-game.service || "
           "sed -i '/Environment=HS_MAX_FRAME_PAYLOAD/a Environment=HS_GAME_RSA_KEY=/opt/hs-server/bin/login_key.pem' /etc/systemd/system/hs-game.service; "
           "systemctl daemon-reload")

    # 4. 重启（先 game，失败即回滚提示）
    run(c, 'systemctl restart hs-game && sleep 2 && systemctl is-active hs-game && ss -tlnp | grep :9000')
    # login/sdk 二进制同步更新
    run(c, 'systemctl restart hs-login hs-sdk && sleep 1 && systemctl is-active hs-login hs-sdk')

    print('\n部署完成，备份目录 bin/backup-%s' % stamp)
    c.close()


if __name__ == '__main__':
    sys.exit(main())
