package game

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestQueuedDungeonFailureReleasesCallbackAfterCommittedResult(t *testing.T) {
	for _, box := range []bool{false, true} {
		t.Run(fmt.Sprint("双参回调", box), func(t *testing.T) {
			ctx := context.Background()
			a := NewFixtureAccounts(nil)
			s := New(a, nil)
			c, av := newBattleConnection(t, ctx, a, s)
			uuid := startObservedBattle(t, ctx, s, c)
			if err := s.updateProgress(ctx, c, func(p *Progress) error {
				p.GuideTasks = map[int]GuideTask{1007: {ID: 1007, Status: 1}}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if out, err := s.Handle(ctx, c, "enter_dungeon", rawArgs(77, 10002, map[string]any{})); err != nil || len(out) != 0 {
				t.Fatal("未进入等待结算队列", err, out)
			}
			c.pendingDungeonBoxCallback = box
			// 排队时合法，结算期间资格变化；下一入口拒绝不能吞掉当前结果。
			if err := s.updateProgress(ctx, c, func(p *Progress) error {
				p.GuideTasks = map[int]GuideTask{1001: {ID: 1001, Status: 1}}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			result := fmt.Sprintf(`{"battle_uuid":%q,"sequence":3,"kind":"result","data":{"winner_eids":["%x"],"finished_task_list":[]}}`, uuid, av.OID)
			out := observeBattleEvent(t, ctx, s, c, result)
			values := []any{androidShop.Errors["RET_DUNGEON_LOCKED"]}
			if box {
				values = append(values, nil)
			}
			count := 0
			for _, item := range out {
				if item.Method == "call_client_callback" && reflect.DeepEqual(item.Args, []any{77, values}) {
					count++
				}
				if item.Method == "start_server_battle_ok" {
					t.Fatal("拒绝后仍建立下一战斗")
				}
			}
			b := c.SelectedAvatarUnsafe().Progress.Battle
			if count != 1 || battleTestResult(out) == nil || b.UUID != uuid || !b.Finished || !b.RewardGranted || len(c.pendingDungeonArgs) != 0 || c.pendingDungeonBoxCallback {
				t.Fatal("结果或失败回调丢失，或留下失效队列", out, b)
			}
			before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
			if out := observeBattleEvent(t, ctx, s, c, result); len(out) != 0 || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
				t.Fatal("重复结果再次释放回调或发奖", out)
			}
		})
	}
}

func TestDungeonEntryRejectionCallbackAndAtomicAssets(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		id      int
		code    string
		prepare func(*Progress)
	}{
		{"未知入口", 999999999, "RET_DUNGEON_DATA_ERROR", func(*Progress) {}},
		{"体力不足", 6101, "RET_DUNGEON_POWER_NOT_ENOUGH", func(p *Progress) {
			p.GuideTasks = map[int]GuideTask{}
			p.AvatarLevel = 60
			for id := range dungeonCatalog {
				p.ClearedDungeons = append(p.ClearedDungeons, id)
			}
			p.Power.Value = 0
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NewFixtureAccounts(nil)
			s := New(a, nil)
			now := time.Now()
			s.Now = func() time.Time { return now }
			c, _ := newBattleConnection(t, ctx, a, s)
			if err := s.updateProgress(ctx, c, func(p *Progress) error {
				tc.prepare(p)
				p.Power.LastTime = float64(now.UnixNano()) / 1e9
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
			out, err := s.Handle(ctx, c, "enter_dungeon", rawArgs(77, tc.id, map[string]any{}))
			if err != nil || len(out) != 1 || out[0].Method != "call_client_callback" ||
				!reflect.DeepEqual(out[0].Args, []any{77, []any{androidShop.Errors[tc.code]}}) {
				t.Fatal("未按原版单ret回调结束拒绝入口", err, out)
			}
			if !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
				t.Fatal("拒绝入口改写资产或战斗会话")
			}
		})
	}
}

func TestDungeonEntryMalformedRequestKeepsProtocolError(t *testing.T) {
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, context.Background(), a, s)
	out, err := s.Handle(context.Background(), c, "enter_dungeon", []json.RawMessage{json.RawMessage(`"bad"`), json.RawMessage(`6101`), json.RawMessage(`{}`)})
	if err == nil || len(out) != 0 {
		t.Fatal("无效请求被伪装为业务回调", err, out)
	}
	if err := ordinaryDungeonConditions(activityRow{"unlock_condition": json.RawMessage(`{}`)}, c.SelectedAvatarUnsafe().Progress, s.Now()); err == nil {
		t.Fatal("损坏条件规则被伪装为正常拒绝")
	}
}

func TestDungeonEntryStorageFailureKeepsErrorAndAssets(t *testing.T) {
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, context.Background(), a, s)
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	s.Accounts = basicFailStore{a}
	out, err := s.Handle(context.Background(), c, "enter_dungeon", rawArgs(78, 6101, map[string]any{}))
	if err == nil || len(out) != 0 || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("数据库失败被改写为业务拒绝或修改资产", err, out)
	}
}
