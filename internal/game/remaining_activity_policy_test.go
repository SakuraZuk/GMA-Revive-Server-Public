package game

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestRemainingActivityNianWeeklyTotalOriginalPrizeAndRetry(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	now := time.Date(2026, 10, 15, 10, 0, 0, 0, shanghaiZone)
	svc, store, c := activityCalendarFixture(t, now)
	svc.Now = func() time.Time { return now }
	ctx := context.Background()
	if err := svc.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	now = time.Date(2026, 10, 21, 23, 59, 59, 0, shanghaiZone)
	if err := svc.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	rows, _ := store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 10})
	for i, av := range rows {
		r := av.Progress.Activities.Calendar.NianWeeks["2026-10-21"]
		bonus, err := activityRankBonus("monster_nian_total_rank", i+1)
		if err != nil || !r.Issued || r.TotalRank != i+1 || r.Bonus != bonus || r.Pending != "" || r.Ranks != [3]int{i + 1, 1, 1} {
			t.Fatal("三榜综合周奖/空榜惩罚/原奖表", r, err)
		}
		if av.Progress.Activities.Nian[20200001].RankDamage != 0 || av.Progress.Activities.Nian[20200001].MaxDamage != 999 {
			t.Fatal("清周榜删除个人纪录")
		}
	}
	if err := svc.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal(err)
	}
	after, _ := store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 10})
	for i := range rows {
		a, _ := json.Marshal(rows[i].Progress)
		b, _ := json.Marshal(after[i].Progress)
		if !reflect.DeepEqual(a, b) {
			t.Fatal("周三结奖重试重写存档")
		}
	}
}

func remainingCthulhuFixture() Progress {
	p := NewProgress(40, time.Unix(1800000000, 0))
	p.Materials[53] = Material{Count: 100, Total: 100}
	p.Materials[54] = Material{Count: 6, Total: 6}
	item := &CthulhuItem{ID: 1201, Index: -1, Status: 1}
	p.Activities.Cthulhu = &CthulhuState{Maps: map[int]*CthulhuMap{1: {ID: 1, Layer: 1010, Layers: map[int]int{1010: 1}, Trunks: map[int]*CthulhuGrid{0: {Item: item}}}}}
	return p
}

func TestRemainingActivityCthulhuFixBoundaryBigSANAllInReload(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	now := time.Unix(1800000000, 0)
	p := remainingCthulhuFixture()
	receipt, err := performCthulhuCheck(&p, 1, 1201, false, now, &[2]int{4, 4})
	if err != nil || !receipt.Success || receipt.Big {
		t.Fatal("原生骰和+fix等于属性应成功", receipt, err)
	}
	before, _ := json.Marshal(p)
	if _, err = performCthulhuCheck(&p, 1, 1201, false, now, &[2]int{6, 6}); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(p)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("检定重试重掷或重复奖")
	}
	p = remainingCthulhuFixture()
	first, err := performCthulhuCheck(&p, 1, 1201, false, now, &[2]int{6, 6})
	if err != nil || first.Success || !first.Big || p.Materials[53].Count != 90 {
		t.Fatal("大失败未减原表10SAN", err, first, p.Materials[53])
	}
	raw, _ := json.Marshal(p)
	p = Progress{}
	if err = json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	second, err := performCthulhuCheck(&p, 1, 1201, true, now, &[2]int{1, 1})
	if err != nil || !second.Success || !second.AllIn || p.Materials[53].Count != 90 || p.Activities.Cthulhu.Maps[1].Checks != 2 {
		t.Fatal("all-in成本20、大成功20、冷恢复次数", err, second, p.Materials[53])
	}
	raw, _ = json.Marshal(p)
	if _, err = performCthulhuCheck(&p, 1, 1201, true, now, &[2]int{6, 6}); err != nil {
		t.Fatal(err)
	}
	after, _ = json.Marshal(p)
	if !reflect.DeepEqual(raw, after) {
		t.Fatal("all-in重试重复成本/奖励")
	}
	// 重掷完成后迟到的首次RPC仍回其原失败骰值，不改当前成功状态。
	late, err := performCthulhuCheck(&p, 1, 1201, false, now, &[2]int{1, 1})
	if err != nil || late.Dice != [2]int{6, 6} || late.AllIn || late.Success {
		t.Fatal("重掷后迟到首次重试丢失原结果", late, err)
	}
	after, _ = json.Marshal(p)
	if !reflect.DeepEqual(raw, after) {
		t.Fatal("迟到首次重试回退重掷状态")
	}
	p = remainingCthulhuFixture()
	p.Materials[53] = Material{Count: 5, Total: 5}
	_, err = performCthulhuCheck(&p, 1, 1201, false, now, &[2]int{6, 6})
	if err != nil || p.Materials[53].Count != 0 {
		t.Fatal("SAN零下界", err)
	}
	if _, err = performCthulhuCheck(&p, 1, 1201, true, now, &[2]int{1, 1}); err == nil {
		t.Fatal("不足20SAN仍all-in")
	}
}

func TestRemainingActivityMikuSurpriseOnceReloadAndEligibility(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	now := time.Unix(1800000000, 0)
	p := NewProgress(40, now)
	p.Activities.Miku = &MikuState{Maps: map[int]*MikuMap{1: {ID: 1}}}
	if _, err := receiveMikuLocalSurprise(&p, 1, now); err == nil {
		t.Fatal("未到终点领取惊喜")
	}
	p.Activities.Miku.Maps[1].CompletedRuns = 1
	if _, err := receiveMikuLocalSurprise(&p, 1, now); err != nil {
		t.Fatal(err)
	}
	if p.Materials[208095].Count != 10 {
		t.Fatal("惊喜对应和声数值错误", p.Materials[208095])
	}
	raw, _ := json.Marshal(p)
	p = Progress{}
	_ = json.Unmarshal(raw, &p)
	if _, err := receiveMikuLocalSurprise(&p, 1, now); err != nil {
		t.Fatal(err)
	}
	if p.Materials[208095].Count != 10 {
		t.Fatal("冷恢复惊喜重复领")
	}
}

func TestRemainingActivityNianCompositeHourlyStableAndAbsent(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	now := time.Date(2026, 10, 9, 10, 0, 0, 0, shanghaiZone)
	firstOID, _ := SocialOID("111111111111111111111111")
	first := &Avatar{OID: firstOID, Hostnum: 3, Progress: NewProgress(20, now)}
	first.Progress.Activities.Nian = map[int]ActivityProgress{20200001: {Ranked: true, RankDamage: 123, RankAt: now.Add(-time.Minute).Unix()}}
	freezeNianHourRanking(map[string]*Avatar{hexOf(firstOID): first}, now)
	if first.Progress.Activities.Calendar == nil || first.Progress.Activities.Calendar.NianTotal == nil || first.Progress.Activities.Calendar.NianTotal.Rank != 1 {
		t.Fatal("首见玩家无旧Calendar未建立本整点榜")
	}
	rows := map[string]*Avatar{}
	for i := 1; i <= 1105; i++ {
		oid, _ := SocialOID(fmt.Sprintf("%024x", i))
		p := NewProgress(20, now)
		p.Activities.Calendar = &ActivityCalendar{}
		p.Activities.Nian = map[int]ActivityProgress{20200001: {Ranked: true, RankDamage: int64(2000 - i), RankAt: now.Add(-time.Minute).Unix()}}
		rows[hexOf(oid)] = &Avatar{OID: oid, Hostnum: 1, Progress: p}
	}
	oid, _ := SocialOID(fmt.Sprintf("%024x", 1106))
	p := NewProgress(20, now)
	p.Activities.Calendar = &ActivityCalendar{}
	rows[hexOf(oid)] = &Avatar{OID: oid, Hostnum: 2, Progress: p}
	freezeNianHourRanking(rows, now)
	own := rows[fmt.Sprintf("%024x", 1105)]
	if own.Progress.Activities.Calendar.NianTotal.Rank != 1105 || own.Progress.Activities.Calendar.NianTotal.SubRanks != [3]int{1105, 1, 1} {
		t.Fatal("全量个人名次/未参与分榜人数+1", own.Progress.Activities.Calendar.NianTotal)
	}
	d := own.Progress.Activities.Nian[20200001]
	recordNianScoreHour(&own.Progress, 20200001, d)
	d.RankDamage = 99999
	d.RankAt = now.Add(time.Minute).Unix()
	own.Progress.Activities.Nian[20200001] = d
	recordNianScoreHour(&own.Progress, 20200001, d)
	freezeNianHourRanking(rows, now.Add(30*time.Minute))
	if own.Progress.Activities.Calendar.NianTotal.Rank != 1105 {
		t.Fatal("小时内改分提前刷新榜")
	}
	freezeNianHourRanking(rows, now.Add(time.Hour))
	if own.Progress.Activities.Calendar.NianTotal.Rank != 1 {
		t.Fatal("下一小时未更新榜")
	}
	if rows[hexOf(oid)].Progress.Activities.Calendar.NianTotal.Rank != 0 {
		t.Fatal("另一服未参赛凭空上榜")
	}
}

func TestRemainingActivityNianZeroResultCountsAndMissingStaysAbsent(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	now := time.Date(2026, 10, 9, 10, 0, 0, 0, shanghaiZone)
	rows := map[string]*Avatar{}
	for i := 1; i <= 4; i++ {
		oid, _ := SocialOID(fmt.Sprintf("%024x", i))
		p := NewProgress(20, now)
		p.Activities.Nian = map[int]ActivityProgress{20200001: {}}
		if i <= 2 {
			bc := &ActivityBattleContext{DungeonID: 20200001, PreparedAt: now.Add(-2 * time.Minute).Unix(), Statistics: &ActivityBattleStatistics{Damage: 0}}
			if i == 1 {
				bc.Statistics.Damage = 99
			}
			if err := recordNianBattleDamage(&p, bc, now.Add(-time.Minute)); err != nil {
				t.Fatal(err)
			}
		}
		if i == 4 {
			p.Activities.Nian[20200001] = ActivityProgress{Ranked: true, RankDamage: 0}
		} // 旧缺时间的未知字段不能补造参与。
		rows[hexOf(oid)] = &Avatar{OID: oid, Hostnum: 1, Progress: p}
	}
	zero := rows[fmt.Sprintf("%024x", 2)]
	if !zero.Progress.Activities.Nian[20200001].Ranked || zero.Progress.Activities.Nian[20200001].RankAt == 0 || len(zero.Progress.Activities.NianHistory[20200001]) != 1 {
		t.Fatal("真实0伤害result未建参赛/小时收据")
	}
	if _, ok := activityRankValue(*zero, 10, 20200001); !ok {
		t.Fatal("分榜查询漏真实0分参赛")
	}
	if _, ok := activityRankValue(*rows[fmt.Sprintf("%024x", 4)], 10, 20200001); ok {
		t.Fatal("旧无时间零字段补造分榜")
	}
	freezeNianHourRanking(rows, now)
	positiveRank := rows[fmt.Sprintf("%024x", 1)].Progress.Activities.Calendar.NianTotal
	zeroRank := zero.Progress.Activities.Calendar.NianTotal
	if positiveRank.Rank != 1 || zeroRank.Rank != 2 || zeroRank.SubRanks != [3]int{2, 1, 1} || zeroRank.Joined != 1 {
		t.Fatal("正分排序/真实零分参榜名次错误", positiveRank, zeroRank)
	}
	for _, i := range []int{3, 4} {
		if rows[fmt.Sprintf("%024x", i)].Progress.Activities.Calendar.NianTotal.Rank != 0 {
			t.Fatal("未参赛/旧未知凭空入总榜", i)
		}
	}
	legacy := CloneProgress(rows[fmt.Sprintf("%024x", 4)].Progress)
	if err := recordNianBattleDamage(&legacy, &ActivityBattleContext{DungeonID: 20200001, PreparedAt: now.Add(-2 * time.Minute).Unix(), Statistics: &ActivityBattleStatistics{}}, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if legacy.Activities.Nian[20200001].RankAt <= 0 || len(legacy.Activities.NianHistory[20200001]) != 1 {
		t.Fatal("旧未知玩家后来真实零分仍无法建立新收据")
	}
	// 真零结果经过JSONB形状冷恢复依旧存在；跨零点零结果依旧不计榜。
	raw, _ := json.Marshal(zero.Progress)
	restored := Progress{}
	_ = json.Unmarshal(raw, &restored)
	if value, ok := nianScoreBefore(restored, 20200001, now.Unix(), now.Add(-7*24*time.Hour).Unix()); !ok || value.Damage != 0 {
		t.Fatal("冷恢复丢零分结果")
	}
	p := NewProgress(20, now)
	p.Activities.Nian = map[int]ActivityProgress{20200001: {}}
	if err := recordNianBattleDamage(&p, &ActivityBattleContext{DungeonID: 20200001, PreparedAt: now.Add(-12 * time.Hour).Unix(), Statistics: &ActivityBattleStatistics{}}, now); err != nil {
		t.Fatal(err)
	}
	if p.Activities.Nian[20200001].Ranked {
		t.Fatal("跨日零分结果计榜")
	}
}

func TestRemainingActivityMountainCyclePreservesAssetsAndRounds(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, shanghaiZone)
	_, _, _ = activityCalendarFixture(t, now)
	end := mountainRankCutoff(1)
	season, _, cutoff := mountainSeasonAt(1, time.Unix(end+7*24*3600-1, 0))
	if season != 1 || cutoff != end+7*24*3600 {
		t.Fatal("7天窗口/22点截止", season, cutoff)
	}
	p := NewProgress(20, now)
	w := ensureMountain(&p)
	w.Dungeons[1] = ActivityProgress{Progress: 123, Bonus: 3}
	w.Circles[1].Progress = 200
	ensureMountainSeason(w, 1, 1)
	if w.Dungeons[1].Progress != 0 || w.Dungeons[1].Bonus != 0 || w.Seasons[1][0].Progress != 123 || w.Circles[1].Progress != 200 {
		t.Fatal("换季未独立进度或清资产", w)
	}
	box := emptyActivityBox()
	box["materials"] = map[int]int64{203001: 3}
	if err := applyFrozenMountainMaterials(&p, &ActivityBattleContext{MaterialRates: map[int]float64{203001: 0.5}}, box, now); err != nil {
		t.Fatal(err)
	}
	if box["materials"].(map[int]int64)[203001] != 4 || p.Materials[203001].Count != 1 {
		t.Fatal("合计倍率向下取整未只加额外资产", box, p.Materials[203001])
	}
}

func TestRemainingActivityMountainLegacyNullJSONMigratesAndReloads(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	now := time.Date(2026, 10, 21, 21, 59, 58, 0, shanghaiZone)
	svc, store, c := activityCalendarFixture(t, now)
	svc.Now = func() time.Time { return now }
	// 实际旧档JSON无season字段，并明确旧mountain_dungeon为空；经存储冷拷贝后触发全服周期刷新。
	var legacy MountainState
	if err := json.Unmarshal([]byte(`{"magic_circles":{},"mountain_dungeon":null}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Dungeons != nil || legacy.Seasons != nil || legacy.ActiveSeasons != nil {
		t.Fatal("夹具未保持旧档null形状")
	}
	ctx := context.Background()
	if _, err := store.UpdateProgress(ctx, selectedOID(c), func(p *Progress) error { p.Activities.Mountain = &legacy; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := svc.refreshActivityAwards(ctx, c); err != nil {
		t.Fatal("正式规则迁移旧null档失败", err)
	}
	p := socialSaved(t, store, c).Progress
	w := p.Activities.Mountain
	if w == nil || w.Dungeons == nil || w.Seasons == nil || w.ActiveSeasons == nil || len(p.ShortMailInfo) != 0 {
		t.Fatal("迁移未初始化或空成绩补造奖", w)
	}
	raw, _ := json.Marshal(p)
	var restored Progress
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	ensureMountainSeason(restored.Activities.Mountain, 1, 1)
	if restored.Activities.Mountain.Dungeons[1].Progress != 0 || restored.Activities.Mountain.Seasons[1][0].Ranked {
		t.Fatal("迁移后新季有伪造进度/旧成绩")
	}
	// 历史有效进度也必须保持到0季，重赛清领奖位但不删除归档。
	var oldScore MountainState
	if err := json.Unmarshal([]byte(`{"magic_circles":{},"mountain_dungeon":{"1":{"current_progress":123,"bonus_progress":3,"server_ranked":true,"server_rank_hard":2}}}`), &oldScore); err != nil {
		t.Fatal(err)
	}
	ensureMountainSeason(&oldScore, 1, 1)
	if oldScore.Seasons[1][0].Progress != 123 || oldScore.Seasons[1][0].Bonus != 3 || !oldScore.Seasons[1][0].Ranked || oldScore.Dungeons[1].Bonus != 0 {
		t.Fatal("有进度旧JSON迁移丢归档/沿用旧领奖位", oldScore)
	}
}
