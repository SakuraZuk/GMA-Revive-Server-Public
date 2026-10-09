package game

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hs-server/internal/hotfix"
)

func TestReconnectRequiresBoundAvatarDeviceAndProof(t *testing.T) {
	s, c := fixture(t), NewConnection()
	c.SetDeviceID("设备甲")
	ctx := context.Background()
	_, err := s.Handle(ctx, c, "quick_login", rawArgs(ClientInfo{Hostnum: 10001, Account: "local", Password: "pw"}))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.BecomePlayer(c)
	av, _ := c.SelectedAvatar()
	if _, err = s.Handle(ctx, c, "set_reconnect_auth_msg", rawArgs("10001_12345")); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		oid           []byte
		device, token string
	}{
		{av.OID, "设备乙", "10001_12345"}, {av.OID, "设备甲", "错误"}, {make([]byte, 12), "设备甲", "10001_12345"},
	} {
		rejected := NewConnection()
		if _, _, err := s.Resume(ctx, rejected, test.oid, test.device, []byte(test.token)); err == nil || rejected.Phase() != Connected {
			t.Fatal("无效重连越过鉴权")
		}
	}
	resumed := NewConnection()
	got, replay, err := s.Resume(ctx, resumed, av.OID, "设备甲", []byte("10001_12345"))
	if err != nil || !bytes.Equal(got.OID, av.OID) || resumed.Phase() != Playing {
		t.Fatalf("重连未恢复角色：%v", err)
	}
	if len(replay) != 0 {
		t.Fatal("无战斗会话时不应重放战斗跳转", replay)
	}
	if _, err := s.Handle(ctx, resumed, "heart_beat", rawArgs(1.5)); err != nil {
		t.Fatal(err)
	}
	now := s.Now()
	s.Now = func() time.Time { return now.Add(24 * time.Hour) }
	if _, _, err := s.Resume(ctx, NewConnection(), av.OID, "设备甲", []byte("10001_12345")); err == nil {
		t.Fatal("过期重连获准")
	}
}

// 普通战斗真实TCP重连必须新建观察世代，不能用断线前的持久序号继续接收丢帧后的上报。
func TestResumeDuringGuideBattleReopensWithNewUUID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hotfix.json")
	if err := os.WriteFile(path, []byte(`{"startup_scripts":{"1.0.125":"pass\n"},"runtime":{"index":2,"script":"pass\n"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var catalog hotfix.Catalog
	if err := catalog.Load(path); err != nil {
		t.Fatal(err)
	}
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, &catalog)
	now := time.Unix(1700000000, 500000000)
	service.Now = func() time.Time { return now }
	ctx := context.Background()
	c, av := newBattleConnection(t, ctx, accounts, service)
	startGuideBattle(t, ctx, service, c, &now)
	if _, err := service.Handle(ctx, c, "set_reconnect_auth_msg", rawArgs("战斗凭据")); err != nil {
		t.Fatal(err)
	}
	resumed := NewConnection()
	previous := c.SelectedAvatarUnsafe().Progress.Battle.UUID
	_, replay, err := service.Resume(ctx, resumed, av.OID, "", []byte("战斗凭据"))
	if err != nil {
		t.Fatal(err)
	}
	if resumed.battleStartSent || resumed.SelectedAvatarUnsafe().Progress.Battle.UUID == previous {
		t.Fatal("真实TCP重连没有新建准备阶段")
	}
	if len(replay) < 4 || replay[1].Method != "start_server_battle_ok" || replay[1].Args[3].(map[string]any)["hs_recover_previous_uuid"] != previous {
		t.Fatalf("真实TCP重连没有明确清场标记和原生prepare：%v", replay)
	}
}
