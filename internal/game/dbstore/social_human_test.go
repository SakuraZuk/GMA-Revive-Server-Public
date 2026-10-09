package dbstore

import (
	"context"
	"encoding/hex"
	"errors"
	"hs-server/internal/game"
	"testing"
	"time"
)

func pgHumanRead(t *testing.T, s *Store, oid []byte) game.Avatar {
	t.Helper()
	rows, err := New(s.pool).SocialAvatars(context.Background(), game.SocialSearch{OIDs: []string{hex.EncodeToString(oid)}, Limit: 1})
	if err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	return rows[0]
}
func pgHumanTick(t *testing.T, s *game.Service, cs ...*game.Connection) []game.Push {
	t.Helper()
	out := []game.Push{}
	for _, c := range cs {
		p, e := s.Tick(context.Background(), c)
		if e != nil {
			t.Fatal(e)
		}
		out = append(out, p...)
	}
	return out
}
func pgHumanUnits(r *game.HumanPvpRoom) []any {
	units := []any{}
	for i, id := range r.IDs {
		p := r.Players[id]
		eid := "1"
		if i == 1 {
			eid = "2"
		}
		units = append(units, map[string]any{"eid": eid, "role": p.Cards[0].CardID, "hex": []int{i, -i, 0}, "master": id, "card_uuid": p.Layout.Fighting[0], "camp": i + 1, "kind": "ally", "alive": true, "hp": 100, "max_hp": 100, "ap": 0})
	}
	return units
}
func pgHumanEvent(t *testing.T, s *game.Service, c *game.Connection, uuid string, seq int, kind string, data map[string]any) {
	t.Helper()
	pgSocialCall(t, s, c, "do_command", "__battle_event__", []any{map[string]any{"battle_uuid": uuid, "sequence": seq, "kind": kind, "data": data}})
}

// 真实PostgreSQL双端原生RPC链：共享匹配/seed/冻结/EID屏障/二结果事务/重连原日志/分歧拒奖。
func TestPostgresSocialHumanSharedMatchSettlementDivergenceAndReconnect(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	svc := pgSocialService(t, store)
	svc.Now = func() time.Time { return now }
	identities := []game.Identity{}
	cs := []*game.Connection{}
	for _, account := range []string{"真人竞技事务甲", "真人竞技事务乙"} {
		info := game.ClientInfo{Account: account, Password: "pw", Hostnum: 1}
		id, e := store.Register(ctx, info)
		if e != nil {
			t.Fatal(e)
		}
		identities = append(identities, id)
		c := pgSocialLogin(t, svc, info, id.Avatars[0].OID)
		cs = append(cs, c)
	}
	defer svc.Detach(cs[0])
	defer svc.Detach(cs[1])
	ids := []string{hex.EncodeToString(identities[0].Avatars[0].OID), hex.EncodeToString(identities[1].Avatars[0].OID)}
	pgSocialCall(t, svc, cs[0], "apply_friend", 1, ids[1], "真实共享战斗", 1, 0)
	pgSocialCall(t, svc, cs[1], "agree_apply_friend", 2, ids[0])
	for _, c := range cs {
		pgSocialCall(t, svc, c, "start_sync_pvp_match", 3)
	}
	now = now.Add(2 * time.Second)
	pgHumanTick(t, svc, cs...)
	r := pgHumanRead(t, store, identities[0].Avatars[0].OID).Progress.Social.HumanRoom
	other := pgHumanRead(t, store, identities[1].Avatars[0].OID).Progress.Social.HumanRoom
	if r == nil || other == nil || r.UUID != other.UUID || r.Seed != other.Seed || !r.Rated {
		t.Fatal("真实库两端未得到同一房间与seed")
	}
	for _, c := range cs {
		pgSocialCall(t, svc, c, "send_pvp_cards", 0)
		pgHumanTick(t, svc, cs...)
	}
	for _, c := range cs {
		pgSocialCall(t, svc, c, "pvp_load_complete")
		pgHumanTick(t, svc, cs...)
	}
	r = pgHumanRead(t, store, identities[0].Avatars[0].OID).Progress.Social.HumanRoom
	for i, c := range cs {
		layout := r.Players[ids[i]].Layout
		pgSocialCall(t, svc, c, "battle_fighting", map[string]any{"fighting_cards": layout.Fighting, "support_cards": layout.Support})
		pgHumanTick(t, svc, cs...)
	}
	for _, c := range cs {
		pgHumanEvent(t, svc, c, r.UUID, 1, "ready", map[string]any{"version": 1})
		pgHumanEvent(t, svc, c, r.UUID, 2, "started", map[string]any{"units": pgHumanUnits(r)})
	}
	owner, peer := cs[0], cs[1]
	ownerIndex := 0
	if ids[0] != r.IDs[0] {
		owner, peer = peer, owner
		ownerIndex = 1
	}
	frame := map[string]any{"action": 1, "eid": "1", "master": r.IDs[0], "units": pgHumanUnits(r), "command_index": 0}
	pgHumanEvent(t, svc, owner, r.UUID, 3, "human_input", frame)
	pgSocialCall(t, svc, owner, "do_command", "move_to", []any{"1", []int{1, -1, 0}})
	av := pgHumanRead(t, store, identities[ownerIndex].Avatars[0].OID)
	if av.Progress.Social.HumanRoom.Pending == nil || len(av.Progress.Social.HumanRoom.Commands) != 0 {
		t.Fatal("真实库单端屏障提前消费")
	}
	pgHumanEvent(t, svc, peer, r.UUID, 3, "human_input", frame)
	av = pgHumanRead(t, store, identities[ownerIndex].Avatars[0].OID)
	if len(av.Progress.Social.HumanRoom.Commands) != 1 {
		t.Fatal("真实库共同屏障未记录唯一输入")
	}
	// 原房间恢复先保留唯一命令位置；正常恢复实际原生双端演算由MuMu门槛继续验。
	pgHumanTick(t, svc, cs...)
	pgSocialCall(t, svc, owner, "client_need_recover_battle")
	av = pgHumanRead(t, store, identities[ownerIndex].Avatars[0].OID)
	if av.Progress.Social.HumanRoom.UUID != r.UUID || av.Progress.Social.HumanRoom.Seed != r.Seed || av.Progress.Social.HumanRoom.Recovering[r.IDs[0]] != 1 {
		t.Fatal("真实库恢复重开成独立胜负")
	}
	pgSocialCall(t, svc, owner, "pvp_load_complete")
	layout := r.Players[r.IDs[0]].Layout
	pgSocialCall(t, svc, owner, "battle_fighting", map[string]any{"fighting_cards": layout.Fighting, "support_cards": layout.Support})
	pgHumanEvent(t, svc, owner, r.UUID, 1, "ready", map[string]any{"version": 1})
	pgHumanEvent(t, svc, owner, r.UUID, 2, "started", map[string]any{"units": pgHumanUnits(r)})
	frame["action"] = 2
	frame["command_index"] = 1
	pgHumanEvent(t, svc, owner, r.UUID, 3, "human_input", frame)
	before := av.Progress.SyncPvpScore
	final := map[string]any{"action": 2, "winner_eids": []string{r.IDs[0]}, "units": pgHumanUnits(r), "command_index": 1, "finished_task_list": []any{}}
	pgHumanEvent(t, svc, owner, r.UUID, 4, "human_result", final)
	if pgHumanRead(t, store, identities[ownerIndex].Avatars[0].OID).Progress.SyncPvpScore != before {
		t.Fatal("真实库第一份result提前发分")
	}
	pgHumanEvent(t, svc, peer, r.UUID, 4, "human_result", final)
	left, right := pgHumanRead(t, store, identities[0].Avatars[0].OID), pgHumanRead(t, store, identities[1].Avatars[0].OID)
	if len(left.Progress.SyncPvpRecords) != 1 || len(right.Progress.SyncPvpRecords) != 1 || left.Progress.Social.HumanRoom.Status != "settled" || right.Progress.Social.HumanRoom.Status != "settled" {
		t.Fatal("真实库双端结果没有原子保存")
	}
	pgHumanEvent(t, svc, owner, r.UUID, 4, "human_result", final)
	if len(pgHumanRead(t, store, identities[ownerIndex].Avatars[0].OID).Progress.SyncPvpRecords) != 1 {
		t.Fatal("真实库重复result重复计奖")
	}
	_, err := store.UpdateSocial(ctx, ids, func(v map[string]*game.Avatar) error {
		v[ids[0]].Progress.SyncPvpScore += 999
		v[ids[1]].Progress.SyncPvpScore += 999
		return errors.New("真人奖励双端故障注入")
	})
	if err == nil {
		t.Fatal("真实库故障提交")
	}
	if pgHumanRead(t, store, identities[0].Avatars[0].OID).Progress.SyncPvpScore != left.Progress.SyncPvpScore || pgHumanRead(t, store, identities[1].Avatars[0].OID).Progress.SyncPvpScore != right.Progress.SyncPvpScore {
		t.Fatal("真实库双端奖励部分提交")
	}
	// 第二场走原生好友挑战；不同胜方必须持久中断并保持第一场积分/记录。
	pgHumanTick(t, svc, cs...)
	pgSocialCall(t, svc, cs[0], "challenge_friend", 4, ids[1])
	pgHumanTick(t, svc, cs...)
	pgSocialCall(t, svc, cs[1], "agree_challenge", 5, ids[0])
	pgHumanTick(t, svc, cs...)
	for _, c := range cs {
		pgSocialCall(t, svc, c, "send_pvp_cards", 0)
		pgHumanTick(t, svc, cs...)
	}
	for _, c := range cs {
		pgSocialCall(t, svc, c, "pvp_load_complete")
		pgHumanTick(t, svc, cs...)
	}
	r = pgHumanRead(t, store, identities[0].Avatars[0].OID).Progress.Social.HumanRoom
	for i, c := range cs {
		layout := r.Players[ids[i]].Layout
		pgSocialCall(t, svc, c, "battle_fighting", map[string]any{"fighting_cards": layout.Fighting, "support_cards": layout.Support})
		pgHumanTick(t, svc, cs...)
	}
	for _, c := range cs {
		pgHumanEvent(t, svc, c, r.UUID, 1, "ready", map[string]any{"version": 1})
		pgHumanEvent(t, svc, c, r.UUID, 2, "started", map[string]any{"units": pgHumanUnits(r)})
	}
	for i, c := range cs {
		pgHumanEvent(t, svc, c, r.UUID, 3, "human_result", map[string]any{"action": 1, "winner_eids": []string{ids[i]}, "units": pgHumanUnits(r), "command_index": 0})
	}
	for i, id := range identities {
		av := pgHumanRead(t, store, id.Avatars[0].OID)
		score := left.Progress.SyncPvpScore
		if i == 1 {
			score = right.Progress.SyncPvpScore
		}
		if av.Progress.Social.HumanRoom.Status != "aborted" || av.Progress.Battle.Outcome != "" || av.Progress.SyncPvpScore != score || len(av.Progress.SyncPvpRecords) != 1 {
			t.Fatal("真实库双结果分歧被假结算")
		}
	}
}
