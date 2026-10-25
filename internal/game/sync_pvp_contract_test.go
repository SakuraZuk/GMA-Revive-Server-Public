package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func newSyncPVPTestService() (*Service, *Connection) {
	now := time.Unix(1000, 0)
	progress := NewProgress(1, now)
	info := DefaultAvatarInfo("竞技测试")
	info.Level = progress.AvatarLevel
	accounts := NewFixtureAccounts(map[string]FixtureAccount{
		"sync-test": {Password: "pw", Avatars: []Avatar{{
			OID: []byte("0123456789ab"), UID: 1, Hostnum: 1, Account: "sync-test", Info: info, Progress: progress,
		}}},
	})
	service := New(accounts, nil)
	service.Now = func() time.Time { return now }
	conn := NewConnection()
	conn.phase = Playing
	conn.hostnum = 1
	conn.identity = Identity{Account: "sync-test", Avatars: []Avatar{{
		OID: []byte("0123456789ab"), UID: 1, Hostnum: 1, Account: "sync-test", Info: info, Progress: progress,
	}}}
	return service, conn
}

func TestSyncPVPRPCMatchLoadCancel(t *testing.T) {
	service, conn := newSyncPVPTestService()
	ctx := context.Background()
	pushes, err := service.Handle(ctx, conn, "start_sync_pvp_match", []json.RawMessage{json.RawMessage("7")})
	if err != nil || len(pushes) != 1 {
		t.Fatalf("匹配 RPC 未接入: pushes=%d err=%v", len(pushes), err)
	}
	now := service.Now()
	service.Now = func() time.Time { return now.Add(20 * time.Second) }
	if _, err = service.Tick(ctx, conn); err != nil {
		t.Fatal(err)
	}
	var saved Progress
	for _, av := range conn.identity.Avatars {
		if av.Hostnum == 1 {
			saved = av.Progress
		}
	}
	if saved.SyncPvpMatch == nil || saved.SyncPvpMatch.Robot.ID == 0 {
		t.Fatal("AI 对手未持久化")
	}
	card := saved.Cards[0]
	if _, err := service.Handle(ctx, conn, "send_pvp_cards", []json.RawMessage{json.RawMessage(`{"fighting_cards":["x"]}`)}); err == nil {
		t.Fatal("未拥有的卡牌未被拒绝")
	}
	team, _ := json.Marshal(map[string]any{"fighting_cards": []string{card.UUID}})
	pushes, err = service.Handle(ctx, conn, "send_pvp_cards", []json.RawMessage{team})
	if err != nil || len(pushes) != 8 || pushes[2].Method != "start_server_battle_ok" {
		t.Fatal(err)
	}
	pushes, err = service.Handle(ctx, conn, "pvp_load_complete", nil)
	if err != nil || len(pushes) != 0 {
		t.Fatalf("重复加载不应重建战斗: %#v %v", pushes, err)
	}
	pushes, err = service.Handle(ctx, conn, "cancel_sync_pvp_match", []json.RawMessage{json.RawMessage("8")})
	if err == nil {
		t.Fatalf("已加载战斗不应允许取消匹配: %#v", pushes)
	}
}

func prepareSyncPvpLoadingTest(t *testing.T) (*Service, *Connection, string, []byte) {
	t.Helper()
	service, conn := newSyncPVPTestService()
	ctx := context.Background()
	if _, err := service.Handle(ctx, conn, "start_sync_pvp_match", []json.RawMessage{json.RawMessage("1")}); err != nil {
		t.Fatal(err)
	}
	now := service.Now()
	service.Now = func() time.Time { return now.Add(20 * time.Second) }
	if _, err := service.Tick(ctx, conn); err != nil {
		t.Fatal(err)
	}
	p := conn.SelectedAvatarUnsafe().Progress
	team, _ := json.Marshal(map[string]any{"fighting_cards": []string{p.Cards[0].UUID}})
	if _, err := service.Handle(ctx, conn, "send_pvp_cards", []json.RawMessage{team}); err != nil {
		t.Fatal(err)
	}
	return service, conn, conn.SelectedAvatarUnsafe().Progress.Battle.UUID, team
}

func assertSyncPvpReadyNotifications(t *testing.T, pushes []Push) {
	t.Helper()
	started := 0
	for _, item := range pushes {
		if item.Method == "notify_battle_fighting" {
			t.Fatal("正式战斗不得进入旧布阵就绪UI", pushes)
		}
		if item.Method == "sync_battle_method" && item.Args[0] == "battle_fighting" {
			started++
		}
	}
	if started != 1 {
		t.Fatal("同步PVP实际加载屏障没有下发一次原生开战", pushes)
	}
}

func TestSyncPvpReadyBarrierUnblocksBothArrivalOrdersAndOldSessions(t *testing.T) {
	ctx := context.Background()
	t.Run("桥先就绪", func(t *testing.T) {
		service, conn, uuid, team := prepareSyncPvpLoadingTest(t)
		pushes := observeBattleEvent(t, ctx, service, conn,
			`{"battle_uuid":"`+uuid+`","sequence":1,"kind":"ready","data":{"version":1}}`)
		if len(pushes) != 0 {
			t.Fatal("实体尚未加载时不能提前放行", pushes)
		}
		pushes, err := service.Handle(ctx, conn, "load_entity_finish", nil)
		if err != nil {
			t.Fatal(err)
		}
		assertSyncPvpReadyNotifications(t, pushes)
		pushes, err = service.Handle(ctx, conn, "battle_fighting", []json.RawMessage{team})
		if err != nil || len(pushes) != 0 {
			t.Fatal("客户端收到就绪后仍不能开战", err, pushes)
		}
		battle := conn.SelectedAvatarUnsafe().Progress.Battle
		if !battle.Started {
			t.Fatal("同步PVP没有从准备推进到进行", battle)
		}
	})

	t.Run("实体先加载", func(t *testing.T) {
		service, conn, uuid, _ := prepareSyncPvpLoadingTest(t)
		if pushes, err := service.Handle(ctx, conn, "load_entity_finish", nil); err != nil || len(pushes) != 0 {
			t.Fatal("桥未握手时不能提前放行", err, pushes)
		}
		pushes := observeBattleEvent(t, ctx, service, conn,
			`{"battle_uuid":"`+uuid+`","sequence":1,"kind":"ready","data":{"version":1}}`)
		assertSyncPvpReadyNotifications(t, pushes)
	})

	t.Run("发布前已卡会话", func(t *testing.T) {
		service, conn, _, _ := prepareSyncPvpLoadingTest(t)
		if err := service.updateProgress(ctx, conn, func(p *Progress) error {
			p.Battle.Loaded, p.Battle.BridgeReady = true, true
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		pushes, err := service.Tick(ctx, conn)
		if err != nil {
			t.Fatal(err)
		}
		assertSyncPvpReadyNotifications(t, pushes)
		pushes, err = service.Tick(ctx, conn)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range pushes {
			if item.Method == "notify_battle_fighting" {
				t.Fatal("同一UUID重复下发同步PVP就绪", pushes)
			}
		}
	})
}

func TestSyncPvpBattleFightingUsesServerLockedLayout(t *testing.T) {
	ctx := context.Background()
	service, conn, uuid, _ := prepareSyncPvpLoadingTest(t)
	if _, err := service.Handle(ctx, conn, "load_entity_finish", nil); err != nil {
		t.Fatal(err)
	}
	pushes := observeBattleEvent(t, ctx, service, conn,
		`{"battle_uuid":"`+uuid+`","sequence":1,"kind":"ready","data":{"version":1}}`)
	assertSyncPvpReadyNotifications(t, pushes)

	p := conn.SelectedAvatarUnsafe().Progress
	wantFighting := append([]string{}, p.SyncPvpMatch.OwnFighting...)
	wantSupport := append([]string{}, p.SyncPvpMatch.OwnSupport...)
	// 真实线上失败形状：fighting_cards是null。同步PVP不再依赖
	// 客户端重复序列化的阵容，而使用send_pvp_cards锁定快照。
	malformed := json.RawMessage(`{"fighting_cards":null,"support_cards":{"0":"旧客户端形状"}}`)
	pushes, err := service.Handle(ctx, conn, "battle_fighting", []json.RawMessage{malformed})
	if err != nil || len(pushes) != 0 {
		t.Fatal("同步PVP未兼容客户端旧阵容形状", err, pushes)
	}
	battle := conn.SelectedAvatarUnsafe().Progress.Battle
	if !battle.Started || battle.Layout == nil ||
		!reflect.DeepEqual(battle.Layout.Fighting, wantFighting) ||
		!reflect.DeepEqual(battle.Layout.Support, wantSupport) ||
		!reflect.DeepEqual(battle.Team, append(append([]string{}, wantFighting...), wantSupport...)) {
		t.Fatal("同步PVP没有使用服务器锁定阵容", battle)
	}
	before := cloneBattleLayout(*battle.Layout)
	if repeat, err := service.Handle(ctx, conn, "battle_fighting", []json.RawMessage{json.RawMessage(`null`)}); err != nil || len(repeat) != 0 {
		t.Fatal("重复battle_fighting应幂等", err, repeat)
	}
	if !reflect.DeepEqual(before, *conn.SelectedAvatarUnsafe().Progress.Battle.Layout) {
		t.Fatal("重复battle_fighting重置了锁定阵容")
	}
}

func TestMalformedBattleFightingStillRejectedForOrdinaryPVE(t *testing.T) {
	_, service, conn, _, _ := layoutTestSetup(t)
	if _, err := service.Handle(context.Background(), conn, "battle_fighting", []json.RawMessage{
		json.RawMessage(`{"fighting_cards":null}`),
	}); err == nil {
		t.Fatal("普通PVE不得因同步PVP兼容而放宽畸形阵容")
	}
	if conn.SelectedAvatarUnsafe().Progress.Battle.Started {
		t.Fatal("普通PVE畸形阵容不得开战")
	}
}

func TestParsePVPDungeonExtraRequiresAndroidActivity(t *testing.T) {
	if !parsePVPDungeonExtra(map[string]any{"activity_id": float64(1)}) {
		t.Fatal("activity_id=1 应识别为同步 PVP")
	}
	if parsePVPDungeonExtra(map[string]any{"activity_id": float64(2)}) {
		t.Fatal("其他活动不得误识别为同步 PVP")
	}
}
