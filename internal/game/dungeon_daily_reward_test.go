package game

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestAllEquipmentDungeonDailyRewardsAndReplay(t *testing.T) {
	t.Setenv("HS_RUNE_FALLBACK_POOLS", approvedGameplayValue(t, "HS_RUNE_FALLBACK_POOLS"))
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	base := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	base.AvatarLevel = 60
	count := 0
	for id, d := range dungeonCatalog {
		if d.Type != 8 || activityData("dungeons", id).flag("_dungeon_disable") {
			continue
		}
		row := activityData("dungeons", id)
		var daily []int
		if err := json.Unmarshal(row["everyday_bonus"], &daily); err != nil || len(daily) != 7 {
			t.Fatal("装备副本缺七日奖励", id, err)
		}
		count++
		for day := 0; day < 7; day++ {
			t.Run(fmt.Sprintf("副本%d星期%d", id, day+1), func(t *testing.T) {
				// UTC周日16点已是北京时间周一，验证日界与周日索引。
				now := time.Date(2026, 10, 4, 16, 0, 0, 0, time.UTC).AddDate(0, 0, day)
				selected, err := ordinaryDailyBonus(row, now)
				if err != nil || selected != daily[day] {
					t.Fatal("原生星期索引不一致", selected, daily[day], err)
				}
				p := CloneProgress(base)
				b := &BattleSession{DungeonID: id}
				before := len(p.Runes)
				box, err := settleOrdinaryDungeonRewards(&p, b, true, now)
				if err != nil || len(p.Runes) <= before || box["runes"] == nil || !b.RewardGranted {
					t.Fatal("实际装备奖励未生成或未进入原盒", id, err, box)
				}
				stored, _ := json.Marshal(p)
				_, err = settleOrdinaryDungeonRewards(&p, b, true, now.AddDate(0, 0, 1))
				after, _ := json.Marshal(p)
				if err != nil || string(stored) != string(after) {
					t.Fatal("跨日重复结果重新发奖", err)
				}
			})
		}
	}
	if count == 0 {
		t.Fatal("没有枚举装备副本")
	}
	t.Logf("装备副本%d项七日索引已验证；基础奖励完成实际生成及跨日重试，幸运保底另补", count)
}

func TestOrdinaryDoubleCallbackCostsFrozenRewardAndColdProjection(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.GuideTasks = map[int]GuideTask{}
		p.AvatarLevel = 60
		p.Power.Value = 100
		p.ClearedDungeons = append(p.ClearedDungeons, 509, 4401)
		p.Materials[29] = Material{ID: 29, Count: 100, Total: 100}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out, err := s.Handle(ctx, c, "set_dungeon_bonus_double", rawArgs(80, 4401, true))
	if err != nil || len(out) != 2 || out[1].Args[1].([]any)[0] != 0 {
		t.Fatal("原生单整数回调缺失", err, out)
	}
	if _, err := s.Handle(ctx, c, "enter_dungeon", rawArgs(81, 4401, map[string]any{})); err != nil {
		t.Fatal(err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Battle.PaidPower != 20 || p.Battle.PaidMaterials[29] != 20 || p.Materials[29].Count != 80 || p.Power.Value != 80 {
		t.Fatal("双倍实际费用未整笔扣取", p.Power, p.Battle)
	}
	before := CloneProgress(p)
	if _, err := s.Handle(ctx, c, "enter_dungeon", rawArgs(82, 4401, map[string]any{})); err != nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("重复准备又扣费", err)
	}
	if _, err := s.Handle(ctx, c, "set_dungeon_bonus_double", rawArgs(83, 4401, false)); err != nil {
		t.Fatal(err)
	}
	p = c.SelectedAvatarUnsafe().Progress
	if remainingDungeonRewardAmount(p.Battle, true) != 2 || remainingDungeonRewardAmount(p.Battle, false) != 1 {
		t.Fatal("中途改变设置覆盖已支付倍率或失败翻倍")
	}
	if _, err := s.Handle(ctx, c, "exit_battle", nil); err != nil {
		t.Fatal(err)
	}
	p = c.SelectedAvatarUnsafe().Progress
	if p.Power.Value != 98 || p.Materials[29].Count != 80 {
		t.Fatal("双倍失败未按原生返还18体力或误退券", p.Power, p.Materials[29])
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error { p.Materials[29] = Material{ID: 29}; return nil }); err != nil {
		t.Fatal(err)
	}
	s.Handle(ctx, c, "set_dungeon_bonus_double", rawArgs(84, 4401, true))
	before = CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if _, err := s.Handle(ctx, c, "enter_dungeon", rawArgs(85, 4401, map[string]any{})); err == nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("券不足没有整笔拒绝")
	}
	raw, _ := json.Marshal(before)
	var cold Progress
	if err := json.Unmarshal(raw, &cold); err != nil {
		t.Fatal(err)
	}
	row := (Avatar{Progress: cold}).InitialProperties("双倍冷登录")["dungeon_mgr"].(map[string]any)["4401"].(map[string]any)
	if row["double"] != true {
		t.Fatal("冷登录丢失双倍选择", row)
	}
	for _, args := range [][]any{{86, 501, true}, {87, -1, true}, {88, 4401, nil}} {
		out, err := s.Handle(ctx, c, "set_dungeon_bonus_double", rawArgs(args...))
		if err != nil || len(out) != 1 || out[0].Args[1].([]any)[0] == 0 {
			t.Fatal("未解锁或非法选择没有整数拒绝回调", args, err, out)
		}
	}
	out, err = s.Handle(ctx, c, "set_dungeon_bonus_double", rawArgs(89, 6109, true))
	if err != nil || len(out) != 2 || out[1].Args[1].([]any)[0] != 0 {
		t.Fatal("已接入高层基础奖励仍拒绝双倍选择", err, out)
	}
}
