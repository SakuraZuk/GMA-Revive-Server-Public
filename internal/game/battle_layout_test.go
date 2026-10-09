package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/mobileproto"
)

func layoutTestSetup(t *testing.T) (*FixtureAccounts, *Service, *Connection, Avatar, []Card) {
	t.Helper()
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	c.battleStartSent = false
	cards := []Card{newCard(4401, 1, time.Now()), newCard(3202, 1, time.Now())}
	_, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = cards
		p.Battle = &BattleSession{UUID: "00112233445566778899aabb", DungeonID: 10001, BattleID: 10001, Loaded: true, Status: "准备"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return accounts, s, c, *av, cards
}

func layoutTestRaw(t *testing.T, fight, support, story []any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"fighting_cards": fight, "support_cards": support, "storyline_cards": story})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestLayoutNativeSlotsPersistenceAndActiveBattle(t *testing.T) {
	accounts, s, c, av, cards := layoutTestSetup(t)
	ctx := context.Background()
	raw := layoutTestRaw(t, []any{0, cards[0].UUID, nil}, []any{cards[1].UUID, 0}, []any{"ffffffffffffffffffffffff"})
	pushes, err := s.Handle(ctx, c, "set_layout_cards", []json.RawMessage{json.RawMessage(`10001`), raw})
	if err != nil || businessPushCount(pushes) != 0 {
		t.Fatalf("原生无回调布阵失败: %v %#v", err, pushes)
	}
	p, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	layout := p.BattleLayouts[10001]
	if !reflect.DeepEqual(layout.Fighting, []string{"", cards[0].UUID, ""}) || !reflect.DeepEqual(layout.Support, []string{cards[1].UUID, ""}) || !reflect.DeepEqual(layout.Storyline, []string{"ffffffffffffffffffffffff"}) {
		t.Fatal("槽位、分组或剧情上下文丢失", layout)
	}
	if !reflect.DeepEqual(p.Battle.Team, []string{cards[0].UUID, cards[1].UUID}) || !reflect.DeepEqual(*p.Battle.Layout, layout) || len(p.Cards) != 2 {
		t.Fatal("活动战斗队伍不一致或剧情卡污染资产")
	}
	props := (Avatar{Progress: p}).InitialProperties("测试")
	if _, exists := props["battle_layouts"]; exists {
		t.Fatal("未知内部布阵属性被下发")
	}
	if _, exists := props["lineup"]; exists {
		t.Fatal("未知lineup属性被下发")
	}
	clone := CloneProgress(p)
	clone.BattleLayouts[10001].Fighting[1] = ""
	if p.BattleLayouts[10001].Fighting[1] == "" {
		t.Fatal("布阵持久快照共享切片")
	}
	p.Cards = p.Cards[1:]
	clean := savedBattleLayout(p, 10001)
	if len(clean.Fighting) != 3 || clean.Fighting[1] != "" || clean.Support[0] != cards[1].UUID {
		t.Fatal("分解卡牌挤压了槽位", clean)
	}
}

func TestLayoutInvalidOrStartedChangesRollback(t *testing.T) {
	accounts, s, c, av, cards := layoutTestSetup(t)
	ctx := context.Background()
	// 日界/竞技基线是独立周期事务；先完成它，再验证非法阵容本身整笔回滚。
	if err := s.refreshPvpAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	before, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	bad := []json.RawMessage{
		json.RawMessage(`[]`), json.RawMessage(`null`), json.RawMessage(`{"fighting_cards":[1]}`),
		json.RawMessage(`{"fighting_cards":[0,0,0,0,0]}`),
		layoutTestRaw(t, []any{cards[0].UUID}, []any{cards[0].UUID}, []any{}),
		layoutTestRaw(t, []any{"000000000000000000000000"}, []any{}, []any{}),
	}
	for _, raw := range bad {
		if _, err := s.Handle(ctx, c, "set_layout_cards", []json.RawMessage{json.RawMessage(`10001`), raw}); err == nil {
			t.Fatal("非法布阵接受", string(raw))
		}
		after, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
		if !reflect.DeepEqual(before, after) {
			t.Fatal("非法布阵部分提交")
		}
	}
	before, _ = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error { p.Battle.Started = true; return nil })
	if _, err := s.Handle(ctx, c, "set_layout_cards", []json.RawMessage{json.RawMessage(`10001`), layoutTestRaw(t, []any{cards[0].UUID}, []any{}, []any{})}); err == nil {
		t.Fatal("已开战仍能改队伍")
	}
	after, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, after) {
		t.Fatal("开战后布阵拒绝未回滚")
	}
}

func TestBattleStartMixedEmptySlotsIncludesOwnedCards(t *testing.T) {
	accounts, s, c, av, cards := layoutTestSetup(t)
	raw := layoutTestRaw(t, []any{0, cards[0].UUID}, []any{0, cards[1].UUID}, []any{"ffffffffffffffffffffffff"})
	pushes, err := s.Handle(context.Background(), c, "battle_fighting", []json.RawMessage{raw})
	if err != nil || businessPushCount(pushes) != 4 {
		t.Fatalf("混合槽位开战失败 %v %#v", err, pushes)
	}
	args := pushes[0].Args[1].([]any)
	uuidList := args[0].([]any)
	cardList := args[1].([]any)
	if len(uuidList) != 2 || len(cardList) != 4 || cardList[2] != "card.card_list" || cardList[3] != "__custom_type" || uuidList[0] != ObjectID(cards[0].UUID) || uuidList[1] != ObjectID(cards[1].UUID) {
		t.Fatal("空槽导致真实卡牌漏发", args)
	}
	p, _ := accounts.UpdateProgress(context.Background(), av.OID, func(*Progress) error { return nil })
	if !p.Battle.Started || len(p.Battle.Team) != 2 || p.Battle.Layout.Support[0] != "" {
		t.Fatal("开战保存不一致")
	}
}

func TestLastFightingWireSeparatesSupportAndKeepsSlots(t *testing.T) {
	player := ObjectID("00112233445566778899aabb")
	fightID := "11112233445566778899aabb"
	supportID := "22112233445566778899aabb"
	wire := lastFightingWire(player, []string{"", fightID}, []string{supportID, ""})
	fight := wire[0].Value.([]any)
	support := wire[2].Value.(mobileproto.Map)[0].Value.([]any)
	if !reflect.DeepEqual(fight, []any{0, ObjectID(fightID)}) || !reflect.DeepEqual(support, []any{ObjectID(supportID), 0}) {
		t.Fatal("援护错发出战卡或槽位丢失", wire)
	}
}

func TestEnterDungeonRestoresSeparateLayoutWithoutChangingOtherBattle(t *testing.T) {
	accounts, s, c, av, cards := layoutTestSetup(t)
	ctx := context.Background()
	raw := layoutTestRaw(t, []any{0, cards[0].UUID}, []any{cards[1].UUID, 0}, []any{})
	if _, err := s.Handle(ctx, c, "set_layout_cards", []json.RawMessage{json.RawMessage(`10001`), raw}); err != nil {
		t.Fatal(err)
	}
	pushes, err := s.Handle(ctx, c, "enter_dungeon", []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`10001`), json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	var wire mobileproto.Map
	for _, p := range pushes {
		if p.Method == "sync_battle_method" && p.Args[0] == "set_last_fighting_cards" {
			wire = p.Args[1].([]any)[0].(mobileproto.Map)
		}
	}
	if wire == nil || !reflect.DeepEqual(wire[0].Value, []any{0, ObjectID(cards[0].UUID)}) || !reflect.DeepEqual(wire[2].Value.(mobileproto.Map)[0].Value, []any{ObjectID(cards[1].UUID), 0}) {
		t.Fatal("重新进入丢失分组或槽位", wire)
	}
	before, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	otherRaw := layoutTestRaw(t, []any{cards[1].UUID}, []any{}, []any{})
	if _, err = s.Handle(ctx, c, "set_layout_cards", []json.RawMessage{json.RawMessage(`10002`), otherRaw}); err != nil {
		t.Fatal(err)
	}
	after, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if !reflect.DeepEqual(before.Battle, after.Battle) || !reflect.DeepEqual(after.BattleLayouts[10001], before.BattleLayouts[10001]) || after.BattleLayouts[10002].Fighting[0] != cards[1].UUID {
		t.Fatal("其他副本布阵污染当前战斗")
	}
}
