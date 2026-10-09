package game

import (
	"context"
	"encoding/hex"
	"testing"
	"time"
)

func TestNewAvatarAllHeroesSwitchAndLogin(t *testing.T) {
	now := time.Unix(1791504000, 0)
	for _, flag := range []string{"", "0", "false", "无效"} {
		t.Setenv("HS_NEW_AVATAR_ALL_HEROES", flag)
		if p := NewAvatarProgress(1, now); len(p.Cards) != 1 || p.Cards[0].CardID != 4401 {
			t.Fatal("关闭或无效开关改变原初始卡")
		}
	}
	t.Setenv("HS_NEW_AVATAR_ALL_HEROES", "1")
	p := NewAvatarProgress(1, now)
	if len(p.Cards) != len(androidProfile.CountedCards) || len(p.ObtainedCardIDs) != len(p.Cards) {
		t.Fatal("英雄或获得记录数量错误", len(p.Cards), len(p.ObtainedCardIDs))
	}
	ids, uuids := map[int]bool{}, map[string]bool{}
	for _, card := range p.Cards {
		raw, err := hex.DecodeString(card.UUID)
		if err != nil || len(raw) != 12 || uuids[card.UUID] || ids[card.CardID] || !containsInt(androidProfile.CountedCards, card.CardID) || card.Level != 1 || card.Grade != 0 {
			t.Fatal("英雄重复、禁用或初始属性无效", card)
		}
		ids[card.CardID], uuids[card.UUID] = true, true
		if card.Dress <= 0 || !containsInt(androidCardAppearances[card.CardID].Dresses, card.Dress) {
			t.Fatal("全英雄默认装帧缺失或不属于本卡", card)
		}
	}
	props := (Avatar{Progress: p}).InitialProperties("测试账号")
	if len(props["card_mgr"].(map[string]any)) != len(p.Cards) || len(props["card_common_mgr"].(map[string]any)) != len(p.Cards) {
		t.Fatal("登录英雄或共有属性缺失")
	}
	if len(p.UnlockSystems) != 0 || len(p.ClearedDungeons) != 0 || p.GuideTasks[1000].Status != 1 || len(p.RandomCardsRecord) != 0 {
		t.Fatal("测试赠送篡改教学、解锁或抽卡记录")
	}
	accounts := NewFixtureAccounts(nil)
	info := ClientInfo{Account: "全英雄测试", Password: "测试密码", Hostnum: 10001}
	first, err := accounts.Register(context.Background(), info)
	if err != nil || len(first.Avatars[0].Progress.Cards) != len(p.Cards) {
		t.Fatal("实际建角没有应用开关", err)
	}
	t.Setenv("HS_NEW_AVATAR_ALL_HEROES", "0")
	again, err := accounts.QuickLogin(context.Background(), info)
	if err != nil || len(again.Avatars[0].Progress.Cards) != len(p.Cards) || again.Avatars[0].Progress.Cards[1].UUID != first.Avatars[0].Progress.Cards[1].UUID {
		t.Fatal("关开关后重登修改已赠送英雄", err)
	}
	info.Hostnum++
	other, err := accounts.QuickLogin(context.Background(), info)
	if err != nil || len(other.Avatars[1].Progress.Cards) != 1 {
		t.Fatal("关闭开关后新服建角仍赠送", err)
	}
}
