package game

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestRepeatedActivityEntryPreservesPreparingBattle(t *testing.T) {
	for _, id := range []int{4101, 4102} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			ctx := context.Background()
			accounts := NewFixtureAccounts(nil)
			svc := New(accounts, nil)
			c, av := newBattleConnection(t, ctx, accounts, svc)
			_, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
				p.Battle = &BattleSession{UUID: "00112233445566778899aabb", DungeonID: id, BattleID: id, Seed: 37, Status: "准备", Loaded: true, PaidPower: 12}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			c.ordinaryPrepareUUID = "00112233445566778899aabb"
			for repeat := 0; repeat < 3; repeat++ {
				out, err := svc.Handle(ctx, c, "enter_dungeon", socialArgs(9, id, map[string]any{}))
				if err != nil || len(out) != 1 || out[0].Method != "call_client_callback" {
					t.Fatal("重复活动重新创建或prepare", err, out)
				}
				b := c.SelectedAvatarUnsafe().Progress.Battle
				if !b.Loaded || b.Seed != 37 || b.PaidPower != 12 || b.UUID != c.ordinaryPrepareUUID {
					t.Fatal("重复活动改写既有会话", b)
				}
			}
		})
	}
}

func TestGuideDungeonSessionAndRequestOrder(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	identity, err := accounts.Register(ctx, ClientInfo{Account: "教学测试", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	av, err := accounts.SetNicknameGender(ctx, identity.Avatars[0].OID, "测试馆长", 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error { return AdvanceGuide(p, 1000, av.CreatedAt, 1) })
	if err != nil {
		t.Fatal(err)
	}
	c := NewConnection()
	c.identity = Identity{Account: identity.Account, Avatars: []Avatar{av}}
	c.hostnum = 1
	c.phase = Playing
	service := New(accounts, nil)
	now := time.Date(2026, 10, 5, 18, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	args := []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`10001`), json.RawMessage(`{}`)}
	// 数据库仍在 1001 时，客户端已经启动 1002 并先发 enter_dungeon；允许紧邻的下一任务。
	// 客户端权威契约：桥脚本安装 → callback 成功 → start_server_battle_ok → prepare → 偏好同步。
	pushes, err := service.Handle(ctx, c, "enter_dungeon", args)
	if err != nil || len(pushes) != 6 || pushes[0].Method != "on_query_hotfix_success" ||
		pushes[1].Method != "call_client_callback" || pushes[2].Method != "start_server_battle_ok" ||
		pushes[3].Method != "sync_battle_method" || pushes[4].Method != "sync_battle_method" {
		t.Fatal("教学入口契约错误", pushes, err)
	}
	prepareArgs, ok := pushes[3].Args[1].([]any)
	if !ok || len(prepareArgs) != 4 {
		t.Fatal("prepare 必须下发四个位置参数，不能再嵌套一层列表", pushes[3].Args)
	}
	players, ok := prepareArgs[0].([]any)
	if !ok || len(players) != 1 || players[0] != ObjectID(fmt.Sprintf("%x", av.OID)) {
		t.Fatal("prepare 玩家列表错误", prepareArgs)
	}
	if _, nested := prepareArgs[0].([]any); !nested {
		t.Fatal("prepare 首参必须是玩家列表", prepareArgs)
	}
	selected, _ := c.SelectedAvatar()
	first := *selected.Progress.Battle
	row, ok := pushes[5].Args[0].([]any)
	if pushes[5].Method != "client_prop_set" || !ok || len(row) != 3 || row[0] != "dungeon_mgr" || row[1] != 10001 || row[2].(map[string]any)["finished"] != 0 {
		t.Fatal("未通关副本退出界面缺少原生实体", pushes[5])
	}
	cold, loginErr := accounts.QuickLogin(ctx, ClientInfo{Account: "教学测试", Password: "pw", Hostnum: 1})
	if loginErr != nil || cold.Avatars[0].InitialProperties("教学测试")["dungeon_mgr"].(map[string]any)["10001"].(map[string]any)["finished"] != 0 {
		t.Fatal("冷登录未结副本退出实体丢失或误标通关", loginErr)
	}
	if first.DungeonID != 10001 || len(first.UUID) != 24 || first.Status != "准备" {
		t.Fatal("数据库会话错误", first)
	}
	if repeated, repeatErr := service.Handle(ctx, c, "enter_dungeon", args); repeatErr != nil || len(repeated) != 1 || repeated[0].Method != "call_client_callback" {
		t.Fatal("重复入场必须只回复callback，不能重复创建客户端战斗", repeated, repeatErr)
	}
	again, _ := c.SelectedAvatar()
	if again.Progress.Battle.UUID != first.UUID || again.Progress.Battle.Seed != first.Seed || again.Progress.Battle.CreatedAt != first.CreatedAt {
		t.Fatal("重复请求重建战斗或种子")
	}
	args[1] = json.RawMessage(`10002`)
	if _, err = service.Handle(ctx, c, "enter_dungeon", args); err == nil {
		t.Fatal("越级进入副本")
	}
	args[1] = json.RawMessage(`999999`)
	if _, err = service.Handle(ctx, c, "enter_dungeon", args); err == nil {
		t.Fatal("未知副本获准")
	}
	startArgs := []json.RawMessage{json.RawMessage(`{"fighting_cards":[0],"support_cards":[],"storyline_cards":[]}`)}
	if deferred, err := service.Handle(ctx, c, "battle_fighting", startArgs); err != nil || len(deferred) != 0 {
		t.Fatal("未加载的开战请求应暂存", deferred, err)
	}
	start, err := service.Handle(ctx, c, "load_entity_finish", nil)
	if err != nil || len(start) != 4 {
		t.Fatal("加载完成后未接续暂存开战请求", start, err)
	}
	for index, method := range []string{"add_fighting_cards", "battle_fighting", "start", "check_on_battle_start"} {
		if start[index].Args[0] != method {
			t.Fatal("出战数据必须先于玩家创建及开战", start)
		}
	}
	initArgs := start[0].Args[1].([]any)
	if initArgs[2] != ObjectID(fmt.Sprintf("%x", av.OID)) || len(initArgs[0].([]any)) != 0 {
		t.Fatal("固定教学角色不得伪造成拥有卡", initArgs)
	}
	if markers := initArgs[1].([]any); len(markers) != 2 || markers[0] != "card.card_list" || markers[1] != "__custom_type" {
		t.Fatal("客户端需要 card_list 实例而非普通列表", initArgs)
	}
	if repeat, err := service.Handle(ctx, c, "battle_fighting", startArgs); err != nil || len(repeat) != 0 {
		t.Fatal("同连接重复开战")
	}
	if pending, err := service.Tick(ctx, c); err != nil || len(pending) != 0 {
		t.Fatal("入场演出尚未结束，不得启动", pending, err)
	}
	// 客户端权威：战斗时钟由客户端本地驱动，服务端 Tick 不再推进
	// 入场演出/回合交接；任何时间点的 Tick 都不得产出战斗跳转。
	now = now.Add(2500 * time.Millisecond)
	if boot, err := service.Tick(ctx, c); err != nil || len(boot) != 0 {
		t.Fatal("客户端权威下 Tick 不得推进战斗", boot, err)
	}
	startArgs[0] = json.RawMessage(`{"fighting_cards":[777]}`)
	if _, err = service.Handle(ctx, c, "battle_fighting", startArgs); err == nil {
		t.Fatal("伪造阵容获准")
	}
}
