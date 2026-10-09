package dbstore

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestPostgresStorylineConcurrentColdLogin(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "剧情记录实库验收", Password: "pw", Hostnum: 1}
	identity, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	// 每次独立服务连接，真实角色行锁必须去重，不能依赖单连接互斥。
	connections := make([]*game.Connection, 4)
	services := make([]*game.Service, 4)
	for i := range connections {
		services[i] = pgActivityService(t, New(store.pool), time.Now)
		connections[i] = pgActivityLogin(t, services[i], info)
	}
	var wait sync.WaitGroup
	failures := make(chan error, 4)
	for i := range connections {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			_, err := services[i].Handle(ctx, connections[i], "finished_storyline", pgSocialArgs(72, "cthulhu_1_0_over"))
			failures <- err
		}(i)
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	cold, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil || !reflect.DeepEqual(cold.Avatars[0].Progress.PlayedStorylines, []string{"cthulhu_1_0_over"}) {
		t.Fatal("真实行锁并发或冷登录记录丢失", err)
	}
	before := identity.Avatars[0].Progress
	after := cold.Avatars[0].Progress
	if before.FreeYuanbao != after.FreeYuanbao || !reflect.DeepEqual(before.ClearedDungeons, after.ClearedDungeons) || !reflect.DeepEqual(before.UnlockSystems, after.UnlockSystems) {
		t.Fatal("剧情记录不能授权奖励、通关或解锁")
	}
	service := pgActivityService(t, New(store.pool), time.Now)
	connection := game.NewConnection()
	connection.SetDeviceID("剧情实库冷登录设备")
	pgRemainingRPC(t, service, connection, "quick_login", info)
	if _, err := service.BecomePlayer(connection); err != nil {
		t.Fatal(err)
	}
	out := pgRemainingRPC(t, service, connection, "set_reconnect_auth_msg", "剧情冷登录")
	if len(out) < 2 || out[0].Method != "client_prop_set" || out[len(out)-1].Method != "on_refresh_login" {
		t.Fatal("冷登录未按剧情桶、刷新顺序恢复", out)
	}
}
