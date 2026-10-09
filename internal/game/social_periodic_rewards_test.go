package game

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestSocialCalendarDailyAssistMailAtomicAndOnce(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	cs = cs[:2]
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 12, 59, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	ids := []string{hexOf(selectedOID(cs[0])), hexOf(selectedOID(cs[1]))}
	_, err := store.UpdateSocial(ctx, ids, func(v map[string]*Avatar) error {
		for _, id := range ids {
			p := &v[id].Progress
			p.AsyncPvp.Score = 1300
			p.Social.Assist.Day = socialDay(now)
			p.Social.Assist.Passive = 7
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.refreshPvpAwards(ctx, cs[0]); err != nil {
		t.Fatal(err)
	}
	if len(socialSaved(t, store, cs[0]).Progress.ShortMailInfo) != 0 {
		t.Fatal("首次启动伪造历史截止日奖励")
	}
	now = now.Add(2 * time.Minute)
	if err = s.refreshPvpAwards(ctx, cs[0]); err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		av := socialSaved(t, store, c)
		if len(av.Progress.ShortMailInfo) != 1 {
			t.Fatal("21点未给离线角色同事务发每日邮件")
		}
		for _, m := range av.Progress.ShortMailInfo {
			if m.Attachments[14] != 40 || m.Title != "奇岩试炼场段位奖励" {
				t.Fatal("每日邮件未取Android bonus1112与模板3", m)
			}
		}
	}
	if err = s.refreshPvpAwards(ctx, cs[0]); err != nil {
		t.Fatal(err)
	}
	if len(socialSaved(t, store, cs[1]).Progress.ShortMailInfo) != 1 {
		t.Fatal("重复21点重复发奖")
	}
	now = time.Date(2026, 10, 7, 16, 1, 0, 0, time.UTC)
	if err = s.refreshPvpAwards(ctx, cs[0]); err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		av := socialSaved(t, store, c)
		if len(av.Progress.ShortMailInfo) != 2 || av.Progress.Social.Assist.Passive != 0 {
			t.Fatal("昨日被动次数未先发邮件再清零")
		}
		found := false
		for _, m := range av.Progress.ShortMailInfo {
			if m.Title == "助战奖励" {
				found = true
				if m.Attachments[17] != 25 || m.Content != "昨天您的助战被使用了7次，可获得以下奖励，请大人查收~" {
					t.Fatal("被动收益未使用原生次数映射与模板5", m)
				}
			}
		}
		if !found {
			t.Fatal("缺被动助战奖励")
		}
	}
}

func TestSocialCalendarAllRoleFailureRollbackRetry(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	cs = cs[:2]
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 12, 59, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	ids := []string{hexOf(selectedOID(cs[0])), hexOf(selectedOID(cs[1]))}
	_, err := store.UpdateSocial(ctx, ids, func(v map[string]*Avatar) error {
		for _, id := range ids {
			v[id].Progress.AsyncPvp.Score = 1000
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.refreshPvpAwards(ctx, cs[0]); err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdateSocial(ctx, []string{ids[1]}, func(v map[string]*Avatar) error {
		p := &v[ids[1]].Progress
		p.ShortMailInfo = map[int]Mail{}
		for i := 0; i < 500; i++ {
			p.ShortMailInfo[i+1] = Mail{MID: i + 1, Title: "占满邮箱", State: MailFinal}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if err = s.refreshPvpAwards(ctx, cs[0]); err == nil {
		t.Fatal("邮箱满未阻止周期事务")
	}
	for _, c := range cs {
		p := socialSaved(t, store, c).Progress
		if p.AsyncPvp.RewardDay != "2026-10-06" {
			t.Fatal("全量失败部分保存周期收据")
		}
	}
	if len(socialSaved(t, store, cs[0]).Progress.ShortMailInfo) != 0 {
		t.Fatal("全量失败部分发件")
	}
	_, err = store.UpdateSocial(ctx, []string{ids[1]}, func(v map[string]*Avatar) error { v[ids[1]].Progress.ShortMailInfo = nil; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = s.refreshPvpAwards(ctx, cs[0]); err != nil {
		t.Fatal("失败后不能真实重试", err)
	}
	for _, c := range cs {
		if len(socialSaved(t, store, c).Progress.ShortMailInfo) != 1 {
			t.Fatal("重试没有完整发送", hexOf(selectedOID(c)), socialSaved(t, store, c).Progress.AsyncPvp.RewardDay, socialSaved(t, store, c).Progress.ShortMailInfo)
		}
	}
}

func TestSocialCalendarSeasonRankSnapshotBeyondThousand(t *testing.T) {
	now := time.Unix(syncPvpRule().SeasonOriginalDate+300*syncPvpRule().SeasonDuration-1, 0)
	period := syncPvpSeasonID(now)
	records := map[string]FixtureAccount{}
	for i := 0; i < 1101; i++ {
		oid := []byte(fmt.Sprintf("%012d", i))
		p := Progress{AvatarLevel: 20, SyncPvpScore: 1000 + i, SyncPvpHighestScore: 1000 + i, SyncPvpMeta: SyncPvpMeta{Period: period, BeginPeriod: period}}
		records[fmt.Sprint(i)] = FixtureAccount{Avatars: []Avatar{{OID: oid, Hostnum: 1, Info: AvatarInfo{Level: 20}, Progress: p}}}
	}
	store := NewFixtureAccounts(records)
	s := New(store, nil)
	s.Now = func() time.Time { return now }
	c := &Connection{phase: Playing, hostnum: 1, identity: Identity{Avatars: records["0"].Avatars}}
	if err := s.refreshPvpAwards(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	if err := s.refreshPvpAwards(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	worst := socialSaved(t, store, c).Progress.SyncPvpMeta.History[period]
	if worst.Rank != 1101 || worst.RankBonus != 0 {
		t.Fatal("赛季全服快照被1000窗口截断", worst)
	}
	best := records["1100"].Avatars[0]
	rows, err := store.SocialAvatars(context.Background(), SocialSearch{OIDs: []string{hexOf(best.OID)}, Limit: 1})
	if err != nil || rows[0].Progress.SyncPvpMeta.History[period].Rank != 1 || rows[0].Progress.SyncPvpMeta.History[period].RankBonus != 1291 {
		t.Fatal("全量赛季排名与原生排名奖错误", err)
	}
}
