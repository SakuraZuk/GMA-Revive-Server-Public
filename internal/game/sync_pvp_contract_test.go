package game

import (
	"context"
	"encoding/json"
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
	if err != nil || len(pushes) != 7 || pushes[2].Method != "start_server_battle_ok" {
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

func TestParsePVPDungeonExtraRequiresAndroidActivity(t *testing.T) {
	if !parsePVPDungeonExtra(map[string]any{"activity_id": float64(1)}) {
		t.Fatal("activity_id=1 应识别为同步 PVP")
	}
	if parsePVPDungeonExtra(map[string]any{"activity_id": float64(2)}) {
		t.Fatal("其他活动不得误识别为同步 PVP")
	}
}
