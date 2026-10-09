package game

import (
	"context"
	"reflect"
	"testing"
)

func TestLeagueNativeBusinessFailureCallbacksLeaveAtomicStateUntouched(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	for _, c := range cs[:2] {
		if _, err := store.UpdateProgress(ctx, selectedOID(c), func(p *Progress) error { p.AvatarLevel = 6; p.Materials[11] = Material{ID: 11}; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	before := socialSaved(t, store, cs[0]).Progress
	out, err := s.Handle(ctx, cs[0], "create_new_league", socialArgs("材料不足", "原生失败回包", 4401))
	reply := remainingFindPush(out, "on_create_new_league")
	if err != nil || reply == nil || reply.Args[0] != androidLeague.Errors["RET_LEAGUE_CREATE_COST_ENOUGH"] || !reflect.DeepEqual(before, socialSaved(t, store, cs[0]).Progress) {
		t.Fatal("创建成本不足没有原生失败或改写存档", err, out)
	}
	if _, err = store.UpdateProgress(ctx, selectedOID(cs[0]), func(p *Progress) error { p.Materials[11] = Material{ID: 11, Count: 5000, Total: 5000}; return nil }); err != nil {
		t.Fatal(err)
	}
	remainingRPC(t, s, cs[0], "create_new_league", "满员失败", "审批原子拒绝", 4401)
	id := socialSaved(t, store, cs[0]).Progress.LeagueID
	remainingRPC(t, s, cs[1], "apply_league", id, nil)
	left, right := socialSaved(t, store, cs[0]).Progress, socialSaved(t, store, cs[1]).Progress
	out, err = s.Handle(ctx, cs[0], "appoint_league_member", socialArgs(hexOf(selectedOID(cs[1])), 1))
	reply = remainingFindPush(out, "on_appoint_league_member")
	if err != nil || reply == nil || reply.Args[0] != androidLeague.Errors["RET_LEAGUE_APPOINT_NOT_EXIST"] || !reflect.DeepEqual(left, socialSaved(t, store, cs[0]).Progress) || !reflect.DeepEqual(right, socialSaved(t, store, cs[1]).Progress) {
		t.Fatal("任命未入会申请者没有原生失败或双档变动", err, out)
	}
	// 主档按原表member_limit填到上限，审批必须拒绝并保留申请者状态。
	if _, err = store.UpdateProgress(ctx, selectedOID(cs[0]), func(p *Progress) error {
		state := p.OwnedLeagues[id]
		limit := leagueData("league_level", state.Level).integer("member_limit")
		for len(state.Members) < limit {
			state.Members[newBattleUUID(s.Now())] = LeagueMember{Type: 3, Joined: s.Now().Unix()}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	left, right = socialSaved(t, store, cs[0]).Progress, socialSaved(t, store, cs[1]).Progress
	out, err = s.Handle(ctx, cs[0], "agree_league_apply", socialArgs(hexOf(selectedOID(cs[1]))))
	reply = remainingFindPush(out, "on_agree_league_apply")
	if err != nil || reply == nil || len(reply.Args) != 3 || reply.Args[0] != androidLeague.Errors["RET_LEAGUE_MEMBER_FULL"] || !reflect.DeepEqual(left, socialSaved(t, store, cs[0]).Progress) || !reflect.DeepEqual(right, socialSaved(t, store, cs[1]).Progress) {
		t.Fatal("满员审批没有原生失败或双档部分提交", err, out)
	}
	out, err = s.Handle(ctx, cs[1], "refuse_league_apply", socialArgs(hexOf(selectedOID(cs[0]))))
	reply = remainingFindPush(out, "on_refuse_league_apply")
	if err != nil || reply == nil || len(reply.Args) != 4 || !reflect.DeepEqual(left, socialSaved(t, store, cs[0]).Progress) || !reflect.DeepEqual(right, socialSaved(t, store, cs[1]).Progress) {
		t.Fatal("越权拒绝须完整回包且双档不变", err, out)
	}
	out, err = s.Handle(ctx, cs[0], "leave_league", nil)
	reply = remainingFindPush(out, "on_leave_league")
	if err != nil || reply == nil || reply.Args[0] != androidLeague.Errors["RET_LEAGUE_PRESIDENT_CANT_LEAVE"] || !reflect.DeepEqual(left, socialSaved(t, store, cs[0]).Progress) {
		t.Fatal("会长未经移交退会没有原生失败回包", err, out)
	}
}
