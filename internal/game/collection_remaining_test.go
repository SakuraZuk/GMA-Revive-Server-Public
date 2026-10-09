package game

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestRemainingFramesStackClaimTimePermanentChoiceAndRollback(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	now := time.Unix(1800000000, 500000000)
	p := Progress{}
	if err := grantProfileHeadBox(&p, 80040, 1041, 2, now); err != nil {
		t.Fatal(err)
	}
	stamp := float64(now.UnixNano()) / 1e9
	if p.OwnedHeadBox[1041] != stamp+3600 {
		t.Fatal("两份一小时框没有叠期", p.OwnedHeadBox)
	}
	p.SelectedHeadBoxID = 1041
	if err := grantProfileHeadBox(&p, 80008, 1010, 1, now); err != nil {
		t.Fatal(err)
	}
	ensureProfileCosmetics(&p, AvatarInfo{}, now)
	if p.SelectedHeadBoxID != 1041 {
		t.Fatal("奖励覆盖了当前选择")
	}
	now = now.Add(time.Hour)
	if err := grantProfileHeadBox(&p, 80040, 1041, 1, now); err != nil {
		t.Fatal(err)
	}
	if p.OwnedHeadBox[1041] != stamp+7200 {
		t.Fatal("未保留旧剩余期限")
	}
	before := CloneProgress(p)
	if err := grantProfileHeadBox(&p, 80040, 1041, 1, now.Add(-time.Second)); err == nil || !reflect.DeepEqual(before, p) {
		t.Fatal("框回拨没有原样拒绝", err)
	}
	p.OwnedHeadBox[1041] = 0
	if err := grantProfileHeadBox(&p, 80040, 1041, 1, now); err != nil || p.OwnedHeadBox[1041] != 0 {
		t.Fatal("永久拥有被有限期覆盖", err)
	}
	p.OwnedHeadBox[1041] = stamp
	ensureProfileCosmetics(&p, AvatarInfo{}, now.Add(time.Second))
	if p.SelectedHeadBoxID != 3 {
		t.Fatal("所选过期框未回默认")
	}
}

func TestRemainingCollectionNativeGraphEnergyRoomAndResidentFrozenReceipt(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	now := time.Date(2026, 10, 8, 6, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	s.Now = func() time.Time { return now }
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.UnlockSystems["house_daily_random_reward"] = 1
		p.UnlockSystems["house_dormitory3"] = 1
		if err := ensureCollection(p, now); err != nil {
			return err
		}
		r := newCollectionRoom(1, now)
		r.Slots = map[int]int{1: 4401}
		p.Collection.Rooms[1] = r
		p.Collection.Facilities[2] = CollectionFacility{ID: 2, Level: 1, ProduceStart: float64(now.Unix())}
		p.GuideTasks[6004] = GuideTask{ID: 6004, Status: 1}
		return ensureCollection(p, now)
	}); err != nil {
		t.Fatal(err)
	}
	call := func(method string, args ...any) []Push {
		t.Helper()
		out, err := s.Handle(ctx, c, method, socialArgs(args...))
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	p := c.SelectedAvatarUnsafe().Progress
	if _, owned := p.Collection.Rooms[3]; !owned {
		t.Fatal("已获系统104门槛没有房3ownership")
	}
	e := p.Collection.Rooms[1].Energy
	if e["value"] != float64(30) || e["start_flag"] != true || e["base_cost"] != float64(0) || e["per_value"] != float64(0) {
		t.Fatal("本版能量公式不一致", e)
	}
	call("gather_produce_material_speed_up", 2, 9000)
	p = c.SelectedAvatarUnsafe().Progress
	keep := p.Collection.Facilities[2].Keep
	if keep != math.Min(p.Collection.Facilities[2].Storage, 9000*p.Collection.Facilities[2].Rate) || p.Collection.TutorialAcceleration[6004].Seconds != 9000 {
		t.Fatal("原件教学未使用9000秒")
	}
	call("gather_produce_material_speed_up", 2, 9000)
	if c.SelectedAvatarUnsafe().Progress.Collection.Facilities[2].Keep != keep {
		t.Fatal("教学重试重复加速")
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if _, err := s.Handle(ctx, c, "gather_produce_material_speed_up", socialArgs(3, 900)); err == nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("任意生产加速未原样拒绝")
	}
	call("player_enter_house")
	p = c.SelectedAvatarUnsafe().Progress
	gift, exists := p.Collection.ResidentWindows[1][4401]
	if !exists || gift.Claimed || gift.Bonus == 0 || !gift.Frozen.RewardsFrozen || !gift.Frozen.hasAttachments() {
		t.Fatal("窗口未冻结真实住客随机箱", gift)
	}
	call("player_enter_house")
	if !reflect.DeepEqual(gift, c.SelectedAvatarUnsafe().Progress.Collection.ResidentWindows[1][4401]) {
		t.Fatal("反复进入重抽奖励")
	}
	// JSON冷存档往返保留窗口、box与一次收据。
	raw, err := json.Marshal(c.SelectedAvatarUnsafe().Progress)
	if err != nil {
		t.Fatal(err)
	}
	var cold Progress
	if err = json.Unmarshal(raw, &cold); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gift, cold.Collection.ResidentWindows[1][4401]) {
		t.Fatal("奖励JSON往返改变冻结结果")
	}
	call("player_get_house_random_reward", 1, 4401)
	p = c.SelectedAvatarUnsafe().Progress
	if !p.Collection.ResidentWindows[1][4401].Claimed || p.Collection.ResidentGiftCount != 1 || p.Achievements[210501].Targets[210501] != 1 {
		t.Fatal("住客真实领取未接成就")
	}
	before = CloneProgress(p)
	if _, err = s.Handle(ctx, c, "player_get_house_random_reward", socialArgs(1, 4401)); err == nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("住客重试重复奖励或写入")
	}
	now = now.Add(10 * time.Hour)
	call("player_enter_house")
	if _, exists = c.SelectedAvatarUnsafe().Progress.Collection.ResidentWindows[2][4401]; !exists {
		t.Fatal("下午窗口未独立抽选")
	}
	if err = s.updateProgress(ctx, c, func(p *Progress) error { p.Collection.ResidentGiftCount = 29; return nil }); err != nil {
		t.Fatal(err)
	}
	call("player_get_house_random_reward", 2, 4401)
	if c.SelectedAvatarUnsafe().Progress.Achievements[210502].Targets[210502] != 1 {
		t.Fatal("30次事件成就未接线")
	}
	now = now.Add(14 * time.Hour)
	call("player_enter_house")
	if err = s.updateProgress(ctx, c, func(p *Progress) error { p.Collection.ResidentGiftCount = 99; return nil }); err != nil {
		t.Fatal(err)
	}
	call("player_get_house_random_reward", 1, 4401)
	if c.SelectedAvatarUnsafe().Progress.Achievements[210503].Targets[210503] != 1 {
		t.Fatal("100次事件成就未接线")
	}
}

func TestRemainingCollectionEnergySettlesOldParametersBeforeLeavingAndNoReadRefill(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	now := time.Unix(1800000000, 0)
	p := Progress{}
	if err := ensureCollection(&p, now); err != nil {
		t.Fatal(err)
	}
	r := newCollectionRoom(1, now)
	r.Slots = map[int]int{1: 4401}
	p.Collection.Rooms[1] = r
	p.Collection.Facilities[2] = CollectionFacility{ID: 2, Level: 1, ProduceStart: float64(now.Unix())}
	if err := ensureCollection(&p, now); err != nil {
		t.Fatal(err)
	}
	r = p.Collection.Rooms[1]
	r.Energy["per_value"] = float64(2)
	p.Collection.Rooms[1] = r
	if err := collectionRefreshEnergy(&p, now.Add(20*time.Second)); err != nil {
		t.Fatal(err)
	}
	r = p.Collection.Rooms[1]
	if r.Energy["value"] != float64(0) || r.Energy["start_flag"] != false {
		t.Fatal("旧速率耗尽没有关闭", r.Energy)
	}
	if err := collectionRefreshEnergy(&p, now.Add(30*time.Second)); err != nil || p.Collection.Rooms[1].Energy["value"] != float64(0) {
		t.Fatal("能量读取免费重置", err)
	}
	r = p.Collection.Rooms[1]
	r.Slots = map[int]int{}
	p.Collection.Rooms[1] = r
	if err := collectionRefreshEnergy(&p, now.Add(40*time.Second)); err != nil {
		t.Fatal(err)
	}
	r = p.Collection.Rooms[1]
	r.Slots = map[int]int{1: 4401}
	p.Collection.Rooms[1] = r
	if err := collectionRefreshEnergy(&p, now.Add(50*time.Second)); err != nil || p.Collection.Rooms[1].Energy["value"] != float64(30) {
		t.Fatal("空房入住没有按人数重启", err)
	}
	before := CloneProgress(p)
	if err := collectionRefreshEnergy(&p, now.Add(49*time.Second)); err == nil || !reflect.DeepEqual(before, p) {
		t.Fatal("能量回拨未拒绝", err)
	}
}
