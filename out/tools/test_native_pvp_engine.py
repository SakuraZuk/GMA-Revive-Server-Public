"""用本版Android字节码运行真实双阵容原生PVP，核验确定性与拒绝无效点击。"""
from pathlib import Path
import datetime,hashlib,json,sys,os

ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/'internal/nativepvp'))
from battle_native import NativeBattleProcess,NativeCommandRejected

def duel(seed, automatic):
    options={'python2':Path(os.environ['HS_NATIVE_PVP_PYTHON2'])} if os.environ.get('HS_NATIVE_PVP_PYTHON2') else {}
    with NativeBattleProcess(**options) as worker:
        ids=[1601,2103,2202,4402]
        roles=worker.request('table',table='role_info',keys=ids,fields=['skill_list'])
        def roster(side):
            return [dict(card_id=card,uuid=('%024x'%(side*100+i+1)),level=40,grade=4,awakened=1,
                         skill_mgr=[dict(skill_id=skill,level=1,enhance_level=0) for skill in roles[str(card)]['skill_list']]) for i,card in enumerate(ids)]
        metadata=dict(dungeon_id=21,avatar_id='000000000000000000000001',enemy_id='000000000000000000000002',
                      battle_uuid='000000000000000000000003',enemy_roster=roster(2),battle_type=2,enemy_auto=automatic)
        started=worker.start(metadata,roster(1),seed,automatic)
        if automatic:
            final=worker.autoplay(10000)
            assert final['status']=='finished' and final['result'] is not None, '真实原生PVP未完成'
            assert final['result']['winner_eids'] and set(final['result']['winner_eids'])<=set((metadata['avatar_id'],metadata['enemy_id']))
            return {'结果':final['result'],'步数':final['steps'],'末态':final['state'],'事件数':len(started['events'])+len(final['events'])}
        update=worker.drive()
        assert update['state']['awaiting_player'],'双手动原生引擎未停在输入窗口'
        before=worker.snapshot()
        try:worker.step({'name':'move_to','args':['不存在的单位',[0,0,0]]})
        except NativeCommandRejected:pass
        else:raise AssertionError('非法单位没有被原生拒绝')
        assert worker.snapshot()==before,'非法点击改变了原生状态或输入计时器'
        after=worker.timeout()
        assert after['steps']>update['steps'] and (after['events'] or after['result'] is not None),'真实原生超时没有执行'
        return {'拒绝前后原生状态一致':True,'输入到期实际执行':True,'事件数':len(after['events'])}

def main():
    sys.stdout.reconfigure(encoding='utf-8');start=datetime.datetime.now().isoformat()
    first=duel(123456,True);second=duel(123456,True)
    assert first==second,'相同冻结阵容/seed的原生服务端计算不一致'
    manual=duel(123456,False)
    report={'开始时间':start,'结束时间':datetime.datetime.now().isoformat(),'退出码':0,'实际引擎':'CPython2.7.18＋本版Android原生server_battle','自动双阵容两次完全相同':True,'自动战斗':first,'手动输入拒绝与超时':manual,'边界':'真实原生引擎运行；尚不是Go房间接入、数据库、双Android播放或发布验收'}
    dest=ROOT/'out/native-pvp-engine-verification.json';dest.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({k:v for k,v in report.items() if k!='自动战斗'},ensure_ascii=False));print('自动战斗结果',first['结果'],'步数',first['步数'],'事件数',first['事件数'])

if __name__=='__main__':main()
