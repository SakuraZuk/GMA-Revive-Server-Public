package game

import (
	"context"
	"reflect"
	"testing"
)

func TestBeginnerPromiseSinglesAndTenDraw(t *testing.T) {
	for _, batch := range []int{1, 10} {
		t.Run(map[int]string{1: "分次单抽", 10: "连续十连"}[batch], func(t *testing.T) {
			ctx := context.Background()
			accounts := NewFixtureAccounts(nil)
			svc := New(accounts, nil)
			c, _ := newBattleConnection(t, ctx, accounts, svc)
			if err := svc.updateProgress(ctx, c, func(p *Progress) error { p.Materials[11] = Material{ID: 11, Count: 2760, Total: 2760}; return nil }); err != nil {
				t.Fatal(err)
			}
			before := len(c.SelectedAvatarUnsafe().Progress.Cards)
			for drawn := 0; drawn < 20; drawn += batch {
				out, err := svc.Handle(ctx, c, "random_cards", socialArgs(7, 3, batch))
				if err != nil || out[len(out)-1].Args[1].([]any)[0] != 0 {
					t.Fatal("启程抽卡失败", err, out)
				}
				p := c.SelectedAvatarUnsafe().Progress
				if p.RandomCardsRecord[3].RandomCount != drawn+batch {
					t.Fatal("启程计数未累计")
				}
				if drawn+batch >= 6 {
					found := false
					for _, card := range p.Cards[before : before+6] {
						if beginnerPromiseRules[5].matches(card.CardID) {
							found = true
						}
					}
					if !found {
						t.Fatal("前六抽缺少原生典藏候选")
					}
				}
			}
			p := c.SelectedAvatarUnsafe().Progress
			if len(p.Cards) != before+20 || p.Materials[11].Count != 0 {
				t.Fatal("成本或卡数异常")
			}
			for _, rule := range beginnerPromiseRules {
				r := p.PromiseCardsRecord[rule.ID]
				if r.Count != 20 || r.FinishCount < 1 {
					t.Fatal("一次性保底记录未完成", rule.ID, r)
				}
			}
			old := CloneProgress(p)
			out, err := svc.Handle(ctx, c, "random_cards", socialArgs(8, 3, 1))
			if err != nil || out[len(out)-1].Args[1].([]any)[0] == 0 || !reflect.DeepEqual(old, c.SelectedAvatarUnsafe().Progress) {
				t.Fatal("上限拒绝修改存档", err)
			}
		})
	}
}

func TestBeginnerPromiseLegacyAndEarlyHit(t *testing.T) {
	p := Progress{PromiseCardsRecord: map[int]PromiseCardRecord{}}
	prepareBeginnerPromises(&p, 6)
	if _, exists := p.PromiseCardsRecord[1007]; exists {
		t.Fatal("旧档到期保底被伪造")
	}
	if p.PromiseCardsRecord[1008].Count != 6 {
		t.Fatal("旧档未到期节点未接续")
	}
	refreshBeginnerPromises(&p, 2302)
	if _, exists := p.PromiseCardsRecord[1007]; exists {
		t.Fatal("刷新补造旧典藏收据")
	}
	p = Progress{PromiseCardsRecord: map[int]PromiseCardRecord{}}
	prepareBeginnerPromises(&p, 0)
	refreshBeginnerPromises(&p, 2403)
	if p.PromiseCardsRecord[1007].FinishCount != 1 {
		t.Fatal("提前命中未记录")
	}
	for i := 1; i < 6; i++ {
		refreshBeginnerPromises(&p, 1201)
	}
	if p.PromiseCardsRecord[1007].FinishCount != 1 {
		t.Fatal("完成次数异常")
	}
}

func TestBeginnerPromiseEmptyWeightsRollback(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	svc := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, svc)
	if err := svc.updateProgress(ctx, c, func(p *Progress) error { p.Materials[11] = Material{ID: 11, Count: 1380, Total: 1380}; return nil }); err != nil {
		t.Fatal(err)
	}
	old := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	pool := androidNativeDrawPools[3]
	defer func() { androidNativeDrawPools[3] = pool }()
	broken := pool
	broken.Weights = map[int]map[int]int64{1: {1201: 1}}
	androidNativeDrawPools[3] = broken
	out, err := svc.Handle(ctx, c, "random_cards", socialArgs(9, 3, 10))
	if err != nil || out[len(out)-1].Args[1].([]any)[0] == 0 {
		t.Fatal("空候选未拒绝", err)
	}
	if !reflect.DeepEqual(old, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("抽卡失败未回滚费用或保底记录")
	}
}
