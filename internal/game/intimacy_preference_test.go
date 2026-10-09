package game

import (
	"context"
	"encoding/json"
	"maps"
	"reflect"
	"testing"
	"time"
)

// 原生 DAE1C2E7.favor_gift 比较 gift.like_tag 与 cards.tag[1][0]。
// 4401 偏好111，749/750匹配；701不匹配，523没有偏好标签。
func TestIntimacyPreferenceDiscoveryUsesWholePacketOldFlag(t *testing.T) {
	rules, err := loadIntimacyCatalog()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 8, 6, 0, 0, 0, time.UTC)
	p := NewProgress(1, now)
	p.Cards = []Card{newCard(4401, 1, now)}
	p.IntimacyCommons = map[int]IntimacyCommon{4401: {RingID: 8, RewardLevel: 2, SpecialCount: 4}}
	for _, id := range []int{749, 750, 701} {
		p.Materials[id] = Material{ID: id, Count: 4, Total: 4}
	}
	base := rules.Gifts[749].Intimacy*2 + rules.Gifts[750].Intimacy + rules.Gifts[701].Intimacy
	if _, err := consumeIntimacyGifts(&p, 4401, map[int]int64{749: 2, 750: 1, 701: 1}, rules, now); err != nil {
		t.Fatal(err)
	}
	common := p.IntimacyCommons[4401]
	if p.Intimacy[4401] != base || !common.LikeGift || common.RingID != 8 || common.RewardLevel != 2 || common.SpecialCount != 4 {
		t.Fatal("首次整笔按旧标记计算并保留既有公共进度失败", p.Intimacy[4401], common)
	}
	if _, err := consumeIntimacyGifts(&p, 4401, map[int]int64{749: 1, 750: 1}, rules, now); err != nil {
		t.Fatal(err)
	}
	want := base + rules.Gifts[749].FavoredIntimacy + rules.Gifts[750].FavoredIntimacy
	if p.Intimacy[4401] != want || !p.IntimacyCommons[4401].LikeGift {
		t.Fatal("后续匹配送礼未使用偏好收益", p.Intimacy[4401], want)
	}
	if _, err := consumeIntimacyGifts(&p, 4401, map[int]int64{701: 1}, rules, now); err != nil || !p.IntimacyCommons[4401].LikeGift {
		t.Fatal("非偏好送礼清除了已发现状态", err)
	}
	for _, id := range []int{749, 750, 701} {
		if p.Materials[id].Total != 4 {
			t.Fatal("送礼改变了累计获得数量", id)
		}
	}
}

func TestIntimacyPreferenceOnlyMatchedTagAndSpecialPreservesDiscovery(t *testing.T) {
	rules, err := loadIntimacyCatalog()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 8, 6, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		gift     int
		choice   int
		wantLike bool
	}{{"普通非偏好", 701, 0, false}, {"正确特殊选择无偏好标签", 523, 3, false}, {"首次特殊送礼匹配偏好", 749, 3, true}} {
		t.Run(tc.name, func(t *testing.T) {
			p := NewProgress(1, now)
			p.Cards = []Card{newCard(4401, 1, now)}
			p.IntimacyCommons = map[int]IntimacyCommon{4401: {RingID: 8, RewardLevel: 2}}
			p.Materials[tc.gift] = Material{ID: tc.gift, Count: 1, Total: 1}
			want := rules.Gifts[tc.gift].Intimacy
			if tc.choice == 0 {
				_, err = consumeIntimacyGifts(&p, 4401, map[int]int64{tc.gift: 1}, rules, now)
			} else {
				var verified bool
				want, verified = rules.Gifts[tc.gift].SpecialAmounts[tc.choice]
				if !verified {
					t.Fatal("原生精确整数特殊选择样本缺失")
				}
				_, err = consumeSpecialIntimacyGift(&p, 4401, map[int]int64{tc.gift: 1}, tc.choice, rules, now)
			}
			if err != nil {
				t.Fatal(err)
			}
			common := p.IntimacyCommons[4401]
			if p.Intimacy[4401] != want || common.LikeGift != tc.wantLike || common.RingID != 8 || common.RewardLevel != 2 {
				t.Fatal("偏好应由礼物标签决定且首次特殊收益沿用旧标记", p.Intimacy[4401], want, common)
			}
			if tc.choice != 0 && (common.SpecialCount != 1 || p.SpecialGiftCounts[4401] != 1 || p.IntimacySpecialTotal != 1) {
				t.Fatal("特殊送礼计数丢失", common, p.SpecialGiftCounts)
			}
		})
	}
}

func TestIntimacyPreferenceDiscoveryRPCPersistenceAndFailureRollback(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	now := time.Date(2026, 10, 8, 6, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	c, av := newBattleConnection(t, ctx, accounts, s)
	if _, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{newCard(4401, 1, now)}
		p.IntimacyCommons = map[int]IntimacyCommon{4401: {RewardLevel: 2}}
		p.Materials[749] = Material{ID: 749, Count: 3, Total: 3}
		p.Materials[750] = Material{ID: 750, Count: 0, Total: 0}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage("62"), json.RawMessage("4401"), json.RawMessage("{\"749\":2,\"750\":1}")}
	if pushes, err := s.Handle(ctx, c, "consume_multi_intimacy_gift", args); err != nil || businessPushCount(pushes) != 1 {
		t.Fatal("缺材料应只返回业务拒绝", err, pushes)
	}
	after, err := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(CloneProgress(before), CloneProgress(after)) || after.IntimacyCommons[4401].LikeGift {
		t.Fatal("失败送礼发现偏好或修改资产", err)
	}
	args = []json.RawMessage{json.RawMessage("63"), json.RawMessage("4401"), json.RawMessage("749"), json.RawMessage("1"), json.RawMessage("0")}
	pushes, err := s.Handle(ctx, c, "consume_intimacy_gift", args)
	if err != nil || businessPushCount(pushes) != 5 {
		t.Fatal(err, pushes)
	}
	after, err = accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if err != nil || !after.IntimacyCommons[4401].LikeGift || after.Intimacy[4401] != 20 || after.Materials[749].Count != 2 {
		t.Fatal("首次匹配送礼没有持久化偏好与基础收益", err, after.IntimacyCommons[4401])
	}
	if !reflect.DeepEqual(pushes[0], push("Avatar", "client_prop_changed", []any{"card_common_mgr", cardCommonMgrPropertiesWithProgress(after)})) {
		t.Fatal("成功赠礼公共属性推送未包含已发现状态")
	}
	// 749先成功扣费，随后751在第二枚回礼处失败，整体事务必须回滚。
	rules, err := loadIntimacyCatalog()
	if err != nil {
		t.Fatal(err)
	}
	broken := rules
	broken.Gifts = maps.Clone(rules.Gifts)
	row := broken.Gifts[751]
	row.ReturnRunes = append(row.ReturnRunes[:0:0], rules.Gifts[523].ReturnRunes...)
	row.ReturnRunes[1].Spec.Suit = 999999
	broken.Gifts[751] = row
	before, err = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.IntimacyCommons[4401] = IntimacyCommon{RewardLevel: 2}
		p.Materials[751] = Material{ID: 751, Count: 1, Total: 1}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		_, err := consumeIntimacyGifts(p, 4401, map[int]int64{749: 1, 751: 1}, broken, now)
		return err
	}); err == nil {
		t.Fatal("回礼生成故障未生效")
	}
	after, err = accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(CloneProgress(before), CloneProgress(after)) || after.IntimacyCommons[4401].LikeGift {
		t.Fatal("回礼中途失败没有整体回滚", err)
	}
}
