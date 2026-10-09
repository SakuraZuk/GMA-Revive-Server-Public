package dbstore

import (
	"context"
	"encoding/json"
	"hs-server/internal/game"
	"math"
	"reflect"
	"testing"
	"time"
)

// 原生业务响应按方法定位，合法的附加属性推送不一定在响应之前。
func pgNativeReply(t *testing.T, out []game.Push, method string) (game.Push, int) {
	t.Helper()
	index := -1
	for i, item := range out {
		if item.Method != method {
			continue
		}
		if index >= 0 {
			t.Fatal("原生业务回包重复", method)
		}
		index = i
	}
	if index < 0 {
		t.Fatal("缺少原生业务回包", method, out)
	}
	return out[index], index
}

func pgNativePropertyBefore(t *testing.T, out []game.Push, replyIndex int, field string) {
	t.Helper()
	for i, item := range out {
		if item.Method != "client_prop_changed" || len(item.Args) != 1 {
			continue
		}
		property, ok := item.Args[0].([]any)
		if ok && len(property) == 2 && property[0] == field && i < replyIndex {
			return
		}
	}
	t.Fatal("原生资产或状态必须先于业务回包同步", field, replyIndex)
}

func TestPostgresCollectionNativeUnlockCraftGatherAndRollback(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	info := game.ClientInfo{Account: "收藏加工事务", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, store, info, &now)
	defer svc.Detach(c)
	read := func() game.Progress {
		t.Helper()
		value, err := New(store.pool).AdminPlayer(ctx, av.OID)
		if err != nil {
			t.Fatal(err)
		}
		return game.CloneProgress(value.Progress)
	}
	old19 := read().Materials[19].Count
	_, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Materials[809] = game.Material{ID: 809, Count: 1, Total: 1}
		p.Materials[801] = game.Material{ID: 801, Count: 1, Total: 1}
		p.Materials[110] = game.Material{ID: 110, Count: 12, Total: 12}
		p.Materials[12] = game.Material{ID: 12, Count: 1000, Total: 1000}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rpc := func(name string, values ...any) []game.Push {
		t.Helper()
		args := []json.RawMessage{}
		for _, v := range values {
			raw, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			args = append(args, raw)
		}
		out, err := svc.Handle(ctx, c, name, args)
		if err != nil {
			t.Fatal(name, err)
		}
		return out
	}
	for _, step := range []struct {
		name, state string
		id, argc    int
	}{
		{"unlock_dormitory", "house_unlock_state", 809, 2},
		{"unlock_facility", "facility_info", 801, 3},
	} {
		out := rpc(step.name, step.id)
		reply, at := pgNativeReply(t, out, "on_"+step.name)
		if len(reply.Args) != step.argc || reply.Args[0] != game.RetSuccess || reply.Args[1] != step.id {
			t.Fatal("收藏室原生解锁回包错误", reply)
		}
		pgNativePropertyBefore(t, out, at, "material_mgr")
		pgNativePropertyBefore(t, out, at, step.state)
	}
	_, err = store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Collection.Facilities[7] = game.CollectionFacility{ID: 7, Level: 2}
		p.Collection.Facilities[2] = game.CollectionFacility{ID: 2, Level: 1, ProduceStart: float64(now.Unix()), Keep: 1.75}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out := rpc("compose_material", 7, 111, 2)
	reply, at := pgNativeReply(t, out, "call_client_callback")
	if len(reply.Args) != 2 || reply.Args[0] != 7 || len(reply.Args[1].([]any)) != 2 || reply.Args[1].([]any)[0] == nil {
		t.Fatal("加工成功没有原生奖励box", reply)
	}
	pgNativePropertyBefore(t, out, at, "material_mgr")
	out = rpc("gather_produce_material", 2)
	reply, at = pgNativeReply(t, out, "on_gather_produce_material")
	if len(reply.Args) != 4 || reply.Args[0] != game.RetSuccess || reply.Args[1] != 2 {
		t.Fatal("设施收获原生回包错误", reply)
	}
	pgNativePropertyBefore(t, out, at, "material_mgr")
	pgNativePropertyBefore(t, out, at, "facility_info")
	p := read()
	if len(p.Collection.Rooms) != 1 || p.Materials[809].Count != 0 || p.Materials[801].Count != 0 || p.Materials[111].Count != 2 || p.Materials[110].Count != 0 || p.Materials[12].Count != 800 || p.Materials[19].Count != old19+1 || math.Abs(p.Collection.Facilities[2].Keep-.75) > 1e-9 {
		t.Fatal("收藏室/加工/生产资产没有在真实数据库保存")
	}
	out = rpc("compose_material", 8, 111, 2)
	reply, _ = pgNativeReply(t, out, "call_client_callback")
	if reply.Args[0] != 8 || reply.Args[1].([]any)[0] != nil {
		t.Fatal("材料不足仍合成成功", reply)
	}
	if !reflect.DeepEqual(p, read()) {
		t.Fatal("加工不足未整进度回滚")
	}
	out = rpc("gather_produce_material", 2)
	reply, _ = pgNativeReply(t, out, "on_gather_produce_material")
	if reply.Args[0] == game.RetSuccess {
		t.Fatal("小数库存重复领取成功")
	}
	if !reflect.DeepEqual(p, read()) {
		t.Fatal("重复收获改变进度或资产")
	}
	_, err = store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Materials[19] = game.Material{ID: 19, Count: math.MaxInt64, Total: math.MaxInt64}
		f := p.Collection.Facilities[2]
		f.Keep = 1.75
		p.Collection.Facilities[2] = f
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before := read()
	if _, err = svc.Handle(ctx, c, "gather_produce_material", []json.RawMessage{json.RawMessage(`2`)}); err == nil {
		t.Fatal("收获材料溢出没有明确失败")
	}
	if !reflect.DeepEqual(before, read()) {
		t.Fatal("收获溢出未整进度回滚")
	}
}
