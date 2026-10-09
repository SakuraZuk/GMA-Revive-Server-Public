package game

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/mobileproto"
)

func battleReceiptMaterialMap(t *testing.T, box map[string]any) map[int]int64 {
	t.Helper()
	pairs, ok := box["materials"].(mobileproto.Map)
	if !ok {
		t.Fatal("奖励材料没有原生整数键", box)
	}
	out := map[int]int64{}
	for _, pair := range pairs {
		id, ok := pair.Key.(int)
		if !ok {
			t.Fatal("材料键不是整数", pair)
		}
		amount, ok := pair.Value.(int64)
		if !ok {
			t.Fatal("材料数量不是整数", pair)
		}
		out[id] = amount
	}
	return out
}

func settlementRecoverySnapshot() map[string]any {
	cardID, runeID := "00112233445566778899aabb", "00112233445566778899aabc"
	return map[string]any{
		"__custom_type": "box.box", "materials": map[int]int64{12: 9007199254740993, 3: 10},
		"cards":             []any{map[string]any{"uuid": ObjectID(cardID), "card_id": 4401, "level": 1, "grow_materials": map[int]int64{}, "embed_runes": map[string]any{}}},
		"runes":             map[string]any{runeID: map[string]any{"uuid": runeID, "card_uuid": cardID, "create_time": float64(1791360000), "extra_attrs_factor": map[int]int{1001: 3}, "base_attrs": []int{1001}}},
		"card_exp_receiver": []any{ObjectID(cardID)},
	}
}

func TestBattleSettlementJSONRestoresOriginalNativeTypesAndPrecision(t *testing.T) {
	box := settlementRecoverySnapshot()
	b := &BattleSession{UUID: "00112233445566778899aabd", DungeonID: 102, Finished: true, Outcome: "win", SettlementBox: box}
	fresh, err := battleSettlementBoxWire(box)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var restored BattleSession
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	after, err := battleSettlementBoxWire(restored.SettlementBox)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fresh, after) {
		t.Fatal("JSON重载改变原盒或wire类型", fresh, after)
	}
	if battleReceiptMaterialMap(t, after)[12] != 9007199254740993 {
		t.Fatal("整数奖励被float64截断")
	}
	cards := after["cards"].([]any)
	if _, ok := cards[0].(map[string]any)["uuid"].(ObjectID); !ok {
		t.Fatal("卡UUID没有恢复ObjectID")
	}
	runes := after["runes"].(mobileproto.Map)
	if _, ok := runes[0].Key.(ObjectID); !ok {
		t.Fatal("契印字典没有ObjectID键")
	}
	rune := runes[0].Value.(map[string]any)
	if _, ok := rune["card_uuid"].(ObjectID); !ok {
		t.Fatal("契印归属没有ObjectID")
	}
	if _, ok := rune["create_time"].(float64); !ok {
		t.Fatal("原生Float创建时间被整数化")
	}
	if _, ok := after["card_exp_receiver"].([]any)[0].(ObjectID); !ok {
		t.Fatal("经验接收者没有ObjectID")
	}
	before, _ := json.Marshal(restored.SettlementBox)
	if _, err = battleSettlementBoxWire(restored.SettlementBox); err != nil {
		t.Fatal(err)
	}
	unchanged, _ := json.Marshal(restored.SettlementBox)
	if string(before) != string(unchanged) {
		t.Fatal("wire生成修改了原始收据")
	}
}

func TestBattleFinishedRecoveryUsesCommittedProgressAndOriginalBox(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 8, 1, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, av := newBattleConnection(t, ctx, accounts, service)
	// 外部事务先写最终奖励和结果，保持此连接的旧余额，复现恢复推旧余额的错误。
	if _, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Materials[12] = Material{ID: 12, Count: 34567, Total: 45678}
		p.Battle = &BattleSession{UUID: "00112233445566778899aabd", DungeonID: 102, Finished: true, Outcome: "win", RewardGranted: true,
			FinishedTaskList: []int{109}, SettlementBox: settlementRecoverySnapshot()}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pushes, err := service.Handle(ctx, c, "client_need_recover_battle", nil)
	if err != nil {
		t.Fatal(err)
	}
	result := battleTestResult(pushes)
	if result == nil || result.Args[0] != true {
		t.Fatal("恢复没有补原结果", pushes)
	}
	box := result.Args[1].(map[string]any)
	if battleReceiptMaterialMap(t, box)[12] != 9007199254740993 {
		t.Fatal("恢复重新生成了奖励")
	}
	foundLatest := false
	for _, item := range pushes {
		if item.Method != "client_prop_changed" {
			continue
		}
		property, ok := item.Args[0].([]any)
		if !ok || len(property) != 2 || property[0] != "material_mgr" {
			continue
		}
		material := property[1].(map[string]any)["12"].(map[string]any)
		if material["count"] != int64(34567) || material["total"] != int64(45678) {
			t.Fatal("恢复覆盖了最新余额", material)
		}
		foundLatest = true
	}
	if !foundLatest {
		t.Fatal("恢复没有资产同步")
	}
	extra := result.Args[3].(map[string]any)
	if !reflect.DeepEqual(extra["task_statistics"].(map[string]any)["tower_finished_task"], []int{109}) {
		t.Fatal("恢复漏掉原任务统计")
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if _, err = service.Handle(ctx, c, "client_need_recover_battle", nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("重复恢复改变了奖励、结果或资产")
	}
}

func TestFreshLoginRecoversUnfinishedBattleBeforeGuideRefresh(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 8, 7, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, accounts, service)
	startGuideBattle(t, ctx, service, c, &now)
	oldUUID := c.SelectedAvatarUnsafe().Progress.Battle.UUID

	pushes, err := service.Handle(ctx, c, "set_reconnect_auth_msg", rawArgs("新进程重连凭证"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pushes) == 0 {
		t.Fatal("新进程登录没有下发战斗恢复链")
	}
	if pushes[0].Method != "on_query_hotfix_success" {
		t.Fatal("恢复链必须先确保客户端桥脚本", pushes)
	}
	foundStart := false
	for _, item := range pushes {
		if item.Method == "start_server_battle_ok" {
			foundStart = true
		}
	}
	if !foundStart {
		t.Fatal("恢复链没有重新创建客户端战斗", pushes)
	}
	if pushes[len(pushes)-1].Method != "on_refresh_login" {
		t.Fatal("恢复链末尾必须执行原生登录收尾以清除遮罩", pushes)
	}
	if got := c.SelectedAvatarUnsafe().Progress.Battle.UUID; got == oldUUID {
		t.Fatal("未结战斗恢复没有更换UUID", got)
	}

	pushes, err = service.Handle(ctx, c, "set_reconnect_auth_msg", rawArgs("重复凭证"))
	if err != nil || len(pushes) != 0 {
		t.Fatalf("同一连接不应重复恢复：%v %v", pushes, err)
	}
}

func TestFreshLoginFinishedOrdinaryBattleUsesCommittedSnapshot(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 8, 7, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, accounts, service)
	startGuideBattle(t, ctx, service, c, &now)
	if err := service.updateProgress(ctx, c, func(p *Progress) error {
		p.Battle.Finished = true
		p.Battle.Outcome = "win"
		p.Battle.SettlementBox = settlementRecoverySnapshot()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	pushes, err := service.Handle(ctx, c, "set_reconnect_auth_msg", rawArgs("结算后冷登录凭证"))
	if err != nil || len(pushes) != 1 || pushes[0].Method != "on_refresh_login" {
		t.Fatal("已结算冷登录只能收尾，不能缓存无实体结果或重开战斗", pushes, err)
	}
	if !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("冷登录不得改写已提交资产或原结算收据")
	}
}

func TestBattleMainline102UnconfiguredRunePoolRollsBackWholeReward(t *testing.T) {
	t.Setenv("HS_RUNE_FALLBACK_POOLS", "")
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, service)
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	err := service.updateProgress(ctx, c, func(p *Progress) error {
		b := &BattleSession{DungeonID: 102, UUID: "00112233445566778899aabd"}
		_, err := settleOrdinaryDungeonRewards(p, b, true, service.Now())
		return err
	})
	if err == nil {
		t.Fatal("原生全零池不得自动启用默认套装")
	}
	if !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("drop400失败后保留了固定奖励或进度")
	}
}

// 实际日志中的材料副本/主线完成态曾只发旧结算，冷客户端一直停登录页。
func TestColdLoginCompletedOrdinaryDungeonsAlwaysFinishLogin(t *testing.T) {
	for _, dungeon := range []int{514, 602, 4101, 4102, 4202, 4402} {
		t.Run(fmt.Sprintf("副本%d", dungeon), func(t *testing.T) {
			ctx := context.Background()
			accounts := NewFixtureAccounts(nil)
			service := New(accounts, nil)
			now := time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)
			service.Now = func() time.Time { return now }
			c, _ := newBattleConnection(t, ctx, accounts, service)
			startGuideBattle(t, ctx, service, c, &now)
			if err := service.updateProgress(ctx, c, func(p *Progress) error {
				p.Battle.DungeonID = dungeon
				p.Battle.Finished, p.Battle.Outcome = true, "win"
				p.Battle.SettlementBox = settlementRecoverySnapshot()
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
			out, err := service.Handle(ctx, c, "set_reconnect_auth_msg", rawArgs("已结算冷登录"))
			if err != nil || len(out) != 1 || out[0].Method != "on_refresh_login" || !c.refreshLoginSent {
				t.Fatal("已完成普通副本没有正常进入大厅", dungeon, err, out)
			}
			if !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
				t.Fatal("改写了已结算资产或收据")
			}
		})
	}
}

func TestColdLoginRecoveryFailureKeepsLoginFinishRetryable(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, accounts, service)
	startGuideBattle(t, ctx, service, c, &now)
	store := &observedWriteStore{FixtureAccounts: accounts, FailNext: true}
	service.Accounts = store
	if _, err := service.Handle(ctx, c, "set_reconnect_auth_msg", rawArgs("恢复失败凭据")); err == nil || c.refreshLoginSent {
		t.Fatal("恢复失败提前标记已完成登录", err)
	}
	if out, err := service.Tick(ctx, c); err != nil || len(out) != 0 {
		t.Fatal("登录恢复失败没有遵守重试间隔", err, out)
	}
	now = now.Add(2 * time.Second)
	out, err := service.Tick(ctx, c)
	if err != nil || len(out) == 0 || out[len(out)-1].Method != "on_refresh_login" || !c.refreshLoginSent {
		t.Fatal("恢复后仍不能正常完成登录", err, out)
	}
}
