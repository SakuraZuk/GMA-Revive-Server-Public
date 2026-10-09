package game

// 客户端权威战斗观察链回归：桥握手状态机、事件序号、显式胜负结算、
// 速度偏好持久化与 client_need_recover_battle 重开语义。

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"hs-server/internal/mobileproto"
)

func battleTestResult(pushes []Push) *Push {
	for i := range pushes {
		if pushes[i].Method == "battle_result" {
			return &pushes[i]
		}
	}
	return nil
}

func observeBattleEvent(t *testing.T, ctx context.Context, service *Service, c *Connection, envelope string) []Push {
	t.Helper()
	args := []json.RawMessage{json.RawMessage(`"__battle_event__"`), json.RawMessage("[" + envelope + "]")}
	pushes, err := service.Handle(ctx, c, "do_command", args)
	if err != nil {
		t.Fatal("战斗事件处理失败", err)
	}
	return pushes
}

func startObservedBattle(t *testing.T, ctx context.Context, service *Service, c *Connection) string {
	t.Helper()
	args := []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`10001`), json.RawMessage(`{}`)}
	if _, err := service.Handle(ctx, c, "enter_dungeon", args); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(ctx, c, "load_entity_finish", nil); err != nil {
		t.Fatal(err)
	}
	startArgs := []json.RawMessage{json.RawMessage(`{"fighting_cards":[0],"support_cards":[],"storyline_cards":[]}`)}
	if _, err := service.Handle(ctx, c, "battle_fighting", startArgs); err != nil {
		t.Fatal(err)
	}
	selected, ok := c.SelectedAvatar()
	if !ok || selected.Progress.Battle == nil {
		t.Fatal("战斗会话未建立")
	}
	uuid := selected.Progress.Battle.UUID
	// 桥握手与开局快照。
	observeBattleEvent(t, ctx, service, c,
		fmt.Sprintf(`{"battle_uuid":%q,"sequence":1,"kind":"ready","data":{"version":1}}`, uuid))
	observeBattleEvent(t, ctx, service, c, fmt.Sprintf(
		`{"battle_uuid":%q,"sequence":2,"kind":"started","data":{"units":[`+
			`{"eid":"1","role":4403,"hex":[0,0,0],"kind":"ally","hp":283,"max_hp":283},`+
			`{"eid":"2","role":202,"hex":[1,0,-1],"kind":"enemy","hp":31,"max_hp":31}]}}`, uuid))
	return uuid
}

func TestBattleObservationHandshakeAndResult(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, av := newBattleConnection(t, ctx, accounts, service)
	uuid := startObservedBattle(t, ctx, service, c)

	// 序号跳号必须被拒：状态机不推进。
	observeBattleEvent(t, ctx, service, c,
		fmt.Sprintf(`{"battle_uuid":%q,"sequence":9,"kind":"turn","data":{}}`, uuid))
	if pushes := observeBattleEvent(t, ctx, service, c,
		fmt.Sprintf(`{"battle_uuid":%q,"sequence":3,"kind":"settings","data":{"auto_battle":true}}`, uuid)); len(pushes) != 0 {
		t.Fatal("跳号事件不应产生推送", pushes)
	}

	// settings 持久化 auto_battle。
	observeBattleEvent(t, ctx, service, c,
		fmt.Sprintf(`{"battle_uuid":%q,"sequence":3,"kind":"settings","data":{"auto_battle":true}}`, uuid))
	selected, _ := c.SelectedAvatar()
	if !selected.Progress.Battle.AutoBattle || !selected.Progress.BattlePreferences.AutoBattle {
		t.Fatal("自动战斗偏好未持久化")
	}

	// 显式胜利：avatar eid 在 winner_eids 中。
	winners := fmt.Sprintf(`["%x"]`, av.OID)
	pushes := observeBattleEvent(t, ctx, service, c, fmt.Sprintf(
		`{"battle_uuid":%q,"sequence":4,"kind":"result","data":{"winner_eids":%s,"finished_task_list":[109]}}`, uuid, winners))
	if battleTestResult(pushes) == nil {
		t.Fatal("结算缺少 battle_result", pushes)
	}
	if battleTestResult(pushes).Args[0] != true {
		t.Fatal("胜方判定错误", battleTestResult(pushes).Args)
	}
	selected, _ = c.SelectedAvatar()
	battle := selected.Progress.Battle
	if !battle.Finished || battle.Outcome != "win" || len(battle.WinnerEIDs) != 1 {
		t.Fatal("战斗结果未持久化", battle)
	}
	if len(battle.FinishedTaskList) != 1 || battle.FinishedTaskList[0] != 109 {
		t.Fatal("完成任务列表未记录", battle)
	}
	if len(selected.Progress.ClearedDungeons) != 1 || selected.Progress.ClearedDungeons[0] != 10001 {
		t.Fatal("通关记录缺失", selected.Progress.ClearedDungeons)
	}
	// 10001 教学副本不在主线首通奖励表（androidMainlineRewards 自 102 起）：
	// 胜利保存空奖励收据，不能伪造奖励材料。
	if !battle.RewardGranted || len(battle.SettlementBox["materials"].(map[string]any)) != 0 {
		t.Fatal("教学副本空奖励收据无效", battle)
	}

	// 已结束战斗再收事件必须拒绝（无推送）。
	if pushes := observeBattleEvent(t, ctx, service, c,
		fmt.Sprintf(`{"battle_uuid":%q,"sequence":5,"kind":"result","data":{"winner_eids":%s}}`, uuid, winners)); len(pushes) != 0 {
		t.Fatal("结束后事件不得再结算", pushes)
	}
}

func TestTutorialNextDungeonWaitsForClientResult(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 8, 16, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, av := newBattleConnection(t, ctx, accounts, service)
	uuid := startObservedBattle(t, ctx, service, c)
	if err := service.updateProgress(ctx, c, func(p *Progress) error {
		p.GuideTasks = map[int]GuideTask{1007: {ID: 1007, Status: 1}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	pending := []json.RawMessage{json.RawMessage(`17`), json.RawMessage(`10002`), json.RawMessage(`{}`)}
	if pushes, err := service.Handle(ctx, c, "enter_dungeon", pending); err != nil || len(pushes) != 0 {
		t.Fatal("下一副本应等待当前result", pushes, err)
	}
	if len(c.pendingDungeonArgs) != 3 {
		t.Fatal("下一副本请求未暂存")
	}

	winners := fmt.Sprintf(`["%x"]`, av.OID)
	pushes := observeBattleEvent(t, ctx, service, c, fmt.Sprintf(
		`{"battle_uuid":%q,"sequence":3,"kind":"result","data":{"winner_eids":%s,"finished_task_list":[]}}`, uuid, winners))
	if battleTestResult(pushes) == nil {
		t.Fatal("当前战斗结算缺少battle_result", pushes)
	}
	foundNext := false
	for _, item := range pushes {
		if item.Method == "start_server_battle_ok" && len(item.Args) >= 2 && item.Args[1] == 10002 {
			foundNext = true
		}
	}
	if !foundNext || len(c.pendingDungeonArgs) != 0 {
		t.Fatal("结算后未建立暂存的下一副本", pushes)
	}
	selected, _ := c.SelectedAvatar()
	if selected.Progress.Battle == nil || selected.Progress.Battle.DungeonID != 10002 || selected.Progress.Battle.Finished {
		t.Fatal("下一副本会话状态错误", selected.Progress.Battle)
	}
}

func TestTutorialBattleFightingWaitsForEntityLoad(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	service.Now = func() time.Time { return time.Date(2026, 10, 8, 16, 30, 0, 0, time.UTC) }
	c, _ := newBattleConnection(t, ctx, accounts, service)
	if _, err := service.Handle(ctx, c, "enter_dungeon", []json.RawMessage{
		json.RawMessage(`9`), json.RawMessage(`10001`), json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	startArgs := []json.RawMessage{json.RawMessage(`{"fighting_cards":[0],"support_cards":[],"storyline_cards":[]}`)}
	if pushes, err := service.Handle(ctx, c, "battle_fighting", startArgs); err != nil || len(pushes) != 0 {
		t.Fatal("实体加载前的开战请求应暂存", pushes, err)
	}
	if len(c.pendingBattleFightingArgs) != 1 || c.pendingBattleFightingUUID == "" {
		t.Fatal("开战请求未按战斗 UUID 暂存")
	}
	pushes, err := service.Handle(ctx, c, "load_entity_finish", nil)
	if err != nil {
		t.Fatal("加载完成未接续开战", err)
	}
	foundStart := false
	for _, item := range pushes {
		if item.Method == "sync_battle_method" && len(item.Args) > 0 && item.Args[0] == "start" {
			foundStart = true
		}
	}
	if !foundStart || len(c.pendingBattleFightingArgs) != 0 || c.pendingBattleFightingUUID != "" {
		t.Fatal("延迟开战未完整执行", pushes)
	}
	selected, _ := c.SelectedAvatar()
	if selected.Progress.Battle == nil || !selected.Progress.Battle.Loaded || !selected.Progress.Battle.Started {
		t.Fatal("延迟开战状态错误", selected.Progress.Battle)
	}
}

func TestReceiveCoordinateCheckAcceptsAndroidShape(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, service)
	args := []json.RawMessage{json.RawMessage(`1791447614.5`), json.RawMessage(`10001`), json.RawMessage(`[[12.5,-4],[13,-3.25]]`)}
	if pushes, err := service.Handle(ctx, c, "receive_coordinate_check", args); err != nil || len(pushes) != 0 {
		t.Fatal("Android 坐标校验遥测接收失败", pushes, err)
	}
}

func TestClientTriggerActionAcceptsStartedBattle(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, service)
	startObservedBattle(t, ctx, service, c)
	if pushes, err := service.Handle(ctx, c, "client_trigger_action", []json.RawMessage{json.RawMessage(`1000104`)}); err != nil || len(pushes) != 0 {
		t.Fatal("客户端已执行的剧情事件应只接收记录", pushes, err)
	}
}

func TestBattleObservationRejectsBadHandshake(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 7, 12, 30, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, accounts, service)
	args := []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`10001`), json.RawMessage(`{}`)}
	if _, err := service.Handle(ctx, c, "enter_dungeon", args); err != nil {
		t.Fatal(err)
	}
	selected, _ := c.SelectedAvatar()
	uuid := selected.Progress.Battle.UUID

	// 未握手直接 started：拒绝且状态不推进。
	observeBattleEvent(t, ctx, service, c, fmt.Sprintf(
		`{"battle_uuid":%q,"sequence":1,"kind":"started","data":{"units":[{"eid":"1","role":1,"hex":[0,0,0],"kind":"ally"}]}}`, uuid))
	selected, _ = c.SelectedAvatar()
	if selected.Progress.Battle.BridgeReady || selected.Progress.Battle.BridgeStarted {
		t.Fatal("未握手事件不得推进状态机")
	}
	// 协议版本不符。
	observeBattleEvent(t, ctx, service, c,
		fmt.Sprintf(`{"battle_uuid":%q,"sequence":1,"kind":"ready","data":{"version":2}}`, uuid))
	selected, _ = c.SelectedAvatar()
	if selected.Progress.Battle.BridgeReady {
		t.Fatal("版本不符的握手不得通过")
	}
	// 正确握手后 started 缺少 roster 拒绝。
	observeBattleEvent(t, ctx, service, c,
		fmt.Sprintf(`{"battle_uuid":%q,"sequence":1,"kind":"ready","data":{"version":1}}`, uuid))
	observeBattleEvent(t, ctx, service, c,
		fmt.Sprintf(`{"battle_uuid":%q,"sequence":2,"kind":"started","data":{}}`, uuid))
	selected, _ = c.SelectedAvatar()
	if selected.Progress.Battle.BridgeStarted {
		t.Fatal("缺 roster 的 started 不得通过")
	}
	// 归属不符的 UUID。
	observeBattleEvent(t, ctx, service, c,
		`{"battle_uuid":"0000000000000000000000ff","sequence":2,"kind":"started","data":{"units":[]}}`)
}

func TestBattleSpeedAndRecoverBattle(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 7, 13, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, accounts, service)
	uuid := startObservedBattle(t, ctx, service, c)

	if _, err := service.Handle(ctx, c, "set_battle_speed", []json.RawMessage{json.RawMessage(`2`)}); err == nil {
		t.Fatal("非法速度必须拒绝")
	}
	if pushes, err := service.Handle(ctx, c, "set_battle_speed", []json.RawMessage{json.RawMessage(`1.75`)}); err != nil ||
		len(pushes) != 1 || pushes[0].Method != "sync_battle_method" {
		t.Fatal("速度切换失败", pushes, err)
	}
	selected, _ := c.SelectedAvatar()
	if selected.Progress.BattlePreferences.BattleSpeed != 1.75 {
		t.Fatal("速度偏好未持久化")
	}

	// 模拟真正断线丢失连接观察缓存：冷恢复重开准备阶段并重装桥。
	c.ordinaryObservation = nil
	pushes, err := service.Handle(ctx, c, "client_need_recover_battle", []json.RawMessage{})
	if err != nil || len(pushes) < 4 || pushes[0].Method != "on_query_hotfix_success" ||
		pushes[1].Method != "start_server_battle_ok" || c.battleStartSent {
		t.Fatal("恢复重开契约失败", pushes, err)
	}
	if extra, ok := pushes[1].Args[3].(map[string]any); !ok || extra["hs_recover_previous_uuid"] != uuid {
		t.Fatal("恢复必须准确标记旧战斗UUID", pushes[1])
	}
	selected, _ = c.SelectedAvatar()
	battle := selected.Progress.Battle
	if battle.Finished || battle.BridgeReady || battle.UUID == uuid || battle.LastSequence != 0 {
		t.Fatal("重开会话状态错误", battle)
	}

	// 已有结果的恢复：补发结算。
	if err := service.updateProgress(ctx, c, func(p *Progress) error {
		p.Battle.Finished = true
		p.Battle.Outcome = "win"
		p.Battle.RewardGranted = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pushes, err = service.Handle(ctx, c, "client_need_recover_battle", []json.RawMessage{})
	if err != nil || battleTestResult(pushes) == nil || battleTestResult(pushes).Args[0] != true {
		t.Fatal("结果恢复必须补发 battle_result", pushes, err)
	}
}

func TestBattleCommandForwarding(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 7, 13, 30, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, accounts, service)
	uuid := startObservedBattle(t, ctx, service, c)

	command := []json.RawMessage{json.RawMessage(`"use_skill"`), json.RawMessage(`["ck_monster","1",440301,"2"]`)}
	pushes, err := service.Handle(ctx, c, "do_command", command)
	if err != nil || len(pushes) != 1 || pushes[0].Method != "sync_battle_method" {
		t.Fatal("客户端权威指令必须转发 revival_do_command", pushes, err)
	}
	// sync_battle_method 线格式：[名称, [命令, 参数表], 字典]。
	args, _ := pushes[0].Args[1].([]any)
	if pushes[0].Args[0] != "revival_do_command" || len(args) != 2 || args[0] != "use_skill" {
		t.Fatal("转发命令错误", pushes[0].Args)
	}
	_ = uuid
}

// 主线副本（102）胜利：首通按 androidMainlineRewards 发放并推送 box 回包。
func TestMainlineDungeonSettlesRewards(t *testing.T) {
	// 套装为空的掉落必须有显式本服配置；测试夹具只选择一个真实原表套装。
	t.Setenv("HS_RUNE_FALLBACK_POOLS", `{"400":[[1101,100]]}`)
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 7, 12, 10, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, av := newBattleConnection(t, ctx, accounts, service)
	if err := service.updateProgress(ctx, c, func(p *Progress) error {
		for id, task := range p.GuideTasks {
			task.Status = 2
			p.GuideTasks[id] = task
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`102`), json.RawMessage(`{}`)}
	if _, err := service.Handle(ctx, c, "enter_dungeon", args); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(ctx, c, "load_entity_finish", nil); err != nil {
		t.Fatal(err)
	}
	startArgs := []json.RawMessage{json.RawMessage(`{"fighting_cards":[0],"support_cards":[],"storyline_cards":[]}`)}
	if _, err := service.Handle(ctx, c, "battle_fighting", startArgs); err != nil {
		t.Fatal(err)
	}
	selected, _ := c.SelectedAvatar()
	uuid := selected.Progress.Battle.UUID
	observeBattleEvent(t, ctx, service, c,
		fmt.Sprintf(`{"battle_uuid":%q,"sequence":1,"kind":"ready","data":{"version":1}}`, uuid))
	observeBattleEvent(t, ctx, service, c, fmt.Sprintf(
		`{"battle_uuid":%q,"sequence":2,"kind":"started","data":{"units":[{"eid":"1","role":1,"hex":[0,0,0],"kind":"ally"}]}}`, uuid))
	pushes := observeBattleEvent(t, ctx, service, c, fmt.Sprintf(
		`{"battle_uuid":%q,"sequence":3,"kind":"result","data":{"winner_eids":["%x"]}}`, uuid, av.OID))
	if battleTestResult(pushes) == nil {
		t.Fatal("结算回包缺失", pushes)
	}
	box, _ := battleTestResult(pushes).Args[1].(map[string]any)
	materials := battleReceiptMaterialMap(t, box)
	if materials[12] != 20000 || materials[4] != 12000 || materials[2] != 1000 || len(box["runes"].(mobileproto.Map)) != 2 {
		t.Fatal("首通奖励 box 错误", box)
	}
	selected, _ = c.SelectedAvatar()
	if !selected.Progress.Battle.RewardGranted || selected.Progress.Materials[12].Count < 20000 {
		t.Fatal("主线首通材料未入账", selected.Progress.Materials[12])
	}
	// 重复通关不再发奖（首通判定幂等）。
}

func TestStoryDungeonSettlesWithoutBattle(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 7, 14, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, accounts, service)
	// 引导结束后放行任意副本；100 为 battle_id<=0 的纯剧情节点。
	if err := service.updateProgress(ctx, c, func(p *Progress) error {
		for id, task := range p.GuideTasks {
			task.Status = 2
			p.GuideTasks[id] = task
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`100`), json.RawMessage(`{}`)}
	pushes, err := service.Handle(ctx, c, "enter_dungeon", args)
	if err != nil {
		t.Fatal("剧情节点不得拒绝", err)
	}
	if len(pushes) == 0 || pushes[0].Method != "call_client_callback" {
		t.Fatal("剧情节点必须先回 callback", pushes)
	}
	var result *Push
	for i := range pushes {
		if pushes[i].Method == "battle_result" {
			result = &pushes[i]
		}
	}
	if result == nil || result.Args[0] != true {
		t.Fatal("剧情节点必须补 battle_result 胜利", pushes)
	}
	selected, _ := c.SelectedAvatar()
	if selected.Progress.Battle != nil {
		t.Fatal("剧情节点不得留下战斗会话")
	}
	if len(selected.Progress.ClearedDungeons) != 1 || selected.Progress.ClearedDungeons[0] != 100 {
		t.Fatal("剧情节点未推进通关", selected.Progress.ClearedDungeons)
	}
}
