package game

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestRandomCardsUsesAndroidCatalogAndPersists(t *testing.T) {
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	ctx := context.Background()
	c, _ := newBattleConnection(t, ctx, accounts, service)
	pushes, err := service.Handle(ctx, c, "random_cards", []json.RawMessage{json.RawMessage(`7`), json.RawMessage(`1`), json.RawMessage(`1`)})
	if err != nil || businessPushCount(pushes) != 6 {
		t.Fatalf("抽卡失败: err=%v pushes=%v", err, pushes)
	}
	av, ok := c.SelectedAvatar()
	if !ok || len(av.Progress.Cards) < 2 {
		t.Fatalf("抽卡未生成卡牌: %#v", av.Progress.Cards)
	}
	if av.Progress.RandomCardsRecord[1].RandomCount != 1 {
		t.Fatalf("抽卡计数错误: %#v", av.Progress.RandomCardsRecord)
	}
	commonChange := pushes[1].Args[0].([]any)
	if commonChange[0] != "card_common_mgr" || pushes[2].Args[0].([]any)[0] != "card_mgr" {
		t.Fatal("抽卡新共享状态必须在原生幻书列表刷新前下发")
	}
	common := commonChange[1].(map[string]any)
	for _, card := range av.Progress.Cards {
		if common[fmt.Sprint(card.CardID)] == nil {
			t.Fatal("抽卡后新幻书共享状态缺失", card.CardID)
		}
	}
	row := av.InitialProperties("")["random_cards_record"].(map[int]any)[1].(map[string]any)
	if row["random_id"] != 1 || row["random_count"] != 1 || row["pool_id"] != nil {
		t.Fatal("抽卡冷登录投影仍使用内部字段", row)
	}
	callback := pushes[len(pushes)-1]
	values := callback.Args[1].([]any)
	cards := values[1].([]any)
	if len(cards) != 3 || cards[1] != "card.card_list" || cards[2] != "__custom_type" {
		t.Fatal("抽卡回调缺原生幻书类型", cards)
	}
}

func TestSetAssistCardPersistsOwnedUUID(t *testing.T) {
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	ctx := context.Background()
	c, av := newBattleConnection(t, ctx, accounts, service)
	card := newCard(4401, 1, time.Unix(1700000000, 0))
	if _, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error { p.Cards = append(p.Cards, card); return nil }); err != nil {
		t.Fatal(err)
	}
	for i := range c.identity.Avatars {
		if c.identity.Avatars[i].Hostnum == 1 {
			c.identity.Avatars[i].Progress.Cards = append(c.identity.Avatars[i].Progress.Cards, card)
		}
	}
	if _, err := service.Handle(ctx, c, "set_assist_card", []json.RawMessage{json.RawMessage(`7`), json.RawMessage(`"` + card.UUID + `"`)}); err != nil {
		t.Fatal(err)
	}
	got, ok := c.SelectedAvatar()
	if !ok || got.Progress.AssistCardUUID != card.UUID {
		t.Fatalf("助战卡未持久化: %#v", got.Progress.AssistCardUUID)
	}
}
