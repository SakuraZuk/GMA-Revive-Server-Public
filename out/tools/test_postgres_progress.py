# -*- coding: utf-8 -*-
"""通过项目 SSH 隧道运行独立 schema 的 PostgreSQL 进度验收，不打印凭据。"""
import json
import os
from pathlib import Path
import select
import socketserver
import subprocess
import sys
import threading
import shutil
from datetime import datetime
from urllib.parse import urlsplit, urlunsplit

from server_ops import connect

ROOT = Path(__file__).resolve().parents[2]
GO = Path(__import__('os').environ.get('HS_GO', 'go'))
sys.stdout.reconfigure(encoding="utf-8")


def main():
    client = None
    server = None
    private_values = []
    selector = sys.argv[1] if len(sys.argv) > 1 else "PostgresRuneLoginMigrationPersistence|PostgresProgressIdempotencyRollbackAndPersistence|PostgresAdminRuneGrantAtomicAudit|PostgresIntimacyChapterRPCPersistence|PostgresSpecialGiftDailyBoxAndRollback|PostgresMailExpiryBatchRollbackAndPersistence|PostgresSyncPVPBridgeSettlementPersistence|PostgresSyncPVPLocalWindowAndTies"
    report = {"时间": datetime.now().isoformat(), "状态": "失败", "范围": "独立schema定向业务事务验收", "测试选择器": selector}
    try:
        client = connect()
        sftp = client.open_sftp()
        try:
            with sftp.open("/opt/hs-server/data/database.env", "rb") as source:
                lines = source.read().decode("utf-8").splitlines()
        finally:
            sftp.close()
        matches = [line.split("=", 1)[1].strip().strip("\"'") for line in lines if line.startswith("HS_DATABASE_URL=")]
        if len(matches) != 1:
            raise RuntimeError("项目数据库连接配置数量无效")
        original_url = matches[0]
        private_values.append(original_url)
        url = urlsplit(original_url)
        if url.hostname not in ("127.0.0.1", "localhost") or url.port != 15432 or url.path != "/hs":
            raise RuntimeError("数据库目标与已授权项目本机 PostgreSQL 不一致，停止验收")
        if url.password:
            private_values.append(url.password)
        transport = client.get_transport()

        class Forward(socketserver.BaseRequestHandler):
            def handle(self):
                channel = None
                try:
                    channel = transport.open_channel("direct-tcpip", (url.hostname, url.port), self.request.getpeername(), timeout=15)
                    while True:
                        readable, _, _ = select.select([self.request, channel], [], [], 30)
                        if not readable:
                            continue
                        for endpoint in readable:
                            data = endpoint.recv(65536)
                            if not data:
                                return
                            (channel if endpoint is self.request else self.request).sendall(data)
                finally:
                    if channel is not None:
                        channel.close()

        class Tunnel(socketserver.ThreadingTCPServer):
            daemon_threads = True
            allow_reuse_address = False

        server = Tunnel(("127.0.0.1", 0), Forward)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        credentials = url.netloc.rsplit("@", 1)[0]
        local_url = urlunsplit((url.scheme, credentials + "@127.0.0.1:" + str(server.server_address[1]), url.path, url.query, url.fragment))
        private_values.append(local_url)
        env = dict(os.environ)
        env["HS_TEST_DATABASE_URL"] = local_url
        result = subprocess.run([str(GO), "test", "./internal/game/dbstore", "-run", selector, "-v", "-count=1"], cwd=ROOT, env=env, capture_output=True, timeout=180)
        output = (result.stdout + result.stderr).decode("utf-8", errors="replace")
        for value in private_values:
            output = output.replace(value, "[凭据已隐藏]")
        report.update({"退出码": result.returncode, "输出": output, "状态": "通过" if result.returncode == 0 and "--- SKIP" not in output else "失败"})
    except Exception as error:
        message = str(error)
        for value in private_values:
            message = message.replace(value, "[凭据已隐藏]")
        report["异常"] = {"类型": type(error).__name__, "消息": message}
    finally:
        if server is not None:
            server.shutdown()
            server.server_close()
        if client is not None:
            client.close()
    destination = ROOT / "out/postgres-progress-verification.json"
    if destination.exists():
        backup = ROOT / "out/backups" / ("postgres-progress-" + datetime.now().strftime("%Y%m%d-%H%M%S") + ".json")
        shutil.copy2(destination, backup)
    destination.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 0 if report["状态"] == "通过" else 1


if __name__ == "__main__":
    raise SystemExit(main())
