"""导出本项目Android活动模块与原生表；外部参考不参与数值。"""
import hashlib,json,sys
from pathlib import Path
from mem_marshal_extract import Loader
from neox_dis import disasm
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
inventory=json.loads((root/'out/npk_scripts/android_inventory.json').read_text(encoding='utf-8'))
names=['exam','summer','cthulhu','miku','mountain','wangyan','monster_nian','house_frage','card_house','frage_stone']
source={};tables={};modules=[]
for p in (root/'out/client_catalogs/tables').glob('*.json'):
 if not any(n in p.stem for n in names) and p.stem not in ['activity_detail','activity_type','bonus','materials','base_target','tower_task','dungeons','item_libs','north_wind_footprint','north_wind_footprint_bonus','handbook','handbook_item','storyline_bonus','cards','card_to_fragment','facility_board_game']:continue
 raw=p.read_bytes();tables[p.stem]=json.loads(raw)['数据'];source[p.stem]={'路径':p.relative_to(root).as_posix(),'SHA256':hashlib.sha256(raw).hexdigest()}
for mod in inventory['modules']:
 fn=mod['filename']
 if not any(n in fn for n in names):continue
 if not (fn.endswith('_mgr.py') or fn.startswith(('custom_types\\','common_logic\\')) or 'activity_cthulhu_check' in fn or 'cthulhu_game.py' in fn or 'house_frage' in fn):continue
 p=root/'out/npk_scripts'/mod['marshal_file'];raw=p.read_bytes()
 modules.append({k:mod[k] for k in ['hash','filename','source','marshal_file','marshal_sha256']})
 out=root/'out/dis'/('activity-'+mod['hash']+'.asm')
 with out.open('w',encoding='utf-8') as f:disasm(Loader(raw).r_object(),out=f)
 print(fn,mod['hash'],[m['name'].split('.')[-1] for m in mod['methods'] if m['argcount']])
result={'表':tables,'模块':modules,'来源':source}
(root/'internal/game/activity_catalog.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('活动目录：',len(tables),'张原生表；',len(modules),'个原生模块。')
