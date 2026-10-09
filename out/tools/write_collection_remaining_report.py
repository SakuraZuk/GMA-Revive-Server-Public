# -*- coding: utf-8 -*-
"""提供根整合现行四份MD的收藏室全分支详细资料，不新建零散MD。"""
import json,hashlib,sys
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
def sha(path):return {'路径':path,'SHA256':hashlib.sha256((root/path).read_bytes()).hexdigest()}
sources=['internal/game/collection_business.go','internal/game/collection_social.go','internal/game/collection_social_catalog.json','internal/game/collection_resident_rewards.go','internal/game/collection_remaining_energy.go','internal/game/collection_tutorial_acceleration.go','internal/game/collection_remaining_catalog.json','internal/game/collection_energy.go','internal/game/collection_energy_test.go','internal/game/profile_cosmetics.go','internal/game/profile_remaining_frames.go','internal/game/profile_limited_frames_test.go','internal/game/collection_social_test.go','internal/game/collection_remaining_test.go','internal/game/dbstore/collection_social_test.go','internal/game/dbstore/collection_remaining_test.go','out/dis/remaining-collection-npk-native.asm','out/dis/remaining-storyline-native-wrapper.asm','out/collection-graph-identity.json','out/collection-resource-scan.json','out/dis/activity-514DD3DE.asm','out/dis/activity-F0E40550.asm','out/dis/activity-0483009D.asm','out/dis/remaining-frame-priority.asm','out/dis/remaining-collection-mood-formulas.asm','out/dis/remaining-guide-acc.asm']
report={
 '状态':'G05、G07及收藏室9条成就源码与本地定向完成；实库专项已编译但本地无数据库跳过，根待远程实库与Android验收。',
 '政策':'用户已明确批准统一推荐本服规则；环境HS_REMAINING_GAMEPLAY_POLICY=local-20261008-v1启用，默认关闭。取证到的原生9000秒优先于早期6004/6005/6006固定周期提案。',
 '新增接口':[
  {'上行':'query_friend_dormitory(ObjectID eid,int hostnum)','下行':'on_query_friend_dormitory(ret,ObjectID eid,restroom.house_total_info)','事务':'实际同服双角色鉴权，黑名单双向拒绝，未开放陌生人时需双向好友；保存当前访问owner/time，禁止自访。'},
  {'上行':'like_friend_house(ObjectID eid)','下行':'on_like_friend_house(ObjectID eid,ret)','事务':'真实访问授权，UTC+8每owner每日一次、每天99次；UpdateSocial双角色原子加累计获赞/日计数与双方六成就；在线角色通知重载；所有失败不改变任一角色。'},
  {'上行':'end_visiting_house()','下行':'on_end_visiting_house(ret)','事务':'清除实际访问授权。'},
  {'上行':'player_get_house_random_reward(int window,int card)','下行':'on_player_get_house_random_reward(ret,box.box)','事务':'已冻结真实候选与箱、当前日期窗口、实际拥有且仍在原已解锁房间；资产/领取状态/计数/三成就一事务。搬出不可领，搬回可领；重复或跨窗拒绝。'},
  {'上行':'gather_produce_material_speed_up(int facility,float seconds)','下行':'原生无专用callback，仅同步设施等属性','事务':'只facility2/9000秒，6004或26004实际进行Status1且一次收据；其他设施/时间/新旧同时进行状态全部拒绝；Keep遵守Storage上限，ProduceStart不移到未来。'},
  {'上行':'update_last_get_time(int room)','下行':'能量属性同步后on_last_get_time_update(room)','事务':'真实已解锁房间，包含空房，按服务器小数时间记录last_get_time；投影与冷读取保持时刻；只有实际关闭能量生产的转换清零，旧读取/更新时间在任何重置前验证；未解锁、时钟回拨和损坏存档拒绝且整体回滚。'}],
 '持久化':{
  'Progress.HeadFrameGrantTime':'map[int]float64，框领取真实时间账本，用于future wire叠期后的回拨判断；由根添加并排除内部初始属性。',
  'CollectionState':'新增VisitOwner/VisitTime/LikeDay/LikedOwners/ReceivedLikes/DailyReceivedLikes；ResidentDay/ResidentWindows/ResidentGiftCount；TutorialAcceleration。',
  'CollectionResidentGift':'card、原room、bonus、原表raw_second、Frozen Mail资产规划容器（未插入邮箱）、claimed；Frozen包含真实材料/卡/契印资产、UUID及RewardsFrozen，不在重试重抽。',
  'CollectionTutorialReceipt':'按guide任务ID保存facility/seconds/time。',
  'CollectionRoom.Energy':'沿原生map字段start_flag,last_update_time,last_get_time,interval,base_cost,per_value,value，内部新增settled_time/resident_count/eligible；原native record schema按已知字段投影。'},
 '属性':['house_likes:Int','house_likes_map:ObjID→Bool','daily_house_likes:Int日期yyyyMMdd→Int计数','house_daily_random_reward:windowID→Int2TupleDict(cardID→Tuple[状态1可领/0已领])'],
 '原生证据':{
  '头像框':'80040材料type9→1041，limit_days1实际小时；六PVP材料明确领取时起算。head_cmp只是UI列表rank排序，没有自动选择证据。所有合法框材料type9统一领取；expiry=max(now,oldexpiry)+count*单份小时，wire=expiry-单份小时；永久0保留；当前合法选择保持，过期默认3；到期超过9999年拒绝以保证原客户端时间格式可表示。',
  '住客':'原表79卡，两UTC+8窗口06:00–13:59:59与16:00–23:59:59，各random_num1。mood=min(mood_max,max(0,comfort*mood_transform_rate))，threshold最高达到索引+2，按最高匹配mood_status选奖励。原表第二整数未找到执行语义，内部保留但不当份数，本服只grantBonus1。每窗口首次进入从当时不同card候选等概率抽取；真实随机箱同时冻结。首次空候选也记规划，反复进入不重抽；不追造历史窗口。',
  '能量':'原生facility.iter_effect取当前级upgrade_effect，首个efficiency_improve为cost。max=30*住客人数，base=cost/unit，per=cost*(1+moodProfit+tagProfit)/unit。已完整核对本版设施2/3/4全部等级，upgrade_effect没有efficiency_improve，故本版base/per确实0，不能捏造消耗；机制支持旧存档per先结算。生产设施已解锁且入住自动启动；耗尽关，读取/换人不补满，空房→再入住重启；30秒更新保留时间余数；不存在能量转免费产物。0483009D的get在start_flag为false时仍保存now，shut_down_energy_generate才清零，故空房读取持久化/属性投影/冷重建均保留last_get_time，关闭转换另行清零。',
  '房间3':'system_unlock house_dormitory3两个版本均104；materials仅809→1/810→2，无3钥匙。按批准本服政策实际取得该系统后自动建立一次房3ownership，设施4仍需803及原成本。',
  '教程原件':'ARM decoder还原两份res全部23032索引与压缩块，0错误；9000节点唯一（两版本资源相同SHA6def625392a5e8bdc922101b73aff8cfa84a60c4603f3f22cf94e3cceeaf0c82）。wrapper/storyline.py构造path/%s.ets，libclient.so Murmur3 seed0x9747b28c。反斜杠filename精确核对6004→storyline\\guide\\guide_trophyroom_facility_production1.ets(7F4D014C)、26004→storyline\\guide_new\\guide_trophyroom_facility_production1.ets(7A48A0CC)。6005/6006没有GuideFacilityAcc，不创造支路。',
  '9条成就':'六点赞210504–509，event1009 param2给予/1收到按实际成功+1；三住客210501/502/503，event1006 param1/30/100在实际累计达到阈值仅触发一次，不能按bonus第二数假设事件档位。'},
 '验证':[
  {'日志':'out/remaining-collection-social-local.log','结果':'PASS','范围':'真实Service.Handle访问授权→双角色点赞→六成就，99日限/UTC+8日界/冷JSON/双角色溢出回滚/隐私拒绝。'},
  {'日志':'out/remaining-collection-existing-local.log','结果':'PASS','范围':'既有收藏室与限时框定向兼容。'},
  {'日志':'out/remaining-collection-policy-local.log','结果':'PASS，3条','范围':'80040多份续期/永久与选择/回拨；房3、能量原式、教学9000一次、任意加速拒绝、真实冻结箱、冷JSON不重抽、两窗口/日界、1/30/100成就；旧per先扣/耗尽不读取补满/空房再入住。'},
  {'日志':'out/remaining-collection-energy-formal-local.log','结果':'正式政策环境PASS，7条','范围':'真实Service.Handle空房两次读取、最终原生回调、精确小数服务器时间、存储/下行一致、不补能不增材料、实存QuickLogin与JSON冷重建、未解锁/回拨整体回滚、非法旧last_get/update初始化前拒绝；两个限时框旧行为测试显式关闭政策，其余正式叠期及能量生命周期保持。'},
  {'日志':'out/remaining-collection-pg-local.log','结果':'仅编译成功，因HS_TEST_DATABASE_URL为空SKIP','测试':['TestPostgresCollectionVisitLikesPairRollbackAndColdRetry','TestPostgresRemainingCollectionFramesFrozenResidentsEnergyTutorialAndRollback'],'范围':'真实PG双角色授权与点赞原子回滚、冷重建跨日；原生unlock/入住/教学生产，原房搬出搬回、冻结箱溢出全回滚、冷重建幂等、1/30/100成就、真实邮件签发领取80040续期/回拨全回滚/过期回退。'}],
 '尚须验收':'根执行远程独立schema PostgreSQL专项/全量并统一构建部署；Android由用户实际操作收藏室点击礼物、能量、双角色赞与教学图，不能把本地PASS、PG或发布成功当Android验收。',
 '备份目录':'out/backups/remaining-20261008-130151-collection',
 '来源与实现SHA256':[sha(p) for p in sources]
}
(root/'out/remaining-collection-report.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('收藏室与头像框详细整合资料已生成')
