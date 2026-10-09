package dbstore

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestPostgresCardSkillTalentActualRPCJSONBAndRejectedDuplicate(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "技能潜质原生事务验收", Password: "pw", Hostnum: 1}
	identity, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	oid := identity.Avatars[0].OID
	now := time.Date(2026, 10, 8, 1, 0, 0, 0, time.UTC)
	svc, c := loginPersistedProgressPlayer(t, store, info, now)
	args := func(values ...any) []json.RawMessage {
		result := []json.RawMessage{}
		for _, value := range values {
			raw, e := json.Marshal(value)
			if e != nil {
				t.Fatal(e)
			}
			result = append(result, raw)
		}
		return result
	}
	uuid, talentUUID := "00112233445566778899aabb", "00112233445566778899aabc"
	before, err := store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		p.Cards = []game.Card{{UUID: uuid, CardID: 4401, Level: 60, Grade: 5}, {UUID: talentUUID, CardID: 1601, Level: 60, Grade: 5}}
		p.Materials[12] = game.Material{ID: 12, Count: 20000, Total: 20000}
		p.Materials[251] = game.Material{ID: 251, Count: 14, Total: 14}
		for _, id := range []int{261, 262, 263, 264} {
			p.Materials[id] = game.Material{ID: id, Count: 10000, Total: 10000}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(ctx, c, "upgrade_card_skill", args(1, uuid, 440101)); err != nil {
		t.Fatal(err)
	}
	after, err := New(store.pool).UpdateProgress(ctx, oid, func(*game.Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("PG技能第二项材料不足后部分提交金币、技能或事件", err)
	}
	if _, err = store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		p.Materials[251] = game.Material{ID: 251, Count: 15, Total: 15}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(ctx, c, "upgrade_card_skill", args(1, uuid, 440101)); err != nil {
		t.Fatal(err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Cards[0].Skills[0].Level != 2 || p.Materials[12].Count != 0 || p.Materials[251].Count != 0 || p.Achievements[304001].Targets[304001] != 1 {
		t.Fatal("PG技能真实消耗未与状态成就同事务提交")
	}
	// 实际逐级升级前置及指定潜质；任何旧currentLevel重试均不能重复扣费。
	for index := 0; index < 3; index++ {
		for level := 0; level < 3; level++ {
			if _, err = svc.Handle(ctx, c, "upgrade_talent_node", args(talentUUID, 0, index, level)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for index := 0; index < 2; index++ {
		if _, err = svc.Handle(ctx, c, "upgrade_talent_node", args(talentUUID, 101, index, 0)); err != nil {
			t.Fatal(err)
		}
	}
	p = c.SelectedAvatarUnsafe().Progress
	if p.Cards[1].TalentTree[101][1].Level != 1 || p.Cards[1].TalentTree[101][1].ID != 31006 || p.Achievements[3011601].Targets[3011601] != 1 || p.Materials[264].Count != 9870 {
		t.Fatal("PG指定潜质原生坐标、成本或成就错误")
	}
	var storedLevel int
	if err = store.pool.QueryRow(ctx, `SELECT (state->'cards'->1->'talent_tree'->'101'->1->>'has_upgrade_time')::int FROM avatar_progress WHERE avatar_oid=$1`, oid).Scan(&storedLevel); err != nil || storedLevel != 1 {
		t.Fatal("PG潜质Int键树JSONB未落库", storedLevel, err)
	}
	before, err = New(store.pool).UpdateProgress(ctx, oid, func(*game.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(ctx, c, "upgrade_talent_node", args(talentUUID, 101, 1, 0)); err != nil {
		t.Fatal(err)
	}
	after, err = New(store.pool).UpdateProgress(ctx, oid, func(*game.Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("PG旧节点等级重复请求部分扣费或重复事件", err)
	}
	// 原生重置仅退一个点；前置破坏拒绝和旧等级重试不能部分写入。
	if _, err = svc.Handle(ctx, c, "reset_talent_node", args(talentUUID, 0, 0, 3)); err != nil {
		t.Fatal(err)
	}
	after, err = New(store.pool).UpdateProgress(ctx, oid, func(*game.Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("PG前置潜质重置拒绝部分提交", err)
	}
	if _, err = svc.Handle(ctx, c, "reset_talent_node", args(talentUUID, 101, 1, 1)); err != nil {
		t.Fatal(err)
	}
	after, err = New(store.pool).UpdateProgress(ctx, oid, func(*game.Progress) error { return nil })
	if err != nil || after.Cards[1].TalentTree[101][1].Level != 0 || !reflect.DeepEqual(before.Materials, after.Materials) || !reflect.DeepEqual(before.Achievements, after.Achievements) {
		t.Fatal("PG重置没有仅退点保留材料及历史", err)
	}
	before = after
	if _, err = svc.Handle(ctx, c, "reset_talent_node", args(talentUUID, 101, 1, 1)); err != nil {
		t.Fatal(err)
	}
	after, err = New(store.pool).UpdateProgress(ctx, oid, func(*game.Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("PG旧等级重置重试写入", err)
	}
	svc.Detach(c)
	_, reconnected := loginPersistedProgressPlayer(t, store, info, now)
	if !reflect.DeepEqual(before.Cards, reconnected.SelectedAvatarUnsafe().Progress.Cards) || !reflect.DeepEqual(before.Achievements, reconnected.SelectedAvatarUnsafe().Progress.Achievements) {
		t.Fatal("PG重登录未恢复技能、潜质或成就")
	}
}
