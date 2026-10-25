package dbstore

import (
	"context"
	"hs-server/internal/game"
	"testing"
	"time"
)

func TestPostgresQQBugVoiceAndAwakenColdPersistence(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.FixedZone("北京时间", 28800))
	info := game.ClientInfo{Account: "腾讯表配音装帧实库", Password: "pw", Hostnum: 1}
	s, c, av := repairPlayer(t, store, info, &now)
	defer s.Detach(c)
	uuid := "00112233445566778899aabb"
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Cards = []game.Card{{UUID: uuid, CardID: 4401, Level: 10, Grade: 0, Dress: 440101}}
		p.Materials[100] = game.Material{ID: 100, Count: 5, Total: 5}
		p.Materials[150] = game.Material{ID: 150, Count: 20, Total: 20}
		p.Materials[12] = game.Material{ID: 12, Count: 24000, Total: 24000}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	read := func() game.Avatar {
		t.Helper()
		identity, err := New(store.pool).QuickLogin(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		return identity.Avatars[0]
	}
	out := pgRemainingRPC(t, s, c, "upgrade_card", 881, uuid)
	cb := pgRemainingPush(out, "call_client_callback")
	if cb == nil || cb.Args[1].([]any)[0] != 0 {
		t.Fatal("实库升格失败", out)
	}
	first := read()
	if first.Progress.Cards[0].Awakened != 1 || first.Progress.Cards[0].Grade != 1 || first.Progress.Materials[12].Count != 0 {
		t.Fatal("升格解锁未原子持久化")
	}
	props := first.InitialProperties("")
	common := props["card_common_mgr"].(map[string]any)["4401"].(map[string]any)
	owned := false
	for _, id := range common["owned_dresses"].([]int) {
		owned = owned || id == 440102
	}
	if !owned {
		t.Fatal("冷登录缺少原生觉醒装帧")
	}
	pgRemainingRPC(t, s, c, "set_card_vo", 882, 4401, 2)
	pgRemainingRPC(t, s, c, "set_global_vo", 883, 1)
	pgRemainingRPC(t, s, c, "set_card_vo", 884, 4401, 2)
	pgRemainingRPC(t, s, c, "set_global_vo", 885, 0)
	cold := read()
	props = cold.InitialProperties("")
	common = props["card_common_mgr"].(map[string]any)["4401"].(map[string]any)
	if cold.Progress.GlobalSetVO != 1 || cold.Progress.GlobalVO != 0 || props["global_set_vo"] != 1 || common["card_vo"] != 2 {
		t.Fatal("全局中配与单卡日配未正确持久化")
	}
	pgRemainingRPC(t, s, c, "upgrade_card", 886, uuid)
	after := read()
	if after.Progress.Cards[0].Grade != 1 || after.Progress.Materials[12].Count != 0 {
		t.Fatal("重复升格绕过等级或费用条件")
	}
}
