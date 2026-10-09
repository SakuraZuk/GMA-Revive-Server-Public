"""材料及家具加工目录：原生Android配方、设施解锁组、成本及源哈希。"""
import hashlib,json,sys
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2];sources={}
def load(name):
 p=root/'out/client_catalogs/tables'/(name+'.json');raw=p.read_bytes();sources[name]={'路径':p.relative_to(root).as_posix(),'SHA256':hashlib.sha256(raw).hexdigest()};return json.loads(raw)['数据']
fusion={}
for key,row in load('furniture_compose').items():
 fusion[key]={'comfort':row['satisfaction_condition'],'cost':row['consume_material'],'groups':[{'selector':g[0],'weight':g[1] or 0,'wish_probability':g[2] or 0} for g in row['furniture_compose_group']]}
result={'recipes':load('material_compose'),'unlocks':load('facility_process_unlock_materials'),'furniture_unlock':load('facility_unlock_furniture_compose'),'levels':load('facility_process'),'furniture_sources':load('furniture_source'),'fusion':fusion,'fusion_input_count':3,'来源':sources}
(root/'internal/game/craft_catalog.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('原生加工配方',len(result['recipes']))
