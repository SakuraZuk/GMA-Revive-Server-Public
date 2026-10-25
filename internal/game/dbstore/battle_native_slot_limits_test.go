package dbstore

import (
	"context"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestPostgresNativeSixCardFormationStartAndColdPersistence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)
	info := game.ClientInfo{Account: "六人阵型原生实库", Password: "pw", Hostnum: 1}
	id, err := s.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	av, err := s.SetNicknameGender(ctx, id.Avatars[0].OID, "阵型验收", 1)
	if err != nil {
		t.Fatal(err)
	}
	fight := []any{}
	if _, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Cards = nil
		for i, cardID := range []int{4401, 3202, 4402, 2202, 4202, 3301} {
			uuid := []string{"00112233445566778899aaa1", "00112233445566778899aaa2", "00112233445566778899aaa3",
				"00112233445566778899aaa4", "00112233445566778899aaa5", "00112233445566778899aaa6"}[i]
			p.Cards = append(p.Cards, game.Card{UUID: uuid, CardID: cardID, Level: 1, Grade: 1, SupportSkillLevel: 1})
			fight = append(fight, uuid)
		}
		p.Battle = &game.BattleSession{UUID: "00112233445566778899aabb", DungeonID: 36700011, BattleID: 367011, Loaded: true, Status: "准备"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	svc := pgActivityService(t, s, func() time.Time { return now })
	c := pgActivityLogin(t, svc, info)
	// 登录恢复会重建准备阶段；沿用原生客户端加载完成握手后才允许开战。
	pgRemainingRPC(t, svc, c, "load_entity_finish")
	cards := map[string]any{"fighting_cards": fight, "support_cards": []any{}, "storyline_cards": []any{}}
	if out, err := svc.Handle(ctx, c, "battle_fighting", pgSocialArgs(cards)); err != nil || len(out) == 0 {
		t.Fatal("原生六人开战失败", err)
	}
	first := game.CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if !first.Battle.Started || len(first.Battle.Team) != 6 {
		t.Fatal("六人未完整冻结")
	}
	if _, err := svc.Handle(ctx, c, "battle_fighting", pgSocialArgs(cards)); err != nil {
		t.Fatal(err)
	}
	cold, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil || !reflect.DeepEqual(first.Battle.Layout, cold.Avatars[0].Progress.Battle.Layout) || !reflect.DeepEqual(first.Battle.Team, cold.Avatars[0].Progress.Battle.Team) {
		t.Fatal("新Store冷读丢失完整六人布局", err)
	}
}
