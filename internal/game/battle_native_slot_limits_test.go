package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestNativeFormationSixCardActualBattleFightingAndColdLayout(t *testing.T) {
	for _, dungeon := range []int{36700011, 36700012, 36700013, 36700021, 36700022, 36700023} {
		t.Run(itoaStage(dungeon), func(t *testing.T) {
			ctx := context.Background()
			a, s, c, av, _ := layoutTestSetup(t)
			cards := []Card{}
			values := []any{}
			for _, id := range []int{4401, 3202, 4402, 2202, 4202, 3301} {
				card := newCard(id, 1, time.Now())
				cards = append(cards, card)
				values = append(values, card.UUID)
			}
			if err := s.updateProgress(ctx, c, func(p *Progress) error {
				p.Cards = cards
				p.Battle = &BattleSession{UUID: "00112233445566778899aabb", DungeonID: dungeon,
					BattleID: dungeonCatalog[dungeon].BattleID, Loaded: true, Status: "准备"}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			raw := layoutTestRaw(t, values, []any{}, []any{})
			out, err := s.Handle(ctx, c, "battle_fighting", []json.RawMessage{raw})
			if err != nil || len(out) == 0 {
				t.Fatal("六人原生开战拒绝", err)
			}
			first := CloneProgress(c.SelectedAvatarUnsafe().Progress)
			if !first.Battle.Started || len(first.Battle.Team) != 6 || len(first.Battle.Layout.Fighting) != 6 {
				t.Fatal("六人未完整冻结", first.Battle)
			}
			if _, err := s.Handle(ctx, c, "battle_fighting", []json.RawMessage{raw}); err != nil {
				t.Fatal(err)
			}
			cold, err := a.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
			if err != nil || !reflect.DeepEqual(first.Battle.Layout, cold.Battle.Layout) || !reflect.DeepEqual(first.Battle.Team, cold.Battle.Team) {
				t.Fatal("冷读或重复开战丢失六人阵容", err)
			}
			if _, err := parseBattleLayoutForDungeon(layoutTestRaw(t, append(values, values[0]), []any{}, []any{}), dungeon); err == nil {
				t.Fatal("超出六人上限仍放行")
			}
		})
	}
}

func TestNativeFormationSupportFourAndOrdinaryIsolation(t *testing.T) {
	values := []any{"00112233445566778899aabb", "00112233445566778899aabc", "00112233445566778899aabd", "00112233445566778899aabe"}
	// 原表20303的battle30311为6出战2援护；只从冻结副本编号查表。
	if _, err := parseBattleLayoutForDungeon(layoutTestRaw(t, append(values, values[:2]...), values[:2], []any{}), 20303); err != nil {
		t.Fatal(err)
	}
	if _, err := parseBattleLayout(layoutTestRaw(t, append(values, values[:2]...), []any{}, []any{})); err == nil {
		t.Fatal("通用/PVP预设被扩成六人")
	}
	for dungeon, row := range dungeonCatalog {
		sizes := battleFormationLimits[row.BattleID]
		if len(sizes) == 2 && sizes[1] == 4 {
			if _, err := parseBattleLayoutForDungeon(layoutTestRaw(t, values[:2], values, []any{}), dungeon); err != nil {
				t.Fatal("原生四援护拒绝", dungeon, err)
			}
			if _, err := parseBattleLayoutForDungeon(layoutTestRaw(t, values[:2], append(values, values[0]), []any{}), dungeon); err == nil {
				t.Fatal("第五援护越界仍放行")
			}
		}
	}
}
