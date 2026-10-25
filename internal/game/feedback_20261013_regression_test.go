package game

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// 玩家反馈的“幻星辰墟第六层不结算”和“残夜任务不累计”属于同一结果事务。
// 本用例必须走真实 Handle 入场、加载、开战、ready、started、result 全链，
// 不能只直接调用奖励函数，否则无法覆盖 battle_result 回包与任务事件桥。
func TestFantasyStarSixthFloorsFullHandleSettlementAndNewTaskCount(t *testing.T) {
	for _, dungeonID := range []int{4106, 4206, 4306, 4406, 4506} {
		t.Run(fmt.Sprint(dungeonID), func(t *testing.T) {
			ctx := context.Background()
			accounts := NewFixtureAccounts(nil)
			service := New(accounts, nil)
			now := time.Now().In(time.FixedZone("北京时间", 8*3600)).Truncate(time.Second)
			service.Now = func() time.Time { return now }
			c, av := newBattleConnection(t, ctx, accounts, service)
			if err := service.updateProgress(ctx, c, func(p *Progress) error {
				p.AvatarLevel = 60
				p.GuideTasks = map[int]GuideTask{}
				p.Power.Value = 300
				p.ClearedDungeons = append(p.ClearedDungeons, 509, dungeonID-1)
				return ensureBasicRewards(p, now, true)
			}); err != nil {
				t.Fatal(err)
			}
			card := c.SelectedAvatarUnsafe().Progress.Cards[0].UUID
			prepared := c.SelectedAvatarUnsafe().Progress
			if !canEnterDungeon(&prepared, dungeonID) {
				t.Fatal("测试账号仍被引导门槛阻断", prepared.GuideTasks)
			}
			if err := ordinaryDungeonConditions(activityData("dungeons", dungeonID), prepared, now); err != nil {
				t.Fatal("测试账号未满足第六层原表前置", err, prepared.ClearedDungeons)
			}
			costTrial := CloneProgress(prepared)
			if _, err := prepareOrdinaryDungeonCosts(&costTrial, dungeonID, now); err != nil {
				t.Fatal("测试账号未满足第六层完整费用前置", err, prepared.ClearedDungeons, prepared.Power, string(activityData("activity_type", 9)["unlock_condition"]))
			}
			entered, err := service.Handle(ctx, c, "enter_dungeon", rawArgs(1, dungeonID, map[string]any{}))
			if err != nil || c.SelectedAvatarUnsafe().Progress.Battle == nil {
				t.Fatal("第六层入场失败", err, entered)
			}
			if _, err := service.Handle(ctx, c, "load_entity_finish", nil); err != nil {
				t.Fatal("第六层加载完成失败", err)
			}
			layout := map[string]any{"fighting_cards": []any{card, 0, 0, 0}, "support_cards": []int{0, 0}, "storyline_cards": []int{}}
			if _, err := service.Handle(ctx, c, "battle_fighting", rawArgs(layout)); err != nil {
				t.Fatal("第六层开战失败", err)
			}
			uuid := c.SelectedAvatarUnsafe().Progress.Battle.UUID
			emit := func(sequence int, kind string, data map[string]any) []Push {
				t.Helper()
				envelope := map[string]any{"battle_uuid": uuid, "sequence": sequence, "kind": kind, "data": data}
				raw, _ := json.Marshal([]any{envelope})
				out, err := service.Handle(ctx, c, "do_command", []json.RawMessage{json.RawMessage(`"__battle_event__"`), raw})
				if err != nil {
					t.Fatal("第六层观察事件失败", kind, err)
				}
				return out
			}
			emit(1, "ready", map[string]any{"version": 1})
			emit(2, "started", map[string]any{"units": []any{map[string]any{"eid": "1", "role": 4403, "hex": []int{0, 0, 0}, "kind": "ally"}}})
			out := emit(3, "result", map[string]any{"winner_eids": []string{fmt.Sprintf("%x", av.OID)}, "finished_task_list": []int{}})
			result := battleTestResult(out)
			p := c.SelectedAvatarUnsafe().Progress
			if result == nil || result.Args[0] != true || p.Battle == nil || !p.Battle.Finished || !p.Battle.RewardGranted || p.Battle.SettlementBox == nil {
				t.Fatal("第六层未返回并持久化完整胜利结算", result, p.Battle)
			}
			if !containsInt(p.ClearedDungeons, dungeonID) {
				t.Fatal("第六层通关记录缺失", p.ClearedDungeons)
			}
			if p.NewTasks[101].FinishedTargets[20102] != 1 {
				t.Fatal("幻星辰墟胜利未累计残夜目标20102", p.NewTasks[101])
			}
		})
	}
}

func TestCelestialMechanicsGiftCrossesLevelTwoAndColdProjection(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, service)
	if err := service.updateProgress(ctx, c, func(p *Progress) error {
		p.Cards = []Card{newCard(4410, 1, service.Now())}
		p.Intimacy = map[int]int{4410: 140}
		p.IntimacyCommons = map[int]IntimacyCommon{}
		p.Materials[701] = Material{ID: 701, Count: 1, Total: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out, err := service.Handle(ctx, c, "consume_intimacy_gift", rawArgs(21, 4410, 701, 1, 0))
	if err != nil {
		t.Fatal("天体运行论跨2级送礼事务失败", err)
	}
	callbackOK, commonBeforeCallback := false, false
	for _, item := range out {
		if item.Method == "client_prop_changed" && len(item.Args) == 1 {
			pair, _ := item.Args[0].([]any)
			if len(pair) == 2 && pair[0] == "card_common_mgr" {
				mgr := pair[1].(map[string]any)
				common := mgr["4410"].(map[string]any)
				commonBeforeCallback = common["intimacy"] == 160
			}
		}
		if item.Method == "call_client_callback" {
			args := item.Args[1].([]any)
			callbackOK = commonBeforeCallback && len(args) == 2 && args[0] == 0
		}
	}
	if !callbackOK {
		t.Fatal("跨2级送礼未在成功回调前同步好感度", out)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if intimacyLevel(p.Intimacy[4410], mustIntimacyCatalog(t)) != 2 || p.Materials[701].Count != 0 {
		t.Fatal("跨2级数值或材料扣除错误", p.Intimacy[4410], p.Materials[701])
	}
	cold, err := accounts.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	props := (Avatar{Progress: cold}).InitialProperties("天体运行论冷登录")
	common := props["card_common_mgr"].(map[string]any)["4410"].(map[string]any)
	if common["intimacy"] != 160 || common["reward_intimacy_level"] != 0 {
		t.Fatal("天体运行论跨2级后冷登录投影损坏", common)
	}
}

func mustIntimacyCatalog(t *testing.T) intimacyCatalog {
	t.Helper()
	rules, err := loadIntimacyCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return rules
}
