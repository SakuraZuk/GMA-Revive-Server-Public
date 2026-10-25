# -*- coding: utf-8 -*-
"""执行原版尖叫屋及委托完成演出，保留错误回包的失败对照；图形对象为夹具。"""
from __future__ import print_function
import marshal
import os
import types
import __builtin__

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))


class Obj(object):
    def __init__(self, **values):
        self.__dict__.update(values)


def native(module, name, scope):
    path = os.path.join(ROOT, 'internal/nativepvp/runtime/native_engine/script', module)
    with open(path, 'rb') as source:
        source.read(8)
        code = marshal.load(source)

    def find(parent):
        if parent.co_name == name:
            return parent
        for child in parent.co_consts:
            if isinstance(child, types.CodeType):
                found = find(child)
                if found:
                    return found
    globals_scope = {'__builtins__': __builtin__, 'True': True, 'False': False}
    globals_scope.update(scope)
    return types.FunctionType(find(code), globals_scope)


ghost_callback = native('guis/dream_land/dream_room_ghost.pyc', 'on_choice_ghost', {})
materials = [18, 22, 1502]
for index in range(3):
    def fixture():
        revealed, rewards, callbacks = [], [], []
        ghosts = []
        for pos in range(3):
            ghosts.append(Obj(pos_index=pos,
                              set_info=lambda mid, pos=pos: revealed.append((pos, mid)),
                              show_double=lambda n: None,
                              ghost=Obj(run_action_by_name=lambda *a, **k: None),
                              get_pos_x=lambda: 0))
        ui = Obj(ghosts=ghosts, ui_path='test',
                 vx_light=Obj(set_pos_x=lambda x: None, show=lambda: None,
                              run_action_by_name=lambda *a, **k: None),
                 add_callback=lambda delay, fn: callbacks.append(fn),
                 show_get_reward=lambda mid, count: rewards.append((mid, count)))
        return ui, revealed, rewards, callbacks

    ui, _, _, _ = fixture()
    try:
        ghost_callback(ui, True, index, [materials[index]], 2)
    except IndexError:
        pass
    else:
        raise AssertionError('旧单材料回包未复现原版IndexError')
    ui, revealed, rewards, callbacks = fixture()
    ghost_callback(ui, True, index, materials, 2)
    for callback in callbacks:
        callback()
    assert revealed == list(enumerate(materials))
    assert rewards == [(materials[index], 2)]

uuid = '00112233445566778899aabb'
player = Obj(card_mgr={uuid: Obj(card_id=2202, get_dress_id=lambda: 220201)},
             card_common_mgr={2202: Obj(get_dress_id=lambda: 220201)})
scope = {'gworld': Obj(get_player=lambda: player),
         'data': Obj(consign_bubble={}),
         'system_hide_check': Obj(check_system_hide=lambda key: False),
         'common_const': Obj(SYSTEM_UNLOCK_CONSIGN_BUBBLE=1)}
bubble = native('guis/activity/consign_task_detail_new.pyc', 'get_vo_bubble_by_card_id', scope)
commit = native('guis/activity/consign_task_detail_new.pyc', 'commit_task_succ', scope)
ui = Obj(get_vo_bubble_by_card_id=lambda key: bubble(ui, key))
try:
    commit(ui, [2202])
except KeyError:
    pass
else:
    raise AssertionError('旧模板编号未复现原版KeyError')
commit(ui, [uuid])
commit(ui, [])
main_bubble = native('guis/activity/main_consign_new.pyc', 'get_vo_bubble_by_card_id', scope)
main_play = native('guis/activity/main_consign_new.pyc', 'play_vo', scope)
main_commit = native('guis/activity/main_consign_new.pyc', 'commit_task_succ', scope)
main_ui = Obj(get_vo_bubble_by_card_id=lambda key: main_bubble(main_ui, key),
              play_vo=lambda keys: main_play(main_ui, keys))
try:
    main_commit(main_ui, [uuid])
except KeyError:
    pass
else:
    raise AssertionError('原版主页面UUID查公共卡牌未复现KeyError')
with open(os.path.join(ROOT, 'internal/game/ui_callback_repair_script.py'), 'rb') as source:
    repair = source.read()
helper = repair[repair.index('def _hs_wrap_consign_common_lookup'):repair.index('def _install_hs_ui_callback_repair')]
repair_scope = {}
exec(compile(helper, '<委托主页面修补>', 'exec'), repair_scope)
import sys
world = types.ModuleType('gworld')
world.get_player = lambda: player
sys.modules['gworld'] = world
patched = repair_scope['_hs_wrap_consign_common_lookup'](main_bubble)
main_ui.get_vo_bubble_by_card_id = lambda key: patched(main_ui, key)
main_commit(main_ui, [uuid])
main_commit(main_ui, [2202])
main_commit(main_ui, [])
main_commit(main_ui, ['removed-card'])
print('委托主页面原版UUID异常已复现；修补后UUID、旧模板编号、空列表及迟到已移除幻书均完成；详情页仍按UUID执行原版回调。')
print('原版回归通过：尖叫屋三个位置均完整揭晓且仅选择项领奖；委托UUID及已归还空列表完成演出；旧回包均复现异常。图形对象为夹具，非Android画面验收。')

# 执行日志中的原版任务领奖闭包；旧行销毁后回调仍须刷新当前界面。
def find_path(module, names):
    with open(os.path.join(ROOT, 'internal/nativepvp/runtime/native_engine/script', module), 'rb') as source:
        source.read(8)
        code = marshal.load(source)
    for name in names:
        code = next(child for child in code.co_consts if isinstance(child, types.CodeType) and child.co_name == name)
    return code

def cell(value):
    return (lambda: value).func_closure[0]

class TypedCommon(dict):
    def get(self, key, default=None):
        return dict.get(self, int(key), default)
    def __getitem__(self, key):
        return dict.__getitem__(self, int(key))

player.card_common_mgr = TypedCommon(player.card_common_mgr)
main_commit(main_ui, [uuid])
spines, shown = [], []
player.get_card_common_info = lambda key: Obj(get_spine=lambda: 220201, get_dress_id=lambda: 220201)
spine = Obj(play_animation=lambda *a: None, set_scale=lambda *a: None, set_pos=lambda *a: None)
scope.update(gui_utils=Obj(create_spine=lambda key: spines.append(key) or spine),
             cc=Obj(Vec2=lambda *a: a, Size=lambda *a: a),
             language_utils=Obj(get_language_csb=lambda name: name))
finish_spine = native('guis/activity/consign_finish_new.pyc', 'create_spine_entity', scope)
finish_show = native('guis/activity/consign_finish_new.pyc', 'show_consign_finish_tips', scope)
noop = lambda *a, **k: None
finish_ui = Obj(spine_socket=Obj(remove_all_children=noop, add_child=noop, set_flipped_x=noop, show=noop),
                reward_item=Obj(get_content_size=lambda: Obj(width=10, height=10)),
                reward_panel=Obj(set_content_size=noop), reward_list=Obj(set_infos=noop, set_pos_x=noop),
                panel_action=Obj(run_action_by_name=noop), panel_rewards=Obj(run_action_by_name=noop), show=noop)
finish_ui.create_spine_entity = lambda key: finish_spine(finish_ui, key)
finish_ui.item_dialog = Obj(hide=noop)
scope['data'].consign_bubble[220201] = Obj(consign_finish_bubble_id=[1], consign_begin_bubble_id=[1])
scope['data'].character_vo_text = {1: Obj(text='完成', sound_id=None)}
scope['random'] = __import__('random')
scope['gworld'].sound_mgr = Obj(play_sound=noop)
finish_ui.set_dialog = noop
# 上述原版函数各自已复制globals，需要同步其真实依赖。
for function in (bubble, main_bubble, finish_spine):
    function.func_globals.update(scope)
bonus = Obj(to_list=lambda: [[1, 112, 2]])
try:
    finish_show(finish_ui, (123, [uuid], bonus))
except (TypeError, ValueError):
    pass
else:
    raise AssertionError('原版完成弹窗的模板编号转换异常未复现')
gui_complete = Obj(is_visible=lambda name: True,
                   consign_task_detail_new=Obj(commit_task_succ=lambda keys: commit(ui, keys), hide=noop),
                   main_consign_new=Obj(commit_task_succ=lambda keys: main_commit(main_ui, keys), update=noop),
                   main_chapter=Obj(destroy_consign_model=noop),
                   get_ui=lambda name: Obj(is_visible=lambda: True, update_dormitory_red_point=noop),
                   general_tips_mgr=Obj(show_tips=lambda *args: shown.append(args)))
sys.modules['gui'] = gui_complete
patched_finish = repair_scope['_hs_wrap_consign_finish'](finish_show)
gui_complete.general_tips_mgr.show_tips = lambda kind,param: patched_finish(finish_ui,param) if kind=='consign_finish' else shown.append((kind,param))
complete_rpc = native('entities/components/consign_task.pyc', 'commit_consign_task_success', {'gui':gui_complete})
complete_rpc(Obj(), 123, [uuid], bonus)
assert spines == [220201] and finish_ui.card_ids == [2202]
patched_finish(finish_ui, (123, [2202], bonus))
assert spines == [220201, 220201]
complete_rpc(Obj(), 123, [], bonus)
assert shown == [('item_get', ('reward', [[1, 112, 2]]))]
print('委托完整原版RPC经详情页、主页面、地图、收藏室到完成弹窗通过；原生类型转换失败对照及空演出奖励通过。')

# 原生提示队列会在real_show_tips中首次获取完成窗；此时完成窗类尚未修补。
# 使用原版show_tips/real_show_tips验证入队转换覆盖首次加载，不以直接调用包装代替。
tips_scope = {'gui': Obj(consign_finish_new=Obj(show_consign_finish_tips=lambda param: finish_show(finish_ui,param))),
              'time_utils': Obj(now_time=lambda: 100), 'max_tips_time': 10}
native_show = native('guis/common/general_tips_mgr.pyc','show_tips',tips_scope)
native_real_show = native('guis/common/general_tips_mgr.pyc','real_show_tips',tips_scope)
tips_mgr = Obj(on_work=False,last_tips_time=0,tips_modules=['consign_finish'],tips_info={'consign_finish':[]})
tips_mgr.real_show_tips = lambda: native_real_show(tips_mgr)
safe_show = repair_scope['_hs_wrap_consign_tips'](native_show)
safe_show(tips_mgr,'consign_finish',(123,[uuid],bonus),None,True,False)
assert finish_ui.card_ids == [2202] and tips_mgr.tips_info['consign_finish'] == []
print('原版提示队列在首次加载未包装完成窗前完成UUID转换，奖励与原生演出保留。')

helper = repair[repair.index('def _hs_wrap_live_task_update'):repair.index('def _install_hs_ui_callback_repair')]
exec(compile(helper, '<入馆指南旧行保护>', 'exec'), repair_scope)
for kind in ('task_item', 'activity_task_item'):
    refreshed, hidden = [], []
    scope = {'__builtins__': __builtin__, 'True': True, 'False': False,
             'gui': Obj(beginner_guide=Obj(update_task_info=lambda: refreshed.append(True),
                                          update_activity_info=lambda: refreshed.append(True)),
                        general_tips_mgr=Obj(show_tips=lambda *args: None))}
    update = types.FunctionType(find_path('guis/prepare/beginner_guide.pyc', [kind, 'update']), scope)
    row = Obj(_ins=None, get_info=lambda: None,
              hide=lambda: row._ins.setVisible(False))
    row.update = lambda: update(row)
    callback = types.FunctionType(find_path('guis/prepare/beginner_guide.pyc', [kind, 'on_press_get', 'get_callback']), scope, closure=(cell(row),))
    callback_args = () if kind == 'task_item' else ('任务', Obj(to_list=lambda: []))
    try:
        callback(*callback_args)
    except AttributeError:
        pass
    else:
        raise AssertionError('旧行销毁后的原生异常未复现')
    assert not refreshed
    patched_update = repair_scope['_hs_wrap_live_task_update'](update)
    row.update = lambda: patched_update(row)
    callback(*callback_args)
    assert refreshed == [True]
    # 活行仍沿用原生隐藏分支，不吞掉其异常或改变任务状态。
    row._ins = Obj(setVisible=lambda value: hidden.append(value))
    callback(*callback_args)
    assert hidden == [False] and refreshed == [True, True]
print('入馆指南普通/活动任务原版领奖闭包均复现销毁异常；修补后旧行跳过并刷新当前列表，活行保留原生行为。')

helper = repair[repair.index('def _hs_receive_power_supply'):repair.index('def _install_hs_ui_callback_repair')]
exec(compile(helper, '<补给回调保护>', 'exec'), repair_scope)
rewards, requests, updates = [], [], []
data_module = types.ModuleType('data'); data_module.bonus_buff = {8: Obj()}
sys.modules['data'] = data_module
buff_utils = Obj(get_empty_buffs=lambda: Obj(add_buff=lambda *a: None, get_bonus_ids=lambda: [100]))
common = types.ModuleType('common_logic'); common.bonus_buff_utils = buff_utils
sys.modules['common_logic'] = common
power_gui = Obj(get_ui=lambda name: Obj(hall_buttons=Obj(), update_list=lambda: updates.append(name)),
                general_tips_mgr=Obj(show_tips=lambda *args: rewards.append(args)))
sys.modules['gui'] = power_gui
native_power = native('entities/components/avatar.pyc', 'receive_power_supply',
                      {'data':data_module, 'bonus_buff_utils':buff_utils, 'gui':power_gui})
owner = Obj(call_server=lambda *args: requests.append(args))
native_power(owner, 8, None)
try:
    requests[-1][1](bonus, '')
except TypeError:
    pass
else:
    raise AssertionError('原版补给回调未复现占位对象索引异常')
assert not rewards
repair_scope['_hs_receive_power_supply'](owner, 8)
assert requests[-1][0] == 'receive_power_supply' and requests[-1][2:] == (8,)
requests[-1][1](bonus, '')
assert rewards == [('item_get', ('reward', bonus.to_list()))]
power_gui.get_ui = lambda name: Obj(hall_buttons={'xianshi':Obj(update_red_point=lambda:updates.append('红点'))}, update_list=lambda: None)
requests[-1][1](bonus, '')
assert updates[-1] == '红点'
before = (len(rewards), len(requests))
requests[-1][1](None, '领取被拒绝')
repair_scope['_hs_receive_power_supply'](owner, 999)
assert before == (len(rewards), len(requests))
print('原版补给预检和回调占位对象异常已复现；修补后真实奖励展示、有效大厅红点、失败不展示及未知补给不请求通过。')

# 原版请求闭包早于按需模块扫描捕获方法；必须在请求入口保护旧绑定回调。
friend_module=types.ModuleType('guis.social.friend_recommend');friend_module.recommend_tab=1
sys.modules['guis.social.friend_recommend']=friend_module
friend_callback=native('guis/social/friend_recommend.pyc','on_get_search_friend',{'recommend_tab':1})
friend_request=native('entities/components/friend_mgr.pyc','get_recommend_friends',{})
friend_owner=Obj(friend_items=[[],[]],on_refresh_list=lambda tab: updates.append(tab))
callback=types.MethodType(friend_callback,friend_owner)
player=Obj(recomment_players={1:'已缓存推荐'},call_server=lambda *a:requests.append(a))
friend_owner.friend_items=[]
try:friend_request(player,False,callback)
except IndexError:pass
else:raise AssertionError('原生缓存推荐旧绑定回调越界未复现')
exec(compile(repair[:repair.index('def _install_hs_ui_callback_repair')],'<请求入口回调保护>','exec'),repair_scope)
guarded=repair_scope['_hs_wrap_recommend_request'](friend_request)
guarded(player,False,callback)
friend_owner.friend_items=[[],[]]
guarded(player,False,callback)
assert friend_owner.friend_items[1]==['已缓存推荐']
guarded(player,True,callback)
stored=requests[-1][1]
# 执行原生server_callback需真实recomment_players自定义加载；提取其已冻结callback。
captured=dict(zip(stored.func_code.co_freevars,[cell.cell_contents for cell in stored.func_closure]))['callback']
friend_owner.friend_items=[[],[]]
captured(['迟到推荐'])
assert friend_owner.friend_items[1]==[], '旧请求更新了新列表'
print('原版缓存推荐越界对照、有效推荐刷新、原生异步闭包捕获及旧列表隔离通过；不依赖GUI按需轮询先安装。')
