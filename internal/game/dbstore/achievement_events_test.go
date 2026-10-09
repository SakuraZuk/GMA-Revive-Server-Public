package dbstore

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/game"
)

// 通过真实送礼/佩戴 RPC 验证成就和资产一起写入 JSONB；失败重试不能补造事件。
func TestPostgresAchievementEventsRealGiftEquipPersistenceAndRollback(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "成就消费与佩戴事务验收", Password: "pw", Hostnum: 1}
	identity, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	oid := identity.Avatars[0].OID
	now := time.Date(2026, 10, 8, 1, 0, 0, 0, time.UTC)
	svc, c := loginPersistedProgressPlayer(t, store, info, now)
	cardUUID := "00112233445566778899aabb"
	if _, err = store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		p.Cards = []game.Card{{UUID: cardUUID, CardID: 4401, Level: 1, Grade: 1}}
		p.Runes = map[string]game.Rune{}
		p.RuneSchemaVersion = game.CurrentRuneSchemaVersion
		p.Materials[523] = game.Material{ID: 523, Count: 1, Total: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
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
	request := args(1, 4401, 523, 1, 0)
	if _, err = svc.Handle(ctx, c, "consume_intimacy_gift", request); err != nil {
		t.Fatal(err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Materials[523].Count != 0 || len(p.Runes) != 4 || p.Achievements[305145].Targets[1000045] != 1 {
		t.Fatal("PG实际回礼与成就未同时完成")
	}
	// 四枚真实回礼契印逐一佩戴；第四位置成功之前不能获得全身5星成就。
	byPosition := map[int]string{}
	for uuid, rune := range p.Runes {
		byPosition[rune.Position] = uuid
	}
	for pos := 1; pos <= 4; pos++ {
		if _, err = svc.Handle(ctx, c, "embed_rune", args(2, cardUUID, byPosition[pos])); err != nil {
			t.Fatal(err)
		}
		count := c.SelectedAvatarUnsafe().Progress.Achievements[303003].Targets[10502]
		if pos < 4 && count != 0 || pos == 4 && count != 1 {
			t.Fatal("PG全身5星佩戴条件错误", pos, count)
		}
	}
	// 直接查PG JSONB，避免只验证连接内的内存投影。
	var persisted int64
	if err = store.pool.QueryRow(ctx, `SELECT (state->'achves'->'305145'->'finished_targets'->>'1000045')::bigint FROM avatar_progress WHERE avatar_oid=$1`, oid).Scan(&persisted); err != nil || persisted != 1 {
		t.Fatal("PG回礼目标未真实落库", persisted, err)
	}
	before, err := New(store.pool).UpdateProgress(ctx, oid, func(*game.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(ctx, c, "consume_intimacy_gift", request); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(ctx, c, "embed_rune", args(2, cardUUID, byPosition[4])); err != nil {
		t.Fatal(err)
	}
	after, err := New(store.pool).UpdateProgress(ctx, oid, func(*game.Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("PG重复回礼拒绝或重复佩戴仍推进资产/成就", err)
	}
	// 注入发生在已写入事件和资产之后的失败，验证PG行锁事务不会部分提交。
	if _, err = New(store.pool).UpdateProgress(ctx, oid, func(p *game.Progress) error {
		m := p.Materials[523]
		m.Count++
		p.Materials[523] = m
		a := p.Achievements[305145]
		a.Targets[1000045]++
		p.Achievements[305145] = a
		return errors.New("成就与资产写入后故障注入")
	}); err == nil {
		t.Fatal("PG事务故障注入未失败")
	}
	rolledBack, err := New(store.pool).UpdateProgress(ctx, oid, func(*game.Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(before, rolledBack) {
		t.Fatal("PG故障后成就或资产部分提交", err)
	}
	svc.Detach(c)
	_, reconnected := loginPersistedProgressPlayer(t, store, info, now)
	if !reflect.DeepEqual(before, reconnected.SelectedAvatarUnsafe().Progress) {
		t.Fatal("PG重登录丢失成就或装备关系")
	}
}
