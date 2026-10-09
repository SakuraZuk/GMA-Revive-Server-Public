# -*- coding: utf-8 -*-
"""项目服务只读检查及单游戏二进制发布；凭据沿用已授权的本机来源。"""
import argparse
import hashlib
import json
import re
import shlex
import sys
from datetime import datetime
from pathlib import Path
import paramiko
from deploy_game_update import HOST, PORT, USER, PW

ROOT = Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')

def connect():
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    try:
        client.connect(HOST, port=PORT, username=USER, password=PW, timeout=60,
                       banner_timeout=60, auth_timeout=60, channel_timeout=60,
                       allow_agent=False, look_for_keys=False)
    except BaseException:
        client.close()
        raise
    return client

def run(client, command, timeout=60):
    _, out, err = client.exec_command(command, timeout=timeout)
    body = out.read().decode('utf-8')
    error = err.read().decode('utf-8')
    if out.channel.recv_exit_status():
        raise RuntimeError('远端检查失败：' + error)
    return body.strip()

def audit(client):
    result = {'检查时间': datetime.now().isoformat(), '服务器': HOST,
              '监听': run(client, 'ss -lntup'),
              '项目目录': run(client, 'readlink -f /opt/hs-server'),
              '游戏哈希': run(client, 'sha256sum /opt/hs-server/bin/gameserver'),
              '服务': run(client, 'systemctl show hs-game hs-sdk hs-login hs-postgres hs-dns -p Id -p ActiveState -p MainPID -p FragmentPath')}
    # 只解析连接目的地，绝不打印环境文件、口令或完整数据库 URL。
    result['数据库目标'] = run(client, "python3 - <<'PY'\nfrom pathlib import Path\nfrom urllib.parse import urlsplit\nfor p in [Path('/opt/hs-server/data/database.env')]:\n if p.exists():\n  for line in p.read_text().splitlines():\n   if line.startswith('HS_DATABASE_URL='):\n    u=urlsplit(line.split('=',1)[1].strip().strip(chr(34)).strip(chr(39)))\n    print('主机=%s 端口=%s 库=%s 用户=%s'%(u.hostname,u.port,u.path,u.username))\nPY")
    return result

def reap_holders(client, path):
    """清理持有目标文件句柄的僵尸 sftp-server（ETXTUB/203 崩溃循环根因）。
    只杀 sftp-server 进程（不碰 systemd/gameserver 自身），返回被清理的 PID 列表。"""
    script = (
        "for p in /proc/[0-9]*; do "
        "  exe=$(readlink $p/exe 2>/dev/null); "
        "  case \"$exe\" in */sftp-server) ;; *) continue;; esac; "
        "  for f in $p/fd/*; do "
        "    t=$(readlink $f 2>/dev/null); "
        "    [ \"$t\" = %s ] && { echo ${p#/proc/}; break; }; "
        "  done; "
        "done" % shlex.quote(path))
    pids = [line for line in run(client, script).splitlines() if line.isdigit()]
    for pid in pids:
        run(client, 'kill -9 '+pid)
    return pids

def upload_resumable(open_client, local, remote, chunk=256*1024, attempts=12):
    """分块断点续传上传（共享主机 SSH 频繁瞬断的适配）：
    每次尝试重开 SSH 连接与 sftp；远端已有字节 r+b seek 续传；远端超长则重传。"""
    import time
    total = local.stat().st_size
    last_error = None
    for attempt in range(attempts):
        try:
            client = open_client()
            try:
                sftp = client.open_sftp()
                try:
                    try:
                        remote_size = sftp.stat(remote).st_size
                    except IOError:
                        sftp.open(remote, 'wb').close()
                        remote_size = 0
                    if remote_size > total:
                        sftp.remove(remote)
                        remote_size = 0
                    with local.open('rb') as src, sftp.open(remote, 'r+b') as dst:
                        # 批量发送写请求；退出with时close收齐全部确认，失败仍走断点重试。
                        # 禁止把write返回当上传完成，正式安装/测试前仍必须验证远端整文件SHA。
                        dst.set_pipelined(True)
                        offset = remote_size
                        src.seek(offset)
                        dst.seek(offset)
                        while offset < total:
                            data = src.read(chunk)
                            if not data:
                                break
                            dst.write(data)
                            offset += len(data)
                finally:
                    sftp.close()
            finally:
                client.close()
            return total
        except Exception as error:
            if isinstance(error, paramiko.AuthenticationException):
                raise
            last_error = error
            print('上传中断（第 %d 次）：%s，重连续传' % (attempt+1, error))
            time.sleep(2)
    raise RuntimeError('分块续传失败：'+str(last_error))

def release(open_client):
    client = open_client()
    before = audit(client)
    target = '/opt/hs-server/bin/gameserver'
    local = ROOT/'out/bin/linux-amd64/gameserver'
    expected = hashlib.sha256(local.read_bytes()).hexdigest()
    assert before['项目目录'] == '/opt/hs-server'
    assert 'Id=hs-game.service\nActiveState=active' in before['服务']
    listeners = run(client, 'ss -lntp')
    owned = [line for line in listeners.splitlines() if re.search(r':9000\s', line)]
    assert len(owned) == 1 and 'gameserver' in owned[0], '9000 监听不属于项目'
    reaped = reap_holders(client, target+'.new')
    if reaped:
        print('清理上次中断上传的僵尸句柄：'+','.join(reaped))
    stamp = datetime.now().strftime('%Y%m%d-%H%M%S')
    backup = '/opt/hs-server/bin/backup-' + stamp
    run(client, 'mkdir -p '+backup+' && cp -a '+target+' '+backup+'/gameserver')
    client.close()
    uploaded = upload_resumable(open_client, local, target+'.new')
    client = open_client()
    assert run(client, 'sha256sum '+target+'.new').split()[0] == expected
    try:
        run(client, 'chmod 755 '+target+'.new && mv '+target+'.new '+target+' && systemctl restart hs-game')
        run(client, 'systemctl is-active --quiet hs-game')
        assert run(client, 'sha256sum '+target).split()[0] == expected
    except Exception:
        # ETXTUB（203/EXEC）时先清句柄再回滚，避免回滚同样卡死。
        reap_holders(client, target)
        run(client, 'cp -a '+backup+'/gameserver '+target+'.rollback && mv '+target+'.rollback '+target+' && systemctl restart hs-game')
        raise
    return {'状态': '单游戏二进制发布通过', '备份': backup, 'SHA256': expected, '上传字节': uploaded,
            '清理僵尸句柄': reaped, '发布前': before, '发布后': audit(client)}

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['audit', 'logs', 'release'])
    parser.add_argument('--since', default='30 minutes ago')
    args = parser.parse_args()
    if args.action == 'release':
        result = release(connect)
        (ROOT/'out/server-release.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
        print(json.dumps(result,ensure_ascii=False,indent=2))
    else:
        client = connect()
        try:
            if args.action == 'logs':
                body = run(client, 'journalctl -u hs-game --since '+shlex.quote(args.since)+' --no-pager -n 300')
                (ROOT/'out/server-current.log').write_text(body+'\n', encoding='utf-8')
                for line in body.splitlines():
                    if re.search('失败|引导|connect_server|账号存储|监听|测速|超时|关闭|重连|读取结束|教学战斗',line): print(line)
            else:
                result = audit(client)
                (ROOT/'out/server-audit.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
                print(json.dumps(result,ensure_ascii=False,indent=2))
        finally: client.close()
