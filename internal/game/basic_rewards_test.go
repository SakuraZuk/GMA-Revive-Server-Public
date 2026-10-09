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
