"""复核现行余项；只生成审计证据，不更改游戏业务或伪造实机验收。"""
import datetime
import hashlib
import json
import re
import shutil
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8")
sys.stderr.reconfigure(encoding="utf-8")
ROOT = Path(__file__).resolve().parents[2]

def read(name):
    return (ROOT / name).read_text(encoding="utf-8-sig")

def proof(name, text):
    lines = read(name).splitlines()
    matches = [i + 1 for i, line in enumerate(lines) if text in line]
    if not matches:
        raise ValueError(f"源码证据不存在：{name} / {text}")
    return {"文件": name, "行": matches, "检索": text,
            "SHA256": hashlib.sha256((ROOT / name).read_bytes()).hexdigest()}

docs = {}
for path in sorted(ROOT.rglob("*.md")):
    content = path.read_text(encoding="utf-8-sig")
    if "\ufffd" in content:
        raise ValueError(f"文档含替代乱码：{path}")
    docs[path.relative_to(ROOT).as_posix()] = hashlib.sha256(path.read_bytes()).hexdigest()

old = json.loads(read("out/achievement-target-mapping-audit.json"))
missing = [row for row in old["成就"] if row["当前源码接线"] != "已接"]
types = sorted({target["target_type"] for row in missing for target in row["目标逐项"]
                if target["状态"].startswith("未接")})
# 不能将任意含有相同数字的 switch 当作成就事件接线。
source = "\n".join(path.read_text(encoding="utf-8") for path in (ROOT / "internal/game").glob("*.go")
                   if not path.name.endswith("_test.go"))
for kind in types:
    if re.search(r"advanceAchievement(?:Amount|Event)\(\s*p\s*,\s*" + str(kind) + r"\s*,", source):
        raise ValueError(f"旧清单中的成就类型 {kind} 已新增调用，需要重新审查")

groups = []
def gap(number, name, ids, detail, file, needle):
    groups.append({"编号": number, "功能": name, "清单项": ids, "实际缺口": detail,
                   "源码证据": [proof(file, needle)]})

gap("G01", "成就及对应底层业务", ["C3", "C8", "C10"],
    "31条成就仍缺目标接线：探索章节9、幻书委托2、绝密任务2、住客事件奖励3、收藏室给赞及获赞6、特别演练伤害3、学会及雅努斯6；须分别补业务或结算接线，不将成就未接等同于整个对应玩法都未实现。",
    "internal/game/achievement_business.go", "func advanceAchievementAmount")
gap("G02", "非好友助战", ["B4", "E1"], "当前选择与开战均走好友授权；陌生人候选、授权和计次尚未实现。",
    "internal/game/social_assist.go", "AuthorizedFriendAssist")
gap("G03", "送礼小数分支", ["B5"], "1.1/1.2倍率产生非整数时拒绝；原生舍入规则与该分支未完成。",
    "internal/game/intimacy_gifts.go", "整数")
gap("G04", "潜质重置", ["C3"], "升级已接；重置、退款规则及事务接口尚未完成，不应将整套技能升级列为未做。",
    "internal/game/card_skill_talent_business.go", "func (s *Service) cardSkillTalentRPC")
gap("G05", "头像框特殊规则", ["C5"], "80040起算来源、多份叠加期限及多个临时框优先级未闭环；基本获取和选择已接。",
    "internal/game/profile_cosmetics.go", "尚未取证")
gap("G06", "随机碎片合成", ["C7"], "2条随机碎片配方因等级保底语义缺证据而拒绝扣料；77条普通配方已接。",
    "internal/game/card_compose_business.go", "UnverifiedPromise")
gap("G07", "收藏室缺失分支", ["C8"], "随机住客礼物状态及抽取、能量启动/生成/关闭、房间3钥匙来源、教程加速资源仍缺；update_last_get_time只同步读取时间。房间3门槛104已经明确。收藏室点赞归入G01。",
    "internal/game/collection_energy.go", 'r.Energy["last_get_time"]')
gap("G08", "夏活数值和战斗效果", ["D2"], "封弊者+60%战斗属性未装配，鱼长精确分布未恢复；划船、钓鱼和领奖已接。",
    "internal/game/activity_business.go", "summerRPC")
gap("G09", "克苏鲁检定", ["D3"], "item_check与check_all_in直接拒绝；大成功/失败、SAN与消耗边界缺原服规则。",
    "internal/game/cthulhu_business.go", "判定边界尚缺")
gap("G10", "初音特殊奖励和战斗加成", ["C4", "D4"], "surprise奖励直接拒绝，手册战斗加成未装配；地图、任务和普通领奖已接。",
    "internal/game/miku_exploration.go", "不伪造发奖")
gap("G11", "山海数值及重赛规则", ["D5"], "材料加成舍入与常驻重赛政策未确定；守护、结界、排名与周期邮件已接。",
    "internal/game/activity_business.go", "mountain")
gap("G12", "汪言战斗加成", ["D6"], "level_correction、affected_by_nodes加成与占领后加成生效时点未闭环。",
    "internal/game/activity_business.go", "wangyanRPC")
gap("G13", "年兽综合榜", ["D7"], "三分榜合成、未参赛计分、同分排序、每小时总榜与周奖未实现；分榜和非周三日奖已完成。静态材料无法唯一推出公式，继续反推，不要求用户提供不存在的数据。",
    "internal/game/activity_business.go", "nian")
gap("G14", "宿舍骰子累计保底", ["D8"], "per_count当前采用累计Total原型，原生保底与动画候选规则未恢复；设施SSR效果已接。",
    "internal/game/house_frage_business.go", "houseFrageSSRWeight(r, w.Total)")
gap("G15", "随机礼盒邮件", ["E7"], "固定附件与各资产已接；type7礼盒附件及冻结随机结果收据未接，当前明确拒绝。",
    "internal/game/mail_business.go", "此邮件附件类型尚未取证")
groups[3]["源码证据"].append(proof("out/dis/activity-AD478664.asm", "code <module>/talent_tree_mgr/reset_talent_node"))
groups[7]["源码证据"].append(proof("internal/game/summer_business.go", "math.Ceil(bounds[0] * 100)"))
groups[10]["源码证据"].append(proof("internal/game/mountain_guard.go", "整数舍入需要额外取证"))
groups[12]["源码证据"].append(proof("internal/game/activity_rank.go", "不能生成总榜"))

shop = json.loads(read("internal/game/shop_catalog.json"))
goods = {key: row for key, row in shop["commodities"].items() if isinstance(row, dict)}
unsupported = [key for key, row in goods.items() if row.get("commodity_type") not in (1, 2, 3, 4)
               or row.get("rune_commodity_list")
               or (row.get("commodity_type") != 3 and row.get("discount_ratio_range"))]

result = {"北京时间": datetime.datetime.now().astimezone().isoformat(), "状态": "本地余项审计完成；实现和验证分层",
          "范围": "当前本地源码与已有报告复核；本轮不改游戏源码、不重新发布、不声称新增PG或Android验收",
          "全部已读MD": docs, "归并后的实现或规则缺口": groups,
          "归并说明": "15组不是15个独立接口；按A–F现行范围归并已确认缺口，共享缺口只算一组。未接成就不证明对应整个玩法未实现。泛型拒绝分支、未解锁校验不能自动算缺功能，不声称全客户端RPC穷尽审计。",
          "成就复核": {"已映射成就": len(old["成就"]) - len(missing), "缺接线成就": len(missing),
                     "缺接线类型": types, "缺接线明细": missing,
                     "边界": "复核旧报告31条缺口类型仍无直接事件调用；223仅已有映射，不等于逐目标原生全流程验收。"},
          "商店门槛复核": {"商品数": len(goods), "显式类型或契印列表门槛拒绝商品": unsupported,
                        "边界": "1011000带18个rune_commodity_list候选，可能为客户端选择容器；未证明UI直接购买该父项，不自动算缺功能。其他商品门槛仍需逐项业务验收。"},
          "仅待验证": ["Android新设备自然注册与教学闭环", "全玩法UI、战斗实际效果及冷恢复", "双Android PVP与原生录像采集播放", "活动全流程专项真实PG、家具融合政策事务覆盖", "持续并发负载及生产管理认证UI"],
          "不计必做缺实现": {"已完成实施": ["同步真人PVP", "同步机器人PVP", "异步真人防守PVP", "异步机器人PVP", "F2协议闸门", "F3证书续期", "F5当次缓存清理"],
                       "持续规程": ["A4文档维护", "F6协作规程"], "可选": ["A7全量事件二维回放"],
                       "范围边界": "真实网易SDK票据认证不属于当前快速登录闭环，不能在本轮擅自扩大必做范围。学会业务已有成就要求，应明确为G01底层缺口。"},
          "沿用证据": ["out/remote-database-verification.json", "out/native-solo-release-verification.json", "out/completion-build.json"]}
target = ROOT / "out/remaining-functions-audit-20261008.json"
target.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
if "--update-docs" in sys.argv:
    stamp = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    backup = ROOT / "out/backups" / ("remaining-audit-" + stamp)
    backup.mkdir(parents=True, exist_ok=True)
    def save(name, content):
        path = ROOT / name
        shutil.copy2(path, backup / (name.replace("/", "_") + ".snapshot"))
        path.write_text(content, encoding="utf-8")
    heading = "## 2026-10-08余项复核：实现缺口与验收分开"
    section = heading + "\n\n本轮已读取全部18份MD并核对当前源码和已有实库/发布报告。**A–F确认的实现或规则缺口归并为15组；不是46项功能都没做完，也不是只剩15个独立接口。**本轮只做审计和文档纠正，没有修改游戏业务、重新发布或新增实机验收。详细源码行号、SHA及31条成就明细见[审计报告](remaining-functions-audit-20261008.json)。\n\n"
    section += "| 缺口编号 | 真正未完成的部分 | 对应原清单 |\n|---|---|---|\n"
    for item in groups:
        section += f"| {item['编号']} {item['功能']} | {item['实际缺口']} | {','.join(item['清单项'])} |\n"
    section += "\n**已实现、只缺验证的内容单列：**新设备自然注册与教学；普通战斗和各系统全部UI/冷恢复；双Android PVP和原生录像；考试等活动全流程真实PG；家具融合正式政策实库事务覆盖；持续并发负载及管理认证UI。44项PG通过只覆盖日志中的具体用例，不能扩张到这些未测流程。\n\n"
    section += "**从缺实现列表移出：**同步真人/机器人及异步真人防守/机器人权威化已经发布；F2、F3、F5实施完成；A4/F6是持续工作规程；A7是可选全量事件回放。C9等级/誓约/知识对应真实PG已通过，余下是全来源边界及Android验证，不再笼统写同事务真实库未做。\n\n"
    section += "**计数与证据边界：**C3为254条成就，不是254个唯一目标；原报告有284个唯一目标。此次复核31条仍缺接线，涉及9种类型；223条只是已映射。收藏室房间3的系统门槛104已确认，缺的是钥匙/所有权来源。881个商品和145条日程均不等于全玩法已实现。商品1011000为带18候选的契印容器，尚未证明UI直接购买父项，不将其拒绝分支冒报为商店缺实现。\n\n"
    section += "**关闭顺序固定：**先补G01–G15已确认缺口并给出每组业务证据；再补未覆盖的真实PG流程；最后集中Android和负载验收。规则反推不能唯一确定时如实保留该小项，不把整套已完成系统退回未实现，也不要求用户重复提供其没有的数据。\n\n"
    name = "out/REPAIR-TASKS.md"
    content = read(name)
    if heading in content:
        start = content.index(heading)
        end = content.index("## 先处理的阻断与运营规则", start)
        content = content[:start] + section + content[end:]
    else:
        content = content.replace("## 先处理的阻断与运营规则", section + "## 先处理的阻断与运营规则", 1)
    content = re.sub(r"02:40历史检查：[^\n]*\n\n", "", content)
    content = content.replace("| 剩余实现或验收 |", "| 当前状态与待关闭层级 |")
    id_gaps = {}
    for item in groups:
        for number in item["清单项"]:
            id_gaps.setdefault(number, []).append(item["编号"])
    closed = {"A1", "F2", "F3", "F5"}
    routine = {"A4", "F6"}
    def label(match):
        number, rest, pending = match.groups()
        pending = re.sub(r"^【[^】]+】\s*", "", pending)
        if number in id_gaps:
            status = "实现/规则缺口" + "/".join(id_gaps[number]) + "；另有验收"
        elif number in closed:
            status = "实施完成"
        elif number in routine:
            status = "持续规程，非功能缺口"
        elif number == "A7":
            status = "可选，非必做阻断"
        else:
            status = "已有实现，待验证"
        return f"| {number}{rest}| 【{status}】{pending}|"
    content = re.sub(r"^\| ([A-F]\d+)([^|]+\|[^|]+)\|\s*(.*?)\|$", label, content, flags=re.M)
    content = content.replace("254项目标审计223已接/31未接", "254条成就审计223已映射/31未接线")
    content = content.replace("31未接逐项真实业务（探索等）", "31条成就逐项业务或结算接线（明细见审计报告）")
    content = content.replace("3号房解锁来源", "3号房钥匙/所有权来源（系统门槛104已证实）")
    content = content.replace("头像资料和公开好友等级同事务真实库", "头像资料和公开好友等级全来源边界复核（对应同事务PG已通过）")
    content = content.replace("收藏室住客礼物/能量/房间3来源", "收藏室住客礼物/能量/房间3钥匙来源")
    save(name, content)
    note = "> 2026-10-08余项复核：A–F已确认缺口归并为15组，31条未接成就已列明；实现、验证、可选和持续规程已分开。具体见[余项复核](REPAIR-TASKS.md#2026-10-08余项复核实现缺口与验收分开)，本轮仅审计文档，不新增发布或验收。\n\n"
    for name in ["out/HANDOFF.md", "out/PROGRESS.md"]:
        content = read(name)
        content = re.sub(r"> 2026-10-08余项复核：[^\n]*\n\n", "", content)
        position = content.index("\n\n") + 2
        content = content[:position] + note + content[position:]
        save(name, content)
    name = "SERVER.md"
    content = read(name)
    content = content.replace("接续重点是异步PVP/AI权威化和双Android验收", "该段为10:56历史发布记录；异步PVP/AI权威化已在16.6完成，现行缺口以REPAIR-TASKS余项复核为准")
    server_heading = "### 16.8 余项审计与状态定义（2026-10-08）"
    server_section = server_heading + "\n\n当前A–F中已确认实现/规则缺口归并为G01–G15，详细范围、共享项去重、31条成就及验收分层已整合到[out/REPAIR-TASKS.md](out/REPAIR-TASKS.md)，源码行号与SHA在[out/remaining-functions-audit-20261008.json](out/remaining-functions-audit-20261008.json)。此为本地审计，沿用44项真实PG和12:24发布核验报告，不是新一次远端或Android验证。\n\n已实现未验收只列缺失验证层，不重新计为待开发；A4/F6规程、F2/F3/F5实施完成和A7可选项不计必做缺口。31条成就未接不能证明对应整个玩法未实现；学会等缺失业务须补业务，已有副本须补真实结算接线，禁止直接填完成状态。反推原服公式不唯一时保持单项未决，禁止以随意公式或重复让用户手动提供数据代替取证。\n"
    if server_heading in content:
        content = content[:content.index(server_heading)] + server_section
    else:
        content += "\n" + server_section
    save(name, content)
    print("已整合四份现行文档；旧文档快照：" + str(backup))
print(json.dumps({"文档数": len(docs), "缺口组数": len(groups), "缺接线成就": len(missing),
                  "类型": types, "商品门槛拒绝": unsupported, "报告": str(target)}, ensure_ascii=False))
