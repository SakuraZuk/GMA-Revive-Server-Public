package nativeengine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func nativeTestConfig(t *testing.T) Config {
	t.Helper()
	python := os.Getenv("HS_NATIVE_PVP_TEST_PYTHON2")
	if python == "" {
		t.Skip("未指定隔离Python2，仅当次真实引擎专项设置此变量")
	}
	base, err := filepath.Abs("../nativepvp")
	if err != nil {
		t.Fatal(err)
	}
	return Config{Python: python, Worker: filepath.Join(base, "battle_native_worker.py"), Directory: filepath.Join(base, "runtime/native_engine")}
}

func TestNativeProcessActualDuelRejectionAndDeterministicFinish(t *testing.T) {
	config := nativeTestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	duel := func(auto bool) json.RawMessage {
		p, err := Open(ctx, config)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		ids := []int{1601, 2103, 2202, 4402}
		roles, err := p.Request(ctx, map[string]any{"operation": "table", "table": "role_info", "keys": ids, "fields": []string{"skill_list"}})
		if err != nil {
			t.Fatal(err)
		}
		var table map[int]struct {
			Skills []int `json:"skill_list"`
		}
		if err := json.Unmarshal(roles, &table); err != nil {
			t.Fatal(err)
		}
		roster := func(side int) []any {
			rows := []any{}
			for i, id := range ids {
				skills := []any{}
				for _, skill := range table[id].Skills {
					skills = append(skills, map[string]any{"skill_id": skill, "level": 1, "enhance_level": 0})
				}
				rows = append(rows, map[string]any{"card_id": id, "uuid": []string{"100000000000000000000001", "100000000000000000000002", "100000000000000000000003", "100000000000000000000004", "200000000000000000000001", "200000000000000000000002", "200000000000000000000003", "200000000000000000000004"}[side*4+i], "level": 40, "grade": 4, "awakened": 1, "skill_mgr": skills})
			}
			return rows
		}
		metadata := map[string]any{"dungeon_id": 21, "avatar_id": "000000000000000000000001", "enemy_id": "000000000000000000000002", "enemy_roster": roster(1), "battle_type": 2, "enemy_auto": auto}
		if _, err = p.Request(ctx, map[string]any{"operation": "start", "metadata": metadata, "roster": roster(0), "seed": 123456, "auto": auto}); err != nil {
			t.Fatal(err)
		}
		if !auto {
			if _, err = p.Request(ctx, map[string]any{"operation": "drive"}); err != nil {
				t.Fatal(err)
			}
			before, err := p.Request(ctx, map[string]any{"operation": "snapshot"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Request(ctx, map[string]any{"operation": "step", "command": map[string]any{"name": "move_to", "args": []any{"不存在的单位", []int{0, 0, 0}}}})
			var reject *Rejection
			if !errors.As(err, &reject) {
				t.Fatal("原生拒绝没有保持可恢复类别", err)
			}
			after, err := p.Request(ctx, map[string]any{"operation": "snapshot"})
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("拒绝输入改变原生状态或废弃进程", err)
			}
			return after
		}
		final, err := p.Request(ctx, map[string]any{"operation": "autoplay", "max_steps": 10000})
		if err != nil {
			t.Fatal(err)
		}
		var outcome struct {
			Status string `json:"status"`
			Result *struct {
				Winners []string `json:"winner_eids"`
			} `json:"result"`
		}
		if err = json.Unmarshal(final, &outcome); err != nil || outcome.Status != "finished" || outcome.Result == nil || len(outcome.Result.Winners) != 1 {
			t.Fatal("原生双阵容未真实结束", err)
		}
		return final
	}
	first, second := duel(true), duel(true)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("同阵容同seed原生权威结果不确定")
	}
	duel(false)
}
