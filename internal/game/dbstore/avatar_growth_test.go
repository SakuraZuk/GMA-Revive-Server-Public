package dbstore

import (
	"context"
	"errors"
	"hs-server/internal/game"
	"testing"
)

func TestPostgresAvatarLevelAndProgressAtomicRollback(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	identity, err := store.Register(ctx, game.ClientInfo{Account: "馆主等级事务", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	oid := identity.Avatars[0].OID
	_, err = store.UpdateProgress(ctx, oid, func(p *game.Progress) error { p.AvatarLevel = 5; p.AvatarExp = 17; return nil })
	if err != nil {
		t.Fatal(err)
	}
	av, err := New(store.pool).AdminPlayer(ctx, oid)
	if err != nil || av.Info.Level != 5 || av.Progress.AvatarLevel != 5 || av.Progress.AvatarExp != 17 {
		t.Fatal("双表等级没有一起持久", av, err)
	}
	_, err = store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		p.AvatarLevel = 6
		p.AvatarExp = 99
		return errors.New("注入资产事务失败")
	})
	if err == nil {
		t.Fatal("故障未传递")
	}
	av, err = New(store.pool).AdminPlayer(ctx, oid)
	if err != nil || av.Info.Level != 5 || av.Progress.AvatarLevel != 5 || av.Progress.AvatarExp != 17 {
		t.Fatal("双表等级事务未回滚", av, err)
	}
}
