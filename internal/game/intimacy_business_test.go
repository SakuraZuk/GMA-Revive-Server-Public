package game

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestIntimacyChapterNativeRPCPersistenceAndNoReplay(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	rules, err := loadIntimacyCatalog()
	if err != nil {
		t.Fatal(err)
	}
	_, err = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{newCard(4401, 1, time.Now()), newCard(4401, 1, time.Now())}
		p.Intimacy = map[int]int{4401: 32000}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage(`77`), json.RawMessage(`4401`)}
	count := len(rules.Cards[4401].Bonuses)
	if count == 0 || autoIntimacyClaim(4401, rules) {
		t.Fatal("章节测试配置失效")
	}
	for level := 1; level <= count; level++ {
		pushes, err := s.Handle(ctx, c, "receive_intimacy_bonus", args)
		pushes = checkedCorePushes(t, pushes, c.SelectedAvatarUnsafe().Progress)
		if err != nil || len(pushes) != 5 {
			t.Fatalf("章节%d：%v %v", level, err, pushes)
		}
		if !reflect.DeepEqual(pushes[4], Callback(77, []any{0})) {
			t.Fatal("回调签名或推送顺序错误", pushes)
		}
		common := pushes[0].Args[0].([]any)[1].(map[string]any)["4401"].(map[string]any)
		if common["reward_intimacy_level"] != level {
			t.Fatal("回调前未下发领取等级")
		}
		stored, err := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
		if err != nil || stored.IntimacyCommons[4401].RewardLevel != level {
			t.Fatal("章节未持久化", err)
		}
		for _, card := range stored.Cards {
			if card.SupportSkillLevel != rules.Levels[level].Support {
				t.Fatal("同名卡援护等级未同步")
			}
		}
	}
	before, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	pushes, err := s.Handle(ctx, c, "receive_intimacy_bonus", args)
	if err != nil || !reflect.DeepEqual(pushes, []Push{Callback(77, []any{rules.Errors["RET_CARD_NO_INTIMACY_BONUS"]})}) {
		t.Fatal("已领完章节未按原生拒绝", err, pushes)
	}
	after, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, after) {
		t.Fatal("已领完仍修改资产")
	}
	props := (Avatar{Progress: after}).InitialProperties("测试账号")
	if _, exists := props["intimacy_commons"]; exists {
		t.Fatal("内部好感度状态泄漏")
	}
	common := props["card_common_mgr"].(map[string]any)["4401"].(map[string]any)
	if common["reward_intimacy_level"] != count {
		t.Fatal("二次登录丢失领取进度")
	}
}

func TestIntimacyConcurrentClaimsAndOriginalRejections(t *testing.T) {
	rules, err := loadIntimacyCatalog()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	_, err = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{newCard(4401, 1, time.Now())}
		p.Intimacy = map[int]int{4401: 32000}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error { return claimIntimacyChapter(p, 4401, rules) })
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	claims := 0
	for err := range results {
		if err == nil {
			claims++
			continue
		}
		if reject, ok := err.(*intimacyBusinessError); !ok || reject.name != "RET_CARD_NO_INTIMACY_BONUS" {
			t.Fatal(err)
		}
	}
	stored, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if claims != len(rules.Cards[4401].Bonuses) || stored.IntimacyCommons[4401].RewardLevel != claims {
		t.Fatal("并发领取重复发奖或丢失章节", claims)
	}
	pushes, err := s.Handle(ctx, c, "receive_intimacy_bonus", []json.RawMessage{json.RawMessage(`2`), json.RawMessage(`3202`)})
	if err != nil || !reflect.DeepEqual(pushes, []Push{Callback(2, []any{rules.Errors["RET_CARD_NOT_EXIST"]})}) {
		t.Fatal("未拥有卡领取拒绝不符合原生", err, pushes)
	}
	// 原表测试卡4403没有残页映射或材料。配置缺口必须拒绝，不能猜一个资产编号。
	p := NewProgress(1, time.Now())
	p.Cards = []Card{newCard(4403, 1, time.Now())}
	p.Intimacy = map[int]int{4403: 32000}
	p.IntimacyCommons = map[int]IntimacyCommon{4403: {RewardLevel: 4}}
	before := CloneProgress(p)
	if err := claimIntimacyChapter(&p, 4403, rules); err == nil || !reflect.DeepEqual(before, CloneProgress(p)) {
		t.Fatal("原表缺口伪造发奖或污染进度", err)
	}
}

func TestIntimacyChapterRewardAssetsAndAtomicCapacity(t *testing.T) {
	rules, err := loadIntimacyCatalog()
	if err != nil {
		t.Fatal(err)
	}
	// 遍历真实Android章节，确保指定残页和永久头像框不会作为普通材料落账。
	for cardID, card := range rules.Cards {
		if autoIntimacyClaim(cardID, rules) || card.Disabled != 0 || card.NotInStat != 0 {
			continue
		}
		for index, bid := range card.Bonuses {
			p := NewProgress(1, time.Now())
			p.Cards = []Card{newCard(cardID, 1, time.Now())}
			p.Intimacy = map[int]int{cardID: 32000}
			p.IntimacyCommons = map[int]IntimacyCommon{cardID: {RewardLevel: index}}
			if err := claimIntimacyChapter(&p, cardID, rules); err != nil {
				t.Fatalf("卡%d章节%d：%v", cardID, index+1, err)
			}
			for _, row := range rules.Bonuses[bid].Fixed {
				mat := rules.Materials[row[0]]
				if mat.Type == rules.Constants["MATERIAL_TYPE_HEADBOX"] {
					if expiry, ok := p.OwnedHeadBox[mat.Target]; !ok || expiry != 0 {
						t.Fatal("永久头像框未解锁")
					}
					if p.Materials[row[0]].Count != 0 {
						t.Fatal("头像框误入背包")
					}
				}
				if mat.Type == rules.Constants["MATERIAL_TYPE_SPECIAL_FRAGE"] {
					id := rules.Fragments[cardID]
					if id == 0 {
						id = cardID
					}
					if p.Materials[row[0]].Count != 0 || p.Materials[id].Count < int64(row[1]) {
						t.Fatal("指定残页未转换")
					}
				}
			}
		}
	}
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	before, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{newCard(4401, 1, time.Now())}
		p.Intimacy = map[int]int{4401: 32000}
		for _, row := range rules.Bonuses[rules.Cards[4401].Bonuses[0]].Fixed {
			id := row[0]
			if id == 9990 {
				id = rules.Fragments[4401]
			}
			p.Materials[id] = Material{ID: id, Count: math.MaxInt64, Total: math.MaxInt64}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Handle(ctx, c, "receive_intimacy_bonus", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`4401`)}); err == nil {
		t.Fatal("溢出资产仍领取成功")
	}
	after, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, after) {
		t.Fatal("溢出失败没有整体回滚")
	}
}
