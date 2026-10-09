package game

import (
	"sort"
	"time"
)

// 已批准本服规则：三分榜名次和升序；缺席为该分榜人数+1，
// 同和已参榜数量多优先，再以OID稳定排序。至少参与一榜才进总榜。
// 每小时保存该小时之前的分榜成绩；同一小时的改分不回写旧整点榜。
type NianScoreSnapshot struct {
	At     int64 `json:"at"`
	Damage int64 `json:"damage"`
	Cards  []any `json:"cards"`
}
type NianTotalSnapshot struct {
	Hour     int64  `json:"hour"`
	Rank     int    `json:"rank"`
	Score    int64  `json:"score"`
	Joined   int    `json:"joined"`
	SubRanks [3]int `json:"sub_ranks"`
	Cards    []any  `json:"cards"`
}

func recordNianScoreHour(p *Progress, did int, d ActivityProgress) {
	if !d.Ranked || d.RankAt <= 0 || d.RankDamage < 0 {
		return
	}
	if p.Activities.NianHistory == nil {
		p.Activities.NianHistory = map[int]map[int64]NianScoreSnapshot{}
	}
	if p.Activities.NianHistory[did] == nil {
		p.Activities.NianHistory[did] = map[int64]NianScoreSnapshot{}
	}
	hour := d.RankAt / 3600
	old, exists := p.Activities.NianHistory[did][hour]
	if !exists || d.RankDamage > old.Damage {
		p.Activities.NianHistory[did][hour] = NianScoreSnapshot{At: d.RankAt, Damage: d.RankDamage, Cards: append([]any{}, d.RankCards...)}
	}
}

func nianScoreBefore(p Progress, did int, exclusive, after int64) (NianScoreSnapshot, bool) {
	best := NianScoreSnapshot{}
	found := false
	for _, record := range p.Activities.NianHistory[did] {
		if record.At > after && record.At < exclusive && record.Damage >= 0 && (!found || record.Damage > best.Damage) {
			best = record
			found = true
		}
	}
	d := p.Activities.Nian[did]
	if d.Ranked && d.RankAt > after && d.RankAt < exclusive && d.RankDamage >= 0 && (!found || d.RankDamage > best.Damage) {
		best = NianScoreSnapshot{At: d.RankAt, Damage: d.RankDamage, Cards: d.RankCards}
		found = true
	}
	return best, found
}

func compositeNianRanks(v map[string]*Avatar, exclusive, after int64) map[string]NianTotalSnapshot {
	hosts := map[int][]*Avatar{}
	for _, av := range v {
		hosts[av.Hostnum] = append(hosts[av.Hostnum], av)
	}
	out := map[string]NianTotalSnapshot{}
	for _, avs := range hosts {
		ranks := [3]map[string]int{}
		counts := [3]int{}
		cards := map[string][]any{}
		for i, did := range []int{20200001, 20200002, 20200003} {
			type pair struct {
				av    *Avatar
				score NianScoreSnapshot
			}
			rows := []pair{}
			for _, av := range avs {
				if score, ok := nianScoreBefore(av.Progress, did, exclusive, after); ok {
					rows = append(rows, pair{av, score})
				}
			}
			sort.Slice(rows, func(a, b int) bool {
				if rows[a].score.Damage != rows[b].score.Damage {
					return rows[a].score.Damage > rows[b].score.Damage
				}
				return hexOf(rows[a].av.OID) < hexOf(rows[b].av.OID)
			})
			ranks[i] = map[string]int{}
			counts[i] = len(rows)
			for j, row := range rows {
				id := hexOf(row.av.OID)
				ranks[i][id] = j + 1
				if cards[id] == nil {
					cards[id] = append([]any{}, row.score.Cards...)
				}
			}
		}
		type total struct {
			id    string
			value NianTotalSnapshot
		}
		rows := []total{}
		for _, av := range avs {
			id := hexOf(av.OID)
			value := NianTotalSnapshot{Hour: exclusive - 1, Cards: cards[id]}
			for i := range ranks {
				rank := ranks[i][id]
				if rank == 0 {
					rank = counts[i] + 1
				} else {
					value.Joined++
				}
				value.SubRanks[i] = rank
				value.Score += int64(rank)
			}
			if value.Joined > 0 {
				rows = append(rows, total{id, value})
			}
		}
		sort.Slice(rows, func(a, b int) bool {
			if rows[a].value.Score != rows[b].value.Score {
				return rows[a].value.Score < rows[b].value.Score
			}
			if rows[a].value.Joined != rows[b].value.Joined {
				return rows[a].value.Joined > rows[b].value.Joined
			}
			return rows[a].id < rows[b].id
		})
		for i, row := range rows {
			row.value.Rank = i + 1
			out[row.id] = row.value
		}
	}
	return out
}

func freezeNianHourRanking(v map[string]*Avatar, now time.Time) {
	hour := now.Unix() / 3600 * 3600
	_, week := activityCutoffs(time.Unix(hour, 0))
	entries := compositeNianRanks(v, hour, week.Unix())
	for id, av := range v {
		cal := av.Progress.Activities.Calendar
		if cal == nil {
			cal = &ActivityCalendar{}
			av.Progress.Activities.Calendar = cal
		}
		if cal.NianTotal != nil && cal.NianTotal.Hour >= hour {
			continue
		}
		entry := entries[id]
		entry.Hour = hour
		cal.NianTotal = &entry
	}
}

func clearNianHistoryThrough(p *Progress, cutoff int64) {
	for did, hours := range p.Activities.NianHistory {
		for hour, record := range hours {
			if record.At <= cutoff {
				delete(hours, hour)
			}
		}
		if len(hours) == 0 {
			delete(p.Activities.NianHistory, did)
		}
	}
}

func FrozenNianTotalRank(av Avatar) (ActivityRankEntry, bool) {
	if av.Progress.Activities.Calendar == nil || av.Progress.Activities.Calendar.NianTotal == nil {
		return ActivityRankEntry{}, false
	}
	r := av.Progress.Activities.Calendar.NianTotal
	if r.Rank <= 0 || r.Joined <= 0 || r.Score <= 0 {
		return ActivityRankEntry{}, false
	}
	return ActivityRankEntry{Avatar: av, Rank: r.Rank, Score: []int64{-r.Score, int64(r.Joined)}, Cards: r.Cards, SubRanks: r.SubRanks}, true
}
