"""保留本版3099个已转换模块，重包最新私有宿主与同源活动脚本。"""
from pathlib import Path
import datetime,hashlib,io,json,tarfile,sys
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
archive=root/'out/native-pvp-engine-linux-resources.tar.gz'
backup=root/'out/backups/remaining-20261008-activity/native-pvp-engine-linux-resources.tar.gz.bak'
if not backup.exists():backup.write_bytes(archive.read_bytes())
script=(root/'internal/game/activity_buffs_script.py').read_text(encoding='utf-8')
native=root/'internal/nativepvp/activity_buffs_native.py'
expected=script.split('def _start_hs_activity_buffs():')[0]
if native.read_text(encoding='utf-8')!=expected:raise ValueError('夏活Android与原生宿主不同源')
extra=['internal/nativepvp/activity_buffs_native.py','internal/game/activity_buffs_script.py','internal/game/activity_metrics_script.py','out/tools/test_remaining_activity_native.py','out/tools/test_activity_metrics_script.py']
updated=[]
target=archive.with_suffix('.new')
with tarfile.open(archive,'r:gz') as old,tarfile.open(target,'w:gz') as new:
 seen=set()
 for info in old.getmembers():
  if not info.isfile():raise ValueError('运行时包只接受普通文件')
  path=info.name.removeprefix('workspace/')
  source=root/path
  if path.startswith('internal/nativepvp/') and '/runtime/' not in path or path.startswith(('out/tools/','internal/game/')):
   data=source.read_bytes();updated.append(path)
  else:data=old.extractfile(info).read()
  info.size=len(data);new.addfile(info,io.BytesIO(data));seen.add(path)
 for path in extra:
  if path in seen:continue
  data=(root/path).read_bytes();info=tarfile.TarInfo('workspace/'+path);info.size=len(data);info.mode=0o644;new.addfile(info,io.BytesIO(data));updated.append(path)
target.replace(archive)
report={'时间':datetime.datetime.now().isoformat(),'已刷新源码':updated,'SHA256':hashlib.sha256(archive.read_bytes()).hexdigest(),'边界':'重新包装最新宿主与活动脚本；3099原生模块保留原包字节，不等于Linux执行或部署'}
(root/'out/remaining-activity-native-package.json').write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
print(json.dumps(report,ensure_ascii=False))
