# -*- coding: utf-8 -*-
"""合并收藏室与特殊头像框剩余规则的可核证事实和待批准政策。"""
import hashlib
import json
from pathlib import Path

root = Path(__file__).resolve().parents[2]
def table(name):
    return json.loads((root / "out/client_catalogs/tables" / (name + ".json")).read_text(encoding="utf-8"))["数据"]

def source(path):
    p = root / path
    return {"路径": path, "SHA256": hashlib.sha256(p.read_bytes()).hexdigest()}

report = {
    "状态": "用户已明确批准统一推荐本服规则；已实现。本文件保留本服政策决策来源，现行详细实现见remaining-collection-report.json。",
    "范围": ["G05", "G07"],
    "已证事实": {
        "框80040": {"材料": table("materials")["80040"], "头像框": table("head_box_info")["1041"]},
        "住客窗口": table("house_daily_random_reward"),
        "住客规则数量": len(table("house_daily_random_reward_rule")),
        "能量基础": table("house_base")["1"],
        "设施1级单位秒数": {k: table(name)["1"]["unit_time"] for k, name in [("2", "facility_production1"), ("3", "facility_production2"), ("4", "facility_production3")]},
        "心情": "C31D8D11已证mood=min(mood_max,max(0,房间舒适度*mood_transform_rate))；get_mood_status按mood_threshold逐项取已达到最高门槛，索引+2，否则1，非入住get_mood_value返回0。",
        "房间3": "system_unlock的house_dormitory3版本1和2均通关104；房间目录有3，materials中只有809→1和810→2两把钥匙，无原生3钥匙。",
        "框列表排序": "91074C76.head_cmp按rank升序、缺rank排后，两者缺rank用ID升序；此为界面列表排序，没有证据表明是自动选择临时框优先级。",
        "住客wire": "5D1B302A:windowID→Int2TupleDict(cardID→Tuple)，F0E40550/F0BFB88A只读tuple[0]领取状态；CARD_CAN_GET_REWARD=1，CARD_HAS_GOT_REWARD=0。未见tuple其他项读法，也未见规则表[bonusID,第二数]的执行函数，第二数不能当倍率。",
        "教程": "ARM索引恢复后完整解码两份res全部23032项0错误；实际6004/26004图仅GuideFacilityAcc facility2/9000秒，原生gather_produce_material_speed_up无callback。6005/6006无加速节点，不创造额外支路。路径Murmur3核对见collection-graph-identity.json。",
    },
    "建议本服政策": [
        {"编号": "G05-起算", "建议": "80040与所有合法限时框材料统一在玩家领取时刻起算，单位严格沿limit_days小时；80040因此1小时。邮件签发及周结算不提前起算。"},
        {"编号": "G05-叠加", "建议": "同框有限期奖励新到期=max(领取时刻,旧到期)+份数*原表小时*3600。wire起算值写新到期减原表单份时长，可为未来时间；因此需同步修正旧previous>now校验。旧0永久拥有保留。整数份数、有限float/溢出及回拨严格校验。"},
        {"编号": "G05-选择", "建议": "多个临时框同时存在保留玩家当前合法选择，不自动覆盖；当前选择过期回默认3。客户端自行沿原rank排序列表。该政策不改变head_cmp的原生排序。"},
        {"编号": "G07-住客", "建议": "UTC+8每窗口首次进入收藏室时从当时已入住且mood_status满足表条件的cardID去重候选等概率抽random_num个；原表每窗口1个，跨房间全局共1个。按最高匹配mood_status选表项，freeze bonusID并只发bonus1次，第二整数原样内部保存以待补证，不解释为倍率；窗口/日期/card/bonus/实际box/领取标记同事务保存，候选不因反复进入重抽。过窗未领失效，不追造旧历史。领取要求仍在同一已解锁房间、拥有且当前窗口有效；抽中后资格冻结，搬出不得领取，搬回仍可领取一次。"},
        {"编号": "G07-能量", "建议": "生产设施已解锁且有入住时自动启动，最大能量=入住人数*30；base_cost/per_value严格按F0E40550.no_back_stage_oper原式，get按实际delta*per_value扣至0，30秒投影更新保留余量。最后住客离开先结旧速率并关闭；换人/设施升级先结旧状态再重算当前参数，不能凭客户端时间或重复读取免费重置。只计算已证能量消耗/显示，不把value转换成未证物品或免费加速。", "尚须定案": "耗尽是否关闭、重新入住是否补满、30秒周期是否重新填满在APK没有服务端资格；建议耗尽关闭，只有空房→首次再次入住才按当时人数重启，不每次读取补满。"},
        {"编号": "G07-房3", "建议": "真实通关104并取得house_dormitory3后自动一次建立房间3 ownership，不创造不存在的钥匙编号、不扣未证费用。设施4仍按材料803及原升级成本，房间1/2已有809/810机制保留。原数据只说明开放门槛，没有3号钥匙，因此明确作为本服政策。"},
        {"编号": "G07-教程", "建议": "原件恢复已覆盖早期固定周期提案：只6004/26004实际进行且已拥有设施2，原件9000秒一次，保留原生参数不改客户端；保存任务收据，同事务增加Keep且遵守Storage，不把ProduceStart推到未来。6005/6006没有节点，不增造任意免费加速。"},
    ],
    "资源恢复结果": "已通过原生ARM libclient.so核对flag>=62 offset=(-100-decoded)^encoded^0x7a090d89，完成全部索引合法范围和非重叠、压缩块长度及解码。两份res扫描0错误，两个任务图同SHA；精确路径与hash及原件证据见collection-resource-scan.json和collection-graph-identity.json。先前失败快照只保留为诊断历史，禁止按其断言图缺失。",
    "来源": [source(p) for p in ["out/dis/remaining-frame-priority.asm", "out/dis/remaining-frame-update.asm", "out/dis/remaining-guide-acc.asm", "out/dis/remaining-collection-mood.asm", "out/dis/remaining-collection-mood-formulas.asm", "out/client_catalogs/tables/house_daily_random_reward_rule.json", "out/client_catalogs/tables/house_base.json", "out/client_catalogs/tables/system_unlock.json"]],
}
out = root / "out/remaining-collection-policy-proposal.json"
out.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(out)
