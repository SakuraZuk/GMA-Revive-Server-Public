package dbstore

import (
	"context"
	"fmt"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestPostgresCollectionRankRealScoresHostIsolationColdReadAndNoLocks(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ids := game.CollectionRankCardIDs()
	avs := []game.Avatar{}
	for i := 0; i < 5; i++ {
		host := 1
		if i == 4 {
			host = 2
		}
		identity, err := s.Register(ctx, game.ClientInfo{Account: fmt.Sprintf("收集实库%d", i), Password: "pw", Hostnum: host})
		if err != nil {
			t.Fatal(err)
		}
		av := identity.Avatars[0]
		avs = append(avs, av)
		_, err = s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
			p.Cards = nil
			p.ObtainedCardIDs = nil
			if i == 0 || i == 1 {
				p.Cards = []game.Card{{CardID: ids[0]}, {CardID: ids[0]}, {CardID: ids[1]}, {CardID: 4}}
				p.ObtainedCardIDs = map[int]int64{ids[0]: 1, ids[2]: 2, 999999: 3}
			}
			if i == 2 {
				p.Cards = []game.Card{{CardID: ids[0]}}
			}
			if i == 4 {
				p.ObtainedCardIDs = map[int]int64{}
				for _, id := range ids {
					p.ObtainedCardIDs[id] = 1
				}
			}
			p.SelectedHeadID = 4401
			p.SelectedHeadBoxID = 1001
			p.CustomHeadCleared = true
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var before int64
	if err := s.pool.QueryRow(ctx, `SELECT SUM(revision) FROM avatar_progress`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	block, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(block)
	if _, err = block.Exec(ctx, `SELECT a.avatar_oid FROM avatars a JOIN avatar_progress p ON p.avatar_oid=a.avatar_oid FOR UPDATE OF a,p`); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	rows, own, err := New(s.pool).ActivityRanking(short, 2, 0, avs[2].OID, 1, 1)
	if err != nil || len(rows) != 1 || own != 3 {
		t.Fatal("只读榜被行锁阻断或全量名次错误", err, len(rows), own)
	}
	if rows[0].Score[0] != 3 || rows[0].Avatar.Info.HeadID != 4401 || rows[0].Avatar.Info.HeadBoxID != 1001 || rows[0].Avatar.Info.CustomHeadImageURL != "" {
		t.Fatal("真实计数或已选头像框错误")
	}
	if err = block.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	rows, own, err = New(s.pool).ActivityRanking(ctx, 2, 0, avs[3].OID, 1, 100)
	if err != nil || len(rows) != 3 || own != 0 {
		t.Fatal("未收集角色、异服过滤或冷查询错误", err, len(rows), own)
	}
	if string(rows[0].Avatar.OID) > string(rows[1].Avatar.OID) || rows[0].Score[0] != rows[1].Score[0] {
		t.Fatal("同分次序不稳定")
	}
	if len(rows[0].Avatar.Progress.Cards) != 0 || len(rows[0].Avatar.Progress.Materials) != 0 {
		t.Fatal("排行读取完整私人背包")
	}
	var after int64
	if err = s.pool.QueryRow(ctx, `SELECT SUM(revision) FROM avatar_progress`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatal("只读榜单改变角色版本")
	}
}
