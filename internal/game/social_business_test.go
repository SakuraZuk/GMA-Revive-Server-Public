package game

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"
)

func socialTestWorld(t *testing.T) (*Service, []*Connection, *FixtureAccounts) {
	t.Helper()
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	records := map[string]FixtureAccount{}
	connections := []*Connection{}
	for i := 0; i < 3; i++ {
		av := Avatar{OID: []byte(fmt.Sprintf("%012d", i+1)), UID: int64(i + 1), Hostnum: 1, Info: DefaultAvatarInfo(fmt.Sprintf("好友%d", i)), Progress: NewProgress(10, now)}
		av.Progress.UnlockSystems["panel_async_pvp"] = 1
		av.Progress.Materials[26] = Material{ID: 26, Count: 5, Total: 5}
		av.Progress.Lineup = []string{av.Progress.Cards[0].UUID}
		av.Progress.AsyncPvp.Defence = append([]string{}, av.Progress.Lineup...)
		name := fmt.Sprintf("好友%d", i)
		records[name] = FixtureAccount{Avatars: []Avatar{av}}
		connections = append(connections, &Connection{phase: Playing, hostnum: 1, identity: Identity{Account: name, Avatars: []Avatar{av}}})
	}
	store := NewFixtureAccounts(records)
	s := New(store, nil)
	s.Now = func() time.Time { return now }
	return s, connections, store
}
func socialArgs(v ...any) []json.RawMessage {
	out := []json.RawMessage{}
	for _, arg := range v {
		raw, _ := json.Marshal(arg)
		out = append(out, raw)
	}
	return out
}
func socialCallback(t *testing.T, pushes []Push) []any {
	t.Helper()
	for i := len(pushes) - 1; i >= 0; i-- {
		if pushes[i].Method == "call_client_callback" {
			return pushes[i].Args[1].([]any)
		}
	}
	t.Fatal("缺少回调")
	return nil
}
func socialSaved(t *testing.T, store SocialAccounts, c *Connection) Avatar {
	t.Helper()
	rows, err := store.SocialAvatars(context.Background(), SocialSearch{OIDs: []string{hexOf(selectedOID(c))}, Limit: 1})
	if err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	return rows[0]
}
func TestSocialFriendCrossAvatarAtomicAndBlock(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	a, b := hexOf(selectedOID(cs[0])), hexOf(selectedOID(cs[1]))
	pushes, err := s.socialRPC(ctx, cs[0], "apply_friend", socialArgs(1, b, "交个朋友", 1, 0))
	if err != nil || socialCallback(t, pushes)[0] != 0 {
		t.Fatal(err, pushes)
	}
	if len(socialSaved(t, store, cs[1]).Progress.Social.Requests) != 1 {
		t.Fatal("离线对方未持久收到申请")
	}
	if _, err = s.socialRPC(ctx, cs[0], "apply_friend", socialArgs(1, b, "再次申请", 1, 0)); err != nil {
		t.Fatal(err)
	}
	if len(socialSaved(t, store, cs[1]).Progress.Social.Requests) != 1 {
		t.Fatal("重复申请生成重复队列")
	}
	pushes, err = s.socialRPC(ctx, cs[1], "agree_apply_friend", socialArgs(2, a))
	if err != nil || socialCallback(t, pushes)[0] != 0 {
		t.Fatal(err, pushes)
	}
	own := socialSaved(t, store, cs[0])
	peer := socialSaved(t, store, cs[1])
	if len(own.Progress.Social.Friends) != 1 || len(peer.Progress.Social.Friends) != 1 || len(peer.Progress.Social.Requests) != 0 {
		t.Fatal("双方关系未原子创建")
	}
	uuid := peer.Progress.Cards[0].UUID
	peer.Progress.AssistCardUUID = uuid
	if err := AuthorizedFriendAssist(own, peer, uuid); err != nil {
		t.Fatal(err)
	}
	if err := AuthorizedFriendAssist(own, peer, own.Progress.Cards[0].UUID); err == nil {
		t.Fatal("非指定助战被允许")
	}
	_, err = s.socialRPC(ctx, cs[0], "add_black_list", socialArgs(3, map[string]any{"eid": b, "nickname": "伪造资料"}))
	if err != nil {
		t.Fatal(err)
	}
	own = socialSaved(t, store, cs[0])
	peer = socialSaved(t, store, cs[1])
	if len(own.Progress.Social.Friends) != 0 || len(peer.Progress.Social.Friends) != 0 || own.Progress.Social.Blacklist[b].Info.Nickname == "伪造资料" {
		t.Fatal("黑名单授权/双向删除失败")
	}
	pushes, err = s.socialRPC(ctx, cs[1], "apply_friend", socialArgs(4, a, "不能申请", 1, 0))
	if err != nil || socialCallback(t, pushes)[0] == 0 {
		t.Fatal("对方黑名单没有拒绝申请", err, pushes)
	}
}
func TestPlayerDetailsNativePublicFields(t *testing.T) {
	s, cs, _ := socialTestWorld(t)
	target := cs[1].SelectedAvatarUnsafe()
	pushes, err := s.socialRPC(context.Background(), cs[0], "get_player_details", socialArgs(7, hexOf(target.OID), target.Hostnum))
	if err != nil {
		t.Fatal(err)
	}
	info := socialCallback(t, pushes)[0].(map[string]any)
	for _, key := range []string{"show_cards", "show_medals", "signature", "gender", "cards_count", "last_main_chapter_dungeon_id", "achv_value"} {
		if _, ok := info[key]; !ok {
			t.Fatal("原生角色详情字段缺失", key)
		}
	}
}

func TestSocialCommentOwnershipDuplicateDayAndRollback(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	pushes, err := s.socialRPC(ctx, cs[0], "send_card_comment", socialArgs(1, 4401, "这本书很有趣"))
	if err != nil {
		t.Fatal(err)
	}
	id := string(socialCallback(t, pushes)[1].(ObjectID))
	pushes, err = s.socialRPC(ctx, cs[1], "update_card_comment", socialArgs(2, 4401, id, "伪造修改"))
	if err != nil || socialCallback(t, pushes)[0] == 0 {
		t.Fatal("修改别人评论未拒绝", err)
	}
	pushes, err = s.socialRPC(ctx, cs[1], "like_card_comment", socialArgs(3, 4401, id))
	if err != nil || socialCallback(t, pushes)[0] != 0 {
		t.Fatal(err)
	}
	pushes, err = s.socialRPC(ctx, cs[1], "like_card_comment", socialArgs(3, 4401, id))
	if err != nil || socialCallback(t, pushes)[0] != 14001 {
		t.Fatal("重复点赞未返回原生错误", err, pushes)
	}
	rows, err := store.QueryComments(ctx, 4401, 0, 2)
	if err != nil || len(rows) != 1 || len(rows[0].Likes) != 1 {
		t.Fatal("赞数未持久或被重复增加", rows, err)
	}
	for i := 0; i < 2; i++ {
		if _, err = s.socialRPC(ctx, cs[0], "send_card_comment", socialArgs(1, 4401, "额外评论")); err != nil {
			t.Fatal(err)
		}
	}
	pushes, err = s.socialRPC(ctx, cs[0], "send_card_comment", socialArgs(1, 4401, "第四条"))
	if err != nil || socialCallback(t, pushes)[0] != 14008 {
		t.Fatal("每日原生上限未执行", err, pushes)
	}
	now := s.Now()
	s.Now = func() time.Time { return now.Add(24 * time.Hour) }
	pushes, err = s.socialRPC(ctx, cs[0], "send_card_comment", socialArgs(1, 4401, "次日评论"))
	if err != nil || socialCallback(t, pushes)[0] != 0 {
		t.Fatal("次日评论没有恢复", err, pushes)
	}
	before := socialSaved(t, store, cs[0])
	s.Now = func() time.Time { return now }
	if _, err = s.socialRPC(ctx, cs[0], "send_card_comment", socialArgs(1, 4401, "回拨时间")); err == nil {
		t.Fatal("时钟回拨未拒绝")
	}
	after := socialSaved(t, store, cs[0])
	if before.Progress.Social.Revision != after.Progress.Social.Revision {
		t.Fatal("失败后存档被修改")
	}
}
func TestAsyncPvpSnapshotTicketCrossAvatarSettlement(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	if _, err := s.asyncPvpRPC(ctx, cs[0], "enter_asyn_pvp", nil); err != nil {
		t.Fatal(err)
	}
	target := hexOf(selectedOID(cs[1]))
	initial := socialSaved(t, store, cs[0]).Progress.Materials[26].Count
	pushes, err := s.enterAsyncPvp(ctx, cs[0], socialArgs(1, 1, map[string]any{"asyn_pvp_eid": target}))
	if err != nil || len(pushes) < 5 {
		t.Fatal(err, pushes)
	}
	p := cs[0].SelectedAvatarUnsafe().Progress
	if p.AsyncPvp.Match == nil || p.Materials[26].Count != initial-1 {
		t.Fatal("真实敌快照或挑战票据未冻结")
	}
	if _, err = s.enterAsyncPvp(ctx, cs[0], socialArgs(1, 1, map[string]any{"asyn_pvp_eid": target})); err != nil {
		t.Fatal(err)
	}
	if cs[0].SelectedAvatarUnsafe().Progress.Materials[26].Count != initial-1 {
		t.Fatal("重复进入重复扣票")
	}
	if err := s.updateProgress(ctx, cs[0], func(p *Progress) error {
		p.Battle.BridgeReady = true
		p.Battle.BridgeStarted = true
		p.Battle.LastSequence = 2
		p.Battle.Team = append([]string{}, p.Lineup...)
		freezeAsyncOwnTeam(p)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	uuid := cs[0].SelectedAvatarUnsafe().Progress.Battle.UUID
	data := map[string]any{"winner_eids": []any{hexOf(selectedOID(cs[0]))}, "outcome": "win"}
	handled, result, err := s.absorbAsyncPvpResult(ctx, cs[0], &battleEnvelope{BattleUUID: uuid, Sequence: 3, Kind: "result", Data: data})
	if !handled || err != nil || len(result) == 0 {
		t.Fatal(err, result)
	}
	av := socialSaved(t, store, cs[0])
	enemy := socialSaved(t, store, cs[1])
	if av.Progress.AsyncPvp.Score <= 1000 || len(av.Progress.AsyncPvp.AttackRecords) != 1 || len(enemy.Progress.AsyncPvp.DefenceRecords) != 1 {
		t.Fatal("双方积分记录未同事务结算", av.Progress.AsyncPvp, enemy.Progress.AsyncPvp)
	}
	score := av.Progress.AsyncPvp.Score
	coin := av.Progress.Materials[14].Count
	_, _, err = s.absorbAsyncPvpResult(ctx, cs[0], &battleEnvelope{BattleUUID: uuid, Sequence: 3, Kind: "result", Data: data})
	if err != nil {
		t.Fatal(err)
	}
	av = socialSaved(t, store, cs[0])
	if av.Progress.AsyncPvp.Score != score || av.Progress.Materials[14].Count != coin || len(av.Progress.AsyncPvp.AttackRecords) != 1 {
		t.Fatal("UUID重复结果重复发奖或加分")
	}
}
func TestSyncPvpWeeklyBonusAtomicOverflow(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	if err := s.updateProgress(ctx, cs[0], func(p *Progress) error {
		if err := ensureSyncPvpPeriod(p, s.Now()); err != nil {
			return err
		}
		p.SyncPvpWeeklyWins = 1
		p.Materials[11] = Material{ID: 11, Count: math.MaxInt64, Total: math.MaxInt64}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pushes, err := s.receiveSyncPvpWeekly(ctx, cs[0], socialArgs(1, 1))
	if err != nil || socialCallback(t, pushes)[0] != nil {
		t.Fatal("奖励溢出未失败", err, pushes)
	}
	p := socialSaved(t, store, cs[0]).Progress
	if p.SyncPvpMeta.WeeklyClaims[1] {
		t.Fatal("溢出后领取标记被提交")
	}
	if err = s.updateProgress(ctx, cs[0], func(p *Progress) error { p.Materials[11] = Material{ID: 11}; return nil }); err != nil {
		t.Fatal(err)
	}
	pushes, err = s.receiveSyncPvpWeekly(ctx, cs[0], socialArgs(1, 1))
	if err != nil || socialCallback(t, pushes)[0] == nil {
		t.Fatal(err, pushes)
	}
	p = socialSaved(t, store, cs[0]).Progress
	if p.Materials[11].Count != 20 || !p.SyncPvpMeta.WeeklyClaims[1] {
		t.Fatal("表内奖励与领取标记未原子保存")
	}
	pushes, err = s.receiveSyncPvpWeekly(ctx, cs[0], socialArgs(1, 1))
	if err != nil || socialCallback(t, pushes)[0] != nil {
		t.Fatal("重复领取未拒绝")
	}
}
