package dbstore

import (
	"context"
	"encoding/hex"
	"hs-server/internal/game"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestPostgresHighEquipmentBaseRuneStoneStagesAndColdRecovery(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 12, 18, 0, 0, 0, time.FixedZone("北京时间", 28800))
	info := game.ClientInfo{Account: "高层原石实库", Password: "pw", Hostnum: 1}
	s, c, av := repairPlayer(t, store, info, &now)
	defer s.Detach(c)
	if _, err := store.pool.Exec(ctx, `UPDATE avatars SET level=60 WHERE avatar_oid=$1`, av.OID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.AvatarLevel = 60
		p.GuideTasks = map[int]game.GuideTask{}
		p.Power.Value = 100
		p.ClearedDungeons = append(p.ClearedDungeons, 509, 6101, 6102, 6103, 6104, 6105, 6106)
		p.Materials[23] = game.Material{ID: 23, Count: 10, Total: 10}
		p.Materials[12] = game.Material{ID: 12, Count: 10000000, Total: 10000000}
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
	pgRemainingRPC(t, s, c, "enter_dungeon", 1, 6107, map[string]any{})
	pgRemainingRPC(t, s, c, "load_entity_finish")
	pgRemainingRPC(t, s, c, "battle_fighting", map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})
	p := read()
	if p.Battle == nil || p.Battle.DungeonID != 6107 {
		t.Fatal("高层实际入口未保存")
	}
	uuid := p.Battle.UUID
	emit := func(seq int, kind string, data map[string]any) {
		pgRemainingRPC(t, s, c, "do_command", "__battle_event__", []any{map[string]any{"battle_uuid": uuid, "sequence": seq, "kind": kind, "data": data}})
	}
	emit(1, "ready", map[string]any{"version": 1})
	emit(2, "started", map[string]any{"units": []any{map[string]any{"eid": "1", "role": 1, "hex": []int{0, 0, 0}, "kind": "ally"}}})
	result := map[string]any{"winner_eids": []string{hex.EncodeToString(av.OID)}}
	emit(3, "result", result)
	p = read()
	if p.RuneStone == nil || p.RuneStone.MaterialID != 40007 || p.RuneStone.Count != 1 || !p.Battle.RewardPlanFrozen || !p.Battle.RewardGranted || len(p.Runes) < 7 {
		t.Fatal("基础契印或原石/收据未实库兑现")
	}
	before := game.CloneProgress(p)
	emit(3, "result", result)
	if !reflect.DeepEqual(before, read()) {
		t.Fatal("高层结果重传重复发奖")
	}
	coldSvc, cold := loginPersistedProgressPlayer(t, store, info, now)
	defer coldSvc.Detach(cold)
	pgRemainingRPC(t, coldSvc, cold, "step_0_rune_stone", 2, 40007)
	p = read()
	if p.RuneStone.OpenCount != 1 || len(p.RuneStone.Runes) != 10 || p.Materials[23].Count != before.Materials[23].Count-1 {
		t.Fatal("原石开启资产未实库提交")
	}
	ids := []string{}
	for id := range p.RuneStone.Runes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ids = ids[:3]
	pgRemainingRPC(t, coldSvc, cold, "step_1_rune_stone", 3, ids)
	pgRemainingRPC(t, coldSvc, cold, "step_2_rune_stone", 4, ids)
	p = read()
	for _, id := range ids {
		if p.RuneStone.States[id] != 3 || !reflect.DeepEqual(p.Runes[id], p.RuneStone.Runes[id]) {
			t.Fatal("鉴定原件未持久")
		}
	}
	before = game.CloneProgress(p)
	pgRemainingRPC(t, coldSvc, cold, "step_2_rune_stone", 5, ids)
	if !reflect.DeepEqual(before, read()) {
		t.Fatal("鉴定重传重复扣费")
	}
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error { delete(p.Runes, ids[0]); return nil }); err != nil {
		t.Fatal(err)
	}
	before = read()
	pgRemainingRPC(t, coldSvc, cold, "step_2_rune_stone", 6, ids)
	if !reflect.DeepEqual(before, read()) {
		t.Fatal("已删除鉴定件被重复领取")
	}
}
