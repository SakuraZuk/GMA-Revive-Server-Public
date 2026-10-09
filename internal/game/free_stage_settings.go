package game

import (
	"context"
	"encoding/json"
	"errors"
)

// 原生auto_list与free_stage_power_dict以节点编号为键；不能上传通关或节点完成。
func (s *Service) freeStageSettingsRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	callback := method == "set_free_stage_power" || method == "set_auto_free_stage_power"
	want := 1
	if callback || method == "unlock_chapter" {
		want = 2
	}
	if len(args) != want {
		return nil, errors.New("探索设置参数数量无效")
	}
	cb := 0
	index := 0
	if callback {
		if json.Unmarshal(args[0], &cb) != nil || cb <= 0 {
			return nil, errors.New("探索设置回调无效")
		}
		index = 1
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		w := ensureRemainingState(p)
		switch method {
		case "set_free_stage_power":
			var n int
			if json.Unmarshal(args[index], &n) != nil || (n != 1 && n != 2) {
				return errors.New("探索体力倍率必须为1或2")
			}
			w.PowerType = n
		case "set_auto_free_stage_power":
			var values map[int]int
			if json.Unmarshal(args[index], &values) != nil || values == nil {
				return errors.New("自动探索体力字典无效")
			}
			for id, n := range values {
				if n != 1 && n != 2 {
					return errors.New("自动探索体力倍率无效")
				}
				stageID := freeStageNodeChapter(id)
				if stageID == 0 {
					return errors.New("自动探索体力节点不存在")
				}
				if err := freeStageUnlocked(*p, stageID); err != nil {
					return err
				}
			}
			w.PowerTypes = values
		case "set_auto_state", "set_auto_fighting":
			var value bool
			if json.Unmarshal(args[0], &value) != nil {
				return errors.New("自动探索状态无效")
			}
			if method == "set_auto_state" {
				w.AutoFight = value
			} else {
				w.AutoFighting = value
			}
		case "set_auto_list":
			var values []int
			if json.Unmarshal(args[0], &values) != nil || len(values) > len(freeStageNodes(w.CurrentStage)) {
				return errors.New("自动探索队列无效")
			}
			seen := map[int]bool{}
			for _, id := range values {
				if seen[id] {
					return errors.New("自动探索节点不能重复")
				}
				seen[id] = true
				if !containsInt(freeStageNodes(w.CurrentStage), id) {
					return errors.New("自动探索节点不属于当前章节")
				}
			}
			w.AutoList = append([]int{}, values...)
		case "unlock_chapter":
			var id, state int
			if json.Unmarshal(args[0], &id) != nil || json.Unmarshal(args[1], &state) != nil || state != 2 {
				return errors.New("探索章节确认参数无效")
			}
			if err := remainingChapterUnlocked(*p, id); err != nil {
				return err
			}
			if p.UnlockedChapters == nil {
				p.UnlockedChapters = map[int]int{}
			}
			p.UnlockedChapters[id] = state
		default:
			return errors.New("未知探索设置接口")
		}
		return nil
	})
	if err != nil {
		if callback {
			return []Push{Callback(cb, []any{activityErrorCode(err)})}, nil
		}
		return []Push{nativeErrorPush(activityErrorCode(err))}, nil
	}
	out := remainingGameplayPushes(c.SelectedAvatarUnsafe().Progress)
	if callback {
		return append(out, Callback(cb, []any{RetSuccess})), nil
	}
	p := c.SelectedAvatarUnsafe().Progress.RemainingGameplay
	switch method {
	case "set_auto_state":
		out = append(out, push("Avatar", "on_set_auto_state", p.AutoFight))
	case "set_auto_fighting":
		out = append(out, push("Avatar", "on_set_auto_fighting", p.AutoFighting))
	case "set_auto_list":
		out = append(out, push("Avatar", "on_set_auto_list"))
	case "unlock_chapter":
		out = append(out, push("Avatar", "client_prop_changed", []any{"unlocked_chapters", activityWire(c.SelectedAvatarUnsafe().Progress.UnlockedChapters)}))
		out = append(out, push("Avatar", "on_unlock_chapter"))
	}
	return out, nil
}

func freeStageNodeChapter(node int) int {
	var stages map[int][]int
	_ = json.Unmarshal(androidRemainingGameplay["free_stage_nodes"]["free_stage_nodes"], &stages)
	for id, nodes := range stages {
		if containsInt(nodes, node) {
			return id
		}
	}
	return 0
}
