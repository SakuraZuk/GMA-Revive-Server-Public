# -*- coding: utf-8 -*-
"""使用Python2原生声音函数验证默认中配与单卡覆盖；音频播放设备为验证边界。"""
from __future__ import print_function
import marshal
import os
import sys
import types
import __builtin__

root = os.path.abspath(sys.argv[1])
path = os.path.join(root, 'internal/nativepvp/runtime/native_engine/script/client/sound_mgr.pyc')
with open(path, 'rb') as source:
    source.read(8)
    code = marshal.load(source)

def find(parent, name):
    if parent.co_name == name:
        return parent
    for child in parent.co_consts:
        if isinstance(child, types.CodeType):
            result = find(child, name)
            if result is not None:
                return result

class Record(dict):
    def __getattr__(self, name):
        return self[name]
    def __setattr__(self, name, value):
        self[name] = value

player = Record(card_mgr={'card': Record(card_id=4401)},
                card_common_mgr={4401: Record(card_vo=0)}, global_set_vo=1, story_vo=1)
bank = 'card_test_vo'
sound = Record(bank_name=bank, event_name='japanese_event', event_name_ch='chinese_event')
data = Record(cards={4401: Record(vo_bank=[bank])}, sound={1: sound})
# 后缀映射逐项来自37C6CA84：0和2为空，1为_ch；不把0误作异常编号。
constants = Record(CARD_VO_JAPANESE=2, CARD_VO_BNK_NAME_SUFFIX={0:'', 1:'_ch', 2:''})
env = {'__builtins__': __builtin__.__dict__, 'gworld': Record(get_player=lambda:player), 'data':data,
       'common_const':constants, 'const':Record(STORY_VO_BNK_SUFFIX='_story', VO_BNK_SUFFIX='_vo')}
init = types.FunctionType(find(code,'init_vo_language_map'),env)
select = types.FunctionType(find(code,'get_sound_event_language'),env)
manager = Record()
init(manager)
assert select(manager,1) == ('japanese_event',0), '旧单卡0没有复现覆盖全局中文'
player.card_common_mgr[4401].card_vo=1
init(manager)
assert select(manager,1) == ('chinese_event',1), '默认中配投影后仍选择日文事件'
player.card_common_mgr[4401].card_vo=2
init(manager)
assert select(manager,1) == ('japanese_event',2), '单卡日配覆盖丢失'
sound.bank_name='generic_vo'
assert select(manager,1) == ('chinese_event',1), '通用语音没有读取global_set_vo'
sound.bank_name='plot_story'
assert select(manager,1) == ('chinese_event',1), '剧情语音未保留独立选项'
print(u'通过：原生声音函数复现旧问题，并验证默认中配、单卡日配、通用语音和剧情语音；不代表音频文件及实机播放验收。'.encode('utf-8'))
