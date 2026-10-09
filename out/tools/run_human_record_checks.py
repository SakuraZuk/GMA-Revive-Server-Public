# -*- coding: utf-8 -*-
"""真人持久投递/录像专项验证，记录退出码、SKIP边界与源文件SHA。"""
import datetime
import hashlib
import json
import os
import subprocess
import sys
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
GO=__import__('os').environ.get('HS_GO', 'go')
commands=[
    ('真人和录像源码', [GO,'test','./internal/game','-run','^(TestHumanPvp|TestNativeRecord|TestSocial)','-count=1','-v']),
    ('最新PG编译与本机边界', [GO,'test','./internal/game/dbstore','-run','^(TestPostgresHuman|TestPostgresNativeRecord)','-count=1','-v']),
    ('原生录像文件和续传模型', ['python','out/tools/test_native_record_bridge.py']),
    ('真人原生桥模型', ['python','out/tools/verify_human_bridge.py']),
]
report={'时间':datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat(),'范围':'仅本地/模型，不替代远端PG、Android或持续负载','结果':[]}
env=dict(os.environ,PYTHONIOENCODING='utf-8')
for index,(name,command) in enumerate(commands):
    completed=subprocess.run(command,cwd=ROOT,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
    text=completed.stdout.decode('utf-8','replace');log=ROOT/f'out/human-record-check-{index+1}.log';log.write_text(text,encoding='utf-8')
    report['结果'].append({'检查':name,'命令':command,'退出码':completed.returncode,'日志':str(log.relative_to(ROOT)),'PASS':text.count('--- PASS:'),'SKIP':text.count('--- SKIP:')})
paths=['internal/game/sync_pvp_human.go','internal/game/human_delivery.go','internal/game/human_delivery_test.go','internal/game/social_presence.go','internal/game/social_business.go','internal/game/native_record.go','internal/game/native_record_pickle.go','internal/game/native_record_catalog.json','internal/game/native_record_test.go','internal/game/native_record_bridge_script.py','internal/game/record_business.go','internal/game/db/schema.sql','internal/game/dbstore/human_presence.go','internal/game/dbstore/human_delivery_test.go','internal/game/dbstore/native_record.go','internal/game/dbstore/native_record_test.go']
report['源码SHA256']=[{'文件':p,'SHA256':hashlib.sha256((ROOT/p).read_bytes()).hexdigest()} for p in paths]
report['已知修复过程']=['首次Human事务测试发现创建房间逐槽复制造成两份投递镜像不一致，改为构造全部描述符后再复制；后续5项Human全部通过','早期录像样本被测试误认未结束，静态解析确认包含完整结束通知；改为验证真实样本完整兼容','Windows模型rename不覆盖目标，模型用replace模拟Android/Linux原生rename覆盖语义；客户端源码仍采用本版目标平台语义','C3 helper并行签名从错误假设返回error改为无返回直接调用，最新编译通过后记录']
(ROOT/'out/human-record-local-verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
print(json.dumps(report['结果'],ensure_ascii=False,indent=2))
raise SystemExit(1 if any(row['退出码'] for row in report['结果']) else 0)
