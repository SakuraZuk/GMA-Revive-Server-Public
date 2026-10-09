package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestLegacyZeroDressProjectsNativeDefaultWithoutChangingAssets(t *testing.T) {
	card := newCard(4401, 1, time.Now())
	card.Dress = 0
	entry := cardMgrProperties([]Card{card})[card.UUID].(map[string]any)
	if entry["dress"] != androidCardAppearances[4401].DefaultDress || card.Dress != 0 {
		t.Fatal("旧0装帧必须只投影默认外观，不覆盖存档")
	}
}

func TestCaptainNativeArgumentsPersistenceAndLegacyLogin(t *testing.T) {
	// 本用例仅构造一张旧装帧卡；全英雄初始化由独立用例验证。
	t.Setenv("HS_NEW_AVATAR_ALL_HEROES", "0")
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	stored, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{newCard(4401, 1, time.Now())}
		p.CaptainCardUUID = p.Cards[0].UUID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy := (Avatar{Progress: stored}).InitialProperties("测试账号")
	if legacy["captain_id"] != 4401 {
		t.Fatal("旧UUID队长未转换", legacy["captain_id"])
	}
	for _, field := range []string{"captain_card_uuid", "captain_dresses", "owned_dresses"} {
		if _, exists := legacy[field]; exists {
			t.Fatal("内部外观字段泄漏", field)
		}
	}
	args := []json.RawMessage{json.RawMessage(`4401`), json.RawMessage(`null`)}
	pushes, err := s.Handle(ctx, c, "set_captain_card_id", args)
	if err != nil || len(pushes) != 2 {
		t.Fatalf("原生队长调用失败: %v %#v", err, pushes)
	}
	if pushes[0].Method != "client_prop_changed" || pushes[1].Method != "client_prop_changed" || !reflect.DeepEqual(pushes[1].Args, []any{[]any{"captain_id", 4401}}) || pushes[0].Args[0].([]any)[0] != "card_common_mgr" {
		t.Fatal("队长属性推送错误或插入了回调", pushes)
	}
	reloaded, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if reloaded.CaptainID != 4401 || reloaded.CaptainCardUUID != "" || reloaded.CaptainDresses[4401] != androidCardAppearances[4401].DefaultDress || !containsInt(reloaded.OwnedDresses[4401], androidCardAppearances[4401].DefaultDress) {
		t.Fatal("队长持久字段错误", reloaded.CaptainID, reloaded.CaptainDresses)
	}
	common := (Avatar{Progress: reloaded}).InitialProperties("测试账号")["card_common_mgr"].(map[string]any)["4401"].(map[string]any)
	if common["captain_dress"] != androidCardAppearances[4401].DefaultDress || len(common["owned_dresses"].([]int)) == 0 {
		t.Fatal("登录缺外观状态", common)
	}
}

func TestCaptainRejectsInvalidOwnershipAndRollsBack(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	before, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{newCard(4401, 1, time.Now()), newCard(4, 1, time.Now())}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]json.RawMessage{
		{json.RawMessage(`3202`), json.RawMessage(`null`)},
		{json.RawMessage(`4`), json.RawMessage(`null`)},
		{json.RawMessage(`4401`), json.RawMessage(`401`)},
		{json.RawMessage(`4401`), json.RawMessage(`0`)},
		{json.RawMessage(`4401`), json.RawMessage(`"旧接口UUID"`)},
		{json.RawMessage(`null`), json.RawMessage(`null`)},
	} {
		if _, err := s.Handle(ctx, c, "set_captain_card_id", args); err == nil {
			t.Fatal("非法队长设置被接受", args)
		}
		after, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
		if !reflect.DeepEqual(before, after) {
			t.Fatal("队长拒绝部分提交")
		}
	}
}

func TestCaptainAwakenedAndPaidDressEntitlements(t *testing.T) {
	var cardID, awakeID, paidID int
	for id, rule := range androidCardAppearances {
		if containsInt(rule.Forbid, androidCaptainForbidden) || rule.AwakenedDress == rule.DefaultDress || !androidDressNeedsAwakened[rule.AwakenedDress] {
			continue
		}
		for _, dress := range rule.Dresses {
			if dress != rule.DefaultDress && dress != rule.AwakenedDress && !androidDressNeedsAwakened[dress] {
				cardID, awakeID, paidID = id, rule.AwakenedDress, dress
				break
			}
		}
		if cardID != 0 {
			break
		}
	}
	if cardID == 0 {
		t.Fatal("Android外观测试配置缺失")
	}
	p := Progress{Cards: []Card{{CardID: cardID}}}
	if validOwnedDress(p, cardID, awakeID) || validOwnedDress(p, cardID, paidID) {
		t.Fatal("未觉醒或付费外观被免费放行")
	}
	p.OwnedDresses = map[int][]int{cardID: {awakeID, paidID}}
	if validOwnedDress(p, cardID, awakeID) || !validOwnedDress(p, cardID, paidID) {
		t.Fatal("外观归属与觉醒条件未分离")
	}
	p.Cards[0].Awakened = 1
	if !validOwnedDress(p, cardID, awakeID) {
		t.Fatal("拥有且觉醒的外观被拒绝")
	}
	clone := CloneProgress(p)
	clone.OwnedDresses[cardID][0] = 0
	if p.OwnedDresses[cardID][0] != awakeID {
		t.Fatal("外观归属克隆共享切片")
	}
}
