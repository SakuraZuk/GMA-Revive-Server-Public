package game

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// 从原表真实商店兑换到残页合成；资格不足和重试均不能凭空赠送角色。
func TestMikuOriginalShopFragmentsComposeAndColdPersistence(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, shanghaiZone)
	s.Now = func() time.Time { return now }
	activitySchedulesForTest(t, now)
	c, av := newBattleConnection(t, ctx, a, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.AvatarLevel = 60
		p.ClearedDungeons = append(p.ClearedDungeons, 601, 510)
		p.Materials[208095] = Material{ID: 208095, Count: 150000, Total: 49999}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	out, err := s.Handle(ctx, c, "buy_commodity", rawArgs(5010009, 50, 0))
	if err != nil || out[len(out)-1].Args[0] == 0 || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("累计资格不足仍兑换或拒绝无回包", err, out)
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		m := p.Materials[208095]
		m.Total = 150000
		p.Materials[208095] = m
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out, err = s.Handle(ctx, c, "buy_commodity", rawArgs(5010009, 50, 0))
	if err != nil || out[len(out)-1].Args[0] != 0 {
		t.Fatal("原表初音兑换被拒绝", err, out)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Materials[3411].Count != 50 || p.Materials[208095].Count != 0 || p.CommodityDetails[5010009].Total != 50 {
		t.Fatal("原表50残页及150000和声实际资产不符")
	}
	before = CloneProgress(p)
	out, err = s.Handle(ctx, c, "buy_commodity", rawArgs(5010009, 1, 0))
	if err != nil || out[len(out)-1].Args[0] == 0 || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("限购耗尽仍发残页", err)
	}
	out, err = s.Handle(ctx, c, "compose_cards", rawArgs(93, []int{3411}))
	if err != nil {
		t.Fatal("原表初音合成失败", err)
	}
	p = c.SelectedAvatarUnsafe().Progress
	if !ownsCardID(p, 3411) || p.Materials[3411].Count != 0 || p.ObtainedCardIDs[3411] != now.Unix() {
		t.Fatal("初音合成没有真实原件/历史")
	}
	identity, err := a.QuickLogin(ctx, ClientInfo{Account: av.Account, Password: "pw", Hostnum: 1})
	if err != nil || !reflect.DeepEqual(p, identity.Avatars[0].Progress) {
		t.Fatal("初音获取冷存档不一致", err)
	}
	before = CloneProgress(p)
	_, err = s.Handle(ctx, c, "compose_cards", rawArgs(94, []int{3411}))
	if err != nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("残页耗尽重试再次合成", err)
	}
}
