package dbstore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestPostgresApprovedRunePolicyMainline102FirstClearAndColdRecovery(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	var pools map[string][][2]int
	if err := json.Unmarshal([]byte(os.Getenv("HS_RUNE_FALLBACK_POOLS")), &pools); err != nil || len(pools) != 17 || len(pools["400"]) != 18 {
		t.Fatal("真实库进程没有绑定获批准的正式17池规则", err)
	}
	for _, rows := range pools {
		for _, row := range rows {
			if row[1] != 1 {
				t.Fatal("真实库运营分布不是获批准的组内等概率")
			}
		}
	}
	now := time.Date(2026, 10, 8, 1, 0, 0, 0, time.UTC)
	info := game.ClientInfo{Account: "契印首通实库", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, store, info, &now)
	defer svc.Detach(c)
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		for id, task := range p.GuideTasks {
			task.Status = 2
			p.GuideTasks[id] = task
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := New(store.pool).AdminPlayer(ctx, av.OID)
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []struct {
		method string
		args   []json.RawMessage
	}{
		{"enter_dungeon", pgSocialArgs(9, 102, map[string]any{})},
		{"load_entity_finish", nil},
		{"battle_fighting", pgSocialArgs(map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})},
	} {
		if _, err := svc.Handle(ctx, c, call.method, call.args); err != nil {
			t.Fatal(call.method, err)
		}
	}
	selected, ok := c.SelectedAvatar()
	if !ok || selected.Progress.Battle == nil {
		t.Fatal("真实库没有创建102战斗会话")
	}
	uuid := selected.Progress.Battle.UUID
	emit := func(seq int, kind string, data map[string]any) []game.Push {
		t.Helper()
		pushes, err := svc.Handle(ctx, c, "do_command", pgSocialArgs("__battle_event__", []any{map[string]any{"battle_uuid": uuid, "sequence": seq, "kind": kind, "data": data}}))
		if err != nil {
			t.Fatal("实际102原生观察事件结算失败", kind, err)
		}
		return pushes
	}
	emit(1, "ready", map[string]any{"version": 1})
	emit(2, "started", map[string]any{"units": []any{map[string]any{"eid": "1", "role": 1, "hex": []int{0, 0, 0}, "kind": "ally"}}})
	final := map[string]any{"winner_eids": []string{hex.EncodeToString(av.OID)}}
	pushes := emit(3, "result", final)
	found := false
	for _, p := range pushes {
		if p.Method == "battle_result" {
			found = true
		}
	}
	if !found {
		t.Fatal("主线102首通未返回原生结果")
	}
	saved, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := saved.Avatars[0].Progress
	if len(p.Runes) != len(before.Progress.Runes)+2 || p.Materials[12].Count-before.Progress.Materials[12].Count != 20000 || !p.Battle.RewardGranted {
		t.Fatal("102首通契印/金币/收据未整笔提交")
	}
	emit(3, "result", final)
	after, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil || !reflect.DeepEqual(p.Runes, after.Avatars[0].Progress.Runes) || p.Materials[12].Count != after.Avatars[0].Progress.Materials[12].Count {
		t.Fatal("102结果重试重复发奖", err)
	}
	reloaded, reconnected := loginPersistedProgressPlayer(t, store, info, now)
	pushes, err = reloaded.Handle(ctx, reconnected, "client_need_recover_battle", nil)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, push := range pushes {
		found = found || push.Method == "battle_result"
	}
	if !found {
		t.Fatal("冷重登录无法恢复已提交102原盒")
	}
	last, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil || !reflect.DeepEqual(p.Runes, last.Avatars[0].Progress.Runes) || p.Materials[12].Count != last.Avatars[0].Progress.Materials[12].Count {
		t.Fatal("恢复原盒重新抽取或发奖", err)
	}
}
