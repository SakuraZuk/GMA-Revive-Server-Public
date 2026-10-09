package dbstore

import (
	"context"
	"encoding/hex"
	"errors"
	"hs-server/internal/game"
	"testing"
	"time"
)

func TestPostgresSocialCalendarGlobalAtomicMailOnceAndSnapshot(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 12, 59, 0, 0, time.UTC)
	svc := pgSocialService(t, store)
	svc.Now = func() time.Time { return now }
	identities := []game.Identity{}
	cs := []*game.Connection{}
	for _, name := range []string{"周期邮件真实甲", "周期邮件真实乙"} {
		info := game.ClientInfo{Account: name, Password: "pw", Hostnum: 1}
		id, e := store.Register(ctx, info)
		if e != nil {
			t.Fatal(e)
		}
		identities = append(identities, id)
		c := pgSocialLogin(t, svc, info, id.Avatars[0].OID)
		cs = append(cs, c)
	}
	defer svc.Detach(cs[0])
	defer svc.Detach(cs[1])
	ids := []string{}
	for _, id := range identities {
		ids = append(ids, hex.EncodeToString(id.Avatars[0].OID))
	}
	_, err := store.UpdateSocial(ctx, ids, func(v map[string]*game.Avatar) error {
		for _, id := range ids {
			p := &v[id].Progress
			p.UnlockSystems["panel_async_pvp"] = 1
			p.AsyncPvp.Score = 1300
			p.AsyncPvp.RewardDay = "2026-10-06"
			p.AsyncPvp.RewardWeek = "2026-10-04"
			p.Social.Assist.Day = "2026-10-07"
			p.Social.Assist.Passive = 7
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pgSocialCall(t, svc, cs[0], "refresh_assist_use_times")
	now = now.Add(2 * time.Minute)
	pgSocialCall(t, svc, cs[0], "refresh_assist_use_times")
	for _, identity := range identities {
		av := pgHumanRead(t, store, identity.Avatars[0].OID)
		if len(av.Progress.ShortMailInfo) != 1 || av.Progress.AsyncPvp.RewardDay != "2026-10-07" {
			t.Fatal("真实库离线角色日21奖励未同事务写入")
		}
		for _, m := range av.Progress.ShortMailInfo {
			if m.Attachments[14] != 40 {
				t.Fatal("真实库每日奖励未取原生数值")
			}
		}
	}
	pgSocialCall(t, svc, cs[0], "refresh_assist_use_times")
	if len(pgHumanRead(t, store, identities[1].Avatars[0].OID).Progress.ShortMailInfo) != 1 {
		t.Fatal("真实库重复执行日奖重复发件")
	}
	now = time.Date(2026, 10, 7, 16, 1, 0, 0, time.UTC)
	pgSocialCall(t, svc, cs[0], "refresh_assist_use_times")
	for _, identity := range identities {
		av := pgHumanRead(t, store, identity.Avatars[0].OID)
		if len(av.Progress.ShortMailInfo) != 2 || av.Progress.Social.Assist.Passive != 0 {
			t.Fatal("真实库被动收益未先保存邮件再清零")
		}
	}
	// 全量结算故障与双角色相同事务语义，不依赖单角色UpdateProgress。
	before := pgHumanRead(t, store, identities[0].Avatars[0].OID)
	_, err = store.UpdateAllSocial(ctx, func(v map[string]*game.Avatar) error {
		for _, id := range ids {
			v[id].Progress.AsyncPvp.RewardDay = "2099-01-01"
			v[id].Progress.ShortMailInfo = nil
		}
		return errors.New("全服周期故障注入")
	})
	if err == nil {
		t.Fatal("真实库全量故障提交")
	}
	for _, identity := range identities {
		av := pgHumanRead(t, store, identity.Avatars[0].OID)
		if len(av.Progress.ShortMailInfo) != 2 || av.Progress.AsyncPvp.RewardDay != before.Progress.AsyncPvp.RewardDay {
			t.Fatal("真实库全量周期部分提交")
		}
	}
	// Sunday21 native周排名邮件也参与同一收据事务；头框附件用真实邮件领取资产链。
	now = time.Date(2026, 10, 11, 12, 59, 0, 0, time.UTC)
	pgSocialCall(t, svc, cs[0], "refresh_assist_use_times")
	now = now.Add(2 * time.Minute)
	pgSocialCall(t, svc, cs[0], "refresh_assist_use_times")
	for _, identity := range identities {
		av := pgHumanRead(t, store, identity.Avatars[0].OID)
		rankmail := false
		for _, m := range av.Progress.ShortMailInfo {
			if m.Title == "奇岩试炼场排名奖励" {
				rankmail = true
				if m.Attachments[80008] != 1 {
					t.Fatal("真实库周榜奖未使用Android排名附件")
				}
			}
		}
		if !rankmail || av.Progress.AsyncPvp.RewardWeek != "2026-10-11" {
			t.Fatal("真实库周日21未发真实跨服排名奖")
		}
	}
}
