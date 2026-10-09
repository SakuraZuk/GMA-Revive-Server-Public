package dbstore

import (
	"context"
	"hs-server/internal/game"
	"reflect"
	"testing"
	"time"
)

func TestPostgresLogRepairPowerLockIntegerSwitchExitColdPersistence(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 13, 0, 0, 0, time.FixedZone("北京时间", 28800))
	info := game.ClientInfo{Account: "日志修补十二实库", Password: "pw", Hostnum: 1}
	s, c, av := repairPlayer(t, store, info, &now)
	defer s.Detach(c)
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Power.Value = 100
		p.Materials[902] = game.Material{ID: 902, Count: 2, Total: 2}
		p.Battle = &game.BattleSession{UUID: "00112233445566778899aabb", DungeonID: 4202, BattleID: 4202, PaidPower: 10, Started: true, Status: "战斗"}
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
	uuid := read().Cards[0].UUID
	out := pgRemainingRPC(t, s, c, "consume_power_material", 1, 902, 1)
	if len(out[len(out)-1].Args[1].([]any)) != 2 || read().Power.Value != 150 || read().Materials[902].Count != 1 || read().Materials[902].Total != 2 {
		t.Fatal("灵感材料实库事务不完整", out)
	}
	before := read()
	pgRemainingRPC(t, s, c, "consume_power_material", 2, 902, 2)
	if !reflect.DeepEqual(before, read()) {
		t.Fatal("材料不足没有整笔回滚")
	}
	pgRemainingRPC(t, s, c, "lock_card", 3, uuid)
	if read().Cards[0].Lock != 1 {
		t.Fatal("幻书锁定未实库持久")
	}
	pgRemainingRPC(t, s, c, "unlock_card", 4, uuid)
	if read().Cards[0].Lock != 0 {
		t.Fatal("幻书解锁未实库持久")
	}
	pgRemainingRPC(t, s, c, "change_dungeon_skip_edit_state", 5, 4202, 1)
	if !read().DungeonSkipEditState[4202] {
		t.Fatal("原生整数开关未持久")
	}
	for repeat := 0; repeat < 3; repeat++ {
		out = pgRemainingRPC(t, s, c, "exit_battle")
		p := read()
		exitIndex, resultIndex := -1, -1
		for i, v := range out {
			if v.Method == "exit_battle_ok" {
				exitIndex = i
			}
			if v.Method == "battle_result" {
				resultIndex = i
			}
		}
		if exitIndex < 0 || repeat == 0 && resultIndex <= exitIndex || repeat > 0 && resultIndex >= 0 || !p.Battle.Finished || p.Battle.Outcome != "loss" || p.Power.Value != 159 {
			t.Fatal("普通退出未实库幂等返还", p.Battle, p.Power)
		}
	}
	before = read()
	pgActivityLogin(t, pgActivityService(t, store, func() time.Time { return now }), info)
	if !reflect.DeepEqual(before.Battle, read().Battle) || before.Materials[902] != read().Materials[902] || before.Power.Value != read().Power.Value {
		t.Fatal("退出后冷登录改写收据或重复资产")
	}
}
