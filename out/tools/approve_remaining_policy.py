"""绑定用户本轮已明确批准的本服规则；保留此前批准与正式文件备份。"""
import hashlib,json,shutil
import sys
from pathlib import Path
from datetime import datetime

ROOT=Path(__file__).resolve().parents[2]
def sha(path): return hashlib.sha256(path.read_bytes()).hexdigest()
def main():
    sys.stdout.reconfigure(encoding='utf-8')
    stamp=datetime.now().strftime('%Y%m%d-%H%M%S')
    backup=ROOT/'out/backups'/('remaining-policy-'+stamp)
    backup.mkdir(parents=True)
    names=['deploy/data/gameplay-rules.env','out/gameplay-policy-approval.json','out/remaining-policy-proposal.json']
    for name in names:
        source=ROOT/name
        if source.exists():
            target=backup/name;target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(source,target)
    proposal_path=ROOT/'out/remaining-policy-proposal.json'
    proposal=json.loads(proposal_path.read_text(encoding='utf-8-sig'))
    proposal['状态']='用户明确批准；以本服运营规则实施，原生直接证据优先'
    proposal['用户原话']='统一采用推荐本服规则，继续全部完成'
    proposal['实施版本']='local-20261008-v1'
    proposal['原生证据修正']={
        '克苏鲁':'原生显示及检定字段证明骰和+fix_check<=属性；提案早期加到属性右侧的方向误读已修正',
        '汪言':'原生battle_base.on_bid_set接收enemy_level_added，修正敌方等级且最低5；不采用早期最低1的猜测',
        '教程加速':'按解码恢复的GuideFacilityAcc原生acc_second；6004为9000秒，优先于早期建议180秒',
        '随机碎片':'现行原生材料和合成表不存在对应随机碎片入口，不新增虚构配方',
        '委托':'20点仅切夜间9小时池；完成时冻结实际奖励并累计未领阶段奖，阶段领奖领取实际累计属于本服发放时点规则'
    }
    for item in proposal.get('建议',[]):
        path=ROOT/item['文件']
        item['内容']=json.loads(path.read_text(encoding='utf-8-sig'))
    proposal_path.write_text(json.dumps(proposal,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
    env_path=ROOT/'deploy/data/gameplay-rules.env'
    env=env_path.read_text(encoding='utf-8-sig')
    lines=[line for line in env.splitlines() if not line.startswith(('HS_REMAINING_GAMEPLAY_POLICY=','HS_SPECIAL_DRILL_REOPEN=','# 用户明确批准的剩余玩法规则版本','# 用户明确批准恢复原有12个特别演练'))]
    lines+=['# 用户明确批准的剩余玩法规则版本，细则见现行SERVER.md。','HS_REMAINING_GAMEPLAY_POLICY=local-20261008-v1',
            '# 用户明确批准恢复原有12个特别演练，保留原敌人、阈值与奖励。','HS_SPECIAL_DRILL_REOPEN=timber-12-20261008']
    env_path.write_text('\n'.join(lines)+'\n',encoding='utf-8',newline='\n')
    approval_path=ROOT/'out/gameplay-policy-approval.json'
    approval=json.loads(approval_path.read_text(encoding='utf-8-sig'))
    approval['批准范围']['剩余玩法规则']={'用户原话':proposal['用户原话'],'版本':proposal['实施版本'],'提案':'out/remaining-policy-proposal.json','提案SHA256':sha(proposal_path),'性质':'本服运营规则；直接原生证据按修正记录执行'}
    approval['批准范围']['特别演练恢复']={'用户原话':'恢复原有12个副本，继续全部完成','版本':'timber-12-20261008','原有副本':[2103,2106,2112,2203,2206,2212,2303,2306,2312,2503,2506,2512],'保留':'原敌人、轮次、伤害阈值、奖励及解锁门槛','来源':'out/client_catalogs/tables/timber_pile_dungeons.json；Android379F2D9A客户端实际enter_dungeon入口'}
    approval['未批准范围']=[]
    approval['正式文件SHA256']=sha(env_path)
    approval['更新时间']=datetime.now().isoformat()
    approval['发布状态']='本轮源码实现与核验进行中，尚未发布'
    approval_path.write_text(json.dumps(approval,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'状态':'已绑定明确授权','规则SHA256':sha(env_path),'提案SHA256':sha(proposal_path),'备份':str(backup)},ensure_ascii=False))

if __name__=='__main__':main()
