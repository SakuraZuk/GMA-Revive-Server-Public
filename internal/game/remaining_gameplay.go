package game

import (
	_ "embed"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

//go:embed remaining_gameplay_catalog.json
var remainingGameplayCatalogRaw []byte

var androidRemainingGameplay = func() map[string]map[string]json.RawMessage {
	var doc struct {
		Tables map[string]map[string]json.RawMessage `json:"表"`
	}
	if err := json.Unmarshal(remainingGameplayCatalogRaw, &doc); err != nil || len(doc.Tables["free_stage_site"]) != 70 {
		panic("探索委托绝密原生目录无效")
	}
	return doc.Tables
}()

func remainingData(table string, id int) activityRow {
	var row activityRow
	_ = json.Unmarshal(androidRemainingGameplay[table][strconv.Itoa(id)], &row)
	return row
}

// 状态仅存于玩家行锁JSONB；所有完成记录与奖励、成就同事务提交。
type RemainingGameplayState struct {
	Stages             map[int]*FreeStageState `json:"stages"`
	CurrentStage       int                     `json:"cur_free_stage_id"`
	PendingSite        int                     `json:"pending_site"`
	PendingDungeon     int                     `json:"pending_dungeon"`
	TaskTowers         map[int]*TaskTowerState `json:"task_towers"`
	PendingTaskDungeon int                     `json:"pending_task_dungeon"`
	Consign            *ConsignState           `json:"consign"`
	PowerTypes         map[int]int             `json:"power_types"`
	PowerType          int                     `json:"power_type"`
	AutoList           []int                   `json:"auto_list"`
	AutoFight          bool                    `json:"auto_fight"`
	AutoFighting       bool                    `json:"auto_fighting"`
}

func ensureRemainingState(p *Progress) *RemainingGameplayState {
	if p.RemainingGameplay == nil {
		p.RemainingGameplay = &RemainingGameplayState{}
	}
	w := p.RemainingGameplay
	if w.Stages == nil {
		w.Stages = map[int]*FreeStageState{}
	}
	if w.TaskTowers == nil {
		w.TaskTowers = map[int]*TaskTowerState{}
	}
	if w.PowerTypes == nil {
		w.PowerTypes = map[int]int{}
	}
	if w.PowerType == 0 {
		w.PowerType = 1
	}
	if w.AutoList == nil {
		w.AutoList = []int{}
	}
	return w
}

func remainingGameplayProperties(p Progress) map[string]any {
	w := p.RemainingGameplay
	if w == nil {
		w = &RemainingGameplayState{PowerType: 1, AutoList: []int{}, PowerTypes: map[int]int{}}
	}
	towers := map[int]*TaskTowerState{}
	for id, tower := range w.TaskTowers {
		towers[id] = tower
	}
	props := map[string]any{
		"free_stage_info": freeStageStatesWire(w.Stages), "cur_free_stage_id": w.CurrentStage,
		"task_tower":       map[string]any{"tower_infos": activityWire(towers)},
		"free_stage_power": w.PowerType, "free_stage_power_dict": activityWire(w.PowerTypes), "auto_list": w.AutoList, "auto_fight": w.AutoFight, "auto_fighting": w.AutoFighting,
	}
	for name, value := range consignProperties(p, w.Consign) {
		props[name] = value
	}
	return props
}

func remainingGameplayPushes(p Progress) []Push {
	props := remainingGameplayProperties(p)
	out := []Push{}
	for _, name := range []string{"free_stage_info", "cur_free_stage_id", "free_stage_power", "free_stage_power_dict", "auto_list", "auto_fight", "auto_fighting", "task_tower", "consign_tasks", "consign_finish_times", "consign_phase_recv_bonus", "consign_phase_total_bonus", "consign_bonus_time", "consign_refresh_point", "consign_next_refresh_time"} {
		out = append(out, push("Avatar", "client_prop_changed", []any{name, props[name]}))
	}
	return out
}

// 新会话创建后、开始下行前冻结服务端授权；恢复与准备态重入复用原Extra。
func prepareRemainingDungeon(p *Progress, b *BattleSession, now time.Time) error {
	if b.Extra == nil {
		b.Extra = map[string]any{}
	}
	delete(b.Extra, "server_free_stage_id")
	delete(b.Extra, "server_free_stage_node")
	delete(b.Extra, "server_task_tower")
	delete(b.Extra, "server_free_stage_reward_multiple")
	delete(b.Extra, "tower_task_list")
	if tasks := activityData("dungeons", b.DungeonID).ids("tower_task_list"); len(tasks) > 0 {
		b.Extra["tower_task_list"] = append([]int{}, tasks...)
	}
	if pair := taskTowerDungeonPair(b.DungeonID); pair != nil {
		if !remainingAuthorizedTaskDungeon(p, b.DungeonID) {
			return errors.New("绝密副本必须经专用入口授权")
		}
		tasks := taskTowerRemainingTasks(p, pair[0], pair[1])
		b.Extra["tower_task_list"] = tasks
		b.Extra["server_task_tower"] = append([]int{}, pair...)
	}
	if dungeonCatalog[b.DungeonID].Type == 2 {
		w := ensureRemainingState(p)
		stage := w.Stages[w.CurrentStage]
		if stage == nil || w.PendingSite <= 0 || w.PendingDungeon != b.DungeonID || stage.Sites[w.PendingSite] == nil {
			return errors.New("探索副本必须来自当前可探索节点")
		}
		b.Extra["server_free_stage_id"], b.Extra["server_free_stage_node"] = w.CurrentStage, w.PendingSite
		multiple := w.PowerType
		if multiple == 0 {
			multiple = 1
		}
		if multiple != 1 && multiple != 2 {
			return errors.New("探索体力倍率存档无效")
		}
		if w.AutoFighting {
			if n := w.PowerTypes[w.PendingSite]; n > 0 {
				multiple = n
			}
		}
		if multiple != 1 && multiple != 2 {
			return errors.New("自动探索体力倍率存档无效")
		}
		if stage.Sites[w.PendingSite].Type == 4 && stage.BossDouble {
			multiple *= 2
		}
		b.Extra["server_free_stage_reward_multiple"] = multiple
	}
	return nil
}

func remainingDungeonPowerMultiple(p *Progress, id int) int {
	if dungeonCatalog[id].Type != 2 || p.RemainingGameplay == nil || p.RemainingGameplay.PendingDungeon != id {
		return 1
	}
	w := p.RemainingGameplay
	n := w.PowerType
	if w.AutoFighting {
		if v := w.PowerTypes[w.PendingSite]; v > 0 {
			n = v
		}
	}
	if n == 2 {
		return 2
	}
	return 1
}
func remainingDungeonRewardAmount(b *BattleSession, win bool) int64 {
	if !win || dungeonCatalog[b.DungeonID].Type != 2 {
		return 1
	}
	n := remainingExtraInt(b.Extra, "server_free_stage_reward_multiple")
	if n == 2 || n == 4 {
		return int64(n)
	}
	return 1
}
func remainingDungeonCompletionPushes(p Progress, b *BattleSession) []Push {
	if b == nil || b.Outcome != "win" || dungeonCatalog[b.DungeonID].Type != 2 {
		return nil
	}
	node, id := remainingExtraInt(b.Extra, "server_free_stage_node"), remainingExtraInt(b.Extra, "server_free_stage_id")
	if p.RemainingGameplay == nil || p.RemainingGameplay.Stages[id] == nil || node <= 0 {
		return nil
	}
	out := []Push{push("Avatar", "on_finish_stage_site", node)}
	if p.RemainingGameplay.Stages[id].State == 3 {
		out = append(out, push("Avatar", "on_finish_free_stage", id))
	}
	return out
}

func remainingExtraInt(extra map[string]any, key string) int {
	value := extra[key]
	switch n := value.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		if float64(int(n)) == n {
			return int(n)
		}
	}
	return 0
}

// 返回的任务/探索附加奖合入本场冻结盒；重复result由原有战斗序号/Finished挡住。
func settleRemainingDungeon(p *Progress, b *BattleSession, win bool, tasks []int, box map[string]any, now time.Time) error {
	if err := settleTaskTower(p, b, win, tasks, box, now); err != nil {
		return err
	}
	if err := settleFreeStage(p, b, win, box, now); err != nil {
		return err
	}
	return nil
}
