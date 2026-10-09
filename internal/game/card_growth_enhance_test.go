package game

import (
	"testing"
	"time"
)

func TestNativeEnhanceUsesRarityAndInheritsConsumedEnhancements(t *testing.T) {
	now := time.Unix(1800000000, 0)
	target := newCard(4401, 1, now)
	target.Grade = 5
	material := newCard(4401, 1, now)
	material.EnhanceCount = 2
	p := Progress{Cards: []Card{target, material}, Materials: map[int]Material{12: {ID: 12, Count: 100000, Total: 100000}}}
	box, e := enhanceCardNative(&p, target.UUID, []string{material.UUID}, now)
	if e != nil || len(p.Cards) != 1 || p.Cards[0].EnhanceCount != 3 || p.Cards[0].SkillEnhanceCount != 2 || p.Materials[12].Count != 70000 || box["__custom_type"] != "box.box" {
		t.Fatal("补完稀有度成本或继承阶段错误", e, p.Cards, p.Materials[12])
	}
	if len(nativeCardEnhanceIDs(p.Cards[0])) != 6 {
		t.Fatal("补完没有同步原生属性ID")
	}
	common := newCard(4, 1, now)
	p.Cards = append(p.Cards, common)
	if _, e := enhanceCardNative(&p, target.UUID, []string{common.UUID}, now); e != nil || p.Cards[0].EnhanceCount != 4 {
		t.Fatal("原生通用补完幻书不适用", e)
	}
	locked := newCard(4401, 1, now)
	locked.Lock = 1
	p.Cards = append(p.Cards, locked)
	if _, e := enhanceCardNative(&p, target.UUID, []string{locked.UUID}, now); e == nil {
		t.Fatal("锁定幻书可以被消耗")
	}
}
