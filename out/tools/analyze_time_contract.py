# -*- coding: utf-8 -*-
"""以客户端静态指令和 Python 时间模型复核时区，保留实机验收边界。"""
from datetime import datetime,timedelta,timezone
import json
from pathlib import Path
import sys
from mem_marshal_extract import Loader
from export_client_catalogs import walk,instructions

ROOT=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
code=Loader((ROOT/'out/npk_scripts/android_base/9AD0A456.marshal').read_bytes()).r_object()
reset=next(item for path,item in walk(code) if path=='<module>/reset_time_function')
assert (27,67,None) in list(instructions(reset))
now=1791162000.5
samples=[]
for tz in (8,28800,-28800):
    offset=-tz
    display=datetime.fromtimestamp(now,timezone(timedelta(seconds=offset)))
    samples.append({'协议时区':tz,'客户端实际偏移秒数':offset,'整分钟偏移':offset%60==0,
                    'Python3时间模型结果':display.isoformat(),
                    'Python2时区约束': '通过' if offset%60==0 else '拒绝非整分钟偏移，不能用 Python3 的成功结果作为客户端兼容证明'})
assert samples[-1]['客户端实际偏移秒数']==28800
report={'状态':'静态契约及时间模型通过','客户端模块':'9AD0A456 utils/time_utils.py',
        '取负指令偏移':27,'协议正确时区秒数':-28800,'测试':samples,
        '异常路径':'time_to_str 同时捕获 fromtimestamp 与 strftime 异常并返回 invalid timestamp，旧记录没有保留异常类型',
        '验收边界':'已修正服务端时区、Float 编码、空字典时间查询并验证公网协议；未重新读取 Android 日志，不能认定 invalid timestamp 已消失'}
(ROOT/'out/time-contract-verification.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps(report,ensure_ascii=False,indent=2))
