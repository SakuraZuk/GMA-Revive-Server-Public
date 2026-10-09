package game

import (
	"encoding/json"
	"math"
	"strconv"
)

func mikuAchievementCount(a MikuAchievement, r activityRow) int {
	var groups [][]int
	if json.Unmarshal(r["target_id"], &groups) != nil || len(groups) == 0 {
		return 0
	}
	minimum := math.MaxInt32
	for _, group := range groups {
		count := 0
		for _, tid := range group {
			count += a.Targets[tid]
		}
		if count < minimum {
			minimum = count
		}
	}
	return minimum
}
func mikuTargetCount(p Progress, tid int) int {
	r := activityData("base_target", tid)
	args := r.ids("target_params")
	w := p.Activities.Miku
	if w == nil {
		return 0
	}
	switch r.integer("target_type") {
	case 2:
		if len(args) == 1 && containsInt(p.ClearedDungeons, args[0]) {
			return 1
		}
	case 50:
		return w.Runs
	case 52:
		return w.Ends
	case 53:
		if len(args) == 2 {
			return w.Visits[strconv.Itoa(args[0])+":"+strconv.Itoa(args[1])]
		}
	case 54:
		if len(args) == 1 {
			mid := activityData("miku_nodes", args[0]).integer("map_id")
			if w.Maps[mid] != nil {
				return 1
			}
		}
	case 56:
		if len(args) == 2 {
			m := w.Maps[args[1]]
			if m != nil && m.Score >= args[0] {
				return 1
			}
		}
	case 69:
		if len(args) == 1 && ownsCardID(p, args[0]) {
			return 1
		}
	case 70:
		if len(args) == 1 {
			return w.Gifts[args[0]]
		}
	case 71:
		if len(args) == 2 && args[0] == 208 {
			return w.Battles[args[1]]
		}
	}
	return 0
}

// 由原生base_target参数读取真实拥有、通关、地图与服务端事件账本，领取后不倒扣历史。
func reconcileMikuAchievements(p *Progress) {
	if p.Activities.Miku == nil {
		return
	}
	w := ensureMiku(p)
	for key := range androidActivities["miku_achv"] {
		id, _ := strconv.Atoi(key)
		r := activityData("miku_achv", id)
		a := w.Achievements[id]
		a.ID = id
		if a.Targets == nil {
			a.Targets = map[int]int{}
		}
		var groups [][]int
		_ = json.Unmarshal(r["target_id"], &groups)
		for _, group := range groups {
			for _, tid := range group {
				n := mikuTargetCount(*p, tid)
				need := r.integer("target_need_count")
				if n > need {
					n = need
				}
				if n > a.Targets[tid] {
					a.Targets[tid] = n
				}
			}
		}
		w.Achievements[id] = a
	}
}
func recordMikuGift(p *Progress, cardID int) {
	if p.Activities.Miku == nil {
		return
	}
	w := ensureMiku(p)
	if w.Gifts[cardID] < math.MaxInt32 {
		w.Gifts[cardID]++
	}
	reconcileMikuAchievements(p)
}
