"""从已授权热更主机只读探测游戏SSH，不发送游戏口令。"""
import json
import time
from server_ops import HOST, PORT
from deploy_hotfix_bridge import connect
started=time.monotonic()
jump=connect()
try:
    channel=jump.get_transport().open_channel('direct-tcpip',(HOST,PORT),('127.0.0.1',0),timeout=15)
    try:
        channel.settimeout(15)
        banner=channel.recv(256).decode('ascii','replace').splitlines()[0]
        print(json.dumps({'握手首行':banner,'秒':round(time.monotonic()-started,3)},ensure_ascii=False))
    finally:
        channel.close()
finally:
    jump.close()
