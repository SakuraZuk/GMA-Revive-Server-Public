package game

import (
	"context"
	"reflect"
	"testing"
)

func TestLogContractFriendDefaultSourceAndDefenceEmptySlots(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	peer := hexOf(selectedOID(cs[1]))
	own := hexOf(selectedOID(cs[0]))
	out, err := s.Handle(ctx, cs[0], "apply_friend", rawArgs(1, peer, "你好", 0, ""))
	if err != nil || socialCallback(t, out)[0] != RetSuccess {
		t.Fatal("原生默认来源申请失败", err, out)
	}
	if socialSaved(t, store, cs[1]).Progress.Social.Requests[own].Message != "你好" {
		t.Fatal("申请未持久化")
	}
	uuid := cs[0].SelectedAvatarUnsafe().Progress.Cards[0].UUID
	out, err = s.Handle(ctx, cs[0], "set_defence_cards", rawArgs(2, []any{0, uuid, nil}))
	if err != nil || socialCallback(t, out)[0] != RetSuccess {
		t.Fatal("原生防守空槽失败", err, out)
	}
	before := socialSaved(t, store, cs[0]).Progress
	if !reflect.DeepEqual(before.AsyncPvp.Defence, []string{"", uuid, ""}) {
		t.Fatal("空槽位置丢失")
	}
	for _, slots := range []any{[]any{uuid, uuid}, []any{0, nil}, []any{false, uuid}, []any{"外来卡"}} {
		out, err = s.Handle(ctx, cs[0], "set_defence_cards", rawArgs(3, slots))
		if err == nil && socialCallback(t, out)[0] == RetSuccess {
			t.Fatal("非法防守被接受", slots)
		}
		if !reflect.DeepEqual(before, socialSaved(t, store, cs[0]).Progress) {
			t.Fatal("拒绝后改变存档", slots)
		}
	}
}

func TestLogContractBusyRefusalUsesPersistedInvitationsAndIsIdempotent(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	own := hexOf(selectedOID(cs[0]))
	ids := []string{own, hexOf(selectedOID(cs[1])), hexOf(selectedOID(cs[2]))}
	_, err := store.UpdateSocial(ctx, ids, func(v map[string]*Avatar) error {
		for _, id := range ids {
			ensureSocial(&v[id].Progress.Social)
		}
		for _, id := range ids[1:] {
			v[own].Progress.Social.Challenges[id] = SocialFriend{Info: socialProfile(*v[id])}
			v[id].Progress.Social.ChallengeSent[own] = s.Now().Unix()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// 连接尚未收取其他玩家的邀请更新，必须以持久化列表为准。
	for i := 0; i < 2; i++ {
		out, e := s.Handle(ctx, cs[0], "refuse_challenge", rawArgs(10, nil, 1))
		if e != nil || socialCallback(t, out)[0] != RetSuccess {
			t.Fatal(e, out)
		}
	}
	if len(socialSaved(t, store, cs[0]).Progress.Social.Challenges) != 0 {
		t.Fatal("未清除本人邀请")
	}
	for _, c := range cs[1:] {
		p := socialSaved(t, store, c).Progress.Social
		if len(p.ChallengeSent) != 0 || len(p.HumanRefusals) != 1 || p.HumanRefusals[0].Reason != 1 {
			t.Fatal("忙碌拒绝未持久或重复通知", p)
		}
	}
	if _, err = s.Handle(ctx, cs[0], "refuse_challenge", rawArgs(11, nil, 0)); err == nil {
		t.Fatal("空目标允许非忙碌拒绝")
	}
}

func TestLogContractHabitWireAndColdCardProperties(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, av := newBattleConnection(t, ctx, a, s)
	uuid := c.SelectedAvatarUnsafe().Progress.Cards[0].UUID
	out, err := s.Handle(ctx, c, "set_auto_support_habit", rawArgs(uuid, false))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatal(out)
	}
	args := out[0].Args[1].([]any)
	want := map[string]any{"__custom_type": "card.card_habit", "support_habit": false, "battle_habit": 100}
	if args[0] != ObjectID(uuid) || !reflect.DeepEqual(args[1], want) {
		t.Fatal("习惯下行不是原生对象", out)
	}
	identity, err := a.QuickLogin(ctx, ClientInfo{Account: av.Account, Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := identity.Avatars[0].Progress
	habit := cardMgrProperties(p.Cards)[uuid].(map[string]any)["habit_mgr"]
	if !reflect.DeepEqual(habit, map[string]any{"support_habit": false, "battle_habit": 100}) {
		t.Fatal("冷登录习惯丢失", habit)
	}
}

func TestLogContractFavoriteAndScorePromptPersistWithoutRewards(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, av := newBattleConnection(t, ctx, a, s)
	id := c.SelectedAvatarUnsafe().Progress.Cards[0].CardID
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	out, err := s.Handle(ctx, c, "set_card_favorites", rawArgs(30, id, true))
	if err != nil || socialCallback(t, out)[0] != RetSuccess {
		t.Fatal(err, out)
	}
	for i := 0; i < 2; i++ {
		if _, err = s.Handle(ctx, c, "set_game_score_flag", rawArgs("taptap", 1)); err != nil {
			t.Fatal(err)
		}
	}
	identity, err := a.QuickLogin(ctx, ClientInfo{Account: av.Account, Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := identity.Avatars[0].Progress
	if !p.CardFavorites[id] || !reflect.DeepEqual(p.GameScoreFlags["taptap"], []int{1}) || !reflect.DeepEqual(before.Materials, p.Materials) {
		t.Fatal("收藏或评分提示未持久、重复或改变资产")
	}
	if cardCommonMgrPropertiesWithProgress(p)[intString(id)].(map[string]any)["is_favorite"] != true {
		t.Fatal("收藏未投影")
	}
	out, err = s.Handle(ctx, c, "set_card_favorites", rawArgs(31, 999999, true))
	if err != nil || socialCallback(t, out)[0] == RetSuccess {
		t.Fatal("接受外来卡牌收藏", err, out)
	}
}
