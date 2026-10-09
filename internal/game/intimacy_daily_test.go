package game

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestSpecialGiftDailyCrossCardCapResetAndRuneBox(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	now := time.Date(2026, 10, 7, 15, 59, 59, 0, time.UTC) // 北京时间午夜前一秒。
	s.Now = func() time.Time { return now }
	c, av := newBattleConnection(t, ctx, accounts, s)
	rules, _ := loadIntimacyCatalog()
	limit := rules.SpecialGiftRules[1]
	_, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{newCard(4401, 1, now), newCard(4402, 1, now)}
		p.Materials[523] = Material{ID: 523, Count: 10, Total: 10}
		p.SpecialGiftDay = "2026-10-07"
		p.SpecialGiftCounts = map[int]int{4402: limit.TotalTimes - 1}
		p.IntimacyCommons = map[int]IntimacyCommon{4401: {SpecialCount: 100}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`4401`), json.RawMessage(`523`), json.RawMessage(`1`), json.RawMessage(`3`)}
	pushes, err := s.Handle(ctx, c, "consume_intimacy_gift", args)
	if err != nil || businessPushCount(pushes) != 6 {
		t.Fatal(err, pushes)
	}
	box := pushes[len(pushes)-1].Args[1].([]any)[1].(map[string]any)
	if len(box["runes"].(map[string]any)) != 4 {
		t.Fatal("特殊回礼未随box返回四枚契印")
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.IntimacyCommons[4401].SpecialCount != 101 || p.SpecialGiftCounts[4401] != 1 {
		t.Fatal("累计序号与当日次数混用")
	}
	before := p.Materials[523].Count
	pushes, err = s.Handle(ctx, c, "consume_intimacy_gift", args)
	if err != nil || businessPushCount(pushes) != 1 || c.SelectedAvatarUnsafe().Progress.Materials[523].Count != before {
		t.Fatal("跨卡总次数限制未整体拒绝", err)
	}
	now = now.Add(time.Second)
	pushes, err = s.Handle(ctx, c, "consume_intimacy_gift", args)
	if err != nil || businessPushCount(pushes) != 6 {
		t.Fatal("跨日未恢复交流次数", err)
	}
	p = c.SelectedAvatarUnsafe().Progress
	if p.SpecialGiftCounts[4402] != 0 || p.IntimacyCommons[4401].SpecialCount != 102 || p.SpecialGiftDay != "2026-10-08" {
		t.Fatal("每日计数重置误删累计对话进度")
	}
}
