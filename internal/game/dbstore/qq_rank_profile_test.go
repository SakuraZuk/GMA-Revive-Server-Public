package dbstore

import (
	"context"
	"hs-server/internal/game"
	"testing"
)

func TestPostgresQQRankKeepsSelectedAppearance(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	identity, err := s.Register(ctx, game.ClientInfo{Account: "排行榜头像回归", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	oid := identity.Avatars[0].OID
	_, err = s.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		p.SyncPvpScore = 2001
		p.SelectedHeadID = 4401
		p.SelectedHeadBoxID = 1001
		p.CustomHeadCleared = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := New(s.pool).SyncPvpRankings(ctx, 100, 1)
	if err != nil || len(rows) != 1 {
		t.Fatal("排行榜读取失败", err, len(rows))
	}
	if rows[0].HeadID != 4401 || rows[0].HeadBoxID != 1001 || rows[0].CustomHead != "" {
		t.Fatal("冷查询丢失已选外观", rows[0])
	}
	_, err = s.UpdateProgress(ctx, oid, func(p *game.Progress) error { p.SelectedHeadID = 0; p.SelectedHeadBoxID = 0; return nil })
	if err != nil {
		t.Fatal(err)
	}
	rows, err = New(s.pool).SyncPvpRankings(ctx, 100, 1)
	if err != nil || len(rows) != 1 || rows[0].HeadID != identity.Avatars[0].Info.HeadID || rows[0].HeadBoxID != identity.Avatars[0].Info.HeadBoxID {
		t.Fatal("原始头像框回退错误", rows, err)
	}
}
