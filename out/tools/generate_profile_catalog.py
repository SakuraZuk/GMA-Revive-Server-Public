"""按本项目Android版本生成头像与统计目录，保留原表哈希。"""
import hashlib,io
import json
import sys
from pathlib import Path
from mem_marshal_extract import Loader
from neox_dis import disasm

sys.stdout.reconfigure(encoding="utf-8")
root=Path(__file__).resolve().parents[2]
sources={}
def load(name):
    path=root/"out/client_catalogs/tables"/(name+".json")
    raw=path.read_bytes()
    sources[name]={"路径":path.relative_to(root).as_posix(),"SHA256":hashlib.sha256(raw).hexdigest()}
    return json.loads(raw)["数据"]
heads=load("head_box_info");cards=load("cards");dungeons=load("dungeons");targets=load("base_target");materials=load("materials")
result={"来源":sources,"heads":heads,
        "claim_time_frames":{k:r['target_id'] for k,r in materials.items() if r.get('type')==9 and heads[str(r['target_id'])].get('limit_days') and '以领取奖励时刻开始计时' in r.get('desc','')},
        "head_targets":{str(h['unlock_target_id']):targets[str(h['unlock_target_id'])] for h in heads.values() if h.get('unlock_target_id')},
        "counted_cards":[int(k) for k,r in cards.items() if not r.get("disable") and not r.get("not_in_stat")],
        "main_dungeons":[int(k) for k,r in dungeons.items() if r["dungeon_type"]==1 and not r.get("_dungeon_disable")]}
if not result["counted_cards"] or not result["main_dungeons"]:
    raise ValueError("头像统计目录缺失")
for module in ('F87E99A2','6562F4CC','91074C76'):
    path=root/'out/npk_scripts/android_base'/(module+'.marshal');raw=path.read_bytes();buffer=io.StringIO();disasm(Loader(raw).r_object(),out=buffer)
    (root/'out/dis'/('profile-limited-'+module+'-native.asm')).write_text(buffer.getvalue(),encoding='utf-8')
    sources[module]={'路径':path.relative_to(root).as_posix(),'SHA256':hashlib.sha256(raw).hexdigest()}
(root/"internal/game/profile_catalog.json").write_text(json.dumps(result,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
print("已生成头像目录",len(heads),"和统计卡/主线目录",len(result["counted_cards"]),len(result["main_dungeons"]))
