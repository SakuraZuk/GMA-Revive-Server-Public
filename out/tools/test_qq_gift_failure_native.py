# -*- coding: utf-8 -*-
"""执行原版特殊送礼选择与消费闭包，验证失败解锁和迟到回调隔离。"""
from __future__ import print_function
import __builtin__, marshal, os, sys, types
root = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
class Obj(object):
    def __init__(self, **kw): self.__dict__.update(kw)
def find(code, name):
    if code.co_name == name: return code
    for child in code.co_consts:
        if isinstance(child, types.CodeType):
            result = find(child, name)
            if result is not None: return result
def native(path, name, env):
    with open(os.path.join(root, 'internal/nativepvp/runtime/native_engine/script', path), 'rb') as stream:
        stream.read(8); code = find(marshal.load(stream), name)
    scope = {'__builtins__':__builtin__, 'True':True, 'False':False}; scope.update(env)
    return types.FunctionType(code, scope)
requests, errors, successes, returns, bonuses = [], [], [], [], []
active = [None]
gift_page = [Obj(set_back_bonus=lambda box:bonuses.append(box))]
gui = types.ModuleType('gui')
gui.get_ui = lambda name: active[0] if name == 'girl_special_gift' else gift_page[0] if name == 'girl_gift' else None
gui.is_visible = lambda name: False
gui.girl_gift = Obj(show_with_special_back=lambda:returns.append(True))
sys.modules['gui'] = gui
data = types.ModuleType('data')
data.special_gift_rule = {1:Obj(single_card_times=3,total_times=10)}
data.gift = {7:Obj(forbid_special_gift=0)}
sys.modules['data'] = data
ret_code = [0]
player = Obj(check_consume_intimacy_gift=lambda *a:ret_code[0], check_ret_success=lambda ret:ret==0,
             show_error_tips=lambda ret:errors.append(ret),
             material_mgr={9:Obj(target_id=7)},card_common_mgr={4410:Obj(intimacy=0,get_max_intimacy=lambda:100)},
             special_gift_card_2_count={},call_server=lambda *a:requests.append(a))
world = types.ModuleType('gworld'); world.get_player=lambda:player; sys.modules['gworld']=world
consume = native('entities/components/material_mgr.pyc','consume_intimacy_gift',{'data':data,'gui':gui})
choose = native('guis/prepare/girl_special_gift.pyc','on_choose_answer',
                {'gworld':world,'answer_level_ss':3,'answer_level_s':2})
after = native('guis/prepare/girl_special_gift.pyc','after_story',{'gui':gui})
def window():
    widget = Obj(show=lambda:None,hide=lambda:None,restart=lambda:None)
    value = Obj(in_anim=False,card_id=4410,gift_id=9,fx_special_1=widget,fx_special_2=widget,
                liked_particle=widget,normal_particle=widget,run_action_by_name=lambda *a:None,
                show_girl_answer=lambda *a:None,check_common_gift_full=lambda:True,
                answer_success=lambda *a:successes.append(a),hide=lambda:None)
    value.after_story=lambda:after(value)
    return value
player.consume_intimacy_gift=lambda *a,**kw:consume(player,callback=kw.get('callback'),*a)
value=window(); active[0]=value
choose(value,3,'原表选项反应')
assert value.in_anim and requests[-1][2:] == (4410,9,1,3)
requests[-1][1](99,None)
assert value.in_anim and not successes and not returns, '原版失败卡动画未复现'
scope={'__builtins__':__builtin__}
with open(os.path.join(root,'internal/game/ui_callback_repair_script.py'),'rb') as stream:
    script=stream.read()
exec(compile(script[:script.index('def _install_hs_ui_callback_repair')],'<实际送礼修补>','exec'),scope)
player.consume_intimacy_gift=lambda *a,**kw:scope['_hs_consume_intimacy_gift'](player,callback=kw.get('callback'),*a)
value=window();active[0]=value;choose(value,3,'原表选项反应');requests[-1][1](99,None)
assert not value.in_anim and len(returns)==1 and not successes and not bonuses
ret_code[0]=99;value=window();active[0]=value;before=len(requests);choose(value,3,'反应')
assert not value.in_anim and len(requests)==before
ret_code[0]=0;value=window();active[0]=value;choose(value,3,'反应');old=requests[-1][1]
value.in_anim=False;choose(value,3,'第二次');old(99,None)
assert value.in_anim, '迟到失败解锁了新请求'
requests[-1][1](0,'真实奖励')
assert successes[-1]==('第二次',) and bonuses==['真实奖励']
# 好感度跨级演出可能先销毁普通赠礼页；成功回调仍必须继续释放特殊演出链。
value=window();active[0]=value;choose(value,3,'跨级成功');gift_page[0]=None
requests[-1][1](0,'跨级奖励')
assert successes[-1]==('跨级成功',) and bonuses==['真实奖励']
gift_page[0]=Obj(set_back_bonus=lambda box:bonuses.append(box))
value=window();active[0]=value;choose(value,3,'反应');old=requests[-1][1];active[0]=window();old(99,None)
assert value.in_anim, '迟到回调触及已切换界面'
player.special_gift_card_2_count[4410]=3
value=window();active[0]=value;choose(value,3,'次数降级');assert requests[-1][2:]==(4410,9,1,0)
print('原版choice3失败锁死对照、预检失败、服务端失败返回赠礼页、成功时赠礼页销毁不中断、真实成功奖励、迟到代次/界面隔离及次数降级通过；图形为夹具。')
