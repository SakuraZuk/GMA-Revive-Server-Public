package game

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestLoggedPreferencesNativeCallbacksOwnershipAndColdPersistence(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	uuid := c.SelectedAvatarUnsafe().Progress.Cards[0].UUID
	for _, v := range []struct {
		method string
		args   []any
	}{
		{"set_card_vo", []any{71, 4401, 1}},
		{"set_show_cards", []any{72, []any{uuid, nil, nil}}},
		{"set_girl_random_enable", []any{73, false}},
		{"set_explore_auto_agent", []any{74, androidMikuActivityType, true}},
		{"change_dungeon_skip_edit_state", []any{75, 4102, true}},
	} {
		out, err := s.Handle(ctx, c, v.method, rawArgs(v.args...))
		if err != nil {
			t.Fatal(v.method, err)
		}
		last := out[len(out)-1]
		if last.Method != "call_client_callback" || last.Args[0] != v.args[0] {
			t.Fatal("回调编号未沿用原生", v.method, out)
		}
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	out, err := s.Handle(ctx, c, "set_show_cards", rawArgs(76, []any{"不属于玩家的卡"}))
	if err != nil || out[len(out)-1].Args[1].([]any)[0] == RetSuccess || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("外来UUID设置未整体拒绝", err, out)
	}
	identity, err := accounts.QuickLogin(ctx, ClientInfo{Account: av.Account, Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := identity.Avatars[0].Progress
	if p.CardVoices[4401] != 1 || p.GirlRandomEnable == nil || *p.GirlRandomEnable || !p.ExploreAutoAgent[androidMikuActivityType] || !p.DungeonSkipEditState[4102] {
		t.Fatal("偏好重登录丢失", p.CardVoices)
	}
	if !reflect.DeepEqual(showCardsProperties(p), []any{uuid, nil, nil}) || cardCommonMgrPropertiesWithProgress(p)["4401"].(map[string]any)["card_vo"] != 1 {
		t.Fatal("原生属性投影错误")
	}
}

func TestRanklessCalendarInitializationDoesNotRescanAllPlayers(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	now := time.Date(2026, 10, 9, 4, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, accounts, s)
	store := &observedWriteStore{FixtureAccounts: accounts}
	s.Accounts = store
	if err := s.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error { p.Activities.Calendar = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	before := store.CalendarCalls
	if err := s.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	if c.SelectedAvatarUnsafe().Progress.Activities.Calendar == nil || store.CalendarCalls != before {
		t.Fatal("无排行历史角色仍锁全服存档", store.CalendarCalls, before)
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.Activities.Calendar = nil
		p.Activities.Nian = map[int]ActivityProgress{20200001: {Ranked: true, RankAt: now.Unix(), RankDamage: 10}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	if store.CalendarCalls != before+1 {
		t.Fatal("有排行历史角色错误走单角色结算")
	}
}
