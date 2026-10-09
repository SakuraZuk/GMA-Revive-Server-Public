package game

import (
	"context"
	"encoding/json"
	"testing"
)

func TestComposeCardsCostCapacityAndRollback(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	svc := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, svc)
	mid := 0
	var recipe composeRecipe
	for id, r := range androidCompose.Recipes {
		if len(r.Cards) == 1 && !r.UnverifiedPromise {
			mid, recipe = id, r
			break
		}
	}
	if mid == 0 {
		t.Fatal("缺少确定性配方")
	}
	if err := svc.updateProgress(ctx, c, func(p *Progress) error {
		p.Materials[mid] = Material{ID: mid, Count: recipe.Need*2 + 1, Total: recipe.Need*2 + 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	list, _ := json.Marshal([]int{mid})
	before := len(c.SelectedAvatarUnsafe().Progress.Cards)
	pushes, err := svc.Handle(ctx, c, "compose_cards", []json.RawMessage{json.RawMessage(`1`), list})
	p := c.SelectedAvatarUnsafe().Progress
	if err != nil || len(pushes) == 0 || pushes[len(pushes)-1].Method != "call_client_callback" || pushes[len(pushes)-1].Args[1].([]any)[0] != RetSuccess || len(p.Cards) != before+2 || p.Materials[mid].Count != 1 || p.Materials[mid].Total != recipe.Need*2+1 {
		t.Fatal("合成发卡扣料失败", err, pushes)
	}
	if _, err := svc.Handle(ctx, c, "compose_cards", []json.RawMessage{json.RawMessage(`2`), list}); err != nil {
		t.Fatal("不足应返回原生业务码", err)
	}
	if len(c.SelectedAvatarUnsafe().Progress.Cards) != before+2 {
		t.Fatal("余额不足重复合成")
	}
	if err := svc.updateProgress(ctx, c, func(p *Progress) error {
		p.Materials[mid] = Material{ID: mid, Count: recipe.Need, Total: recipe.Need}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	duplicates, _ := json.Marshal([]int{mid, mid})
	if _, err := svc.Handle(ctx, c, "compose_cards", []json.RawMessage{json.RawMessage(`3`), duplicates}); err == nil {
		t.Fatal("重复材料未拒绝")
	}
	if c.SelectedAvatarUnsafe().Progress.Materials[mid].Count != recipe.Need {
		t.Fatal("拒绝未回滚扣料")
	}
}

func TestGachaGuaranteeUsesRarityAndPreservesOwnedCards(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	svc := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, svc)
	old := c.SelectedAvatarUnsafe().Progress.Cards[0].UUID
	if err := svc.updateProgress(ctx, c, func(p *Progress) error { p.Materials[501] = Material{ID: 501, Count: 100, Total: 100}; return nil }); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		pushes, err := svc.Handle(ctx, c, "random_cards", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`1`), json.RawMessage(`10`)})
		if err != nil {
			t.Fatal(err)
		}
		callback := pushes[len(pushes)-1]
		if callback.Method != "call_client_callback" {
			t.Fatal("抽卡回调未位于属性推送之后")
		}
		cards := callback.Args[1].([]any)[1].([]any)
		if len(cards) != 12 || cards[10] != "card.card_list" || cards[11] != "__custom_type" {
			t.Fatal("十连回调缺少原生类型标记", cards)
		}
		found := false
		for _, v := range cards[:10] {
			if gachaCardRarity(androidGachaPools[1], v.(map[string]any)["card_id"].(int)) >= androidGachaPools[1].GuaranteeRarity {
				found = true
			}
		}
		if !found {
			t.Fatal("十连未满足Android保底稀有度")
		}
		mgr := pushes[1].Args[0].([]any)[1].(map[string]any)
		if _, ok := mgr[old]; !ok {
			t.Fatal("抽卡推送丢失既有卡")
		}
	}
}
