package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestCollectionFusionRejectsUnknownPoolAndSettlesConfiguredReward(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	now := time.Unix(1800000000, 0)
	s.Now = func() time.Time { return now }
	if e := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureCollection(p, now); e != nil {
			return e
		}
		p.Collection.Facilities[7] = CollectionFacility{ID: 7, Level: 4, ComposeUnlocked: true, ProduceStart: float64(now.Unix())}
		p.Materials[19] = Material{ID: 19, Count: 60000, Total: 60000}
		p.Collection.Furniture[31001] = 3
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	args := []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`{"6031001":3}`)}
	t.Setenv("HS_FURNITURE_FUSION_POOLS", "")
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	pushes, e := s.collectionFusionRPC(ctx, c, args)
	if e != nil || pushes[len(pushes)-1].Args[1].([]any)[0] == RetSuccess || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("未配置候选池仍扣费或生成奖励", e)
	}
	// 明确的测试池只用于该测试，三组均指向同一个真实限定家具。
	t.Setenv("HS_FURNITURE_FUSION_POOLS", `{"3:1":[[6031001,1]],"3:2":[[6031001,1]],"3:3":[[6031001,1]]}`)
	pushes, e = s.collectionFusionRPC(ctx, c, args)
	p := c.SelectedAvatarUnsafe().Progress
	if e != nil || pushes[len(pushes)-1].Args[1].([]any)[0] != RetSuccess || p.Materials[19].Count != 30000 || collectionFurnitureCount(p, 31001) != 1 || p.Collection.Handbook[31001] != 1 {
		t.Fatal("家具融合消费、奖励或图鉴未真实结算", e)
	}
	// 原料不足时交易回滚，包括先扣的榫卯。
	before = CloneProgress(p)
	pushes, e = s.collectionFusionRPC(ctx, c, args)
	if e != nil || pushes[len(pushes)-1].Args[1].([]any)[0] == RetSuccess || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("家具融合原料不足仍提交成本", e)
	}
}
