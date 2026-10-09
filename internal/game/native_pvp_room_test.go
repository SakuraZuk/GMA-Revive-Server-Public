package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hs-server/internal/nativeengine"
)

func nativeRoomTestUnits(t *testing.T, r *HumanPvpRoom, recipient string) []any {
	t.Helper()
	left := recipient == r.IDs[0]
	sign, offset := 1, [3]int{}
	if !left {
		sign, offset = -1, [3]int{5, -2, -3}
	}
	ids := map[string]string{}
	for _, unit := range r.Native.InitialUnits {
		nativeID := fmt.Sprint(unit["eid"])
		ids[nativeID] = "client-" + recipient[:4] + "-" + nativeID
	}
	result := make([]any, 0, len(r.Native.InitialUnits))
	for _, unit := range r.Native.InitialUnits {
		raw, _ := json.Marshal(unit)
		var client map[string]any
		if err := json.Unmarshal(raw, &client); err != nil {
			t.Fatal(err)
		}
		nativeID := fmt.Sprint(unit["eid"])
		client["eid"] = ids[nativeID]
		if camp, ok := client["camp"].(float64); ok && !left && (camp == 1 || camp == 2) {
			client["camp"] = 3 - camp
		}
		if kind, _ := client["kind"].(string); kind == "hero" || kind == "monster" || kind == "fighter" {
			camp, _ := client["camp"].(float64)
			if camp == 2 {
				client["kind"] = "enemy"
			} else {
				client["kind"] = "ally"
			}
		}
		var cube []int
		cubeRaw, _ := json.Marshal(unit["hex"])
		if json.Unmarshal(cubeRaw, &cube) == nil && len(cube) == 3 {
			for i := range cube {
				cube[i] = sign * (cube[i] - offset[i])
			}
			client["hex"] = cube
		}
		if creator := fmt.Sprint(unit["create_entity_eid"]); creator != "<nil>" && ids[creator] != "" {
			client["create_entity_eid"] = ids[creator]
		}
		result = append(result, client)
	}
	return result
}

func nativeRoomEvent(t *testing.T, s *Service, store SocialAccounts, c *Connection, generation, sequence int64, kind string, data map[string]any) {
	t.Helper()
	r := c.SelectedAvatarUnsafe().Progress.Social.HumanRoom
	raw, _ := json.Marshal(data)
	var normalized map[string]any
	_ = json.Unmarshal(raw, &normalized)
	handled, _, err := s.absorbHumanBattleEvent(context.Background(), c, &battleEnvelope{BattleUUID: r.UUID, Sequence: sequence, Generation: generation, Kind: kind, Data: normalized})
	if !handled || err != nil {
		t.Fatalf("权威事件%s seq=%d失败：%v", kind, sequence, err)
	}
	humanTestRefresh(t, store, c)
}

func TestNativePvpRoomActualAuthorityTimeoutDuelSettlesOnce(t *testing.T) {
	python := os.Getenv("HS_NATIVE_PVP_TEST_PYTHON2")
	if python == "" {
		t.Skip("隔离Python2真实引擎只由专项显式启用")
	}
	s, all, store := socialTestWorld(t)
	root, err := filepath.Abs("../nativepvp")
	if err != nil {
		t.Fatal(err)
	}
	s.nativePvpConfig = &nativeengine.Config{Python: python, Worker: filepath.Join(root, "battle_native_worker.py"), Directory: filepath.Join(root, "runtime/native_engine")}
	s.nativePvpResource = "bed95c22e6e6adf4ccf01cabb5156b4888ccc5744f0e192d24501e6df3c19ef3"
	cs := all[:2]
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	ids := []string{hexOf(selectedOID(cs[0])), hexOf(selectedOID(cs[1]))}
	_, err = store.UpdateSocial(ctx, ids, func(v map[string]*Avatar) error {
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
		room, e := s.newHumanRoom(v, ids, true)
		_ = room
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		humanTestRefresh(t, store, c)
		s.attachPlayer(c)
	}
	for _, c := range cs {
		if handled, _, e := s.humanCardsRPC(ctx, c, "send_pvp_cards", socialArgs(0)); !handled || e != nil {
			t.Fatal("冻结阵容或启动原生失败", e)
		}
		for _, other := range cs {
			humanTestRefresh(t, store, other)
		}
	}
	room := cs[0].SelectedAvatarUnsafe().Progress.Social.HumanRoom
	if room.Native == nil || room.Native.Journal.Head == "" {
		t.Fatal("双方选卡后未持久启动原生权威")
	}
	for _, c := range cs {
		if handled, _, e := s.humanCardsRPC(ctx, c, "pvp_load_complete", nil); !handled || e != nil {
			t.Fatal(e)
		}
		for _, other := range cs {
			humanTestRefresh(t, store, other)
		}
	}
	for _, c := range cs {
		room = c.SelectedAvatarUnsafe().Progress.Social.HumanRoom
		id := hexOf(selectedOID(c))
		layout := room.Players[id].Layout
		if handled, _, e := s.humanBattleFighting(ctx, c, socialArgs(map[string]any{"fighting_cards": layout.Fighting, "support_cards": layout.Support})); !handled || e != nil {
			t.Fatal(e)
		}
		for _, other := range cs {
			humanTestRefresh(t, store, other)
		}
	}
	seq := map[string]int64{ids[0]: 0, ids[1]: 0}
	for _, c := range cs {
		id := hexOf(selectedOID(c))
		generation := c.SelectedAvatarUnsafe().Progress.Social.HumanRoom.Native.Clients[id].Generation
		seq[id]++
		nativeRoomEvent(t, s, store, c, generation, seq[id], "ready", map[string]any{"version": 1, "bridge_revision": 31})
		room = c.SelectedAvatarUnsafe().Progress.Social.HumanRoom
		seq[id]++
		nativeRoomEvent(t, s, store, c, generation, seq[id], "started", map[string]any{"units": nativeRoomTestUnits(t, room, id)})
		for _, other := range cs {
			humanTestRefresh(t, store, other)
		}
	}
	seenOutbound := map[string]int{}
	ack := func() {
		for _, c := range cs {
			id := hexOf(selectedOID(c))
			humanTestRefresh(t, store, c)
			room = c.SelectedAvatarUnsafe().Progress.Social.HumanRoom
			client := room.Native.Clients[id]
			for _, outbound := range room.Native.Outbound {
				if outbound.Target != id || outbound.Index <= seenOutbound[id] {
					continue
				}
				seenOutbound[id] = outbound.Index
				if outbound.Window {
					continue
				}
				eid, command, round, e := nativeengine.PlaybackExpectation(&outbound.Playback)
				if e != nil {
					t.Fatal(e)
				}
				seq[id]++
				nativeRoomEvent(t, s, store, c, client.Generation, seq[id], "command", map[string]any{"eid": eid, "command": command})
				seq[id]++
				nativeRoomEvent(t, s, store, c, client.Generation, seq[id], "input", map[string]any{"eid": round})
			}
		}
		for _, c := range cs {
			humanTestRefresh(t, store, c)
		}
	}
	ack()
	_, err = s.humanTransaction(ctx, cs[0], func(current *HumanPvpRoom, _ map[string]*Avatar, _ string) error {
		if !nativeAllClientsReady(current) || !current.Native.Update.State.AwaitingPlayer {
			return errors.New("恢复测试前客户端播放屏障未完成")
		}
		return s.advanceNativeUntilBoundary(ctx, current, map[string]any{"operation": "timeout"})
	})
	if err != nil {
		t.Fatal("恢复测试的首个权威超时失败", err)
	}
	ack()
	room = cs[1].SelectedAvatarUnsafe().Progress.Social.HumanRoom
	if room.Native.Update.Result != nil || len(room.Native.Canonical) == 0 {
		t.Fatal("恢复测试需要至少一条已提交规范命令且尚未结束")
	}
	if handled, _, e := s.humanResume(ctx, cs[1]); !handled || e != nil {
		t.Fatal("权威房间恢复初始化失败", e)
	}
	for _, c := range cs {
		humanTestRefresh(t, store, c)
	}
	if handled, _, e := s.humanCardsRPC(ctx, cs[1], "pvp_load_complete", nil); !handled || e != nil {
		t.Fatal("恢复端重新加载失败", e)
	}
	for _, c := range cs {
		humanTestRefresh(t, store, c)
	}
	room = cs[1].SelectedAvatarUnsafe().Progress.Social.HumanRoom
	recoverID := ids[1]
	layout := room.Players[recoverID].Layout
	if handled, _, e := s.humanBattleFighting(ctx, cs[1], socialArgs(map[string]any{"fighting_cards": layout.Fighting, "support_cards": layout.Support})); !handled || e != nil {
		t.Fatal("恢复端共同开战屏障失败", e)
	}
	for _, c := range cs {
		humanTestRefresh(t, store, c)
	}
	room = cs[1].SelectedAvatarUnsafe().Progress.Social.HumanRoom
	generation := room.Native.Clients[recoverID].Generation
	if generation != 2 {
		t.Fatal("恢复端没有进入新播放世代", generation)
	}
	seq[recoverID] = 1
	nativeRoomEvent(t, s, store, cs[1], generation, seq[recoverID], "ready", map[string]any{"version": 1, "bridge_revision": 31})
	room = cs[1].SelectedAvatarUnsafe().Progress.Social.HumanRoom
	seq[recoverID]++
	nativeRoomEvent(t, s, store, cs[1], generation, seq[recoverID], "started", map[string]any{"units": nativeRoomTestUnits(t, room, recoverID)})
	for _, c := range cs {
		humanTestRefresh(t, store, c)
	}
	ack()
	room = cs[1].SelectedAvatarUnsafe().Progress.Social.HumanRoom
	recovered := room.Native.Clients[recoverID]
	if !recovered.Checkpoint.Ready() {
		t.Fatal("恢复端没有按新映射完整重放已提交规范命令")
	}
	for turn := 0; turn < 80; turn++ {
		room = cs[0].SelectedAvatarUnsafe().Progress.Social.HumanRoom
		if room.Native.Update.Result != nil {
			break
		}
		_, err = s.humanTransaction(ctx, cs[0], func(current *HumanPvpRoom, _ map[string]*Avatar, _ string) error {
			if !nativeAllClientsReady(current) || !current.Native.Update.State.AwaitingPlayer {
				return errors.New("测试推进前客户端播放屏障未完成")
			}
			return s.advanceNativeUntilBoundary(ctx, current, map[string]any{"operation": "timeout"})
		})
		if err != nil {
			t.Fatal("权威超时推进失败", err)
		}
		ack()
	}
	room = cs[0].SelectedAvatarUnsafe().Progress.Social.HumanRoom
	if room.Native.Update.Result == nil || len(room.Native.Winners) != 1 {
		t.Fatal("真实原生权威未在保护轮数内结束")
	}
	for _, c := range cs {
		id := hexOf(selectedOID(c))
		client := c.SelectedAvatarUnsafe().Progress.Social.HumanRoom.Native.Clients[id]
		seq[id]++
		nativeRoomEvent(t, s, store, c, client.Generation, seq[id], "result", map[string]any{"winner_eids": room.Native.Winners})
		for _, other := range cs {
			humanTestRefresh(t, store, other)
		}
	}
	room = cs[0].SelectedAvatarUnsafe().Progress.Social.HumanRoom
	if room.Status != "settled" || room.Winner != room.Native.Winners[0] || room.FinalDigest != room.Native.Journal.Head {
		t.Fatal("原生胜方、日志末端与唯一结算未原子落地")
	}
	before := mustJSON(room)
	client := room.Native.Clients[ids[0]]
	_, _, retryErr := s.absorbHumanBattleEvent(ctx, cs[0], &battleEnvelope{BattleUUID: room.UUID, Sequence: seq[ids[0]], Generation: client.Generation, Kind: "result", Data: map[string]any{"winner_eids": []any{room.Winner}}})
	if retryErr != nil {
		t.Fatal("结算重试应保持幂等", retryErr)
	}
	humanTestRefresh(t, store, cs[0])
	if string(before) != string(mustJSON(cs[0].SelectedAvatarUnsafe().Progress.Social.HumanRoom)) {
		t.Fatal("结算网络重试改变了已提交房间")
	}
	t.Logf("真实原生房间完成：日志%d项，规范命令%d条，投递%d条", len(room.Native.Journal.Entries), len(room.Native.Canonical), len(room.Native.Outbound))
}
