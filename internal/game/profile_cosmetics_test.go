package game

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestCosmeticOwnershipExpiryAndPreservedSelection(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	svc := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, svc)
	now := time.Unix(1800000000, 0)
	svc.Now = func() time.Time { return now }
	_, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.SelectedHeadID = 2
		p.SelectedHeadBoxID = 3
		p.OwnedHeadBox = map[int]float64{2: 0, 3: 0}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	props := profileCosmeticProperties(c.SelectedAvatarUnsafe(), now)
	// SelectedAvatar缓存尚未刷新，RPC须重新读取行锁状态，不能覆盖持久选择。
	_ = props
	pushes, err := svc.Handle(ctx, c, "update_head_box_by_client", nil)
	if err != nil || len(pushes) != 5 {
		t.Fatal(err, pushes)
	}
	if c.SelectedAvatarUnsafe().Progress.SelectedHeadID != 2 {
		t.Fatal("迁移覆盖已有头像选择")
	}
	pushes, err = svc.Handle(ctx, c, "change_head_box", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`1`)})
	if err != nil || pushes[0].Args[1].([]any)[0] == 0 {
		t.Fatal("跨类型头像框未拒绝", err)
	}
	timed := 0
	for id, r := range androidProfile.Heads {
		if r.Kind == 2 && r.LimitHours > 0 && r.Dress == 0 {
			timed = id
			break
		}
	}
	if timed == 0 {
		t.Fatal("限时头像框目录缺失")
	}
	_, err = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.OwnedHeadBox[timed] = float64(now.Unix()) - androidProfile.Heads[timed].LimitHours*3600
		p.SelectedHeadBoxID = timed
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(ctx, c, "update_head_box_by_client", nil); err != nil {
		t.Fatal(err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if _, ok := p.OwnedHeadBox[timed]; ok || p.SelectedHeadBoxID != 3 {
		t.Fatal("到期头像框未删除及回退")
	}
	selected, _ := json.Marshal(2)
	pushes, err = svc.Handle(ctx, c, "change_head", []json.RawMessage{json.RawMessage(`2`), selected})
	if err != nil || len(pushes) != 6 {
		t.Fatal(err, pushes)
	}
	stored, err := accounts.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil || stored.Avatars[0].InitialProperties("")["head_id"] != 2 {
		t.Fatal("头像复登丢失", err)
	}
}

func TestHistoricalCardAcquisitionSurvivesRemoval(t *testing.T) {
	p := Progress{Cards: []Card{{CardID: 4401, Time: 100}}}
	ensureObtainedCardHistory(&p)
	p.Cards = nil
	if appendOwnedCard(&p, Card{CardID: 4401, Time: 200}) {
		t.Fatal("移除后重新获得误报首次")
	}
	if !appendOwnedCard(&p, Card{CardID: 1101, Time: 300}) || p.ObtainedCardIDs[4401] != 100 {
		t.Fatal("首次记录被覆盖")
	}
	if countedCardCount(p) != 2 {
		t.Fatal("图鉴统计未按唯一幻书模板计数")
	}
}
