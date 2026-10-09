"""导出Android成就目标和奖励，包括即时礼物盒的原生物品库。"""
import hashlib
import json
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8")
sys.stderr.reconfigure(encoding="utf-8")
root=Path(__file__).resolve().parents[2]
sources={}
def load(name):
    path=root/"out/client_catalogs/tables"/(name+".json")
    raw=path.read_bytes();sources[name]={"路径":path.relative_to(root).as_posix(),"SHA256":hashlib.sha256(raw).hexdigest()}
    return json.loads(raw)["数据"]
achvs=load("achv");targets=load("base_target");bonuses=load("bonus");materials=load("materials")
libs=load("item_libs");weights=load("item_libs_data")
rewards={}
def reward(bid,stack=()):
    if str(bid) in rewards:return
    if bid in stack:raise ValueError("成就奖励循环")
    row=bonuses[str(bid)]
    if any(row.get(k) for k in ("inner_bonus_id","random_items","random_runes","begin_time","end_time","lucky_rule_id")):
        raise ValueError("成就奖励含未支持结构")
    groups=[]
    for group in row.get("random_item_libs") or []:
        if len(group)!=4 or group[1:]!=[1,1,1]:raise ValueError("即时奖励物品库参数未取证")
        choices=[]
        for key,pair in weights[str(group[0])].items():
            item=libs[key]
            if pair[1] or item.get("unlock_condition") or item.get("item_lib") or len(item["item_count"])!=1:raise ValueError("奖励物品库存在动态条件")
            if pair[0]<=0 or item["item_count"][0]<=0:raise ValueError("奖励权重或数量无效")
            choices.append({"weight":pair[0],"item_id":item["item_id"],"count":item["item_count"][0]})
        groups.append(choices)
    rewards[str(bid)]={"fixed":row.get("fixed_items") or [],"groups":groups}
    for mid in [v[0] for v in rewards[str(bid)]["fixed"]]+[x["item_id"] for g in groups for x in g]:
        mat=materials[str(mid)]
        if mat["type"]==7 and mat["stype"]==1:reward(mat["target_id"],stack+(bid,))
        elif mat["type"] not in (3,4,8,9):raise ValueError("成就材料类型尚未支持")
for row in achvs.values():
    if row.get("bonus_by_mail"):raise ValueError("存在邮件发成就奖励")
    if row.get("bonus_id"):reward(row["bonus_id"])
result={"来源":sources,"achievements":achvs,"targets":targets,"rewards":rewards,
        "materials":{k:{"type":v["type"],"stype":v.get("stype"),"target":v.get("target_id")} for k,v in materials.items()}}
(root/"internal/game/achievement_catalog.json").write_text(json.dumps(result,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
print("已生成成就目录",len(achvs),"奖励目录",len(rewards))
