package game

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestShopWangyanRewardUsesSupplyAndKeepsExplicitOverflow(t *testing.T) {
	now := time.Date(2026, 10, 8, 6, 0, 0, 0, time.UTC)
	p := NewProgress(1, now)
	w := ensureWangyan(&p, now)
	w.Supply.Value = w.Supply.Max
	changes := map[int]int64{}
	cards := []string{}
	if err := grantNativeItem(&p, 206001, 25, 1, now, changes, &cards, 0); err != nil {
		t.Fatal(err)
	}
	if w.Supply.Value != w.Supply.Max+25 || changes[206001] != 25 || p.Materials[206001].Count != 0 || p.Materials[206001].Total != 0 {
		t.Fatal("显式补给奖励应入活动供给并保留超额", w.Supply, changes, p.Materials[206001])
	}
	if err := refreshWangyanSupply(w, now.Add(24*time.Hour)); err != nil || w.Supply.Value != w.Supply.Max+25 {
		t.Fatal("自然回复误缩减超额或持续额外生产", err, w.Supply)
	}
	// 先结算未领取的自然回复，再获得额外补给，不把回复余量当额外奖励。
	w.Supply.Value = w.Supply.Max - 2
	later := now.Add(24 * time.Hour)
	w.Supply.LastTime = float64(later.UnixNano())/1e9 - float64(w.Supply.Interval*10)
	if err := grantNativeItem(&p, 206001, 1, 1, later, changes, &cards, 0); err != nil ||
		w.Supply.Value != w.Supply.Max+1 || changes[206001] != 26 {
		t.Fatal("补给奖励前未结算回复或收据错误", err, w.Supply, changes)
	}
	if err := grantNativeItem(&p, 206001, int64(math.MaxInt32)+1, 1, later, changes, &cards, 0); err == nil {
		t.Fatal("补给奖励超出原生Int范围仍被接受")
	}
}

func TestShopWangyanRewardPersistenceAndWholeRewardRollback(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	now := time.Date(2026, 10, 8, 6, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	c, av := newBattleConnection(t, ctx, accounts, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		w := ensureWangyan(p, now)
		w.Supply.Value = w.Supply.Max
		p.Materials[12] = Material{ID: 12, Count: math.MaxInt64, Total: math.MaxInt64}
		return grantNativeItem(p, 206001, 7, 1, now, map[int]int64{}, &[]string{}, 0)
	}); err != nil {
		t.Fatal(err)
	}
	before, err := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if err != nil || before.Activities.Wangyan == nil || before.Activities.Wangyan.Supply.Value != before.Activities.Wangyan.Supply.Max+7 {
		t.Fatal("实际补给奖励未持久化", err)
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		changes := map[int]int64{}
		cards := []string{}
		if err := grantNativeItem(p, 206001, 3, 1, now, changes, &cards, 0); err != nil {
			return err
		}
		return grantNativeItem(p, 12, math.MaxInt64, 1, now, changes, &cards, 0)
	}); err == nil {
		t.Fatal("后续物品奖励故障未触发")
	}
	after, err := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(CloneProgress(before), CloneProgress(after)) {
		t.Fatal("补给先成功而后奖励失败没有整体回滚", err)
	}
}
