# -*- coding: utf-8 -*-
"""启动临时加密网关进程，跑协议冒烟并清理进程，不操作客户端。"""
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time

import smoke_gate

sys.stdout.reconfigure(encoding="utf-8")
root = Path(__file__).resolve().parents[2]
with socket.socket() as socket_probe:
    socket_probe.bind(("127.0.0.1",0))
    port=socket_probe.getsockname()[1]
environment=os.environ.copy()
environment.update(HS_GAME_BIND=f"127.0.0.1:{port}", HS_GAME_DEBUG_BIND="",
                   HS_GAME_RSA_KEY=str(root/"deploy/data/login_key.pem"), HS_DATABASE_URL="")
with tempfile.TemporaryFile() as log:
    process=subprocess.Popen([str(root/"out/bin/windows/gameserver.exe")],cwd=root,env=environment,
                             stdout=log,stderr=log,creationflags=subprocess.CREATE_NO_WINDOW)
    try:
        for attempt in range(60):
            try:
                with socket.create_connection(("127.0.0.1",port),timeout=1):
                    break
            except OSError:
                time.sleep(.1)
        else:
            raise RuntimeError("临时游戏服未启动")
        smoke_gate.HOST,smoke_gate.PORT="127.0.0.1",port
        os.environ['SMOKE_REPORT']=str(root/'out/gate-local-smoke.json')
        smoke_gate.main()
    except Exception:
        log.seek(0)
        print(log.read().decode("utf-8"))
        raise
    finally:
        process.terminate()
        process.wait(timeout=5)
