package dbstore

import (
	"context"
	"encoding/hex"
	"hs-server/internal/game"
	"reflect"
	"testing"
	"time"
)

func TestPostgresEquipmentDailyDoubleReceiptColdRetryAndStage210Slots(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.FixedZone("北京时间", 28800))
	info := game.ClientInfo{Account: "装备双倍实库", Password: "pw", Hostnum: 1}
	s, c, av := repairPlayer(t, store, info, &now)
	defer s.Detach(c)
	if _, err := store.pool.Exec(ctx, `UPDATE avatars SET level=60 WHERE avatar_oid=$1`, av.OID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.AvatarLevel = 60
		p.GuideTasks = map[int]game.GuideTask{}
		p.Power.Value = 100
		p.ClearedDungeons = append(p.ClearedDungeons, 509)
		p.Materials[29] = game.Material{ID: 29, Count: 100, Total: 100}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	read := func() game.Progress {
		id, err := New(store.pool).QuickLogin(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		return id.Avatars[0].Progress
	}
	before := read()
	pgRemainingRPC(t, s, c, "set_dungeon_bonus_double", 1, 6101, true)
	pgRemainingRPC(t, s, c, "enter_dungeon", 2, 6101, map[string]any{})
	pgRemainingRPC(t, s, c, "load_entity_finish")
	pgRemainingRPC(t, s, c, "battle_fighting", map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})
	p := read()
	if p.Power.Value != 80 || p.Materials[29].Count != 80 || !p.DungeonBonusDouble[6101] {
		t.Fatal("双倍费用或选择未实库提交")
	}
	uuid := p.Battle.UUID
	emit := func(seq int, kind string, data map[string]any) {
		pgRemainingRPC(t, s, c, "do_command", "__battle_event__", []any{map[string]any{"battle_uuid": uuid, "sequence": seq, "kind": kind, "data": data}})
	}
	emit(1, "ready", map[string]any{"version": 1})
	emit(2, "started", map[string]any{"units": []any{map[string]any{"eid": "1", "role": 1, "hex": []int{0, 0, 0}, "kind": "ally"}}})
	emit(3, "result", map[string]any{"winner_eids": []string{hex.EncodeToString(av.OID)}})
	p = read()
	if len(p.Runes) != len(before.Runes)+8 || p.Battle.SettlementBox["runes"] == nil || !p.Battle.RewardGranted {
		t.Fatal("星期五双倍八件契印或完整收据未持久", len(p.Runes), len(before.Runes))
	}
	emit(3, "result", map[string]any{"winner_eids": []string{hex.EncodeToString(av.OID)}})
	if !reflect.DeepEqual(p.Runes, read().Runes) {
		t.Fatal("结果重试重复发奖")
	}
	coldSvc, cold := loginPersistedProgressPlayer(t, store, info, now)
	defer coldSvc.Detach(cold)
	pgRemainingRPC(t, coldSvc, cold, "client_need_recover_battle")
	if !reflect.DeepEqual(p.Runes, read().Runes) || read().Materials[29].Count != 80 {
		t.Fatal("冷登录改写契印或重复扣券")
	}
	pgRemainingRPC(t, coldSvc, cold, "enter_dungeon", 3, 610, map[string]any{})
	pgRemainingRPC(t, coldSvc, cold, "load_entity_finish")
	fight := []any{read().Cards[0].UUID, 0, 0, 0, 0}
	pgRemainingRPC(t, coldSvc, cold, "battle_fighting", map[string]any{"fighting_cards": fight, "support_cards": []int{0, 0}, "storyline_cards": []int{}})
	if !read().Battle.Started || len(read().Battle.Layout.Fighting) != 5 || len(read().Battle.Team) != 1 {
		t.Fatal("2-10五槽开战未实库持久")
	}
}
