package game

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func basicFixture(t *testing.T) (*Service, *Connection, *FixtureAccounts, *time.Time) {
	t.Helper()
	ctx := context.Background()
	store := NewFixtureAccounts(nil)
	s := New(store, nil)
	now := time.Date(2026, 10, 8, 21, 0, 0, 0, time.FixedZone("北京时间", 8*3600))
	s.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, store, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.GuideTasks[1000] = GuideTask{1000, 2, now.Unix(), 1}
		return ensureBasicRewards(p, now, true)
	}); err != nil {
		t.Fatal(err)
	}
	return s, c, store, &now
}

func basicReply(t *testing.T, s *Service, c *Connection, method string, args ...any) []any {
	t.Helper()
	pushes, err := s.Handle(context.Background(), c, method, rawArgs(args...))
	if err != nil {
		t.Fatal(method, err)
	}
	for _, item := range pushes {
		if item.Method == "call_client_callback" {
			if len(item.Args) != 2 || item.Args[0] != args[0] {
				t.Fatal("回调编号和展开层级错误", item)
			}
			return item.Args[1].([]any)
		}
	}
	t.Fatal("缺少原生回调", method)
	return nil
}

func TestBasicPowerSupplyActualHandleColdPersistenceAndRepeat(t *testing.T) {
	s, c, store, now := basicFixture(t)
	ctx := context.Background()
	start := c.SelectedAvatarUnsafe().Progress.Power.Value
	reply := basicReply(t, s, c, "receive_power_supply", 81, 8)
	if len(reply) != 2 || reply[0] == nil || reply[1] != "" {
		t.Fatal("补给需要box,msg回调，不是ret,box", reply)
	}
	p := c.SelectedAvatarUnsafe().Progress
	// 奖励数量来自本版200711；实际体力允许超上限，不使用弹窗作为到账证据。
	bonus := androidShop.Bonuses[200711]
	var expected int64
	for _, row := range bonus.Fixed {
		if len(row) == 2 && row[0] == 1 {
			expected += row[1]
		}
	}
	if expected <= 0 || p.Power.Value != start+int(expected) || p.BasicRewards.BuffCounts[8] != 1 {
		t.Fatal("原表补给与到账收据不一致", p.Power, expected)
	}
	before := CloneProgress(p)
	reply = basicReply(t, s, c, "receive_power_supply", 82, 8)
	if reply[0] != nil || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("重复补给发奖或修改状态", reply)
	}
	oid := c.SelectedAvatarUnsafe().OID
	cold, err := store.UpdateProgress(ctx, oid, func(p *Progress) error { return nil })
	if err != nil || cold.Power.Value != before.Power.Value || cold.BasicRewards.BuffCounts[8] != 1 {
		t.Fatal("补给未持久化", err)
	}
	*now = now.AddDate(0, 0, 1)
	reply = basicReply(t, s, c, "receive_power_supply", 83, 8)
	if reply[0] == nil || c.SelectedAvatarUnsafe().Progress.BasicRewards.BuffCounts[8] != 1 {
		t.Fatal("跨日没有重新开放补给")
	}
	for _, id := range []int{1, 101, 10001} {
		if basicReply(t, s, c, "receive_power_supply", 84, id)[0] != nil {
			t.Fatal("倍率buff被当作奖励凭证", id)
		}
	}
}

func TestBasicPowerSupplyAllowsValueAboveNaturalMaximum(t *testing.T) {
	s, c, store, _ := basicFixture(t)
	ctx := context.Background()
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.Power.Value = p.Power.Max + 123
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := c.SelectedAvatarUnsafe().Progress.Power.Value
	reply := basicReply(t, s, c, "receive_power_supply", 88, 8)
	if len(reply) != 2 || reply[0] == nil || reply[1] != "" {
		t.Fatal("体力超过自然上限后补给缺少原生成功回调", reply)
	}
	after := c.SelectedAvatarUnsafe().Progress
	if after.Power.Value != before+50 || after.BasicRewards.BuffCounts[8] != 1 {
		t.Fatal("体力超过自然上限后每日50体力未到账", before, after.Power)
	}
	cold, err := store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if err != nil || cold.Power.Value != after.Power.Value || cold.BasicRewards.BuffCounts[8] != 1 {
		t.Fatal("超自然上限补给未冷持久化", err)
	}
}

func TestBasicPowerSupplyTimeAndRewardFailureRollback(t *testing.T) {
	s, c, _, now := basicFixture(t)
	*now = time.Date(2026, 10, 9, 6, 59, 59, 0, now.Location())
	if err := s.updateProgress(context.Background(), c, func(p *Progress) error { return ensureBasicRewards(p, *now, true) }); err != nil {
		t.Fatal(err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if basicReply(t, s, c, "receive_power_supply", 85, 8)[0] != nil || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("7点前发补给")
	}
	*now = now.Add(time.Second)
	if basicReply(t, s, c, "receive_power_supply", 86, 8)[0] == nil {
		t.Fatal("7点边界未开放")
	}
	if err := s.updateProgress(context.Background(), c, func(p *Progress) error { p.Power.Value = 2147483647; return nil }); err != nil {
		t.Fatal(err)
	}
	before = CloneProgress(c.SelectedAvatarUnsafe().Progress)
	*now = time.Date(2026, 10, 9, 12, 0, 0, 0, now.Location())
	if basicReply(t, s, c, "receive_power_supply", 87, 9)[0] != nil || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("奖励失败未整体回滚")
	}
}

func TestBasicNewTaskNativeStatusClaimMigrationAndExpiry(t *testing.T) {
	s, c, _, now := basicFixture(t)
	ctx := context.Background()
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		advanceBasicRewardEvent(p, 4, []any{9}, 1, *now)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.NewTasks[101].Status != 1 || p.NewTasks[101].FinishedTargets[20102] != 1 || p.NewTasks[201].Status != 0 {
		t.Fatal("完成误作已领奖或提前开放次日任务")
	}
	if reply := basicReply(t, s, c, "receive_new_task_bonus", 91, 101); reply[0] != RetSuccess {
		t.Fatal("完成任务不可领取", reply)
	}
	p = c.SelectedAvatarUnsafe().Progress
	if p.NewTasks[101].Status != 2 || p.BasicRewards.NewClaims[101] == 0 {
		t.Fatal("领奖没有持久收据")
	}
	before := CloneProgress(p)
	if basicReply(t, s, c, "receive_new_task_bonus", 92, 101)[0] == RetSuccess || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("新手领奖重复发奖")
	}
	legacy := NewProgress(1, *now)
	legacy.NewTasks[101] = NewTaskProgress{TaskID: 101, Status: 2, FinishedTargets: map[int]int{20102: 1}}
	if err := ensureBasicRewards(&legacy, *now, false); err != nil || legacy.NewTasks[101].Status != 1 {
		t.Fatal("旧版错误完成态未恢复可领", err)
	}
	if err := ensureBasicRewards(&legacy, now.AddDate(0, 0, 14), false); err != nil || !legacy.NewTasks[101].Expired {
		t.Fatal("14天期限未保留", err)
	}
}

func TestBasicDailyActiveActualConsumptionThresholdClaimAndWeek(t *testing.T) {
	s, c, _, now := basicFixture(t)
	ctx := context.Background()
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if !p.consumePower(99, *now) {
			return errorsNewBasicTest()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if reply := basicReply(t, s, c, "receive_daily_task_active", 101, 1); len(reply) != 3 || reply[0] != false {
		t.Fatal("99点消费提前完成100点日常", reply)
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if !p.consumePower(1, *now) {
			return errorsNewBasicTest()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if reply := basicReply(t, s, c, "receive_daily_task_active", 102, 1); len(reply) != 3 || reply[0] != true || reply[2] != 20 {
		t.Fatal("日常原生bool,msg,count回包错误", reply)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.BasicRewards.DailyActive != 20 || p.BasicRewards.WeeklyActive != 20 {
		t.Fatal("活跃度未到账")
	}
	if basicReply(t, s, c, "receive_daily_active_bonus", 103, 1)[0] != RetSuccess {
		t.Fatal("20活跃原奖励箱不可领取")
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if basicReply(t, s, c, "receive_daily_active_bonus", 104, 1)[0] == RetSuccess || basicReply(t, s, c, "receive_daily_task_active", 105, 1)[0] != false || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("重复活跃任务或箱子发奖")
	}
	if basicReply(t, s, c, "receive_weekly_active_bonus", 106, 1)[0] == RetSuccess {
		t.Fatal("不足100周活跃发奖")
	}
	*now = now.AddDate(0, 0, 1)
	if err := s.updateProgress(ctx, c, func(p *Progress) error { return ensureBasicRewards(p, *now, true) }); err != nil {
		t.Fatal(err)
	}
	p = c.SelectedAvatarUnsafe().Progress
	if p.BasicRewards.DailyActive != 0 || len(p.BasicRewards.DailyClaims) != 0 || p.BasicRewards.WeeklyActive != 20 {
		t.Fatal("跨日未清日状态或误清周状态")
	}
	*now = time.Date(2026, 10, 12, 0, 0, 0, 0, now.Location())
	if err := s.updateProgress(ctx, c, func(p *Progress) error { return ensureBasicRewards(p, *now, true) }); err != nil {
		t.Fatal(err)
	}
	if c.SelectedAvatarUnsafe().Progress.BasicRewards.WeeklyActive != 0 {
		t.Fatal("周一未刷新周活跃")
	}
}

func TestBasicDailySixtyActiveRewardActualRPCCallbackAndColdProjection(t *testing.T) {
	s, c, store, _ := basicFixture(t)
	ctx := context.Background()
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.BasicRewards.DailyActive = 60
		p.BasicRewards.WeeklyActive = 60
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reply := basicReply(t, s, c, "receive_daily_active_bonus", 109, 3)
	if len(reply) != 2 || reply[0] != RetSuccess {
		t.Fatal("60活跃第三档缺少原生ret、box成功回调", reply)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if !containsInt(p.BasicRewards.DailyClaims, 3) {
		t.Fatal("60活跃第三档没有写入领取收据")
	}
	props := (Avatar{Progress: p}).InitialProperties("60活跃冷投影")
	raw, _ := json.Marshal(props["daily_bonus_active"])
	if string(raw) != "[60]" {
		t.Fatal("客户端领取态没有按阈值60投影", string(raw))
	}
	cold, err := store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if err != nil || !containsInt(cold.BasicRewards.DailyClaims, 3) {
		t.Fatal("60活跃领取收据未冷持久化", err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if basicReply(t, s, c, "receive_daily_active_bonus", 110, 3)[0] == RetSuccess || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("60活跃奖励被重复发放")
	}
}

func errorsNewBasicTest() error { return errors.New("测试体力不足") }

func TestBasicCheckInActualLoginClaimsNoOfflineDaysAndPrivateProjection(t *testing.T) {
	s, c, _, now := basicFixture(t)
	if basicReply(t, s, c, "receive_check_in_bonus", 111, 1, 1)[0] != RetSuccess {
		t.Fatal("首日签到奖励不可领取")
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if basicReply(t, s, c, "receive_check_in_bonus", 112, 1, 1)[0] == RetSuccess || basicReply(t, s, c, "receive_check_in_bonus", 113, 1, 2)[0] == RetSuccess || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("重复或未来签到发奖")
	}
	*now = now.AddDate(0, 0, 3)
	if err := s.updateProgress(context.Background(), c, func(p *Progress) error { return ensureBasicRewards(p, *now, true) }); err != nil {
		t.Fatal(err)
	}
	var record basicCheckIn
	raw, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress.CheckInRecords[1])
	_ = json.Unmarshal(raw, &record)
	if record.Count != 2 {
		t.Fatal("离线三天伪造了三天签到", record)
	}
	if basicReply(t, s, c, "receive_all_check_in_bonus", 114, 1)[0] != RetSuccess {
		t.Fatal("一键签到补领奖失败")
	}
	p := c.SelectedAvatarUnsafe().Progress
	props := (Avatar{Progress: p}).InitialProperties("基础奖励验收")
	if _, exists := props["server_basic_rewards"]; exists || props["daily_tasks"] == nil || props["check_in_records"] == nil {
		t.Fatal("私有账本泄露或原生投影缺失")
	}
	copy := CloneProgress(p)
	copy.BasicRewards.BuffCounts[8] = 99
	if p.BasicRewards.BuffCounts[8] == 99 {
		t.Fatal("基础奖励克隆未隔离")
	}
}

func TestSevenDayCheckInPublishesHeadFrameBeforeRewardCallback(t *testing.T) {
	for _, method := range []string{"receive_check_in_bonus", "receive_all_check_in_bonus"} {
		t.Run(method, func(t *testing.T) {
			s, c, _, now := basicFixture(t)
			for day := 2; day <= 7; day++ {
				*now = now.AddDate(0, 0, 1)
				if err := s.updateProgress(context.Background(), c, func(p *Progress) error { return ensureBasicRewards(p, *now, true) }); err != nil {
					t.Fatal(err)
				}
			}
			args := []any{711, 1}
			if method == "receive_check_in_bonus" {
				args = append(args, 7)
			}
			out, err := s.Handle(context.Background(), c, method, rawArgs(args...))
			if err != nil {
				t.Fatal(err)
			}
			if _, owned := c.SelectedAvatarUnsafe().Progress.OwnedHeadBox[1004]; !owned {
				t.Fatal("七日原表头像框没有入账")
			}
			seen, callbacks := false, 0
			for _, item := range out {
				if item.Method == "client_prop_changed" && len(item.Args) == 1 {
					if pair, ok := item.Args[0].([]any); ok && len(pair) == 2 && pair[0] == "owned_head_box" {
						raw, _ := json.Marshal(pair[1])
						var frames map[int]float64
						if json.Unmarshal(raw, &frames) == nil {
							_, seen = frames[1004]
						}
					}
				}
				if item.Method == "call_client_callback" {
					callbacks++
					if !seen || item.Args[1].([]any)[0] != RetSuccess {
						t.Fatal("七日领奖回调前未同步新增头像框")
					}
				}
			}
			if callbacks != 1 {
				t.Fatal("领奖回调次数错误", callbacks)
			}
			if basicReply(t, s, c, method, args...)[0] == RetSuccess {
				t.Fatal("七日奖励重复领取成功")
			}
		})
	}
}

func TestBasicScoreBoxOriginalThresholdNoConsumeAndRepeat(t *testing.T) {
	s, c, _, _ := basicFixture(t)
	if basicReply(t, s, c, "receive_score_bonus", 131, 1, 20)[0] != nil {
		t.Fatal("积分不足发奖")
	}
	if err := s.updateProgress(context.Background(), c, func(p *Progress) error { p.Materials[52] = Material{52, 20, 20}; return nil }); err != nil {
		t.Fatal(err)
	}
	if basicReply(t, s, c, "receive_score_bonus", 132, 1, 20)[0] == nil {
		t.Fatal("20群星原表积分箱不可领取")
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Materials[52].Count != 20 || !containsInt(p.BasicRewards.ScoreClaims[1], 20) {
		t.Fatal("累计积分被当作消耗货币或无领取收据")
	}
	before := CloneProgress(p)
	if basicReply(t, s, c, "receive_score_bonus", 133, 1, 20)[0] != nil || basicReply(t, s, c, "receive_score_bonus", 134, 1, 21)[0] != nil || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("积分箱重复或伪造档位发奖")
	}
}

type basicFailStore struct{ *FixtureAccounts }

func (a basicFailStore) UpdateProgress(context.Context, []byte, func(*Progress) error) (Progress, error) {
	return Progress{}, errors.New("测试数据库提交失败")
}

type basicSQLStateFailure struct{ code string }

func (e basicSQLStateFailure) Error() string    { return "测试玩家行锁冲突" }
func (e basicSQLStateFailure) SQLState() string { return e.code }

type basicRetryStore struct {
	*FixtureAccounts
	failures int
	calls    int
}

func (a *basicRetryStore) UpdateProgress(ctx context.Context, oid []byte, fn func(*Progress) error) (Progress, error) {
	a.calls++
	if a.calls <= a.failures {
		return Progress{}, basicSQLStateFailure{code: "55P03"}
	}
	return a.FixtureAccounts.UpdateProgress(ctx, oid, fn)
}

func TestBasicRewardRetriesOnlyTransientRowContention(t *testing.T) {
	s, c, store, _ := basicFixture(t)
	retry := &basicRetryStore{FixtureAccounts: store, failures: 2}
	s.Accounts = retry
	reply := basicReply(t, s, c, "receive_power_supply", 140, 8)
	if retry.calls != 3 || reply[0] == nil || c.SelectedAvatarUnsafe().Progress.BasicRewards.BuffCounts[8] != 1 {
		t.Fatal("短暂玩家行锁冲突未重试成功或重复发奖", retry.calls, reply)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	retry.calls, retry.failures = 0, 3
	out, err := s.Handle(context.Background(), c, "receive_power_supply", rawArgs(141, 9))
	if err == nil || len(out) != 0 || retry.calls != 3 || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("超过重试上限的存储冲突被伪装成功或修改资产", retry.calls, out, err)
	}
}

func TestBasicStorageFailureDoesNotSendSuccessOrChangeAssets(t *testing.T) {
	s, c, store, _ := basicFixture(t)
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	s.Accounts = basicFailStore{store}
	out, err := s.Handle(context.Background(), c, "receive_power_supply", rawArgs(141, 8))
	if err == nil || len(out) != 0 || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("数据库失败被伪装成功或修改连接资产", out, err)
	}
}

func TestBasicTargetGroupsClampAndClosedDayProgress(t *testing.T) {
	row := activityRow{"target_id": json.RawMessage(`[[1,2],[3]]`)}
	if basicTargetRoom(map[int]int64{1: 1, 2: 1}, row, 1, 2) != 0 || basicTargetRoom(map[int]int64{3: 1}, row, 3, 2) != 1 {
		t.Fatal("或目标组封顶错误")
	}
	s, c, _, now := basicFixture(t)
	if err := s.updateProgress(context.Background(), c, func(p *Progress) error { advanceBasicRewardEvent(p, 4, []any{9}, 100, *now); return nil }); err != nil {
		t.Fatal(err)
	}
	for id, task := range c.SelectedAvatarUnsafe().Progress.NewTasks {
		row := basicRewardRow("new_task", id)
		targets := map[int]int64{}
		for tid, count := range task.FinishedTargets {
			targets[tid] = int64(count)
		}
		if basicTargetCount(targets, row) > int64(row.integer("target_need_count")) {
			t.Fatal("累计任务超过原生等值完成阈值", id)
		}
	}
	if c.SelectedAvatarUnsafe().Progress.NewTasks[201].Status != 0 {
		t.Fatal("提前开放第二日领奖")
	}
}

func TestBasicCrossDayRejectedClaimKeepsTransactionAndHeartbeatRefreshes(t *testing.T) {
	s, c, _, now := basicFixture(t)
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	*now = now.AddDate(0, 0, 1)
	if basicReply(t, s, c, "receive_power_supply", 151, 101)[0] != nil || !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("跨日失败操作先提交了日界")
	}
	out, err := s.Handle(context.Background(), c, "heart_beat", rawArgs(float64(now.Unix())))
	if err != nil || len(out) < 2 || c.SelectedAvatarUnsafe().Progress.BasicRewards.Day != socialDay(*now) {
		t.Fatal("成功心跳没有刷新日界及原生状态", err)
	}
	after := c.SelectedAvatarUnsafe().Progress
	if after.Power != before.Power || !reflect.DeepEqual(after.Materials, before.Materials) {
		t.Fatal("日界刷新改变资产")
	}
}
