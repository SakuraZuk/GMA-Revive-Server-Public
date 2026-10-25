package dbstore

import (
	"context"
	"errors"
	"testing"

	"hs-server/internal/game"
)

type rankWithoutRoster struct{ *Store }

func (s *rankWithoutRoster) SyncPvpRankings(context.Context, int, ...int) ([]game.SyncPvpRankEntry, error) {
	return nil, errors.New("本人名次不得读取榜单")
}
func (s *rankWithoutRoster) SocialAvatars(context.Context, game.SocialSearch) ([]game.Avatar, error) {
	return nil, errors.New("本人名次不得加载全服存档")
}

func TestPostgresPersonalRankHandleColdAndNoRoster(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "个人名次", Password: "pw", Hostnum: 1}
	identity, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdateProgress(ctx, identity.Avatars[0].OID, func(p *game.Progress) error { p.SyncPvpScore = 1000; return nil })
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.pool.Exec(ctx, `INSERT INTO accounts(account,password_hash) SELECT '名次夹具'||g,'测试无登录' FROM generate_series(1,1101) g;
INSERT INTO avatars(avatar_oid,account,hostnum,nickname) SELECT decode(lpad(to_hex(g),24,'0'),'hex'),'名次夹具'||g,CASE WHEN g=1101 THEN 1 ELSE 2 END,'名次'||g FROM generate_series(1,1101) g;
INSERT INTO avatar_progress(avatar_oid,state) SELECT avatar_oid,jsonb_build_object('sync_pvp_score',2000) FROM avatars WHERE account LIKE '名次夹具%';`)
	if err != nil {
		t.Fatal(err)
	}
	fresh := New(store.pool)
	svc := pgSocialService(t, fresh)
	cold := pgSocialLogin(t, svc, info, identity.Avatars[0].OID)
	svc.Accounts = &rankWithoutRoster{fresh}
	// 名次是只读快照，不因其他请求持有角色写锁而阻塞。
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, `SELECT avatar_oid FROM avatar_progress FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	for method, expected := range map[string]int{"query_sync_pvp_rank": 2, "query_sync_pvp_world_rank": 1102} {
		out, err := svc.Handle(ctx, cold, method, pgSocialArgs(71))
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 1 || out[0].Method != "call_client_callback" {
			t.Fatal("名次回调结构错误")
		}
		values := out[0].Args[1].([]any)
		if len(values) != 1 || values[0] != expected {
			t.Fatal("本服或世界名次错误", method, values)
		}
	}
}
