package dbstore

import (
	"context"
	"errors"
	"hs-server/internal/game"
	"sync"
	"testing"
)

func TestPostgresProfileNamingAndPersistence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	var ids [][]byte
	infos := []game.ClientInfo{{Account: "取名甲", Password: "pw", Hostnum: 1}, {Account: "取名乙", Password: "pw", Hostnum: 1}}
	for _, info := range infos {
		id, err := s.Register(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id.Avatars[0].OID)
	}
	var wg sync.WaitGroup
	result := make(chan int, 2)
	bad := make(chan error, 2)
	for i, oid := range ids {
		wg.Add(1)
		go func(i int, oid []byte) {
			defer wg.Done()
			_, err := s.SetNicknameGender(ctx, oid, "并发书灵", 2)
			if err == nil {
				result <- i
			} else {
				bad <- err
			}
		}(i, oid)
	}
	wg.Wait()
	close(result)
	close(bad)
	if len(result) != 1 || len(bad) != 1 {
		t.Fatalf("同服并发重名约束失败：成功%d 失败%d", len(result), len(bad))
	}
	for err := range bad {
		if !errors.Is(err, game.ErrNicknameExists) {
			t.Fatal(err)
		}
	}
	winner := <-result
	saved, err := New(s.pool).QuickLogin(ctx, infos[winner])
	if err != nil {
		t.Fatal(err)
	}
	av := saved.Avatars[0]
	if av.Info.Nickname != "并发书灵" || av.Gender != 2 || !av.NicknameSet || av.CreatedAt.IsZero() {
		t.Fatalf("复登资料错误：%+v", av)
	}
	if _, err = s.SetNicknameGender(ctx, ids[winner], "并发书灵", 2); err != nil {
		t.Fatal("同值重试失败", err)
	}
	if _, err = s.SetNicknameGender(ctx, ids[winner], "擅自改名", 1); !errors.Is(err, game.ErrNicknameExists) {
		t.Fatal("免费更名获准")
	}
	if _, err = s.SetNicknameGender(ctx, ids[1-winner], "A", 1); err == nil {
		t.Fatal("非法长度取名获准")
	}
}
