package dbstore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/game"
)

// 在实际结算事务执行完成后注入回滚，第一笔冻结事务仍真实提交。
type battleSettlementRollbackStore struct {
	*Store
	writes int
}

func (s *battleSettlementRollbackStore) UpdateProgress(ctx context.Context, oid []byte, fn func(*game.Progress) error) (game.Progress, error) {
	s.writes++
	rollback := s.writes == 2
	return s.Store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		if err := fn(p); err != nil {
			return err
		}
		if rollback {
			return context.DeadlineExceeded
		}
		return nil
	})
}

func TestPostgresBattleFrozenRandomRollbackColdLoginCrossDay(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	info := game.ClientInfo{Account: "战斗冻结实库", Password: "pw", Hostnum: 1}
	s, c, av := repairPlayer(t, store, info, &now)
	defer s.Detach(c)
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.AvatarLevel = 60
		p.GuideTasks = map[int]game.GuideTask{}
		p.Power.Value = 100
		p.ClearedDungeons = append(p.ClearedDungeons, 509)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pgRemainingRPC(t, s, c, "enter_dungeon", 1, 6101, map[string]any{})
	pgRemainingRPC(t, s, c, "load_entity_finish")
	pgRemainingRPC(t, s, c, "battle_fighting", map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})
	uuid := c.SelectedAvatarUnsafe().Progress.Battle.UUID
	emit := func(seq int, kind string, data map[string]any) {
		pgRemainingRPC(t, s, c, "do_command", "__battle_event__", []any{map[string]any{"battle_uuid": uuid, "sequence": seq, "kind": kind, "data": data}})
	}
	emit(1, "ready", map[string]any{"version": 1})
	emit(2, "started", map[string]any{"units": []any{map[string]any{"eid": "1", "role": 1, "hex": []int{0, 0, 0}, "kind": "ally"}}})
	before := game.CloneProgress(c.SelectedAvatarUnsafe().Progress)
	s.Accounts = &battleSettlementRollbackStore{Store: store}
	emit(3, "result", map[string]any{"winner_eids": []string{hex.EncodeToString(av.OID)}})
	read := func() game.Progress {
		id, err := New(store.pool).QuickLogin(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		return id.Avatars[0].Progress
	}
	frozen := read()
	if !frozen.Battle.RewardPlanFrozen || frozen.Battle.Finished || !reflect.DeepEqual(before.Runes, frozen.Runes) {
		t.Fatal("实库冻结未提交或回滚泄漏资产")
	}
	expected := map[string]game.Rune{}
	for _, r := range frozen.Battle.RewardBonusPlan {
		for id, rune := range r.Runes {
			expected[id] = rune
		}
	}
	if len(expected) != 4 {
		t.Fatal("原表星期五四件契印收据未冻结", len(expected))
	}
	s.Detach(c)
	now = now.Add(24 * time.Hour)
	coldSvc, cold := loginPersistedProgressPlayer(t, New(store.pool), info, now)
	defer coldSvc.Detach(cold)
	loginOut, err := coldSvc.Handle(ctx, cold, "set_reconnect_auth_msg", remainingPGArgs(t, "冻结冷登录收尾"))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range loginOut {
		if item.Method == "battle_result" || item.Method == "start_server_battle_ok" {
			t.Fatal("冷登录向不存在的战斗场景推送结果")
		}
	}
	settled := read()
	if !settled.Battle.Finished || settled.Battle.UUID != uuid {
		t.Fatal("冷登录重开已抽奖战斗")
	}
	for id, rune := range expected {
		if !reflect.DeepEqual(rune, settled.Runes[id]) {
			t.Fatal("跨日冷登录重新抽奖")
		}
	}
	pgRemainingRPC(t, coldSvc, cold, "client_need_recover_battle")
	if !reflect.DeepEqual(settled.Runes, read().Runes) {
		t.Fatal("结果恢复重复发奖")
	}
}

func TestPostgresBattleOverflowFullMailboxColdClaim(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	info := game.ClientInfo{Account: "溢出邮箱实库", Password: "pw", Hostnum: 1}
	s, c, av := repairPlayer(t, store, info, &now)
	defer s.Detach(c)
	// 固定收据夹具仅验证已冻结结果的兑现链，不作为原版副本掉落证据。
	uuid := "00112233445566778899aabb"
	data := map[string]any{"winner_eids": []string{hex.EncodeToString(av.OID)}}
	event := map[string]any{"battle_uuid": uuid, "sequence": 3, "kind": "result", "data": data}
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.GuideTasks = map[int]game.GuideTask{}
		p.Materials[29] = game.Material{ID: 29, Count: 998, Total: 998}
		p.ShortMailInfo = map[int]game.Mail{}
		for i := 1; i <= 500; i++ {
			p.ShortMailInfo[i] = game.Mail{MID: i, State: game.MailFinal, Title: "已读验收邮件"}
		}
		body, _ := json.Marshal(map[string]any{"uuid": uuid, "dungeon_id": 10001, "bridge_ready": true, "bridge_started": true, "last_sequence": 2, "reward_plan_frozen": true, "reward_planned_at": now.Unix(), "reward_result": event, "reward_bonus_plan": []game.BattleBonusReceipt{{BonusID: 200305, Amount: 1, Materials: map[int]int64{29: 100}}}})
		return json.Unmarshal(body, &p.Battle)
	}); err != nil {
		t.Fatal(err)
	}
	s.Detach(c)
	coldSvc, cold := loginPersistedProgressPlayer(t, New(store.pool), info, now)
	defer coldSvc.Detach(cold)
	pgRemainingRPC(t, coldSvc, cold, "set_reconnect_auth_msg", "溢出冷登录收尾")
	read := func() game.Progress {
		id, err := New(store.pool).QuickLogin(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		return id.Avatars[0].Progress
	}
	p := read()
	if !p.Battle.Finished || p.Materials[29].Count != 1000 || len(p.BattleOverflowMailbox) != 1 || len(p.ShortMailInfo) != 500 {
		t.Fatal("满邮箱阻塞结算或丢失奖励")
	}
	pgRemainingRPC(t, coldSvc, cold, "delete_mail", 1, 1)
	p = read()
	if len(p.BattleOverflowMailbox) != 0 || len(p.ShortMailInfo) != 500 {
		t.Fatal("删除邮件未投递暂存奖励")
	}
	var mail game.Mail
	for _, m := range p.ShortMailInfo {
		if m.Attachments[29] == 98 {
			mail = m
		}
	}
	if mail.MID == 0 || mail.ExpiresAt != 0 {
		t.Fatal("溢出邮件丢失或会过期")
	}
	pgRemainingRPC(t, coldSvc, cold, "receive_attachment", 2, mail.UUID)
	if !reflect.DeepEqual(p.Materials, read().Materials) || read().ShortMailInfo[mail.MID].State == game.MailFinal {
		t.Fatal("满容量领取部分入账")
	}
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error { p.Materials[29] = game.Material{ID: 29, Total: 1000}; return nil }); err != nil {
		t.Fatal(err)
	}
	pgRemainingRPC(t, coldSvc, cold, "receive_attachment", 3, mail.UUID)
	p = read()
	if p.Materials[29].Count != 98 || p.ShortMailInfo[mail.MID].State != game.MailFinal {
		t.Fatal("释放容量未领取")
	}
	_, _ = coldSvc.Handle(ctx, cold, "receive_attachment", remainingPGArgs(t, 4, mail.UUID))
	if !reflect.DeepEqual(p.Materials, read().Materials) {
		t.Fatal("重复领取再次发奖")
	}
}
