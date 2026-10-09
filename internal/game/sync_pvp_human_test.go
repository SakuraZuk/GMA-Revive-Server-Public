package game

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func humanTestPair(t *testing.T, rated bool) (*Service, []*Connection, *FixtureAccounts, *HumanPvpRoom) {
	t.Helper()
	s, all, store := socialTestWorld(t)
	cs := all[:2]
	ctx := context.Background()
	ids := []string{hexOf(selectedOID(cs[0])), hexOf(selectedOID(cs[1]))}
	_, err := store.UpdateSocial(ctx, ids, func(v map[string]*Avatar) error {
		for _, id := range ids {
			av := v[id]
			ensureSocial(&av.Progress.Social)
			peer := ids[0]
			if peer == id {
				peer = ids[1]
			}
			av.Progress.Social.Friends[peer] = SocialFriend{Info: socialProfile(*v[peer]), Time: s.Now().Unix()}
			if e := ensureSyncPvpPeriod(&av.Progress, s.Now()); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		humanTestRefresh(t, store, c)
		s.attachPlayer(c)
	}
	var room *HumanPvpRoom
	if rated {
		_, err = store.UpdateSocial(ctx, ids, func(v map[string]*Avatar) error { var e error; room, e = s.newHumanRoom(v, ids, true); return e })
	} else {
		p, e := s.humanChallengeRPC(ctx, cs[0], "challenge_friend", socialArgs(1, ids[1]))
		if e != nil || socialCallback(t, p)[0] != 0 {
			t.Fatal(e, p)
		}
		humanTestRefresh(t, store, cs[1])
		p, err = s.humanChallengeRPC(ctx, cs[1], "agree_challenge", socialArgs(2, ids[0]))
		if err == nil && socialCallback(t, p)[0] != 0 {
			t.Fatal(p)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		humanTestRefresh(t, store, c)
		_ = s.flushHumanPvp(c)
	}
	room = cs[0].SelectedAvatarUnsafe().Progress.Social.HumanRoom
	for _, c := range cs {
		handled, _, e := s.humanCardsRPC(ctx, c, "send_pvp_cards", socialArgs(0))
		if !handled || e != nil {
			t.Fatal(e)
		}
		for _, other := range cs {
			humanTestRefresh(t, store, other)
		}
	}
	for _, c := range cs {
		handled, _, e := s.humanCardsRPC(ctx, c, "pvp_load_complete", nil)
		if !handled || e != nil {
			t.Fatal(e)
		}
		for _, other := range cs {
			humanTestRefresh(t, store, other)
		}
	}
	for _, c := range cs {
		layout := c.SelectedAvatarUnsafe().Progress.Social.HumanRoom.Players[hexOf(selectedOID(c))].Layout
		handled, _, e := s.humanBattleFighting(ctx, c, socialArgs(map[string]any{"fighting_cards": layout.Fighting, "support_cards": layout.Support}))
		if !handled || e != nil {
			t.Fatal(e)
		}
		for _, other := range cs {
			humanTestRefresh(t, store, other)
		}
	}
	room = cs[0].SelectedAvatarUnsafe().Progress.Social.HumanRoom
	for _, c := range cs {
		humanTestEvent(t, s, c, 1, "ready", map[string]any{"version": 1})
		humanTestEvent(t, s, c, 2, "started", map[string]any{"units": humanTestUnits(room)})
	}
	for _, c := range cs {
		humanTestRefresh(t, store, c)
		_ = s.flushHumanPvp(c)
	}
	return s, cs, store, room
}
func humanTestRefresh(t *testing.T, store SocialAccounts, c *Connection) {
	t.Helper()
	av := socialSaved(t, store, c)
	for i := range c.identity.Avatars {
		if c.identity.Avatars[i].Hostnum == c.hostnum {
			c.identity.Avatars[i] = av
		}
	}
}
func humanTestUnits(r *HumanPvpRoom) []any {
	result := []any{}
	for index, id := range r.IDs {
		player := r.Players[id]
		result = append(result, map[string]any{"eid": intString(index + 1), "role": player.Cards[0].CardID, "hex": []int{index, -index, 0}, "master": id, "card_uuid": player.Layout.Fighting[0], "camp": index + 1, "alive": true, "hp": 100, "max_hp": 100, "ap": 0, "kind": "ally"})
	}
	return result
}
func humanTestEnvelope(c *Connection, seq int64, kind string, data map[string]any) *battleEnvelope {
	blob, _ := json.Marshal(data)
	var normalized map[string]any
	_ = json.Unmarshal(blob, &normalized)
	return &battleEnvelope{BattleUUID: c.SelectedAvatarUnsafe().Progress.Social.HumanRoom.UUID, Sequence: seq, Kind: kind, Data: normalized}
}
func humanTestEvent(t *testing.T, s *Service, c *Connection, seq int64, kind string, data map[string]any) {
	t.Helper()
	handled, _, err := s.absorbHumanBattleEvent(context.Background(), c, humanTestEnvelope(c, seq, kind, data))
	if !handled || err != nil {
		t.Fatal(kind, err)
	}
}

func TestHumanPvpSharedRoomInputBarrierAtomicResultAndResume(t *testing.T) {
	s, cs, store, r := humanTestPair(t, true)
	ctx := context.Background()
	id := r.IDs[0]
	owner := cs[0]
	peer := cs[1]
	if hexOf(selectedOID(owner)) != id {
		owner, peer = peer, owner
	}
	frame := map[string]any{"action": 1, "eid": "1", "master": id, "units": humanTestUnits(r), "command_index": 0}
	humanTestEvent(t, s, owner, 3, "human_input", frame)
	args := []any{"1", []any{float64(1), float64(-1), float64(0)}}
	handled, _, err := s.humanDoCommand(ctx, owner, "move_to", args)
	if !handled || err != nil {
		t.Fatal(err)
	}
	pending := socialSaved(t, store, owner).Progress.Social.HumanRoom
	if pending.Pending == nil || len(pending.Commands) != 0 {
		t.Fatal("另一端未确认时命令已经消费")
	}
	humanTestEvent(t, s, peer, 3, "human_input", frame)
	for _, c := range cs {
		humanTestRefresh(t, store, c)
		commands := s.flushHumanPvp(c)
		if len(commands) != 1 || commands[0].Args[0] != "revival_do_shared_command" {
			t.Fatal("未同时投递唯一共享输入", commands)
		}
	}
	if _, _, err = s.humanDoCommand(ctx, peer, "move_to", args); err == nil {
		t.Fatal("非当前控制者可以提交输入")
	}
	before := socialSaved(t, store, owner).Progress.SyncPvpScore
	final := map[string]any{"action": 2, "winner_eids": []string{id}, "units": humanTestUnits(r), "command_index": 1, "finished_task_list": []any{}}
	humanTestEvent(t, s, owner, 4, "human_result", final)
	if socialSaved(t, store, owner).Progress.SyncPvpScore != before {
		t.Fatal("只有一端结果时提前加分")
	}
	humanTestEvent(t, s, peer, 4, "human_result", final)
	for _, c := range cs {
		humanTestRefresh(t, store, c)
	}
	left, right := socialSaved(t, store, owner), socialSaved(t, store, peer)
	if left.Progress.SyncPvpScore <= before || len(left.Progress.SyncPvpRecords) != 1 || len(right.Progress.SyncPvpRecords) != 1 || left.Progress.Social.HumanRoom.Status != "settled" {
		t.Fatal("双端结算未同事务生成记录与收据")
	}
	humanTestEvent(t, s, owner, 4, "human_result", final)
	if socialSaved(t, store, owner).Progress.SyncPvpScore != left.Progress.SyncPvpScore || len(socialSaved(t, store, peer).Progress.SyncPvpRecords) != 1 {
		t.Fatal("重复结果重复发奖")
	}
	changed := map[string]any{"action": 2, "winner_eids": []string{hexOf(selectedOID(peer))}, "units": humanTestUnits(r), "command_index": 1}
	if _, _, err = s.absorbHumanBattleEvent(ctx, owner, humanTestEnvelope(owner, 4, "human_result", changed)); err == nil {
		t.Fatal("结算后可覆盖胜方")
	}
	handled, pushes, err := s.humanResume(ctx, owner)
	if !handled || err != nil || len(pushes) < 2 {
		t.Fatal("重连未补发原收据", err)
	}
}

func TestHumanPvpDivergenceRollbackAndReplaySameSeed(t *testing.T) {
	s, cs, store, r := humanTestPair(t, false)
	ctx := context.Background()
	id := r.IDs[0]
	frame := map[string]any{"action": 1, "eid": "1", "master": id, "units": humanTestUnits(r), "command_index": 0}
	for _, c := range cs {
		humanTestEvent(t, s, c, 3, "human_input", frame)
	}
	owner := cs[0]
	if hexOf(selectedOID(owner)) != id {
		owner = cs[1]
	}
	if _, _, err := s.humanDoCommand(ctx, owner, "move_to", []any{"1", []any{1, -1, 0}}); err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		humanTestRefresh(t, store, c)
	}
	seed, uuid := r.Seed, r.UUID
	handled, pushes, err := s.humanResume(ctx, owner)
	if !handled || err != nil || len(pushes) != 6 {
		t.Fatal(err, pushes)
	}
	recovered := socialSaved(t, store, owner).Progress.Social.HumanRoom
	if recovered.UUID != uuid || recovered.Seed != seed || recovered.Recovering[id] != 1 || len(recovered.Commands) != 1 {
		t.Fatal("恢复生成了另一场独立对局")
	}
	before := socialSaved(t, store, owner)
	_, err = store.UpdateSocial(ctx, r.IDs, func(v map[string]*Avatar) error {
		v[id].Progress.SyncPvpScore += 99
		v[r.IDs[1]].Progress.Materials[26] = Material{ID: 26, Count: 999}
		return errors.New("故障注入")
	})
	if err == nil {
		t.Fatal("故障注入成功提交")
	}
	if socialSaved(t, store, owner).Progress.SyncPvpScore != before.Progress.SyncPvpScore || socialSaved(t, store, cs[1]).Progress.Materials[26].Count == 999 {
		t.Fatal("双端事务失败后部分提交")
	}
	// 用另一个完整房间证明双端显式相反结果只中断，绝不各发一次胜利。
	s, cs, store, r = humanTestPair(t, false)
	final := map[string]any{"action": 1, "winner_eids": []string{r.IDs[0]}, "units": humanTestUnits(r), "command_index": 0}
	humanTestEvent(t, s, cs[0], 3, "human_result", final)
	final["winner_eids"] = []string{r.IDs[1]}
	humanTestEvent(t, s, cs[1], 3, "human_result", final)
	for _, c := range cs {
		av := socialSaved(t, store, c)
		if av.Progress.Social.HumanRoom.Status != "aborted" || av.Progress.Battle.Outcome != "" || len(av.Progress.SyncPvpRecords) != 0 {
			t.Fatal("分歧被结算成胜负")
		}
	}
}

func TestHumanPvpActualOnlineRankedMatchBeforeRobot(t *testing.T) {
	s, all, store := socialTestWorld(t)
	ctx := context.Background()
	cs := all[:2]
	for _, c := range cs {
		s.attachPlayer(c)
		if _, err := s.startSyncPVP(ctx, c, socialArgs(1)); err != nil {
			t.Fatal(err)
		}
	}
	now := s.Now()
	s.Now = func() time.Time { return now.Add(time.Duration(syncPvpRule().MinMatchTime) * time.Second) }
	if _, err := s.tickHumanMatch(ctx, cs[0]); err != nil {
		t.Fatal(err)
	}
	a, b := socialSaved(t, store, cs[0]), socialSaved(t, store, cs[1])
	if a.Progress.Social.HumanRoom == nil || b.Progress.Social.HumanRoom == nil || a.Progress.Social.HumanRoom.UUID != b.Progress.Social.HumanRoom.UUID || !a.Progress.Social.HumanRoom.Rated || a.Progress.SyncPvpMatch != nil || b.Progress.SyncPvpMatch != nil {
		t.Fatal("两个真实排队角色没有共同匹配")
	}
}
