package dbstore

import (
	"context"
	"encoding/hex"
	"errors"
	"hs-server/internal/game"
	"sync"
	"testing"
	"time"
)

func TestPostgresHumanPresenceLeasesAcrossStoresAndExpiry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, err := s.Register(ctx, game.ClientInfo{Account: "租约甲", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Register(ctx, game.ClientInfo{Account: "租约乙", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	tokens := []string{"123456789012345678901234", "234567890123456789012345"}
	other := New(s.pool)
	for _, token := range tokens {
		if err := s.RefreshHumanPresence(ctx, a.Avatars[0].OID, token, 120); err != nil {
			t.Fatal(err)
		}
	}
	if err := other.RefreshHumanPresence(ctx, b.Avatars[0].OID, tokens[0], 130); err == nil {
		t.Fatal("租约token转移角色")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := other.RefreshHumanPresence(ctx, a.Avatars[0].OID, tokens[0], 140); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if err := s.RemoveHumanPresence(ctx, tokens[1]); err != nil {
		t.Fatal(err)
	}
	ids, err := other.HumanOnlineIDs(ctx, 121)
	if err != nil || len(ids) != 1 || ids[0] != hex.EncodeToString(a.Avatars[0].OID) {
		t.Fatal("多连接只删一个租约却离线", err, ids)
	}
	ids, err = New(s.pool).HumanOnlineIDs(ctx, 140)
	if err != nil || len(ids) != 0 {
		t.Fatal("进程崩溃租约没有自动失效", err, ids)
	}
}

func TestPostgresHumanCrossProcessDurableDeliveryAndConcurrentBarrier(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	services := []*game.Service{pgSocialService(t, store), pgSocialService(t, New(store.pool))}
	cs := []*game.Connection{}
	avatars := []game.Avatar{}
	for i, account := range []string{"跨进程真人甲", "跨进程真人乙"} {
		info := game.ClientInfo{Account: account, Password: "pw", Hostnum: 1}
		identity, err := store.Register(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		avatars = append(avatars, identity.Avatars[0])
		services[i].Now = func() time.Time { return now }
		c := pgSocialLogin(t, services[i], info, identity.Avatars[0].OID)
		cs = append(cs, c)
		defer services[i].Detach(c)
	}
	ids := []string{hex.EncodeToString(avatars[0].OID), hex.EncodeToString(avatars[1].OID)}
	pgSocialCall(t, services[0], cs[0], "apply_friend", 1, ids[1], "真实跨进程", 1, 0)
	pgSocialCall(t, services[1], cs[1], "agree_apply_friend", 2, ids[0])
	for i, c := range cs {
		pgSocialCall(t, services[i], c, "start_sync_pvp_match", 3)
	}
	now = now.Add(2 * time.Second)
	selection := []game.Push{}
	for i, c := range cs {
		selection = append(selection, pgHumanTick(t, services[i], c)...)
	}
	r := pgHumanRead(t, store, avatars[0].OID).Progress.Social.HumanRoom
	peerRoom := pgHumanRead(t, store, avatars[1].OID).Progress.Social.HumanRoom
	if r == nil || peerRoom == nil || r.UUID != peerRoom.UUID || r.Seed != peerRoom.Seed {
		t.Fatal("独立Service未跨PG租约同房匹配")
	}
	selectCount := 0
	for _, p := range selection {
		if p.Method == "select_pvp_cards" {
			selectCount++
		}
	}
	if selectCount != 2 {
		t.Fatal("共同select未跨服务投递", selectCount)
	}
	tickBoth := func() {
		for i, c := range cs {
			pgHumanTick(t, services[i], c)
		}
	}
	for i, c := range cs {
		pgSocialCall(t, services[i], c, "send_pvp_cards", 0)
		tickBoth()
	}
	for i, c := range cs {
		pgSocialCall(t, services[i], c, "pvp_load_complete")
		tickBoth()
	}
	r = pgHumanRead(t, store, avatars[0].OID).Progress.Social.HumanRoom
	for i, c := range cs {
		layout := r.Players[ids[i]].Layout
		pgSocialCall(t, services[i], c, "battle_fighting", map[string]any{"fighting_cards": layout.Fighting, "support_cards": layout.Support})
		tickBoth()
	}
	for i, c := range cs {
		pgHumanEvent(t, services[i], c, r.UUID, 1, "ready", map[string]any{"version": 1})
		pgHumanEvent(t, services[i], c, r.UUID, 2, "started", map[string]any{"units": pgHumanUnits(r)})
	}
	frame := map[string]any{"action": 1, "eid": "1", "master": r.IDs[0], "units": pgHumanUnits(r), "command_index": 0}
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, c := range cs {
		wg.Add(1)
		go func(i int, c *game.Connection) {
			defer wg.Done()
			_, err := services[i].Handle(ctx, c, "do_command", pgSocialArgs("__battle_event__", []any{map[string]any{"battle_uuid": r.UUID, "sequence": 3, "kind": "human_input", "data": frame}}))
			if err != nil {
				errs <- err
			}
		}(i, c)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal("双服务屏障死锁或失败", err)
	}
	owner := 0
	if ids[0] != r.IDs[0] {
		owner = 1
	}
	pgSocialCall(t, services[owner], cs[owner], "do_command", "move_to", []any{"1", []int{1, -1, 0}})
	for i, c := range cs {
		pushes := pgHumanTick(t, services[i], c)
		commands := 0
		for _, p := range pushes {
			if p.Method == "sync_battle_method" && p.Args[0] == "revival_do_shared_command" {
				commands++
			}
		}
		if commands != 1 {
			t.Fatal("共同命令未从PG唯一投递", i, commands)
		}
	}
	before := pgHumanRead(t, store, avatars[0].OID).Progress.Social.HumanRoom
	_, err := store.UpdateSocial(ctx, ids, func(v map[string]*game.Avatar) error {
		for _, id := range ids {
			v[id].Progress.Social.HumanRoom.Deliveries = append(v[id].Progress.Social.HumanRoom.Deliveries, game.HumanDelivery{Sequence: 999, Target: id, Kind: "abort"})
		}
		return errors.New("PG动作故障注入")
	})
	if err == nil {
		t.Fatal("故障没有回滚")
	}
	after := pgHumanRead(t, New(store.pool), avatars[1].OID).Progress.Social.HumanRoom
	if len(before.Deliveries) != len(after.Deliveries) || len(after.Commands) != 1 {
		t.Fatal("PG动作部分提交")
	}
	for i, c := range cs {
		final := map[string]any{"action": 2, "winner_eids": []string{r.IDs[0]}, "units": pgHumanUnits(r), "command_index": 1}
		pgHumanEvent(t, services[i], c, r.UUID, 4, "human_result", final)
	}
	for i, c := range cs {
		pushes := pgHumanTick(t, services[i], c)
		found := false
		for _, p := range pushes {
			found = found || p.Method == "battle_result"
		}
		if !found {
			t.Fatal("共同result未跨服务投递")
		}
		av := pgHumanRead(t, store, avatars[i].OID)
		if av.Progress.Social.HumanRoom.Status != "settled" || len(av.Progress.SyncPvpRecords) != 1 {
			t.Fatal("两进程结果未同事务提交")
		}
	}
}
