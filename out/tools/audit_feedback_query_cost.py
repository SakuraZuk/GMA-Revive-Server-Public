"""只读EXPLAIN统计反馈查询耗时，不导出角色、成绩、库存或查询计划正文。"""
import json
import re
import shlex
import sys
from pathlib import Path
from audit_live_pvp_state import REMOTE
from server_ops import connect, run

sys.stdout.reconfigure(encoding='utf-8')
root = Path(__file__).resolve().parents[2]
projection = re.search(r'const assistRefreshStateSQL = `([^`]+)`',
                      (root/'internal/game/dbstore/assist_refresh.go').read_text(encoding='utf-8'))[1]
queries = {
    '同步竞技现有排行': "SELECT a.avatar_oid,COALESCE((p.state->>'sync_pvp_score')::int,0) AS score FROM avatars a JOIN avatar_progress p USING(avatar_oid) WHERE COALESCE((p.state->>'sync_pvp_score')::int,0)>0 AND a.hostnum=(SELECT hostnum FROM avatars GROUP BY hostnum ORDER BY count(*) DESC LIMIT 1) ORDER BY score DESC,a.avatar_oid LIMIT 100",
    '助战候选现有投影': "SELECT "+projection+" FROM avatars a JOIN avatar_progress p USING(avatar_oid) WHERE a.hostnum=(SELECT hostnum FROM avatars GROUP BY hostnum ORDER BY count(*) DESC LIMIT 1) AND COALESCE(p.state->>'assist_card_uuid','')<>'' ORDER BY a.avatar_oid LIMIT 1000",
}
candidate_projection = "jsonb_build_object('server_avatar_level',r.server_avatar_level,'selected_head_id',r.selected_head_id,'selected_head_box_id',r.selected_head_box_id,'custom_head_cleared',r.custom_head_cleared,'sync_pvp_score',r.sync_pvp_score,'assist_card_uuid',r.assist_card_uuid,'cards',(SELECT COALESCE(jsonb_agg(card),'[]'::jsonb) FROM jsonb_array_elements(COALESCE(r.cards,'[]'::jsonb)) card WHERE card->>'uuid'=r.assist_card_uuid),'server_social',jsonb_build_object('friends',r.server_social->'friends','blacklist',r.server_social->'blacklist'))"
queries['助战资格预选提案'] = "SELECT "+candidate_projection+" FROM avatars a JOIN avatar_progress p USING(avatar_oid) CROSS JOIN LATERAL jsonb_to_record(p.state) AS r(server_avatar_level int,selected_head_id int,selected_head_box_id int,custom_head_cleared boolean,sync_pvp_score int,assist_card_uuid text,cards jsonb,server_social jsonb) WHERE a.hostnum=(SELECT hostnum FROM avatars GROUP BY hostnum ORDER BY count(*) DESC LIMIT 1) AND COALESCE(r.assist_card_uuid,'')<>'' ORDER BY a.avatar_oid LIMIT 1000"
queries['同步竞技单次提取提案'] = "SELECT a.avatar_oid,COALESCE(r.sync_pvp_score,0) AS score,COALESCE(NULLIF(r.selected_head_id,0),a.head_id),COALESCE(NULLIF(r.selected_head_box_id,0),a.head_box_id),CASE WHEN COALESCE(r.custom_head_cleared,false) THEN '' ELSE a.custom_head_image_url END FROM avatars a JOIN avatar_progress p USING(avatar_oid) CROSS JOIN LATERAL jsonb_to_record(p.state) AS r(sync_pvp_score int,selected_head_id int,selected_head_box_id int,custom_head_cleared boolean) WHERE COALESCE(r.sync_pvp_score,0)>0 AND a.hostnum=(SELECT hostnum FROM avatars GROUP BY hostnum ORDER BY count(*) DESC LIMIT 1) ORDER BY score DESC,a.avatar_oid LIMIT 100"
prefix = REMOTE[:REMOTE.index('sql=')]
remote = prefix + '\nqueries='+repr(queries)+r'''
reports={}
for name,query in queries.items():
 result=subprocess.run(['/opt/hs-server/pgsql/runtime/usr/lib/postgresql/15/bin/psql','-X','-A','-t','-v','ON_ERROR_STOP=1','-c','EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) '+query],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=10)
 if result.returncode:
  reports[name]={'状态':'失败或超时','超时':b'statement timeout' in result.stderr}
  continue
 value=json.loads(result.stdout.decode())[0]
 plan=value['Plan']
 reports[name]={'状态':'通过','规划毫秒':value.get('Planning Time'),'执行毫秒':value.get('Execution Time'),'实际行数':plan.get('Actual Rows'),'共享命中块':plan.get('Shared Hit Blocks'),'共享读取块':plan.get('Shared Read Blocks')}
print(json.dumps(reports,ensure_ascii=False))
'''
client=connect()
try:
    result=json.loads(run(client,'python3 -c '+shlex.quote(remote),timeout=30))
finally:
    client.close()
(root/'out/feedback-query-cost.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps(result,ensure_ascii=False,indent=2))
