package game

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

type TaskTowerFloor struct {
	TowerID  int           `json:"tower_id"`
	Floor    int           `json:"floor"`
	Finished []int         `json:"finish_task_list"`
	Received map[int]int64 `json:"gift_receive_state"`
}
type TaskTowerState struct {
	ID     int                     `json:"tower_id"`
	Floors map[int]*TaskTowerFloor `json:"floor_infos"`
}

func taskTowerDungeonPair(id int) []int {
	var pair []int
	_ = json.Unmarshal(androidRemainingGameplay["task_tower_dungeon"][strconv.Itoa(id)], &pair)
	if len(pair) != 2 {
		return nil
	}
	return pair
}
func taskTowerRow(tower, floor int) activityRow {
	var entries []struct {
		Key   []int       `json:"键"`
		Value activityRow `json:"值"`
	}
	_ = json.Unmarshal(androidRemainingGameplay["task_tower_info"]["键值条目"], &entries)
	for _, e := range entries {
		if len(e.Key) == 2 && e.Key[0] == tower && e.Key[1] == floor {
			return e.Value
		}
	}
	return nil
}
func taskTowerFloor(p *Progress, tower, floor int) *TaskTowerFloor {
	w := ensureRemainingState(p)
	if w.TaskTowers[tower] == nil {
		w.TaskTowers[tower] = &TaskTowerState{ID: tower, Floors: map[int]*TaskTowerFloor{}}
	}
	t := w.TaskTowers[tower]
	if t.Floors == nil {
		t.Floors = map[int]*TaskTowerFloor{}
	}
	if t.Floors[floor] == nil {
		t.Floors[floor] = &TaskTowerFloor{TowerID: tower, Floor: floor, Finished: []int{}, Received: map[int]int64{}}
	}
	if t.Floors[floor].Received == nil {
		t.Floors[floor].Received = map[int]int64{}
	}
	return t.Floors[floor]
}
func taskTowerRemainingTasks(p *Progress, tower, floor int) []int {
	state := taskTowerFloor(p, tower, floor)
	out := []int{}
	for _, id := range taskTowerRow(tower, floor).ids("tower_task_list") {
		if !containsInt(state.Finished, id) {
			out = append(out, id)
		}
		if len(out) == 3 {
			break
		}
	}
	return out
}
func remainingAuthorizedTaskDungeon(p *Progress, id int) bool {
	pair := taskTowerDungeonPair(id)
	if p.RemainingGameplay == nil || p.RemainingGameplay.PendingTaskDungeon != id || pair == nil {
		return false
	}
	row := taskTowerRow(pair[0], pair[1])
	return len(row) > 0 && !row.flag("disable_activity") && row.integer("dungeon_id") == id
}
func (s *Service) taskTowerRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 4 {
		return nil, errors.New("绝密记录需要回调、塔与层和附加字典")
	}
	var cb, tower, floor int
	var extra map[string]any
	if json.Unmarshal(args[0], &cb) != nil || cb <= 0 || json.Unmarshal(args[1], &tower) != nil || json.Unmarshal(args[2], &floor) != nil || json.Unmarshal(args[3], &extra) != nil || extra == nil {
		return nil, errors.New("绝密记录参数无效")
	}
	row := taskTowerRow(tower, floor)
	if len(row) == 0 || row.flag("disable_activity") {
		return nil, errors.New("绝密记录层不可用")
	}
	id := row.integer("dungeon_id")
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if p.Battle != nil && !p.Battle.Finished && p.Battle.DungeonID != id {
			return errors.New("已有其他战斗会话")
		}
		if err := activityData("dungeons", id).conditions("unlock_condition", *p, p.AvatarLevel); err != nil {
			return err
		}
		ensureRemainingState(p).PendingTaskDungeon = id
		return nil
	}); err != nil {
		return nil, err
	}
	return s.enterDungeon(ctx, c, []json.RawMessage{args[0], json.RawMessage(strconv.Itoa(id)), json.RawMessage(`{}`)})
}

// Android 376E5D47 每场最多冻结3个尚未完成任务；任务结果沿用普通副本原生权威。
func settleTaskTower(p *Progress, b *BattleSession, win bool, tasks []int, box map[string]any, now time.Time) error {
	pair := taskTowerDungeonPair(b.DungeonID)
	if pair == nil {
		return nil
	}
	if !remainingAuthorizedTaskDungeon(p, b.DungeonID) {
		return errors.New("绝密结果缺少专用战斗授权")
	}
	var allowed []int
	raw, err := json.Marshal(b.Extra["tower_task_list"])
	if err != nil || json.Unmarshal(raw, &allowed) != nil {
		return errors.New("绝密任务快照无效")
	}
	for _, id := range tasks {
		if !containsInt(allowed, id) {
			return errors.New("绝密结果包含未冻结的任务")
		}
	}
	state := taskTowerFloor(p, pair[0], pair[1])
	seen := map[int]bool{}
	for _, id := range tasks {
		if seen[id] || containsInt(state.Finished, id) {
			continue
		}
		seen[id] = true
		row := remainingData("tower_task", id)
		if len(row) == 0 {
			return errors.New("绝密任务配置不存在")
		}
		if !win && !row.flag("no_need_win") {
			continue
		}
		part, err := grantNativeBonus(p, row.integer("bonus_id"), 1, p.AvatarLevel, now)
		if err != nil {
			return err
		}
		if err := mergeActivityBox(box, part); err != nil {
			return err
		}
		state.Finished = append(state.Finished, id)
		state.Received[id] = now.Unix()
		advanceAchievementEvent(p, 48, pair[0], now)
	}
	p.RemainingGameplay.PendingTaskDungeon = 0
	return nil
}
