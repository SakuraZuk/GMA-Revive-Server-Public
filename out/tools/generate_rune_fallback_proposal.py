"""生成未指定套装的明确运营候选池草案；用户确认前不启用或发布。"""
import json,hashlib,sys
from pathlib import Path
sys.stdout.reconfigure(encoding='utf-8')
ROOT=Path(__file__).resolve().parents[2]
base=ROOT/'out/client_catalogs/tables'
def data(name):return json.loads((base/(name+'.json')).read_text(encoding='utf-8'))['数据']
suits=data('runes_suits');drops=data('runes_drops');runes=data('runes')
eligible={int(key) for key,row in suits.items() if row.get('drop_flag')==1}
pool=[[sid,100] for sid in sorted(eligible)]
proposal={key:pool for key,row in drops.items() if not any(pair[1] for pair in row['suit_id_weight'])}
target=ROOT/'out/rune-fallback-pools-proposal.json'
target.write_text(json.dumps(proposal,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
report={'状态':'等待用户选择运营规则，未启用','套装候选数量':len(pool),'缺默认套装的掉落数量':len(proposal),'候选':pool,'草案SHA256':hashlib.sha256(target.read_bytes()).hexdigest(),'原表来源':{name:hashlib.sha256((base/(name+'.json')).read_bytes()).hexdigest() for name in ['runes_suits','runes_drops']}}
(ROOT/'out/rune-fallback-proposal-report.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('已生成运营候选草案：%d个掉落，%d种drop_flag=1套装，尚未启用' % (len(proposal),len(pool)))
