package dbstore

import (
	"context"
	"testing"

	"hs-server/internal/game"
)

func TestPostgresGirlRandomModesColdPersistence(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "随机看板", Password: "pw", Hostnum: 1}
	id, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []int{2, 0, 1} {
		svc := pgSocialService(t, New(store.pool))
		c := pgSocialLogin(t, svc, info, id.Avatars[0].OID)
		pgSocialCall(t, svc, c, "set_girl_random_enable", 91, mode)
		svc.Detach(c)
		fresh, err := New(store.pool).QuickLogin(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		p := fresh.Avatars[0].Progress
		if p.GirlRandomType == nil || *p.GirlRandomType != mode || p.GirlRandomEnable == nil || *p.GirlRandomEnable != (mode != 0) {
			t.Fatal("随机模式或开关未跨Store持久化")
		}
		props := fresh.Avatars[0].InitialProperties(info.Account)
		if props["girl_random_type"] != mode || props["girl_random_enable"] != (mode != 0) {
			t.Fatal("冷投影没有同步随机范围")
		}
	}
}
