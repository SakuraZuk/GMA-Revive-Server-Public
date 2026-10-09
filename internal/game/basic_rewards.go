package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"
)

//go:embed basic_rewards_catalog.json
var basicRewardsRaw []byte

var basicRewardTables = func() map[string]map[string]json.RawMessage {
	var data struct {
		Tables map[string]map[string]json.RawMessage `json:"表"`
	}
	if json.Unmarshal(basicRewardsRaw, &data) != nil || len(data.Tables["new_task"]) != 109 {
		panic("本版基础奖励目录无效")
	}
	return data.Tables
}()

func basicRewardRow(table string, id int) activityRow {
	var row activityRow
	_ = json.Unmarshal(basicRewardTables[table][strconv.Itoa(id)], &row)
	return row
}

type BasicDailyTask struct {
	ID      int           `json:"task_id"`
	Targets map[int]int64 `json:"finished_targets"`
	Claimed bool          `json:"is_bonus"`
}

// 私有收据和日界只能由服务端推进，原生属性由basicRewardProperties投影。
type BasicRewardState struct {
	Version      int                    `json:"version"`
	Start        int64                  `json:"start"`
	Day          string                 `json:"day"`
	Week         string                 `json:"week"`
	Daily        map[int]BasicDailyTask `json:"daily"`
	DailyActive  int                    `json:"daily_active"`
	WeeklyActive int                    `json:"weekly_active"`
	DailyClaims  []int                  `json:"daily_claims"`
	WeeklyClaims []int                  `json:"weekly_claims"`
	BuffCounts   map[int]int            `json:"buff_counts"`
	NewClaims    map[int]int64          `json:"new_claims"`
	CheckInDays  map[int]string         `json:"check_in_days"`
	ScoreClaims  map[int][]int          `json:"score_claims"`
}

type basicCheckIn struct {
	ID       int   `json:"check_in_id"`
	Count    int   `json:"check_in_count"`
	Received []int `json:"received_bonus"`
}

type basicRewardRejection struct{ message string }

func (e *basicRewardRejection) Error() string { return e.message }
func rejectBasic(message string) error        { return &basicRewardRejection{message} }

func basicWeek(now time.Time) string {
	local := now.In(time.FixedZone("北京时间", 8*3600))
	weekday := (int(local.Weekday()) + 6) % 7
	return local.AddDate(0, 0, -weekday).Format("2006-01-02")
}

func basicRowOpen(row activityRow, p Progress, now time.Time) bool {
	if len(row) == 0 || row.conditions("unlock_condition", p, p.AvatarLevel) != nil {
		return false
	}
	if id := row.integer("game_activity_id"); id > 0 {
		schedule := activitySchedule(id)
		if !schedule.Enabled || now.Unix() < schedule.Begin || (!schedule.Permanent && schedule.End > 0 && now.Unix() > schedule.End) {
			return false
		}
	} else {
		for _, key := range []string{"begin_time", "end_time"} {
			var values []int64
			if len(row[key]) > 0 && string(row[key]) != "null" && json.Unmarshal(row[key], &values) != nil {
				return false
			}
			var value int64
			for _, n := range values {
				value += n
			}
			if value > 0 && ((key == "begin_time" && now.Unix() < value) || (key == "end_time" && now.Unix() > value)) {
				return false
			}
		}
	}
	weekdays := row.ids("open_time")
	weekday := (int(now.In(time.FixedZone("北京时间", 8*3600)).Weekday())+6)%7 + 1
	return len(weekdays) == 0 || containsInt(weekdays, weekday)
}

// 登录只计真实出现的这一天；离线数日不补造签到、任务或活跃度。
func ensureBasicRewards(p *Progress, now time.Time, login bool) error {
	day, week := socialDay(now), basicWeek(now)
	if p.BasicRewards == nil {
		start := now.Unix()
		if first, ok := p.GuideTasks[1000]; ok && first.BeginTime > 0 && first.BeginTime <= now.Unix() {
			start = first.BeginTime
		}
		p.BasicRewards = &BasicRewardState{Start: start, NewClaims: map[int]int64{}, CheckInDays: map[int]string{}}
	}
	b := p.BasicRewards
	if b.Day > day || b.Week > week || b.Start > now.Unix() {
		return errors.New("基础奖励日界发生时钟回拨")
	}
	if b.NewClaims == nil {
		b.NewClaims = map[int]int64{}
	}
	if b.CheckInDays == nil {
		b.CheckInDays = map[int]string{}
	}
	if b.ScoreClaims == nil {
		b.ScoreClaims = map[int][]int{}
	}
	if b.Version == 0 {
		// 旧版没有新手领奖入口却将完成写为status=2；仅无领取收据的旧态恢复为可领取。
		for id, task := range p.NewTasks {
			if task.Status == 2 && b.NewClaims[id] == 0 {
				task.Status = 1
				p.NewTasks[id] = task
			}
		}
		b.Version = 1
	}
	if b.Day != day {
		b.Day, b.DailyActive, b.DailyClaims = day, 0, []int{}
		b.Daily, b.BuffCounts = map[int]BasicDailyTask{}, map[int]int{}
	}
	if b.Week != week {
		b.Week, b.WeeklyActive, b.WeeklyClaims = week, 0, []int{}
	}
	if b.Daily == nil {
		b.Daily = map[int]BasicDailyTask{}
	}
	if b.BuffCounts == nil {
		b.BuffCounts = map[int]int{}
	}
	for key := range basicRewardTables["daily_task"] {
		id, _ := strconv.Atoi(key)
		row := basicRewardRow("daily_task", id)
		// 多活动常驻不能将同一基础日常的全部替代版本并列发活跃度。
		// 本批补基础日常；替代活动的唯一选择规则另行取证，不伪造额外任务。
		if row.integer("replace_task_id") > 0 {
			continue
		}
		if _, ok := b.Daily[id]; !ok && basicRowOpen(row, *p, now) {
			b.Daily[id] = BasicDailyTask{ID: id, Targets: map[int]int64{}}
		}
	}
	if p.NewTasks == nil {
		p.NewTasks = map[int]NewTaskProgress{}
	}
	startDay := time.Unix(b.Start, 0).In(time.FixedZone("北京时间", 8*3600))
	startDay = time.Date(startDay.Year(), startDay.Month(), startDay.Day(), 0, 0, 0, 0, startDay.Location())
	age := int(now.In(startDay.Location()).Sub(startDay).Hours()/24) + 1
	for key := range basicRewardTables["new_task"] {
		id, _ := strconv.Atoi(key)
		row := basicRewardRow("new_task", id)
		task, exists := p.NewTasks[id]
		if !exists {
			task = NewTaskProgress{TaskID: id, FinishedTargets: map[int]int{}}
		}
		if task.Status == 0 && age >= row.integer("day") {
			task.Status = 1
		}
		task.Expired = age > basicRewardRow("new_task_params", 1).integer("max_days")
		p.NewTasks[id] = task
	}
	reconcileBasicRewardState(p)
	if !login {
		return nil
	}
	if p.CheckInRecords == nil {
		p.CheckInRecords = map[int]any{}
	}
	for key := range basicRewardTables["check_in_bonus"] {
		id, _ := strconv.Atoi(key)
		row := basicRewardRow("check_in_bonus", id)
		if !basicRowOpen(row, *p, now) || b.CheckInDays[id] == day {
			continue
		}
		var record basicCheckIn
		raw, err := json.Marshal(p.CheckInRecords[id])
		if err != nil || json.Unmarshal(raw, &record) != nil {
			return errors.New("签到存档无效")
		}
		if record.Count < 0 {
			return errors.New("签到累计次数无效")
		}
		var bonuses [][]int
		_ = json.Unmarshal(row["check_in_bonus"], &bonuses)
		maximum := 0
		for _, pair := range bonuses {
			if len(pair) == 2 {
				maximum = max(maximum, pair[0])
			}
		}
		if record.Count < maximum {
			record.Count++
		}
		record.ID = id
		if record.Received == nil {
			record.Received = []int{}
		}
		p.CheckInRecords[id], b.CheckInDays[id] = record, day
	}
	return nil
}

func basicRewardProperties(p Progress) map[string]any {
	out := map[string]any{}
	if p.BasicRewards == nil {
		return out
	}
	b := p.BasicRewards
	out["daily_tasks"] = activityWire(b.Daily)
	out["daily_active"], out["weekly_active"] = b.DailyActive, b.WeeklyActive
	out["daily_bonus_active"], out["weekly_bonus_active"] = activityWire(b.DailyClaims), activityWire(b.WeeklyClaims)
	out["activity_buff_count"] = activityWire(b.BuffCounts)
	out["score_bonus_record"] = activityWire(b.ScoreClaims)
	out["check_in_records"] = activityWire(p.CheckInRecords)
	out["new_tasks"] = newTaskProperties(p.NewTasks)
	return out
}

func basicRewardPushes(p Progress) []Push {
	properties := basicRewardProperties(p)
	out := []Push{}
	for _, key := range []string{"daily_tasks", "daily_active", "weekly_active", "daily_bonus_active", "weekly_bonus_active", "activity_buff_count", "score_bonus_record", "check_in_records", "new_tasks"} {
		if value, ok := properties[key]; ok {
			out = append(out, push("Avatar", "client_prop_changed", []any{key, value}))
		}
	}
	return out
}

func basicTargetCount(targets map[int]int64, row activityRow) int64 {
	var groups [][]int
	_ = json.Unmarshal(row["target_id"], &groups)
	if len(groups) == 0 {
		return 0
	}
	minimum := int64(math.MaxInt64)
	for _, group := range groups {
		var sum int64
		for _, id := range group {
			if targets[id] < 0 || sum > math.MaxInt64-targets[id] {
				return 0
			}
			sum += targets[id]
		}
		minimum = min(minimum, sum)
	}
	return minimum
}

// 当前拥有态可从真实存档重建；累计消费/战斗次数缺历史时不得凭资产倒推。
func reconcileBasicRewardState(p *Progress) {
	if p.BasicRewards == nil {
		return
	}
	for id, task := range p.NewTasks {
		if task.Status > 1 || task.Expired {
			continue
		}
		row := basicRewardRow("new_task", id)
		var groups [][]int
		_ = json.Unmarshal(row["target_id"], &groups)
		for _, group := range groups {
			for _, tid := range group {
				target := basicRewardRow("base_target", tid)
				var params []int
				if json.Unmarshal(target["target_params"], &params) != nil {
					continue
				}
				count := 0
				switch target.integer("target_type") {
				case 2:
					if len(params) == 1 && containsInt(p.ClearedDungeons, params[0]) {
						count = 1
					}
				case 14:
					if len(params) == 2 && params[0] == 0 {
						count = int(achievementFullRuneCards(*p, params[1]))
					}
				case 15, 16:
					if len(params) != 1 {
						continue
					}
					for _, card := range p.Cards {
						if target.integer("target_type") == 15 && card.Level >= params[0] || target.integer("target_type") == 16 && card.Grade >= params[0] {
							count++
						}
					}
				case 17:
					if len(params) != 1 {
						continue
					}
					for _, rune := range p.Runes {
						if rune.Level-1 >= params[0] {
							count++
						}
					}
				default:
					continue
				}
				if task.FinishedTargets == nil {
					task.FinishedTargets = map[int]int{}
				}
				if count > task.FinishedTargets[tid] {
					task.FinishedTargets[tid] = min(row.integer("target_need_count"), count)
				}
			}
		}
		p.NewTasks[id] = task
	}
}

// 复用成功事务中的原生成就事件，不接受客户端报完成/活跃度。事件类型和参数逐项匹配base_target。
func advanceBasicRewardEvent(p *Progress, eventType int, params []any, amount int64, now time.Time) {
	if amount <= 0 || p.BasicRewards == nil || ensureBasicRewards(p, now, false) != nil {
		return
	}
	want, _ := json.Marshal(params)
	for key := range basicRewardTables["base_target"] {
		tid, _ := strconv.Atoi(key)
		target := basicRewardRow("base_target", tid)
		var actual []any
		_ = json.Unmarshal(target["target_params"], &actual)
		if actual == nil {
			actual = []any{}
		}
		raw, _ := json.Marshal(actual)
		if target.integer("target_type") != eventType || string(raw) != string(want) {
			continue
		}
		for id, task := range p.BasicRewards.Daily {
			row := basicRewardRow("daily_task", id)
			if task.Claimed || !basicRowOpen(row, *p, now) {
				continue
			}
			var groups [][]int
			_ = json.Unmarshal(row["target_id"], &groups)
			matched := false
			for _, group := range groups {
				matched = matched || containsInt(group, tid)
			}
			if !matched {
				continue
			}
			cap := int64(row.integer("task_limit_count")) * int64(row.integer("target_need_count"))
			if cap <= 0 {
				continue
			}
			if task.Targets == nil {
				task.Targets = map[int]int64{}
			}
			task.Targets[tid] += min(basicTargetRoom(task.Targets, row, tid, cap), amount)
			p.BasicRewards.Daily[id] = task
		}
		for id, task := range p.NewTasks {
			row := basicRewardRow("new_task", id)
			if task.Status > 1 || task.Expired {
				continue
			}
			var groups [][]int
			_ = json.Unmarshal(row["target_id"], &groups)
			matched := false
			for _, group := range groups {
				matched = matched || containsInt(group, tid)
			}
			if !matched {
				continue
			}
			if task.FinishedTargets == nil {
				task.FinishedTargets = map[int]int{}
			}
			cap := row.integer("target_need_count")
			targets := map[int]int64{}
			for k, count := range task.FinishedTargets {
				targets[k] = int64(count)
			}
			task.FinishedTargets[tid] += int(min(basicTargetRoom(targets, row, tid, int64(cap)), amount))
			p.NewTasks[id] = task
		}
	}
}

// 原生new_task.is_finished使用等于而非大于；或目标组求和也必须封顶。
func basicTargetRoom(targets map[int]int64, row activityRow, tid int, cap int64) int64 {
	if cap <= 0 {
		return 0
	}
	var groups [][]int
	_ = json.Unmarshal(row["target_id"], &groups)
	room := cap
	matched := false
	for _, group := range groups {
		if !containsInt(group, tid) {
			continue
		}
		matched = true
		var sum int64
		for _, id := range group {
			if targets[id] < 0 || targets[id] >= cap || sum > cap-targets[id] {
				return 0
			}
			sum += targets[id]
		}
		room = min(room, cap-sum)
	}
	if !matched {
		return 0
	}
	return room
}

func (s *Service) basicRewardRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) < 2 || len(args) > 3 {
		return nil, errors.New("基础奖励需要回调及原生编号")
	}
	cb, ok := callbackArg(args)
	var id int
	if !ok || cb <= 0 || json.Unmarshal(args[1], &id) != nil || id <= 0 {
		return nil, errors.New("基础奖励参数无效")
	}
	if (method == "receive_check_in_bonus" || method == "receive_score_bonus") != (len(args) == 3) {
		return nil, errors.New("基础奖励参数数量无效")
	}
	var day int
	if len(args) == 3 && (json.Unmarshal(args[2], &day) != nil || day <= 0) {
		return nil, errors.New("签到奖励天数无效")
	}
	box := emptyActivityBox()
	boxes := map[int]any{}
	active := 0
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if err := ensureBasicRewards(p, s.Now(), true); err != nil {
			return err
		}
		b := p.BasicRewards
		var bonus int
		switch method {
		case "receive_power_supply":
			row := basicRewardRow("bonus_buff", id)
			// 本版仅这四条是可领取补给；其他倍率buff不能作为奖励凭证。
			if !containsInt([]int{8, 9, 10, 18}, id) || row.integer("bonus_id") <= 0 {
				return rejectBasic("补给编号无效")
			}
			local := s.Now().In(time.FixedZone("北京时间", 8*3600))
			second := local.Hour()*3600 + local.Minute()*60 + local.Second()
			if second < row.integer("begin_time") || second > row.integer("end_time") {
				return rejectBasic("补给未到领取时段")
			}
			if b.BuffCounts[id] >= row.integer("max_count") {
				return rejectBasic("本日补给已领取")
			}
			bonus = row.integer("bonus_id")
			b.BuffCounts[id]++
		case "receive_new_task_bonus":
			row := basicRewardRow("new_task", id)
			task, exists := p.NewTasks[id]
			targets := map[int]int64{}
			for tid, value := range task.FinishedTargets {
				targets[tid] = int64(value)
			}
			if len(row) == 0 || !exists || task.Status != 1 || task.Expired || b.NewClaims[id] != 0 || basicTargetCount(targets, row) < int64(row.integer("target_need_count")) {
				return rejectBasic("新手任务未完成、未开放、过期或已领取")
			}
			bonus = row.integer("bonus_id")
			task.Status, task.Time = 2, s.Now().Unix()
			p.NewTasks[id], b.NewClaims[id] = task, s.Now().Unix()
		case "receive_score_bonus":
			row := basicRewardRow("score_bonus", id)
			var rewards [][]int
			_ = json.Unmarshal(row["score_bonus_list"], &rewards)
			if len(row) == 0 || containsInt(b.ScoreClaims[id], day) || p.Materials[row.integer("score_id")].Count < int64(day) {
				return rejectBasic("累计积分奖励未达条件或已领取")
			}
			for _, pair := range rewards {
				if len(pair) == 2 && pair[0] == day {
					bonus = pair[1]
				}
			}
			if bonus <= 0 {
				return rejectBasic("积分奖励档位不存在")
			}
			b.ScoreClaims[id] = append(b.ScoreClaims[id], day)
		case "receive_daily_task_active":
			row := basicRewardRow("daily_task", id)
			task, exists := b.Daily[id]
			need, count := row.integer("target_need_count"), row.integer("task_limit_count")
			if !exists || !basicRowOpen(row, *p, s.Now()) || task.Claimed || row.integer("auto_add") != 0 || need <= 0 || count <= 0 || basicTargetCount(task.Targets, row)/int64(need) < int64(count) {
				return rejectBasic("日常任务尚未完成或已经领取")
			}
			active = row.integer("add_active")
			if active <= 0 || b.DailyActive > math.MaxInt32-active || b.WeeklyActive > math.MaxInt32-active {
				return rejectBasic("任务活跃度无效")
			}
			b.DailyActive += active
			b.WeeklyActive += active
			task.Claimed = true
			b.Daily[id] = task
			return nil
		case "receive_daily_active_bonus", "receive_weekly_active_bonus":
			table, available, claims := "daily_task_bonus", b.DailyActive, b.DailyClaims
			if method == "receive_weekly_active_bonus" {
				table, available, claims = "weekly_task_bonus", b.WeeklyActive, b.WeeklyClaims
			}
			row := basicRewardRow(table, id)
			if !basicRowOpen(row, *p, s.Now()) || row.integer("active_scale") <= 0 || available < row.integer("active_scale") || containsInt(claims, id) {
				return rejectBasic("活跃度奖励尚未达到条件或已领取")
			}
			bonus = row.integer("bonus_id")
			if method == "receive_weekly_active_bonus" {
				b.WeeklyClaims = append(b.WeeklyClaims, id)
			} else {
				b.DailyClaims = append(b.DailyClaims, id)
			}
		case "receive_check_in_bonus", "receive_all_check_in_bonus":
			row := basicRewardRow("check_in_bonus", id)
			if !basicRowOpen(row, *p, s.Now()) {
				return rejectBasic("签到活动未开放")
			}
			var record basicCheckIn
			raw, err := json.Marshal(p.CheckInRecords[id])
			if err != nil || json.Unmarshal(raw, &record) != nil || record.ID != id {
				return rejectBasic("签到记录不存在")
			}
			var rewards [][]int
			if json.Unmarshal(row["check_in_bonus"], &rewards) != nil {
				return errors.New("签到奖励配置无效")
			}
			for _, pair := range rewards {
				if len(pair) != 2 || pair[0] > record.Count || containsInt(record.Received, pair[0]) || (method == "receive_check_in_bonus" && pair[0] != day) {
					continue
				}
				part, err := grantNativeBonus(p, pair[1], 1, p.AvatarLevel, s.Now())
				if err != nil {
					return rejectBasic("签到奖励未发出：" + err.Error())
				}
				boxes[pair[0]] = activityWire(part)
				if err := mergeActivityBox(box, part); err != nil {
					return err
				}
				record.Received = append(record.Received, pair[0])
			}
			if len(boxes) == 0 {
				return rejectBasic("签到没有可领取奖励")
			}
			p.CheckInRecords[id] = record
			return nil
		default:
			return errors.New("基础奖励方法无效")
		}
		var err error
		box, err = grantNativeBonus(p, bonus, 1, p.AvatarLevel, s.Now())
		if err != nil {
			return rejectBasic("奖励未发出：" + err.Error())
		}
		return nil
	})
	if err != nil {
		var rejection *basicRewardRejection
		if !errors.As(err, &rejection) {
			return nil, err
		} // 存储故障不伪装业务拒绝或成功。
		switch method {
		case "receive_power_supply", "receive_score_bonus":
			return []Push{Callback(cb, []any{nil, err.Error()})}, nil
		case "receive_daily_task_active":
			return []Push{Callback(cb, []any{false, err.Error(), 0})}, nil
		case "receive_all_check_in_bonus":
			return []Push{Callback(cb, []any{1, activityWire(map[int]any{})})}, nil
		default:
			return []Push{Callback(cb, []any{1, activityWire(emptyActivityBox())})}, nil
		}
	}
	p := c.SelectedAvatarUnsafe().Progress
	out := basicRewardPushes(p)
	if method == "receive_daily_task_active" {
		return append(out, Callback(cb, []any{true, "", active})), nil
	}
	out = append(out, materialManagerPush(c), cardMgrPush(c), push("Avatar", "client_prop_changed", []any{"rune_mgr", runeMgrProperties(p.Runes)}), powerPush(c), knowledgePush(c))
	switch method {
	case "receive_power_supply", "receive_score_bonus":
		return append(out, Callback(cb, []any{activityWire(box), ""})), nil
	case "receive_all_check_in_bonus":
		return append(out, Callback(cb, []any{RetSuccess, activityWire(boxes)})), nil
	default:
		return append(out, Callback(cb, []any{RetSuccess, activityWire(box)})), nil
	}
}
