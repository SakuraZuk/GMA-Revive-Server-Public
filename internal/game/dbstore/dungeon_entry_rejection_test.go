package dbstore

import (
	"context"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestPostgresDungeonEntryRejectCallbackNoChargeAndColdPersistence(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)
	info := game.ClientInfo{Account: "拒绝回包", Password: "pw", Hostnum: 1}
	s, c, av := repairPlayer(t, store, info, &now)
	defer s.Detach(c)
	if _, err := store.pool.Exec(ctx, `UPDATE avatars SET level=60 WHERE avatar_oid=$1`, av.OID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.AvatarLevel = 60
		p.GuideTasks = map[int]game.GuideTask{}
		p.ClearedDungeons = append(p.ClearedDungeons, 509)
		p.Power.Value = 0
		p.Power.LastTime = float64(now.Unix())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	read := func() game.Progress {
		p, err := New(store.pool).UpdateProgress(ctx, av.OID, func(*game.Progress) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		return game.CloneProgress(p)
	}
	before := read()
	for _, pair := range [][2]int{{6101, 1005}, {999999999, 1019}} {
		out, err := s.Handle(ctx, c, "enter_dungeon", pgSocialArgs(91, pair[0], map[string]any{}))
		if err != nil || len(out) != 1 || out[0].Method != "call_client_callback" || !reflect.DeepEqual(out[0].Args, []any{91, []any{pair[1]}}) {
			t.Fatal("原版拒绝回包不完整", err, out)
		}
		if !reflect.DeepEqual(before, read()) {
			t.Fatal("拒绝入口扣费或创建战斗，独立Store冷读不一致")
		}
	}
}
