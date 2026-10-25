package dbstore

import (
	"context"
	"hs-server/internal/game"
	"testing"
	"time"
)

func TestPostgresMikuEntranceBeforeMapAndColdPersistence(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
	info := game.ClientInfo{Account: "初音首次地图剧情", Password: "pw", Hostnum: 1}
	s, c, av := repairPlayer(t, store, info, &now)
	defer s.Detach(c)
	if err := game.SetActivitySchedules(game.DefaultPermanentActivitySchedules(now.Add(-24 * time.Hour))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = game.SetActivitySchedules(nil) })
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.GuideTasks = map[int]game.GuideTask{}
		p.ClearedDungeons = append(p.ClearedDungeons, 601)
		p.Power.Value = 100
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out := pgRemainingRPC(t, s, c, "enter_dungeon", 71, 20831002, map[string]any{})
	if len(out) == 0 {
		t.Fatal("原版地图前置剧情没有返回")
	}
	identity, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := identity.Avatars[0].Progress
	if p.Activities.Miku == nil || p.Activities.Miku.Maps[1] == nil {
		t.Fatal("首次入场地图未冷持久")
	}
	// 新Store读取后重复进入同一入口不能生成第二份地图或清空已得探索。
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error { p.Activities.Miku.Maps[1].Score = 12; return nil }); err != nil {
		t.Fatal(err)
	}
	pgRemainingRPC(t, s, c, "enter_dungeon", 72, 20831002, map[string]any{})
	identity, err = New(store.pool).QuickLogin(ctx, info)
	if err != nil || identity.Avatars[0].Progress.Activities.Miku.Maps[1].Score != 12 {
		t.Fatal("重复入场重置地图", err)
	}
}
