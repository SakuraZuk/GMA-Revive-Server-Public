from pathlib import Path
p=Path('internal/game/house_frage_business.go');s=p.read_text(encoding='utf-8')
s=s.replace('"receive_board_game_bonus": 1','"get_house_board_game_reward": 0').replace('case "receive_board_game_bonus":','case "get_house_board_game_reward":').replace('method == "receive_board_game_bonus"','method == "get_house_board_game_reward"')
s=s.replace('if json.Unmarshal(args[0], &cb) != nil || cb <= 0 {','direct:=method=="get_house_board_game_reward"\n    if !direct&&(json.Unmarshal(args[0], &cb) != nil || cb <= 0) {')
s=s.replace('w := ensureHouseFrage(p)\n\t\tswitch method {','w := ensureHouseFrage(p)\n        refreshHouseFrageDaily(p,s.Now())\n\t\tswitch method {')
s=s.replace('f.BoardReward = true','f.BoardReward = false').replace('reply = []any{RetSuccess}\n\t\t\treturn nil','reply = []any{RetSuccess,[]any{[]any{401,count}}}\n\t\t\treturn nil')
s=s.replace('case "get_house_board_game_reward":\n\t\t\treply = []any{activityErrorCode(err)}','case "get_house_board_game_reward":\n            return []Push{push("Avatar","on_get_house_board_game_reward",activityErrorCode(err),[]any{})},nil')
s=s.replace('return append(out, Callback(cb, reply)), nil','if direct{return append(out,push("Avatar","on_get_house_board_game_reward",reply...)),nil}\n    return append(out, Callback(cb, reply)), nil')
s+='''
// 原生board_game_reward表示可领取；UTC+8日界只恢复资格，领取才实际入库存。
func refreshHouseFrageDaily(p *Progress,now time.Time){if p.Collection==nil||p.Collection.Facilities[6].Level<=0{return};w:=ensureHouseFrage(p);day:=now.In(time.FixedZone("北京时间",28800)).Format("2006-01-02");f:=p.Collection.Facilities[6];f.BoardReward=day>w.RewardDay;p.Collection.Facilities[6]=f}
'''
p.write_text(s,encoding='utf-8')
p=Path('internal/game/activity_business.go');s=p.read_text(encoding='utf-8').replace('"receive_board_game_bonus"','"get_house_board_game_reward"')
s=s.replace('func ensureActivityLogin(p *Progress, level int, now time.Time) error {','func ensureActivityLogin(p *Progress, level int, now time.Time) error {\n    refreshHouseFrageDaily(p,now)')
s=s.replace('ensureMiku(p)\n\t}', 'ensureMiku(p)\n        reconcileMikuAchievements(p)\n\t}')
p.write_text(s,encoding='utf-8')
