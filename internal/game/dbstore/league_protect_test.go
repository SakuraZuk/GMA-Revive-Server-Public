package dbstore

import (
	"context"
	"encoding/hex"
	"fmt"
	"hs-server/internal/game"
	"reflect"
	"testing"
	"time"
)

func TestPostgresLeagueActualMembersJanusNativeStatsThirtyReceiptsColdRetryAndTransfer(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	store := testStore(t)
	ctx := context.Background()
	zone := time.FixedZone("UTC+8", 28800)
	now := time.Date(2026, 10, 8, 10, 0, 0, 0, zone)
	if err := game.SetActivitySchedules(game.DefaultPermanentActivitySchedules(now.Add(-24 * time.Hour))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = game.SetActivitySchedules(nil) })
	info := game.ClientInfo{Account: "学会雅努斯全链实库", Password: "pw", Hostnum: 1}
	peerInfo := game.ClientInfo{Account: "学会真实会员实库", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, store, info, &now)
	peerSvc, peerC, peer := repairPlayer(t, store, peerInfo, &now)
	defer svc.Detach(c)
	defer peerSvc.Detach(peerC)
	for _, oid := range [][]byte{av.OID, peer.OID} {
		if _, err := store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
			p.AvatarLevel = 6
			p.GuideTasks = map[int]game.GuideTask{}
			p.UnlockSystems["league"] = 1
			p.Materials[11] = game.Material{ID: 11, Count: 10000, Total: 10000}
			p.Cards = []game.Card{{UUID: "00112233445566778899aabb", CardID: 4401, Level: 1, Grade: 1, SupportSkillLevel: 1}}
			p.Lineup = []string{p.Cards[0].UUID}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.pool.Exec(ctx, `UPDATE avatars SET level=6 WHERE avatar_oid=$1`, oid); err != nil {
			t.Fatal(err)
		}
	}
	read := func(oid []byte) game.Progress {
		t.Helper()
		row, e := New(store.pool).AdminPlayer(ctx, oid)
		if e != nil {
			t.Fatal(e)
		}
		return game.CloneProgress(row.Progress)
	}
	pgRemainingRPC(t, svc, c, "create_new_league", "学会实库", "击倒计分", 4401)
	owner := hex.EncodeToString(av.OID)
	member := hex.EncodeToString(peer.OID)
	leagueID := read(av.OID).LeagueID
	refused, e := peerSvc.Handle(ctx, peerC, "league_protect_start", pgSocialArgs(1, 0))
	noMember := pgRemainingPush(refused, "on_league_protect_start")
	if e != nil || noMember == nil || noMember.Args[0] != 12073 {
		t.Fatal("实库未入会角色没有原生失败回包", e, refused)
	}
	pgRemainingRPC(t, peerSvc, peerC, "apply_league", leagueID, nil)
	approvalReply := pgRemainingPush(pgRemainingRPC(t, svc, c, "agree_league_apply", member), "on_agree_league_apply")
	if approvalReply == nil || len(approvalReply.Args) != 3 {
		t.Fatal("实库审批回包缺原生成员字典", approvalReply)
	}
	if read(peer.OID).LeagueID != leagueID || len(read(av.OID).OwnedLeagues[leagueID].Members) != 2 {
		t.Fatal("真实学会主档与会员关系未同时保存")
	}
	if _, e := svc.Handle(ctx, c, "enter_dungeon", pgSocialArgs(9, 110001, map[string]any{"league_protect_card": map[string]any{"card_id": 99999}})); e == nil {
		t.Fatal("实库伪造学会卡可直接进雅努斯")
	}
	before := read(av.OID)
	for _, args := range [][]any{{2, 0}, {1, 1}, {1, 2}} {
		out, e := svc.Handle(ctx, c, "league_protect_start", pgSocialArgs(args...))
		reply := pgRemainingPush(out, "on_league_protect_start")
		want := 12077
		if args[0] == 2 {
			want = 12083
		}
		if e != nil || reply == nil || reply.Args[0] != want {
			t.Fatal("实库未解锁难度或技能缺原生回包", args, e, out)
		}
	}
	if !reflect.DeepEqual(before, read(av.OID)) {
		t.Fatal("实库拒绝起战保留部分修改")
	}
	resultEvent := func(uuid string, seq, kills, treasure int, win bool) []game.Push {
		t.Helper()
		winners := []string{}
		if win {
			winners = append(winners, owner)
		}
		return pgRemainingRPC(t, svc, c, "do_command", "__battle_event__", []any{map[string]any{"battle_uuid": uuid, "sequence": seq, "kind": "result", "data": map[string]any{"winner_eids": winners, "finished_task_list": []int{}, "league_protect_statistics": map[string]any{"kill_count": kills, "treasure_role_id": treasure}}}})
	}
	start := func(id int) string {
		t.Helper()
		out := pgRemainingRPC(t, svc, c, "league_protect_start", id, 0)
		reply, _ := pgNativeReply(t, out, "on_league_protect_start")
		if reply.Args[0] != game.RetSuccess || pgRemainingPush(out, "call_client_callback") != nil {
			t.Fatal("实库雅努斯专用回包错误")
		}
		p := read(av.OID)
		if p.LeagueProtect.TotalWeekly != 0 {
			t.Fatal("实库布阵提前扣次")
		}
		uuid := p.Battle.UUID
		pgRemainingRPC(t, svc, c, "load_entity_finish")
		pgRemainingRPC(t, svc, c, "battle_fighting", map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})
		if read(av.OID).LeagueProtect.TotalWeekly != 1 {
			t.Fatal("实库开始战斗没有原子扣次")
		}
		for seq, kind := range []string{"ready", "started"} {
			data := map[string]any{"version": 1}
			if kind == "started" {
				data = map[string]any{"units": []any{}}
			}
			pgRemainingRPC(t, svc, c, "do_command", "__battle_event__", []any{map[string]any{"battle_uuid": uuid, "sequence": seq + 1, "kind": kind, "data": data}})
		}
		return uuid
	}
	uuid := start(1)
	// 真实恢复更换UUID，已消耗资格保留同一次窗口别名，不能再次扣次。
	oldUUID := uuid
	svc.Detach(c)
	svc, c = loginPersistedProgressPlayer(t, store, info, now)
	svc.Now = func() time.Time { return now }
	defer svc.Detach(c)
	pgRemainingRPC(t, svc, c, "client_need_recover_battle")
	uuid = read(av.OID).Battle.UUID
	if uuid == oldUUID || read(av.OID).LeagueProtect.Consumed[uuid] != read(av.OID).LeagueProtect.Consumed[oldUUID] {
		t.Fatal("实库恢复没有迁移旧UUID计次收据")
	}
	pgRemainingRPC(t, svc, c, "load_entity_finish")
	pgRemainingRPC(t, svc, c, "battle_fighting", map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})
	if read(av.OID).LeagueProtect.TotalWeekly != 1 {
		t.Fatal("实库恢复开战重复扣次")
	}
	for seq, kind := range []string{"ready", "started"} {
		data := map[string]any{"version": 1}
		if kind == "started" {
			data = map[string]any{"units": []any{}}
		}
		pgRemainingRPC(t, svc, c, "do_command", "__battle_event__", []any{map[string]any{"battle_uuid": uuid, "sequence": seq + 1, "kind": kind, "data": data}})
	}
	before = read(av.OID)
	if out := resultEvent(uuid, 3, 51, 0, true); pgRemainingPush(out, "battle_result") != nil || !reflect.DeepEqual(before, read(av.OID)) {
		t.Fatal("实库超50原生击杀未全回滚")
	}
	out := resultEvent(uuid, 3, 18, 501, false)
	result, at := pgNativeReply(t, out, "battle_result")
	pgNativePropertyBefore(t, out, at, "league_activity_weekly_info")
	extra := result.Args[3].(map[string]any)["league_activity_battle_result"].(map[string]any)
	if extra["delta_contribution"] != int64(360) || extra["treasure_role_id"] != 501 {
		t.Fatal("实库实际击倒贡献宝藏错误")
	}
	p := read(av.OID)
	if p.LeagueProtect.Contribution != 360 || p.LeagueProtect.TotalWeekly != 1 || p.Materials[15].Count != 400 || p.Materials[11].Count != before.Materials[11].Count+8 || p.Achievements[403002].Targets[1020] != 1 || p.LeagueProtect.Unlocked[1] {
		t.Fatal("实库失败计次奖励参加成就未同事务")
	}
	// Cold Store和新Service重登后，已结算UUID改口100%也不得多发或解锁。
	cold := pgSocialService(t, New(store.pool))
	cold.Now = func() time.Time { return now }
	coldC := pgSocialLogin(t, cold, info, av.OID)
	defer cold.Detach(coldC)
	before = read(av.OID)
	env := map[string]any{"battle_uuid": uuid, "sequence": 4, "kind": "result", "data": map[string]any{"winner_eids": []string{owner}, "league_protect_statistics": map[string]any{"kill_count": 50, "treasure_role_id": 501}}}
	if _, e := cold.Handle(ctx, coldC, "do_command", pgSocialArgs("__battle_event__", []any{env})); e != nil || !reflect.DeepEqual(before, read(av.OID)) {
		t.Fatal("实库冷重建重复UUID改口又发奖", e)
	}
	refused, e = svc.Handle(ctx, c, "league_protect_start", pgSocialArgs(1, 0))
	limit := pgRemainingPush(refused, "on_league_protect_start")
	if e != nil || limit == nil || limit.Args[0] != 12044 {
		t.Fatal("实库同次开放缺1次上限失败回包", e, refused)
	}
	for attempt := 2; attempt <= 30; attempt++ {
		if now.Weekday() == time.Thursday {
			now = now.Add(5 * 24 * time.Hour)
		} else {
			now = now.Add(2 * 24 * time.Hour)
		}
		protect := min(attempt-1, 3)
		uuid = start(protect)
		out = resultEvent(uuid, 3, 50, 0, true)
		if pgRemainingPush(out, "battle_result") == nil {
			t.Fatal("实库完整击倒结算失败", attempt)
		}
	}
	p = read(av.OID)
	for _, id := range []int{403001, 403002, 403003, 403004, 403005, 403006} {
		if p.Achievements[id].Time == 0 {
			t.Fatal("实库学会六条成就缺接线", id)
		}
	}
	if len(p.LeagueProtect.Results) != 30 || len(p.LeagueProtect.Consumed) != 31 || !p.LeagueProtect.Unlocked[3] {
		t.Fatal("实库完整参加与三难度100%收据缺失")
	}
	pgRemainingRPC(t, svc, c, "appoint_league_member", member, 1)
	pgRemainingRPC(t, svc, c, "leave_league")
	left, right := read(av.OID), read(peer.OID)
	main := left.OwnedLeagues[leagueID]
	if left.LeagueID != "" || main.President != member || main.Dissolved || len(main.Members) != 1 || right.LeagueID != leagueID {
		t.Fatal("实库会长移交退会后主档与新会长关系损坏")
	}
	if main.Members[member].Type != 1 || fmt.Sprint(main.CardID) != "4401" {
		t.Fatal("实库新会长职位或冻结学会卡错误")
	}
}
