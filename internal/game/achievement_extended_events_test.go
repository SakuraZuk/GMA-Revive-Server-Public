package game

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func achievementTargetCount(p Progress, achievementID, targetID int) int64 {
	return p.Achievements[achievementID].Targets[targetID]
}

func TestAchievementEmptyAndMultipleParameterAmount(t *testing.T) {
	now := time.Unix(1791360000, 0)
	p := NewProgress(1, now)
	if advanceAchievementAmount(&p, 1008, 1, 1, now) {
		t.Fatal("单参数误匹配原生空参数骰子目标")
	}
	if !advanceAchievementAmount(&p, 1008, nil, 2, now) || achievementTargetCount(p, 209002, 1008) != 2 {
		t.Fatal("原生空参数目标未按实际消费数量推进")
	}
	advanceAchievementAmount(&p, 1008, []any{}, 700, now)
	if achievementTargetCount(p, 209001, 1008) != 1 || achievementTargetCount(p, 209002, 1008) != 30 || achievementTargetCount(p, 209003, 1008) != 666 {
		t.Fatal("多级共享目标未按各成就阈值封顶")
	}
	before := CloneProgress(p)
	if advanceAchievementAmount(&p, 1008, nil, -1, now) || !reflect.DeepEqual(before, CloneProgress(p)) {
		t.Fatal("非正消费数量仍推进目标")
	}
	if advanceAchievementAmount(&p, 14, []int{3, 0}, 1, now) || !advanceAchievementAmount(&p, 14, []int{0, 3}, 1, now) {
		t.Fatal("多参数目标未严格保留原生顺序")
	}
}

func TestAchievementReturnGiftActualRPCAndRejectedRepeat(t *testing.T) {
	ctx := context.Background()
	store := NewFixtureAccounts(nil)
	s := New(store, nil)
	c, av := newBattleConnection(t, ctx, store, s)
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{newCard(4401, 1, s.Now())}
		p.Materials[701] = Material{ID: 701, Count: 1, Total: 1}
		p.Materials[523] = Material{ID: 523, Count: 1, Total: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Handle(ctx, c, "consume_intimacy_gift", socialArgs(1, 4401, 701, 1, 0)); err != nil {
		t.Fatal(err)
	}
	if achievementTargetCount(c.SelectedAvatarUnsafe().Progress, 305145, 1000045) != 0 {
		t.Fatal("没有返还奖励的普通礼物被当成回礼")
	}
	args := socialArgs(2, 4401, 523, 1, 0)
	if _, err := s.Handle(ctx, c, "consume_intimacy_gift", args); err != nil {
		t.Fatal(err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if achievementTargetCount(p, 305145, 1000045) != 1 || len(p.Runes) != 4 {
		t.Fatal("真实回礼与对应幻书目标未一起持久化")
	}
	before := CloneProgress(p)
	if _, err := s.Handle(ctx, c, "consume_intimacy_gift", args); err != nil {
		t.Fatal(err)
	}
	after, err := store.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("材料不足的重复回礼改变资产或成就", err)
	}
}

func TestAchievementFriendAssistActualConsumeRollbackAndDuplicate(t *testing.T) {
	s, cs, store, _, layout := friendAssistWorld(t)
	ctx := context.Background()
	setAssistTestBattle(t, s, cs[0], "00112233445566778899aabb")
	ownBefore, peerBefore := socialSaved(t, store, cs[0]), socialSaved(t, store, cs[1])
	if err := s.updateBattleFormation(ctx, cs[0], layout, 0, true, func(*Progress) error { return errors.New("实际消费后的业务失败") }); err == nil {
		t.Fatal("失败事务未拒绝")
	}
	if !reflect.DeepEqual(ownBefore.Progress, socialSaved(t, store, cs[0]).Progress) || !reflect.DeepEqual(peerBefore.Progress, socialSaved(t, store, cs[1]).Progress) {
		t.Fatal("实际助战事件或双人资产在失败后部分提交")
	}
	start := func(p *Progress) error { p.Battle.Started = true; return nil }
	for n := 0; n < 2; n++ {
		if err := s.updateBattleFormation(ctx, cs[0], layout, 0, true, start); err != nil {
			t.Fatal(err)
		}
		if achievementTargetCount(socialSaved(t, store, cs[0]).Progress, 402004, 402001) != 1 {
			t.Fatal("助战消费未计一次或同一战斗重复计数")
		}
	}
}

func TestAchievementFullRuneEquipActualRPCDistinctSlots(t *testing.T) {
	ctx := context.Background()
	store := NewFixtureAccounts(nil)
	s := New(store, nil)
	c, av := newBattleConnection(t, ctx, store, s)
	uuid := "00112233445566778899aabb"
	ids := []string{}
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{newCard(4401, 1, s.Now())}
		p.Cards[0].UUID = uuid
		p.Runes = map[string]Rune{}
		p.RuneSchemaVersion = CurrentRuneSchemaVersion
		for pos := 1; pos <= 4; pos++ {
			r, err := GrantRune(p, RuneSpec{Suit: 1101, Position: pos, Star: 5, Level: 1}, s.Now())
			if err != nil {
				return err
			}
			ids = append(ids, r.UUID)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for index, id := range ids {
		if _, err := s.Handle(ctx, c, "embed_rune", socialArgs(1, uuid, id)); err != nil {
			t.Fatal(err)
		}
		count := achievementTargetCount(c.SelectedAvatarUnsafe().Progress, 303003, 10502)
		if index < 3 && count != 0 || index == 3 && count != 1 {
			t.Fatal("全身佩戴未按四个不同位置判定", index, count)
		}
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if _, err := s.Handle(ctx, c, "embed_rune", socialArgs(1, uuid, ids[3])); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("重复佩戴增加成就或改档")
	}
	// 四件背包契印分属不同幻书，不能组合成一名幻书的全身装备。
	p := NewProgress(1, s.Now())
	p.Cards = []Card{{UUID: uuid}, {UUID: "00112233445566778899aabc"}}
	p.Runes = map[string]Rune{}
	for i, id := range ids {
		p.Runes[id] = Rune{UUID: id, Star: 5, Position: i + 1, CardUUID: p.Cards[i%2].UUID}
	}
	if achievementFullRuneCards(p, 3) != 0 {
		t.Fatal("跨幻书星级总和冒充全身佩戴")
	}
}

func TestAchievementSyncAIWinReceiptDuplicateAndLoss(t *testing.T) {
	for _, outcome := range []string{"win", "loss"} {
		t.Run(outcome, func(t *testing.T) {
			now := time.Unix(1791360000, 0)
			p := NewProgress(1, now)
			if _, err := prepareSyncPvpMatch(&p, "C3竞技事件", now); err != nil {
				t.Fatal(err)
			}
			p.Battle = &BattleSession{UUID: p.SyncPvpMatch.BattleUUID, BridgeReady: true, BridgeStarted: true, Finished: true, Outcome: outcome}
			if _, err := settleSyncPvpResult(&p, 0, now); err != nil {
				t.Fatal(err)
			}
			want := int64(0)
			if outcome == "win" {
				want = 1
			}
			if achievementTargetCount(p, 207001, 207001) != want {
				t.Fatal("竞技事件胜负归属错误", outcome)
			}
			before := CloneProgress(p)
			if _, err := settleSyncPvpResult(&p, 0, now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, CloneProgress(p)) {
				t.Fatal("同UUID竞技收据重复推进成就")
			}
		})
	}
}

func TestAchievementAsyncRealWinReceiptDuplicate(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	if _, err := s.asyncPvpRPC(ctx, cs[0], "enter_asyn_pvp", nil); err != nil {
		t.Fatal(err)
	}
	target := hexOf(selectedOID(cs[1]))
	if _, err := s.enterAsyncPvp(ctx, cs[0], socialArgs(1, 1, map[string]any{"asyn_pvp_eid": target})); err != nil {
		t.Fatal(err)
	}
	if err := s.updateProgress(ctx, cs[0], func(p *Progress) error {
		p.Battle.BridgeReady = true
		p.Battle.BridgeStarted = true
		p.Battle.LastSequence = 2
		p.Battle.Team = append([]string{}, p.Lineup...)
		freezeAsyncOwnTeam(p)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	event := &battleEnvelope{BattleUUID: cs[0].SelectedAvatarUnsafe().Progress.Battle.UUID, Sequence: 3, Kind: "result", Data: map[string]any{"winner_eids": []any{hexOf(selectedOID(cs[0]))}, "outcome": "win"}}
	for n := 0; n < 2; n++ {
		if _, _, err := s.absorbAsyncPvpResult(ctx, cs[0], event); err != nil {
			t.Fatal(err)
		}
		if achievementTargetCount(socialSaved(t, store, cs[0]).Progress, 206001, 206001) != 1 {
			t.Fatal("真实异步胜利未记一次或同收据重试重复推进")
		}
	}
	if achievementTargetCount(socialSaved(t, store, cs[1]).Progress, 206001, 206001) != 0 {
		t.Fatal("防守败者收到胜利事件")
	}
}
