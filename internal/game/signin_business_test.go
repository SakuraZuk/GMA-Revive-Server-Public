package game

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestSigninMonthlyIsIdempotentPerShanghaiDay(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	now := time.Date(2026, 10, 7, 1, 0, 0, 0, time.UTC)
	service := New(accounts, nil)
	service.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, accounts, service)
	first, err := service.Handle(ctx, c, "signin_monthly", nil)
	if err != nil || len(first) != 3 {
		t.Fatalf("首次签到失败: %v %#v", err, first)
	}
	second, err := service.Handle(ctx, c, "signin_monthly", nil)
	if err != nil || len(second) != 3 {
		t.Fatalf("重复签到失败: %v %#v", err, second)
	}
	got, ok := c.SelectedAvatar()
	if !ok || got.Progress.SigninTimes != 1 {
		t.Fatalf("签到未幂等: %#v", got.Progress)
	}
}

func TestSigninBoxMonthResetAndOverflowRollback(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	now := time.Date(2026, 1, 31, 16, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	c, av := newBattleConnection(t, ctx, accounts, s)
	_, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.SigninMonth = "2026-01"
		p.SigninTimes = 31
		p.SigninDay = "2026-01-31"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Handle(ctx, c, "signin_monthly", nil)
	if err != nil {
		t.Fatal(err)
	}
	box := first[2].Args[1].(map[string]any)
	if first[2].Method != "on_signin_monthly" || box["__custom_type"] != "box.box" || len(box["materials"].(map[string]any)) == 0 || c.SelectedAvatarUnsafe().Progress.SigninTimes != 1 {
		t.Fatal("月切换或原生box错误")
	}
	second, err := s.Handle(ctx, c, "signin_monthly", nil)
	if err != nil || len(second[2].Args[1].(map[string]any)["materials"].(map[string]any)) != 0 {
		t.Fatal("同日重复发奖")
	}
	now = now.Add(24 * time.Hour)
	before, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Materials[11] = Material{ID: 11, Count: math.MaxInt64, Total: math.MaxInt64}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Handle(ctx, c, "signin_monthly", nil); err == nil {
		t.Fatal("溢出奖励被接受")
	}
	after, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, after) {
		t.Fatal("失败签到部分提交")
	}
}
