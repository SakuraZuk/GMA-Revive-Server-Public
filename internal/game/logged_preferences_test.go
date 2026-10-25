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
	if p.CardVoices[4401] != 1 || p.GirlRandomEnable == nil || *p.GirlRandomEnable || !p.ExploreAutoAgent[androidMikuActivityType] || !p.DungeonSkipEditState[900000001] {
		t.Fatal("偏好重登录丢失", p.CardVoices, p.DungeonSkipEditState)
	}
	props := identity.Avatars[0].InitialProperties(av.Account)
	if props["dungeon_skip_edit_data"].(map[string]any)["900000001"] != true {
		t.Fatal("登录没有按原生组合键下发跳过布阵", props["dungeon_skip_edit_data"])
	}
	out, err = s.Handle(ctx, c, "change_explore_skip_edit_state", rawArgs(77, androidMikuActivityType, true))
	if err != nil || out[len(out)-1].Args[1].([]any)[0] != true || !c.SelectedAvatarUnsafe().Progress.ExploreSkipEdit[androidMikuActivityType] {
		t.Fatal("探索跳过布阵没有按活动类型保存", err, out)
	}
	if !reflect.DeepEqual(showCardsProperties(p), []any{uuid, nil, nil}) || cardCommonMgrPropertiesWithProgress(p)["4401"].(map[string]any)["card_vo"] != 1 {
		t.Fatal("原生属性投影错误")
	}
}

func TestLiveLogPreferencesPersistAndRejectWithoutChangingAssets(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	uuid := c.SelectedAvatarUnsafe().Progress.Cards[0].UUID
	out, err := s.Handle(ctx, c, "set_auto_battle_habit", rawArgs(uuid, 100))
	if err != nil || len(out) != 1 || out[0].Method != "sync_battle_method" || out[0].Args[0] != "on_set_auto_battle_habit" {
		t.Fatal("战斗习惯没有按同步方法回推", err, out)
	}
	out, err = s.Handle(ctx, c, "set_auto_support_habit", rawArgs(uuid, true))
	if err != nil || len(out) != 1 {
		t.Fatal("助战习惯回推丢失", err, out)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if _, err = s.Handle(ctx, c, "set_auto_battle_habit", rawArgs("不属于玩家", 1)); err == nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("外来卡牌习惯被接受", err)
	}
	out, err = s.Handle(ctx, c, "save_rune_auto_decompose_stars", rawArgs(81, []int{1, 4}))
	changed, _ := out[0].Args[0].([]any)
	if err != nil || len(changed) != 2 || changed[0] != "rune_auto_decompose_stars" || out[1].Args[1].([]any)[0] != RetSuccess {
		t.Fatal("契印自动分解没有保存并回调", err, out)
	}
	out, err = s.Handle(ctx, c, "save_rune_auto_decompose_stars", rawArgs(82, []int{5}))
	if err != nil || out[0].Method != "call_client_callback" || out[0].Args[1].([]any)[0] == RetSuccess {
		t.Fatal("超限星级没有回调拒绝", err, out)
	}
	out, err = s.Handle(ctx, c, "change_channel", rawArgs(12))
	if err != nil || out[1].Method != "on_change_channel" || out[1].Args[0] != RetSuccess {
		t.Fatal("频道切换没有确认", err, out)
	}
	if _, err = s.Handle(ctx, c, "change_channel", rawArgs(0)); err != nil || c.SelectedAvatarUnsafe().Progress.ChatChannel != 12 {
		t.Fatal("非法频道改变了已保存频道", err)
	}
	identity, err := accounts.QuickLogin(ctx, ClientInfo{Account: av.Account, Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := identity.Avatars[0].Progress
	if p.AutoBattleHabits[uuid][battleHabitKey] != 100 || p.AutoBattleHabits[uuid][supportHabitKey] != 1 || p.ChatChannel != 12 || p.RuneAutoDecomposeStars == nil || !reflect.DeepEqual(*p.RuneAutoDecomposeStars, []int{1, 4}) {
		t.Fatal("重登录丢失习惯、频道或分解星级", p.AutoBattleHabits, p.ChatChannel, p.RuneAutoDecomposeStars)
	}
	props := identity.Avatars[0].InitialProperties(av.Account)
	if props["chat_channel"] != 12 || !reflect.DeepEqual(props["rune_auto_decompose_stars"], []int{1, 4}) {
		t.Fatal("登录属性没有投影频道和分解星级", props["chat_channel"], props["rune_auto_decompose_stars"])
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
