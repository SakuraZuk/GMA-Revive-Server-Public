# -*- coding: utf-8 -*-
"""只读检查生产角色的引导任务与当前战斗观察事件；不输出数据库口令。"""
import argparse
import json
import re
import sys

from server_ops import connect, run


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("avatar_oid", help="角色 ObjectID，24 位十六进制")
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9a-fA-F]{24}", args.avatar_oid):
        raise SystemExit("角色 ObjectID 必须是 24 位十六进制")
    oid = args.avatar_oid.lower()
    remote = f'''from pathlib import Path
from urllib.parse import urlsplit
import os, subprocess
raw = next(line.split('=', 1)[1].strip().strip(chr(34)).strip(chr(39))
           for line in Path('/opt/hs-server/data/database.env').read_text().splitlines()
           if line.startswith('HS_DATABASE_URL='))
u = urlsplit(raw)
env = dict(os.environ, PGHOST=u.hostname, PGPORT=str(u.port), PGUSER=u.username,
           PGPASSWORD=u.password, PGDATABASE=u.path.lstrip('/'),
           LD_LIBRARY_PATH='/opt/hs-server/pgsql/runtime/usr/lib/x86_64-linux-gnu')
sql = "SELECT json_build_object('guide_tasks',state->'guide_tasks','battle',state->'server_battle','revision',revision,'level',state->'server_avatar_level','power',state->'power','materials',state->'material_mgr','basic_rewards',state->'server_basic_rewards','cleared_dungeons',state->'cleared_dungeons') FROM avatar_progress WHERE avatar_oid=decode('{oid}','hex')"
result = subprocess.run(['/opt/hs-server/pgsql/runtime/usr/lib/postgresql/15/bin/psql',
                         '-X','-A','-t','-v','ON_ERROR_STOP=1','-c',sql], env=env,
                        capture_output=True, text=True, check=True)
print(result.stdout.strip())
'''
    client = connect()
    try:
        raw = run(client, "python3 - <<'PY'\n" + remote + "\nPY")
    finally:
        client.close()
    print(json.dumps(json.loads(raw), ensure_ascii=False, indent=2))


if __name__ == "__main__":
    sys.stdout.reconfigure(encoding="utf-8")
    main()
