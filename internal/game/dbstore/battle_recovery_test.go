package dbstore

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/game"
	"hs-server/internal/hotfix"
	"hs-server/internal/mobileproto"
)

// 与既有repairPlayer的协议阶段一致；重登录必须新建连接，不能对Playing连接再次quick_login。
// 每次用独立Store/Service读同一PG，非空热更目录和BecomePlayer保留真实登录初始化。
func loginPersistedProgressPlayer(t *testing.T, store *Store, info game.ClientInfo, now time.Time) (*game.Service, *game.Connection) {
	t.Helper()
	svc := game.New(New(store.pool), &hotfix.Catalog{})
	svc.Now = func() time.Time { return now }
	c := game.NewConnection()
	t.Cleanup(func() { svc.Detach(c) })
	login, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(context.Background(), c, "quick_login", []json.RawMessage{login}); err != nil {
		t.Fatal("PG协议登录失败", err)
	}
	if c.Phase() != game.Authenticated {
		t.Fatal("PG登录未完成认证阶段", c.Phase())
	}
	if _, err = svc.BecomePlayer(c); err != nil {
		t.Fatal("PG进入玩家阶段失败", err)
	}
	if c.Phase() != game.Playing {
		t.Fatal("PG协议登录未进入玩家阶段", c.Phase())
	}
	return svc, c
}

func TestPostgresFinishedBattleRecoveryOriginalBoxAndLatestAssets(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "已结算恢复原盒验收", Password: "pw", Hostnum: 1}
	id, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	oid := id.Avatars[0].OID
	now := time.Date(2026, 10, 8, 1, 0, 0, 0, time.UTC)
	svc, c := loginPersistedProgressPlayer(t, store, info, now)
	cardID, runeID := "00112233445566778899aabb", "00112233445566778899aabc"
	// 此连接先登录，再由另一存储对象提交新资产；恢复必须读取PG最新JSONB。
	if _, err = New(store.pool).UpdateProgress(ctx, oid, func(p *game.Progress) error {
		p.Materials[12] = game.Material{ID: 12, Count: 34567, Total: 45678}
		p.Battle = &game.BattleSession{UUID: "00112233445566778899aabd", DungeonID: 102, Finished: true, Outcome: "win", RewardGranted: true,
			FinishedTaskList: []int{109}, SettlementBox: map[string]any{
				"__custom_type": "box.box", "materials": map[int]int64{12: 9007199254740993, 3: 10},
				"cards":             []any{map[string]any{"uuid": game.ObjectID(cardID), "card_id": 4401, "level": 1}},
				"runes":             map[string]any{runeID: map[string]any{"uuid": runeID, "card_uuid": nil, "create_time": float64(1791360000), "extra_attrs_factor": map[int]int{1001: 3}}},
				"card_exp_receiver": []any{game.ObjectID(cardID)},
			}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var storedAmount string
	if err = store.pool.QueryRow(ctx, `SELECT state->'server_battle'->'settlement_box'->'materials'->>'12' FROM avatar_progress WHERE avatar_oid=$1`, oid).Scan(&storedAmount); err != nil {
		t.Fatal("JSONB原整数收据无法读取", err)
	}
	if storedAmount != "9007199254740993" {
		t.Fatal("JSONB入库存量已被舍入", storedAmount)
	}
	pushes, err := svc.Handle(ctx, c, "client_need_recover_battle", nil)
	if err != nil {
		t.Fatal(err)
	}
	var result *game.Push
	latest := false
	for index := range pushes {
		item := &pushes[index]
		if item.Method == "battle_result" {
			result = item
		}
		if item.Method != "client_prop_changed" {
			continue
		}
		property, ok := item.Args[0].([]any)
		if !ok || len(property) != 2 || property[0] != "material_mgr" {
			continue
		}
		material := property[1].(map[string]any)["12"].(map[string]any)
		if material["count"] != int64(34567) || material["total"] != int64(45678) {
			t.Fatal("PG恢复向客户端推了旧余额", material)
		}
		latest = true
	}
	if result == nil || result.Args[0] != true || !latest {
		t.Fatal("PG恢复缺原结果或最新属性", pushes)
	}
	box := result.Args[1].(map[string]any)
	amountOK := false
	for _, pair := range box["materials"].(mobileproto.Map) {
		if pair.Key == 12 && pair.Value == int64(9007199254740993) {
			amountOK = true
		}
	}
	if !amountOK {
		t.Fatal("PG恢复截断整数收据", box)
	}
	if box["cards"].([]any)[0].(map[string]any)["uuid"] != game.ObjectID(cardID) {
		t.Fatal("卡ObjectID没有恢复")
	}
	runes := box["runes"].(mobileproto.Map)
	if len(runes) != 1 || runes[0].Key != game.ObjectID(runeID) {
		t.Fatal("契印ObjectID键没有恢复", runes)
	}
	rune := runes[0].Value.(map[string]any)
	if rune["uuid"] != game.ObjectID(runeID) {
		t.Fatal("契印ObjectID没有恢复")
	}
	if _, ok := rune["create_time"].(float64); !ok {
		t.Fatal("原生Float创建时间被整数化")
	}
	if box["card_exp_receiver"].([]any)[0] != game.ObjectID(cardID) {
		t.Fatal("经验接收ObjectID没有恢复")
	}
	before := game.CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if _, err = svc.Handle(ctx, c, "client_need_recover_battle", nil); err != nil {
		t.Fatal(err)
	}
	after, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil || !reflect.DeepEqual(before, after.Avatars[0].Progress) {
		t.Fatal("重复恢复重抽、发奖或修改了存档", err)
	}
	svc.Detach(c)
	_, reconnected := loginPersistedProgressPlayer(t, store, info, now)
	if !reflect.DeepEqual(before, reconnected.SelectedAvatarUnsafe().Progress) {
		t.Fatal("PG原生登录重载改变已结算收据或资产")
	}
}
