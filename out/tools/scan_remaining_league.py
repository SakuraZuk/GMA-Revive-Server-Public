"""只读扫描学会真实入口、成长门槛和战斗结果；保存源哈希与反汇编。"""
import hashlib,io,json,sys
from pathlib import Path
from mem_marshal_extract import Loader
from neox_dis import disasm
ROOT=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
words={'league_protect_start','on_league_protect_end','unlock_growth','get_growth','league_card_feed','feed_league_card','league_explore_start','league_boss_challenge_start','league_card_grade','league_card_exp','league_card_enhance','league_card_skill','league_card_psychic','league_card_donate','league_card_add_score','protect_unlock_map','on_league_protect_start','league_activity_weekly_info','change_league_member_type','transfer_league_president'}
inv=json.loads((ROOT/'out/npk_scripts/android_inventory.json').read_text(encoding='utf-8'))
hits=[]
for module in inv['modules']:
    path=ROOT/'out/npk_scripts'/module['marshal_file'];raw=path.read_bytes();c=Loader(raw).r_object()
    def visit(c,parent=''):
        name=c['name'].decode('utf-8');full=parent+'/'+name
        symbols={v.decode('utf-8',errors='backslashreplace') if isinstance(v,bytes) else str(v) for v in c['names']+c['varnames']+tuple(v for v in c['consts'] if isinstance(v,(str,bytes)))}
        matched=(symbols|{name})&words
        if matched or b'league_activity_weekly_info.py' in c.get('filename',b''):
            shallow=dict(c);shallow['consts']=[v if not isinstance(v,dict) else {'引用函数':str(v.get('name'))} for v in c['consts']]
            out=io.StringIO();disasm(shallow,out=out)
            hits.append({'来源':str(path.relative_to(ROOT)),'SHA256':hashlib.sha256(raw).hexdigest(),'函数':full,'命中':sorted(matched),'反汇编':out.getvalue()})
        for child in c['consts']:
            if isinstance(child,dict) and child.get('type')=='code':visit(child,full)
    visit(c)
report={'模块数':len(inv['modules']),'函数':hits}
(ROOT/'out/remaining-league-native-scan.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
(ROOT/'out/dis/remaining-league-full-chain.asm').write_text('\n'.join(x['来源']+' '+x['SHA256']+'\n'+x['反汇编'] for x in hits),encoding='utf-8')
print('学会入口命中：',len(hits))
