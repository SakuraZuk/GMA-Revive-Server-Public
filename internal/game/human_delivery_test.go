package game

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestHumanPvpDurableDeliveryAcrossServicesRollbackAndBoundedLoad(t *testing.T) {
	s, cs, store, r := humanTestPair(t, false)
	other := New(store, nil)
	other.Now = s.Now
	services := map[*Connection]*Service{cs[0]: s, cs[1]: other}
	owner, peer := cs[0], cs[1]
	if hexOf(selectedOID(owner)) != r.IDs[0] {
		owner, peer = peer, owner
	}
	ctx := context.Background()
	for step := 0; step < 128; step++ {
		frame := map[string]any{"action": step + 1, "eid": "1", "master": r.IDs[0], "units": humanTestUnits(r), "command_index": step}
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, c := range cs {
			wg.Add(1)
			go func(c *Connection) {
				defer wg.Done()
				_, _, err := services[c].absorbHumanBattleEvent(ctx, c, humanTestEnvelope(c, int64(step+3), "human_input", frame))
				if err != nil {
					errs <- err
				}
			}(c)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal("双服务输入行锁失败", err)
		}
		if _, _, err := services[owner].humanDoCommand(ctx, owner, "move_to", []any{"1", []any{1, -1, 0}}); err != nil {
			t.Fatal(err)
		}
		for _, c := range []*Connection{owner, peer} {
			pushes := services[c].flushHumanPvp(c)
			if len(pushes) != 1 || pushes[0].Args[0] != "revival_do_shared_command" {
				t.Fatal("独立Service没有从持久房间投递", step, pushes)
			}
			if len(services[c].flushHumanPvp(c)) != 0 {
				t.Fatal("同连接游标重复投递")
			}
		}
	}
	before := socialSaved(t, store, owner).Progress.Social.HumanRoom
	_, err := store.UpdateSocial(ctx, r.IDs, func(v map[string]*Avatar) error {
		for _, id := range r.IDs {
			room := v[id].Progress.Social.HumanRoom
			appendHumanDelivery(room, id, "abort", 0, false)
		}
		return errors.New("持久动作事务故障注入")
	})
	if err == nil {
		t.Fatal("故障注入没有回滚")
	}
	after := socialSaved(t, store, peer).Progress.Social.HumanRoom
	if len(before.Deliveries) != len(after.Deliveries) || len(after.Commands) != 128 || after.UUID != r.UUID || after.Seed != r.Seed {
		t.Fatal("跨服务日志或动作未保持原UUID/seed")
	}
	if len(other.flushHumanPvp(peer)) != 0 {
		t.Fatal("回滚动作被投递")
	}
}

type testHumanLease struct {
	id    string
	until int64
}
type humanLeaseFixture struct {
	*FixtureAccounts
	leaseMu sync.Mutex
	leases  map[string]testHumanLease
}

func (a *humanLeaseFixture) RefreshHumanPresence(ctx context.Context, oid []byte, token string, until int64) error {
	a.leaseMu.Lock()
	defer a.leaseMu.Unlock()
	a.leases[token] = testHumanLease{hexOf(oid), until}
	return ctx.Err()
}
func (a *humanLeaseFixture) RemoveHumanPresence(ctx context.Context, token string) error {
	a.leaseMu.Lock()
	defer a.leaseMu.Unlock()
	delete(a.leases, token)
	return ctx.Err()
}
func (a *humanLeaseFixture) HumanOnlineIDs(ctx context.Context, now int64) ([]string, error) {
	a.leaseMu.Lock()
	defer a.leaseMu.Unlock()
	out := []string{}
	for _, lease := range a.leases {
		if lease.until > now {
			out = append(out, lease.id)
		}
	}
	return out, ctx.Err()
}
func TestHumanPvpCrossServicePresenceMatchingAndLeaseExpiry(t *testing.T) {
	base, all, store := socialTestWorld(t)
	leased := &humanLeaseFixture{FixtureAccounts: store, leases: map[string]testHumanLease{}}
	services := []*Service{New(leased, nil), New(leased, nil)}
	cs := all[:2]
	now := base.Now()
	for i, s := range services {
		s.Now = func() time.Time { return now }
		s.attachPlayer(cs[i])
		if err := s.refreshHumanPresence(context.Background(), cs[i]); err != nil {
			t.Fatal(err)
		}
		if _, err := s.startSyncPVP(context.Background(), cs[i], socialArgs(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	now = now.Add(2 * time.Second)
	if _, err := services[0].tickHumanMatch(context.Background(), cs[0]); err != nil {
		t.Fatal(err)
	}
	a, b := socialSaved(t, store, cs[0]), socialSaved(t, store, cs[1])
	if a.Progress.Social.HumanRoom == nil || b.Progress.Social.HumanRoom == nil || a.Progress.Social.HumanRoom.UUID != b.Progress.Social.HumanRoom.UUID {
		t.Fatal("两独立游戏Service跨进程租约没有同房匹配")
	}
	services[0].onlineMu.Lock()
	delete(services[0].online, hexOf(selectedOID(cs[0])))
	services[0].onlineMu.Unlock()
	services[1].onlineMu.Lock()
	delete(services[1].online, hexOf(selectedOID(cs[1])))
	services[1].onlineMu.Unlock()
	now = now.Add(21 * time.Second)
	ids, err := services[0].humanOnlineIDs(context.Background())
	if err != nil || len(ids) != 0 {
		t.Fatal(fmt.Sprintf("崩溃租约未过期: %v", ids), err)
	}
}
