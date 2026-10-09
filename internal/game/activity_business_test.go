package game

import (
	"context"
	"encoding/json"
	"fmt"
	"hs-server/internal/mobileproto"
	"testing"
	"time"
)

func activitySchedulesForTest(t *testing.T, now time.Time) {
	t.Helper()
	previous := configuredActivities.Load()
	t.Cleanup(func() {
		if previous != nil {
			configuredActivities.Store(previous)
		} else {
			configuredActivities.Store(map[int]ActivitySchedule{})
		}
	})
	if err := SetActivitySchedules(DefaultPermanentActivitySchedules(now.Add(-24 * time.Hour))); err != nil {
		t.Fatal(err)
	}
}

func TestActivityPermanentPhaseAndWire(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	activitySchedulesForTest(t, now.Add(-1000*24*time.Hour))
	p := NewProgress(20, now)
	p.ClearedDungeons = []int{601, 610}
	for id, want := range map[int]int{203: 21, 206: 21, 328: 14} {
		day, e := activityOpen(p, id, 20, now)
		if e != nil || day != want {
			t.Fatalf("常驻活动阶段无效 %d %d %v", id, day, e)
		}
	}
	object := ObjectID("0123456789abcdef01234567")
	wire := activityWire(map[int]any{1: map[string]any{"oid": object, "value": SummerMap{ID: 1}}}).(mobileproto.Map)
	row := wire[0].Value.(map[string]any)
	if _, ok := row["oid"].(ObjectID); !ok {
		t.Fatal("活动边界丢失ObjectID类型")
	}
	if _, ok := row["value"].(map[string]any); !ok {
		t.Fatal("活动状态未转字典")
	}
}

func TestActivityNativeRewardMerge(t *testing.T) {
	a := emptyActivityBox()
	b := emptyActivityBox()
	a["cards"] = []any{map[string]any{"uuid": "甲"}}
	b["cards"] = []any{map[string]any{"uuid": "乙"}, map[string]any{"uuid": "丙"}, map[string]any{"uuid": "丁"}}
	b["runes"] = map[string]any{"契印": map[string]any{"level": 1}}
	b["materials"].(map[int]int64)[12] = 20
	if err := mergeActivityBox(a, b); err != nil {
		t.Fatal(err)
	}
	if len(a["cards"].([]any)) != 4 || len(a["runes"].(map[string]any)) != 1 || a["materials"].(map[int]int64)[12] != 20 {
		t.Fatal("奖励拼接漏项", a)
	}
}

func TestActivitySummerActualBattleSettlementAndRecovery(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	activitySchedulesForTest(t, now)
	c, av := newBattleConnection(t, ctx, accounts, service)
	if err := service.updateProgress(ctx, c, func(p *Progress) error {
		for id, g := range p.GuideTasks {
			g.Status = 2
			p.GuideTasks[id] = g
		}
		p.ClearedDungeons = append(p.ClearedDungeons, 601, 21120011)
		p.Power.Value = 100
		m, e := openSummerMap(p, 1, 20)
		if e != nil {
			return e
		}
		m.Unlocked = append(m.Unlocked, 10203)
		m.Finished = append(m.Finished, activityData("summer_node", 10203).integer("pre_node"))
		summerSetup(m)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := c.SelectedAvatarUnsafe().Progress
	pushes, e := service.Handle(ctx, c, "summer_game_enter_node", rawArgs(9, 10203, map[string]any{}))
	if e != nil {
		t.Fatal(e)
	}
	callbackFound := false
	for _, v := range pushes {
		if v.Method == "call_client_callback" && v.Args[0] == int64(9) {
			values := v.Args[1].([]any)
			callbackFound = len(values) == 2 && values[0] == int64(RetSuccess) && values[1] == nil
		}
	}
	if !callbackFound {
		t.Fatal("夏日原生战斗入口没有ret、box两参回调", pushes)
	}
	b := c.SelectedAvatarUnsafe().Progress.Battle
	if b == nil || b.ActivityContext == nil || b.ActivityContext.NodeID != 10203 {
		t.Fatal("真实入口未冻结夏日节点", pushes, b)
	}
	uuid := b.UUID
	if _, e = service.Handle(ctx, c, "load_entity_finish", nil); e != nil {
		t.Fatal(e)
	}
	if _, e = service.Handle(ctx, c, "battle_fighting", []json.RawMessage{json.RawMessage(`{"fighting_cards":[0],"support_cards":[],"storyline_cards":[]}`)}); e != nil {
		t.Fatal(e)
	}
	observeBattleEvent(t, ctx, service, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":1,"kind":"ready","data":{"version":1}}`, uuid))
	observeBattleEvent(t, ctx, service, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":2,"kind":"started","data":{"units":[{"eid":"1","role":1,"hex":[0,0,0],"kind":"ally"}]}}`, uuid))
	pushes = observeBattleEvent(t, ctx, service, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":3,"kind":"result","data":{"winner_eids":["%x"],"finished_task_list":[]}}`, uuid, av.OID))
	p := c.SelectedAvatarUnsafe().Progress
	resultSeen := false
	for _, v := range pushes {
		if v.Method == "battle_result" {
			resultSeen = true
		}
	}
	if !resultSeen || !p.Battle.RewardGranted || !containsInt(p.Activities.Summer.Maps[1].Finished, 10203) || !containsInt(p.ClearedDungeons, 21110001) {
		t.Fatal("活动真实战斗未结算", pushes, p.Battle)
	}
	if p.AvatarExp <= before.AvatarExp && p.AvatarLevel <= before.AvatarLevel {
		t.Fatal("活动馆主经验未实际升级/累计")
	}
	snapshot := p.Materials[211001]
	if repeated := observeBattleEvent(t, ctx, service, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":4,"kind":"result","data":{"winner_eids":["%x"]}}`, uuid, av.OID)); len(repeated) != 0 {
		t.Fatal("活动重复结果仍回包")
	}
	if c.SelectedAvatarUnsafe().Progress.Materials[211001] != snapshot {
		t.Fatal("活动重复发奖")
	}
	recovered, e := service.Handle(ctx, c, "client_need_recover_battle", nil)
	recoverSeen := false
	for _, v := range recovered {
		if v.Method == "battle_result" {
			recoverSeen = true
		}
	}
	if e != nil || !recoverSeen {
		t.Fatal("活动收据重连未补发", recovered, e)
	}
}
