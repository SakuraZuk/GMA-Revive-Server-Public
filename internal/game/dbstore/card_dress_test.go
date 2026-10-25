package dbstore

import (
	"context"
	"hs-server/internal/game"
	"testing"
	"time"
)

func TestPostgresCardDressColdPersistence(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 10, 16, 0, 0, 0, time.UTC)
	info := game.ClientInfo{Account: "换装冷登录实库", Password: "pw", Hostnum: 1}
	s, c, av := repairPlayer(t, store, info, &now)
	defer s.Detach(c)
	uuid := "00112233445566778899aabb"
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Cards = []game.Card{{UUID: uuid, CardID: 4401, Grade: 1, Dress: 440101}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	check := func(dress, want int) {
		t.Helper()
		out := pgRemainingRPC(t, s, c, "change_card_dress", 908, uuid, dress)
		cb := pgRemainingPush(out, "call_client_callback")
		if cb == nil || cb.Args[1].([]any)[0] != want {
			t.Fatal("换装回调不符", out)
		}
	}
	check(440102, 0)
	check(44010101, 5022)
	check(340701, 5022)
	check(440102, 5021)
	cold, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil || cold.Avatars[0].Progress.Cards[0].Dress != 440102 {
		t.Fatal("拒绝或重登丢失换装", err)
	}
}
