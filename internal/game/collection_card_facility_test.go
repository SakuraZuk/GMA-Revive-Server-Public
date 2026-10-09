package game

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestCollectionCultivationStageLedgerAndCompleteCycle(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	now := time.Unix(1800000000, 0)
	s.Now = func() time.Time { return now }
	id := 0
	for fid, rule := range androidCollection.Facilities {
		if rule.Sheet == "facility_card" {
			id = fid
		}
	}
	if id == 0 {
		t.Fatal("原生培育设施不存在")
	}
	rule := androidCollection.Levels[id][1]
	if e := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureCollection(p, now); e != nil {
			return e
		}
		p.Collection.Rooms[androidCollection.Facilities[id].Room] = newCollectionRoom(androidCollection.Facilities[id].Room, now)
		p.Collection.Facilities[id] = CollectionFacility{ID: id, Level: 1}
		p.Materials[rule.Material] = Material{ID: rule.Material, Count: rule.Count + 1, Total: rule.Count + 1}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	rpc := func(method string, values ...int64) []Push {
		t.Helper()
		args := []json.RawMessage{}
		for _, v := range values {
			raw, _ := json.Marshal(v)
			args = append(args, raw)
		}
		out, e := s.collectionCardFacilityRPC(ctx, c, method, args)
		if e != nil {
			t.Fatal(method, e)
		}
		return out
	}
	out := rpc("facility_card_upgrade", int64(id), int64(rule.Material), rule.Count)
	if out[len(out)-1].Args[0] != RetSuccess || c.SelectedAvatarUnsafe().Progress.Collection.Facilities[id].Recycle != int(rule.Count) || c.SelectedAvatarUnsafe().Progress.Materials[rule.Material].Count != 1 {
		t.Fatal("培育进度或扣材错误")
	}
	out = rpc("upgrade_house_facility_card", int64(id))
	if out[len(out)-1].Args[0] == RetSuccess {
		t.Fatal("未领取阶段奖励仍开始下一轮")
	}
	for _, stage := range rule.StageRewards {
		out = rpc("get_facility_card_reward", int64(id), int64(stage[0]))
		if out[len(out)-1].Args[0] != RetSuccess {
			t.Fatal("阶段领奖失败")
		}
		out = rpc("get_facility_card_reward", int64(id), int64(stage[0]))
		if out[len(out)-1].Args[0] == RetSuccess {
			t.Fatal("阶段奖励重复领取")
		}
	}
	out = rpc("upgrade_house_facility_card", int64(id))
	f := c.SelectedAvatarUnsafe().Progress.Collection.Facilities[id]
	if out[len(out)-1].Args[0] != RetSuccess || f.Level != 2 || f.Recycle != 0 || len(f.CardReward) != len(androidCollection.Levels[id][2].StageRewards) {
		t.Fatal("原生培育轮次没有正确重置")
	}
	if e := s.updateProgress(ctx, c, func(p *Progress) error {
		p.Materials[12] = Material{ID: 12, Count: math.MaxInt64, Total: math.MaxInt64}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	_, e := s.collectionCardFacilityRPC(ctx, c, "facility_card_upgrade", []json.RawMessage{json.RawMessage(itoa(id)), json.RawMessage(itoa(rule.Material)), json.RawMessage(`1`)})
	if e == nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("培育奖励溢出没有整事务回滚", e)
	}
}
