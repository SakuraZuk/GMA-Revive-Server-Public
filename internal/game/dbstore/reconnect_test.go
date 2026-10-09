package dbstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestPostgresReconnectSurvivesStoreRecreation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, err := s.Register(ctx, game.ClientInfo{Account: "重连测试", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	av := id.Avatars[0]
	hash := sha256.Sum256([]byte("10001_12345"))
	now := time.Now()
	if err = s.SaveReconnect(ctx, av.OID, "设备甲", hash[:], now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := New(s.pool).Resume(ctx, av.OID, "设备甲", hash[:], now)
	if err != nil || !bytes.Equal(got.Avatars[0].OID, av.OID) {
		t.Fatalf("持久重连失败：%v", err)
	}
	if _, err = s.Resume(ctx, av.OID, "设备乙", hash[:], now); err == nil {
		t.Fatal("设备绑定失效")
	}
	if _, err = s.Resume(ctx, av.OID, "设备甲", hash[:], now.Add(time.Hour)); err == nil {
		t.Fatal("过期凭证获准")
	}
	var stored []byte
	if err = s.pool.QueryRow(ctx, `SELECT token_hash FROM avatar_reconnect WHERE avatar_oid=$1`, av.OID).Scan(&stored); err != nil || len(stored) != 32 {
		t.Fatal("凭证未存摘要")
	}
}
