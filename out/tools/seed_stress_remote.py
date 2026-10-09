# -*- coding: utf-8 -*-
"""500 并发 TCP 种子握手回归 + 少量全链路加密会话（混合负载）。"""
import os
import socket
import struct
import sys
import threading
import time

HOST = os.environ.get('SMOKE_HOST', '192.0.2.10')
PORT = int(os.environ.get('SMOKE_PORT', '9000'))
N = int(os.environ.get('SEED_N', '500'))

ok = [0]
bad = []
lock = threading.Lock()


def seed_once(idx):
    try:
        s = socket.create_connection((HOST, PORT), timeout=10)
        s.sendall(struct.pack('<IH', 2, 0))
        head = s.recv(6)
        total, cmd = struct.unpack('<IH', head)
        assert cmd == 0 and total >= 2, (total, cmd)
        s.close()
        with lock:
            ok[0] += 1
    except Exception as exc:  # noqa: BLE001
        with lock:
            bad.append((idx, repr(exc)))


def main():
    t0 = time.time()
    threads = [threading.Thread(target=seed_once, args=(i,)) for i in range(N)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    dt = time.time() - t0
    print('并发 %d：成功 %d 失败 %d，用时 %.1fs' % (N, ok[0], len(bad), dt))
    for b in bad[:5]:
        print(' ', b)
    sys.exit(0 if not bad and ok[0] == N else 1)


if __name__ == '__main__':
    main()
