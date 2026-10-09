"""导出Android商品与商店目录，保留源表哈希。"""
import hashlib,json,sys
from pathlib import Path
from mem_marshal_extract import Loader
from export_client_catalogs import instructions,text
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
sources={}
def load(name):
 p=root/'out/client_catalogs/tables'/(name+'.json');raw=p.read_bytes()
 sources[name]={'路径':p.relative_to(root).as_posix(),'SHA256':hashlib.sha256(raw).hexdigest()}
 return json.loads(raw)['数据']
ep=root/'out/npk_scripts/android_base/875C5BCF.marshal';raw=ep.read_bytes();code=Loader(raw).r_object();ops=list(instructions(code))
errors={text(code['names'][arg]):code['consts'][ops[i-1][2]] for i,(_,op,arg) in enumerate(ops) if i and op==116 and ops[i-1][1]==153 and isinstance(code['consts'][ops[i-1][2]],int)}
sources['error_code']={'路径':ep.relative_to(root).as_posix(),'SHA256':hashlib.sha256(raw).hexdigest()}
boxdata=load('gift_box');boxrows=boxdata.get('键值条目',[]);boxes={}
for row in boxrows:
 r=row['值'];boxes.setdefault(str(r['box_id']),[]).append(r)
result={'commodities':load('commodity'),'shops':load('shop'),'materials':load('materials'),'bonuses':load('bonus'),'rune_drops':load('runes_drops'),'item_libs':load('item_libs'),'item_libs_data':load('item_libs_data'),'gift_boxes':boxes,'mail_templates':load('mail_template'),'errors':errors,'来源':sources}
(root/'internal/game/shop_catalog.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print('商店目录',len(result['commodities']),len(result['shops']))
