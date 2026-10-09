package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestNativeCraftWorkshopGateCostsAndFurnitureOwnership(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	now := time.Unix(1800000000, 0)
	s.Now = func() time.Time { return now }
	setup := func(p *Progress) error {
		if e := ensureCollection(p, now); e != nil {
			return e
		}
		p.Collection.Rooms[2] = newCollectionRoom(2, now)
		p.Collection.Facilities[7] = CollectionFacility{ID: 7, Level: 2}
		p.Materials[110] = Material{ID: 110, Count: 12, Total: 12}
		p.Materials[12] = Material{ID: 12, Count: 1000, Total: 1000}
		return nil
	}
	if e := s.updateProgress(ctx, c, setup); e != nil {
		t.Fatal(e)
	}
	args := []json.RawMessage{json.RawMessage(`7`), json.RawMessage(`111`), json.RawMessage(`2`)}
	out, e := s.craftRPC(ctx, c, "compose_material", args)
	if e != nil || out[len(out)-1].Method != "call_client_callback" || out[len(out)-1].Args[1].([]any)[0] == nil || c.SelectedAvatarUnsafe().Progress.Materials[111].Count != 2 || c.SelectedAvatarUnsafe().Progress.Materials[110].Count != 0 || c.SelectedAvatarUnsafe().Progress.Materials[12].Count != 800 {
		t.Fatal("原生材料合成回调、扣费或产物错误", e)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	out, e = s.craftRPC(ctx, c, "compose_material", args)
	if e != nil || out[len(out)-1].Args[1].([]any)[0] != nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("材料不足没有原生失败回调/整事务回滚", e)
	}
	if e := s.updateProgress(ctx, c, func(p *Progress) error {
		f := p.Collection.Facilities[7]
		f.Level = 4
		p.Collection.Facilities[7] = f
		p.Materials[19] = Material{ID: 19, Count: 2000000, Total: 2000000}
		p.Materials[12] = Material{ID: 12, Count: 10000, Total: 10000}
		changes := map[int]int64{}
		cards := []string{}
		for _, row := range androidCraft.Recipes[6011005].Sources {
			if e := grantNativeItem(p, int(row[0]), row[1], p.AvatarLevel, now, changes, &cards, 0); e != nil {
				return e
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	out, e = s.craftRPC(ctx, c, "unlock_furniture_compose", nil)
	if e != nil || out[len(out)-1].Args[0] != RetSuccess || c.SelectedAvatarUnsafe().Progress.Materials[19].Count != 0 {
		t.Fatal("原生家具加工2百万解锁成本错误", e)
	}
	before = CloneProgress(c.SelectedAvatarUnsafe().Progress)
	out, e = s.craftRPC(ctx, c, "unlock_furniture_compose", nil)
	if e != nil || out[len(out)-1].Args[0] == RetSuccess || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("重复解锁扣费或重置")
	}
	out, e = s.craftRPC(ctx, c, "compose_furniture_normal", []json.RawMessage{json.RawMessage(`8`), json.RawMessage(`6011005`), json.RawMessage(`1`)})
	if e != nil || out[len(out)-1].Args[1].([]any)[0] != RetSuccess || c.SelectedAvatarUnsafe().Progress.Materials[6011005].Count != 1 || c.SelectedAvatarUnsafe().Progress.Collection.Handbook[11005] == 0 {
		t.Fatal("家具合成没有实际归属和图鉴", e)
	}
}
