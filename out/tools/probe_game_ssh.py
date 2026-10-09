"""只读检测授权游戏主机SSH握手首行，不发送账号口令。"""
import json
import socket
import time
from server_ops import HOST, PORT
started=time.monotonic()
with socket.create_connection((HOST,PORT),timeout=10) as sock:
    sock.settimeout(10)
    banner=sock.recv(256).decode('ascii','replace').splitlines()[0]
print(json.dumps({'握手首行':banner,'秒':round(time.monotonic()-started,3)},ensure_ascii=False))
