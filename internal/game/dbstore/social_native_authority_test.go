package dbstore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hs-server/internal/game"
	"hs-server/internal/nativeengine"
)

func pgNativeUnits(t *testing.T, r *game.HumanPvpRoom, recipient string) []any {
	t.Helper()
	left := recipient == r.IDs[0]
	sign, offset := 1, [3]int{}
	if !left {
		sign, offset = -1, [3]int{5, -2, -3}
	}
	ids := map[string]string{}
	for _, unit := range r.Native.InitialUnits {
		nativeID := fmt.Sprint(unit["eid"])
		ids[nativeID] = "pg-" + recipient[:4] + "-" + nativeID
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

func pgNativeEvent(t *testing.T, s *game.Service, c *game.Connection, uuid string, generation, sequence int64, kind string, data map[string]any) {
	t.Helper()
	pgSocialCall(t, s, c, "do_command", "__battle_event__", []any{map[string]any{
		"battle_uuid": uuid, "generation": generation, "sequence": sequence, "kind": kind, "data": data,
	}})
}

func pgNativeJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

// 真实PostgreSQL验证原生日志、双方房间镜像、持久投递和唯一积分结算在同一事务。
func TestPostgresNativePvpAuthorityJournalPlaybackAndSettlement(t *testing.T) {
	python := os.Getenv("HS_NATIVE_PVP_TEST_PYTHON2")
	if python == "" {
		t.Skip("隔离Python2真实引擎只由专项显式启用")
	}
	store := testStore(t)
	root, err := filepath.Abs("../../nativepvp")
	if err != nil {
		t.Fatal(err)
	}
	worker := os.Getenv("HS_NATIVE_PVP_TEST_WORKER")
	directory := os.Getenv("HS_NATIVE_PVP_TEST_DIRECTORY")
	resource := os.Getenv("HS_NATIVE_PVP_TEST_RESOURCE_SHA256")
	if worker == "" {
		worker = filepath.Join(root, "battle_native_worker.py")
	}
	if directory == "" {
		directory = filepath.Join(root, "runtime/native_engine")
	}
	if resource == "" {
		resource = "bed95c22e6e6adf4ccf01cabb5156b4888ccc5744f0e192d24501e6df3c19ef3"
	}
	t.Setenv("HS_NATIVE_PVP_PYTHON", python)
	t.Setenv("HS_NATIVE_PVP_WORKER", worker)
	t.Setenv("HS_NATIVE_PVP_DIRECTORY", directory)
	t.Setenv("HS_NATIVE_PVP_RESOURCE_SHA256", resource)
	ctx := context.Background()
	now := time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC)
	svc := pgSocialService(t, store)
	svc.Now = func() time.Time { return now }
	identities := []game.Identity{}
	cs := []*game.Connection{}
	for _, account := range []string{"单原生实库甲", "单原生实库乙"} {
		info := game.ClientInfo{Account: account, Password: "pw", Hostnum: 1}
		identity, e := store.Register(ctx, info)
		if e != nil {
			t.Fatal(e)
		}
		identities = append(identities, identity)
		cs = append(cs, pgSocialLogin(t, svc, info, identity.Avatars[0].OID))
	}
	defer svc.Detach(cs[0])
	defer svc.Detach(cs[1])
	ids := []string{hex.EncodeToString(identities[0].Avatars[0].OID), hex.EncodeToString(identities[1].Avatars[0].OID)}
	byID := map[string]*game.Connection{ids[0]: cs[0], ids[1]: cs[1]}
	pgSocialCall(t, svc, cs[0], "apply_friend", 1, ids[1], "单原生实库", 1, 0)
	pgSocialCall(t, svc, cs[1], "agree_apply_friend", 2, ids[0])
	for _, c := range cs {
		pgSocialCall(t, svc, c, "start_sync_pvp_match", 3)
	}
	now = now.Add(2 * time.Second)
	pgHumanTick(t, svc, cs...)
	for _, c := range cs {
		pgSocialCall(t, svc, c, "send_pvp_cards", 0)
		pgHumanTick(t, svc, cs...)
	}
	r := pgHumanRead(t, store, identities[0].Avatars[0].OID).Progress.Social.HumanRoom
	if r == nil || r.Native == nil || r.Native.Journal.Head == "" {
		t.Fatal("真实库没有把原生启动日志写入双方房间")
	}
	for _, c := range cs {
		pgSocialCall(t, svc, c, "pvp_load_complete")
		pgHumanTick(t, svc, cs...)
	}
	for _, id := range ids {
		r = pgHumanRead(t, store, identities[0].Avatars[0].OID).Progress.Social.HumanRoom
		layout := r.Players[id].Layout
		pgSocialCall(t, svc, byID[id], "battle_fighting", map[string]any{"fighting_cards": layout.Fighting, "support_cards": layout.Support})
		pgHumanTick(t, svc, cs...)
	}
	seq := map[string]int64{ids[0]: 0, ids[1]: 0}
	for _, id := range ids {
		r = pgHumanRead(t, store, identities[0].Avatars[0].OID).Progress.Social.HumanRoom
		generation := r.Native.Clients[id].Generation
		seq[id]++
		pgNativeEvent(t, svc, byID[id], r.UUID, generation, seq[id], "ready", map[string]any{"version": 1, "bridge_revision": 31})
		seq[id]++
		pgNativeEvent(t, svc, byID[id], r.UUID, generation, seq[id], "started", map[string]any{"units": pgNativeUnits(t, r, id)})
		pgHumanTick(t, svc, cs...)
	}
	seen := map[string]int{}
	ack := func() {
		for _, id := range ids {
			r = pgHumanRead(t, store, identities[0].Avatars[0].OID).Progress.Social.HumanRoom
			client := r.Native.Clients[id]
			for _, outbound := range r.Native.Outbound {
				if outbound.Target != id || outbound.Index <= seen[id] {
					continue
				}
				seen[id] = outbound.Index
				if outbound.Window {
					continue
				}
				eid, command, round, e := nativeengine.PlaybackExpectation(&outbound.Playback)
				if e != nil {
					t.Fatal(e)
				}
				seq[id]++
				pgNativeEvent(t, svc, byID[id], r.UUID, client.Generation, seq[id], "command", map[string]any{"eid": eid, "command": command})
				seq[id]++
				pgNativeEvent(t, svc, byID[id], r.UUID, client.Generation, seq[id], "input", map[string]any{"eid": round})
			}
		}
		pgHumanTick(t, svc, cs...)
	}
	ack()
	for turn := 0; turn < 80; turn++ {
		r = pgHumanRead(t, store, identities[0].Avatars[0].OID).Progress.Social.HumanRoom
		if r.Native.Update.Result != nil {
			break
		}
		now = now.Add(21 * time.Second)
		pgHumanTick(t, svc, cs...)
		ack()
	}
	r = pgHumanRead(t, store, identities[0].Avatars[0].OID).Progress.Social.HumanRoom
	if r.Native.Update.Result == nil || len(r.Native.Winners) != 1 {
		t.Fatal("真实库权威房间未完成原生对局")
	}
	ack()
	for _, id := range ids {
		r = pgHumanRead(t, store, identities[0].Avatars[0].OID).Progress.Social.HumanRoom
		client := r.Native.Clients[id]
		seq[id]++
		pgNativeEvent(t, svc, byID[id], r.UUID, client.Generation, seq[id], "result", map[string]any{"winner_eids": r.Native.Winners})
		pgHumanTick(t, svc, cs...)
	}
	left := pgHumanRead(t, store, identities[0].Avatars[0].OID)
	right := pgHumanRead(t, store, identities[1].Avatars[0].OID)
	if left.Progress.Social.HumanRoom.Status != "settled" || right.Progress.Social.HumanRoom.Status != "settled" || len(left.Progress.SyncPvpRecords) != 1 || len(right.Progress.SyncPvpRecords) != 1 {
		t.Fatal("真实库没有原子提交双方权威结算与记录")
	}
	if left.Progress.Social.HumanRoom.FinalDigest != left.Progress.Social.HumanRoom.Native.Journal.Head || pgNativeJSON(left.Progress.Social.HumanRoom) != pgNativeJSON(right.Progress.Social.HumanRoom) {
		t.Fatal("真实库双方房间镜像或权威日志末端不一致")
	}
	t.Logf("真实PG权威房间完成：日志%d项，规范命令%d条", len(left.Progress.Social.HumanRoom.Native.Journal.Entries), len(left.Progress.Social.HumanRoom.Native.Canonical))
}
