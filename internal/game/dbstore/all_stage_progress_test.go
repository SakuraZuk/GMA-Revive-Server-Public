package dbstore

import (
	"context"
	"hs-server/internal/game"
	"testing"
	"time"
)

// 2-3纯剧情、后继战斗重复入口和带助战解锁的冷登录在同一个真实存档中核验。
func TestPostgresStage23StorySyncBattleDuplicateAndColdLogin(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.FixedZone("北京时间", 28800))
	info := game.ClientInfo{Account: "全关卡共用链实库", Password: "pw", Hostnum: 1}
	s, c, av := repairPlayer(t, store, info, &now)
	defer s.Detach(c)
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.GuideTasks = map[int]game.GuideTask{}
		p.AvatarLevel = 60
		p.Power.Value = 9998
		p.UnlockSystems["assist"] = 1
		p.PlayedStorylines = []string{"story/01ud/mainstory/ud_2-2-2"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for repeat := 0; repeat < 2; repeat++ {
		out := pgRemainingRPC(t, s, c, "enter_dungeon", 301+repeat, 603, map[string]any{})
		progressIndex, resultIndex := -1, -1
		for i, v := range out {
			if v.Method == "client_prop_set" {
				path, ok := v.Args[0].([]any)
				if ok && len(path) == 3 && path[0] == "dungeon_mgr" && path[1] == 603 && path[2].(map[string]any)["finished"] == 1 {
					progressIndex = i
				}
			}
			if v.Method == "battle_result" {
				resultIndex = i
			}
		}
		if progressIndex < 0 || resultIndex <= progressIndex {
			t.Fatal("实库剧情完成回包未先同步通关", out)
		}
	}
	pgRemainingRPC(t, s, c, "enter_dungeon", 303, 604, map[string]any{})
	read := func() game.Progress {
		id, err := New(store.pool).QuickLogin(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		return id.Avatars[0].Progress
	}
	before := read()
	out := pgRemainingRPC(t, s, c, "enter_dungeon", 304, 604, map[string]any{})
	after := read()
	if len(out) != 1 || out[0].Method != "call_client_callback" || after.Battle.UUID != before.Battle.UUID || after.Power.Value != before.Power.Value {
		t.Fatal("实库重复入口重建或重扣", out)
	}
	pgRemainingRPC(t, s, c, "exit_battle")
	coldService := pgActivityService(t, New(store.pool), func() time.Time { return now })
	cold := game.NewConnection()
	defer coldService.Detach(cold)
	pgRemainingRPC(t, coldService, cold, "quick_login", info)
	if _, err := coldService.BecomePlayer(cold); err != nil {
		t.Fatal(err)
	}
	out = pgRemainingRPC(t, coldService, cold, "set_reconnect_auth_msg", "全关卡冷登录")
	if len(out) < 2 || out[0].Method != "client_prop_set" || out[len(out)-1].Method != "on_refresh_login" {
		t.Fatal("助战刷新覆盖剧情桶", out)
	}
	id, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	props := id.Avatars[0].InitialProperties(info.Account)
	if props["dungeon_mgr"].(map[string]any)["603"].(map[string]any)["finished"] != 1 {
		t.Fatal("2-3通关冷登录丢失")
	}
}
