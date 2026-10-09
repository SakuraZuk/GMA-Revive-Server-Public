package dbstore

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestPostgresUIGuideConcurrentColdLogin(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "界面引导持久化", Password: "pw", Hostnum: 1}
	identity, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	connections := make([]*game.Connection, 4)
	services := make([]*game.Service, 4)
	for i := range connections {
		services[i] = pgActivityService(t, New(store.pool), time.Now)
		connections[i] = pgActivityLogin(t, services[i], info)
	}
	var wait sync.WaitGroup
	errors := make(chan error, 4)
	for i := range connections {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			_, err := services[i].Handle(ctx, connections[i], "finished_guide", pgSocialArgs(13))
			errors <- err
		}(i)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	cold, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil || !reflect.DeepEqual(cold.Avatars[0].Progress.FinishedGuides, []int{13}) {
		t.Fatal("并发或冷登录丢失界面引导", err)
	}
	before, after := identity.Avatars[0].Progress, cold.Avatars[0].Progress
	if before.FreeYuanbao != after.FreeYuanbao || !reflect.DeepEqual(before.Cards, after.Cards) || !reflect.DeepEqual(before.GuideTasks, after.GuideTasks) {
		t.Fatal("界面引导越权修改玩法资产")
	}
}
