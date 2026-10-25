package dbstore

import (
	"context"
	"encoding/json"
	"hs-server/internal/game"
	"hs-server/internal/hotfix"
	"reflect"
	"testing"
	"time"
)

func TestPostgresBeginnerPromiseColdLoginAndRollback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	info := game.ClientInfo{Account: "启程保底冷恢复", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, s, info, &now)
	if _, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Materials[11] = game.Material{ID: 11, Count: 2760, Total: 2760}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	draw := func(count int) {
		t.Helper()
		out, err := svc.Handle(ctx, c, "random_cards", pgSocialArgs(8, 3, count))
		if err != nil || len(out) == 0 || out[len(out)-1].Args[1].([]any)[0] != 0 {
			t.Fatal("启程实库抽卡失败", err, out)
		}
	}
	for i := 0; i < 5; i++ {
		draw(1)
	}
	svc.Detach(c)
	svc = game.New(New(s.pool), &hotfix.Catalog{})
	svc.Now = func() time.Time { return now }
	c = game.NewConnection()
	defer svc.Detach(c)
	raw, _ := json.Marshal(info)
	if _, err := svc.Handle(ctx, c, "quick_login", []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BecomePlayer(c); err != nil {
		t.Fatal(err)
	}
	draw(1)
	id, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := id.Avatars[0].Progress
	if p.RandomCardsRecord[3].RandomCount != 6 || p.PromiseCardsRecord[1007].Count != 6 || p.PromiseCardsRecord[1007].FinishCount < 1 {
		t.Fatal("冷恢复第六抽未完成保底", p.PromiseCardsRecord)
	}
	if len(p.Cards) < 6 {
		t.Fatal("缺少实际入账卡")
	}
	found := false
	for _, card := range p.Cards[len(p.Cards)-6:] {
		switch card.CardID {
		case 2403, 2404, 2405, 2502, 3102, 3203, 4405:
			found = true
		}
	}
	if !found {
		t.Fatal("保底记录完成但未入账典藏")
	}
	draw(10)
	for i := 0; i < 4; i++ {
		draw(1)
	}
	id, err = New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p = id.Avatars[0].Progress
	if p.Materials[11].Count != 0 || p.RandomCardsRecord[3].RandomCount != 20 {
		t.Fatal("成本或上限计数异常")
	}
	for rule := 1002; rule <= 1011; rule++ {
		if p.PromiseCardsRecord[rule].Count != 20 || p.PromiseCardsRecord[rule].FinishCount < 1 {
			t.Fatal("保底记录持久化异常", rule)
		}
	}
	out, err := svc.Handle(ctx, c, "random_cards", pgSocialArgs(9, 3, 1))
	if err != nil || out[len(out)-1].Args[1].([]any)[0] == 0 {
		t.Fatal("超出上限未拒绝", err)
	}
	id, err = New(s.pool).QuickLogin(ctx, info)
	if err != nil || !reflect.DeepEqual(p, id.Avatars[0].Progress) {
		t.Fatal("失败请求修改持久记录", err)
	}
}
