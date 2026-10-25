package game

import (
	"context"
	"reflect"
	"testing"
)

func TestAllNativeFixedStorySlotsPreserveCapacityAndIdentity(t *testing.T) {
	checked := 0
	for id, d := range dungeonCatalog {
		fixed := battleStorySlots[d.BattleID]
		if len(fixed) == 0 {
			continue
		}
		checked++
		fighting := make([]any, len(fixed))
		for i := range fighting {
			fighting[i] = 0
		}
		raw := layoutTestRaw(t, fighting, []any{0, 0}, []any{})
		layout, err := parseBattleLayoutForDungeon(raw, id)
		if err != nil || len(layout.Fighting) != len(fixed) || len(layout.team()) != 0 {
			t.Fatal("剧情固定槽仍被四槽限制拒绝或混入玩家队伍", id, err, layout)
		}
		fighting[4] = "00112233445566778899aabb"
		if _, err := parseBattleLayoutForDungeon(layoutTestRaw(t, fighting, []any{}, []any{}), id); err == nil {
			t.Fatal("允许用第五张玩家幻书替换剧情角色", id)
		}
		if _, err := parseBattleLayout(raw); err == nil {
			t.Fatal("无关卡上下文放开了普通/PVP四槽限制")
		}
	}
	if checked < 9 {
		t.Fatal("未覆盖全部五、六槽剧情战斗", checked)
	}
	t.Logf("五、六槽剧情副本%d项全部核验", checked)
}

func TestStage210NativeFiveSlotsActuallyStartAndDuplicateDoesNotReset(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.GuideTasks = map[int]GuideTask{}
		p.Power.Value = 100
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Handle(ctx, c, "enter_dungeon", rawArgs(91, 610, map[string]any{})); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Handle(ctx, c, "load_entity_finish", nil); err != nil {
		t.Fatal(err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	fight := []any{p.Cards[0].UUID, 0, 0, 0, 0}
	raw := layoutTestRaw(t, fight, []any{0, 0}, []any{})
	if _, err := s.Handle(ctx, c, "set_layout_cards", rawArgs(610, map[string]any{"fighting_cards": fight, "support_cards": []any{0, 0}, "storyline_cards": []any{}})); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Handle(ctx, c, "battle_fighting", rawArgs(map[string]any{"fighting_cards": fight, "support_cards": []any{0, 0}, "storyline_cards": []any{}})); err != nil {
		t.Fatal(err)
	}
	p = c.SelectedAvatarUnsafe().Progress
	if !p.Battle.Started || len(p.Battle.Team) != 1 || len(p.Battle.Layout.Fighting) != 5 {
		t.Fatal("2-10未实际开战", p.Battle)
	}
	if _, err := parseBattleLayoutForDungeon(raw, 610); err != nil {
		t.Fatal(err)
	}
	before := CloneProgress(p)
	if _, err := s.Handle(ctx, c, "enter_dungeon", rawArgs(92, 610, map[string]any{})); err != nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("2-10重复点击重置或重复扣费", err)
	}
}
