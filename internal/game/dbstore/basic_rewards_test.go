package dbstore

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestPostgresBasicRewardsConcurrentClaimColdLogin(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "基础领奖实库验收", Password: "pw", Hostnum: 1}
	identity, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(time.FixedZone("北京时间", 8*3600))
	if now.Hour() < 19 {
		now = time.Date(now.Year(), now.Month(), now.Day(), 21, 0, 0, 0, now.Location())
	}
	services := make([]*game.Service, 4)
	connections := make([]*game.Connection, 4)
	for i := range services {
		services[i] = pgActivityService(t, New(store.pool), func() time.Time { return now })
		connections[i] = pgActivityLogin(t, services[i], info)
	}
	initial, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	start := initial.Avatars[0].Progress.Power.Value
	claimParallel := func(method string, id int) int {
		t.Helper()
		var wait sync.WaitGroup
		outcomes := make(chan bool, 4)
		failures := make(chan error, 4)
		for i := range services {
			wait.Add(1)
			go func(i int) {
				defer wait.Done()
				out, err := services[i].Handle(ctx, connections[i], method, pgSocialArgs(121, id))
				if err != nil {
					failures <- err
					return
				}
				for _, push := range out {
					if push.Method == "call_client_callback" {
						result := push.Args[1].([]any)
						success := false
						if method == "receive_power_supply" {
							success = result[0] != nil
						} else {
							raw, _ := json.Marshal(result[0])
							success = string(raw) == "0"
						}
						outcomes <- success
						return
					}
				}
				failures <- context.Canceled
			}(i)
		}
		wait.Wait()
		close(outcomes)
		close(failures)
		for err := range failures {
			t.Fatal("真实领取缺回调或失败", err)
		}
		count := 0
		for success := range outcomes {
			if success {
				count++
			}
		}
		return count
	}
	if count := claimParallel("receive_power_supply", 8); count != 1 {
		t.Fatal("四独立连接补给没有恰好一次成功", count)
	}
	cold, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil || cold.Avatars[0].Progress.Power.Value <= start || cold.Avatars[0].Progress.BasicRewards.BuffCounts[8] != 1 {
		t.Fatal("补给未真实到账或无持久收据", err)
	}
	power := cold.Avatars[0].Progress.Power.Value
	if _, err := store.UpdateProgress(ctx, identity.Avatars[0].OID, func(p *game.Progress) error {
		// 本项SQL夹具只准备原表已完成任务，不冒充Android赚取进度。
		p.NewTasks[101] = game.NewTaskProgress{TaskID: 101, Status: 1, FinishedTargets: map[int]int{20102: 1}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	if count := claimParallel("receive_new_task_bonus", 101); count != 1 {
		t.Fatal("四独立连接新手奖没有恰好一次成功", count)
	}
	cold, err = New(store.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := cold.Avatars[0].Progress
	if p.Materials[12].Count != before.Avatars[0].Progress.Materials[12].Count+20000 || p.Materials[52].Count != before.Avatars[0].Progress.Materials[52].Count+1 || p.NewTasks[101].Status != 2 || p.BasicRewards.NewClaims[101] == 0 || p.Power.Value != power {
		t.Fatal("新手奖励部分到账、重复到账或收据错误")
	}
	if _, leaked := (game.Avatar{Progress: p}).InitialProperties("基础验收")["server_basic_rewards"]; leaked {
		t.Fatal("私有收据泄露给客户端")
	}
	// 第五个全新存储/服务连接读取已有收据；不能重登后再发。
	fresh := pgActivityService(t, New(store.pool), func() time.Time { return now })
	connection := pgActivityLogin(t, fresh, info)
	for _, entry := range []struct {
		method string
		id     int
	}{{"receive_power_supply", 8}, {"receive_new_task_bonus", 101}} {
		out, err := fresh.Handle(ctx, connection, entry.method, pgSocialArgs(122, entry.id))
		if err != nil {
			t.Fatal(err)
		}
		for _, push := range out {
			if push.Method == "call_client_callback" {
				args := push.Args[1].([]any)
				if entry.method == "receive_power_supply" && args[0] != nil {
					t.Fatal("冷登录补给重复发奖")
				}
				if entry.method == "receive_new_task_bonus" {
					raw, _ := json.Marshal(args[0])
					if string(raw) == "0" {
						t.Fatal("冷登录新手奖重复发奖")
					}
				}
			}
		}
	}
}
