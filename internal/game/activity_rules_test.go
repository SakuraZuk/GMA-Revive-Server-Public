package game

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestActivityRulesNianDailyResetAndCrossMidnight(t *testing.T) {
	start := time.Date(2026, 10, 8, 23, 59, 0, 0, shanghaiZone)
	p := NewProgress(20, start)
	if err := refreshNianDaily(&p, start); err != nil {
		t.Fatal(err)
	}
	bc := &ActivityBattleContext{ActivityID: 328, DungeonID: 20200001, PreparedAt: start.Unix(), Statistics: &ActivityBattleStatistics{Damage: 40}, RankCards: []any{[]any{4401, 1, 1, 0, false}}}
	for i := 0; i < 5; i++ {
		if err := finishNianDungeon(&p, bc, start); err != nil {
			t.Fatal(err)
		}
	}
	if p.Activities.Nian[bc.DungeonID].Progress != 3 {
		t.Fatal("每天前三次进度未封顶")
	}
	if err := recordNianBattleDamage(&p, bc, start); err != nil {
		t.Fatal(err)
	}
	d := p.Activities.Nian[bc.DungeonID]
	d.Bonus = 7
	p.Activities.Nian[bc.DungeonID] = d
	after := start.Add(2 * time.Minute)
	bc.Statistics.Damage = 900
	if err := recordNianBattleDamage(&p, bc, after); err != nil {
		t.Fatal(err)
	}
	d = p.Activities.Nian[bc.DungeonID]
	if d.MaxDamage != 900 || d.TotalDamage != 940 || d.RankDamage != 40 {
		t.Fatal("跨日真实个人记录和排行资格混淆", d)
	}
	if err := refreshNianDaily(&p, after); err != nil {
		t.Fatal(err)
	}
	d = p.Activities.Nian[bc.DungeonID]
	if d.Progress != 0 || d.Bonus != 0 || d.RankDamage != 40 || d.MaxDamage != 900 {
		t.Fatal("每日重置破坏排名或未清领取账本", d)
	}
	bc.PreparedAt = after.Unix()
	bc.Statistics.Damage = 60
	if err := recordNianBattleDamage(&p, bc, after); err != nil {
		t.Fatal(err)
	}
	if p.Activities.Nian[bc.DungeonID].RankDamage != 60 {
		t.Fatal("有效低于个人最高的成绩没有更新分榜")
	}
	before, _ := json.Marshal(p)
	errBack := refreshNianDaily(&p, start)
	afterBack, _ := json.Marshal(p)
	if errBack == nil || !reflect.DeepEqual(before, afterBack) {
		t.Fatal("日期回拨未零变更拒绝")
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var restored Progress
	if err = json.Unmarshal(raw, &restored); err != nil || restored.Activities.NianDay != p.Activities.NianDay || restored.Activities.Nian[bc.DungeonID].RankDamage != 60 {
		t.Fatal("日界与成绩未持久", err)
	}
	// 没有日界的旧档只建立基线，不以缺失字段重发旧奖励。
	restored.Activities.NianDay = ""
	old := restored.Activities.Nian[bc.DungeonID]
	old.Progress, old.Bonus = 3, 7
	restored.Activities.Nian[bc.DungeonID] = old
	if err = refreshNianDaily(&restored, after); err != nil || restored.Activities.Nian[bc.DungeonID].Bonus != 7 {
		t.Fatal("旧档初始化重发进度奖", err)
	}
}

func TestActivityRulesShopDeterministicUnion(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	activitySchedulesForTest(t, now)
	// 当前145项没有重复shop_ids；模拟两项共享只验证确定性分支，不能当本版数据。
	shop, ids := 999990, []int{203, 328}
	originals := map[string]json.RawMessage{}
	for _, id := range ids {
		key := fmt.Sprint(id)
		originals[key] = androidActivities["activity_type"][key]
		var row map[string]any
		if err := json.Unmarshal(originals[key], &row); err != nil {
			t.Fatal(err)
		}
		row["shop_ids"] = []int{shop}
		encoded, _ := json.Marshal(row)
		androidActivities["activity_type"][key] = encoded
	}
	t.Cleanup(func() {
		for key, raw := range originals {
			androidActivities["activity_type"][key] = raw
		}
	})
	for i := 0; i < 100; i++ {
		first, ok := activityForShop(shop)
		if !ok || first != ids[0] {
			t.Fatal("共享商店选择不稳定", shop, ids)
		}
	}
	rows := DefaultPermanentActivitySchedules(now.Add(-24 * time.Hour))
	for i := range rows {
		if rows[i].ID == ids[0] {
			rows[i].Enabled = false
		}
	}
	if err := SetActivitySchedules(rows); err != nil {
		t.Fatal(err)
	}
	p := NewProgress(60, now)
	for id := 100; id <= 1000; id++ {
		p.ClearedDungeons = append(p.ClearedDungeons, id)
	}
	ok, err := activityShopAvailable(p, shop, 60, now)
	if err != nil || !ok {
		t.Fatal("共享商店错误选择第一项关闭活动", shop, ids, err)
	}
	if ok, err = activityShopAvailable(p, 999999, 60, now); err != nil || ok {
		t.Fatal("无关联商品被活动规则放行", err)
	}
}

func TestActivityRulesRankingWholePopulationAndRPC(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "")
	ctx := context.Background()
	accounts := map[string]FixtureAccount{}
	for i := 1; i <= 1105; i++ {
		oid := make([]byte, 12)
		binary.BigEndian.PutUint32(oid[8:], uint32(i))
		p := NewProgress(20, time.Unix(1700000000, 0))
		p.Activities.Mountain = &MountainState{Dungeons: map[int]ActivityProgress{1: {ID: 20402011, Ranked: true, RankHard: 2, Actions: i, Cards: []any{[]any{4401, 1, 1, 0, false}}}}}
		p.Activities.Nian = map[int]ActivityProgress{20200001: {Ranked: true, RankDamage: int64(2000 - i), MaxDamage: 99999}}
		accounts[fmt.Sprint(i)] = FixtureAccount{Avatars: []Avatar{{OID: oid, Hostnum: 10001, Progress: p, Info: AvatarInfo{Nickname: fmt.Sprint(i), Level: 20}}}}
	}
	store := NewFixtureAccounts(accounts)
	own := accounts["1105"].Avatars[0].OID
	for _, kind := range []int{8, 10} {
		sub := 1
		if kind == 10 {
			sub = 20200001
		}
		rows, rank, err := store.ActivityRanking(ctx, kind, sub, own, 10001, 2)
		if err != nil || rank != 1105 || len(rows) != 2 || rows[0].Rank != 1 {
			t.Fatal("个人名次被页面截断", kind, rank, len(rows), err)
		}
		if kind == 8 && rows[0].Score[1] != -1 {
			t.Fatal("山海回合数必须传负AP", rows[0].Score)
		}
	}
	if _, _, err := store.ActivityRanking(ctx, 10, 0, own, 10001, 10); err == nil {
		t.Fatal("未知综合公式生成总榜")
	}
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	activitySchedulesForTest(t, now.Add(-1000*24*time.Hour))
	av := accounts["1105"].Avatars[0]
	av.Progress.ClearedDungeons = []int{601, 610}
	accounts["1105"] = FixtureAccount{Avatars: []Avatar{av}}
	c := NewConnection()
	c.phase = Playing
	c.hostnum = 10001
	c.identity = Identity{Account: "1105", Avatars: []Avatar{av}}
	service := New(store, nil)
	service.Now = func() time.Time { return now }
	pushes, err := service.Handle(ctx, c, "query_mountain_sea_own_rank", []json.RawMessage{json.RawMessage("77"), json.RawMessage("1")})
	if err != nil || len(pushes) == 0 {
		t.Fatal("山海个人名次RPC失败", pushes, err)
	}
	found := false
	for _, v := range pushes {
		if v.Method == "call_client_callback" {
			values := v.Args[1].([]any)
			if len(values) == 1 {
				switch value := values[0].(type) {
				case int:
					found = value == 1105
				case int64:
					found = value == 1105
				}
			}
		}
	}
	if !found {
		t.Fatal("山海个人名次原生回调必须单整数", pushes)
	}
	pushes, err = service.Handle(ctx, c, "query_rank_list", []json.RawMessage{json.RawMessage("8"), json.RawMessage("1")})
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, v := range pushes {
		if v.Method == "on_query_rank_list" && v.Args[0] == "8&1" {
			found = true
		}
	}
	if !found {
		t.Fatal("山海列表未按原生finalID返回", pushes)
	}
}
