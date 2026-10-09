"""验证运营热更与145条服务端活动配置一致，保留节点及星期限制。"""
import json, sys, types
from pathlib import Path
from types import SimpleNamespace as N
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')

root=Path(__file__).resolve().parents[2]
catalog=json.loads((root/'internal/game/activity_catalog.json').read_text(encoding='utf-8'))['表']
rows=json.loads((root/'deploy/data/activity-schedules.json').read_text(encoding='utf-8'))
clock=[rows[0]['begin_time']]
class NativeDict(dict):
    def itervalues(self):return iter(self.values())
drill_ids=(2103,2106,2112,2203,2206,2212,2303,2306,2312,2503,2506,2512)
data=N(activity_type={int(k):N(**v) for k,v in catalog['activity_type'].items()},
       activity_banner={},activity_board_info={},shop={},commodity={},banner_activity_type={'banner_dict':{}})
data.timber_pile_dungeons=NativeDict({i:N(dungeon_id=i,disable_activity=1) for i in drill_ids})
data.dungeons={i:N(_dungeon_disable=True) for i in drill_ids}
data.dungeons[10001]=N(_dungeon_disable=True)
shop_catalog=json.loads((root/'internal/game/shop_catalog.json').read_text(encoding='utf-8'))
data.commodity={int(k):N(**v) for k,v in shop_catalog['commodities'].items()}
original_goods={k:dict(vars(v)) for k,v in data.commodity.items()}
data.activity_type[203].open_time=[1]
data.activity_banner[203]=N(begin_time=[1612368000,0],end_time=[1614182400,0],open_time=[1],banner_path='活动',lock_system_name='')
data.banner_activity_type['banner_dict'][203]=203
timeutils=N(now_time=lambda:clock[0])
errors=N(RET_SUCCESS=0,RET_LIMIT_ACTIVITY_NO_OPEN=101,RET_ACTIVITY_HAS_ENDED=102,RET_SHOP_NO_OPEN=103)
au=types.ModuleType('activity_utils');au.data=data;au.time_utils=timeutils;au.error_code=errors
au.get_activity_time=lambda *a:('原生','时间');au.check_activity_open=lambda *a:999;au.get_activity_day_num=lambda *a:999
dc=types.ModuleType('dungeon_check');dc.check_dungeon_type_opened=lambda *a:False;dc.check_banner_type_opened=lambda *a:False
weekly=[True];dc.check_dungeon_open_time=lambda *a:weekly[0];dc.check_activity_open_time=lambda *a:weekly[0]
dc.check_system_unlock=lambda *a:False
gu=types.ModuleType('gui_utils');gu.check_shop_time_valid=lambda *a:True;gu.gworld=N(get_player=lambda:N(show_error_tips=lambda *a:None))
common=types.ModuleType('common_logic');common.activity_utils=au;common.dungeon_check=dc
guis=types.ModuleType('guis');guis.gui_utils=gu
sys.modules['common_logic']=common;sys.modules['guis']=guis
mid_age_module=types.ModuleType('entities.components.mid_age_game_mgr')
class MidAge:
    def get_mid_age_activity_id(self):return 999
mid_age_module.mid_age_game_mgr=MidAge
sys.modules['entities.components.mid_age_game_mgr']=mid_age_module
class Avatar:
    get_mid_age_activity_id=MidAge.get_mid_age_activity_id
sys.modules['entities.Avatar']=N(Avatar=Avatar)
sys.modules['common_const']=N(ACTIVITY_TYPE_MID_AGE=210,ACTIVITY_TYPE_MID_AGE_TWO=212)
sys.modules['gworld']=N(get_player=lambda:N(hostnum=1))
monster_module=types.ModuleType('entities.components.monster_nian_mgr')
components_module=types.ModuleType('entities.components');components_module.monster_nian_mgr=monster_module
entities_module=types.ModuleType('entities');entities_module.components=components_module
sys.modules['entities']=entities_module;sys.modules['entities.components']=components_module
sys.modules['entities.components.monster_nian_mgr']=monster_module
chapter_module=types.ModuleType('guis.prepare.main_line_chapter')
class ActivityItem:
    def set_time_panel(self):return '原生倒计时'
chapter_module.activity_item=ActivityItem
beginner_module=types.ModuleType('guis.prepare.beginner_guide')
class BeginnerGuide:
    def update_activity_count_down(self):return '原生新手倒计时'
beginner_module.beginner_guide=BeginnerGuide
beginner_module.activity_mission_id=210
prepare_module=types.ModuleType('guis.prepare');prepare_module.beginner_guide=beginner_module
sys.modules['guis.prepare']=prepare_module
sys.modules['guis.prepare.main_line_chapter']=chapter_module
sys.modules['guis.prepare.beginner_guide']=beginner_module
data.chapter={501:N(chapter_type=210),999:N(chapter_type=999)}
code=(root/'internal/game/activity_schedule_script.py').read_text(encoding='utf-8').replace('__HS_ACTIVITY_SCHEDULES_JSON__',repr(json.dumps(rows))).replace('__HS_SPECIAL_DRILL_REOPEN__','True')
exec(compile(code,'activity_schedule_script.py','exec'),{})
assert len(au._hs_activity_schedules)==145
assert monster_module.gworld is sys.modules['gworld']
assert dc.get_banner_next_refresh_time()==2147483647
data.activity_banner[999]=N(begin_time=[clock[0]+100,0],end_time=None,open_time=None,banner_path='将来活动',lock_system_name='')
assert dc.get_banner_next_refresh_time()==100
data.activity_banner[999].lock_system_name='未解锁系统'
assert dc.get_banner_next_refresh_time()==2147483647
data.activity_banner.pop(999)
saved_banner_end=data.activity_banner[203].end_time
data.activity_banner[203].end_time=(clock[0]+50,0)
saved_permanent=au._hs_activity_schedules[203]['permanent']
saved_end=au._hs_activity_schedules[203]['end_time']
au._hs_activity_schedules[203]['permanent']=False
au._hs_activity_schedules[203]['end_time']=clock[0]+50
assert dc.get_banner_next_refresh_time()==50
au._hs_activity_schedules[203]['permanent']=saved_permanent
au._hs_activity_schedules[203]['end_time']=saved_end
data.activity_banner[203].end_time=saved_banner_end
assert au.get_activity_time(210,1)[1]==(2145801600,86399)
assert all(sum(au.get_activity_time(row['activity_id'],1)[1])==2145887999 for row in rows if row['permanent'])
assert Avatar().get_mid_age_activity_id()==210
assert MidAge().get_mid_age_activity_id()==999
assert all(not data.dungeons[i]._dungeon_disable and not data.timber_pile_dungeons[i].disable_activity for i in drill_ids)
assert data.dungeons[10001]._dungeon_disable is True
window=data.activity_banner[203].begin_time
exec(compile(code,'activity_schedule_script.py','exec'),{})
assert data.activity_banner[203].begin_time==window
labels=[];cancelled=[]
item=ActivityItem();item.chapter_id=501;item.text_limit_time=N(set_string=labels.append)
assert item.set_time_panel() is None and labels[-1]=='常驻开放'
item.chapter_id=999;assert item.set_time_panel()=='原生倒计时'
guide=BeginnerGuide();guide.title_time=N(set_string=labels.append);guide.count_down_handle=42;guide.cancel_callback=cancelled.append
assert guide.update_activity_count_down() is True and cancelled==[42] and guide.count_down_handle is None
assert guide.update_activity_count_down() is True and cancelled==[42]
beginner_module.activity_mission_id=999
assert guide.update_activity_count_down()=='原生新手倒计时'
beginner_module.activity_mission_id=210
assert Avatar().get_mid_age_activity_id()==210
# 有限日期仍按原生最近端点选取；相同时保持210优先。
saved_210=au._hs_activity_schedules[210]
saved_212=au._hs_activity_schedules[212]
au._hs_activity_schedules[212]=dict(saved_212,begin_time=clock[0]-5)
assert Avatar().get_mid_age_activity_id()==210
au._hs_activity_schedules[210]=dict(saved_210,begin_time=clock[0]-100)
assert Avatar().get_mid_age_activity_id()==212
au._hs_activity_schedules[210]=dict(saved_210,permanent=False,end_time=clock[0]+1)
au._hs_activity_schedules[212]=dict(saved_212,permanent=False,end_time=clock[0]+2)
assert Avatar().get_mid_age_activity_id()==999
au._hs_activity_schedules[210]=saved_210
au._hs_activity_schedules[212]=saved_212
permanent_ids={1071011,1071012,1071013,1071014,2010010}
for key,goods in data.commodity.items():
    before=original_goods[key];after=dict(vars(goods))
    # 原有145活动日程会重排这些绝对窗口；本用例核对新增商品政策不改其他字段。
    before=dict(before)
    for window_key in ('begin_time','end_time'):
        before.pop(window_key,None)
        after.pop(window_key,None)
    if key in permanent_ids:
        assert after['open_server_days'] is None and after['count_down_flag']==0
        after['open_server_days']=before['open_server_days']
        after['count_down_flag']=before['count_down_flag']
    assert after==before,('商品的价格、解锁、限购或未知商品被修改',key)
assert au.get_activity_day_num(203,0)==1
assert dc.check_dungeon_type_opened(203,0,open_days=[1])
weekly[0]=False
assert not dc.check_dungeon_type_opened(203,0,open_days=[1])
assert not dc.check_banner_type_opened(203)
weekly[0]=True
clock[0]+=1000*86400
assert au.get_activity_day_num(203,0)==21
assert au.get_activity_day_num(206,0)==21
assert au.get_activity_day_num(328,0)==14
assert dc.check_dungeon_type_opened(203,0,open_days=list(range(1,22)))
assert not dc.check_dungeon_type_opened(203,0,open_days=[1])
assert au.check_activity_open(203,0)==0
saved_clock=clock[0]
clock[0]=2145887999+86400
assert au.check_activity_open(203,0)==0
clock[0]=saved_clock
assert au.check_activity_open(99999,0)==999
clock[0]=rows[0]['begin_time']-1
assert au.check_activity_open(203,0)==101
assert not dc.check_dungeon_type_opened(203,0)
assert au.get_activity_day_num(203,0)==-1
print('145项配置、时间边界、阶段封顶、星期限制及五商品永久开放/重复安装/其余字段不变验证通过。')
