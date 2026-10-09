package game

import (
	"context"
	"reflect"
	"testing"
)

func TestStrangerAssistActualRosterAuthorizationLimitAndFrozenRetry(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	own, peer := hexOf(selectedOID(cs[0])), hexOf(selectedOID(cs[1]))
	_, err := store.UpdateSocial(ctx, []string{own, peer}, func(rows map[string]*Avatar) error {
		rows[own].Progress.UnlockSystems["assist"] = 1
		rows[own].Progress.UnlockSystems["support"] = 1
		rows[peer].Progress.AssistCardUUID = rows[peer].Progress.Cards[0].UUID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	uuid := socialSaved(t, store, cs[1]).Progress.AssistCardUUID
	args := socialArgs(1, uuid, map[string]any{"eid": peer, "hostnum": 1})
	pushes, err := s.Handle(ctx, cs[0], "select_assist_card", args)
	if err != nil || socialCallback(t, pushes)[0] == RetSuccess {
		t.Fatal("无候选授权可凭陌生UUID借卡", err)
	}
	if _, err = s.Handle(ctx, cs[0], "refresh_assist_use_times", nil); err != nil {
		t.Fatal(err)
	}
	p := socialSaved(t, store, cs[0]).Progress
	if len(p.Social.Assist.Strangers) != 1 || p.Social.Assist.Strangers[peer].Card.UUID != uuid {
		t.Fatal("没有实际陌生人候选", p.Social.Assist)
	}
	pushes, err = s.Handle(ctx, cs[0], "select_assist_card", args)
	if err != nil || socialCallback(t, pushes)[0] != RetSuccess {
		t.Fatal("有效陌生候选无法选择", err, pushes)
	}
	layout := BattleLayout{Fighting: []string{p.Cards[0].UUID, uuid}}
	setAssistTestBattle(t, s, cs[0], "00112233445566778899aabb")
	start := func(p *Progress) error { p.Battle.Started = true; p.Battle.Team = layout.team(); return nil }
	if err = s.updateBattleFormation(ctx, cs[0], layout, 0, true, start); err != nil {
		t.Fatal(err)
	}
	after := socialSaved(t, store, cs[0]).Progress
	other := socialSaved(t, store, cs[1]).Progress
	if after.Social.Assist.Active[uuid] != 1 || other.Social.Assist.Passive != 1 || len(after.Cards) != len(p.Cards) {
		t.Fatal("陌生计次/提供者计次或真实归属错误")
	}
	if err = s.updateBattleFormation(ctx, cs[0], layout, 0, true, start); err != nil {
		t.Fatal("冻结原场重试被拒绝", err)
	}
	if !reflect.DeepEqual(after, socialSaved(t, store, cs[0]).Progress) || !reflect.DeepEqual(other, socialSaved(t, store, cs[1]).Progress) {
		t.Fatal("冻结重试重复计次或发奖")
	}
	setAssistTestBattle(t, s, cs[0], "00112233445566778899aabc")
	before := socialSaved(t, store, cs[0]).Progress
	if err = s.updateBattleFormation(ctx, cs[0], layout, 0, true, start); err == nil {
		t.Fatal("陌生助战突破每日每卡一次")
	}
	if !reflect.DeepEqual(before, socialSaved(t, store, cs[0]).Progress) || !reflect.DeepEqual(other, socialSaved(t, store, cs[1]).Progress) {
		t.Fatal("次数拒绝未双角色整事务回滚")
	}
}
