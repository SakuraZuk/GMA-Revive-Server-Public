package dbstore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestSocialStatePatchPreservesValuesAndRemovals(t *testing.T) {
	before := []byte(`{"大整数":9223372036854775807,"保留":{"零":0},"删除":[1],"数组":[1,2],"空值":true}`)
	after := []byte(`{"大整数":9223372036854775807,"保留":{"零":0},"数组":[],"空值":null,"新增":{"数量":7}}`)
	patch, err := socialStatePatch(before, after)
	if err != nil || !reflect.DeepEqual(patch.remove, []string{"删除"}) {
		t.Fatal("遗漏字段删除", patch, err)
	}
	var old, set, want map[string]json.RawMessage
	_ = json.Unmarshal(before, &old)
	_ = json.Unmarshal(patch.set, &set)
	_ = json.Unmarshal(after, &want)
	if _, exists := set["大整数"]; exists {
		t.Fatal("未变动的大整数被重写")
	}
	for _, key := range patch.remove {
		delete(old, key)
	}
	for key, value := range set {
		old[key] = value
	}
	if !reflect.DeepEqual(old, want) {
		t.Fatal("差异应用改变了原值语义")
	}
	if _, err := socialStatePatch([]byte(`null`), after); err == nil {
		t.Fatal("空根对象应拒绝")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := socialStatePatches(ctx, []string{"一"}, map[string][]byte{"一": before}, [][]byte{after}, 4); err == nil || result != nil {
		t.Fatal("取消后仍生成可提交差异")
	}
}

func TestPostgresPeriodicPatchKeepsAssetsAndUnknownFields(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	identity, err := s.Register(ctx, game.ClientInfo{Account: "周期差异存档验收", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	oid := identity.Avatars[0].OID
	id := hex.EncodeToString(oid)
	_, err = s.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		p.CardFavorites = map[int]bool{1001: true}
		p.CheckInRecords = map[int]any{1: []any{1, 2}}
		p.PayCoin = 9223372036854775000
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE avatar_progress SET state=state || '{"兼容保留":{"原值":9007199254740993}}'::jsonb WHERE avatar_oid=$1`, oid); err != nil {
		t.Fatal(err)
	}
	before := pgHumanRead(t, s, oid).Progress
	rows, err := s.UpdatePeriodicSocial(ctx, func(v map[string]*game.Avatar) error {
		p := &v[id].Progress
		p.CardFavorites = nil  // omitempty需要从JSON删除旧字段。
		p.CheckInRecords = nil // 无omitempty需要保留JSON null。
		p.AsyncPvp.RewardDay = "2026-10-10"
		return nil
	})
	if err != nil || len(rows) != 1 {
		t.Fatal("周期差异写入失败", err, len(rows))
	}
	var raw []byte
	if err = s.pool.QueryRow(ctx, `SELECT state FROM avatar_progress WHERE avatar_oid=$1`, oid).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["server_card_favorites"]; exists {
		t.Fatal("省略字段未删除")
	}
	if string(fields["check_in_records"]) != "null" || len(fields["兼容保留"]) == 0 {
		t.Fatal("空值或未知字段未保留")
	}
	got := pgHumanRead(t, s, oid).Progress
	if got.PayCoin != before.PayCoin || !reflect.DeepEqual(got.Materials, before.Materials) || got.AsyncPvp.RewardDay != "2026-10-10" {
		t.Fatal("周期字段写入改变无关资产或丢失收据")
	}
	rows, err = s.UpdatePeriodicSocial(ctx, func(v map[string]*game.Avatar) error {
		v[id].Progress.AsyncPvp.RewardDay = "2026-10-10"
		return nil
	})
	if err != nil || len(rows) != 0 {
		t.Fatal("重复结算重新写入", err, len(rows))
	}
}

// 使用隔离测试表比较相同存档结果的传输量和耗时；时间只报告，不以单次快慢判定正确性。
func TestPostgresPeriodicPatchPayloadComparison(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	large := map[string]any{}
	for i := 0; i < 6000; i++ {
		large[fmt.Sprint(i)] = map[string]any{"数量": i + 1, "等级": i % 90, "描述": fmt.Sprintf("测试资产%d", i)}
	}
	before, _ := json.Marshal(map[string]any{"资产": large, "周期": "旧周期"})
	after, _ := json.Marshal(map[string]any{"资产": large, "周期": "新周期"})
	start := time.Now()
	patch, err := socialStatePatch(before, after)
	if err != nil {
		t.Fatal(err)
	}
	encode := time.Since(start)
	if len(patch.set)*100 >= len(after) {
		t.Fatal("未显著缩小单字段周期更新的传输量")
	}
	if _, err = s.pool.Exec(ctx, `CREATE TABLE periodic_payload_probe(id int PRIMARY KEY, state jsonb NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `INSERT INTO periodic_payload_probe SELECT n,$1::jsonb FROM generate_series(1,24) n`, string(before)); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"整份", "差异"} {
		tx, e := s.pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		start = time.Now()
		for i := 1; i <= 24; i++ {
			if mode == "整份" {
				_, e = tx.Exec(ctx, `UPDATE periodic_payload_probe SET state=$2::jsonb WHERE id=$1`, i, string(after))
			} else {
				_, e = tx.Exec(ctx, `UPDATE periodic_payload_probe SET state=(state - $3::text[]) || $2::jsonb WHERE id=$1`, i, string(patch.set), patch.remove)
			}
			if e != nil {
				_ = tx.Rollback(ctx)
				t.Fatal(e)
			}
		}
		elapsed := time.Since(start)
		var matched int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM periodic_payload_probe WHERE state=$1::jsonb`, string(after)).Scan(&matched); e != nil || matched != 24 {
			_ = tx.Rollback(ctx)
			t.Fatal("差异与整份更新的数据库结果不一致", matched, e)
		}
		if e = tx.Rollback(ctx); e != nil {
			t.Fatal(e)
		}
		t.Logf("%s写入24行耗时=%s；原文=%d字节；差异=%d字节；单行差异生成=%s", mode, elapsed, len(after), len(patch.set), encode)
	}
}
