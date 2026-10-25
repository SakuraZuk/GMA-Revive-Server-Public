package game

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestSyncPVPObservedResultRecoveryAndFrozenLineup(t *testing.T) {
	ctx := context.Background()
	s, c := newSyncPVPTestService()
	if err := s.updateProgress(ctx, c, func(p *Progress) error { p.Lineup = []string{p.Cards[0].UUID}; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Handle(ctx, c, "start_sync_pvp_match", []json.RawMessage{json.RawMessage(`1`)}); err != nil {
		t.Fatal(err)
	}
	now := s.Now()
	s.Now = func() time.Time { return now.Add(20 * time.Second) }
	if _, err := s.Tick(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Handle(ctx, c, "send_pvp_cards", []json.RawMessage{json.RawMessage(`0`)}); err != nil {
		t.Fatal(err)
	}
	old := c.SelectedAvatarUnsafe().Progress.Battle.UUID
	if _, err := s.Handle(ctx, c, "send_pvp_cards", []json.RawMessage{json.RawMessage(`0`)}); err == nil {
		t.Fatal("战中原生索引选卡未拒绝")
	}
	pushes, err := s.Handle(ctx, c, "client_need_recover_battle", nil)
	p := c.SelectedAvatarUnsafe().Progress
	if err != nil || len(pushes) != 7 || p.Battle.UUID == old || p.SyncPvpMatch.BattleUUID != p.Battle.UUID {
		t.Fatal("PVP重连未同步UUID及准备链", err, p.Battle)
	}
	if pushes[2].Method != "sync_battle_method" || pushes[2].Args[0] != "revival_restore_pvp_enemy" || pushes[3].Method != "pvp_battle_start_load" {
		t.Fatal("重连必须先恢复对手快照再进入加载界面")
	}
	uuid := p.Battle.UUID
	observeBattleEvent(t, ctx, s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":1,"kind":"ready","data":{"version":1}}`, uuid))
	observeBattleEvent(t, ctx, s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":2,"kind":"started","data":{"units":[{"eid":"1","role":4403,"hex":[0,0,0],"kind":"ally","hp":100,"max_hp":100}]}}`, uuid))
	player := hexOf(selectedOID(c))
	pushes = observeBattleEvent(t, ctx, s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":3,"kind":"result","data":{"outcome":"win","player_eid":%q,"winner_eids":[%q]}}`, uuid, player, player))
	p = c.SelectedAvatarUnsafe().Progress
	if !p.Battle.Finished || p.SyncPvpMatch.Status != syncPvpMatchSettled || len(p.SyncPvpSettlements) != 1 {
		t.Fatal("观察桥未原子结算PVP", p.SyncPvpMatch, p.SyncPvpSettlements, pushes)
	}
	score := p.SyncPvpScore
	if _, err := s.Handle(ctx, c, "client_need_recover_battle", nil); err != nil {
		t.Fatal(err)
	}
	if c.SelectedAvatarUnsafe().Progress.SyncPvpScore != score {
		t.Fatal("结算重播重复加分")
	}
	if _, err := s.Handle(ctx, c, "sync_pvp_battle_result", nil); err == nil {
		t.Fatal("客户端不能调用结果下行")
	}
}

func TestSyncPVPLocalRankFiltersBeforeWindow(t *testing.T) {
	accounts := NewFixtureAccounts(nil)
	for i := 0; i < 1100; i++ {
		host, score := 2, 2000
		if i == 1098 || i == 1099 {
			host, score = 1, 1000
		}
		oid := []byte(fmt.Sprintf("%012d", i))
		accounts.records[fmt.Sprint(i)] = FixtureAccount{Avatars: []Avatar{{OID: oid, Hostnum: host, Progress: Progress{SyncPvpScore: score}}}}
	}
	rows, err := accounts.SyncPvpRankings(context.Background(), 1000, 1)
	if err != nil || len(rows) != 2 {
		t.Fatal("本服分页遗漏世界千名外角色", len(rows), err)
	}
	rank, err := accounts.SyncPvpRank(context.Background(), rows[1].OID, 1)
	if err != nil || rank != 2 {
		t.Fatal("同分名次错误", rank, err)
	}
	if _, err := accounts.SyncPvpRank(context.Background(), []byte("不存在"), 1); err == nil {
		t.Fatal("未知角色不应得到名次")
	}
}

func TestSyncPvpRankListUsesRealScoresAndLeavesActivityRanks(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error { p.SyncPvpScore = 80; return nil }); err != nil {
		t.Fatal(err)
	}
	accounts.records["零分"] = FixtureAccount{Avatars: []Avatar{{OID: []byte("000000000099"), UID: 99, Hostnum: 1, Info: AvatarInfo{Nickname: "零分"}, Progress: Progress{SyncPvpScore: 0}}}}
	accounts.records["他服"] = FixtureAccount{Avatars: []Avatar{{OID: []byte("000000000088"), UID: 88, Hostnum: 2, Info: AvatarInfo{Nickname: "他服"}, Progress: Progress{SyncPvpScore: 200}}}}
	out, err := s.Handle(ctx, c, "query_rank_list", rawArgs(4, 0))
	if err != nil || len(out) != 1 || out[0].Method != "on_query_rank_list" || out[0].Args[0] != "4&0" {
		t.Fatal("同步竞技榜没有按原生编号回推", err, out)
	}
	infos, _ := out[0].Args[1].([]any)
	if len(infos) != 1 {
		t.Fatal("零分或他服角色进入本服同步榜", infos)
	}
	info := infos[0].(map[string]any)
	score, _ := info["score"].([]any)
	if len(score) != 1 || fmt.Sprint(score[0]) != "80" || info["uid"] != int64(av.UID) {
		t.Fatal("同步竞技榜积分或角色标识错误", info)
	}
	out, err = s.Handle(ctx, c, "query_rank_list", rawArgs(2, 0))
	if err != nil || len(out) != 1 || out[0].Method != "on_query_rank_list" || out[0].Args[0] != "2&0" {
		t.Fatal("已取证的收集榜没有独立回包", err)
	}
	infos, _ = out[0].Args[1].([]any)
	if len(infos) != 1 {
		t.Fatal("收集榜未按真实持有历史过滤", len(infos))
	}
	info = infos[0].(map[string]any)
	score, _ = info["score"].([]any)
	if len(score) != 1 || fmt.Sprint(score[0]) != fmt.Sprint(CollectionRankCount(c.SelectedAvatarUnsafe().Progress)) || info["uid"] != av.UID {
		t.Fatal("收集榜混用竞技积分或角色", info)
	}
	if _, err = s.Handle(ctx, c, "query_rank_list", rawArgs(6, 0)); err == nil {
		t.Fatal("尚未接真实活跃积分的学会榜被空成功")
	}
}
