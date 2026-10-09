package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"
)

var shanghaiZone = time.FixedZone("北京时间", 28800)

type MountainCircle struct {
	ID       int    `json:"circle_id"`
	Progress int    `json:"current_process_value"`
	Old      int    `json:"old_process_value" wire:"-"`
	Unlocked []bool `json:"guard_site_lock_status"`
	Cards    []int  `json:"guard_site_cards"`
}
type MountainState struct {
	Circles       map[int]*MountainCircle          `json:"magic_circles"`
	Dungeons      map[int]ActivityProgress         `json:"mountain_dungeon"`
	Seasons       map[int]map[int]ActivityProgress `json:"server_seasons,omitempty" wire:"-"`
	ActiveSeasons map[int]int                      `json:"server_active_seasons,omitempty" wire:"-"`
}

func nianProperties(rows map[int]ActivityProgress) map[int]any {
	out := map[int]any{}
	for id, d := range rows {
		out[id] = map[string]any{"current_progress": d.Progress, "bonus_progress": d.Bonus, "max_damage": d.MaxDamage, "total_damage": d.TotalDamage, "cards_info": d.Cards}
	}
	return out
}

func ensureMountain(p *Progress) *MountainState {
	if p.Activities.Mountain == nil {
		p.Activities.Mountain = &MountainState{Circles: map[int]*MountainCircle{}, Dungeons: map[int]ActivityProgress{}}
		for key := range androidActivities["mountain_game_guard"] {
			id, _ := strconv.Atoi(key)
			p.Activities.Mountain.Circles[id] = &MountainCircle{ID: id, Unlocked: []bool{false, false, false}, Cards: []int{0, 0, 0}}
		}
	}
	return p.Activities.Mountain
}
func prepareMountainDungeon(p *Progress, bc *ActivityBattleContext, day, level int, now time.Time) error {
	r := activityData("mountain_game_dungeon", bc.DungeonID)
	if len(r) == 0 || r.flag("_dungeon_disable") {
		return errors.New("山海副本不存在")
	}
	base := activityData("mountain_game_base_rule", 1)
	stage := r.integer("stage")
	key := "first_phase_days"
	if stage == 2 {
		key = "second_phase_days"
	}
	if !containsInt(base.ids(key), day) || day < r.integer("unlock_day") {
		return errors.New("山海副本阶段尚未开放")
	}
	w := ensureMountain(p)
	if pid := r.integer("progress_bonus_id"); pid > 0 && remainingPolicyEnabled() {
		season, _, end := mountainSeasonAt(pid, now)
		if season < 0 || now.Unix() >= end {
			return errors.New("山海挑战当前赛季尚未开放或已经截止")
		}
		ensureMountainSeason(w, pid, season)
		bc.MountainSeason = season
	}
	bc.NodeID = r.integer("guard_direction")
	if bc.DungeonID == base.integer("first_phase_end_dungeon") {
		for _, circle := range w.Circles {
			if circle.Progress < base.integer("first_phase_end_dungeon_process") {
				return errors.New("山海法阵充能未达到终章门槛")
			}
		}
	}
	return freezeMountainGuardEffects(p, bc, r)
}
func finishMountainDungeon(p *Progress, bc *ActivityBattleContext, now time.Time) error {
	w := p.Activities.Mountain
	if w == nil {
		return errors.New("山海存档丢失")
	}
	r := activityData("mountain_game_dungeon", bc.DungeonID)
	base := activityData("mountain_game_base_rule", 1)
	if r.integer("stage") == 1 && bc.NodeID >= 1 && bc.NodeID <= 4 && activityData("dungeons", bc.DungeonID).integer("dungeon_battle_id") > 0 {
		circle := w.Circles[bc.NodeID]
		if circle == nil {
			return errors.New("山海法阵不存在")
		}
		day, err := activityOpen(*p, 203, 0, now)
		if err != nil {
			return err
		}
		limit := base.integer("total_process_value")
		var limits [][]int
		_ = json.Unmarshal(base["process_value_limt"], &limits)
		for _, pair := range limits {
			if len(pair) == 2 && pair[0] == day {
				limit = pair[1]
			}
		}
		addition := base.integer("victory_process_value")
		if addition <= 0 || circle.Progress > math.MaxInt32-addition {
			return errors.New("山海法阵充能数无效")
		}
		circle.Old = circle.Progress
		circle.Progress += addition
		if circle.Progress > limit {
			circle.Progress = limit
		}
		var siteIDs []int
		_ = json.Unmarshal(androidActivities["mountain_game_guard"][strconv.Itoa(bc.NodeID)], &siteIDs)
		for i, site := range siteIDs {
			if i >= len(circle.Unlocked) {
				return errors.New("山海守护槽配置无效")
			}
			if circle.Progress >= activityData("mountain_game_guard_site", site).integer("unlock_process_value") {
				circle.Unlocked[i] = true
			}
		}
	}
	pid := r.integer("progress_bonus_id")
	if pid > 0 {
		seasonProgress := false
		competitionOpen := mountainRankCompetitionOpen(pid, now)
		if remainingPolicyEnabled() {
			season, _, end := mountainSeasonAt(pid, now)
			competitionOpen = season == bc.MountainSeason && now.Unix() < end
			// 已冻结会话跨赛季只结其原赛季进度，不能污染下一赛季榜/领奖位。
			if season >= 0 {
				ensureMountainSeason(w, pid, season)
			}
			seasonProgress = w.ActiveSeasons[pid] != bc.MountainSeason
		}
		d := w.Dungeons[pid]
		if seasonProgress {
			d = w.Seasons[pid][bc.MountainSeason]
		}
		previousHard := activityData("mountain_game_dungeon", d.ID).integer("hard_level")
		hard := r.integer("hard_level")
		if bc.Statistics != nil && competitionOpen && (!d.Ranked || hard > previousHard || (hard == previousHard && int(bc.Statistics.AP) < d.Actions)) {
			d.ID = bc.DungeonID
			d.Actions = int(bc.Statistics.AP)
			d.Cards = append([]any{}, bc.RankCards...)
			d.Ranked = true
			d.RankHard = hard
			d.RankAt = now.Unix()
		} else if !d.Ranked && hard > previousHard {
			// 旧桥没有行动统计，只保存实际通关编号，不填造排行榜成绩。
			d.ID = bc.DungeonID
		}
		add := r.integer("progress_value")
		if add < 0 || d.Progress > math.MaxInt32-add {
			return errors.New("山海领奖进度溢出")
		}
		d.Progress += add
		if !seasonProgress {
			w.Dungeons[pid] = d
		}
		if remainingPolicyEnabled() {
			w.Seasons[pid][bc.MountainSeason] = d
		}
	}
	return nil
}
func prepareNianDungeon(p *Progress, bc *ActivityBattleContext, day int) error {
	r := activityData("monster_nian_dungeon", bc.DungeonID)
	if len(r) == 0 || !containsInt(r.ids("open_days"), day) {
		return errors.New("年兽副本今日未开放")
	}
	return refreshNianDaily(p, time.Unix(bc.PreparedAt, 0))
}

// Android system_tips.monster_nian_dungeon：每副本每日前三次进度奖。
// 老档没有日期只建立本日基线，避免登录补发已领的历史奖。
func refreshNianDaily(p *Progress, now time.Time) error {
	day := now.In(shanghaiZone).Format("2006-01-02")
	if p.Activities.NianDay > day {
		return errors.New("年兽挑战日期回拨")
	}
	if p.Activities.Nian == nil {
		p.Activities.Nian = map[int]ActivityProgress{}
	}
	if p.Activities.NianDay != "" && p.Activities.NianDay != day {
		for id, state := range p.Activities.Nian {
			state.Progress, state.Bonus = 0, 0
			p.Activities.Nian[id] = state
		}
	}
	p.Activities.NianDay = day
	return nil
}
func finishNianDungeon(p *Progress, bc *ActivityBattleContext, now time.Time) error {
	if p.Activities.Nian == nil {
		return errors.New("年兽存档丢失")
	}
	if err := refreshNianDaily(p, now); err != nil {
		return err
	}
	r := activityData("monster_nian_dungeon", bc.DungeonID)
	d := p.Activities.Nian[bc.DungeonID]
	add := r.integer("progress_value")
	if add < 0 || d.Progress > math.MaxInt32-add {
		return errors.New("年兽领奖进度溢出")
	}
	d.ID = bc.DungeonID
	d.Progress += add
	thresholds := activityData("north_wind_footprint", 1).ids("reward_progress")
	if len(thresholds) == 0 {
		return errors.New("年兽每日挑战奖励门槛缺失")
	}
	if limit := thresholds[len(thresholds)-1]; d.Progress > limit {
		d.Progress = limit
	}
	p.Activities.Nian[bc.DungeonID] = d
	return nil
}
func claimActivityProgress(p *Progress, nian bool, id, index int, now time.Time) (map[string]any, error) {
	base, bonuses := "mountain_game_base_rule", "mountain_game_bonus"
	var states map[int]ActivityProgress
	if nian {
		if err := refreshNianDaily(p, now); err != nil {
			return nil, err
		}
		base, bonuses = "north_wind_footprint", "monster_nian_bonus"
		states = p.Activities.Nian
	} else {
		if p.Activities.Mountain == nil {
			return nil, errors.New("山海活动尚无奖励进度")
		}
		states = p.Activities.Mountain.Dungeons
	}
	thresholds := activityData(base, 1).ids("reward_progress")
	if index < 0 || index >= len(thresholds) || index > 30 {
		return nil, errors.New("活动进度奖励编号无效")
	}
	d, ok := states[id]
	if !ok || d.Progress < thresholds[index] {
		return nil, errors.New("活动进度尚未达领奖门槛")
	}
	if d.Bonus&(1<<index) != 0 {
		return nil, errors.New("活动进度奖励已经领取")
	}
	pid := id
	if nian {
		pid = activityData("monster_nian_dungeon", id).integer("progress_bonus_id")
	}
	ids := activityData(bonuses, pid).ids("bonus_ids")
	if index >= len(ids) {
		return nil, errors.New("活动进度奖励配置不存在")
	}
	box, err := grantActivityBonus(p, []int{ids[index]}, now)
	if err != nil {
		return nil, err
	}
	d.Bonus |= 1 << index
	states[id] = d
	return box, nil
}
func (s *Service) mountainNianRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if method == "mountainsea_enter_dungeon" {
		return s.enterDungeon(ctx, c, args)
	}
	direct := method == "change_guard_site_cards"
	expected := 3
	if direct {
		expected = 2
	}
	if len(args) != expected {
		return nil, errors.New("山海或年兽参数数量无效")
	}
	var cb, id, index int
	var cards []int
	if direct {
		if json.Unmarshal(args[0], &id) != nil || json.Unmarshal(args[1], &cards) != nil || len(cards) != 3 {
			return nil, errors.New("山海守护卡列表无效")
		}
	} else {
		if json.Unmarshal(args[0], &cb) != nil || cb <= 0 || json.Unmarshal(args[1], &id) != nil || json.Unmarshal(args[2], &index) != nil {
			return nil, errors.New("活动进度奖励参数无效")
		}
	}
	var box map[string]any
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		act := 203
		if method == "receive_monster_nian_progress_bonus" {
			act = 328
		}
		if _, err := activityOpen(*p, act, c.SelectedAvatarUnsafe().Info.Level, s.Now()); err != nil {
			return err
		}
		if direct {
			w := ensureMountain(p)
			circle := w.Circles[id]
			if circle == nil {
				return errors.New("山海法阵不存在")
			}
			var sites []int
			_ = json.Unmarshal(androidActivities["mountain_game_guard"][strconv.Itoa(id)], &sites)
			owned := map[int]bool{}
			for _, card := range p.Cards {
				owned[card.CardID] = true
			}
			seen := map[int]bool{}
			for i, card := range cards {
				if i >= len(sites) || i >= len(circle.Unlocked) {
					return errors.New("山海守护槽不存在")
				}
				if card == 0 {
					continue
				}
				if !circle.Unlocked[i] || !owned[card] || seen[card] || !containsInt(activityData("mountain_game_guard_site", sites[i]).ids("guard_card"), card) {
					return errors.New("山海守护卡资格或槽位无效")
				}
				seen[card] = true
			}
			circle.Cards = append([]int{}, cards...)
			return nil
		}
		var e error
		box, e = claimActivityProgress(p, act == 328, id, index, s.Now())
		return e
	})
	if err != nil {
		if direct {
			return []Push{push("Avatar", "on_change_guard_site_cards", activityErrorCode(err))}, nil
		}
		return []Push{Callback(cb, []any{activityErrorCode(err), emptyActivityBox()})}, nil
	}
	out := append(activityPushes(c, s.Now()), materialManagerPush(c))
	if direct {
		return append(out, push("Avatar", "on_change_guard_site_cards", RetSuccess)), nil
	}
	return append(out, Callback(cb, []any{RetSuccess, box})), nil
}
