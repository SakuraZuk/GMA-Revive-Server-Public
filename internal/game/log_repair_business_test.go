package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestLogRepairPowerMaterialTransactionLimitsAndColdReload(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, av := newBattleConnection(t, ctx, a, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.Power.Value = 100
		p.Materials[902] = Material{ID: 902, Count: 4, Total: 4}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out, err := s.Handle(ctx, c, "consume_power_material", rawArgs(9, 902, 2))
	if err != nil || len(out) != 3 || !reflect.DeepEqual(out[2].Args[1], []any{int64(100), ""}) {
		t.Fatal("原生两参回调无效", err, out)
	}
	id, err := a.QuickLogin(ctx, ClientInfo{Account: av.Account, Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := id.Avatars[0].Progress
	if p.Power.Value != 200 || p.Materials[902].Count != 2 || p.Materials[902].Total != 4 {
		t.Fatal("扣料加灵感未一起持久", p.Power, p.Materials[902])
	}
	for _, args := range [][]any{{10, 902, 3}, {11, 902, -1}, {12, 100, 1}, {13, 902, int64(9223372036854775807)}} {
		before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
		out, err = s.Handle(ctx, c, "consume_power_material", rawArgs(args...))
		if err != nil || len(out) != 1 || len(out[0].Args[1].([]any)) != 2 || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
			t.Fatal("非法消耗没有原子拒绝", args, err, out)
		}
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error { p.Power.Value = 9949; return nil }); err != nil {
		t.Fatal(err)
	}
	out, err = s.Handle(ctx, c, "consume_power_material", rawArgs(14, 902, 1))
	if err != nil || len(out) != 1 || c.SelectedAvatarUnsafe().Progress.Power.Value != 9949 || c.SelectedAvatarUnsafe().Progress.Materials[902].Count != 2 {
		t.Fatal("原生等于9999边界没有拒绝", err, out)
	}
}

func TestLogRepairNativeIntegerSwitchCardLockAndOrdinaryExitIdempotency(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	for _, value := range []any{0, 1, false, true, 2, "1", nil} {
		out, err := s.Handle(ctx, c, "change_dungeon_skip_edit_state", rawArgs(20, 4202, value))
		if err != nil || len(out) != 1 {
			t.Fatal("跳过开关回调未释放", value, err, out)
		}
		want := value == 0 || value == 1 || value == false || value == true
		if out[0].Args[1].([]any)[0] != want {
			t.Fatal("跳过开关接受了非原生值", value, out)
		}
	}
	uuid := c.SelectedAvatarUnsafe().Progress.Cards[0].UUID
	for _, method := range []string{"lock_card", "unlock_card", "lock_card"} {
		out, err := s.Handle(ctx, c, method, rawArgs(21, uuid))
		if err != nil || out[len(out)-1].Args[1].([]any)[0] != RetSuccess {
			t.Fatal(method, err, out)
		}
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	out, err := s.Handle(ctx, c, "decompose_cards", rawArgs(22, []string{uuid}))
	if err != nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("锁定幻书归还仍改资产", err, out)
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.Power.Value = 100
		p.Battle = &BattleSession{UUID: "00112233445566778899aabb", DungeonID: 4202, BattleID: 4202, PaidPower: 10, Started: true, Status: "战斗"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for repeat := 0; repeat < 3; repeat++ {
		if repeat == 0 {
			c.pendingDungeonArgs = rawArgs(25, 4102, map[string]any{})
			c.pendingDungeonBoxCallback = true
		}
		out, err = s.Handle(ctx, c, "exit_battle", nil)
		if err != nil {
			t.Fatal("普通退出仍走真人房间", err, out)
		}
		exitIndex, resultIndex := -1, -1
		for i, v := range out {
			if v.Method == "exit_battle_ok" {
				exitIndex = i
			}
			if v.Method == "battle_result" {
				resultIndex = i
				if v.Args[0] != false {
					t.Fatal("退出结果伪造胜利", v)
				}
			}
		}
		if exitIndex < 0 || repeat == 0 && resultIndex <= exitIndex || repeat > 0 && resultIndex >= 0 {
			t.Fatal("退出确认后的结果等待没有释放或重复弹窗", repeat, out)
		}
		if repeat == 0 {
			found := false
			for _, v := range out {
				if v.Method == "call_client_callback" && v.Args[0] == 25 {
					found = reflect.DeepEqual(v.Args[1], []any{1, nil})
				}
			}
			if !found {
				t.Fatal("退出清除排队入口但没有释放夏日回调")
			}
		}
		p := c.SelectedAvatarUnsafe().Progress
		if !p.Battle.Finished || p.Battle.Outcome != "loss" || p.Power.Value != 109 || containsInt(p.ClearedDungeons, 4202) {
			t.Fatal("退出伪造通关或重复返还", p.Battle, p.Power)
		}
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error { p.Battle.Finished = false; p.Battle.NativeSolo = &HumanPvpRoom{}; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Handle(ctx, c, "exit_battle", nil); err == nil {
		t.Fatal("单端竞技错误按普通失败处理")
	}
	var enabled bool
	if nativeBinarySwitch(json.RawMessage("null"), &enabled) {
		t.Fatal("null被当作开关")
	}
}
