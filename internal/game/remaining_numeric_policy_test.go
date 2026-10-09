package game

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestRemainingIntimacyHalfUpPolicyAndDefaultRefusal(t *testing.T) {
	rules, err := loadIntimacyCatalog()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000000, 0)
	makeProgress := func() Progress {
		p := NewProgress(20, now)
		p.Cards = []Card{newCard(4401, 1, now)}
		p.IntimacyCommons = map[int]IntimacyCommon{4401: {LikeGift: true}}
		p.Materials[749] = Material{ID: 749, Count: 1, Total: 1}
		return p
	}
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "")
	p := makeProgress()
	before, _ := json.Marshal(p)
	if _, err := consumeSpecialIntimacyGift(&p, 4401, map[int]int64{749: 1}, 2, rules, now); err == nil {
		t.Fatal("默认未批准的小数收益被启用")
	}
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("默认拒绝仍改变玩家资产")
	}
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	for choice, want := range map[int]int{2: 24, 3: 26} {
		p = makeProgress()
		if _, err := consumeSpecialIntimacyGift(&p, 4401, map[int]int64{749: 1}, choice, rules, now); err != nil || p.Intimacy[4401] != want || p.Materials[749].Count != 0 || p.IntimacyCommons[4401].SpecialCount != 1 {
			t.Fatal("真实特殊送礼最终舍入或扣费次数错误", choice, p.Intimacy[4401], err)
		}
	}
	for _, row := range []struct {
		base  int
		ratio float64
		want  int
	}{{22, 1.1, 24}, {22, 1.2, 26}, {55, 1.1, 61}, {20, 1.2, 24}, {0, 1.2, 0}} {
		got, err := specialIntimacyHalfUp(row.base, row.ratio)
		if err != nil || got != row.want {
			t.Fatal("有理数half-up边界错误", row, got, err)
		}
	}
	if _, err := specialIntimacyHalfUp(1, math.NaN()); err == nil {
		t.Fatal("非法倍率被允许")
	}
}

func TestRemainingHousePolicyCounterActualRewardsAndPersistence(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	now := time.Unix(1800000000, 0)
	p := houseFrageOccupiedFixture(now)
	w := p.Activities.House
	w.Total, w.SSR = 100, 20
	if count, err := houseFragePolicyPerCount(w); err != nil || count != 0 {
		t.Fatal("从历史总数猜连续失败次数", count, err)
	}
	for i := 0; i < 2; i++ {
		if _, _, err := houseFrageLanding(&p, &HouseFrageSite{ID: 2, Card: 3201, Fixed: 3201}, 1, now); err != nil {
			t.Fatal(err)
		}
	}
	if w.ConsecutiveWithoutSSR != 2 {
		t.Fatal("真实精装残页事件未累计", w)
	}
	if _, _, err := houseFrageLanding(&p, &HouseFrageSite{ID: 3}, 1, now); err != nil || w.ConsecutiveWithoutSSR != 2 {
		t.Fatal("金币格错误累加连续残页事件", w, err)
	}
	if _, _, err := houseFrageLanding(&p, &HouseFrageSite{ID: 2, Card: 2404, Fixed: 2404}, 1, now); err != nil || w.ConsecutiveWithoutSSR != 0 {
		t.Fatal("实际固定典藏未重置", w, err)
	}
	if err := houseFragePolicyReward(w, 1); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p)
	var restored Progress
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if count, err := houseFragePolicyPerCount(restored.Activities.House); err != nil || count != 1 {
		t.Fatal("连续计数重载丢失", count, err)
	}
	for count, factor := range map[int]float64{0: 1, 1: 0.6, 9: 0.6, 10: 0.7, 15: 0.85, 20: 1, 30: 1.2, 40: 1.4, 10001: 1.4} {
		row := activityRow{"ssr_weight": json.RawMessage(`100`)}
		if math.Abs(houseFrageSSRWeight(row, count)-100*factor) > 1e-12 {
			t.Fatal("原生严格大于段位错误", count)
		}
	}
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "")
	old := w.ConsecutiveWithoutSSR
	if err := houseFragePolicyReward(w, 3); err != nil || w.ConsecutiveWithoutSSR != old {
		t.Fatal("关闭政策后仍推进", w, err)
	}
}
