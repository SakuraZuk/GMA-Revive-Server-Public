package game

import (
	"context"
	"hs-server/internal/mobileproto"
	"testing"
)

func presenceOnline(t *testing.T, s *Service, c *Connection, peer string) bool {
	t.Helper()
	props := s.socialLiveProperties(c.SelectedAvatarUnsafe().Progress.Social)
	for _, row := range props["friend_dict"].(mobileproto.Map) {
		if row.Key == ObjectID(peer) {
			return row.Value.(map[string]any)["info"].(map[string]any)["online"].(bool)
		}
	}
	t.Fatal("好友投影不存在")
	return false
}
func TestSocialPresenceMultipleSessionsAndDetachNotify(t *testing.T) {
	s, cs, _, _, _ := friendAssistWorld(t)
	ctx := context.Background()
	a, b := hexOf(selectedOID(cs[0])), hexOf(selectedOID(cs[1]))
	if err := s.updateProgress(ctx, cs[1], func(*Progress) error { return nil }); err != nil {
		t.Fatal(err)
	}
	s.attachPlayer(cs[0])
	s.attachPlayer(cs[1])
	if !presenceOnline(t, s, cs[0], b) || !presenceOnline(t, s, cs[1], a) {
		t.Fatal("真实连接未投影为在线")
	}
	if len(cs[0].pendingSocialOIDs) == 0 || len(cs[1].pendingSocialOIDs) == 0 {
		t.Fatal("登入没有排队通知好友")
	}
	duplicate := &Connection{phase: Playing, hostnum: 1, identity: Identity{Account: cs[1].identity.Account, Avatars: append([]Avatar{}, cs[1].identity.Avatars...)}}
	s.attachPlayer(duplicate)
	s.Detach(cs[1])
	if !presenceOnline(t, s, cs[0], b) || !cs[0].pendingPlayerReload {
		t.Fatal("一连接离线误令另一在线连接离线或没有通知好友")
	}
	cs[0].pendingPlayerReload = false
	s.Detach(duplicate)
	if presenceOnline(t, s, cs[0], b) || !cs[0].pendingPlayerReload {
		t.Fatal("全部连接关闭仍投影在线或好友未通知")
	}
}

func TestSocialChallengeAuthorizationAndPersistentInvite(t *testing.T) {
	s, cs, store, _, _ := friendAssistWorld(t)
	ctx := context.Background()
	peer := hexOf(selectedOID(cs[1]))
	pushes, err := s.socialRPC(ctx, cs[0], "challenge_friend", socialArgs(41, peer))
	if err != nil || socialCallback(t, pushes)[0] != 7004 {
		t.Fatal("离线好友未返回原生离线码", err, pushes)
	}
	s.attachPlayer(cs[1])
	pushes, err = s.socialRPC(ctx, cs[0], "challenge_friend", socialArgs(42, peer))
	if err != nil || socialCallback(t, pushes)[0] != 0 {
		t.Fatal("真实在线好友邀请失败", err, pushes)
	}
	if len(socialSaved(t, store, cs[1]).Progress.Social.Challenges) != 1 {
		t.Fatal("成功回调没有真实持久邀请")
	}
	pushes, err = s.socialRPC(ctx, cs[0], "challenge_friend", socialArgs(43, hexOf(selectedOID(cs[2]))))
	if err != nil || socialCallback(t, pushes)[0] != 7001 {
		t.Fatal("非好友挑战未按原生拒绝", err, pushes)
	}
	if cs[0].SelectedAvatarUnsafe().Progress.SyncPvpMatch != nil {
		t.Fatal("好友尚未同意却创建战斗")
	}
	s.Detach(cs[1])
}
