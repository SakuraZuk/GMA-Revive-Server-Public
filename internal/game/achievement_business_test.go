package game

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

// 旧业务断言只计算该业务消息；成就消息另由本文件验证。
func businessPushCount(pushes []Push) int {
	n := 0
	for _, p := range pushes {
		if p.Method == "client_prop_changed" && len(p.Args) == 1 {
			if args, ok := p.Args[0].([]any); ok && len(args) == 2 && (args[0] == "achves" || args[0] == "achv_value" || args[0] == "cards_count" || args[0] == "last_main_chapter_dungeon_id") {
				continue
			}
		}
		n++
	}
	return n
}

func TestAchievementBusinessEventsStateAndPoints(t *testing.T) {
	p := Progress{Cards: []Card{{UUID: "a", CardID: 4401, Level: 60, Grade: 5}}, Runes: map[string]Rune{"r": {Level: 11}}}
	now := time.Now()
	advanceAchievementAmount(&p, 33, "up_level_card", 1, now)
	advanceAchievementAmount(&p, 28, 1, 20, now)
	advanceAchievementEvent(&p, 9, 3, now)
	reconcileAchievementState(&p, 30, now)
	for _, id := range []int{301101, 301104, 301201, 301204, 302001, 303101, 303103, 303104, 305001, 305002} {
		if achievementCount(p.Achievements[id], androidAchievements.Rules[id]) < androidAchievements.Rules[id].Need {
			t.Fatalf("业务成就未推进%d", id)
		}
	}
	if achievementCount(p.Achievements[301105], androidAchievements.Rules[301105]) != 1 {
		t.Fatal("单卡误计为四卡")
	}
	before := CloneProgress(p)
	reconcileAchievementState(&p, 30, now.Add(time.Hour))
	if !reflect.DeepEqual(before, p) {
		t.Fatal("状态重复扫描虚增积分或更新完成时间")
	}
	p.Cards = nil
	p.Runes = nil
	reconcileAchievementState(&p, 30, now)
	if p.Achievements[301104].Targets[301104] != 1 {
		t.Fatal("历史已完成成就被倒扣")
	}
}

func TestAchievementGiftTransactionPushAndFailure(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	svc := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, svc)
	if err := svc.updateProgress(ctx, c, func(p *Progress) error { p.Materials[701] = Material{ID: 701, Count: 20, Total: 20}; return nil }); err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`4401`), json.RawMessage(`{"701":20}`)}
	pushes, err := svc.Handle(ctx, c, "consume_multi_intimacy_gift", args)
	if err != nil {
		t.Fatal(err)
	}
	if len(pushes)-businessPushCount(pushes) != 2 || pushes[len(pushes)-1].Method != "call_client_callback" {
		t.Fatal("成就未在线推送或回调提前")
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Achievements[305002].Targets[20401] != 20 {
		t.Fatal("多份礼物计数错误")
	}
	before := CloneProgress(p)
	_, _ = svc.Handle(ctx, c, "consume_multi_intimacy_gift", args)
	if !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("失败送礼推进成就")
	}
}

func TestAchievementLoginDayAndNativeGroupCount(t *testing.T) {
	p := Progress{}
	now := time.Date(2026, 10, 7, 15, 59, 59, 0, time.UTC)
	if err := recordAchievementLogin(&p, now); err != nil {
		t.Fatal(err)
	}
	if err := recordAchievementLogin(&p, now); err != nil {
		t.Fatal(err)
	}
	if p.Achievements[101001].Targets[101001] != 1 || achievementPoints(p) != 10 {
		t.Fatal("登录成就同日重复或积分错误")
	}
	if err := recordAchievementLogin(&p, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if p.Achievements[101002].Targets[101001] != 2 {
		t.Fatal("跨日登录未推进")
	}
	before := CloneProgress(p)
	if recordAchievementLogin(&p, now) == nil || !reflect.DeepEqual(before, p) {
		t.Fatal("回拨登录污染成就")
	}
	a := Achievement{Targets: map[int]int64{1: 2, 2: 3, 3: 4}}
	if achievementCount(a, achievementRule{Targets: [][]int{{1, 2}, {3}}}) != 4 {
		t.Fatal("成就原生组间最小计数错误")
	}
}

func TestAchievementRewardAllCatalogVariants(t *testing.T) {
	for bid := range androidAchievements.Rewards {
		p := Progress{Materials: map[int]Material{}}
		m := map[int]int64{}
		uuids := []string{}
		if err := grantAchievementReward(&p, bid, time.Now(), m, &uuids, 0); err != nil {
			t.Fatalf("成就奖励%d: %v", bid, err)
		}
		for id, n := range m {
			if n <= 0 || p.Materials[id].Count != n || p.Materials[id].Total != n {
				t.Fatal("奖励余额错误")
			}
		}
	}
}

func TestAchievementClaimIdempotencyAndBatchRollback(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	svc := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, svc)
	_, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		ensureAchievements(p)
		a := p.Achievements[101001]
		a.Targets[101001] = 1
		p.Achievements[101001] = a
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`101001`)}
	pushes, err := svc.Handle(ctx, c, "receive_achv_bonus", args)
	if err != nil || pushes[len(pushes)-1].Args[1].([]any)[0] != 0 {
		t.Fatal(err, pushes)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	pushes, err = svc.Handle(ctx, c, "receive_achv_bonus", args)
	if err != nil || pushes[0].Args[1].([]any)[0] != 6004 {
		t.Fatal("重复领奖未返回原生码", err)
	}
	stored, err := accounts.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil || !reflect.DeepEqual(before, stored.Avatars[0].Progress) {
		t.Fatal("重复领奖修改存档", err)
	}
	_, err = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		for _, id := range []int{101001, 101002} {
			a := p.Achievements[id]
			a.Claimed = false
			a.Targets[101001] = 7
			p.Achievements[id] = a
		}
		m := p.Materials[11]
		m.Count = math.MaxInt64 - 50
		m.Total = math.MaxInt64 - 50
		p.Materials[11] = m
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(ctx, c, "receive_all_achv_bonus", []json.RawMessage{json.RawMessage(`2`), json.RawMessage(`[101001,101002]`)}); err == nil {
		t.Fatal("批量溢出未拒绝")
	}
	stored, err = accounts.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	p := stored.Avatars[0].Progress
	if err != nil || p.Achievements[101001].Claimed || p.Materials[11].Count != math.MaxInt64-50 {
		t.Fatal("批量领奖未全部回滚", err)
	}
}
