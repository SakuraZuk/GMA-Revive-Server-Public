package game

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func friendAssistWorld(t *testing.T) (*Service, []*Connection, *FixtureAccounts, string, BattleLayout) {
	t.Helper()
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	own, peer := hexOf(selectedOID(cs[0])), hexOf(selectedOID(cs[1]))
	_, err := store.UpdateSocial(ctx, []string{own, peer}, func(v map[string]*Avatar) error {
		for _, av := range v {
			ensureSocial(&av.Progress.Social)
			av.Progress.UnlockSystems["assist"] = 1
			av.Progress.UnlockSystems["support"] = 1
		}
		v[own].Progress.Social.Friends[peer] = SocialFriend{Info: socialProfile(*v[peer])}
		v[peer].Progress.Social.Friends[own] = SocialFriend{Info: socialProfile(*v[own])}
		v[peer].Progress.Cards = []Card{newCard(3202, 1, s.Now())}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	uuid := socialSaved(t, store, cs[1]).Progress.Cards[0].UUID
	pushes, err := s.friendAssistRPC(ctx, cs[1], "set_assist_card", socialArgs(29, uuid))
	if err != nil || socialCallback(t, pushes)[0] != 0 {
		t.Fatal(err, pushes)
	}
	pushes, err = s.friendAssistRPC(ctx, cs[0], "refresh_assist_use_times", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(socialSaved(t, store, cs[0]).Progress.Social.Assist.Cards) != 1 {
		t.Fatal("好友助战名册未从真实提供者生成")
	}
	pushes, err = s.friendAssistRPC(ctx, cs[0], "select_assist_card", socialArgs(30, uuid, map[string]any{"eid": peer, "hostnum": 1, "nickname": "伪造"}))
	if err != nil || socialCallback(t, pushes)[0] != 0 {
		t.Fatal(err, pushes)
	}
	if socialSaved(t, store, cs[0]).Progress.Social.Assist.Current.Profile.Nickname == "伪造" {
		t.Fatal("客户端资料覆盖提供者")
	}
	layout := BattleLayout{Fighting: []string{socialSaved(t, store, cs[0]).Progress.Cards[0].UUID, uuid}, Support: []string{}, Storyline: []string{}}
	return s, cs, store, uuid, layout
}
func setAssistTestBattle(t *testing.T, s *Service, c *Connection, id string) {
	t.Helper()
	if err := s.updateProgress(context.Background(), c, func(p *Progress) error {
		p.Battle = &BattleSession{UUID: id, DungeonID: 100101, BattleID: 1101, Loaded: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestSocialFriendAssistFrozenAuthorizationAndDuplicate(t *testing.T) {
	s, cs, store, uuid, layout := friendAssistWorld(t)
	ctx := context.Background()
	battle := "00112233445566778899aabb"
	setAssistTestBattle(t, s, cs[0], battle)
	before := socialSaved(t, store, cs[0]).Progress.Materials[17].Count
	start := func(p *Progress) error {
		if e := validateBattleLayout(*p, layout); e != nil {
			return e
		}
		p.Battle.Started = true
		p.Battle.Team = layout.team()
		return nil
	}
	if err := s.updateBattleFormation(ctx, cs[0], layout, 0, true, start); err != nil {
		t.Fatal(err)
	}
	own, peer := socialSaved(t, store, cs[0]), socialSaved(t, store, cs[1])
	if len(own.Progress.Cards) != 1 || own.Progress.Social.Assist.Active[uuid] != 1 || peer.Progress.Social.Assist.Passive != 1 || own.Progress.Materials[17].Count != before+1 {
		t.Fatal("借用卡污染资产或双人次数/奖励缺失")
	}
	originalLevel := own.Progress.Social.Assist.Frozen.Card.Level
	if err := s.updateProgress(ctx, cs[1], func(p *Progress) error { p.Cards[0].Level = 30; p.AssistCardUUID = ""; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.socialRPC(ctx, cs[0], "add_black_list", socialArgs(31, map[string]any{"eid": hexOf(selectedOID(cs[1]))})); err != nil {
		t.Fatal(err)
	}
	if err := s.updateBattleFormation(ctx, cs[0], layout, 0, true, start); err != nil {
		t.Fatal("已开战重连无法复用锁定快照", err)
	}
	own = socialSaved(t, store, cs[0])
	peer = socialSaved(t, store, cs[1])
	if own.Progress.Social.Assist.Active[uuid] != 1 || peer.Progress.Social.Assist.Passive != 1 || own.Progress.Social.Assist.Frozen.Card.Level != originalLevel {
		t.Fatal("重复开战再次计数或外部卡随提供者变更")
	}
	cards := battleCardListWire(&own.Progress, layout.team())
	if len(cards) != 2 {
		t.Fatal("助战卡实体没有加入实际战斗", cards)
	}
	frozen := cards[1].(map[string]any)
	if frozen["level"] != originalLevel {
		t.Fatal("助战没有采用冻结等级")
	}
	reopened := "22212233445566778899aabb"
	if err := s.updateProgress(ctx, cs[0], func(p *Progress) error {
		p.Battle.UUID = reopened
		p.Battle.Started = false
		p.Social.Assist.Frozen.BattleUUID = reopened
		p.Social.Assist.LastBattle = reopened
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.updateBattleFormation(ctx, cs[0], layout, 0, true, start); err != nil {
		t.Fatal("恢复重开错误重新验证已撤销好友", err)
	}
	own = socialSaved(t, store, cs[0])
	peer = socialSaved(t, store, cs[1])
	if own.Progress.Social.Assist.Active[uuid] != 1 || peer.Progress.Social.Assist.Passive != 1 || own.Progress.Social.Assist.Frozen.Card.Level != originalLevel {
		t.Fatal("恢复新UUID再次计数或改冻结快照")
	}
	setAssistTestBattle(t, s, cs[0], "11112233445566778899aabb")
	old := socialSaved(t, store, cs[0])
	if err := s.updateBattleFormation(ctx, cs[0], layout, 0, true, start); err == nil {
		t.Fatal("拉黑并撤销指定卡后仍允许新局助战")
	}
	after := socialSaved(t, store, cs[0])
	if !reflect.DeepEqual(old.Progress, after.Progress) {
		t.Fatal("授权失败后部分提交")
	}
}
func TestSocialFriendAssistLimitAndTransactionRollback(t *testing.T) {
	s, cs, store, uuid, layout := friendAssistWorld(t)
	ctx := context.Background()
	setAssistTestBattle(t, s, cs[0], "00112233445566778899aabb")
	before := socialSaved(t, store, cs[0])
	peerBefore := socialSaved(t, store, cs[1])
	if err := s.updateBattleFormation(ctx, cs[0], layout, 0, true, func(*Progress) error { return errors.New("布阵业务失败") }); err == nil {
		t.Fatal("业务失败未回滚")
	}
	if !reflect.DeepEqual(before.Progress, socialSaved(t, store, cs[0]).Progress) || !reflect.DeepEqual(peerBefore.Progress, socialSaved(t, store, cs[1]).Progress) {
		t.Fatal("双人次数或奖励部分提交")
	}
	for i := 0; i < 10; i++ {
		id := newBattleUUID(s.Now())
		setAssistTestBattle(t, s, cs[0], id)
		if err := s.updateBattleFormation(ctx, cs[0], layout, 0, true, func(p *Progress) error { p.Battle.Started = true; return nil }); err != nil {
			t.Fatal(i, err)
		}
	}
	setAssistTestBattle(t, s, cs[0], newBattleUUID(s.Now()))
	before = socialSaved(t, store, cs[0])
	peerBefore = socialSaved(t, store, cs[1])
	if err := s.updateBattleFormation(ctx, cs[0], layout, 0, true, func(*Progress) error { return nil }); err == nil {
		t.Fatal("原生每卡10次上限未执行")
	}
	if before.Progress.Social.Assist.Active[uuid] != 10 || !reflect.DeepEqual(before.Progress, socialSaved(t, store, cs[0]).Progress) || !reflect.DeepEqual(peerBefore.Progress, socialSaved(t, store, cs[1]).Progress) {
		t.Fatal("次数超限仍提交")
	}
	now := s.Now()
	s.Now = func() time.Time { return now.Add(24 * time.Hour) }
	if err := s.updateBattleFormation(ctx, cs[0], layout, 0, true, func(*Progress) error { return nil }); err != nil {
		t.Fatal("复刻日界线没有恢复次数", err)
	}
	s.Now = func() time.Time { return now }
	before = socialSaved(t, store, cs[0])
	if _, err := s.friendAssistRPC(ctx, cs[0], "refresh_assist_use_times", nil); err == nil {
		t.Fatal("时钟回拨未拒绝")
	}
	if !reflect.DeepEqual(before.Progress, socialSaved(t, store, cs[0]).Progress) {
		t.Fatal("回拨时间仍写档")
	}
}
func TestSocialFriendAssistNoPvpBorrowingAndCallbackSignature(t *testing.T) {
	s, cs, store, _, layout := friendAssistWorld(t)
	ctx := context.Background()
	if err := s.updateBattleFormation(ctx, cs[0], layout, 1, false, func(*Progress) error { return nil }); err == nil {
		t.Fatal("异步竞技禁助战仍接受")
	}
	if err := validatePresetCards(socialSaved(t, store, cs[0]).Progress, layout, true); err == nil {
		t.Fatal("同步竞技预设借用好友卡")
	}
	if err := validateNormalPresetCards(socialSaved(t, store, cs[0]).Progress, layout); err != nil {
		t.Fatal("普通预设无法引用好友卡", err)
	}
	pushes, err := s.friendAssistRPC(ctx, cs[1], "set_assist_card", []json.RawMessage{json.RawMessage(`1`)})
	if err == nil || len(pushes) > 0 {
		t.Fatal("旧伪造callback0单参入口仍成功")
	}
}
