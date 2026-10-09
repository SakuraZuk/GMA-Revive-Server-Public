package game

import (
	"encoding/json"
	"errors"
	"time"
)

// 特别演练三个伤害成就由原生tower_task引擎判定；只接受当前副本配置的
// 同一任务。7151129/11037000/14383421阈值已在本版任务表明确，
// 不用命令数量或客户端任意伤害数字替代任务结算。
func recordSpecialDrillAchievements(p *Progress, b *BattleSession, win bool, tasks []int, now time.Time) error {
	allowed := []int{}
	raw := activityData("dungeons", b.DungeonID)["tower_task_list"]
	if len(raw) > 0 && string(raw) != "null" && json.Unmarshal(raw, &allowed) != nil {
		return errors.New("副本原生演练任务配置无效")
	}
	seen := map[int]bool{}
	for _, task := range tasks {
		isDrill := false
		for _, target := range androidAchievements.Targets {
			var id int
			if target.Type == 66 && len(target.Params) == 1 && json.Unmarshal(target.Params[0], &id) == nil && id == task {
				isDrill = true
				break
			}
		}
		if !isDrill {
			continue
		}
		if seen[task] || !containsInt(allowed, task) {
			return errors.New("特别演练成就任务重复或不属于本副本")
		}
		seen[task] = true
		if win {
			advanceAchievementEvent(p, 66, task, now)
		}
	}
	return nil
}
