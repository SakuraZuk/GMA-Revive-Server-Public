package game

import (
	"context"
	"encoding/json"
	"hs-server/internal/mobileproto"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestCollectionEnergyUsesOwnedRoomAndServerClock(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	now := time.Unix(1800000000, 0)
	s.Now = func() time.Time { return now }
	if e := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureCollection(p, now); e != nil {
			return e
		}
		p.Collection.Rooms[1] = newCollectionRoom(1, now)
		return ensureCollection(p, now)
	}); e != nil {
		t.Fatal(e)
	}
	materials := c.SelectedAvatarUnsafe().Progress.Materials
	now = now.Add(10 * time.Second)
	pushes, e := s.collectionEnergyRPC(ctx, c, []json.RawMessage{json.RawMessage(`1`)})
	if e != nil || pushes[len(pushes)-1].Method != "on_last_get_time_update" || pushes[len(pushes)-1].Args[0] != 1 || c.SelectedAvatarUnsafe().Progress.Collection.Rooms[1].Energy["last_get_time"] != float64(now.Unix()) || !reflect.DeepEqual(materials, c.SelectedAvatarUnsafe().Progress.Materials) {
		t.Fatal("能量时刻未使用真实房间与服务器时间", e)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if _, e = s.collectionEnergyRPC(ctx, c, []json.RawMessage{json.RawMessage(`3`)}); e == nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("未解锁房间能量仍可同步", e)
	}
	now = now.Add(-20 * time.Second)
	if _, e = s.collectionEnergyRPC(ctx, c, []json.RawMessage{json.RawMessage(`1`)}); e == nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("时间回拨覆盖能量存档", e)
	}
}

func TestRemainingCollectionEmptyRoomEnergyRPCPersistsAndProjectsReadTime(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	now := time.Unix(1800000000, 500000000)
	s.Now = func() time.Time { return now }
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if err := ensureCollection(p, now); err != nil {
			return err
		}
		p.Collection.Rooms[1] = newCollectionRoom(1, now)
		return ensureCollection(p, now)
	}); err != nil {
		t.Fatal(err)
	}
	materials := CloneProgress(c.SelectedAvatarUnsafe().Progress).Materials
	for i := 0; i < 2; i++ {
		now = now.Add(10 * time.Second)
		pushes, err := s.Handle(ctx, c, "update_last_get_time", socialArgs(1))
		if err != nil || len(pushes) == 0 {
			t.Fatal("空房能量原生RPC失败", err)
		}
		last := pushes[len(pushes)-1]
		if last.Method != "on_last_get_time_update" || !reflect.DeepEqual(last.Args, []any{1}) {
			t.Fatal("原生能量回调顺序或房间编号错误", last)
		}
		stamp := float64(now.UnixNano()) / 1e9
		p := c.SelectedAvatarUnsafe().Progress
		energy := p.Collection.Rooms[1].Energy
		if energy["last_get_time"] != stamp || energy["start_flag"] != false || energy["value"] != float64(0) || !reflect.DeepEqual(materials, p.Materials) {
			t.Fatal("空房读取没有保存时刻或错误补能/发材料", energy)
		}
		var projected map[string]any
		for _, item := range pushes {
			if item.Method != "client_prop_changed" || len(item.Args) != 1 {
				continue
			}
			field := item.Args[0].([]any)
			if field[0] != "restroom_info" {
				continue
			}
			for _, pair := range field[1].(mobileproto.Map) {
				if pair.Key == int64(1) {
					projected = pair.Value.(map[string]any)["dormitory_energy"].(map[string]any)
				}
			}
		}
		if projected == nil || projected["last_get_time"] != stamp || projected["start_flag"] != false || projected["value"] != float64(0) {
			t.Fatal("属性投影清除了空房读取时刻", projected)
		}
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	identity, err := a.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := json.Marshal(identity.Avatars[0].Progress)
	if err != nil {
		t.Fatal(err)
	}
	var cold Progress
	if err = json.Unmarshal(stored, &cold); err != nil {
		t.Fatal(err)
	}
	if err = ensureCollection(&cold, now); err != nil || !reflect.DeepEqual(cold.Collection.Rooms[1].Energy, before.Collection.Rooms[1].Energy) {
		t.Fatal("空房读取时刻冷重建后丢失", err, cold.Collection.Rooms[1].Energy)
	}
	if _, err := s.Handle(ctx, c, "update_last_get_time", socialArgs(3)); err == nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("未解锁房间RPC未原子拒绝", err)
	}
	now = now.Add(-20 * time.Second)
	if _, err := s.Handle(ctx, c, "update_last_get_time", socialArgs(1)); err == nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("时钟回拨RPC未原子拒绝", err)
	}
	now = now.Add(20 * time.Second)
	for _, key := range []string{"last_get_time", "last_update_time"} {
		for _, invalid := range []any{float64(now.Unix() + 100), -1.0, math.NaN(), math.Inf(1), "错误时刻"} {
			p := CloneProgress(before)
			p.Collection.Rooms[1].Energy[key] = invalid
			if err := collectionRefreshEnergy(&p, now); err == nil {
				t.Fatal("空房初始化掩盖了非法已有时刻", key, invalid)
			}
		}
	}
}
