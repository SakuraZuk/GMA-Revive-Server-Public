package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestCardOathQualificationInstanceBreakthroughAndRollback(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	svc := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, svc)
	a := newCard(4401, 1, time.Now())
	a.Level, a.Grade = 60, 5
	b := newCard(4401, 1, time.Now())
	b.Level, b.Grade = 60, 5
	_, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{a, b}
		p.Intimacy = map[int]int{4401: 32000}
		p.IntimacyCommons = map[int]IntimacyCommon{4401: {RewardLevel: 5}}
		p.Materials[820] = Material{ID: 820, Count: 2, Total: 2}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ring := []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`4401`), json.RawMessage(`820`)}
	call := func(method string, args []json.RawMessage) []Push {
		t.Helper()
		pushes, err := svc.Handle(ctx, c, method, args)
		if err != nil {
			t.Fatal(err)
		}
		return pushes
	}
	code := func(pushes []Push) int { t.Helper(); return pushes[len(pushes)-1].Args[1].([]any)[0].(int) }
	if code(call("consume_ring", ring)) != 5052 {
		t.Fatal("未领奖不得誓约")
	}
	_, err = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		common := p.IntimacyCommons[4401]
		common.RewardLevel = 6
		p.IntimacyCommons[4401] = common
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if code(call("consume_ring", ring)) != 0 {
		t.Fatal("合格誓约失败")
	}
	if code(call("consume_ring", ring)) != 5053 {
		t.Fatal("重复誓约未拒绝")
	}
	uuid, _ := json.Marshal(a.UUID)
	if code(call("update_level_one", []json.RawMessage{json.RawMessage(`2`), uuid})) != 0 {
		t.Fatal("满级突破失败")
	}
	if code(call("update_level_one", []json.RawMessage{json.RawMessage(`2`), uuid})) != 5050 {
		t.Fatal("重复突破未拒绝")
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Materials[820].Count != 1 || p.IntimacyCommons[4401].RingID != 1 || !p.Cards[0].IsUpdateLevelOne || p.Cards[1].IsUpdateLevelOne || p.Cards[1].RingID != 0 {
		t.Fatal("誓约共享或实例突破状态错误")
	}
	before := CloneProgress(p)
	_, err = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error { p.Materials[820] = Material{ID: 820, Count: 0}; return context.Canceled })
	if err == nil {
		t.Fatal("事务错误未返回")
	}
	stored, err := accounts.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil || !reflect.DeepEqual(before, stored.Avatars[0].Progress) {
		t.Fatal("誓约状态回滚失败", err)
	}
	wire := cardMgrProperties(p.Cards)[a.UUID].(map[string]any)
	if wire["ring_id"] != 1 || wire["is_update_level_one"] != true {
		t.Fatal("突破字段未同步")
	}
}

func TestKnowledgeGrowthCapPartialExperienceAndMasterGate(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	svc := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, svc)
	if _, e := accounts.AdminUpdatePlayer(ctx, av.OID, func(av *Avatar) error { av.Info.Level = 60; av.Progress.AvatarLevel = 60; return nil }); e != nil {
		t.Fatal(e)
	}
	card := newCard(4401, 1, time.Now())
	card.Level = 19
	card.Grade = 1
	cost := androidOath.Levels[19].Exp
	card.Exp = int(cost - 7)
	_, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{card}
		p.Materials[4] = Material{ID: 4, Count: 1000000000, Total: 1000000000}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	uuid, _ := json.Marshal(card.UUID)
	args := []json.RawMessage{json.RawMessage(`1`), uuid, json.RawMessage(`true`)}
	pushes, err := svc.Handle(ctx, c, "up_level_card", args)
	if err != nil || len(pushes) == 0 || pushes[len(pushes)-1].Method != "call_client_callback" || pushes[len(pushes)-1].Args[1].([]any)[0] != RetSuccess {
		t.Fatal(err, pushes)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Cards[0].Level != 20 || p.Materials[4].Count != 999999993 {
		t.Fatal("越过品阶或忽略已有经验")
	}
	pushes, err = svc.Handle(ctx, c, "up_level_card", args)
	if err != nil || pushes[len(pushes)-1].Args[1].([]any)[0] != 5005 {
		t.Fatal("满品阶等级未拒绝", err)
	}
	_, err = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error { p.Cards[0].Level = 59; p.Cards[0].Grade = 5; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, e := accounts.AdminUpdatePlayer(ctx, av.OID, func(av *Avatar) error { av.Info.Level = 1; av.Progress.AvatarLevel = 1; return nil }); e != nil {
		t.Fatal(e)
	}
	pushes, err = svc.Handle(ctx, c, "up_level_card", args)
	if err != nil || pushes[len(pushes)-1].Args[1].([]any)[0] != 5049 {
		t.Fatal("馆主门槛未拒绝", err)
	}
	if cap, err := cardLevelCap(&Card{Grade: 5, RingID: 1, IsUpdateLevelOne: true}); err != nil || cap != 70 {
		t.Fatal("誓约上限无效")
	}
}

func TestUpgradeUsesAndroidGradeMaterialsAndAtomicFailure(t *testing.T) {
	for _, example := range []struct {
		name                   string
		grade, level, item     int
		fragments, items, gold int64
	}{
		{"原生品阶0升品", 0, 10, 150, 5, 20, 24000},
		{"已有品阶1升品", 1, 20, 151, 10, 30, 48000},
	} {
		t.Run(example.name, func(t *testing.T) {
			ctx := context.Background()
			accounts := NewFixtureAccounts(nil)
			svc := New(accounts, nil)
			c, av := newBattleConnection(t, ctx, accounts, svc)
			card := newCard(4401, example.level, svc.Now())
			if card.Grade != example.grade {
				t.Fatal("夹具品阶不符本版新建上限映射", card.Grade)
			}
			expectedCost := [][]int{{100, int(example.fragments)}, {example.item, int(example.items)}, {12, int(example.gold)}}
			if !reflect.DeepEqual(androidOath.Cards[4401].Upgrade[example.grade], expectedCost) {
				t.Fatal("本版幻书4401升品表与取证成本不符")
			}
			_, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
				p.Cards = []Card{card}
				p.Materials[100] = Material{ID: 100, Count: example.fragments, Total: example.fragments}
				p.Materials[example.item] = Material{ID: example.item, Count: example.items, Total: example.items}
				p.Materials[12] = Material{ID: 12, Count: example.gold - 1, Total: example.gold}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			before, err := accounts.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
			if err != nil {
				t.Fatal(err)
			}
			uuid, _ := json.Marshal(card.UUID)
			args := []json.RawMessage{json.RawMessage(`1`), uuid}
			pushes, err := svc.Handle(ctx, c, "upgrade_card", args)
			if err != nil || len(pushes) != 1 || pushes[0].Args[1].([]any)[0] != 5 {
				t.Fatal("升品材料不足未回调", err, pushes)
			}
			stored, err := accounts.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
			if err != nil || !reflect.DeepEqual(CloneProgress(before.Avatars[0].Progress), CloneProgress(stored.Avatars[0].Progress)) {
				t.Fatal("升品不足未整进度回滚", err)
			}
			_, err = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
				p.Materials[12] = Material{ID: 12, Count: example.gold, Total: example.gold}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			pushes, err = svc.Handle(ctx, c, "upgrade_card", args)
			if err != nil || businessPushCount(pushes) != 3 {
				t.Fatal(err, pushes)
			}
			p := c.SelectedAvatarUnsafe().Progress
			if p.Cards[0].Grade != example.grade+1 || p.Materials[100].Count != 0 || p.Materials[example.item].Count != 0 || p.Materials[12].Count != 0 {
				t.Fatal("升品成本或品阶错误")
			}
			if p.Materials[100].Total != example.fragments || p.Materials[example.item].Total != example.items || p.Materials[12].Total != example.gold {
				t.Fatal("升品消费改变累计获得数")
			}
		})
	}
}

func TestCardLevelUnlockNativeAndOfOrGroups(t *testing.T) {
	old := androidOath.Levels[2]
	defer func() { androidOath.Levels[2] = old }()
	row := old
	row.Conditions = [][][]int{{{2, 10}, {2, 20}}, {{2, 15}}}
	androidOath.Levels[2] = row
	if cardLevelUnlocked(2, 10) || !cardLevelUnlocked(2, 15) {
		t.Fatal("原生组内或组间且未保留")
	}
}
