package dbstore

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hs-server/internal/game"
	"hs-server/internal/hotfix"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPostgresShopPurchaseAndAchievementEventsPersistence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	info := game.ClientInfo{Account: "商店与成就事件验收", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, s, info, &now)
	if _, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Materials[11] = game.Material{ID: 11, Count: 5000, Total: 5000}
		p.Materials[701] = game.Material{ID: 701, Count: 20, Total: 20}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, rpc := range []struct {
		name string
		args []json.RawMessage
	}{
		{"buy_commodity", []json.RawMessage{json.RawMessage(`1010003`), json.RawMessage(`1`), json.RawMessage(`0`)}},
		{"consume_multi_intimacy_gift", []json.RawMessage{json.RawMessage(`91`), json.RawMessage(`4401`), json.RawMessage(`{"701":20}`)}},
	} {
		pushes, err := svc.Handle(ctx, c, rpc.name, rpc.args)
		if err != nil {
			t.Fatal(err)
		}
		last := pushes[len(pushes)-1]
		if rpc.name == "buy_commodity" {
			if last.Method != "on_buy_commodity" || last.Args[0] != 0 {
				t.Fatal("商城原生下行错误")
			}
		} else if last.Args[1].([]any)[0] != 0 {
			t.Fatal("送礼回调失败")
		}
	}
	after, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := after.Avatars[0].Progress
	if p.Materials[11].Count != 2700 || p.Materials[501].Count != 12 || p.Materials[701].Count != 0 || p.CommodityDetails[1010003].Total != 1 || p.Achievements[305002].Targets[20401] != 20 {
		t.Fatal("商店资产/成就JSONB未持久")
	}
	if _, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		m := p.Materials[501]
		m.Count = math.MaxInt64
		m.Total = math.MaxInt64
		p.Materials[501] = m
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Handle(ctx, c, "buy_commodity", []json.RawMessage{json.RawMessage(`1010003`), json.RawMessage(`1`), json.RawMessage(`0`)}); err == nil {
		t.Fatal("商店奖励溢出未拒绝")
	}
	after, err = New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	if after.Avatars[0].Progress.Materials[11].Count != 2700 || after.Avatars[0].Progress.CommodityDetails[1010003].Total != 1 {
		t.Fatal("商店失败事务扣费或推进次数")
	}
}

func TestPostgresOathKnowledgeAndComposePersistence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	info := game.ClientInfo{Account: "誓约知识合成验收", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, s, info, &now)
	_, err := s.pool.Exec(ctx, `UPDATE avatars SET level=60 WHERE avatar_oid=$1`, av.OID)
	if err != nil {
		t.Fatal(err)
	}
	const uuid = "00112233445566778899aabb"
	_, err = s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Cards = []game.Card{{UUID: uuid, CardID: 4401, Level: 60, Grade: 5}}
		p.Intimacy = map[int]int{4401: 32000}
		p.IntimacyCommons = map[int]game.IntimacyCommon{4401: {RewardLevel: 6}}
		p.Materials[820] = game.Material{ID: 820, Count: 2, Total: 2}
		p.Materials[4] = game.Material{ID: 4, Count: 60000000, Total: 60000000}
		p.Materials[1101] = game.Material{ID: 1101, Count: 81, Total: 81}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	c = game.NewConnection()
	raw, _ := json.Marshal(info)
	if _, err = svc.Handle(ctx, c, "quick_login", []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.BecomePlayer(c); err != nil {
		t.Fatal(err)
	}
	rawUUID, _ := json.Marshal(uuid)
	for _, rpc := range []struct {
		name string
		args []json.RawMessage
	}{
		{"consume_ring", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`4401`), json.RawMessage(`820`)}},
		{"update_level_one", []json.RawMessage{json.RawMessage(`2`), rawUUID}},
		{"up_level_card", []json.RawMessage{json.RawMessage(`3`), rawUUID, json.RawMessage(`false`)}},
		{"compose_cards", []json.RawMessage{json.RawMessage(`4`), json.RawMessage(`[1101]`)}},
	} {
		pushes, err := svc.Handle(ctx, c, rpc.name, rpc.args)
		if err != nil {
			t.Fatal(rpc.name, err)
		}
		if pushes[len(pushes)-1].Args[1].([]any)[0] != 0 {
			t.Fatal(rpc.name, "返回失败", pushes)
		}
	}
	after, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := after.Avatars[0].Progress
	if p.Cards[0].Level != 61 || !p.Cards[0].IsUpdateLevelOne || p.Cards[0].RingID != 1 || p.IntimacyCommons[4401].RingID != 1 || p.Materials[820].Count != 1 || p.Materials[4].Count != 8800000 || p.Materials[1101].Count != 1 || len(p.Cards) != 3 {
		t.Fatal("誓约知识合成JSONB持久化错误")
	}
	pushes, err := svc.Handle(ctx, c, "consume_ring", []json.RawMessage{json.RawMessage(`5`), json.RawMessage(`4401`), json.RawMessage(`820`)})
	if err != nil || pushes[0].Args[1].([]any)[0] != 5053 {
		t.Fatal("重复誓约未拒绝", err)
	}
	after, err = New(s.pool).QuickLogin(ctx, info)
	if err != nil || after.Avatars[0].Progress.Materials[820].Count != 1 {
		t.Fatal("重复誓约再次扣料", err)
	}
	if after.Avatars[0].InitialProperties(info.Account)["exp_pool"] != int64(8800000) {
		t.Fatal("登录知识储备未同步")
	}
}

func TestPostgresTenDrawAndFirstBonusPersistence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	info := game.ClientInfo{Account: "抽卡首抽十连验收", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, s, info, &now)
	_, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Materials[501] = game.Material{ID: 501, Count: 20, Total: 20}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []string{"1", "10"} {
		pushes, err := svc.Handle(ctx, c, "random_cards", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`1`), json.RawMessage(count)})
		if err != nil || len(pushes) < 3 || pushes[len(pushes)-1].Method != "call_client_callback" || pushes[len(pushes)-1].Args[1].([]any)[0] != 0 {
			t.Fatal("抽卡RPC失败", err, pushes)
		}
	}
	after, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := after.Avatars[0].Progress
	if len(p.Cards) != 12 || p.Materials[501].Count != 9 || p.RandomCardsRecord[1].RandomCount != 11 || p.PromiseCardsRecord[1001].FinishCount != 1 {
		t.Fatal("抽卡卡数成本或首抽计数错误", p.PromiseCardsRecord)
	}
	before := p.Materials[521].Count
	if _, err = svc.Handle(ctx, c, "random_cards", []json.RawMessage{json.RawMessage(`2`), json.RawMessage(`1`), json.RawMessage(`1`)}); err != nil {
		t.Fatal(err)
	}
	after, err = New(s.pool).QuickLogin(ctx, info)
	if err != nil || after.Avatars[0].Progress.Materials[521].Count != before {
		t.Fatal("首抽奖励重复", err)
	}
}

func TestPostgresCosmeticSelectionAndAcquisitionHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	info := game.ClientInfo{Account: "头像历史验收", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, s, info, &now)
	_, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.OwnedHeadBox = map[int]float64{2: 0, 3: 0}
		p.ObtainedCardIDs = map[int]int64{1101: 100}
		p.Materials[1101] = game.Material{ID: 1101, Count: 40, Total: 40}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(ctx, c, "change_head", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`2`)}); err != nil {
		t.Fatal(err)
	}
	pushes, err := svc.Handle(ctx, c, "compose_cards", []json.RawMessage{json.RawMessage(`2`), json.RawMessage(`[1101]`)})
	if err != nil || len(pushes[len(pushes)-1].Args[1].([]any)[2].([]int)) != 0 {
		t.Fatal("历史获取幻书误报首次", err)
	}
	after, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := after.Avatars[0].Progress
	if p.SelectedHeadID != 2 || p.ObtainedCardIDs[1101] != 100 || after.Avatars[0].InitialProperties(info.Account)["head_id"] != 2 {
		t.Fatal("头像选择及获取历史未持久化")
	}
}

func TestPostgresAchievementClaimAndLoginPersistence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	info := game.ClientInfo{Account: "成就领取验收", Password: "pw", Hostnum: 1}
	svc, c, _ := repairPlayer(t, s, info, &now)
	args := []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`101001`)}
	pushes, err := svc.Handle(ctx, c, "receive_achv_bonus", args)
	if err != nil || pushes[len(pushes)-1].Args[1].([]any)[0] != 0 {
		t.Fatal("登录成就领奖失败", err, pushes)
	}
	after, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := after.Avatars[0].Progress
	if !p.Achievements[101001].Claimed || p.Achievements[101002].Targets[101001] != 1 || p.Materials[11].Count != 50 {
		t.Fatal("成就和奖励未原子持久化", p.Materials[11])
	}
	pushes, err = svc.Handle(ctx, c, "receive_achv_bonus", args)
	if err != nil || pushes[0].Args[1].([]any)[0] != 6004 {
		t.Fatal("重复领取未拒绝", err)
	}
	c = game.NewConnection()
	raw, _ := json.Marshal(info)
	if _, err = svc.Handle(ctx, c, "quick_login", []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	after, err = New(s.pool).QuickLogin(ctx, info)
	if err != nil || after.Avatars[0].Progress.Achievements[101002].Targets[101001] != 1 || after.Avatars[0].Progress.Materials[11].Count != 50 {
		t.Fatal("重登重复计数或奖励", err)
	}
}

func repairPlayer(t *testing.T, s *Store, info game.ClientInfo, now *time.Time) (*game.Service, *game.Connection, game.Avatar) {
	t.Helper()
	ctx := context.Background()
	id, err := s.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	av, err := s.SetNicknameGender(ctx, id.Avatars[0].OID, info.Account, 1)
	if err != nil {
		t.Fatal(err)
	}
	svc := game.New(s, &hotfix.Catalog{})
	svc.Now = func() time.Time { return *now }
	c := game.NewConnection()
	raw, _ := json.Marshal(info)
	if _, err := svc.Handle(ctx, c, "quick_login", []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BecomePlayer(c); err != nil {
		t.Fatal(err)
	}
	return svc, c, av
}

func TestPostgresSpecialGiftDailyBoxAndRollback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 15, 59, 59, 0, time.UTC)
	info := game.ClientInfo{Account: "特殊送礼日界验收", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, s, info, &now)
	_, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Cards = []game.Card{{UUID: "00112233445566778899aabb", CardID: 4401, Level: 1, Grade: 1, SupportSkillLevel: 1}}
		p.Intimacy = map[int]int{}
		p.IntimacyCommons = map[int]game.IntimacyCommon{4401: {SpecialCount: 100}}
		p.Materials[523] = game.Material{ID: 523, Count: 10, Total: 10}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`4401`), json.RawMessage(`523`), json.RawMessage(`1`), json.RawMessage(`3`)}
	pushes, err := svc.Handle(ctx, c, "consume_intimacy_gift", args)
	if err != nil || len(pushes) < 6 {
		t.Fatal("特殊送礼RPC失败", err, pushes)
	}
	box := pushes[len(pushes)-1].Args[1].([]any)[1].(map[string]any)
	if len(box["runes"].(map[string]any)) != 4 {
		t.Fatal("契印盒未返回四枚")
	}
	after, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := after.Avatars[0].Progress
	if len(p.Runes) != 4 || p.SpecialGiftCounts[4401] != 1 || p.IntimacyCommons[4401].SpecialCount != 101 || p.Materials[523].Count != 9 {
		t.Fatal("特殊送礼未完整持久")
	}
	now = now.Add(time.Second)
	if _, err := svc.Handle(ctx, c, "consume_intimacy_gift", args); err != nil {
		t.Fatal(err)
	}
	after, err = New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p = after.Avatars[0].Progress
	if p.SpecialGiftDay != "2026-10-08" || p.SpecialGiftCounts[4401] != 1 || p.IntimacyCommons[4401].SpecialCount != 102 || len(p.Runes) != 8 {
		t.Fatal("日界丢失累计状态")
	}
	now = now.Add(-time.Second)
	if _, err := svc.Handle(ctx, c, "consume_intimacy_gift", args); err == nil {
		t.Fatal("时钟回拨不能倒退存档日")
	}
	after, err = New(s.pool).QuickLogin(ctx, info)
	if err != nil || after.Avatars[0].Progress.Materials[523].Count != 8 {
		t.Fatal("回拨拒绝未回滚", err)
	}
}

func TestPostgresMailExpiryBatchRollbackAndPersistence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	info := game.ClientInfo{Account: "邮件扩展验收", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, s, info, &now)
	for _, m := range []game.Mail{{MID: 1, Title: "有效附件", Attachments: map[int]int64{12: 3}}, {MID: 2, Title: "溢出附件", Attachments: map[int]int64{13: 1}}} {
		if err := svc.IssueMail(ctx, c, m); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Materials[13] = game.Material{ID: 13, Count: math.MaxInt64, Total: math.MaxInt64}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Handle(ctx, c, "receive_all_attachments", []json.RawMessage{json.RawMessage(`1`)}); err == nil {
		t.Fatal("批量溢出未拒绝")
	}
	after, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := after.Avatars[0].Progress
	if p.ShortMailInfo[1].State != game.MailUnread || p.ShortMailInfo[2].State != game.MailUnread {
		t.Fatal("批量领取没有全部回滚")
	}
	raw, _ := json.Marshal(p.ShortMailInfo[1].UUID)
	if _, err := svc.Handle(ctx, c, "query_mail_content", []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage(`2`), raw}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage(`3`), raw}); err == nil {
		t.Fatal("重复领取未拒绝")
	}
	after, err = New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	if after.Avatars[0].Progress.Materials[12].Count != p.Materials[12].Count+3 || after.Avatars[0].Progress.ShortMailInfo[1].State != game.MailFinal {
		t.Fatal("邮件领取持久失败")
	}
}

func TestPostgresSyncPVPBridgeSettlementPersistence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	info := game.ClientInfo{Account: "PVP观察桥验收", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, s, info, &now)
	defer svc.Detach(c)
	_, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Cards = []game.Card{{UUID: "00112233445566778899aabb", CardID: 4401, Level: 1, Grade: 1, SupportSkillLevel: 1}}
		p.SyncPvpPresetIDs = []string{"00112233445566778899aabc"}
		p.PresetCardsRecord = map[string]game.PresetRecord{"00112233445566778899aabc": {PresetID: "00112233445566778899aabc", Cards: game.BattleLayout{Fighting: []string{"00112233445566778899aabb"}}}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.Handle(ctx, c, "start_sync_pvp_match", []json.RawMessage{json.RawMessage(`1`)})
	if err != nil {
		t.Fatal(err)
	}
	reply, _ := pgNativeReply(t, out, "call_client_callback")
	if reply.Args[0] != 1 || reply.Args[1].([]any)[0] != game.RetSuccess {
		t.Fatal("同步PVP排队没有原生成功回调", reply)
	}
	waiting, err := New(s.pool).AdminPlayer(ctx, av.OID)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.Progress.SyncPvpMatch == nil || waiting.Progress.SyncPvpMatch.Status != "waiting" || waiting.Progress.Battle != nil {
		t.Fatal("新匹配应先等待真人或机器人")
	}
	if _, err = svc.Handle(ctx, c, "send_pvp_cards", []json.RawMessage{json.RawMessage(`0`)}); err == nil {
		t.Fatal("等待匹配时不应允许原生索引选卡")
	}
	stillWaiting, err := New(s.pool).AdminPlayer(ctx, av.OID)
	if err != nil || !reflect.DeepEqual(game.CloneProgress(waiting.Progress), game.CloneProgress(stillWaiting.Progress)) {
		t.Fatal("等待期间选卡失败改变整进度", err)
	}
	// 本版1000分档lose_match_time上界10秒；推进夹具时钟后由真实Tick完成AI兜底。
	now = now.Add(20 * time.Second)
	out, err = svc.Tick(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	reply, _ = pgNativeReply(t, out, "select_pvp_cards")
	if len(reply.Args) != 2 || reply.Args[1] != 1 {
		t.Fatal("AI匹配缺少原生选卡通知", reply)
	}
	ready := c.SelectedAvatarUnsafe().Progress.SyncPvpMatch
	if ready == nil || ready.Status != "ready" || len(ready.PresetIDs) != 1 || ready.PresetIDs[0] != "00112233445566778899aabc" {
		t.Fatal("真实Tick未冻结同步PVP预设", ready)
	}
	out, err = svc.Handle(ctx, c, "send_pvp_cards", []json.RawMessage{json.RawMessage(`0`)})
	if err != nil {
		t.Fatal(err)
	}
	reply, _ = pgNativeReply(t, out, "set_cards_result")
	if len(reply.Args) != 2 || reply.Args[0] != true {
		t.Fatal("原生索引选卡未成功", reply)
	}
	locked, err := New(s.pool).AdminPlayer(ctx, av.OID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(ctx, c, "send_pvp_cards", []json.RawMessage{json.RawMessage(`0`)}); err == nil {
		t.Fatal("战中重复索引选卡未拒绝")
	}
	unchanged, err := New(s.pool).AdminPlayer(ctx, av.OID)
	if err != nil || !reflect.DeepEqual(game.CloneProgress(locked.Progress), game.CloneProgress(unchanged.Progress)) {
		t.Fatal("战中重复选卡改变整进度", err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	uuid := p.Battle.UUID
	player := hex.EncodeToString(av.OID)
	envelopes := []string{
		fmt.Sprintf(`{"battle_uuid":%q,"sequence":1,"kind":"ready","data":{"version":1}}`, uuid),
		fmt.Sprintf(`{"battle_uuid":%q,"sequence":2,"kind":"started","data":{"units":[{"eid":"1","role":4401,"hex":[0,0,0],"kind":"ally","hp":100,"max_hp":100}]}}`, uuid),
		fmt.Sprintf(`{"battle_uuid":%q,"sequence":3,"kind":"result","data":{"outcome":"win","player_eid":%q,"winner_eids":[%q]}}`, uuid, player, player),
	}
	for _, e := range envelopes {
		if _, err := svc.Handle(ctx, c, "do_command", []json.RawMessage{json.RawMessage(`"__battle_event__"`), json.RawMessage("[" + e + "]")}); err != nil {
			t.Fatal(err)
		}
	}
	after, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p = after.Avatars[0].Progress
	if !p.Battle.Finished || len(p.SyncPvpSettlements) != 1 || p.SyncPvpScore <= 1000 || p.SyncPvpWeeklyWins != 1 {
		t.Fatal("PVP桥和积分未原子持久", p.SyncPvpSettlements)
	}
	score := p.SyncPvpScore
	if _, err := svc.Handle(ctx, c, "client_need_recover_battle", nil); err != nil {
		t.Fatal(err)
	}
	after, err = New(s.pool).QuickLogin(ctx, info)
	if err != nil || after.Avatars[0].Progress.SyncPvpScore != score {
		t.Fatal("PVP重播重复结算", err)
	}
}

func TestPostgresSyncPVPLocalWindowAndTies(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	// 隔离schema直接建立测试资料，避免1100次bcrypt掩盖排行测试。
	_, err := s.pool.Exec(ctx, `INSERT INTO accounts(account,password_hash) SELECT '排名'||g,'测试专用无登录' FROM generate_series(1,1100) g;
	INSERT INTO avatars(avatar_oid,account,hostnum,nickname) SELECT decode(lpad(to_hex(g),24,'0'),'hex'),'排名'||g,CASE WHEN g>1098 THEN 1 ELSE 2 END,'角色'||g FROM generate_series(1,1100) g;
	INSERT INTO avatar_progress(avatar_oid,state) SELECT avatar_oid,jsonb_build_object('sync_pvp_score',CASE WHEN hostnum=1 THEN 1000 ELSE 2000 END) FROM avatars;`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.SyncPvpRankings(ctx, 1000, 1)
	if err != nil || len(rows) != 2 {
		t.Fatal("本服分页范围错误", len(rows), err)
	}
	rank, err := s.SyncPvpRank(ctx, rows[1].OID, 1)
	if err != nil || rank != 2 {
		t.Fatal("同分排名错误", rank, err)
	}
	rank, err = s.SyncPvpRank(ctx, rows[1].OID, 0)
	if err != nil || rank != 1100 {
		t.Fatal("世界名次被窗口截断", rank, err)
	}
	if _, err := s.SyncPvpRank(ctx, make([]byte, 12), 1); err == nil {
		t.Fatal("未知角色不应得到rank1")
	}
}

func TestPostgresMailAssetsAdminReceiptAndRollback(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Now()
	info := game.ClientInfo{Account: "邮件附件管理验收", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, store, info, &now)
	defer svc.Detach(c)
	r, err := game.GenerateRune(game.RuneSpec{Suit: 1101, Position: 1, Star: 5, Level: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	card := game.Card{UUID: "00112233445566778899aacc", CardID: 4401, Level: 1, Grade: 0, SupportSkillLevel: 1, Time: now.Unix()}
	token := strings.Repeat("q", 32)
	handler := game.AdminHandler(svc, token)
	req := map[string]any{"avatar_oid": hex.EncodeToString(av.OID), "receipt": "真实库邮件001", "operator": "验收员", "reason": "隔离数据库验收", "mail": game.Mail{MID: 701, Title: "卡契印材料", Cards: []game.Card{card}, Runes: map[string]game.Rune{r.UUID: r}, Attachments: map[int]int64{12: 3}}}
	body, _ := json.Marshal(req)
	call := func(body []byte) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/admin/mail/issue", bytes.NewReader(body))
		request.RemoteAddr = "127.0.0.1:9001"
		request.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, request)
		return w
	}
	var wg sync.WaitGroup
	responses := make(chan int, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); responses <- call(body).Code }()
	}
	wg.Wait()
	close(responses)
	created := 0
	for code := range responses {
		if code == 201 {
			created++
		} else if code != 200 {
			t.Fatal("真实库并发发件失败", code)
		}
	}
	if created != 1 {
		t.Fatal("重复发件", created)
	}
	pushes, err := svc.Tick(ctx, c)
	if err != nil || len(pushes) < 2 || pushes[1].Method != "notify_new_mail" {
		t.Fatal("真实库管理发件未通知", err)
	}
	after, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := after.Avatars[0].Progress
	before := p.Materials[12].Count
	raw, _ := json.Marshal(p.ShortMailInfo[701].UUID)
	if _, err = svc.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage("1"), raw}); err != nil {
		t.Fatal(err)
	}
	after, err = New(store.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p = after.Avatars[0].Progress
	found := false
	for _, owned := range p.Cards {
		found = found || (owned.UUID == card.UUID && owned.Grade == 0)
	}
	if !found || p.Runes[r.UUID].UUID != r.UUID || p.Materials[12].Count != before+3 || p.ShortMailInfo[701].State != game.MailFinal || len(p.MailAdminReceipts) != 1 {
		t.Fatal("附件或收据JSONB未持久")
	}
	if _, err = svc.Handle(ctx, c, "delete_mail", []json.RawMessage{json.RawMessage("2"), raw}); err != nil {
		t.Fatal(err)
	}
	if response := call(body); response.Code != 200 {
		t.Fatal("删除后收据重试失败", response.Code)
	}
	after, err = New(store.pool).QuickLogin(ctx, info)
	if err != nil || len(after.Avatars[0].Progress.ShortMailInfo) != 0 {
		t.Fatal("删除后重试重发邮件", err)
	}
	if _, err = store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Materials[12] = game.Material{ID: 12, Count: math.MaxInt64, Total: math.MaxInt64}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = svc.IssueMail(ctx, c, game.Mail{MID: 702, Title: "溢出回滚", Attachments: map[int]int64{12: 1}}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(ctx, c, "receive_all_attachments", []json.RawMessage{json.RawMessage("3")}); err == nil {
		t.Fatal("邮件资产溢出未拒绝")
	}
	after, err = New(store.pool).QuickLogin(ctx, info)
	if err != nil || after.Avatars[0].Progress.ShortMailInfo[702].State != game.MailUnread || after.Avatars[0].Progress.Materials[12].Count != math.MaxInt64 {
		t.Fatal("邮件失败整事务未回滚", err)
	}
}
