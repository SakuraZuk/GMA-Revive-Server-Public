package game

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestShopPurchaseNativeRPCAndTransactionalRollback(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, av := newBattleConnection(t, ctx, a, s)
	s.Now = func() time.Time { return time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC) }
	if err := s.updateProgress(ctx, c, func(p *Progress) error { p.Materials[11] = Material{ID: 11, Count: 1000, Total: 1000}; return nil }); err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage(`1010002`), json.RawMessage(`2`), json.RawMessage(`0`)}
	pushes, err := s.Handle(ctx, c, "buy_commodity", args)
	if err != nil || pushes[len(pushes)-1].Method != "on_buy_commodity" || pushes[len(pushes)-1].Args[0] != 0 {
		t.Fatal("原生商店下行失败", err, pushes)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Materials[11].Count != 540 || p.Materials[501].Count != 3 || p.CommodityDetails[1010002].Total != 2 {
		t.Fatal("购买扣费发货次数错误")
	}
	stored, err := a.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil || !reflect.DeepEqual(p, stored.Avatars[0].Progress) {
		t.Fatal("商店购买未持久", err)
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		m := p.Materials[501]
		m.Count = math.MaxInt64
		m.Total = math.MaxInt64
		p.Materials[501] = m
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	_, err = s.Handle(ctx, c, "buy_commodity", args)
	if err == nil {
		t.Fatal("溢出未拒绝")
	}
	stored, err = a.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil || !reflect.DeepEqual(before, stored.Avatars[0].Progress) {
		t.Fatal("购买失败未整事务回滚", err)
	}
	_ = av
}

func TestShopNativePriceLimitDiscountAndPeriods(t *testing.T) {
	now := time.Date(2026, 10, 31, 23, 59, 59, 0, time.FixedZone("北京时间", 8*3600))
	p := Progress{Materials: map[int]Material{11: {ID: 11, Count: 10000, Total: 10000}}}
	for i, cost := range []int64{50, 100, 150} {
		before := p.Materials[11].Count
		if _, err := buyCommodity(&p, 1010001, 1, 0, 50, now); err != nil {
			t.Fatal(err)
		}
		if before-p.Materials[11].Count != cost || p.CommodityDetails[1010001].Daily != int64(i+1) {
			t.Fatal("原生阶梯价错误")
		}
	}
	d := p.CommodityDetails[1010001]
	if err := refreshCommodity(&d, now.Add(time.Second)); err != nil || d.Daily != 0 || d.Monthly != 0 || d.Total != 3 {
		t.Fatal("跨月限购未刷新", err, d)
	}
	if _, err := buyCommodity(&p, 1010001, 2, 0, 50, now); err == nil {
		t.Fatal("非批量商品接受批量")
	}
	q := CloneProgress(p)
	if _, err := buyCommodity(&q, 5001001, 1, 0, 50, now); err == nil {
		t.Fatal("过期活动商品未拒绝")
	}
	d.LastPurchase = now.Add(time.Hour).Unix()
	if refreshCommodity(&d, now) == nil {
		t.Fatal("时钟回拨未拒绝")
	}
	p.UnlockSystems = map[string]int{"panel_async_pvp": 1}
	p.Materials[14] = Material{ID: 14, Count: 1000, Total: 1000}
	if _, err := buyCommodity(&p, 1020001, 1, 0, 50, now); err != nil || p.Materials[14].Count != 920 {
		t.Fatal("原生折扣错误", err)
	}
	if _, err := buyCommodity(&p, 1020001, 1, 0, 50, now); err != nil {
		t.Fatal(err)
	}
	if _, err := buyCommodity(&p, 1020001, 1, 0, 50, now); err == nil {
		t.Fatal("周限购未阻止重复购买")
	}
}

func TestShopUnlockNativeGroupsAndUnknown(t *testing.T) {
	conditions := [][][]json.RawMessage{{{json.RawMessage(`1`), json.RawMessage(`1`)}, {json.RawMessage(`2`), json.RawMessage(`10`)}}, {{json.RawMessage(`2`), json.RawMessage(`15`)}}}
	if ok, _ := shopConditions(conditions, Progress{}, 10); ok {
		t.Fatal("组间未执行且")
	}
	if ok, err := shopConditions(conditions, Progress{}, 15); !ok || err != nil {
		t.Fatal("组内未执行或", err)
	}
	unknown := [][][]json.RawMessage{{{json.RawMessage(`13`), json.RawMessage(`"382#0"`)}}}
	if _, err := shopConditions(unknown, Progress{}, 50); err == nil {
		t.Fatal("未知条件被猜测放行")
	}
}

func TestShopInstantFixedGiftAndOverflowRollback(t *testing.T) {
	now := time.Now()
	p := Progress{Materials: map[int]Material{11: {ID: 11, Count: 3000, Total: 3000}}}
	box, err := buyCommodity(&p, 1010003, 1, 0, 50, now)
	if err != nil || p.Materials[501].Count != 11 || p.Materials[70306].Count != 0 || box["materials"].(map[int]int64)[501] != 11 {
		t.Fatal("即时礼盒未按Android奖励展开", err)
	}
	if p.Materials[11].Count != 700 {
		t.Fatal("礼盒扣费错误")
	}
	q := Progress{Runes: map[string]Rune{"初始": {Level: 1}}}
	ensureAchievements(&q)
	reconcileAchievementState(&q, 1, now)
	if q.Achievements[303101].Targets[303101] != 0 {
		t.Fatal("契印初始内部等级1误计强化+1")
	}
}
