package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestShopApprovedFivePermanentGoodsRetainPriceAndDailyLimit(t *testing.T) {
	t.Setenv("HS_SERVER_OPEN_TIME", "")
	ids := []int{1071011, 1071012, 1071013, 1071014, 2010010}
	for _, id := range ids {
		t.Run(intString(id), func(t *testing.T) {
			rule := androidShop.Commodities[id]
			if !shopPermanentOpenDays(rule) {
				t.Fatal("已批准永久商品仍要求开服时间")
			}
			for _, stamp := range []int64{1800000000, 1800000000 + 3650*86400} {
				p := NewProgress(60, time.Unix(stamp, 0))
				p.AvatarLevel = 60
				p.UnlockSystems[androidShop.Shops[rule.Shop].System] = 1
				p.Materials[rule.Coin] = Material{ID: rule.Coin, Count: 100000, Total: 100000}
				before := p.Materials[rule.Coin].Count
				box, err := buyCommodity(&p, id, 1, 0, 60, time.Unix(stamp, 0))
				if err != nil || box == nil || p.Materials[rule.Coin].Count >= before || p.CommodityDetails[id].Total != 1 {
					t.Fatal("永久商品未正常扣费发奖/记录购买", err, box)
				}
				wantCost := int64(300)
				if id == 2010010 {
					wantCost = 24
				}
				if before-p.Materials[rule.Coin].Count != wantCost {
					t.Fatal("永久开放改变原生价格")
				}
				if id == 2010010 {
					snapshot := CloneProgress(p)
					if _, err := buyCommodity(&p, id, 1, 0, 60, time.Unix(stamp, 0)); err == nil || !reflect.DeepEqual(snapshot.Materials, p.Materials) || !reflect.DeepEqual(snapshot.CommodityDetails, p.CommodityDetails) {
						t.Fatal("永久开放绕过每日一次或失败扣费/增加购买次数", err)
					}
				}
			}
		})
	}
	other := androidShop.Commodities[2010010]
	other.ID = 9999999
	if shopPermanentOpenDays(other) {
		t.Fatal("未知商品被自动永久化")
	}
	other = androidShop.Commodities[2010010]
	days := 8
	other.OpenDays = &days
	if shopPermanentOpenDays(other) {
		t.Fatal("原生期限漂移被自动永久化")
	}
}

func TestShopPermanentDailyLimitActualRPCFullRollback(t *testing.T) {
	t.Setenv("HS_SERVER_OPEN_TIME", "")
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	s.Now = func() time.Time { return time.Unix(1800000000, 0) }
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.AvatarLevel = 60
		p.UnlockSystems[androidShop.Shops[201].System] = 1
		p.Materials[10] = Material{ID: 10, Count: 1000, Total: 1000}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage(`2010010`), json.RawMessage(`1`), json.RawMessage(`0`)}
	if _, err := s.Handle(ctx, c, "buy_commodity", args); err != nil {
		t.Fatal(err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	pushes, err := s.Handle(ctx, c, "buy_commodity", args)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, response := range pushes {
		if response.Method == "on_buy_commodity" && len(response.Args) > 0 && response.Args[0] == androidShop.Errors["RET_SHOP_GOOD_HAS_SOLD_OUT"] {
			found = true
		}
	}
	if !found {
		t.Fatal("超过每日限购未返回原生拒绝", pushes)
	}
	stored, err := a.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) || !reflect.DeepEqual(before, stored.Avatars[0].Progress) {
		t.Fatal("真实购买RPC拒绝没有整事务回滚", err)
	}
}
