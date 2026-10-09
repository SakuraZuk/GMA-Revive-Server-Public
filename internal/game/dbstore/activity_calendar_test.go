package dbstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestPostgresActivityCalendarAtomicDailyMountainReloadAndRetry(t *testing.T) {
	// 专测默认未启用政策的周Pending收据；正式周奖事务另有Remaining专项。
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "")
	store := testStore(t)
	ctx := context.Background()
	zone := time.FixedZone("北京时间", 28800)
	begin := time.Date(2026, 10, 7, 0, 0, 0, 0, zone)
	if err := game.SetActivitySchedules(game.DefaultPermanentActivitySchedules(begin)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = game.SetActivitySchedules(nil) })
	now := time.Date(2026, 10, 21, 21, 59, 58, 0, zone)
	svc := pgActivityService(t, store, func() time.Time { return now })
	cs := []*game.Connection{}
	oids := [][]byte{}
	for i := 1; i <= 2; i++ {
		info := game.ClientInfo{Account: fmt.Sprintf("活动周期真实%d", i), Password: "pw", Hostnum: 10001}
		identity, err := store.Register(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		oid := identity.Avatars[0].OID
		if _, err = store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
			p.AvatarLevel = 20
			p.ClearedDungeons = []int{601, 610}
			p.Activities.Mountain = &game.MountainState{Dungeons: map[int]game.ActivityProgress{1: {ID: 20321111, Ranked: true, RankHard: 2, Actions: i * 10}}}
			p.Activities.Nian = map[int]game.ActivityProgress{20200001: {ID: 20200001, Ranked: true, RankDamage: int64(110 - i*10), RankAt: now.Unix(), MaxDamage: 999}}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err = store.pool.Exec(ctx, `UPDATE avatars SET level=20 WHERE avatar_oid=$1`, oid); err != nil {
			t.Fatal(err)
		}
		c := pgActivityLogin(t, svc, info)
		cs, oids = append(cs, c), append(oids, oid)
	}
	// 初见只建立基线；不能用当前成绩重建历史日奖。
	for _, oid := range oids {
		p := pgHumanRead(t, store, oid).Progress
		if len(p.ShortMailInfo) != 0 || p.Activities.Calendar == nil {
			t.Fatal("首次登录补发历史活动奖")
		}
	}
	if _, err := store.UpdateProgress(ctx, oids[1], func(p *game.Progress) error {
		p.ShortMailInfo = map[int]game.Mail{}
		for i := 1; i <= 500; i++ {
			p.ShortMailInfo[i] = game.Mail{MID: i, UUID: fmt.Sprintf("%024x", 10000+i), Title: "隔离容量夹具"}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := [][]byte{}
	for _, oid := range oids {
		raw, _ := json.Marshal(pgHumanRead(t, store, oid).Progress)
		before = append(before, raw)
	}
	now = now.Add(2 * time.Second)
	if _, err := svc.Handle(ctx, cs[0], "query_rank_list", pgSocialArgs(8, 1)); err == nil {
		t.Fatal("邮箱容量故障仍完成全服七日奖")
	}
	for i, oid := range oids {
		raw, _ := json.Marshal(pgHumanRead(t, store, oid).Progress)
		if !bytes.Equal(raw, before[i]) {
			t.Fatal("真实数据库活动邮件与收据部分提交", i)
		}
	}
	if _, err := store.UpdateProgress(ctx, oids[1], func(p *game.Progress) error { p.ShortMailInfo = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	pgSocialCall(t, svc, cs[0], "query_rank_list", 8, 1)
	for i, oid := range oids {
		p := pgHumanRead(t, store, oid).Progress
		r := p.Activities.Calendar.Mountain[1]
		if r.Rank != i+1 || !r.Issued || r.Bonus != 20407001 || len(p.ShortMailInfo) != 1 {
			t.Fatal("真实数据库山海七日名次与奖表错误", r)
		}
		for _, m := range p.ShortMailInfo {
			if m.Attachments[80031] != 1 || m.Attachments[12] != 1000000 {
				t.Fatal("真实数据库山海原生排名附件错误", m)
			}
		}
	}
	now = time.Date(2026, 10, 21, 23, 59, 59, 0, zone)
	pgSocialCall(t, svc, cs[0], "query_rank_list", 10, 20200001)
	for i, oid := range oids {
		p := pgHumanRead(t, store, oid).Progress
		r := p.Activities.Calendar.NianDaily[20200001]
		w := p.Activities.Calendar.NianWeeks["2026-10-21"]
		if r.Rank != i+1 || r.Issued || r.Bonus != 0 || r.Reason == "" || len(p.ShortMailInfo) != 1 || w.Ranks[0] != i+1 || w.Pending == "" || p.Activities.Nian[20200001].RankDamage != 0 || p.Activities.Nian[20200001].MaxDamage != 999 {
			t.Fatal("真实数据库日奖/周截止/个人成绩保护错误", r, w)
		}
	}
	// 重建Store与Service并重新登录，截止收据继续防重发，未用进程缓存代替数据库。
	reloaded := pgActivityService(t, New(store.pool), func() time.Time { return now })
	info := game.ClientInfo{Account: "活动周期真实1", Password: "pw", Hostnum: 10001}
	c := pgActivityLogin(t, reloaded, info)
	pgSocialCall(t, reloaded, c, "query_rank_list", 8, 1)
	for _, oid := range oids {
		if p := pgHumanRead(t, store, oid).Progress; len(p.ShortMailInfo) != 1 {
			t.Fatal("真实数据库重新登录重复发活动奖")
		}
	}
	// 周四重新参榜：正常日奖仍发放，同样通过持久收据防重复。
	now = time.Date(2026, 10, 22, 23, 59, 58, 0, zone)
	for i, oid := range oids {
		if _, err := store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
			d := p.Activities.Nian[20200001]
			d.Ranked, d.RankDamage, d.RankAt = true, int64(100-i*10), now.Unix()
			p.Activities.Nian[20200001] = d
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	now = now.Add(time.Second)
	// 截止时先完成真实跨日登录，再与同一时刻的冷登录比较；正常补给时钟推进不是重发奖。
	c = pgActivityLogin(t, reloaded, info)
	if p := pgHumanRead(t, store, oids[0]).Progress; p.AchievementLoginDay != "2026-10-22" {
		t.Fatal("周四首次登录未完成真实跨日刷新")
	}
	pgSocialCall(t, reloaded, c, "query_rank_list", 10, 20200001)
	for i, oid := range oids {
		p := pgHumanRead(t, store, oid).Progress
		r := p.Activities.Calendar.NianDaily[20200001]
		if r.Rank != i+1 || !r.Issued || r.Bonus != 20204001 || r.Reason != "" || len(p.ShortMailInfo) != 2 {
			t.Fatal("真实数据库非周三日奖错误", r)
		}
	}
	beforeRetry := [][]byte{}
	for _, oid := range oids {
		raw, _ := json.Marshal(pgHumanRead(t, store, oid).Progress)
		beforeRetry = append(beforeRetry, raw)
	}
	reloaded = pgActivityService(t, New(store.pool), func() time.Time { return now })
	c = pgActivityLogin(t, reloaded, info)
	pgSocialCall(t, reloaded, c, "query_rank_list", 10, 20200001)
	for i, oid := range oids {
		raw, _ := json.Marshal(pgHumanRead(t, store, oid).Progress)
		if !bytes.Equal(raw, beforeRetry[i]) {
			var a, b map[string]json.RawMessage
			_ = json.Unmarshal(beforeRetry[i], &a)
			_ = json.Unmarshal(raw, &b)
			for key, value := range a {
				if !bytes.Equal(value, b[key]) {
					t.Logf("变化字段%s：之前=%s；之后=%s", key, value, b[key])
				}
			}
			t.Fatal("真实数据库周四日奖冷重试修改完整进度", i)
		}
	}
}
