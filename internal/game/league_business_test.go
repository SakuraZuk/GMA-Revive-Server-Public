package game

import (
	"context"
	"reflect"
	"testing"
)

func TestLeagueActualCreateApplyApprovalAtomicMembershipAndAchievement(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	for _, c := range cs[:3] {
		if _, err := store.UpdateProgress(ctx, selectedOID(c), func(p *Progress) error {
			p.AvatarLevel = 6
			p.UnlockSystems["league"] = 1
			p.Materials[11] = Material{ID: 11, Count: 5000, Total: 5000}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	before := socialSaved(t, store, cs[0]).Progress
	if _, err := s.Handle(ctx, cs[0], "create_new_league", socialArgs("学会事务", "真实申请审批", 4401)); err != nil {
		t.Fatal(err)
	}
	p := socialSaved(t, store, cs[0]).Progress
	if p.LeagueID == "" || p.Achievements[403001].Targets[403001] != 1 || p.Materials[11].Count >= before.Materials[11].Count {
		t.Fatal("创建学会未真实扣费/入会/成就")
	}
	if _, err := s.Handle(ctx, cs[1], "apply_league", socialArgs(p.LeagueID, nil)); err != nil {
		t.Fatal(err)
	}
	if socialSaved(t, store, cs[1]).Progress.LeagueID != "" {
		t.Fatal("未审批申请直接入会")
	}
	peerID := hexOf(selectedOID(cs[1]))
	approved, approvalErr := s.Handle(ctx, cs[0], "agree_league_apply", socialArgs(peerID))
	approvalReply := remainingFindPush(approved, "on_agree_league_apply")
	if approvalErr != nil || approvalReply == nil || len(approvalReply.Args) != 3 || reflect.ValueOf(approvalReply.Args[2]).Len() != 2 {
		t.Fatal("同意申请须携带原生成员字典", approvalErr, approved)
	}
	left, right := socialSaved(t, store, cs[0]).Progress, socialSaved(t, store, cs[1]).Progress
	if right.LeagueID != p.LeagueID || right.Achievements[403001].Targets[403001] != 1 || len(left.OwnedLeagues[p.LeagueID].Members) != 2 {
		t.Fatal("审批成员和成就未同事务")
	}
	out, err := s.Handle(ctx, cs[1], "agree_league_apply", socialArgs(hexOf(selectedOID(cs[2]))))
	reply := remainingFindPush(out, "on_agree_league_apply")
	if err != nil || reply == nil || len(reply.Args) != 3 || reply.Args[0] != androidLeague.Errors["RET_LEAGUE_AGREE_APPLY_NO_POWER"] {
		t.Fatal("普通成员越权审批未回原生错误码", err, out)
	}
	if !reflect.DeepEqual(left, socialSaved(t, store, cs[0]).Progress) || !reflect.DeepEqual(right, socialSaved(t, store, cs[1]).Progress) {
		t.Fatal("越权审批部分修改双方")
	}
	remainingRPC(t, s, cs[2], "apply_league", p.LeagueID, nil)
	thirdID := hexOf(selectedOID(cs[2]))
	refused, refusalErr := s.Handle(ctx, cs[0], "refuse_league_apply", socialArgs(thirdID))
	refusalReply := remainingFindPush(refused, "on_refuse_league_apply")
	if refusalErr != nil || refusalReply == nil || len(refusalReply.Args) != 4 || reflect.ValueOf(refusalReply.Args[2]).Len() != 0 || reflect.ValueOf(refusalReply.Args[3]).Len() != 0 {
		t.Fatal("拒绝须携带真实剩余申请队列/字典", refusalErr, refused)
	}
	if socialSaved(t, store, cs[2]).Progress.LeagueID != "" || len(socialSaved(t, store, cs[0]).Progress.OwnedLeagues[p.LeagueID].Members) != 2 {
		t.Fatal("拒绝不能改变成员或把申请者加入学会")
	}
	if _, err := s.Handle(ctx, cs[1], "query_total_league_info", socialArgs(p.LeagueID, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Handle(ctx, cs[1], "leave_league", nil); err != nil {
		t.Fatal(err)
	}
	if socialSaved(t, store, cs[1]).Progress.LeagueID != "" || len(socialSaved(t, store, cs[0]).Progress.OwnedLeagues[p.LeagueID].Members) != 1 {
		t.Fatal("退会没有清双档关系")
	}
	out, err = s.Handle(ctx, cs[1], "apply_league", socialArgs(p.LeagueID, nil))
	reply = remainingFindPush(out, "on_apply_league")
	if err != nil || reply == nil || reply.Args[0] != androidLeague.Errors["RET_LEAGUE_LEFTED_TIME_NOT_ENOUGH"] {
		t.Fatal("退会四小时内再次申请未回原生冷却错误码", err, out)
	}
}
