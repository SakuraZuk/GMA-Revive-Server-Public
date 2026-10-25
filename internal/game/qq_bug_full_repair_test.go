package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// 原生check_mission_state查询活跃度阈值；数据库收据始终按奖励行编号去重。
func TestQQClaimedDailyWeeklyBoxesProjectNativeThresholds(t *testing.T) {
	s, c, _, _ := basicFixture(t)
	if err := s.updateProgress(context.Background(), c, func(p *Progress) error {
		p.BasicRewards.DailyActive = 100
		p.BasicRewards.WeeklyActive = 500
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"daily", "weekly"} {
		for id := 1; id <= 5; id++ {
			if reply := basicReply(t, s, c, "receive_"+kind+"_active_bonus", 100+id, id); reply[0] != RetSuccess {
				t.Fatal("达到活跃度的真实奖励未领取", kind, id, reply)
			}
		}
	}
	p := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	// 同时覆盖已有旧收据冷加载，无需迁移玩家资产或重复领奖。
	raw, _ := json.Marshal(p)
	var cold Progress
	if err := json.Unmarshal(raw, &cold); err != nil {
		t.Fatal(err)
	}
	for _, state := range []Progress{p, cold} {
		for _, item := range []struct {
			field  string
			scales []int
		}{
			{"daily_bonus_active", []int{20, 40, 60, 80, 100}},
			{"weekly_bonus_active", []int{100, 200, 300, 400, 500}},
		} {
			wire, _ := json.Marshal(basicRewardProperties(state)[item.field])
			var values []int
			if json.Unmarshal(wire, &values) != nil || !reflect.DeepEqual(values, item.scales) {
				t.Fatal("客户端领取判定所需阈值不正确", item.field, string(wire))
			}
		}
	}
	if !reflect.DeepEqual(p.BasicRewards.DailyClaims, []int{1, 2, 3, 4, 5}) || !reflect.DeepEqual(p.BasicRewards.WeeklyClaims, []int{1, 2, 3, 4, 5}) {
		t.Fatal("内部奖励行收据被错误改为阈值")
	}
	before, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
	if basicReply(t, s, c, "receive_daily_active_bonus", 201, 1)[0] == RetSuccess || basicReply(t, s, c, "receive_weekly_active_bonus", 202, 1)[0] == RetSuccess {
		t.Fatal("重复领取成功")
	}
	after, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
	if string(before) != string(after) {
		t.Fatal("重复领取改变资产")
	}
}

func TestQQActivityDayDungeonGate(t *testing.T) {
	previous := configuredActivities.Load()
	defer func() {
		if previous != nil {
			configuredActivities.Store(previous)
		} else {
			configuredActivities.Store(map[int]ActivitySchedule{})
		}
	}()
	begin := time.Date(2026, 10, 1, 23, 30, 0, 0, time.FixedZone("北京时间", 8*3600))
	if err := SetActivitySchedules([]ActivitySchedule{{ID: 210, Begin: begin.Unix(), End: begin.Add(7 * 24 * time.Hour).Unix(), Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	r := activityData("dungeons", 21000104)
	p := NewProgress(60, begin)
	p.ClearedDungeons = append(p.ClearedDungeons, 601)
	for _, tc := range []struct {
		now  time.Time
		want bool
	}{
		{begin.Add(-time.Second), false}, {begin.Add(24 * time.Hour), false},
		{time.Date(2026, 10, 3, 0, 0, 0, 0, begin.Location()), true},
		{begin.Add(8 * 24 * time.Hour), false},
	} {
		if got := r.conditions("unlock_condition", p, 60, tc.now) == nil; got != tc.want {
			t.Fatal("活动第3天门槛不符", tc.now, got)
		}
	}
	if _, err := prepareOrdinaryDungeonCosts(&p, 21000104, begin.Add(48*time.Hour)); err != nil {
		t.Fatal("实际副本入场仍拒绝活动天数", err)
	}
}

func TestQQSearchAndRankNativeIdentity(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	out, err := s.Handle(ctx, cs[0], "search_friend", rawArgs(91, cs[1].SelectedAvatarUnsafe().Info.Nickname, 0, 1, true))
	if err != nil {
		t.Fatal(err)
	}
	list := socialCallback(t, out)[1].([]any)
	if len(list) != 1 || list[0].(map[string]any)["_id"] != ObjectID(hexOf(selectedOID(cs[1]))) {
		t.Fatal("原生好友搜索缺少_id", list)
	}
	_, err = store.UpdateProgress(ctx, selectedOID(cs[1]), func(p *Progress) error {
		p.SyncPvpScore = 2000
		p.SelectedHeadID = 4401
		p.SelectedHeadBoxID = 1001
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.syncPvpRankList(ctx, cs[0], 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range out[0].Args[1].([]any) {
		info := item.(map[string]any)
		if info["eid"] == ObjectID(hexOf(selectedOID(cs[1]))) {
			found = true
			if info["head_id"] != 4401 || info["head_box_id"] != 1001 {
				t.Fatal("排行榜遗漏已选头像框", info)
			}
		}
	}
	if !found {
		t.Fatal("排行榜缺少真实玩家")
	}
}
