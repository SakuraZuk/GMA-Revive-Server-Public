package game

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func activityCalendarFixture(t *testing.T, now time.Time) (*Service, *FixtureAccounts, *Connection) {
	t.Helper()
	previous := configuredActivities.Load()
	t.Cleanup(func() {
		if previous == nil {
			configuredActivities.Store(map[int]ActivitySchedule{})
		} else {
			configuredActivities.Store(previous)
		}
	})
	begin := time.Date(2026, 10, 7, 0, 0, 0, 0, shanghaiZone)
	if err := SetActivitySchedules(DefaultPermanentActivitySchedules(begin)); err != nil {
		t.Fatal(err)
	}
	records := map[string]FixtureAccount{}
	for i := 1; i <= 2; i++ {
		oid, _ := SocialOID(fmt.Sprintf("%024x", i))
		p := NewProgress(20, now)
		p.ClearedDungeons = []int{601, 610}
		p.Activities.Mountain = &MountainState{Dungeons: map[int]ActivityProgress{1: {ID: 20321111, Ranked: true, RankHard: 2, Actions: i * 10}}}
		p.Activities.Nian = map[int]ActivityProgress{20200001: {ID: 20200001, Ranked: true, RankDamage: int64(110 - i*10), RankAt: now.Unix(), MaxDamage: 999}}
		records[fmt.Sprint(i)] = FixtureAccount{Avatars: []Avatar{{Account: fmt.Sprint(i), OID: oid, Hostnum: 1, Info: AvatarInfo{Nickname: fmt.Sprintf("排行%d", i), Level: 20}, Progress: p}}}
	}
	store := NewFixtureAccounts(records)
	service := New(store, nil)
	service.Now = func() time.Time { return now }
	c := NewConnection()
	c.phase = Playing
	c.hostnum = 1
	c.identity = Identity{Account: "1", Avatars: records["1"].Avatars}
	return service, store, c
}
func TestActivityRulesCalendarNativeDailyMountainAndPendingWeek(t *testing.T) {
	// 此项保留原“未启用本服综合公式”的Pending分支；获批周奖由Remaining专项覆盖。
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "")
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 21, 59, 58, 0, shanghaiZone)
	service, store, c := activityCalendarFixture(t, now)
	service.Now = func() time.Time { return now }
	if err := service.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	rows, _ := store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 10})
	for _, row := range rows {
		if len(row.Progress.ShortMailInfo) != 0 {
			t.Fatal("首次建立基线补发历史奖")
		}
	}
	now = now.Add(2 * time.Second)
	if err := service.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	rows, _ = store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 10})
	for i, row := range rows {
		receipt := row.Progress.Activities.Calendar.Mountain[1]
		if !receipt.Closed || !receipt.Issued || receipt.Rank != i+1 || receipt.Bonus != 20407001 || len(row.Progress.ShortMailInfo) != 1 {
			t.Fatal("山海七日22点邮件/冻结名次错误", receipt, row.Progress.ShortMailInfo)
		}
		for _, mail := range row.Progress.ShortMailInfo {
			if mail.Attachments[80031] != 1 || mail.Attachments[12] != 1000000 {
				t.Fatal("山海原生排名资产不符", mail)
			}
		}
	}
	now = time.Date(2026, 10, 21, 23, 59, 59, 0, shanghaiZone)
	if err := service.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	rows, _ = store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 10})
	for i, row := range rows {
		cal := row.Progress.Activities.Calendar
		daily := cal.NianDaily[20200001]
		weekly := cal.NianWeeks["2026-10-21"]
		if daily.Rank != i+1 || daily.Bonus != 0 || daily.Issued || daily.Reason == "" || len(row.Progress.ShortMailInfo) != 1 {
			t.Fatal("周三错误发放年兽日奖", daily, row.Progress.ShortMailInfo)
		}
		if weekly.Ranks[0] != i+1 || weekly.Pending == "" || row.Progress.Activities.Nian[20200001].RankDamage != 0 || row.Progress.Activities.Nian[20200001].MaxDamage != 999 {
			t.Fatal("周截止冻结/清榜/保留个人伤害错误", weekly, row.Progress.Activities.Nian)
		}
	}
	if err := service.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	// Avatar的JSON标签不含Progress，比较每个真实存档而不是空表面字段。
	after, _ := store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 10})
	for i := range rows {
		a, _ := json.Marshal(rows[i].Progress)
		b, _ := json.Marshal(after[i].Progress)
		if !reflect.DeepEqual(a, b) {
			t.Fatal("周期重试重复邮件或重写名次")
		}
	}
}
func TestActivityRulesCalendarMailboxRollbackAndRetry(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 21, 59, 58, 0, shanghaiZone)
	service, store, c := activityCalendarFixture(t, now)
	service.Now = func() time.Time { return now }
	if err := service.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	oid, _ := SocialOID(fmt.Sprintf("%024x", 2))
	if _, err := store.UpdateProgress(ctx, oid, func(p *Progress) error {
		p.ShortMailInfo = map[int]Mail{}
		for i := 1; i <= 500; i++ {
			p.ShortMailInfo[i] = Mail{MID: i, UUID: fmt.Sprintf("%024x", i+1000), Title: "隔离邮箱夹具"}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 10})
	now = now.Add(2 * time.Second)
	if err := service.refreshActivityAwards(ctx, c); err == nil {
		t.Fatal("邮箱已满仍完成排名奖励")
	}
	after, _ := store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 10})
	for i := range before {
		a, _ := json.Marshal(before[i].Progress)
		b, _ := json.Marshal(after[i].Progress)
		if !reflect.DeepEqual(a, b) {
			t.Fatal("全服奖励失败部分提交", i)
		}
	}
	if _, err := store.UpdateProgress(ctx, oid, func(p *Progress) error { p.ShortMailInfo = map[int]Mail{}; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := service.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	after, _ = store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 10})
	for _, row := range after {
		if len(row.Progress.ShortMailInfo) != 1 || !row.Progress.Activities.Calendar.Mountain[1].Issued {
			t.Fatal("修复容量后未原子重试", row.Progress.Activities.Calendar)
		}
	}
}
func TestActivityRulesCalendarNonWednesdayDailyReward(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 22, 23, 59, 58, 0, shanghaiZone)
	service, store, c := activityCalendarFixture(t, now)
	service.Now = func() time.Time { return now }
	if err := service.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if err := service.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	rows, _ := store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 10})
	for i, row := range rows {
		r := row.Progress.Activities.Calendar.NianDaily[20200001]
		if !r.Closed || !r.Issued || r.Rank != i+1 || r.Bonus != 20204001 || r.Reason != "" || len(row.Progress.ShortMailInfo) != 1 {
			t.Fatal("非周三年兽原生日奖错误", r)
		}
	}
}
func TestActivityRulesCalendarZeroRankLateInitialAndAllNativeMailTemplates(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 23, 59, 59, 0, shanghaiZone)
	service, store, c := activityCalendarFixture(t, now)
	if err := service.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	rows, _ := store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 10})
	for _, row := range rows {
		cal := row.Progress.Activities.Calendar
		if len(row.Progress.ShortMailInfo) != 0 || cal.Mountain[1].Missing == "" || len(cal.NianDaily) != 0 || len(cal.NianWeeks) != 0 {
			t.Fatal("晚初见玩家用当前成绩补发历史奖", cal)
		}
	}
	for _, bid := range []int{20204001, 20204002, 20204003, 20407001, 20407002, 20407003, 20407004} {
		p := NewProgress(20, now)
		if err := service.issueActivityRankMail(&p, bid, fmt.Sprint(bid), now); err != nil {
			t.Fatal("已支持的实际原生模板不能生成邮件", bid, err)
		}
		if err := service.issueActivityRankMail(&p, bid, fmt.Sprint(bid), now); err != nil || len(p.ShortMailInfo) != 1 {
			t.Fatal("实际模板重试重复发件", bid, err)
		}
		for _, mail := range p.ShortMailInfo {
			if mail.Title == "" || mail.Content == "" || len(mail.Attachments) != 2 {
				t.Fatal("实际模板或资产缺失", bid, mail)
			}
		}
	}
	if bid, err := activityRankBonus("mountain_game_rank", 0); err != nil || bid != 0 {
		t.Fatal("零名次获得排名奖励", bid, err)
	}
	// 未参榜角色到达截止仍写关闭收据，但不领取已参榜角色的奖励。
	now = time.Date(2026, 10, 21, 21, 59, 58, 0, shanghaiZone)
	service, store, c = activityCalendarFixture(t, now)
	service.Now = func() time.Time { return now }
	oid, _ := SocialOID(fmt.Sprintf("%024x", 2))
	if _, err := store.UpdateProgress(ctx, oid, func(p *Progress) error {
		p.Activities.Mountain.Dungeons = nil
		p.Activities.Nian = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	now = time.Date(2026, 10, 21, 23, 59, 59, 0, shanghaiZone)
	if err := service.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	rows, _ = store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 10})
	if rows[1].Progress.Activities.Calendar.Mountain[1].Rank != 0 || len(rows[1].Progress.ShortMailInfo) != 0 || rows[1].Progress.Activities.Calendar.NianDaily[20200001].Issued {
		t.Fatal("零名次角色截止发奖", rows[1].Progress.Activities.Calendar)
	}
}
func TestActivityRulesCalendarLateBestDoesNotForgePastRanks(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 21, 59, 58, 0, shanghaiZone)
	service, store, c := activityCalendarFixture(t, now)
	service.Now = func() time.Time { return now }
	if err := service.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	now = time.Date(2026, 10, 23, 10, 0, 0, 0, shanghaiZone)
	oid, _ := SocialOID(fmt.Sprintf("%024x", 2))
	if _, err := store.UpdateProgress(ctx, oid, func(p *Progress) error {
		d := p.Activities.Nian[20200001]
		d.RankDamage = 9999
		d.RankAt = now.Unix()
		p.Activities.Nian[20200001] = d
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	rows, _ := store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 10})
	for _, row := range rows {
		week := row.Progress.Activities.Calendar.NianWeeks["2026-10-21"]
		if week.Ranks != [3]int{} || week.Pending == "" {
			t.Fatal("截止后新best被当旧周榜或删人后重排", week)
		}
		if daily := row.Progress.Activities.Calendar.NianDaily[20200001]; daily.Issued || daily.Missing == "" {
			t.Fatal("缺历史仍生成日奖", daily)
		}
	}
}
func TestActivityRulesMountainGuardNativeFrozenExtra(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, shanghaiZone)
	p := NewProgress(20, now)
	p.Cards = append(p.Cards, Card{UUID: "000000000000000000004409", CardID: 4409, Level: 20})
	w := ensureMountain(&p)
	w.Circles[3].Progress = 500
	w.Circles[3].Unlocked = []bool{true, true, true}
	w.Circles[3].Cards = []int{0, 4401, 0}
	w.Circles[1].Progress = 500
	w.Circles[1].Unlocked = []bool{true, true, true}
	w.Circles[1].Cards = []int{0, 0, 4409}
	bc := &ActivityBattleContext{ActivityID: 203, DungeonID: 20311111}
	if err := freezeMountainGuardEffects(&p, bc, activityData("mountain_game_dungeon", bc.DungeonID)); err != nil {
		t.Fatal(err)
	}
	if len(bc.NativeBuffs[1]) != 2 || bc.NativeBuffs[1][0].Property != 6 || len(bc.NativeMFields[1]) != 1 || bc.NativeMFields[1][0] != [3]int{4031, 3, 2} {
		t.Fatal("原生守护公式/结界等级冻结错误", bc)
	}
	extra := activityBattleExtra(bc, map[string]any{"change_attr_data": map[string]any{"伪buff": true}})
	attrs := extra["change_attr_data"].(map[string]any)
	if _, bad := attrs["伪buff"]; bad {
		t.Fatal("客户端任意加成未去除")
	}
	w.Circles[3].Progress = 1000
	w.Circles[3].Cards = []int{0, 3101, 0}
	if bc.NativeBuffs[1][0].Property != 6 {
		t.Fatal("开战后改守护覆盖冻结buff")
	}
	raw, _ := json.Marshal(bc)
	var restored ActivityBattleContext
	if err := json.Unmarshal(raw, &restored); err != nil || restored.NativeMFields[1][0] != [3]int{4031, 3, 2} {
		t.Fatal("守护上下文未真实持久", err)
	}
}
