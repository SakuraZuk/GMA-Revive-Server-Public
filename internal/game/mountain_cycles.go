package game

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

// 已批准本服常驻政策：同一挑战每7天循环，原始七日窗口与22点截止保留。
// 每个赛季保存成绩/进度/领奖位，圈充能和已获得资产不重置。
func mountainSeasonAt(pid int, now time.Time) (int, int64, int64) {
	cutoff := mountainRankCutoff(pid)
	if cutoff <= 0 {
		return -1, 0, 0
	}
	end := time.Unix(cutoff, 0).In(shanghaiZone)
	start := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, shanghaiZone).AddDate(0, 0, -6)
	if now.Before(start) {
		return -1, start.Unix(), cutoff
	}
	season := int(now.Sub(start) / (7 * 24 * time.Hour))
	return season, start.AddDate(0, 0, season*7).Unix(), end.AddDate(0, 0, season*7).Unix()
}

func ensureMountainSeason(w *MountainState, pid int, season int) ActivityProgress {
	// 旧JSON允许mountain_dungeon:null；迁移须先构造可写原进度映射。
	if w.Dungeons == nil {
		w.Dungeons = map[int]ActivityProgress{}
	}
	if w.Seasons == nil {
		w.Seasons = map[int]map[int]ActivityProgress{}
	}
	if w.ActiveSeasons == nil {
		w.ActiveSeasons = map[int]int{}
	}
	if w.Seasons[pid] == nil {
		w.Seasons[pid] = map[int]ActivityProgress{0: w.Dungeons[pid]}
	}
	active := w.ActiveSeasons[pid]
	w.Seasons[pid][active] = w.Dungeons[pid]
	d := w.Seasons[pid][season]
	w.ActiveSeasons[pid] = season
	w.Dungeons[pid] = d
	return d
}

func (s *Service) refreshMountainCycles(v map[string]*Avatar, now time.Time) error {
	if !remainingPolicyEnabled() {
		return nil
	}
	for pid := 1; pid <= 7; pid++ {
		current, _, _ := mountainSeasonAt(pid, now)
		if current < 0 {
			continue
		}
		for _, av := range v {
			if av.Progress.Activities.Mountain != nil {
				ensureMountainSeason(av.Progress.Activities.Mountain, pid, current)
			}
		}
		seasons := map[int]bool{}
		for _, av := range v {
			if w := av.Progress.Activities.Mountain; w != nil {
				for season := range w.Seasons[pid] {
					if season > 0 {
						seasons[season] = true
					}
				}
			}
		}
		ordered := []int{}
		for season := range seasons {
			ordered = append(ordered, season)
		}
		sort.Ints(ordered)
		for _, season := range ordered {
			cutoff := mountainRankCutoff(pid) + int64(season)*7*24*3600
			if now.Unix() < cutoff {
				continue
			}
			groups := map[int][]ActivityRankEntry{}
			for _, av := range v {
				w := av.Progress.Activities.Mountain
				if w == nil {
					continue
				}
				d := w.Seasons[pid][season]
				if d.Ranked && d.RankHard > 0 && d.Actions >= 0 && d.RankAt > 0 && d.RankAt < cutoff {
					groups[av.Hostnum] = append(groups[av.Hostnum], ActivityRankEntry{Avatar: *av, Score: []int64{int64(d.RankHard), -int64(d.Actions)}, Cards: d.Cards})
				}
			}
			for _, rows := range groups {
				sort.Slice(rows, func(i, j int) bool {
					if rows[i].Score[0] != rows[j].Score[0] {
						return rows[i].Score[0] > rows[j].Score[0]
					}
					if rows[i].Score[1] != rows[j].Score[1] {
						return rows[i].Score[1] > rows[j].Score[1]
					}
					return hexOf(rows[i].Avatar.OID) < hexOf(rows[j].Avatar.OID)
				})
				for i, entry := range rows {
					p := &v[hexOf(entry.Avatar.OID)].Progress
					cal := p.Activities.Calendar
					if cal == nil {
						return errors.New("山海赛季领奖账本丢失")
					}
					if cal.MountainSeasons == nil {
						cal.MountainSeasons = map[string]ActivityRankReceipt{}
					}
					key := fmt.Sprintf("%d/%d", pid, season)
					if receipt, ok := cal.MountainSeasons[key]; ok && receipt.Closed {
						continue
					}
					bonus, err := activityRankBonus("mountain_game_rank", i+1)
					if err != nil {
						return err
					}
					receipt := ActivityRankReceipt{Cutoff: cutoff, ObservedAt: now.Unix(), Closed: true, Rank: i + 1, Score: entry.Score, Bonus: bonus}
					if bonus > 0 {
						if err = s.issueActivityRankMail(p, bonus, fmt.Sprintf("mountain-season:%s:%d:%d", hexOf(entry.Avatar.OID), pid, season), now); err != nil {
							return err
						}
						receipt.Issued = true
					}
					cal.MountainSeasons[key] = receipt
				}
			}
		}
	}
	return nil
}

func applyFrozenMountainMaterials(p *Progress, bc *ActivityBattleContext, box map[string]any, now time.Time) error {
	if len(bc.MaterialRates) == 0 {
		return nil
	}
	materials, ok := box["materials"].(map[int]int64)
	if !ok {
		return errors.New("山海材料奖励盒无效")
	}
	for id, rate := range bc.MaterialRates {
		base := materials[id]
		if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) || base < 0 {
			return errors.New("山海材料倍率或数量无效")
		}
		value := math.Floor(float64(base) * rate)
		if value > math.MaxInt32 {
			return errors.New("山海材料加成溢出")
		}
		added := int64(value)
		if added == 0 {
			continue
		}
		changes := map[int]int64{}
		cards := []string{}
		if err := grantNativeItem(p, id, added, p.AvatarLevel, now, changes, &cards, 0); err != nil {
			return err
		}
		materials[id] += added
	}
	return nil
}
