"""只生成可审阅的本服家具融合分组提案，不修改运行配置。"""
import hashlib,json,sys
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
path=root/'out/client_catalogs/tables/furniture_source.json'
raw=path.read_bytes();sources=json.loads(raw)['数据']
groups={'1':sources['2'],'2':sources['4'],'3':[6999999]}
pools={}
for tier in (1,2,3):
 for group,ids in groups.items():
  if tier==1 and group=='2':continue
  # 第一档原生第二组是礼包组，第三组为0概率。
  if tier==1 and group=='3':continue
  key=f'{tier}:{group}'
  pools[key]=[[mid,1] for mid in ids]
pools['1:2']=[[6999999,1]]
proposal={'状态':'待用户批准，未启用','用途':'HS_FURNITURE_FUSION_POOLS 候选池配置','证据边界':'原生档位门槛、分组概率及心愿单60%来自Android；选择器解释器不在APK。以下按source2为普通家具池、source4为限定家具池、6999999为礼包池定义本服规则，组内等权不是原服概率结论。','原生分组概率':{'1':[98,2,0],'2':[92,6,2],'3':[92,6,2]},'原生心愿单':{'3:2':0.6},'本服候选池':pools,'来源':{'路径':path.relative_to(root).as_posix(),'SHA256':hashlib.sha256(raw).hexdigest()}}
output=root/'out/furniture-fusion-pools-proposal.json'
output.write_text(json.dumps(proposal,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('家具融合候选池提案已生成，未修改环境配置：',len(pools),'组')
