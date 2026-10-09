package game

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestCollectionProductionKeepsFractionsAndResidentProfit(t *testing.T) {
	now := time.Unix(1800000000, 0)
	p := Progress{AvatarLevel: 60}
	if e := ensureCollection(&p, now); e != nil {
		t.Fatal(e)
	}
	p.Collection.Rooms[1] = newCollectionRoom(1, now)
	p.Collection.Facilities[2] = CollectionFacility{ID: 2, Level: 1, ProduceStart: float64(now.Unix())}
	if e := collectionRefreshFacilities(&p, now); e != nil {
		t.Fatal(e)
	}
	f := p.Collection.Facilities[2]
	rule := androidCollection.Levels[2][1]
	if math.Abs(f.Rate-rule.ProduceNum/rule.Unit) > 1e-12 || f.Storage != rule.Storage {
		t.Fatal("设施基础生产公式不一致", f, rule)
	}
	if e := collectionRefreshFacilities(&p, now.Add(time.Duration(rule.Unit*1.5)*time.Second)); e != nil {
		t.Fatal(e)
	}
	f = p.Collection.Facilities[2]
	if math.Abs(f.Keep-rule.ProduceNum*1.5) > 1e-9 {
		t.Fatal("生产小数丢失", f.Keep)
	}
	r := p.Collection.Rooms[1]
	r.Slots = map[int]int{1: 4401}
	p.Collection.Rooms[1] = r
	if e := collectionRefreshFacilities(&p, time.Unix(int64(f.ProduceStart), 0)); e != nil {
		t.Fatal(e)
	}
	f = p.Collection.Facilities[2]
	expected := rule.ProduceNum / rule.Unit * (1 + androidCollection.Rooms[1].CardProfit[0]*4)
	if math.Abs(f.Rate-expected) > 1e-12 {
		t.Fatal("入住收益lambda没有乘4", f.Rate, expected)
	}
	if e := collectionRefreshFacilities(&p, now.Add(365*24*time.Hour)); e != nil || p.Collection.Facilities[2].Keep != rule.Storage {
		t.Fatal("离线积累超过原生库存上限", e)
	}
}

func TestCollectionGatherUsesNativeAssetsAndNoRepeatedReward(t *testing.T) {
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
		p.Collection.Rooms[1] = newCollectionRoom(1, now)
		p.Collection.Facilities[2] = CollectionFacility{ID: 2, Level: 1, ProduceStart: float64(now.Unix()), Keep: 1.75}
		return collectionRefreshFacilities(p, now)
	}); e != nil {
		t.Fatal(e)
	}
	old := c.SelectedAvatarUnsafe().Progress.Materials[19].Count
	pushes, e := s.collectionProductionRPC(ctx, c, []json.RawMessage{json.RawMessage(`2`)})
	if e != nil || pushes[len(pushes)-1].Method != "on_gather_produce_material" || pushes[len(pushes)-1].Args[0] != 0 || c.SelectedAvatarUnsafe().Progress.Materials[19].Count != old+1 || math.Abs(c.SelectedAvatarUnsafe().Progress.Collection.Facilities[2].Keep-.75) > 1e-9 {
		t.Fatal("设施收获资产、下行或小数错误", e)
	}
	pushes, e = s.collectionProductionRPC(ctx, c, []json.RawMessage{json.RawMessage(`2`)})
	if e != nil || pushes[len(pushes)-1].Args[0] == 0 || c.SelectedAvatarUnsafe().Progress.Materials[19].Count != old+1 {
		t.Fatal("重复收获仍发奖励", e)
	}
}
