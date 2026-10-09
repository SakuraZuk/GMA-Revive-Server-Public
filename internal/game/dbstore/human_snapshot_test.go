package dbstore

import (
	"context"
	"hs-server/internal/game"
	"testing"
)

func TestPostgresHumanAvatarSnapshotRevisionAndMetadata(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, err := s.Register(ctx, game.ClientInfo{Account: "在线快照版本校验", Password: "测试密码", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	oid := id.Avatars[0].OID
	av, revision, changed, err := s.HumanAvatarSnapshot(ctx, oid, -1)
	if err != nil || !changed || len(av.Progress.Cards) != 1 {
		t.Fatal("首次快照缺存档", err)
	}
	av, same, changed, err := s.HumanAvatarSnapshot(ctx, oid, revision)
	if err != nil || changed || same != revision || av.Progress.Cards != nil {
		t.Fatal("无变化重复传输存档", err)
	}
	_, err = New(s.pool).UpdateProgress(ctx, oid, func(p *game.Progress) error { p.AvatarExp = 37; return nil })
	if err != nil {
		t.Fatal(err)
	}
	av, next, changed, err := s.HumanAvatarSnapshot(ctx, oid, revision)
	if err != nil || !changed || next <= revision || av.Progress.AvatarExp != 37 {
		t.Fatal("跨存储修改未立即可见", err)
	}
	_, err = s.pool.Exec(ctx, `UPDATE avatars SET nickname='快照改名' WHERE avatar_oid=$1`, oid)
	if err != nil {
		t.Fatal(err)
	}
	av, same, changed, err = s.HumanAvatarSnapshot(ctx, oid, next)
	if err != nil || changed || same != next || av.Info.Nickname != "快照改名" {
		t.Fatal("元数据更新不可见", err)
	}
	if _, _, _, err = s.HumanAvatarSnapshot(ctx, []byte("不存在角色"), -1); err == nil {
		t.Fatal("不存在角色返回成功")
	}
}
