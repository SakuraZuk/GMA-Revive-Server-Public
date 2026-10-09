# -*- coding: utf-8 -*-
"""进服故障只读采集：不输出登录参数、环境文件或连接口令。"""
import json
import re
import socket
import sys
import urllib.request
from datetime import datetime
from server_ops import connect, run, ROOT
from local_ops_credentials import load_target
GAME_HOST = load_target("game")[0]
HOTFIX_HOST = load_target("hotfix")[0]

sys.stdout.reconfigure(encoding='utf-8')
dest = ROOT / 'out/evidence/player-bugs-20261009' / ('login-' + datetime.now().strftime('%H%M%S'))
dest.mkdir(parents=True, exist_ok=True)
c = connect()
try:
    commands = {
        '服务日志': "journalctl -u hs-login -u hs-sdk -u hs-dns --since '20 minutes ago' --no-pager -n 250",
        '资源状态': "date -Is; uptime; free -m; df -h /opt/hs-server; ps -p $(systemctl show hs-game -p MainPID --value) -o pid,pcpu,pmem,rss,etime",
        '登录结果': "journalctl -u hs-game --since '20 minutes ago' --no-pager | grep -E 'login_result|玩家绑定|成为玩家失败|玩家初始属性编码失败|账号存储|quick_login.*失败|register_login.*失败|所选服务器' | tail -n 250",
    }
    for name, command in commands.items():
        body = run(c, command)
        (dest / (name + '.log')).write_text(body, encoding='utf-8')
        print(name, body[-14000:])
finally:
    c.close()
for host, port in [(GAME_HOST, 9000), (HOTFIX_HOST, 80)]:
    try:
        with socket.create_connection((host, port), 8):
            print('公网连接', host, port, '通过')
    except Exception as e:
        print('公网连接', host, port, type(e).__name__)
for url in ['http://' + HOTFIX_HOST + '/h62/pl/netease/patch_hotfix_data_pub']:
    try:
        with urllib.request.urlopen(url, timeout=15) as response:
            body = response.read()
            print('启动热更', response.status, '字节数', len(body))
    except Exception as e:
        print('启动热更', type(e).__name__)
print('证据目录', dest)
