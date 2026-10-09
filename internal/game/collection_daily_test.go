package game

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestCollectionDailyRewardPriorityDayAndRollback(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, s)
	now := time.Unix(1800000000, 0)
	s.Now = func() time.Time { return now }
	if e := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureCollection(p, now); e != nil {
			return e
		}
		p.UnlockSystems["house_daily_reward"] = 1
		p.Collection.Rooms[1] = newCollectionRoom(1, now)
		p.Collection.Rooms[2] = newCollectionRoom(2, now)
		p.ClearedDungeons = append(p.ClearedDungeons, 604, 910)
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	p := c.SelectedAvatarUnsafe().Progress
	old := p.Materials[4].Count
	amount1, e := collectionDailyAmounts(p, 1)
	if e != nil || amount1[4] != 184000 {
		t.Fatal("未采用最高已通关副本优先级", amount1, e)
	}
	pushes, e := s.collectionDailyRPC(ctx, c, nil)
	if e != nil || !reflect.DeepEqual(pushes[len(pushes)-1].Args[0], []int{RetSuccess}) || c.SelectedAvatarUnsafe().Progress.Materials[4].Count != old+368000 {
		t.Fatal("每日多房奖励或回包错误", e)
	}
	pushes, e = s.collectionDailyRPC(ctx, c, nil)
	if e != nil || reflect.DeepEqual(pushes[len(pushes)-1].Args[0], []int{RetSuccess}) || c.SelectedAvatarUnsafe().Progress.Materials[4].Count != old+368000 {
		t.Fatal("当日重复领奖成功", e)
	}
	// 删除在线对象并重连后仍使用持久化收据，不因重连重复领奖。
	record, e := accounts.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if e != nil {
		t.Fatal(e)
	}
	if record.Avatars[0].Progress.Collection.DailyClaims[1] != collectionDay(now) {
		t.Fatal("每日收据未持久化")
	}
	// 下个日期跨日可领；库存溢出时两房收据和所有资产一起回滚。
	now = now.Add(24 * time.Hour)
	if e := s.updateProgress(ctx, c, func(p *Progress) error {
		p.Materials[4] = Material{Count: math.MaxInt64, Total: math.MaxInt64}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if _, e = s.collectionDailyRPC(ctx, c, nil); e == nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("溢出奖励仍提交收据", e)
	}
}

func TestCollectionWishlistOnlyNativeLimitedFurniture(t *testing.T) {
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
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	args := []json.RawMessage{json.RawMessage(`6`), json.RawMessage(`6031001`)}
	pushes, e := s.collectionWishlistRPC(ctx, c, "set_up_furniture_material_id", args)
	if e != nil || pushes[len(pushes)-1].Args[1].([]any)[0] != RetSuccess || c.SelectedAvatarUnsafe().Progress.Collection.Wishlist != 6031001 {
		t.Fatal("原生限定家具心愿单未保存", e)
	}
	args[1] = json.RawMessage(`6011001`)
	pushes, e = s.collectionWishlistRPC(ctx, c, "set_up_furniture_material_id", args)
	if e != nil || pushes[len(pushes)-1].Args[1].([]any)[0] == RetSuccess || c.SelectedAvatarUnsafe().Progress.Collection.Wishlist != 6031001 {
		t.Fatal("普通家具绕过心愿单候选校验", e)
	}
	if _, e = s.collectionWishlistRPC(ctx, c, "reset_up_furniture_material_id", args[:1]); e != nil || c.SelectedAvatarUnsafe().Progress.Collection.Wishlist != 0 {
		t.Fatal("清空心愿单失败", e)
	}
}
