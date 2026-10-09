package game

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestLimitedFrameStartsAtClaimAndRepeatedClaimRestartsSevenDays(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "")
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	now := time.Unix(1800000000, 500000000)
	s.Now = func() time.Time { return now }
	claim := func() {
		t.Helper()
		if e := s.updateProgress(ctx, c, func(p *Progress) error {
			changes := map[int]int64{}
			cards := []string{}
			if e := grantNativeItem(p, 80008, 1, p.AvatarLevel, s.Now(), changes, &cards, 0); e != nil {
				return e
			}
			if changes[80008] != 1 || len(cards) != 0 {
				t.Fatal("限时框奖励收据未按真实材料记录", changes, cards)
			}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
	}
	claim()
	first := float64(now.UnixNano()) / 1e9
	p := c.SelectedAvatarUnsafe().Progress
	if androidShop.Materials[80008].Target != 1010 || androidProfile.Heads[1010].LimitHours != 168 || p.OwnedHeadBox[1010] != first || p.Materials[80008].Count != 0 {
		t.Fatal("限时框目标、领取时刻或库存语义错误", p.OwnedHeadBox)
	}
	now = now.Add(2 * 24 * time.Hour)
	claim()
	second := float64(now.UnixNano()) / 1e9
	p = c.SelectedAvatarUnsafe().Progress
	if p.OwnedHeadBox[1010] != second || p.OwnedHeadBox[1010]+168*3600 != second+7*86400 {
		t.Fatal("重复领取没有从本次领取重计标准7天")
	}
	// 原生wire保存开始时刻，不能把旧期限再加7天编码为未来的领取时刻。
	if p.OwnedHeadBox[1010]+168*3600 == first+14*86400 {
		t.Fatal("重复领取错误叠加剩余天数")
	}
	stored, e := a.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if e != nil || stored.Avatars[0].Progress.OwnedHeadBox[1010] != second {
		t.Fatal("限时框领取时刻未持久化", e)
	}
	p.SelectedHeadID = 1
	p.SelectedHeadBoxID = 1010
	ensureProfileCosmetics(&p, AvatarInfo{}, now.Add(7*24*time.Hour-time.Millisecond))
	if _, owned := p.OwnedHeadBox[1010]; !owned {
		t.Fatal("头像框提前到期")
	}
	ensureProfileCosmetics(&p, AvatarInfo{}, now.Add(7*24*time.Hour))
	if _, owned := p.OwnedHeadBox[1010]; owned || p.SelectedHeadBoxID != 3 {
		t.Fatal("精确到期时未删除并回退默认框")
	}
}

func TestLimitedFrameProtectsPermanentClockAndUnknownMultiCopyDuration(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "")
	now := time.Unix(1800000000, 0)
	p := Progress{OwnedHeadBox: map[int]float64{1010: 0}}
	if e := grantProfileHeadBox(&p, 80008, 1010, 1, now); e != nil || p.OwnedHeadBox[1010] != 0 {
		t.Fatal("永久拥有被临时奖励缩短", e)
	}
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1), float64(now.Unix() + 1)} {
		p.OwnedHeadBox[1010] = bad
		if e := grantProfileHeadBox(&p, 80008, 1010, 1, now); e == nil {
			t.Fatal("非法或未来领取时刻被覆盖", bad)
		}
	}
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	s.Now = func() time.Time { return now }
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	e := s.updateProgress(ctx, c, func(p *Progress) error {
		changes := map[int]int64{}
		cards := []string{}
		if e := grantNativeItem(p, 12, 7, p.AvatarLevel, now, changes, &cards, 0); e != nil {
			return e
		}
		return grantNativeItem(p, 80008, 2, p.AvatarLevel, now, changes, &cards, 0)
	})
	if e == nil || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("未取证的双份期限没有整体回滚", e)
	}
	// 80040只有占位文案、1小时表值，不能自动假定与PVP6种领取起算源相同。
	if e := grantProfileHeadBox(&p, 80040, 1041, 1, now); e == nil {
		t.Fatal("无领取起算证据的限时框被发放")
	}
}
