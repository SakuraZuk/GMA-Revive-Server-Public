package game

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func leagueProtectFixture(t *testing.T) (*Service, *Connection, *FixtureAccounts, *time.Time) {
	t.Helper()
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	s, cs, store := socialTestWorld(t)
	now := time.Date(2026, 10, 8, 10, 0, 0, 0, shanghaiZone)
	s.Now = func() time.Time { return now }
	activitySchedulesForTest(t, now)
	for _, c := range cs[:2] {
		if _, err := store.UpdateProgress(context.Background(), selectedOID(c), func(p *Progress) error {
			p.AvatarLevel = 6
			p.GuideTasks = map[int]GuideTask{}
			p.UnlockSystems["league"] = 1
			p.Materials[11] = Material{ID: 11, Count: 10000, Total: 10000}
			p.Cards = []Card{newCard(4401, 1, now)}
			p.Lineup = []string{p.Cards[0].UUID}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	// 夹具社交事务仅持久化Progress；完成注册元信息属于最初角色夹具。
	store.mu.Lock()
	for account, record := range store.records {
		for i := range record.Avatars {
			av := &record.Avatars[i]
			av.NicknameSet = true
			av.Gender = 1
			av.Info.Level = av.Progress.AvatarLevel
		}
		store.records[account] = record
	}
	store.mu.Unlock()
	remainingRPC(t, s, cs[0], "create_new_league", "雅努斯实测", "冻结统计", 4401)
	if err := s.refreshActivityAwards(context.Background(), cs[0]); err != nil {
		t.Fatal(err)
	}
	return s, cs[0], store, &now
}
func leagueProtectStartObserved(t *testing.T, s *Service, c *Connection) {
	t.Helper()
	remainingRPC(t, s, c, "load_entity_finish")
	remainingRPC(t, s, c, "battle_fighting", map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})
	uuid := c.SelectedAvatarUnsafe().Progress.Battle.UUID
	observeBattleEvent(t, context.Background(), s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":1,"kind":"ready","data":{"version":1}}`, uuid))
	observeBattleEvent(t, context.Background(), s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":2,"kind":"started","data":{"units":[]}}`, uuid))
}
func leagueProtectResultObserved(t *testing.T, s *Service, c *Connection, kills, treasure int, win bool) []Push {
	t.Helper()
	uuid := c.SelectedAvatarUnsafe().Progress.Battle.UUID
	winners := []string{}
	if win {
		winners = append(winners, hexOf(selectedOID(c)))
	}
	raw, _ := json.Marshal(map[string]any{"battle_uuid": uuid, "sequence": 3, "kind": "result", "data": map[string]any{"winner_eids": winners, "finished_task_list": []int{}, "league_protect_statistics": map[string]any{"kill_count": kills, "treasure_role_id": treasure}}})
	return observeBattleEvent(t, context.Background(), s, c, string(raw))
}

func TestLeagueProtectActualStartGuardianStatisticsFailureCountingWindowsAndTransfer(t *testing.T) {
	s, c, store, now := leagueProtectFixture(t)
	ctx := context.Background()
	before := socialSaved(t, store, c).Progress
	if _, err := s.Handle(ctx, c, "enter_dungeon", socialArgs(9, 110001, map[string]any{"activity_id": 14, "league_protect_card": map[string]any{"card_id": 99999}})); err == nil {
		t.Fatal("伪造原生卡可直接进入雅努斯")
	}
	for _, args := range [][]any{{2, 0}, {1, 1}, {1, 2}} {
		out, err := s.Handle(ctx, c, "league_protect_start", socialArgs(args...))
		reply := remainingFindPush(out, "on_league_protect_start")
		want := androidLeague.Errors["RET_LEAGUE_CARD_SKILL_NOT_UNLOCK"]
		if args[0] == 2 {
			want = androidLeague.Errors["RET_LEAGUE_ERROR"]
		}
		if err != nil || reply == nil || reply.Args[0] != want {
			t.Fatal("未解锁难度或援护没有原生失败回包", args, err, out)
		}
	}
	if !reflect.DeepEqual(before, socialSaved(t, store, c).Progress) {
		t.Fatal("拒绝入口修改学会资产或授权")
	}
	out := remainingRPC(t, s, c, "league_protect_start", 1, 0)
	if remainingFindPush(out, "on_league_protect_start") == nil || remainingFindPush(out, "call_client_callback") != nil {
		t.Fatal("专用原生起战回包错误", out)
	}
	p := c.SelectedAvatarUnsafe().Progress
	b := p.Battle
	if b == nil || b.ActivityContext == nil || b.ActivityContext.LeagueProtect.CardID != 4401 || p.LeagueProtect.TotalWeekly != 0 {
		t.Fatal("布阵提前扣次或没有真实学会卡")
	}
	uuid := b.UUID
	out = remainingRPC(t, s, c, "league_protect_start", 1, 0)
	if c.SelectedAvatarUnsafe().Progress.Battle.UUID != uuid {
		t.Fatal("恢复布阵重建UUID")
	}
	guardian := remainingFindPush(out, "start_server_battle_ok").Args[3].(map[string]any)["league_protect_card"].(map[string]any)
	if guardian["card_id"] != int64(4401) || guardian["extra_support_skill_idx"] != int64(0) {
		t.Fatal("冻结守门人卡wire错", guardian)
	}
	leagueProtectStartObserved(t, s, c)
	p = c.SelectedAvatarUnsafe().Progress
	if p.LeagueProtect.TotalWeekly != 1 || !p.Battle.ActivityContext.LeagueProtect.AttemptConsumed {
		t.Fatal("开始实际战斗没有原子计次")
	}
	before = CloneProgress(p)
	if out = leagueProtectResultObserved(t, s, c, 51, 0, true); battleTestResult(out) != nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("超原表50击杀没有全回滚")
	}
	out = leagueProtectResultObserved(t, s, c, 18, 501, false)
	result := battleTestResult(out)
	if result == nil {
		t.Fatal("全队失败没有真实结算", out)
	}
	extra := result.Args[3].(map[string]any)["league_activity_battle_result"].(map[string]any)
	if extra["kill_count"] != 18 || extra["delta_contribution"] != int64(360) || extra["win_flag"] != false {
		t.Fatal("原生失败实际击倒学分错误", extra)
	}
	p = c.SelectedAvatarUnsafe().Progress
	if p.Materials[15].Count != 400 || p.Materials[11].Count != before.Materials[11].Count+8 || p.LeagueProtect.TotalWeekly != 1 || p.LeagueProtect.Unlocked[1] || achievementCount(p.Achievements[403002], androidAchievements.Rules[403002]) != 1 || p.Achievements[403004].Time != 0 {
		t.Fatal("失败奖档宝藏/计次/参加/通关成就错误")
	}
	before = CloneProgress(p)
	observeBattleEvent(t, ctx, s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":4,"kind":"result","data":{"winner_eids":[],"league_protect_statistics":{"kill_count":50,"treasure_role_id":501}}}`, uuid))
	if !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("同UUID重复战报重复发奖或进成就")
	}
	refused, err := s.Handle(ctx, c, "league_protect_start", socialArgs(1, 0))
	limitReply := remainingFindPush(refused, "on_league_protect_start")
	if err != nil || limitReply == nil || limitReply.Args[0] != androidLeague.Errors["RET_LEAGUE_PROTECT_MAX_LIMIT"] {
		t.Fatal("同次开放没有原生1次上限回包", err, refused)
	}
	*now = now.Add(5 * 24 * time.Hour)
	remainingRPC(t, s, c, "league_protect_start", 1, 0)
	if c.SelectedAvatarUnsafe().Progress.LeagueProtect.TotalWeekly != 0 {
		t.Fatal("下次周二开启未恢复资格")
	}
	// 同服真实成员审批后原生移交会长，创建者档保留，旧会长可以正常退会。
	peerID := ""
	rows, err := store.SocialAvatars(ctx, SocialSearch{Hostnum: 1, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, av := range rows {
		if hexOf(av.OID) != hexOf(selectedOID(c)) && av.Progress.AvatarLevel == 6 {
			peerID = hexOf(av.OID)
			break
		}
	}
	if peerID == "" {
		t.Fatal("缺真实同服成员")
	}
	if _, err = store.UpdateAllSocial(ctx, func(v map[string]*Avatar) error {
		_, league := leagueOwner(v, v[hexOf(selectedOID(c))].Progress.LeagueID)
		return joinRealLeague(v[peerID], league, *now)
	}); err != nil {
		t.Fatal(err)
	}
	remainingRPC(t, s, c, "appoint_league_member", peerID, 1)
	remainingRPC(t, s, c, "leave_league")
	p = socialSaved(t, store, c).Progress
	for _, league := range p.OwnedLeagues {
		if league.President != peerID || league.Dissolved || len(league.Members) != 1 {
			t.Fatal("会长移交没有保留真实主档与成员")
		}
	}
}

func TestLeagueProtectAllThreeJanusAndThirtyActualCompletionsColdWire(t *testing.T) {
	s, c, _, now := leagueProtectFixture(t)
	for attempt := 1; attempt <= 30; attempt++ {
		protect := min(attempt, 3)
		remainingRPC(t, s, c, "league_protect_start", protect, 0)
		leagueProtectStartObserved(t, s, c)
		out := leagueProtectResultObserved(t, s, c, 50, 0, true)
		if battleTestResult(out) == nil {
			t.Fatal("真实雅努斯完成缺少原生结果", attempt)
		}
		p := c.SelectedAvatarUnsafe().Progress
		if !p.LeagueProtect.Unlocked[protect] || p.LeagueProtect.TotalWeekly != 1 {
			t.Fatal("100%真实胜利未解锁或计次不符")
		}
		if attempt < 30 {
			if now.Weekday() == time.Thursday {
				*now = now.Add(5 * 24 * time.Hour)
			} else {
				*now = now.Add(2 * 24 * time.Hour)
			}
		}
	}
	p := c.SelectedAvatarUnsafe().Progress
	for _, id := range []int{403001, 403002, 403003, 403004, 403005, 403006} {
		if achievementCount(p.Achievements[id], androidAchievements.Rules[id]) < androidAchievements.Rules[id].Need || p.Achievements[id].Time == 0 {
			t.Fatal("学会六条成就未全部完成", id, p.Achievements[id])
		}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var cold Progress
	if err = json.Unmarshal(raw, &cold); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(leagueProtectBattleExtra(p.Battle.ActivityContext), leagueProtectBattleExtra(cold.Battle.ActivityContext)) {
		t.Fatal("冷JSON守门人技能或整数键变化")
	}
	if len(cold.LeagueProtect.Results) != 30 || len(cold.LeagueProtect.Consumed) != 30 {
		t.Fatal("冷JSON丢失真实结果或计次收据")
	}
}
