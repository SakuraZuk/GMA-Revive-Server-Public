package game

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestCardGrowthLevelAndEnhanceAreTransactional(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, service)
	target := newCard(4401, 1, time.Unix(1700000000, 0))
	material := newCard(4401, 1, time.Unix(1700000001, 0))
	if _, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{target, material}
		p.Materials[4] = Material{ID: 4, Count: 1000, Total: 1000}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pushes, err := service.Handle(ctx, c, "up_level_card", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`"` + target.UUID + `"`), json.RawMessage(`false`)})
	if err != nil || businessPushCount(pushes) != 4 {
		t.Fatalf("升级失败: %v %#v", err, pushes)
	}
	got, ok := c.SelectedAvatar()
	if !ok || got.Progress.Cards[0].Level != 2 || got.Progress.Materials[4].Count >= 1000 {
		t.Fatalf("升级未持久化: %#v", got.Progress)
	}
	pushes, err = service.Handle(ctx, c, "card_enhance", []json.RawMessage{json.RawMessage(`2`), json.RawMessage(`"` + target.UUID + `"`), json.RawMessage(`[` + `"` + material.UUID + `"` + `]`)})
	if err != nil || businessPushCount(pushes) != 5 {
		t.Fatalf("补完失败: %v %#v", err, pushes)
	}
	got, _ = c.SelectedAvatar()
	if len(got.Progress.Cards) != 1 || got.Progress.Cards[0].EnhanceCount != 1 {
		t.Fatalf("补完删除或计数错误: %#v", got.Progress.Cards)
	}
}

func TestCardGrowthRejectsDuplicateMaterialWithoutMutation(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, service)
	target := newCard(4401, 1, time.Unix(1700000000, 0))
	material := newCard(4401, 1, time.Unix(1700000001, 0))
	_, _ = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error { p.Cards = []Card{target, material}; return nil })
	pushes, err := service.Handle(ctx, c, "card_enhance", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`"` + target.UUID + `"`), json.RawMessage(`[` + `"` + material.UUID + `","` + material.UUID + `"` + `]`)})
	if err != nil || pushes[len(pushes)-1].Args[1].([]any)[0] == 0 {
		t.Fatal("重复补完应拒绝")
	}
	stored, err := accounts.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil || len(stored.Avatars[0].Progress.Cards) != 2 {
		t.Fatalf("拒绝请求修改了卡牌: %#v %v", stored.Avatars[0].Progress.Cards, err)
	}
}
