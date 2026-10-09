package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestShopSubscriptionNativeThirtyDaysAndMailboxRollback(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	now := time.Date(2026, 10, 7, 23, 59, 59, 0, time.FixedZone("北京时间", 8*3600))
	s.Now = func() time.Time { return now }
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.Materials[10] = Material{ID: 10, Count: 100, Total: 100}
		p.ClearedDungeons = append(p.ClearedDungeons, 504)
		_, e := buyCommodity(p, 2010001, 1, 0, 60, now)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.CommodityDetails[2010001].SubscribeCount != 1 || len(p.CommodityDetails[2010001].SubscribeJobs) != 1 || p.Materials[10].Count != 70 {
		t.Fatal("首日奖励或发行任务错误")
	}
	start := p.CommodityDetails[2010001].SubscribeJobs[0].StartDay
	if start != now.Add(time.Second).Unix() {
		t.Fatal("月卡日界不是北京时间零点")
	}
	for day := int64(0); day < 29; day++ {
		now = time.Unix(start+day*86400, 0)
		if e := s.updateProgress(ctx, c, func(p *Progress) error { return refreshShopSubscription(s, p, now) }); e != nil {
			t.Fatal(day, e)
		}
		p = c.SelectedAvatarUnsafe().Progress
		if len(p.ShortMailInfo) != int(day+1) || p.CommodityDetails[2010001].SubscribeCount != day+2 {
			t.Fatal("逐日月卡没有准确发行", day)
		}
		for _, mail := range p.ShortMailInfo {
			if mail.Attachments[11] != 200 || mail.ExpiresAt-mail.CreatedAt != 30*86400 {
				t.Fatal("原生月卡奖励或有效期错误")
			}
		}
		before := CloneProgress(p)
		if e := s.updateProgress(ctx, c, func(p *Progress) error { return refreshShopSubscription(s, p, now) }); e != nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
			t.Fatal("同日重复发行", e)
		}
	}
	if len(c.SelectedAvatarUnsafe().Progress.CommodityDetails[2010001].SubscribeJobs) != 0 {
		t.Fatal("已完成订阅任务未清理")
	}
	if e := s.updateProgress(ctx, c, func(p *Progress) error { p.ShortMailInfo = map[int]Mail{}; return refreshShopSubscription(s, p, now) }); e != nil || len(c.SelectedAvatarUnsafe().Progress.ShortMailInfo) != 0 {
		t.Fatal("删除邮件后旧月卡重发", e)
	}
	// 新任务第二日邮箱已满：邮件、发行计数和玩家存档必须整事务回滚。
	if e := s.updateProgress(ctx, c, func(p *Progress) error {
		_, e := buyCommodity(p, 2010001, 1, 0, 60, now)
		if e != nil {
			return e
		}
		p.ShortMailInfo = map[int]Mail{}
		for i := 1; i <= 500; i++ {
			p.ShortMailInfo[i] = Mail{MID: i, UUID: newCardUUID(), Title: "邮箱容量验收"}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	now = time.Unix(nextShopDay(now), 0)
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if e := s.updateProgress(ctx, c, func(p *Progress) error { return refreshShopSubscription(s, p, now) }); e == nil {
		t.Fatal("满邮箱仍发行月卡")
	}
	if !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("满邮箱失败未整体回滚")
	}
}

func TestShopNativeRuneSubcommodityAndCapacityRollback(t *testing.T) {
	now := time.Unix(1800000000, 0)
	p := Progress{Materials: map[int]Material{601: {ID: 601, Count: 1000, Total: 1000}}}
	box, e := buyCommodity(&p, 1011101, 4, 0, 45, now)
	if e != nil || len(p.Runes) != 4 || p.Materials[601].Count != 800 || len(box["runes"].(map[string]any)) != 4 {
		t.Fatal("原生契印子商品未真实发货", e)
	}
	for _, r := range p.Runes {
		if r.Suit != 1101 || r.Star != 5 || r.Level != 1 || r.Position < 1 || r.Position > 4 {
			t.Fatal("契印未依Android权重", r)
		}
	}
	if _, e = buyCommodity(&p, 1011000, 1, 0, 60, now); e == nil {
		t.Fatal("契印目录UI容器当作奖励购买")
	}
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	s.Now = func() time.Time { return now }
	tables, _ := loadRuneTables()
	if e = s.updateProgress(ctx, c, func(p *Progress) error {
		p.Materials[601] = Material{ID: 601, Count: 1000, Total: 1000}
		p.Runes = map[string]Rune{}
		for i := 0; i < tables.Tables.Limits.Max; i++ {
			p.Runes[itoa(i)] = Rune{}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if e = s.updateProgress(ctx, c, func(p *Progress) error { _, e := buyCommodity(p, 1011101, 1, 0, 60, now); return e }); e == nil {
		t.Fatal("满背包仍扣费")
	}
	if !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("契印容量失败未整事务回滚")
	}
}

func TestShopGiftBoxWithoutReplacementAndNativeBonusLibraries(t *testing.T) {
	now := time.Unix(1800000000, 0)
	p := Progress{}
	rules := androidShop.GiftBoxes[209002]
	seen := map[int]bool{}
	for i := 0; i < len(rules); i++ {
		prior := map[int]int64{}
		for id, n := range p.GiftBoxInfo[209002].Received {
			prior[id] = n
		}
		changes := map[int]int64{}
		cards := []string{}
		if e := grantShopGiftBox(&p, 209002, now, changes, &cards); e != nil {
			t.Fatal(e)
		}
		if i == len(rules)-1 {
			if len(p.GiftBoxInfo[209002].Received) != 0 {
				t.Fatal("原生最后一次礼盒没有自动重置")
			}
			continue
		}
		for id, n := range p.GiftBoxInfo[209002].Received {
			if n > prior[id] {
				if seen[id] {
					t.Fatal("礼盒重复抽已取完奖励")
				}
				seen[id] = true
			}
		}
	}
	// 主证据素材库预计算条件：馆主不足时不纳入10001，不放回取完可用条目。
	ids, _, e := nativeLibraryWeights(p, 1, 1)
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range ids {
		if id == 10001 {
			t.Fatal("素材库未执行等级条件")
		}
	}
	rows := []json.RawMessage{json.RawMessage(`[1,99,1,1]`)}
	changes := map[int]int64{}
	cards := []string{}
	if e := grantNativeItemLibraries(&p, rows, 1, 3, now, changes, &cards, 0); e != nil {
		t.Fatal(e)
	}
	if len(changes) == 0 {
		t.Fatal("素材库权重奖励为空")
	}
}
