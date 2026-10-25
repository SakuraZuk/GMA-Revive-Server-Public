package dbstore

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestPostgresEmptyNianHourLeavesDormantRevisionUnchanged(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 2, 0, 0, 0, time.UTC)
	svc := pgSocialService(t, store)
	svc.Now = func() time.Time { return now }
	identity, err := store.Register(ctx, game.ClientInfo{Account: "空榜整点", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.RunPeriodicMaintenanceOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var before, after int64
	oid := identity.Avatars[0].OID
	if err = store.pool.QueryRow(ctx, `SELECT revision FROM avatar_progress WHERE avatar_oid=$1`, oid).Scan(&before); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if err = svc.RunPeriodicMaintenanceOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if err = store.pool.QueryRow(ctx, `SELECT revision FROM avatar_progress WHERE avatar_oid=$1`, oid).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("空活动榜在下一小时仍写入数据库", before, after)
	}
	_, err = store.UpdateSocial(ctx, []string{hex.EncodeToString(oid)}, func(v map[string]*game.Avatar) error {
		v[hex.EncodeToString(oid)].Progress.Activities.Nian = map[int]game.ActivityProgress{20200001: {Ranked: true, RankAt: now.Add(time.Minute).Unix(), RankDamage: 0}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if err = svc.RunPeriodicMaintenanceOnce(ctx); err != nil {
		t.Fatal(err)
	}
	av := pgHumanRead(t, New(store.pool), oid)
	if row, ok := game.FrozenNianTotalRank(av); !ok || row.Rank != 1 {
		t.Fatal("实库真实零伤害首次参赛没有进入下一整点榜")
	}
}

func TestPostgresAsyncIdleCalendarColdEntryAndNextDayAward(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 11, 12, 59, 0, 0, time.UTC)
	svc := pgSocialService(t, store)
	svc.Now = func() time.Time { return now }
	info := game.ClientInfo{Account: "竞技空周期", Password: "pw", Hostnum: 1}
	identity, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	info2 := game.ClientInfo{Account: "竞技休眠档", Password: "pw", Hostnum: 1}
	other, err := store.Register(ctx, info2)
	if err != nil {
		t.Fatal(err)
	}
	c := pgSocialLogin(t, svc, info, identity.Avatars[0].OID)
	ids := []string{hex.EncodeToString(identity.Avatars[0].OID), hex.EncodeToString(other.Avatars[0].OID)}
	_, err = store.UpdateSocial(ctx, ids, func(v map[string]*game.Avatar) error {
		for _, id := range ids {
			p := &v[id].Progress
			p.UnlockSystems["panel_async_pvp"] = 1
			p.AsyncPvp = game.AsyncPvpState{RewardDay: "2026-10-01", RewardWeek: "2026-09-27"}
			p.Social.Assist.Day = "2026-10-11"
			p.Social.Assist.Active = map[string]int{}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var before, after int64
	if err = store.pool.QueryRow(ctx, `SELECT revision FROM avatar_progress WHERE avatar_oid=$1`, other.Avatars[0].OID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	pgSocialCall(t, svc, c, "refresh_assist_use_times")
	now = now.Add(2 * time.Minute)
	pgSocialCall(t, svc, c, "refresh_assist_use_times")
	if err = store.pool.QueryRow(ctx, `SELECT revision FROM avatar_progress WHERE avatar_oid=$1`, other.Avatars[0].OID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("跨21点仍改写未参赛离线存档", before, after)
	}
	svc.Detach(c)
	fresh := pgSocialService(t, New(store.pool))
	fresh.Now = func() time.Time { return now }
	cold := pgSocialLogin(t, fresh, info, identity.Avatars[0].OID)
	pgSocialCall(t, fresh, cold, "enter_asyn_pvp")
	p := pgHumanRead(t, New(store.pool), identity.Avatars[0].OID).Progress
	if p.AsyncPvp.Score != 1000 || p.AsyncPvp.RewardDay != "2026-10-11" || p.AsyncPvp.RewardWeek != "2026-10-11" {
		t.Fatal("真实库冷入口未保存首次参赛基线")
	}
	pgSocialCall(t, fresh, cold, "refresh_assist_use_times")
	if len(pgHumanRead(t, store, identity.Avatars[0].OID).Progress.ShortMailInfo) != 0 {
		t.Fatal("首次参赛补发历史奖")
	}
	now = now.AddDate(0, 0, 1)
	pgSocialCall(t, fresh, cold, "refresh_assist_use_times")
	if len(pgHumanRead(t, store, identity.Avatars[0].OID).Progress.ShortMailInfo) != 1 {
		t.Fatal("首次参赛后次日奖励缺失")
	}
	pgSocialCall(t, fresh, cold, "refresh_assist_use_times")
	if len(pgHumanRead(t, store, identity.Avatars[0].OID).Progress.ShortMailInfo) != 1 {
		t.Fatal("次日奖励重复发放")
	}
}
