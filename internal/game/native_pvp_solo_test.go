package game

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hs-server/internal/nativeengine"
)

func enableSoloTestRuntime(t *testing.T, s *Service) {
	t.Helper()
	python := os.Getenv("HS_NATIVE_PVP_TEST_PYTHON2")
	if python == "" {
		t.Skip("单端真实原生引擎须显式启用隔离Python2")
	}
	root, err := filepath.Abs("../nativepvp")
	if err != nil {
		t.Fatal(err)
	}
	s.nativePvpConfig = &nativeengine.Config{Python: python, Worker: filepath.Join(root, "battle_native_worker.py"), Directory: filepath.Join(root, "runtime/native_engine")}
	s.nativePvpResource = "bed95c22e6e6adf4ccf01cabb5156b4888ccc5744f0e192d24501e6df3c19ef3"
}

func TestNativeSoloPvpActualDuelRecoveryAndForgeryRollback(t *testing.T) {
	for _, kind := range []string{"异步真人防守", "异步机器人", "同步机器人"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			s, cs, store := socialTestWorld(t)
			c := cs[0]
			enableSoloTestRuntime(t, s)
			now := s.Now()
			s.Now = func() time.Time { return now }
			id := hexOf(selectedOID(c))
			call := func(method string, args ...any) []Push {
				t.Helper()
				pushes, err := s.Handle(ctx, c, method, socialArgs(args...))
				if err != nil {
					t.Fatalf("%s失败：%v", method, err)
				}
				return pushes
			}
			if kind != "同步机器人" {
				call("enter_asyn_pvp")
				target := hexOf(selectedOID(cs[1]))
				if kind == "异步机器人" {
					for candidateID, candidate := range c.SelectedAvatarUnsafe().Progress.AsyncPvp.Candidates {
						if candidate.Robot {
							target = candidateID
							break
						}
					}
					if target == hexOf(selectedOID(cs[1])) {
						t.Fatal("没有实际原生机器人候选")
					}
				}
				call("enter_dungeon", 0, 1, map[string]any{"asyn_pvp_eid": target})
			} else {
				call("start_sync_pvp_match", 0)
				now = now.Add(30 * time.Second)
				if _, err := s.Tick(ctx, c); err != nil {
					t.Fatal(err)
				}
				call("send_pvp_cards", 0)
			}
			uuid := c.SelectedAvatarUnsafe().Progress.Battle.UUID
			call("client_need_recover_battle")
			if c.SelectedAvatarUnsafe().Progress.Battle.UUID != uuid {
				t.Fatal("冻结前恢复改变原UUID")
			}
			call("do_command", battleEventCommand, []any{map[string]any{"battle_uuid": uuid, "generation": 1, "sequence": 1, "kind": "ready", "data": map[string]any{"version": 1, "bridge_revision": 31}}})
			call("load_entity_finish")
			layout := BattleLayout{Fighting: append([]string(nil), c.SelectedAvatarUnsafe().Progress.Lineup...)}
			if kind == "同步机器人" {
				m := c.SelectedAvatarUnsafe().Progress.SyncPvpMatch
				layout = BattleLayout{Fighting: m.OwnFighting, Support: m.OwnSupport}
			}
			if layout.Support == nil {
				layout.Support = []string{}
			}
			call("battle_fighting", map[string]any{"fighting_cards": layout.Fighting, "support_cards": layout.Support})
			r := nativeSoloRoom(c)
			if r == nil || len(r.Native.Clients) != 1 || r.Native.Journal.Head == "" {
				t.Fatal("单端冻结原生权威未创建")
			}
			defer s.closeNativeAuthority(uuid)
			seq := int64(1)
			generation := int64(1)
			event := func(kind string, data map[string]any) {
				t.Helper()
				seq++
				call("do_command", battleEventCommand, []any{map[string]any{"battle_uuid": uuid, "generation": generation, "sequence": seq, "kind": kind, "data": data}})
			}
			event("started", map[string]any{"units": nativeRoomTestUnits(t, nativeSoloRoom(c), id)})
			rawState := func() string { data, _ := json.Marshal(socialSaved(t, store, c).Progress); return string(data) }
			before := rawState()
			_, err := s.Handle(ctx, c, "do_command", socialArgs(battleEventCommand, []any{map[string]any{"battle_uuid": uuid, "generation": generation, "sequence": seq + 1, "kind": "result", "data": map[string]any{"winner_eids": []string{id}, "player_eid": id, "outcome": "win"}}}))
			if err == nil || rawState() != before {
				t.Fatal("客户端提前伪胜方未零写入拒绝", err)
			}
			seen := 0
			ack := func() {
				t.Helper()
				r := nativeSoloRoom(c)
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
			before = rawState()
			_, err = s.Handle(ctx, c, "do_command", socialArgs("use_skill", []any{"ck_monster", "不存在", 440301, "不存在"}))
			if err == nil || rawState() != before {
				t.Fatal("非法原生点击改变持久状态", err)
			}
			// 真正执行原生动作但故意回滚玩家事务，下一次按已提交Journal重建。
			head := nativeSoloRoom(c).Native.Journal.Head
			err = s.updateProgress(ctx, c, func(p *Progress) error {
				if e := s.advanceNativeUntilBoundary(ctx, p.Battle.NativeSolo, map[string]any{"operation": "timeout"}); e != nil {
					return e
				}
				return errors.New("故意拒绝提交")
			})
			if err == nil || rawState() != before || nativeSoloRoom(c).Native.Journal.Head != head {
				t.Fatal("未提交原生动作没有完整回滚", err)
			}
			for turn := 0; turn < 4 && len(nativeSoloRoom(c).Native.Canonical) == 0; turn++ {
				now = now.Add(30 * time.Second)
				if _, err = s.Tick(ctx, c); err != nil {
					t.Fatal(err)
				}
				ack()
			}
			if len(nativeSoloRoom(c).Native.Canonical) == 0 {
				t.Fatal("没有实际规范播放命令")
			}
			ticket := c.SelectedAvatarUnsafe().Progress.Materials[26].Count
			head = nativeSoloRoom(c).Native.Journal.Head
			s.closeNativeAuthority(uuid)
			call("client_need_recover_battle")
			if nativeSoloRoom(c).UUID != uuid || nativeSoloRoom(c).Native.Journal.Head != head || c.SelectedAvatarUnsafe().Progress.Materials[26].Count != ticket {
				t.Fatal("恢复改变UUID、Journal或重复扣票")
			}
			generation = 2
			seq = 0
			seen = 0
			call("load_entity_finish")
			call("battle_fighting", map[string]any{"fighting_cards": layout.Fighting, "support_cards": layout.Support})
			event("ready", map[string]any{"version": 1, "bridge_revision": 31})
			event("started", map[string]any{"units": nativeRoomTestUnits(t, nativeSoloRoom(c), id)})
			ack()
			event("settings", map[string]any{"auto_battle": true})
			for step := 0; step < 120; step++ {
				if nativeSoloRoom(c).Native.Update.Result != nil {
					break
				}
				now = now.Add(30 * time.Second)
				if _, err = s.Tick(ctx, c); err != nil {
					t.Fatal(err)
				}
				ack()
			}
			r = nativeSoloRoom(c)
			if r.Native.Update.Result == nil || len(r.Native.Winners) != 1 {
				t.Fatal("真实单端PVP未到唯一末态")
			}
			winners := append([]string(nil), r.Native.Winners...)
			outcome := "loss"
			if winners[0] == id {
				outcome = "win"
			}
			data := map[string]any{"winner_eids": winners, "player_eid": id, "outcome": outcome}
			event("result", data)
			if !c.SelectedAvatarUnsafe().Progress.Battle.Finished {
				t.Fatal("原生末态未结算")
			}
			before = rawState()
			call("do_command", battleEventCommand, []any{map[string]any{"battle_uuid": uuid, "generation": generation, "sequence": seq, "kind": "result", "data": data}})
			forged := map[string]any{"winner_eids": []string{r.IDs[1]}, "player_eid": id, "outcome": "loss", "伪造": true}
			if _, err := s.Handle(ctx, c, "do_command", socialArgs(battleEventCommand, []any{map[string]any{"battle_uuid": uuid, "generation": generation, "sequence": seq, "kind": "result", "data": forged}})); err == nil || rawState() != before {
				t.Fatal("结算后篡改原result没有零写入拒绝", err)
			}
			call("client_need_recover_battle")
			if rawState() != before {
				t.Fatal("结果重试或结算恢复重复资产/积分")
			}
			if kind == "异步真人防守" && len(socialSaved(t, store, cs[1]).Progress.AsyncPvp.DefenceRecords) != 1 {
				t.Fatal("离线真人防守者没有同事务战绩")
			}
			t.Logf("单端原生对局：%s；Journal=%d，规范命令=%d，恢复世代=%d", kind, len(r.Native.Journal.Entries), len(r.Native.Canonical), generation)
		})
	}
}
