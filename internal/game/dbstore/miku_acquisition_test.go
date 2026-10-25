package dbstore

import (
	"context"
	"hs-server/internal/game"
	"reflect"
	"testing"
	"time"
)

func TestPostgresMikuOriginalShopFragmentsComposeAndColdPersistence(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.FixedZone("北京时间", 28800))
	info := game.ClientInfo{Account: "原表初音兑换合成", Password: "pw", Hostnum: 1}
	s, c, av := repairPlayer(t, store, info, &now)
	defer s.Detach(c)
	if err := game.SetActivitySchedules(game.DefaultPermanentActivitySchedules(now.Add(-24 * time.Hour))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = game.SetActivitySchedules(nil) })
	if _, err := store.pool.Exec(ctx, `UPDATE avatars SET level=60 WHERE avatar_oid=$1`, av.OID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.AvatarLevel = 60
		p.ClearedDungeons = append(p.ClearedDungeons, 510, 601)
		p.Materials[208095] = game.Material{ID: 208095, Count: 150000, Total: 49999}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	read := func() game.Progress {
		identity, err := New(store.pool).QuickLogin(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		return identity.Avatars[0].Progress
	}
	before := read()
	pgRemainingRPC(t, s, c, "buy_commodity", 5010009, 50, 0)
	if !reflect.DeepEqual(before, read()) {
		t.Fatal("资格不足却实库扣款发货")
	}
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		m := p.Materials[208095]
		m.Total = 150000
		p.Materials[208095] = m
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pgRemainingRPC(t, s, c, "buy_commodity", 5010009, 50, 0)
	p := read()
	if p.Materials[3411].Count != 50 || p.Materials[208095].Count != 0 || p.CommodityDetails[5010009].Total != 50 {
		t.Fatal("原表兑换未持久化实际50残页")
	}
	coldSvc, cold := loginPersistedProgressPlayer(t, store, info, now)
	defer coldSvc.Detach(cold)
	pgRemainingRPC(t, coldSvc, cold, "compose_cards", 1, []int{3411})
	p = read()
	owned := false
	for _, card := range p.Cards {
		if card.CardID == 3411 {
			owned = true
		}
	}
	if !owned || p.Materials[3411].Count != 0 || p.ObtainedCardIDs[3411] != now.Unix() {
		t.Fatal("初音真实实例/获得时间/扣料未实库同存")
	}
	before = game.CloneProgress(p)
	pgRemainingRPC(t, coldSvc, cold, "compose_cards", 2, []int{3411})
	pgRemainingRPC(t, coldSvc, cold, "buy_commodity", 5010009, 1, 0)
	if !reflect.DeepEqual(before, read()) {
		t.Fatal("合成或限购重试重复生成资产")
	}
}
