# -*- coding: utf-8 -*-
"""学会六成就原生全链资料供根合并到现行MD，不新建零散说明。"""
import json,hashlib,sys
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
def sha(path):return {'路径':path,'SHA256':hashlib.sha256((root/path).read_bytes()).hexdigest()}
paths=['internal/game/league_business.go','internal/game/league_catalog.json','internal/game/league_protect.go','internal/game/league_protect_catalog.json','internal/game/league_business_test.go','internal/game/league_failures_test.go','internal/game/league_protect_test.go','internal/game/league_protect_recovery_test.go','internal/game/dbstore/league_protect_test.go','out/dis/remaining-league-full-chain.asm','out/dis/remaining-league-card-fields.asm','out/remaining-league-battle-guardians.asm','out/client_catalogs/tables/new_system_tips.json','out/client_catalogs/tables/league_protect.json','out/client_catalogs/tables/role_info.json']
data={
 '范围':'G01学会6条：403001真实入会；403002/403003实际参加1/30次；403004/5/6三难度真实胜利且击倒50。非完整学会捐赠/成长/职务/PVP系统。',
 '状态':'源码冻结，5条学会本地定向PASS，完整PG专项已落，真实PG与Android由根验收。',
 '会员主档':'OwnedLeagues主档只在实际创建者OID玩家行保存，ID为真实生成ObjectID/同服唯一UID，主档Members键为真实玩家OID；不是catalog静态假学会。UpdateAllSocial原子创建成本/成员关系/申请/双方入会成就与退会。真实成员会长移交后创建者可退出，主档继续留存，最后成员退出才标Dissolved。',
 '接口':[
 {'上行':'create_new_league(name,public_notice,cardID)','下行':'on_create_new_league(ret)','说明':'6级门槛、同服名字唯一、原rarity对应11材料成本，不能成功假创建。'},
 {'上行':'apply_league(ObjectID leagueID,nil)','下行':'on_apply_league(ret,ObjectID leagueID)','说明':'审批前仅待处理真实申请；自动审批执行真实同服成员事务。'},
 {'上行':'agree_league_apply/refuse_league_apply(ObjectID applicant)','下行':'on_原方法(ret,ObjectID applicant)','说明':'管理权限、实际申请及角色、新成员满额与4小时退会冷却，失败不改任何档。'},
 {'上行':'leave_league()','下行':'on_leave_league(ret)','说明':'真实退会与主档Member清除同事务；多个成员时会长须先移交。'},
 {'上行':'appoint_league_member(ObjectID target,1)','下行':'on_appoint_league_member(ret,ObjectID target)','说明':'只本轮所需会长移交，须现任会长及同服现成员，旧会长降普通、新会长升1和President同事务。其他完整职务系统没有扩展。'},
 {'上行':'league_setup_auto_agree(bool)','下行':'on_league_setup_auto_agree(ret,bool)','说明':'仅管理员设置真实审批策略。'},
 {'上行':'query_total_league_info(leagueID,host)/search_league(name,uid)','下行':'原无ret的真实字典/列表','说明':'真实同服学会档与玩家信息，不修改成有ret的协议。'},
 {'上行':'league_protect_start(protectID,extra_support_skill_idx)','下行':'on_league_protect_start(ret,protectID)+原start_server_battle_ok/prepare','说明':'真实会员/同服/实际守门人卡/技能解锁/开放窗口冻结，专用内部callback0移除，直接enterDungeon无冻结授权拒绝。'},
 ],
 '常规失败协议':'真实存储/畸形参数维持Go错误；合法业务失败通过原native RET字段回on_*，包括创建成本12004、满员12021、审批权限12012、冷却12072、会长不得退出12016、任命对象12019、未入会12073、雅努斯上限12044与援护未解锁12077。不成功假加、不让原UI等不到回调。',
 '持久化':{
 'Progress.LeagueProtect':'内部server_league_protect，原weekly字段TotalWeekly/CurrentTimes/Contribution/ProtectID/Kills/Unlocked/Updated，LastTime回拨账本，Pending真实授权，Consumed按UUID→冻结窗口收据，Results按UUID→完整结果盒/宝藏/卡/真实统计/贡献。内部Week字段保存最近周二/四开放日，不是周一自然周；本原表wire名字total_weekly_protect_times保留。',
 'ActivityBattleContext.LeagueProtect':'冻结LeagueID/MemberID/Host/ProtectID/DungeonID/开放日/CardID/CardLevel/援护索引/CardWire/Statistics/Result/AttemptConsumed；恢复与冷读复用，不读取后来换的卡。',
 '属性':'league_activity_weekly_info原native custom type，protect_unlock_map保留跨次开启，计次/贡献/kill_map在下次周二/四开启恢复。'},
 '原件规则':{
 '计次':'new_system_tips league_protect明确每周二/周四开放，下次玩法开启次数恢复，无论胜负消耗，布阵退出不消耗。故每个周二/四开放窗口限1，battle_fighting成功阵容冻结同事务consume。撤销早期周一每周限1的临时设计，原件优先。',
 '死亡/宝藏':'root activity_metrics_script rev2按CF38A2E4 get_statistics()[2]非召唤死亡角色，role_info.role_tag501独计宝藏不计普通50。结果league_protect_statistics必须有kill_count0..50及treasure_role_id0/501；缺统计、51、非整数或别角色拒绝，不用win伪造50。普通PvE仍Android权威，服务器校验授权与范围，不声称抗作弊。',
 '奖档':'三难度原50敌，kill_nums9–18/19–27/28–39/40–49/50，分别111011–15/111021–25/111031–35；0–8没有档。失败也按实际击倒档发奖；宝藏原111101/2/3单次bonus，不能把501混入50。',
 '学分':'原说明每击倒一个敌人获得按难度学分，原basic_contribution20/30/40×实际普通kill_count；宝藏不纳入，不能按参与固定20或假成长公式。撤销早期临时固定参与贡献设计。',
 '解锁/成就':'真实win且50才protect_unlock_map[当前难度]=true并event29[100,protectID]；下一档只检查unlock_based_id，不把未使用unlock_growth200/400猜成成长扣费。实际结算一次event21 param14驱动1/30参加，只有进布阵不推进。',
 '守门人':'原league_card_base real_role_id=-1，role[-1]技能91001/91011/91012/91013，援护列表91012/91013/91011解锁等级1/30/40。已实现的创建守门人level1只可idx0；不能用所选卡4401的普通card skills代替。原extra_info league_protect_card触发native event_mgr加载，并由update_battle_skills筛选真实援护；完整SkillMgr/UnlockMap冷JSON规范恢复整数键。',
 '恢复与跨窗口':'client_need_recover_battle更换UUID后migrate复制旧消费窗口收据为别名，不增次数；已消费旧场在新窗口恢复/结算不消耗新额度；未开始旧布阵过窗不能再消费，专用起战重建新窗口授权。已完成结果只重放冻结原结果，不重开、不多奖。结果账本不随窗口清空。'},
 '共享挂钩':'根已接Progress与ActivityContext字段/内部InitialProperties过滤/Service入口；prepareActivityDungeon专用14分流；battle_fighting consume；recover migrate；result record原统计→settle；activityBattleExtra删除请求guardian/技能/敌修正，只填context；battleSettlement.extra league_activity_battle_result保持原dungeon_mgr自动on_league_protect_end，不额外重复弹UI。leagueProtectCompletionPushes仅状态与成就属性。',
 '测试':[
 {'日志':'out/remaining-league-business-local.log','结果':'PASS，5条','内容':'实际成员创建→真实申请审批→入会成就→退会冷却；素材不足/满员/权限/任命/会长退出精确失败回包+双档不变；实际专用起战/合法guardian/布阵不扣次/启动扣次/51统计全回滚/18失败+501宝藏及360学分；实际recover改UUID→跨新窗重打结算→已完成恢复只重放；30次真实ready/started/result完成全部6成就与冷JSON卡技能wire。'},
 {'日志':'out/remaining-league-collection-pg-local.log','结果':'本地编译成功，未配置HS_TEST_DATABASE_URL显式SKIP；真实PG待根统一执行','测试':'TestPostgresLeagueActualMembersJanusNativeStatsThirtyReceiptsColdRetryAndTransfer','内容':'真实PostgreSQL双方注册/创建/审批/同服会员；正确业务失败ret；专用start/真实battle_fighting/原生ready/started/result/51全回滚/失败18+501真实奖档与学分；真正recover后再fighting不扣次，新Store新Service冷重登同UUID改口不得重奖；30次真实跨开放窗口战斗驱动6成就，31计次键包含一次恢复别名；原生会长移交后退会，创建者主档仍保存新会长。'}],
 '验收边界':'本地fixture通过不等于Android实机；真实Android点击会员/申请/雅努斯守门人加载与援护、实际波次统计与结果原UI待用户操作。root统一远程独立schema PG、构建发布与本服策略环境，不由子代理部署。',
 '备份':'out/backups/remaining-20261008-130151-collection/league_business.go.bak 与league_business_test.go.bak',
 '来源与实现SHA256':[sha(p) for p in paths]
}
(root/'out/remaining-league-report.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('学会完整原生全链整合资料已生成')
