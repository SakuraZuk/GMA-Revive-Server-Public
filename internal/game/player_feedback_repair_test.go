package game

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

type observedWriteStore struct {
	*FixtureAccounts
	Writes         int
	SocialReads    int
	FailNext       bool
	FailCalendar   bool
	CalendarCalls  int
	FailSnapshot   bool
	FailSocialRead bool
}

func (a *observedWriteStore) SocialAvatars(ctx context.Context, search SocialSearch) ([]Avatar, error) {
	a.SocialReads++
	if a.FailSocialRead {
		return nil, context.DeadlineExceeded
	}
	return a.FixtureAccounts.SocialAvatars(ctx, search)
}

func TestAsyncPvpSavedCandidatesAvoidGlobalRead(t *testing.T) {
	s, cs, base := socialTestWorld(t)
	ctx := context.Background()
	store := &observedWriteStore{FixtureAccounts: base}
	s.Accounts = store
	if _, err := s.asyncPvpRPC(ctx, cs[0], "enter_asyn_pvp", nil); err != nil {
		t.Fatal(err)
	}
	if store.SocialReads != 1 {
		t.Fatal("首次生成必须读取真实对手", store.SocialReads)
	}
	before, _ := json.Marshal(socialSaved(t, base, cs[0]).Progress.AsyncPvp.Candidates)
	store.FailSocialRead = true
	store.SocialReads = 0
	if _, err := s.asyncPvpRPC(ctx, cs[0], "enter_asyn_pvp", nil); err != nil {
		t.Fatal("已有对手仍依赖全服读取", err)
	}
	if store.SocialReads != 0 {
		t.Fatal("重进异步竞技扫描了全服")
	}
	if _, err := s.asyncPvpRPC(ctx, cs[0], "refresh_asyn_pvp", nil); err == nil {
		t.Fatal("未到刷新时间被放行")
	}
	if store.SocialReads != 0 {
		t.Fatal("刷新门槛拒绝前扫描了全服")
	}
	after, _ := json.Marshal(socialSaved(t, base, cs[0]).Progress.AsyncPvp.Candidates)
	if string(before) != string(after) {
		t.Fatal("重进或未到期刷新改变了候选")
	}
	// 连接仍有旧候选，但存档已清空；必须以锁内存档为准重新获取，读取失败不补机器人。
	if _, err := base.UpdateProgress(ctx, selectedOID(cs[0]), func(p *Progress) error { p.AsyncPvp.Candidates = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.asyncPvpRPC(ctx, cs[0], "enter_asyn_pvp", nil); err == nil {
		t.Fatal("候选查询失败仍返回了旧列表")
	}
	if store.SocialReads != 1 || len(socialSaved(t, base, cs[0]).Progress.AsyncPvp.Candidates) != 0 {
		t.Fatal("失败读取改变了空候选状态")
	}
	store.FailSocialRead = false
	if _, err := s.asyncPvpRPC(ctx, cs[0], "enter_asyn_pvp", nil); err != nil {
		t.Fatal(err)
	}
	if store.SocialReads != 2 || len(socialSaved(t, base, cs[0]).Progress.AsyncPvp.Candidates) == 0 {
		t.Fatal("恢复读取后未生成新候选")
	}
}

func (a *observedWriteStore) HumanAvatarSnapshot(ctx context.Context, oid []byte, since int64) (Avatar, int64, bool, error) {
	if a.FailSnapshot {
		return Avatar{}, 0, false, context.DeadlineExceeded
	}
	av, err := a.FixtureAccounts.AdminPlayer(ctx, oid)
	return av, 1, true, err
}

func (a *observedWriteStore) UpdateAllSocial(ctx context.Context, fn func(map[string]*Avatar) error) ([]Avatar, error) {
	a.CalendarCalls++
	if a.FailCalendar {
		return nil, context.DeadlineExceeded
	}
	return a.FixtureAccounts.UpdateAllSocial(ctx, fn)
}

func (a *observedWriteStore) UpdateProgress(ctx context.Context, oid []byte, fn func(*Progress) error) (Progress, error) {
	a.Writes++
	if a.FailNext {
		a.FailNext = false
		return Progress{}, context.DeadlineExceeded
	}
	return a.FixtureAccounts.UpdateProgress(ctx, oid, fn)
}

func TestOrdinaryBattleThreeWavesDoNotWriteEveryEventOrRestartOnSlowHeartbeat(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	svc := New(accounts, nil)
	now := time.Date(2026, 10, 9, 2, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	if err := SetActivitySchedules(DefaultPermanentActivitySchedules(now.Add(-24 * time.Hour))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = SetActivitySchedules(nil) })
	c, av := newBattleConnection(t, ctx, accounts, svc)
	if err := svc.updateProgress(ctx, c, func(p *Progress) error {
		p.ClearedDungeons = append(p.ClearedDungeons, 509, 4101)
		for id, task := range p.GuideTasks {
			task.Status = 2
			p.GuideTasks[id] = task
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Handle(ctx, c, "enter_dungeon", socialArgs(1, 4102, map[string]any{})); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Handle(ctx, c, "load_entity_finish", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Handle(ctx, c, "battle_fighting", socialArgs(map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})); err != nil {
		t.Fatal(err)
	}
	uuid := c.SelectedAvatarUnsafe().Progress.Battle.UUID
	store := &observedWriteStore{FixtureAccounts: accounts, FailSnapshot: true}
	svc.Accounts = store
	observeBattleEvent(t, ctx, svc, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":1,"kind":"ready","data":{"version":1}}`, uuid))
	store.FailNext = true // 开局检查点超时不能丢失握手、永久跳号。
	observeBattleEvent(t, ctx, svc, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":2,"kind":"started","data":{"units":[]}}`, uuid))
	writes := store.Writes
	calendarCalls := store.CalendarCalls
	store.FailCalendar = true      // 全服结奖超时不能阻断普通观察流。
	activityAwardBuses.Delete(svc) // 模拟需要结算的全服周期，不让缓存掩盖阻塞。
	for seq := 3; seq <= 302; seq++ {
		observeBattleEvent(t, ctx, svc, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":%d,"kind":"round_end","data":{"units":[{"eid":"wave%d","role":202,"hex":[0,0,0]}]}}`, uuid, seq, (seq-3)/100+1))
	}
	if store.Writes != writes || store.CalendarCalls != calendarCalls || c.ordinaryObservation.LastSequence != 302 {
		t.Fatal("观察事件仍逐条落库或超时后出现跳号", store.Writes, writes)
	}
	store.FailCalendar = false
	store.FailSnapshot = false
	out, err := svc.Handle(ctx, c, "client_need_recover_battle", nil)
	if err != nil || len(out) != 0 || c.SelectedAvatarUnsafe().Progress.Battle.UUID != uuid {
		t.Fatal("同连接慢心跳重开第三波", err, out)
	}
	store.FailNext = true
	result := fmt.Sprintf(`{"battle_uuid":%q,"sequence":303,"kind":"result","data":{"winner_eids":["%x"]}}`, uuid, av.OID)
	if out := observeBattleEvent(t, ctx, svc, c, result); battleTestResult(out) != nil || c.pendingOrdinaryResult == nil {
		t.Fatal("结算超时没有保留重试")
	}
	now = now.Add(2 * time.Second)
	out, err = svc.Tick(ctx, c)
	if err != nil || battleTestResult(out) == nil || !c.SelectedAvatarUnsafe().Progress.Battle.Finished {
		t.Fatal("结算超时重试没有发出原生结果", err, out)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	observeBattleEvent(t, ctx, svc, c, result)
	if c.SelectedAvatarUnsafe().Progress.Materials[100].Count != before.Materials[100].Count {
		t.Fatal("重复结算重复发升格材料")
	}
}

func TestCardReturnNativeMaterialsAtomicReplayAndUnlockRunes(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	svc := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, svc)
	card := newCard(2401, 2, time.Now())
	card.EnhanceCount = 2
	if err := svc.updateProgress(ctx, c, func(p *Progress) error {
		p.Cards = append(p.Cards, card)
		p.Runes = map[string]Rune{"00112233445566778899aabb": {UUID: "00112233445566778899aabb", CardUUID: card.UUID}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if _, err := svc.Handle(ctx, c, "decompose_cards", socialArgs(7, []string{card.UUID, card.UUID})); err != nil {
		t.Fatal(err)
	}
	if len(c.SelectedAvatarUnsafe().Progress.Cards) != len(before.Cards) {
		t.Fatal("重复UUID半删卡")
	}
	out, err := svc.Handle(ctx, c, "decompose_cards", socialArgs(8, []string{card.UUID}))
	after := c.SelectedAvatarUnsafe().Progress
	if err != nil || len(after.Cards) != len(before.Cards)-1 || after.Materials[100].Count-before.Materials[100].Count != 10 || after.Materials[101].Count-before.Materials[101].Count != 10 || after.Runes["00112233445566778899aabb"].CardUUID != "" {
		t.Fatal("原生固定归还材料、卸印或删卡错误", err, out)
	}
	if after.Materials[4].Count-before.Materials[4].Count != int64(float64(androidCardReturn.Levels[1].Exp)*androidCardReturn.Levels[1].Factor) {
		t.Fatal("原生经验返还公式不符")
	}
	if _, err := svc.Handle(ctx, c, "decompose_cards", socialArgs(9, []string{card.UUID})); err != nil {
		t.Fatal(err)
	}
	if c.SelectedAvatarUnsafe().Progress.Materials[100].Count != after.Materials[100].Count {
		t.Fatal("归还重放重复发奖")
	}
}

func TestReportedDrawPoolsAndFailureCallbacks(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	svc := New(accounts, nil)
	svc.Now = func() time.Time { return time.Date(2026, 10, 9, 2, 0, 0, 0, time.UTC) }
	if err := SetActivitySchedules(DefaultPermanentActivitySchedules(svc.Now().Add(-24 * time.Hour))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = SetActivitySchedules(nil) })
	c, _ := newBattleConnection(t, ctx, accounts, svc)
	if err := svc.updateProgress(ctx, c, func(p *Progress) error {
		p.AvatarLevel = 60
		p.Materials[500] = Material{ID: 500, Count: 20, Total: 20}
		p.Materials[501] = Material{ID: 501, Count: 20, Total: 20}
		for id, task := range p.GuideTasks {
			task.Status = 2
			p.GuideTasks[id] = task
		}
		p.ClearedDungeons = append(p.ClearedDungeons, 510)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, pool := range []int{2, 109, 122, 501, 601} {
		out, err := svc.Handle(ctx, c, "random_cards", socialArgs(7, pool, 1))
		if err != nil {
			t.Fatal(pool, err)
		}
		last := out[len(out)-1]
		values := last.Args[1].([]any)
		if values[0] != 0 {
			t.Fatal("玩家上报卡池仍不可抽", pool, values[0])
		}
		if c.SelectedAvatarUnsafe().Progress.RandomCardsRecord[pool].RandomCount != 1 {
			t.Fatal("抽卡计数未保存", pool)
		}
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	for _, pool := range []int{101, 9999, 123456} {
		out, err := svc.Handle(ctx, c, "random_cards", socialArgs(8, pool, 1))
		if err != nil || out[len(out)-1].Method != "call_client_callback" {
			t.Fatal("抽卡拒绝没有原生回调", err, out)
		}
	}
	if c.SelectedAvatarUnsafe().Progress.Materials[501].Count != before.Materials[501].Count {
		t.Fatal("未配置卡池扣料")
	}
	_, _ = json.Marshal(before)
}
