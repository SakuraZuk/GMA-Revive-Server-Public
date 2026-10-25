package game

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestBackgroundOwnerFailureNeverFallsBackToPlayerSettlement(t *testing.T) {
	base, cs, fixture := socialTestWorld(t)
	store := &observedWriteStore{FixtureAccounts: fixture, FailCalendar: true}
	s := New(store, nil)
	s.Now = base.Now
	s.backgroundPeriodicStarted.Store(true)
	for i := 0; i < 4; i++ {
		if err := s.refreshPvpAwards(context.Background(), cs[0]); err != nil {
			t.Fatal("后台失败阻断了前台", err)
		}
	}
	if store.CalendarCalls != 0 {
		t.Fatal("前台接管了后台失败的全服结算", store.CalendarCalls)
	}
	if _, err := s.tryRefreshPvpAwards(context.Background(), nil); err == nil {
		t.Fatal("后台结算错误被吞掉")
	}
	if store.CalendarCalls != 1 {
		t.Fatal("后台未按原入口尝试结算")
	}
	if err := s.refreshPvpAwards(context.Background(), cs[0]); err != nil || store.CalendarCalls != 1 {
		t.Fatal("失败后前台再次接管", err)
	}
	store.FailCalendar = false
	if _, err := s.tryRefreshPvpAwards(context.Background(), nil); err != nil {
		t.Fatal("后台未恢复重试", err)
	}
}

func TestLoginFinishDoesNotScanAssistRoster(t *testing.T) {
	base, cs, fixtures := socialTestWorld(t)
	store := &observedWriteStore{FixtureAccounts: fixtures}
	s := New(store, nil)
	s.Now = base.Now
	c := cs[0]
	for i := range c.identity.Avatars {
		if c.identity.Avatars[i].Hostnum == c.hostnum {
			c.identity.Avatars[i].Progress.UnlockSystems["assist"] = 1
		}
	}
	c.reconnectAuth = json.RawMessage(`"登录收尾凭证"`)
	c.deviceID = "登录收尾设备"
	pushes, err := s.finishLogin(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if len(pushes) == 0 || pushes[len(pushes)-1].Method != "on_refresh_login" {
		t.Fatal("登录收尾未发送刷新", pushes)
	}
	if store.SocialReads != 0 {
		t.Fatalf("登录收尾读取了%d次同服助战候选", store.SocialReads)
	}
}

func TestBackgroundTickDoesNotQueueBehindAnotherPeriodicAward(t *testing.T) {
	s, cs, _ := socialTestWorld(t)
	value, _ := pvpAwardBuses.LoadOrStore(s, &pvpAwardBus{})
	bus := value.(*pvpAwardBus)
	bus.mu.Lock()
	locked := true
	defer func() {
		if locked {
			bus.mu.Unlock()
		}
	}()
	done := make(chan error, 1)
	go func() { _, err := s.Tick(context.Background(), cs[1]); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		bus.mu.Unlock()
		locked = false
		<-done
		t.Fatal("另一连接的结奖占住了本连接时钟锁")
	}
	if bus.key != "" {
		t.Fatal("跳过时钟错误推进周期键")
	}
	bus.mu.Unlock()
	locked = false
	ready, err := s.tryRefreshPvpAwards(context.Background(), cs[1])
	if err != nil || !ready || bus.key == "" {
		t.Fatal("解锁后没有按原事务继续结奖", ready, err)
	}
}

func TestPvpRequestDoesNotQueueBehindPeriodicAward(t *testing.T) {
	s, cs, _ := socialTestWorld(t)
	value, _ := pvpAwardBuses.LoadOrStore(s, &pvpAwardBus{})
	bus := value.(*pvpAwardBus)
	bus.mu.Lock()
	defer bus.mu.Unlock()

	done := make(chan error, 1)
	go func() { done <- s.refreshPvpAwards(context.Background(), cs[0]) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("PVP请求仍在排队等待全服结算")
	}
}

func TestExpiredBackgroundTickDoesNotStartPeriodicTransaction(t *testing.T) {
	s, cs, _ := socialTestWorld(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Tick(ctx, cs[0]); err != nil {
		t.Fatal(err)
	}
	if value, ok := pvpAwardBuses.Load(s); ok && value.(*pvpAwardBus).key != "" {
		t.Fatal("已过期时钟仍提交周期事务")
	}
}

func TestPlayerPathsNeverStartFullTableSettlement(t *testing.T) {
	base, cs, fixtures := socialTestWorld(t)
	store := &observedWriteStore{FixtureAccounts: fixtures}
	s := New(store, nil)
	s.Now = base.Now

	if _, err := s.Tick(context.Background(), cs[0]); err != nil {
		t.Fatal(err)
	}
	// 该方法过去命中通用活动前置钩子；即使业务参数无效，也不能先扫描全服。
	_, _ = s.Handle(context.Background(), cs[0], "query_rank_list", nil)

	login := NewConnection()
	login.phase = Authenticated
	login.hostnum = cs[1].hostnum
	login.identity = cs[1].identity
	if _, err := s.BecomePlayer(login); err != nil {
		t.Fatal(err)
	}
	if store.CalendarCalls != 0 {
		t.Fatalf("玩家Tick/请求/登录触发了%d次全表结算", store.CalendarCalls)
	}
}

func TestPeriodicMaintenanceOwnsFullTableSettlement(t *testing.T) {
	base, _, fixtures := socialTestWorld(t)
	store := &observedWriteStore{FixtureAccounts: fixtures}
	s := New(store, nil)
	s.Now = base.Now

	if err := s.RunPeriodicMaintenanceOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.CalendarCalls != 2 {
		t.Fatalf("后台单轮应依次执行活动和PVP结算，实际%d次", store.CalendarCalls)
	}
	if err := s.RunPeriodicMaintenanceOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.CalendarCalls != 2 {
		t.Fatalf("周期键未阻止同周期重复结算，实际%d次", store.CalendarCalls)
	}
	identity, err := store.Register(context.Background(), ClientInfo{Account: "后台结算后新角色", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	login := NewConnection()
	login.phase = Authenticated
	login.hostnum = 1
	login.identity = identity
	if _, err = s.BecomePlayer(login); err != nil {
		t.Fatal(err)
	}
	selected, ok := login.SelectedAvatar()
	if !ok || selected.Progress.Activities.Calendar == nil {
		t.Fatal("后台已就绪后，新角色未按单角色路径初始化活动日历")
	}
	if store.CalendarCalls != 2 {
		t.Fatalf("新角色登录退化成全表结算，实际%d次", store.CalendarCalls)
	}
}
