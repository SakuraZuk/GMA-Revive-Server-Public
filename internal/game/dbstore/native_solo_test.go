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

func TestPostgresNativeSoloAsyncAuthorityAndOfflineDefender(t *testing.T) { pgNativeSoloDuel(t, true) }
func TestPostgresNativeSoloRobotAuthorityAndColdRecovery(t *testing.T)    { pgNativeSoloDuel(t, false) }

// 实际PG/Handle和原生计算：冷读Journal、完整恢复、伪结果零写入及一次结算。
func pgNativeSoloDuel(t *testing.T, asynchronous bool) {
	t.Helper()
	python := os.Getenv("HS_NATIVE_PVP_TEST_PYTHON2")
	if python == "" {
		t.Skip("真实单端PVP必须显式启用私有Python2")
	}
	root, err := filepath.Abs("../../nativepvp")
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[string]string{"HS_NATIVE_PVP_TEST_WORKER": filepath.Join(root, "battle_native_worker.py"), "HS_NATIVE_PVP_TEST_DIRECTORY": filepath.Join(root, "runtime/native_engine"), "HS_NATIVE_PVP_TEST_RESOURCE_SHA256": "bed95c22e6e6adf4ccf01cabb5156b4888ccc5744f0e192d24501e6df3c19ef3"}
	t.Setenv("HS_NATIVE_PVP_PYTHON", python)
	for key, value := range defaults {
		if actual := os.Getenv(key); actual != "" {
			value = actual
		}
		t.Setenv("HS_NATIVE_PVP_"+key[len("HS_NATIVE_PVP_TEST_"):], value)
	}
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC)
	svc := pgSocialService(t, store)
	svc.Now = func() time.Time { return now }
	infos := []game.ClientInfo{}
	identities := []game.Identity{}
	for index := 0; index < 2; index++ {
		info := game.ClientInfo{Account: fmt.Sprintf("单端原生实库%d", index), Password: "pw", Hostnum: 1}
		identity, e := store.Register(ctx, info)
		if e != nil {
			t.Fatal(e)
		}
		infos = append(infos, info)
		identities = append(identities, identity)
	}
	ownOID, enemyOID := identities[0].Avatars[0].OID, identities[1].Avatars[0].OID
	id, enemyID := hex.EncodeToString(ownOID), hex.EncodeToString(enemyOID)
	_, err = store.UpdateSocial(ctx, []string{id, enemyID}, func(avatars map[string]*game.Avatar) error {
		for _, av := range avatars {
			p := &av.Progress
			p.UnlockSystems["panel_async_pvp"] = 1
			p.Materials[26] = game.Material{ID: 26, Count: 5, Total: 5}
			p.Lineup = []string{p.Cards[0].UUID}
			p.AsyncPvp.Defence = append([]string(nil), p.Lineup...)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	c := pgSocialLogin(t, svc, infos[0], ownOID)
	if asynchronous {
		pgSocialCall(t, svc, c, "enter_asyn_pvp")
		pgSocialCall(t, svc, c, "enter_dungeon", 0, 1, map[string]any{"asyn_pvp_eid": enemyID})
	} else {
		pgSocialCall(t, svc, c, "start_sync_pvp_match", 0)
		now = now.Add(30 * time.Second)
		pgHumanTick(t, svc, c)
		pgSocialCall(t, svc, c, "send_pvp_cards", 0)
	}
	read := func() game.Progress { t.Helper(); return pgHumanRead(t, New(store.pool), ownOID).Progress }
	layout := game.BattleLayout{Fighting: read().Lineup, Support: []string{}}
	if !asynchronous {
		m := read().SyncPvpMatch
		layout = game.BattleLayout{Fighting: m.OwnFighting, Support: append([]string{}, m.OwnSupport...)}
	}
	load := func() {
		t.Helper()
		pgSocialCall(t, svc, c, "load_entity_finish")
		pgSocialCall(t, svc, c, "battle_fighting", map[string]any{"fighting_cards": layout.Fighting, "support_cards": layout.Support})
	}
	uuid := read().Battle.UUID
	pgNativeEvent(t, svc, c, uuid, 1, 1, "ready", map[string]any{"version": 1, "bridge_revision": 31})
	load()
	r := read().Battle.NativeSolo
	if r == nil || len(r.Native.Clients) != 1 || r.Native.Journal.Head == "" {
		t.Fatal("PG没有单端原生冻结状态")
	}
	seq, generation := int64(1), int64(1)
	event := func(kind string, data map[string]any) {
		t.Helper()
		seq++
		pgNativeEvent(t, svc, c, uuid, generation, seq, kind, data)
	}
	event("started", map[string]any{"units": pgNativeUnits(t, r, id)})
	snapshot := func() string {
		t.Helper()
		var state string
		if e := store.pool.QueryRow(ctx, `SELECT state::text||revision::text||updated_at::text FROM avatar_progress WHERE avatar_oid=$1`, ownOID).Scan(&state); e != nil {
			t.Fatal(e)
		}
		return state
	}
	before := snapshot()
	_, err = svc.Handle(ctx, c, "do_command", pgSocialArgs("__battle_event__", []any{map[string]any{"battle_uuid": uuid, "generation": generation, "sequence": seq + 1, "kind": "result", "data": map[string]any{"winner_eids": []string{id}}}}))
	if err == nil || snapshot() != before {
		t.Fatal("PG提前伪胜方没有完整零写入回滚", err)
	}
	seen := 0
	ack := func() {
		t.Helper()
		r := read().Battle.NativeSolo
		for _, out := range r.Native.Outbound {
			if out.Index <= seen || out.Generation != generation {
				continue
			}
			seen = out.Index
			if out.Window {
				continue
			}
			eid, command, round, e := nativeengine.PlaybackExpectation(&out.Playback)
			if e != nil {
				t.Fatal(e)
			}
			event("command", map[string]any{"eid": eid, "command": command})
			event("input", map[string]any{"eid": round})
		}
	}
	ack()
	for turn := 0; turn < 4 && len(read().Battle.NativeSolo.Native.Canonical) == 0; turn++ {
		now = now.Add(30 * time.Second)
		pgHumanTick(t, svc, c)
		ack()
	}
	p := read()
	head := p.Battle.NativeSolo.Native.Journal.Head
	ticket := p.Materials[26].Count
	if len(p.Battle.NativeSolo.Native.Canonical) == 0 {
		t.Fatal("真实PG没有已提交的规范命令")
	}
	// 新Service与新连接模拟进程缓存消失；数据库JSONB重排不能破坏链哈希。
	svc.Detach(c)
	svc = pgSocialService(t, New(store.pool))
	svc.Now = func() time.Time { return now }
	c = pgSocialLogin(t, svc, infos[0], ownOID)
	pgSocialCall(t, svc, c, "client_need_recover_battle")
	p = read()
	if p.Battle.UUID != uuid || p.Battle.NativeSolo.Native.Journal.Head != head || p.Materials[26].Count != ticket {
		t.Fatal("真实PG冷恢复改Journal/UUID或重复扣票")
	}
	generation = 2
	seq = 0
	seen = 0
	load()
	event("ready", map[string]any{"version": 1, "bridge_revision": 31})
	event("started", map[string]any{"units": pgNativeUnits(t, read().Battle.NativeSolo, id)})
	ack()
	event("settings", map[string]any{"auto_battle": true})
	for turn := 0; turn < 120; turn++ {
		if read().Battle.NativeSolo.Native.Update.Result != nil {
			break
		}
		now = now.Add(30 * time.Second)
		pgHumanTick(t, svc, c)
		ack()
	}
	r = read().Battle.NativeSolo
	if r.Native.Update.Result == nil || len(r.Native.Winners) != 1 {
		t.Fatal("真实PG单端原生未结束")
	}
	data := map[string]any{"winner_eids": r.Native.Winners}
	event("result", data)
	if !read().Battle.Finished {
		t.Fatal("PG末态没有唯一结算")
	}
	before = pgNativeJSON(read())
	pgNativeEvent(t, svc, c, uuid, generation, seq, "result", data)
	pgSocialCall(t, svc, c, "client_need_recover_battle")
	if pgNativeJSON(read()) != before {
		var a, b map[string]json.RawMessage
		_ = json.Unmarshal([]byte(before), &a)
		_ = json.Unmarshal([]byte(pgNativeJSON(read())), &b)
		for key, value := range a {
			if string(value) != string(b[key]) {
				t.Logf("变化字段%s：之前=%s；之后=%s", key, value, b[key])
			}
		}
		t.Fatal("PG重复result或原收据恢复改变完整进度")
	}
	if asynchronous && len(pgHumanRead(t, store, enemyOID).Progress.AsyncPvp.DefenceRecords) != 1 {
		t.Fatal("离线防守者PG战绩没有同事务持久")
	}
	t.Logf("单端真实PG：异步=%v，Journal=%d，规范命令=%d", asynchronous, len(r.Native.Journal.Entries), len(r.Native.Canonical))
}
