package dbstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"hs-server/internal/game"
	"hs-server/internal/mobileproto"
)

type pgActivityHouseBoundaryEntropy struct{ data []byte }

func (r *pgActivityHouseBoundaryEntropy) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
		if len(r.data) > 0 {
			p[i], r.data = r.data[0], r.data[1:]
		}
	}
	return len(p), nil
}

func TestPostgresActivityHouseFacilitySSRActualRewardReloadAndRollback(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	svc := pgActivityService(t, store, func() time.Time { return now })
	info := game.ClientInfo{Account: "设施入住典藏真实", Password: "pw", Hostnum: 10001}
	identity, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	oid := identity.Avatars[0].OID
	if _, err = store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		p.Collection = &game.CollectionState{
			Rooms:      map[int]game.CollectionRoom{1: {ID: 1, Cards: map[int]int{4401: 1}, Slots: map[int]int{1: 4401}, LastExp: float64(now.Unix()), Velocity: 1, Limit: 1000}},
			Facilities: map[int]game.CollectionFacility{6: {ID: 6, Level: 1}},
		}
		p.Activities.House = &game.HouseFrageState{ID: 1, Pos: 1, Sites: map[int]*game.HouseFrageSite{1: {ID: 1}, 2: {ID: 2, Card: 2404}}}
		p.Materials[402] = game.Material{ID: 402, Count: 1, Total: 1}
		p.Materials[2404] = game.Material{ID: 2404}
		p.Materials[20] = game.Material{ID: 20}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	c := pgActivityLogin(t, svc, info)
	move := func() []any {
		t.Helper()
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(0.402*float64(1<<53)))
		previous := rand.Reader
		rand.Reader = &pgActivityHouseBoundaryEntropy{data: append([]byte{}, b[1:]...)}
		defer func() { rand.Reader = previous }()
		out, e := svc.Handle(ctx, c, "house_frage_ctrl_move", pgSocialArgs(1, 1))
		if e != nil {
			t.Fatal(e)
		}
		return out[len(out)-1].Args[1].([]any)
	}
	reply := move()
	rewards, ok := reply[3].(mobileproto.Map)
	if !ok || len(rewards) != 1 || rewards[0].Key != int64(2404) || rewards[0].Value != int64(1) {
		t.Fatal("真实库典藏奖励未按原生字典回调", reply)
	}
	p := pgHumanRead(t, New(store.pool), oid).Progress
	if p.Materials[402].Count != 0 || p.Materials[2404].Count != 1 || p.Materials[20].Count != 0 || p.Activities.House.SSR != 1 {
		t.Fatal("真实库入住概率边界未发典藏残页或扣骰子部分提交", p.Materials[2404], p.Activities.House)
	}
	// 将起点恢复到未完成棋盘，再用库存不足验证整个事务失败。
	if _, err = store.UpdateProgress(ctx, oid, func(p *game.Progress) error { p.Activities.House.Pos = 1; return nil }); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(pgHumanRead(t, store, oid).Progress)
	reply = move()
	rewards, ok = reply[3].(mobileproto.Map)
	if !ok || len(rewards) != 0 {
		t.Fatal("真实库失败回调奖励不是空字典", reply)
	}
	after, _ := json.Marshal(pgHumanRead(t, store, oid).Progress)
	if !bytes.Equal(before, after) {
		t.Fatal("真实库不足骰子失败仍推进棋盘或发入住奖励")
	}
	if _, err = store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		p.Materials[402] = game.Material{ID: 402, Count: 1, Total: 2}
		room := p.Collection.Rooms[1]
		room.Cards, room.Slots = map[int]int{}, map[int]int{}
		p.Collection.Rooms[1] = room
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	move()
	p = pgHumanRead(t, New(store.pool), oid).Progress
	if p.Materials[2404].Count != 1 || p.Materials[20].Count != 20 || p.Activities.House.SSR != 1 {
		t.Fatal("真实库离住后仍沿用旧概率", p.Materials[2404], p.Materials[20], p.Activities.House)
	}
}
