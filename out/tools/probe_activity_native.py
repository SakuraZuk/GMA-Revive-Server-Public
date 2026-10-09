"""仅用隔离原生Python2核对活动Record和方法；不连接客户端或生产。"""
from pathlib import Path
import subprocess
root = Path(__file__).resolve().parents[2]
source = r'''
# -*- coding: utf-8 -*-
import sys, copy, glob
sys.path.extend(glob.glob(r'.\internal\nativepvp\runtime\native_engine\deps\*.whl'))
sys.path.insert(0, r'.\internal\nativepvp')
sys.path.insert(0, r'.\internal\nativepvp\runtime\native_engine\script')
from battle_native_host import Host
h = Host(); h.install()
import data
b = data.buff[701424]
print(type(b), type(b).__bases__, hasattr(b, '__dict__'), hasattr(type(b),'__slots__'))
c = copy.copy(b)
try:
 c.multiply_modifiers = [('damage_add_rate',0.6,None,None,None)]
 print('属性赋值成功',c.multiply_modifiers)
except Exception as e:
 print('属性赋值失败',type(e),e)
from battle_logic.skill_events.cure_add_rate import cure_add_rate
import dis
dis.dis(cure_add_rate)
'''
path=root/'out/activity-native-probe.py'
path.write_text(source,encoding='utf8')
result=subprocess.run([str(root/'internal/nativepvp/runtime/native_engine/python2/package/tools/python.exe'),str(path)],cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
(root/'out/activity-native-probe.log').write_bytes(result.stdout)
print(result.stdout.decode('utf8'))
raise SystemExit(result.returncode)
