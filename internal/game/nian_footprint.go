package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"
)

// 27A317D7原生属性：current_site/real_site/open_grid_bonuses下发，次数与随机池只保存。
type NianFootprintState struct {
	Site      int         `json:"current_site"`
	Real      int         `json:"real_site"`
	Opened    map[int]int `json:"open_grid_bonuses"`
	Times     int         `json:"activity_times" wire:"-"`
	Remaining []int       `json:"surplus_bonus_indexes" wire:"-"`
}

func ensureNianFootprint(p *Progress) *NianFootprintState {
	if p.Activities.Footprint == nil {
		p.Activities.Footprint = &NianFootprintState{Site: 1, Real: 1, Opened: map[int]int{}, Remaining: []int{}}
	}
	return p.Activities.Footprint
}
func nianSample(pool []int, count int) ([]int, error) {
	if count < 0 || count > len(pool) {
		return nil, errors.New("北风足迹随机池样本数量无效")
	}
	pool = append([]int{}, pool...)
	out := []int{}
	for len(out) < count {
		i, e := activityAmount([]int64{0, int64(len(pool) - 1)})
		if e != nil {
			return nil, e
		}
		out = append(out, pool[i])
		pool = append(pool[:i], pool[i+1:]...)
	}
	return out, nil
}
func refreshNianFootprint(w *NianFootprintState) error {
	r := activityData("north_wind_footprint", 1)
	if w.Site >= math.MaxInt32 {
		return errors.New("北风足迹层数溢出")
	}
	w.Site++
	w.Opened = map[int]int{}
	w.Remaining = []int{}
	start, cycle := r.integer("endless_mode_start_site"), r.integer("endless_mode_cycle")
	if start <= 0 || cycle <= 0 {
		return errors.New("北风足迹循环配置无效")
	}
	w.Real = w.Site
	if w.Site >= start {
		w.Real = (w.Site-start)%cycle + start
	}
	if w.Site >= r.integer("prob_modifier_start_site") {
		// Android27A317D7：fluctuate=current_site-1-activity_times/5；整数计数按Python2原生整除。
		fluct := w.Site - 1 - w.Times/5
		low, high := r.integer("prob_modifier_fluctuate_min"), r.integer("prob_modifier_fluctuate_max")
		pool := []int{1, 2, 3, 4, 5, 6, 7, 8}
		if fluct < low {
			count := 9 - (low-fluct)*2
			if count < 1 {
				count = 1
			}
			picked, e := nianSample(pool, count-1)
			if e != nil {
				return e
			}
			w.Remaining = append(picked, 0)
		}
		if fluct > high {
			count := (fluct - high) * 2
			if count > 8 {
				count = 8
			}
			picked, e := nianSample(pool, count)
			if e != nil {
				return e
			}
			w.Remaining = picked
		}
	}
	return nil
}
func openNianGrid(p *Progress, grid int, now time.Time) (map[string]any, error) {
	w := ensureNianFootprint(p)
	if grid < 0 || grid >= 9 || w.Site < 1 || w.Real < 1 || w.Times >= math.MaxInt32 {
		return nil, errors.New("北风足迹格子或计数无效")
	}
	if _, found := w.Opened[grid]; found {
		return nil, errors.New("北风足迹格子已经开启")
	}
	r := activityData("north_wind_footprint", 1)
	mid := r.integer("activity_ticket")
	m := p.Materials[mid]
	if mid <= 0 || m.Count < 1 {
		return nil, runeReject("RET_MONSTER_NIAN_ACTIVITY_TICKET_NOT_ENOUGH", "北风足迹票券不足")
	}
	if len(w.Remaining) == 0 {
		used := map[int]bool{}
		for _, i := range w.Opened {
			used[i] = true
		}
		for i := 0; i < 9; i++ {
			if !used[i] {
				w.Remaining = append(w.Remaining, i)
			}
		}
	}
	if len(w.Remaining) == 0 {
		return nil, errors.New("北风足迹没有剩余奖励")
	}
	index, e := activityAmount([]int64{0, int64(len(w.Remaining) - 1)})
	if e != nil {
		return nil, e
	}
	selected := w.Remaining[index]
	ids := activityData("north_wind_footprint_bonus", w.Real).ids("bonus_ids")
	if selected < 0 || selected >= len(ids) || len(ids) != 9 {
		return nil, errors.New("北风足迹原生奖励配置无效")
	}
	box, e := grantActivityBonus(p, []int{ids[selected]}, now)
	if e != nil {
		return nil, e
	}
	w.Remaining = append(w.Remaining[:index], w.Remaining[index+1:]...)
	w.Opened[grid] = selected
	w.Times++
	m.Count--
	p.Materials[mid] = m
	if selected == 0 {
		if e = refreshNianFootprint(w); e != nil {
			return nil, e
		}
	}
	return box, nil
}
func (s *Service) nianFootprintRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	var grid int
	if len(args) != 1 || json.Unmarshal(args[0], &grid) != nil {
		return nil, errors.New("北风足迹接口需要一个格子编号")
	}
	box := emptyActivityBox()
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if _, e := activityOpen(*p, 327, p.AvatarLevel, s.Now()); e != nil {
			return e
		}
		var e error
		box, e = openNianGrid(p, grid, s.Now())
		return e
	})
	if err != nil {
		return []Push{push("Avatar", "on_open_grid", activityErrorCode(err), emptyActivityBox())}, nil
	}
	out := append(activityPushes(c, s.Now()), materialManagerPush(c), cardMgrPush(c), runePush(c))
	return append(out, push("Avatar", "on_open_grid", RetSuccess, box)), nil
}
