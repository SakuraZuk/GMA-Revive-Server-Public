package dbstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"hs-server/internal/game"
	"testing"
	"time"
)

type feedbackWriteStore struct {
	*Store
	writes int
}

func (s *feedbackWriteStore) UpdateProgress(ctx context.Context, oid []byte, fn func(*game.Progress) error) (game.Progress, error) {
	s.writes++
	return s.Store.UpdateProgress(ctx, oid, fn)
}

func TestPostgresPlayerFeedbackAscendReturnAndReportedDrawPools(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 10, 0, 0, 0, time.FixedZone("北京时间", 28800))
	if err := game.SetActivitySchedules(game.DefaultPermanentActivitySchedules(now.Add(-24 * time.Hour))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = game.SetActivitySchedules(nil) })
	info := game.ClientInfo{Account: "玩家反馈实库", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, store, info, &now)
	defer svc.Detach(c)
	card := game.Card{UUID: "00112233445566778899aabb", CardID: 2401, Level: 2, EnhanceCount: 2, Dress: 240101}
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.AvatarLevel = 60
		p.ClearedDungeons = append(p.ClearedDungeons, 509, 4101)
		for id, task := range p.GuideTasks {
			task.Status = 2
			p.GuideTasks[id] = task
		}
		p.Materials[500] = game.Material{ID: 500, Count: 20, Total: 20}
		p.Materials[501] = game.Material{ID: 501, Count: 20, Total: 20}
		p.Cards = append(p.Cards, card)
		if p.Runes == nil {
			p.Runes = map[string]game.Rune{}
		}
		p.Runes["00112233445566778899aabc"] = game.Rune{UUID: "00112233445566778899aabc", CardUUID: card.UUID}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, pool := range []int{2, 109, 122, 501, 601} {
		out, err := svc.Handle(ctx, c, "random_cards", pgSocialArgs(1, pool, 1))
		if err != nil || len(out) == 0 || out[len(out)-1].Args[1].([]any)[0] != 0 {
			t.Fatal("上报卡池实库抽卡失败", pool, err, out)
		}
	}
	read := func() game.Progress {
		id, err := New(store.pool).QuickLogin(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		return id.Avatars[0].Progress
	}
	before := read()
	pgRemainingRPC(t, svc, c, "set_card_vo", 81, 4401, 1)
	pgRemainingRPC(t, svc, c, "set_show_cards", 82, []any{card.UUID, nil, nil})
	pgRemainingRPC(t, svc, c, "set_girl_random_enable", 83, false)
	pgRemainingRPC(t, svc, c, "set_explore_auto_agent", 84, 208, true)
	pgRemainingRPC(t, svc, c, "change_dungeon_skip_edit_state", 85, 4102, true)
	preferences := read()
	if preferences.CardVoices[4401] != 1 || preferences.GirlRandomEnable == nil || *preferences.GirlRandomEnable || !preferences.ExploreAutoAgent[208] || !preferences.DungeonSkipEditState[4102] {
		t.Fatal("原生偏好实际RPC未在实库持久")
	}
	if _, err := svc.Handle(ctx, c, "decompose_cards", pgSocialArgs(2, []string{card.UUID})); err != nil {
		t.Fatal(err)
	}
	after := read()
	if len(after.ShowCards) != 3 || after.ShowCards[0] != "" {
		t.Fatal("归还后展示槽残留已删除卡UUID")
	}
	if after.Materials[100].Count-before.Materials[100].Count != 10 || after.Materials[101].Count-before.Materials[101].Count != 10 || len(after.Cards) != len(before.Cards)-1 || after.Runes["00112233445566778899aabc"].CardUUID != "" {
		t.Fatal("归还材料/删卡/卸印未原子保存")
	}
	if _, err := svc.Handle(ctx, c, "decompose_cards", pgSocialArgs(3, []string{card.UUID})); err != nil {
		t.Fatal(err)
	}
	if read().Materials[100].Count != after.Materials[100].Count {
		t.Fatal("归还重放重复发料")
	}
	pgRemainingRPC(t, svc, c, "enter_dungeon", 4, 4102, map[string]any{})
	pgRemainingRPC(t, svc, c, "load_entity_finish")
	pgRemainingRPC(t, svc, c, "battle_fighting", map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})
	uuid := c.SelectedAvatarUnsafe().Progress.Battle.UUID
	counted := &feedbackWriteStore{Store: store}
	svc.Accounts = counted
	send := func(seq int, kind string, data map[string]any) []game.Push {
		return pgRemainingRPC(t, svc, c, "do_command", "__battle_event__", []any{map[string]any{"battle_uuid": uuid, "sequence": seq, "kind": kind, "data": data}})
	}
	send(1, "ready", map[string]any{"version": 1})
	send(2, "started", map[string]any{"units": []any{}})
	writes := counted.writes
	for seq := 3; seq <= 302; seq++ {
		send(seq, "round_end", map[string]any{"units": []any{}})
	}
	if counted.writes != writes || read().Battle.LastSequence != 2 {
		t.Fatal("实库仍逐观察事件重写JSONB", counted.writes, writes)
	}
	pgRemainingRPC(t, svc, c, "client_need_recover_battle")
	if c.SelectedAvatarUnsafe().Progress.Battle.UUID != uuid {
		t.Fatal("同连接慢心跳重开")
	}
	out := send(303, "result", map[string]any{"winner_eids": []any{hex.EncodeToString(av.OID)}})
	found := false
	for _, v := range out {
		found = found || v.Method == "battle_result"
	}
	if !found || !read().Battle.Finished || read().Battle.LastSequence != 303 {
		t.Fatal("升格二层长事件结算未持久")
	}
	receipt := read()
	// 真实新连接鉴权、角色绑定和材料副本完成态冷登录收尾。
	coldService := pgActivityService(t, store, func() time.Time { return now })
	_ = pgActivityLogin(t, coldService, info)
	if read().Battle.UUID != receipt.Battle.UUID || read().Materials[100].Count != receipt.Materials[100].Count {
		t.Fatal("完成态冷登录改写战斗或重复发升格材料")
	}
	send(303, "result", map[string]any{"winner_eids": []any{hex.EncodeToString(av.OID)}})
	if read().Materials[100].Count != receipt.Materials[100].Count {
		t.Fatal("升格重复结算发料")
	}
	for _, pool := range []int{2, 109, 122, 501, 601} {
		if read().RandomCardsRecord[pool].RandomCount != 1 {
			t.Fatal("卡池计数重登录丢失", pool)
		}
	}
	// 真正connect_server(type=1)的Resume入口，新连接没有原连接观察缓存。
	pgRemainingRPC(t, svc, c, "enter_dungeon", 5, 4102, map[string]any{})
	pgRemainingRPC(t, svc, c, "load_entity_finish")
	pgRemainingRPC(t, svc, c, "battle_fighting", map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})
	uuid = c.SelectedAvatarUnsafe().Progress.Battle.UUID
	send(1, "ready", map[string]any{"version": 1})
	send(2, "started", map[string]any{"units": []any{}})
	send(3, "round_end", map[string]any{"units": []any{}})
	token := []byte("实库暖TCP重连凭据")
	digest := sha256.Sum256(token)
	if err := store.SaveReconnect(ctx, av.OID, "反馈设备", digest[:], now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	warm := game.NewConnection()
	_, replay, err := svc.Resume(ctx, warm, av.OID, "反馈设备", token)
	if err != nil || len(replay) < 4 || warm.SelectedAvatarUnsafe().Progress.Battle.UUID == uuid || read().Battle.LastSequence != 0 {
		t.Fatal("真实TCP重连续用了丢帧旧序号", err, replay)
	}
	if replay[1].Args[3].(map[string]any)["hs_recover_previous_uuid"] != uuid {
		t.Fatal("真实TCP恢复没有授权清场标记")
	}
}
