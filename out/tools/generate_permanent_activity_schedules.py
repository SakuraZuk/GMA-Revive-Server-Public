"""按用户授权生成本服常驻运营日程，原表节点资格及奖励不变。"""
import json
import sys
from datetime import datetime, timezone, timedelta
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
ROOT=Path(__file__).resolve().parents[2]
doc=json.loads((ROOT/'out/client_catalogs/tables/activity_type.json').read_text(encoding='utf-8'))
begin=int(datetime(2026,10,7,tzinfo=timezone(timedelta(hours=8))).timestamp())
rows=[{'activity_id':int(key),'begin_time':begin,'end_time':0,'enabled':True,'permanent':True} for key in sorted(doc['数据'],key=int)]
target=ROOT/'deploy/data/activity-schedules.json'
target.write_text(json.dumps(rows,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('已生成全部%d项常驻活动；起始时间2026-10-07北京时间零点' % len(rows))
