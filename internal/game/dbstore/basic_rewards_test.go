package dbstore

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestPostgresSevenDayHeadFrameClaimColdPersistence(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "七日头像框冷登录验收", Password: "pw", Hostnum: 1}
	if _, err := store.Register(ctx, info); err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(time.FixedZone("北京时间", 8*3600))
	var service *game.Service
	var connection *game.Connection
	for day := 1; day <= 7; day++ {
		service = pgActivityService(t, New(store.pool), func() time.Time { return now })
		connection = pgActivityLogin(t, service, info)
		if day < 7 {
			now = now.AddDate(0, 0, 1)
		}
	}
	out, err := service.Handle(ctx, connection, "receive_check_in_bonus", pgSocialArgs(711, 1, 7))
	if err != nil {
		t.Fatal(err)
	}
	frameBeforeCallback, callbacks := false, 0
	for _, item := range out {
		if item.Method == "client_prop_changed" && len(item.Args) == 1 {
			if pair, ok := item.Args[0].([]any); ok && len(pair) == 2 && pair[0] == "owned_head_box" {
				var frames map[int]float64
				raw, _ := json.Marshal(pair[1])
				if json.Unmarshal(raw, &frames) == nil {
					_, frameBeforeCallback = frames[1004]
				}
			}
		}
		if item.Method == "call_client_callback" {
			callbacks++
			raw, _ := json.Marshal(item.Args[1].([]any)[0])
			if string(raw) != "0" || !frameBeforeCallback {
				t.Fatal("真实数据库七日领奖缺成功回调前的头像框同步")
			}
		}
	}
	if callbacks != 1 {
		t.Fatal("七日领奖回调次数错误", callbacks)
	}
	fresh := pgActivityService(t, New(store.pool), func() time.Time { return now })
	cold := pgActivityLogin(t, fresh, info)
	identity, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := identity.Avatars[0].Progress
	if acquired, owned := p.OwnedHeadBox[1004]; !owned || acquired != 0 {
		t.Fatal("七日永久头像框未跨连接持久化")
	}
	var receipt struct {
		Count    int   `json:"check_in_count"`
		Received []int `json:"received_bonus"`
	}
	raw, _ := json.Marshal(p.CheckInRecords[1])
	if json.Unmarshal(raw, &receipt) != nil || receipt.Count != 7 || len(receipt.Received) != 1 || receipt.Received[0] != 7 {
		t.Fatal("真实七次登录及第七日收据不符")
	}
	out, err = fresh.Handle(ctx, cold, "receive_check_in_bonus", pgSocialArgs(712, 1, 7))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range out {
		if item.Method == "call_client_callback" {
			raw, _ := json.Marshal(item.Args[1].([]any)[0])
			if string(raw) == "0" {
				t.Fatal("重登后七日奖励重复领取成功")
			}
			return
		}
	}
	t.Fatal("重复领取缺少明确拒绝回调")
}

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
		p.BasicRewards.DailyActive = 100
		p.BasicRewards.WeeklyActive = 500
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
	// 原生红点按活跃度阈值查询；四连接竞争后冷登录仍只领取一次。
	for _, kind := range []string{"daily", "weekly"} {
		for id := 1; id <= 5; id++ {
			if count := claimParallel("receive_"+kind+"_active_bonus", id); count != 1 {
				t.Fatal("活跃度奖励并发领取次数错误", kind, id, count)
			}
		}
	}
	cold, err = New(store.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	props := cold.Avatars[0].InitialProperties("基础验收")
	for field, expected := range map[string]string{"daily_bonus_active": "[20,40,60,80,100]", "weekly_bonus_active": "[100,200,300,400,500]"} {
		raw, err := json.Marshal(props[field])
		if err != nil || string(raw) != expected {
			t.Fatal("冷登录领取阈值与原生判定不符", field, string(raw), err)
		}
	}
	// 第五个全新存储/服务连接读取已有收据；不能重登后再发。
	fresh := pgActivityService(t, New(store.pool), func() time.Time { return now })
	connection := pgActivityLogin(t, fresh, info)
	for _, entry := range []struct {
		method string
		id     int
	}{{"receive_power_supply", 8}, {"receive_new_task_bonus", 101}, {"receive_daily_active_bonus", 1}, {"receive_weekly_active_bonus", 1}} {
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
				if entry.method != "receive_power_supply" {
					raw, _ := json.Marshal(args[0])
					if string(raw) == "0" {
						t.Fatal("冷登录新手奖重复发奖")
					}
				}
			}
		}
	}
}

func TestPostgresBasicRewardRetriesShortAvatarRowContention(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "基础领奖短锁重试", Password: "pw", Hostnum: 1}
	identity, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(time.FixedZone("北京时间", 8*3600))
	now = time.Date(now.Year(), now.Month(), now.Day(), 21, 0, 0, 0, now.Location())
	service := pgActivityService(t, New(store.pool), func() time.Time { return now })
	connection := pgActivityLogin(t, service, info)
	beforeLogin, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	before := beforeLogin.Avatars[0].Progress.Power.Value
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT level FROM avatars WHERE avatar_oid=$1 FOR UPDATE`, identity.Avatars[0].OID); err != nil {
		t.Fatal(err)
	}
	released := make(chan error, 1)
	go func() {
		time.Sleep(1150 * time.Millisecond)
		released <- tx.Commit(ctx)
	}()
	out, err := service.Handle(ctx, connection, "receive_power_supply", pgSocialArgs(131, 8))
	if err != nil {
		t.Fatal("短暂玩家行锁冲突没有在原请求内恢复", err)
	}
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	succeeded := false
	for _, item := range out {
		if item.Method == "call_client_callback" {
			args := item.Args[1].([]any)
			succeeded = len(args) == 2 && args[0] != nil && args[1] == ""
		}
	}
	cold, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil || !succeeded || cold.Avatars[0].Progress.Power.Value != before+50 || cold.Avatars[0].Progress.BasicRewards.BuffCounts[8] != 1 {
		t.Fatal("短锁恢复后缺原生成功回调、到账或唯一收据", err, succeeded)
	}
}
