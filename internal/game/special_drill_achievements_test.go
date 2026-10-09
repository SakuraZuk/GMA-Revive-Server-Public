package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestSpecialDrillActualResultAllowedTaskRollbackAndFrozenRetry(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	t.Setenv("HS_SPECIAL_DRILL_REOPEN", "timber-12-20261008")
	t.Setenv("HS_RUNE_FALLBACK_POOLS", approvedGameplayValue(t, "HS_RUNE_FALLBACK_POOLS"))
	ctx := context.Background()
	store := NewFixtureAccounts(nil)
	svc := New(store, nil)
	c, _ := newBattleConnection(t, ctx, store, svc)
	if _, err := store.UpdateProgress(ctx, selectedOID(c), func(p *Progress) error {
		p.AvatarLevel = 60
		for id, g := range p.GuideTasks {
			g.Status = 2
			p.GuideTasks[id] = g
		}
		p.Power.Value = 1000
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]int{{2103, 211200}, {2303, 231200}, {2503, 251200}} {
		if _, err := svc.Handle(ctx, c, "enter_dungeon", socialArgs(1, pair[0], map[string]any{})); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Handle(ctx, c, "load_entity_finish", nil); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Handle(ctx, c, "battle_fighting", socialArgs(map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})); err != nil {
			t.Fatal(err)
		}
		b := c.SelectedAvatarUnsafe().Progress.Battle
		event := func(seq int, kind string, data map[string]any) []Push {
			raw, _ := json.Marshal(map[string]any{"battle_uuid": b.UUID, "sequence": seq, "kind": kind, "data": data})
			return observeBattleEvent(t, ctx, svc, c, string(raw))
		}
		event(1, "ready", map[string]any{"version": 1})
		event(2, "started", map[string]any{"units": []any{}})
		before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
		wrong := 211200
		if wrong == pair[1] {
			wrong = 231200
		}
		result := map[string]any{"winner_eids": []string{hexOf(selectedOID(c))}, "player_eid": hexOf(selectedOID(c)), "outcome": "win", "finished_task_list": []int{wrong}}
		if len(event(3, "result", result)) != 0 {
			t.Fatal("异副本演练任务未拒绝")
		}
		saved, _ := store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
		if !reflect.DeepEqual(before, saved) {
			t.Fatal("错误伤害任务报告部分提交")
		}
		result["finished_task_list"] = []int{pair[1]}
		if battleTestResult(event(3, "result", result)) == nil {
			t.Fatal("正确原生任务结算缺回包")
		}
		saved, _ = store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
		found := false
		for id, r := range androidAchievements.Rules {
			for _, targets := range r.Targets {
				for _, tid := range targets {
					target := androidAchievements.Targets[tid]
					var param int
					if target.Type == 66 && len(target.Params) == 1 && json.Unmarshal(target.Params[0], &param) == nil && param == pair[1] {
						found = saved.Achievements[id].Targets[tid] == 1
					}
				}
			}
		}
		if !found {
			t.Fatal("正确原生演练任务未推进对应伤害成就", pair)
		}
		before = CloneProgress(saved)
		event(3, "result", result)
		saved, _ = store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
		if !reflect.DeepEqual(before, saved) {
			t.Fatal("重复演练result推进成就或发奖")
		}
	}
}
