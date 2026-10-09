package game

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"strconv"
)

func (s *Service) consignRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("委托需要玩家状态")
	}
	direct := method == "commit_consign_task" || method == "reward_consign_phase_bonus"
	want := 2
	if method == "start_consign_task" {
		want = 3
	}
	if method == "commit_consign_task" {
		want = 1
	}
	if method == "reward_consign_phase_bonus" {
		want = 0
	}
	if len(args) != want {
		return nil, errors.New("委托参数数量无效")
	}
	cb, id := 0, 0
	if !direct {
		if json.Unmarshal(args[0], &cb) != nil || cb <= 0 {
			return nil, errors.New("委托回调无效")
		}
	}
	if method != "reward_consign_phase_bonus" {
		index := 1
		if direct {
			index = 0
		}
		if json.Unmarshal(args[index], &id) != nil || id <= 0 {
			return nil, errors.New("委托任务编号无效")
		}
	}
	var cards []int
	if method == "start_consign_task" {
		if json.Unmarshal(args[2], &cards) != nil {
			return nil, errors.New("委托幻书列表无效")
		}
	}
	var box map[string]any
	var returnedCards []int
	newID := id
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if err := ensureRemainingGameplay(p, s.Now()); err != nil {
			return err
		}
		w := p.RemainingGameplay.Consign
		if method == "reward_consign_phase_bonus" {
			if len(w.PhaseRecv) == 0 {
				return intimacyReject("RET_CONSIGN_TASK_NO_PHASE_BONUS", "没有未领取的委托阶段奖")
			}
			box = emptyActivityBox()
			changes := box["materials"].(map[int]int64)
			cardIDs := []string{}
			for mid, n := range w.PhaseRecv {
				if n <= 0 {
					return errors.New("委托待领奖数量无效")
				}
				if err := grantNativeItem(p, mid, n, p.AvatarLevel, s.Now(), changes, &cardIDs, 0); err != nil {
					return err
				}
			}
			box["cards"] = cardListWire(p, cardIDs)
			w.PhaseTotal[int(s.Now().Unix())] = w.PhaseRecv
			w.PhaseRecv = map[int]int64{}
			w.BonusTime = s.Now().Unix()
			return nil
		}
		task := w.Tasks[id]
		if task == nil {
			return intimacyReject("RET_CONSIGN_TASK_NOT_EXIST", "委托任务不存在")
		}
		row := remainingData("consign_task", id)
		if len(row) == 0 {
			return errors.New("委托模板不存在")
		}
		switch method {
		case "start_consign_task":
			teams, maxTasks, err := consignLimits(*p)
			if err != nil {
				return err
			}
			if w.FinishTimes >= maxTasks {
				return intimacyReject("RET_CONSIGN_TASK_MAX_COUNT_LIMIT", "每日委托次数用尽")
			}
			if task.StartTime > 0 {
				return intimacyReject("RET_CONSIGN_TASK_REPEAT_FIGHT", "委托已经开始")
			}
			if len(cards) == 0 {
				return intimacyReject("RET_CONSIGN_TASK_NOT_SET_CARDS", "委托未选择幻书")
			}
			if len(cards) > row.integer("teammate_num") {
				return intimacyReject("RET_CONSIGN_TASK_TEAM_NUM_LIMIT", "委托幻书人数超限")
			}
			seen := map[int]bool{}
			active := 0
			for _, other := range w.Tasks {
				if other.StartTime > 0 {
					active++
				}
			}
			if active >= teams {
				return intimacyReject("RET_CONSIGN_TASK_TEAM_NUM_LIMIT", "可同时委托队伍数用尽")
			}
			for _, cid := range cards {
				if seen[cid] {
					return intimacyReject("RET_CONSIGN_TASK_REPEAT_CARD", "重复委托幻书")
				}
				seen[cid] = true
				if !ownsCardID(*p, cid) {
					return intimacyReject("RET_CONSIGN_TASK_CARD_NOT_EXIST", "委托幻书不存在")
				}
				for _, other := range w.Tasks {
					if other.StartTime > 0 && containsInt(other.Cards, cid) {
						return intimacyReject("RET_CONSIGN_TASK_CARD_IN_CONSIGN", "幻书正在其他委托")
					}
				}
			}
			duration, err := consignDuration(*p, cards, row)
			if err != nil {
				return err
			}
			if s.Now().Unix() > math.MaxInt64-duration {
				return errors.New("委托结束时间溢出")
			}
			task.Cards = append([]int{}, cards...)
			task.StartTime = s.Now().Unix()
			task.FinishTime = task.StartTime + duration
		case "drop_consign_task":
			if task.StartTime == 0 {
				return intimacyReject("RET_CONSIGN_TASK_NOT_START", "委托未开始")
			}
			if task.FinishTime < s.Now().Unix() {
				return intimacyReject("RET_CONSIGN_TASK_HAS_FINISHED", "委托已经结束")
			}
			task.StartTime, task.FinishTime, task.Cards = 0, 0, []int{}
		case "refresh_consign_task_one":
			if task.StartTime > 0 {
				return intimacyReject("RET_CONSIGN_TASK_HAS_START", "进行中委托不能刷新")
			}
			cost := int64(remainingData("consign_base", 1).integer("refresh_consume"))
			if w.RefreshPoint < cost {
				return intimacyReject("RET_CONSIGN_TASK_REFRESH_POINT_NOT_ENOUGH", "委托刷新点不足")
			}
			pool, err := consignPoolAt(*p, task.Index, s.Now())
			if err != nil {
				return err
			}
			picked, err := consignPick(p, pool)
			if err != nil {
				return err
			}
			newID = picked
			delete(w.Tasks, id)
			w.Tasks[newID] = &ConsignTaskState{Index: task.Index, Level: p.AvatarLevel, Cards: []int{}}
			w.RefreshPoint -= cost
			w.RefreshTimes++
		case "commit_consign_task":
			if task.StartTime == 0 || task.FinishTime > s.Now().Unix() {
				return intimacyReject("RET_CONSIGN_TASK_NOT_FINISH", "委托尚未完成")
			}
			key := strconv.Itoa(row.integer("task_bonus_id")) + "_" + strconv.Itoa(task.Level)
			var bid int
			if json.Unmarshal(androidRemainingGameplay["consign_bonus_pool"][key], &bid) != nil || bid <= 0 {
				return errors.New("委托冻结等级奖励缺失")
			}
			box = emptyActivityBox()
			bonusIDs := []int{bid}
			returnedCards = append([]int{}, task.Cards...)
			if len(task.Cards) > 0 && containsInt(row.ids("hide_story_cards"), task.Cards[0]) {
				bonusIDs = append(bonusIDs, row.integer("hide_story_bonus"))
			}
			for _, bid := range bonusIDs {
				part, err := grantNativeBonus(p, bid, 1, task.Level, s.Now())
				if err != nil {
					return err
				}
				if err := mergeActivityBox(box, part); err != nil {
					return err
				}
			}
			phase := row.ids("phase_bonus")
			extra := row.ids("tag_extra_bonus")
			if len(phase) != 2 || len(extra) != 2 {
				return errors.New("委托阶段奖励配置错误")
			}
			acc := map[int]int64{phase[0]: int64(phase[1])}
			for _, cid := range task.Cards {
				tags, err := consignCardTags(*p, cid)
				if err != nil {
					return err
				}
				for _, tid := range tags {
					if containsInt(row.ids("task_tag"), tid) {
						if acc[extra[0]] > math.MaxInt64-int64(extra[1]) {
							return errors.New("委托阶段奖溢出")
						}
						acc[extra[0]] += int64(extra[1])
					}
				}
			}
			for mid, n := range acc {
				if w.PhaseRecv[mid] > math.MaxInt64-n {
					return errors.New("委托累计阶段奖溢出")
				}
				w.PhaseRecv[mid] += n
			}
			delete(w.Tasks, id)
			w.FinishTimes++
			advanceAchievementEvent(p, 7, 1, s.Now())
		default:
			return errors.New("未知委托接口")
		}
		return nil
	})
	code := RetSuccess
	if err != nil {
		log.Printf("委托事务拒绝 method=%s task=%d 原因=%v", method, id, err)
		var rejection *intimacyBusinessError
		if errors.As(err, &rejection) {
			rules, e := loadIntimacyCatalog()
			if e != nil {
				return nil, e
			}
			var exists bool
			code, exists = rules.Errors[rejection.name]
			if !exists {
				return nil, errors.New("委托原生错误码缺失")
			}
		} else {
			code = activityErrorCode(err)
		}
	}
	if err != nil {
		if method == "commit_consign_task" {
			return []Push{push("Avatar", "commit_consign_task_failed", id, code)}, nil
		}
		if method == "reward_consign_phase_bonus" {
			return []Push{push("Avatar", "on_reward_consign_phase_bonus", code, consignBonusWire(map[int]int64{}))}, nil
		}
		if method == "refresh_consign_task_one" {
			return []Push{Callback(cb, []any{code, id})}, nil
		}
		return []Push{Callback(cb, []any{code})}, nil
	}
	out := append(remainingGameplayPushes(c.SelectedAvatarUnsafe().Progress), materialManagerPush(c), cardMgrPush(c))
	if method == "reward_consign_phase_bonus" {
		return append(out, push("Avatar", "on_reward_consign_phase_bonus", RetSuccess, consignBonusWire(box["materials"].(map[int]int64)))), nil
	}
	if method == "commit_consign_task" {
		return append(out, push("Avatar", "commit_consign_task_success", id, returnedCards, box)), nil
	}
	if method == "refresh_consign_task_one" {
		return append(out, Callback(cb, []any{RetSuccess, newID})), nil
	}
	return append(out, Callback(cb, []any{RetSuccess})), nil
}

func consignBonusWire(items map[int]int64) map[string]any {
	return map[string]any{"__custom_type": "bonus.bonus", "bonus_id": 0, "contain_items": items, "contain_runes": map[int]int64{}, "buff_ids": []int{}, "is_double": false}
}
