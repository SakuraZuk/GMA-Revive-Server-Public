from pathlib import Path

p=Path('internal/game/activity_business.go')
s=p.read_text(encoding='utf-8')
s=s.replace('if len(rows) > 128 {','if len(rows) > len(androidActivities["activity_type"]) {')
a=s.index('// 固定奖励全量校验后提交；')
b=s.index('func activityChance(',a)
s=s[:a]+'''// 所有活动共用Android D44DBBA7奖励解释器；奖励盒、材料、幻书及契印同事务。
func grantActivityBonus(p *Progress, ids []int, now time.Time, levels ...int) (map[string]any, error) {
    level:=p.AvatarLevel
    if len(levels)>0 {level=levels[0]}
    box:=emptyActivityBox()
    for _,id:=range ids {
        if id<=0 {continue}
        part,err:=grantNativeBonus(p,id,1,level,now)
        if err!=nil{return nil,err}
        if err=mergeActivityBox(box,part);err!=nil{return nil,err}
    }
    return box,nil
}
''' + s[b:]
s=s.replace('Place      string `json:"place,omitempty"`','Place      string `json:"place,omitempty"`\n    Spent map[int]int64 `json:"spent_materials,omitempty"`\n    Angry bool `json:"angry,omitempty"`')
s=s.replace('if len(r["need_materials"]) > 0 && string(r["need_materials"]) != "[]" && string(r["need_materials"]) != "null" {\n\t\treturn nil, errors.New("活动副本专属材料费用尚未接线")\n\t}', '')
s=s.replace('p.Activities.LastTime = now.Unix()\n\treturn bc, nil', '''spent,err:=activityMaterialCosts(p,r["need_materials"])
    if err!=nil{return nil,err};bc.Spent=spent
    p.Activities.LastTime = now.Unix()
    return bc, nil''')
s=s.replace('for _, t := range tasks {\n\t\tif !containsInt(bc.Tasks, t) {','seenTasks:=map[int]bool{}\n    for _, t := range tasks {\n\t\tif !containsInt(bc.Tasks, t)||seenTasks[t] {')
s=s.replace('return nil, errors.New("活动结果包含未注入的战斗任务")\n\t\t}\n\t}', 'return nil, errors.New("活动结果包含未注入或重复的战斗任务")\n\t\t};seenTasks[t]=true\n\t}')
s=s.replace('\tif err != nil {\n\t\treturn nil, err\n\t}\n\tb.RewardGranted = true', '''    if !win && err==nil {
        returned:=r.integer("return_power")
        if returned<0||int64(returned)>math.MaxInt32-int64(p.currentPower(now)){return nil,errors.New("活动失败返还体力无效或溢出")}
        if returned>0{p.settlePowerRecovery(now);p.Power.Value+=returned;box["materials"].(map[int]int64)[1]+=int64(returned)}
        var pairs [][]int64
        if raw:=r["return_materials"];len(raw)>0&&string(raw)!="null" {
            if json.Unmarshal(raw,&pairs)!=nil{return nil,errors.New("活动失败返还材料结构无效")}
        }
        for _,pair:=range pairs {
            if len(pair)!=2||pair[0]<=0||pair[1]<0||pair[1]>bc.Spent[int(pair[0])]{return nil,errors.New("活动失败返还超过已扣材料")}
            if pair[1]==0{continue}
            part:=emptyActivityBox();changes:=part["materials"].(map[int]int64);cards:=[]string{}
            if err=grantNativeItem(p,int(pair[0]),pair[1],p.AvatarLevel,now,changes,&cards,0);err!=nil{return nil,err}
            part["cards"]=cardListWire(p,cards);if err=mergeActivityBox(box,part);err!=nil{return nil,err}
        }
    }
    if err != nil {return nil,err}
    b.RewardGranted = true''')
s += '''
// 只扣原生need_materials明确列出的库存费用；失败返还只允许已冻结的扣费。
func activityMaterialCosts(p *Progress,raw json.RawMessage)(map[int]int64,error){
    costs:=map[int]int64{};if len(raw)==0||string(raw)=="null"{return costs,nil}
    var pairs [][]int64;if json.Unmarshal(raw,&pairs)!=nil{return nil,errors.New("活动材料费用结构无效")}
    for _,v:=range pairs{if len(v)!=2||v[0]<=0||v[1]<=0||costs[int(v[0])]>math.MaxInt64-v[1]{return nil,errors.New("活动材料费用数量无效")};costs[int(v[0])]+=v[1]}
    for id,n:=range costs{m,ok:=androidShop.Materials[id];if !ok||m.Type!=4||p.Materials[id].Count<n{return nil,errors.New("活动费用材料无效或不足")}}
    for id,n:=range costs{m:=p.Materials[id];m.Count-=n;p.Materials[id]=m};return costs,nil
}

// 登录同玩家事务初始化原生活动容器，不能把进入活动当作开放状态的唯一来源。
// 地图、事件、初始赠送和消耗仍由对应入口校验，登录不代替节点解锁。
func ensureActivityLogin(p *Progress,level int,now time.Time)error{
    if _,e:=activityOpen(*p,203,level,now);e==nil{ensureMountain(p)}
    if _,e:=activityOpen(*p,206,level,now);e==nil{ensureCthulhu(p)}
    if _,e:=activityOpen(*p,207,level,now);e==nil{w:=ensureWangyan(p,now);if e=refreshWangyanSupply(w,now);e!=nil{return e}}
    if _,e:=activityOpen(*p,208,level,now);e==nil{ensureMiku(p)}
    if _,e:=activityOpen(*p,209,level,now);e==nil{ensureExam(p)}
    if _,e:=activityOpen(*p,211,level,now);e==nil{ensureSummer(p)}
    if _,e:=activityOpen(*p,328,level,now);e==nil&&p.Activities.Nian==nil{p.Activities.Nian=map[int]ActivityProgress{}}
    return nil
}
'''
p.write_text(s,encoding='utf-8')

p=Path('internal/game/cthulhu_business.go');s=p.read_text(encoding='utf-8')
s=s.replace('candidates := map[string]*CthulhuItem{"grid": m.grid().Item, "init": m.Init}', 'candidates := map[string]*CthulhuItem{"init": m.Init}\n        if g:=m.grid();g!=nil{candidates["grid"]=g.Item}')
s=s.replace('\tvar cr []string\n\t_ = cr\n','')
marker='\treturn nil\n}\n\nfunc (s *Service) cthulhuRPC'
s=s.replace(marker,'''    if value,ok:=src["runes"];ok {
        incoming,ok:=value.(map[string]any);if !ok{return errors.New("活动奖励盒契印结构无效")}
        present,ok:=dst["runes"].(map[string]any);if !ok{present=map[string]any{}}
        for id,row:=range incoming{if _,exists:=present[id];exists{return errors.New("活动奖励盒契印重复")};present[id]=row};dst["runes"]=present
    }
    return nil
}

func (s *Service) cthulhuRPC''')
s=s.replace('func ensureCthulhu(p *Progress) *CthulhuState {','func ensureCthulhu(p *Progress) *CthulhuState {\n    if p.Materials==nil{p.Materials=map[int]Material{}}')
p.write_text(s,encoding='utf-8')
