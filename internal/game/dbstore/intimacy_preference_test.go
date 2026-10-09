package dbstore

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/game"
	"hs-server/internal/hotfix"
)

func TestPostgresIntimacyPreferenceDiscoveryPersistenceAndRollback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 8, 6, 0, 0, 0, time.UTC)
	info := game.ClientInfo{Account: "喜好发现持久验收", Password: "pw", Hostnum: 1}
	id, err := s.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	av, err := s.SetNicknameGender(ctx, id.Avatars[0].OID, "发现喜好", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Cards = []game.Card{{UUID: "00112233445566778899aabb", CardID: 4401, Level: 1, Grade: 1, SupportSkillLevel: 1}}
		p.Intimacy = map[int]int{4401: 0}
		p.IntimacyCommons = map[int]game.IntimacyCommon{4401: {RewardLevel: 2}}
		p.Materials[701] = game.Material{ID: 701, Count: 1, Total: 1}
		p.Materials[749] = game.Material{ID: 749, Count: 3, Total: 3}
		p.Materials[750] = game.Material{ID: 750, Count: 0, Total: 0}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	svc := game.New(s, &hotfix.Catalog{})
	svc.Now = func() time.Time { return now }
	c := game.NewConnection()
	raw, _ := json.Marshal(info)
	if _, err := svc.Handle(ctx, c, "quick_login", []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BecomePlayer(c); err != nil {
		t.Fatal(err)
	}
	read := func() game.Progress {
		t.Helper()
		reloaded, err := New(s.pool).QuickLogin(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		return reloaded.Avatars[0].Progress
	}
	ordinary := []json.RawMessage{json.RawMessage("71"), json.RawMessage("4401"), json.RawMessage("701"), json.RawMessage("1"), json.RawMessage("0")}
	if _, err := svc.Handle(ctx, c, "consume_intimacy_gift", ordinary); err != nil {
		t.Fatal(err)
	}
	before := read()
	if before.IntimacyCommons[4401].LikeGift || before.Intimacy[4401] != 20 {
		t.Fatal("非偏好赠礼不应发现喜好", before.IntimacyCommons[4401], before.Intimacy[4401])
	}
	multi := []json.RawMessage{json.RawMessage("72"), json.RawMessage("4401"), json.RawMessage("{\"749\":2,\"750\":1}")}
	pushes, err := svc.Handle(ctx, c, "consume_multi_intimacy_gift", multi)
	if err != nil || len(pushes) != 1 || pushes[0].Method != "call_client_callback" {
		t.Fatal("缺材料赠礼未返回原生拒绝", err, pushes)
	}
	after := read()
	if after.IntimacyCommons[4401].LikeGift || !reflect.DeepEqual(game.CloneProgress(before), game.CloneProgress(after)) {
		t.Fatal("真实数据库拒绝赠礼后误解锁或部分扣费")
	}
	ordinary[2] = json.RawMessage("749")
	if _, err := svc.Handle(ctx, c, "consume_intimacy_gift", ordinary); err != nil {
		t.Fatal(err)
	}
	after = read()
	if !after.IntimacyCommons[4401].LikeGift || after.Intimacy[4401] != 40 || after.Materials[749].Count != 2 || after.IntimacyCommons[4401].RewardLevel != 2 {
		t.Fatal("首次匹配未持久化基础收益、发现状态和原进度", after.Intimacy[4401], after.IntimacyCommons[4401])
	}
	if _, err := svc.Handle(ctx, c, "consume_intimacy_gift", ordinary); err != nil {
		t.Fatal(err)
	}
	after = read()
	if after.Intimacy[4401] != 62 || after.Materials[749].Count != 1 || after.Materials[749].Total != 3 || !after.IntimacyCommons[4401].LikeGift {
		t.Fatal("持久偏好未应用到下一笔收益", after.Intimacy[4401], after.Materials[749])
	}
	var persisted []byte
	if err := s.pool.QueryRow(ctx, "SELECT state FROM avatar_progress WHERE avatar_oid=$1", av.OID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	var stored game.Progress
	if err := json.Unmarshal(persisted, &stored); err != nil || !stored.IntimacyCommons[4401].LikeGift || stored.Intimacy[4401] != 62 {
		t.Fatal("JSONB偏好发现状态未真实保存", err)
	}
	// 再登录另一连接，公共幻书属性与真实进度都应保留发现状态。
	c2 := game.NewConnection()
	if _, err := svc.Handle(ctx, c2, "quick_login", []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BecomePlayer(c2); err != nil {
		t.Fatal(err)
	}
	if !c2.SelectedAvatarUnsafe().Progress.IntimacyCommons[4401].LikeGift {
		t.Fatal("重新登录丢失偏好发现状态")
	}
}

func TestPostgresSpecialGiftPreservesFirstPreferenceDiscovery(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "特殊赠礼偏好持久验收", Password: "pw", Hostnum: 1}
	id, err := s.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	av, err := s.SetNicknameGender(ctx, id.Avatars[0].OID, "特殊喜好", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Cards = []game.Card{{UUID: "00112233445566778899aabb", CardID: 4401, Level: 1, Grade: 1, SupportSkillLevel: 1}}
		p.Intimacy = map[int]int{4401: 0}
		p.IntimacyCommons = map[int]game.IntimacyCommon{4401: {}}
		p.Materials[749] = game.Material{ID: 749, Count: 1, Total: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	svc := game.New(s, &hotfix.Catalog{})
	svc.Now = func() time.Time { return time.Date(2026, 10, 8, 6, 0, 0, 0, time.UTC) }
	c := game.NewConnection()
	raw, _ := json.Marshal(info)
	if _, err := svc.Handle(ctx, c, "quick_login", []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BecomePlayer(c); err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage("73"), json.RawMessage("4401"), json.RawMessage("749"), json.RawMessage("1"), json.RawMessage("3")}
	if _, err := svc.Handle(ctx, c, "consume_intimacy_gift", args); err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := reloaded.Avatars[0].Progress
	// 原生 gift749基础20，choice3为1.2；首次仍无10%已发现偏好加成。
	if !p.IntimacyCommons[4401].LikeGift || p.Intimacy[4401] != 24 || p.IntimacyCommons[4401].SpecialCount != 1 ||
		p.SpecialGiftCounts[4401] != 1 || p.IntimacySpecialTotal != 1 || p.Materials[749].Count != 0 || p.Materials[749].Total != 1 {
		t.Fatal("特殊赠礼旧快照覆盖发现状态或收益计数错误", p.IntimacyCommons[4401], p.Intimacy[4401], p.SpecialGiftCounts)
	}
}
