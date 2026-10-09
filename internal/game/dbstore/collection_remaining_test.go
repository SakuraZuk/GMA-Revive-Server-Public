package dbstore

import (
	"context"
	"hs-server/internal/game"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestPostgresRemainingCollectionFramesFrozenResidentsEnergyTutorialAndRollback(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 8, 6, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	info := game.ClientInfo{Account: "收藏室全分支实库", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, store, info, &now)
	defer svc.Detach(c)
	read := func() game.Progress {
		t.Helper()
		v, e := New(store.pool).AdminPlayer(ctx, av.OID)
		if e != nil {
			t.Fatal(e)
		}
		return game.CloneProgress(v.Progress)
	}
	update := func(fn func(*game.Progress) error) {
		t.Helper()
		if _, e := store.UpdateProgress(ctx, av.OID, fn); e != nil {
			t.Fatal(e)
		}
	}
	call := func(method string, args ...any) []game.Push {
		t.Helper()
		out, e := svc.Handle(ctx, c, method, pgSocialArgs(args...))
		if e != nil {
			t.Fatal(method, e)
		}
		return out
	}
	update(func(p *game.Progress) error {
		p.UnlockSystems["house_dormitory3"] = 1
		p.UnlockSystems["house_daily_random_reward"] = 1
		p.Materials[809] = game.Material{ID: 809, Count: 1, Total: 1}
		p.Materials[801] = game.Material{ID: 801, Count: 1, Total: 1}
		p.GuideTasks[6004] = game.GuideTask{ID: 6004, Status: 1}
		return nil
	})
	call("unlock_dormitory", 809)
	call("unlock_facility", 801)
	call("set_restroom_girls", 1, []int{4401})
	p := read()
	if _, owned := p.Collection.Rooms[3]; !owned {
		t.Fatal("实库系统104门槛缺房3ownership")
	}
	if p.Collection.Rooms[1].Energy["value"] != float64(30) || p.Collection.Rooms[1].Energy["start_flag"] != true {
		t.Fatal("实库原生入住没有启动能量")
	}
	call("gather_produce_material_speed_up", 2, 9000)
	p = read()
	keep := p.Collection.Facilities[2].Keep
	if p.Collection.TutorialAcceleration[6004].Seconds != 9000 || keep <= 0 {
		t.Fatal("实库原件教学没有加速")
	}
	call("gather_produce_material_speed_up", 2, 9000)
	if read().Collection.Facilities[2].Keep != keep {
		t.Fatal("实库教学重试重复加速")
	}
	before := read()
	if _, e := svc.Handle(ctx, c, "gather_produce_material_speed_up", pgSocialArgs(2, 1)); e == nil || !reflect.DeepEqual(before, read()) {
		t.Fatal("实库任意加速没有全事务拒绝", e)
	}
	call("player_enter_house")
	p = read()
	gift, exists := p.Collection.ResidentWindows[1][4401]
	if !exists || !gift.Frozen.RewardsFrozen || len(gift.Frozen.Attachments) == 0 {
		t.Fatal("实库住客候选没有真实冻结结果")
	}
	call("player_enter_house")
	if !reflect.DeepEqual(gift, read().Collection.ResidentWindows[1][4401]) {
		t.Fatal("实库重复进入重抽")
	}
	// 搬出不能领取，原候选仍然冻结；搬回原房间恢复一次领取资格。
	call("set_restroom_girls", 1, []int{})
	before = read()
	if _, e := svc.Handle(ctx, c, "player_get_house_random_reward", pgSocialArgs(1, 4401)); e == nil || !reflect.DeepEqual(before, read()) {
		t.Fatal("实库搬出仍能领礼物或失败改存档", e)
	}
	call("set_restroom_girls", 1, []int{4401})
	// 冻结实际材料奖励溢出，不能消费住客事件收据或先推进成就。
	update(func(p *game.Progress) error {
		g := p.Collection.ResidentWindows[1][4401]
		g.Frozen.Attachments = map[int]int64{12: math.MaxInt64}
		p.Collection.ResidentWindows[1][4401] = g
		p.Materials[12] = game.Material{ID: 12, Count: 1, Total: 1}
		return nil
	})
	before = read()
	if _, e := svc.Handle(ctx, c, "player_get_house_random_reward", pgSocialArgs(1, 4401)); e == nil || !reflect.DeepEqual(before, read()) {
		t.Fatal("实库冻结奖励失败未全回滚", e)
	}
	update(func(p *game.Progress) error { p.Collection.ResidentWindows[1][4401] = gift; return nil })
	out := call("player_get_house_random_reward", 1, 4401)
	reply, at := pgNativeReply(t, out, "on_player_get_house_random_reward")
	pgNativePropertyBefore(t, out, at, "house_daily_random_reward")
	pgNativePropertyBefore(t, out, at, "material_mgr")
	if len(reply.Args) != 2 || reply.Args[0] != game.RetSuccess {
		t.Fatal("实库住客原生回包不符")
	}
	p = read()
	if p.Collection.ResidentGiftCount != 1 || p.Achievements[210501].Targets[210501] != 1 || !p.Collection.ResidentWindows[1][4401].Claimed {
		t.Fatal("实库礼物与成就没有同时保存")
	}
	cold := pgSocialService(t, New(store.pool))
	cold.Now = func() time.Time { return now }
	coldC := pgSocialLogin(t, cold, info, av.OID)
	defer cold.Detach(coldC)
	before = read()
	if _, e := cold.Handle(ctx, coldC, "player_get_house_random_reward", pgSocialArgs(1, 4401)); e == nil || !reflect.DeepEqual(before, read()) {
		t.Fatal("实库冷重建重复发礼物", e)
	}
	now = now.Add(10 * time.Hour)
	call("player_enter_house")
	update(func(p *game.Progress) error { p.Collection.ResidentGiftCount = 29; return nil })
	call("player_get_house_random_reward", 2, 4401)
	if read().Achievements[210502].Targets[210502] != 1 {
		t.Fatal("实库30次礼物成就未接线")
	}
	now = now.Add(14 * time.Hour)
	call("player_enter_house")
	update(func(p *game.Progress) error { p.Collection.ResidentGiftCount = 99; return nil })
	call("player_get_house_random_reward", 1, 4401)
	if read().Achievements[210503].Targets[210503] != 1 {
		t.Fatal("实库100次礼物成就未接线")
	}
	// 限时框走真实邮件发奖与领取入口，不在签发时起算。
	if e := svc.IssueMail(ctx, c, game.Mail{MID: 99001, Title: "限时头像框实库", Attachments: map[int]int64{80040: 2}}); e != nil {
		t.Fatal(e)
	}
	if _, owned := read().OwnedHeadBox[1041]; owned {
		t.Fatal("实库邮件签发提前起算头像框")
	}
	now = now.Add(time.Minute)
	call("receive_attachment", 51, 99001)
	p = read()
	stamp := float64(now.Unix())
	if p.OwnedHeadBox[1041] != stamp+3600 || p.HeadFrameGrantTime[1041] != stamp {
		t.Fatal("实库80040多份框没有按领取叠期")
	}
	call("change_head_box", 52, 1041)
	if e := svc.IssueMail(ctx, c, game.Mail{MID: 99002, Title: "限时头像框续期", Attachments: map[int]int64{80040: 1}}); e != nil {
		t.Fatal(e)
	}
	call("receive_attachment", 53, 99002)
	if read().OwnedHeadBox[1041] != stamp+7200 || read().SelectedHeadBoxID != 1041 {
		t.Fatal("实库续期未保留剩余期限与当前选择")
	}
	if e := svc.IssueMail(ctx, c, game.Mail{MID: 99003, Title: "限时头像框回拨测试", Attachments: map[int]int64{80040: 1}}); e != nil {
		t.Fatal(e)
	}
	before = read()
	now = now.Add(-time.Second)
	if _, e := svc.Handle(ctx, c, "receive_attachment", pgSocialArgs(54, 99003)); e == nil || !reflect.DeepEqual(before, read()) {
		t.Fatal("实库头像框回拨没有连邮件全回滚", e)
	}
	now = now.Add(4 * time.Hour)
	call("update_head_box_by_client")
	if read().SelectedHeadBoxID != 3 {
		t.Fatal("实库已选择过期框没有回默认")
	}
}
