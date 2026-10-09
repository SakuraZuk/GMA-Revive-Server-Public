"""显式启用项目私有Python2的单端权威专项，保存原始日志。"""
from pathlib import Path
import os, subprocess, sys, datetime
root=Path(__file__).resolve().parents[2]
env=os.environ.copy()
env['HS_NATIVE_PVP_TEST_PYTHON2']=str(root/'internal/nativepvp/runtime/native_engine/python2/package/tools/python.exe')
result=subprocess.run([r__import__('os').environ.get('HS_GO', 'go'),'test','./internal/game','-run','TestNativeSolo|TestNativePvpRoom','-count=1','-v'],cwd=root,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
(root/'out/native-solo-local.log').write_bytes(result.stdout)
(root/('out/native-solo-local-'+datetime.datetime.now().strftime('%Y%m%d-%H%M%S')+'.log')).write_bytes(result.stdout)
sys.stdout.reconfigure(encoding='utf-8')
print(result.stdout.decode('utf-8'))
raise SystemExit(result.returncode)
