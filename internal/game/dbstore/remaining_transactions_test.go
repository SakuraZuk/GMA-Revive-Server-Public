package dbstore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"hs-server/internal/game"
	"math"
	"reflect"
	"testing"
	"time"
)

func remainingPGArgs(t *testing.T, values ...any) []json.RawMessage {
	t.Helper()
	result := []json.RawMessage{}
	for _, v := range values {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, raw)
	}
	return result
}

func TestPostgresRemainingMailRandomFrozenAtIssueClaimRollbackAndColdRetry(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 8, 5, 0, 0, 0, time.UTC)
	info := game.ClientInfo{Account: "随机邮件验收", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, store, info, &now)
	defer svc.Detach(c)
	before := game.CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if err := svc.IssueMail(ctx, c, game.Mail{MID: 601, Title: "随机真实冻结", Attachments: map[int]int64{921: 1, 12: 7}}); err != nil {
		t.Fatal(err)
	}
	frozen, err := New(store.pool).UpdateProgress(ctx, av.OID, func(*game.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	mail := frozen.ShortMailInfo[601]
	if !mail.RewardsFrozen || mail.SourceAttachments[921] != 1 || len(mail.Cards) != 1 || !reflect.DeepEqual(before.Cards, frozen.Cards) || !reflect.DeepEqual(before.Materials, frozen.Materials) {
		t.Fatal("PG签发提前入账或没有冻结真实随机卡", mail)
	}
	if _, err = store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Materials[12] = game.Material{ID: 12, Count: math.MaxInt64, Total: math.MaxInt64}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err = New(store.pool).UpdateProgress(ctx, av.OID, func(*game.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.Handle(ctx, c, "receive_attachment", remainingPGArgs(t, 1, mail.UUID))
	after, err := New(store.pool).UpdateProgress(ctx, av.OID, func(*game.Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("PG随机卡已发但金币溢出没有整盒回滚", err)
	}
	if _, err = store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error { p.Materials[12] = frozen.Materials[12]; return nil }); err != nil {
		t.Fatal(err)
	}
	svc.Detach(c)
	cold, coldC := loginPersistedProgressPlayer(t, New(store.pool), info, now)
	defer cold.Detach(coldC)
	if _, err = cold.Handle(ctx, coldC, "receive_attachment", remainingPGArgs(t, 2, mail.UUID)); err != nil {
		t.Fatal(err)
	}
	claimed, err := New(store.pool).UpdateProgress(ctx, av.OID, func(*game.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	saved := claimed.ShortMailInfo[601]
	if saved.State != game.MailFinal || !reflect.DeepEqual(mail.Cards, saved.Cards) || !reflect.DeepEqual(mail.Attachments, saved.Attachments) || len(claimed.Cards) != len(before.Cards)+1 || claimed.Cards[len(claimed.Cards)-1].UUID != mail.Cards[0].UUID {
		t.Fatal("PG冷重连重新随机或未发原冻结卡")
	}
	_, _ = cold.Handle(ctx, coldC, "receive_attachment", remainingPGArgs(t, 3, mail.UUID))
	after, err = New(store.pool).UpdateProgress(ctx, av.OID, func(*game.Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(claimed, after) {
		t.Fatal("PG随机邮件重复领取有变动", err)
	}
}

func TestPostgresRemainingStrangerAssistRosterLimitPairAndFrozenRetry(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 8, 5, 0, 0, 0, time.UTC)
	svc := pgSocialService(t, store)
	svc.Now = func() time.Time { return now }
	avatars := []game.Avatar{}
	conns := []*game.Connection{}
	for _, name := range []string{"陌生助战调用者", "陌生助战提供者"} {
		info := game.ClientInfo{Account: name, Password: "pw", Hostnum: 1}
		id, err := store.Register(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		av := id.Avatars[0]
		avatars = append(avatars, av)
		if _, err = store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
			p.UnlockSystems["assist"] = 1
			p.UnlockSystems["support"] = 1
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		conns = append(conns, pgSocialLogin(t, svc, info, av.OID))
	}
	defer svc.Detach(conns[0])
	defer svc.Detach(conns[1])
	provider := hex.EncodeToString(avatars[1].OID)
	uuid := avatars[1].Progress.Cards[0].UUID
	pgSocialCall(t, svc, conns[1], "set_assist_card", 1, uuid)
	_, _ = svc.Handle(ctx, conns[0], "select_assist_card", remainingPGArgs(t, 2, uuid, map[string]any{"eid": provider, "hostnum": 1}))
	p, err := New(store.pool).UpdateProgress(ctx, avatars[0].OID, func(*game.Progress) error { return nil })
	if err != nil || p.Social.Assist.Current != nil {
		t.Fatal("未列入候选的陌生卡获授权", err)
	}
	pgSocialCall(t, svc, conns[0], "refresh_assist_use_times")
	pgSocialCall(t, svc, conns[0], "select_assist_card", 3, uuid, map[string]any{"eid": provider, "hostnum": 1})
	if _, err = store.UpdateProgress(ctx, avatars[0].OID, func(p *game.Progress) error {
		p.Battle = &game.BattleSession{UUID: "112233445566778899aabbcc", DungeonID: 100101, BattleID: 1101, Loaded: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	layout := map[string]any{"fighting_cards": []string{avatars[0].Progress.Cards[0].UUID, uuid}, "support_cards": []string{}, "storyline_cards": []string{}}
	pgSocialCall(t, svc, conns[0], "battle_fighting", layout)
	rows, err := New(store.pool).SocialAvatars(ctx, game.SocialSearch{Hostnum: 1, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	saved := map[string]game.Progress{}
	for _, av := range rows {
		saved[hex.EncodeToString(av.OID)] = game.CloneProgress(av.Progress)
	}
	own := hex.EncodeToString(avatars[0].OID)
	if saved[own].Social.Assist.Active[uuid] != 1 || saved[provider].Social.Assist.Passive != 1 || saved[own].Social.Assist.Frozen == nil || len(saved[own].Cards) != 1 {
		t.Fatal("陌生助战计次、快照或资产隔离失败")
	}
	pgSocialCall(t, svc, conns[0], "battle_fighting", layout)
	rows, err = New(store.pool).SocialAvatars(ctx, game.SocialSearch{Hostnum: 1, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, av := range rows {
		if !reflect.DeepEqual(saved[hex.EncodeToString(av.OID)], av.Progress) {
			t.Fatal("同UUID陌生助战重试写入双方状态")
		}
	}
	if _, err = store.UpdateProgress(ctx, avatars[0].OID, func(p *game.Progress) error {
		p.Battle = &game.BattleSession{UUID: "112233445566778899aabbcd", DungeonID: 100101, BattleID: 1101, Loaded: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p, err = New(store.pool).UpdateProgress(ctx, avatars[0].OID, func(*game.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.Handle(ctx, conns[0], "battle_fighting", remainingPGArgs(t, layout))
	after, err := New(store.pool).UpdateProgress(ctx, avatars[0].OID, func(*game.Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(p, after) {
		t.Fatal("陌生助战每日一次数额拒绝部分写入", err)
	}
}

func TestPostgresRemainingSpecialDrillActualRPCThreeTasksAndColdRetry(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	t.Setenv("HS_SPECIAL_DRILL_REOPEN", "timber-12-20261008")
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 8, 5, 0, 0, 0, time.UTC)
	info := game.ClientInfo{Account: "演练伤害验收", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, store, info, &now)
	defer svc.Detach(c)
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.AvatarLevel = 60
		for id, g := range p.GuideTasks {
			g.Status = 2
			p.GuideTasks[id] = g
		}
		p.Power.Value = 1000
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]int{{2103, 211200}, {2303, 231200}, {2503, 251200}} {
		for _, call := range []struct {
			method string
			values []any
		}{{"enter_dungeon", []any{1, pair[0], map[string]any{}}}, {"load_entity_finish", nil}, {"battle_fighting", []any{map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}}}}} {
			if _, err := svc.Handle(ctx, c, call.method, remainingPGArgs(t, call.values...)); err != nil {
				t.Fatal(err)
			}
		}
		b := c.SelectedAvatarUnsafe().Progress.Battle
		oid := hex.EncodeToString(av.OID)
		event := func(seq int, kind string, data map[string]any) []game.Push {
			payload := map[string]any{"battle_uuid": b.UUID, "sequence": seq, "kind": kind, "data": data}
			pushes, err := svc.Handle(ctx, c, "do_command", remainingPGArgs(t, "__battle_event__", []any{payload}))
			if err != nil {
				t.Fatal(err)
			}
			return pushes
		}
		event(1, "ready", map[string]any{"version": 1})
		event(2, "started", map[string]any{"units": []any{}})
		before, err := New(store.pool).UpdateProgress(ctx, av.OID, func(*game.Progress) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		wrong := 211200
		if pair[1] == wrong {
			wrong = 231200
		}
		data := map[string]any{"winner_eids": []string{oid}, "player_eid": oid, "outcome": "win", "finished_task_list": []int{wrong}}
		if len(event(3, "result", data)) != 0 {
			t.Fatal("PG异副本任务没有零写入拒绝")
		}
		after, err := New(store.pool).UpdateProgress(ctx, av.OID, func(*game.Progress) error { return nil })
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("PG演练错误任务部分提交", err)
		}
		data["finished_task_list"] = []int{pair[1]}
		pushes := event(3, "result", data)
		found := false
		for _, p := range pushes {
			if p.Method == "battle_result" {
				found = true
			}
		}
		if !found {
			t.Fatal("PG真实演练结算缺失")
		}
		before, err = New(store.pool).UpdateProgress(ctx, av.OID, func(*game.Progress) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		achID, tid := 301231, 209112
		if pair[1] == 231200 {
			achID, tid = 301232, 209312
		} else if pair[1] == 251200 {
			achID, tid = 301233, 209512
		}
		if before.Achievements[achID].Targets[tid] != 1 || before.Achievements[achID].Time == 0 {
			t.Fatal("PG实际演练结算未落对应伤害成就", achID)
		}
		event(3, "result", data)
		after, err = New(store.pool).UpdateProgress(ctx, av.OID, func(*game.Progress) error { return nil })
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("PG演练重试发奖或成就增加", err)
		}
	}
	svc.Detach(c)
	cold, coldC := loginPersistedProgressPlayer(t, New(store.pool), info, now)
	defer cold.Detach(coldC)
	before, err := New(store.pool).UpdateProgress(ctx, av.OID, func(*game.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cold.Handle(ctx, coldC, "client_need_recover_battle", nil); err != nil {
		t.Fatal(err)
	}
	after, err := New(store.pool).UpdateProgress(ctx, av.OID, func(*game.Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("PG演练冷恢复修改已冻结结果", err)
	}
}
