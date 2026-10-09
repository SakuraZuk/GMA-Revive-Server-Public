package game

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strconv"
	"time"
)

func consignLimits(p Progress) (int, int, error) {
	if p.Collection == nil {
		return 0, 0, errors.New("委托收藏室未初始化")
	}
	row := remainingData("facility_consign", p.Collection.Facilities[5].Level)
	if len(row) == 0 {
		return 0, 0, errors.New("委托设施等级无效")
	}
	var effects [][]json.RawMessage
	if json.Unmarshal(row["upgrade_effect"], &effects) != nil {
		return 0, 0, errors.New("委托设施效果无效")
	}
	base := remainingData("house_base", 1)
	teams, tasks := base.integer("consign_team_num"), base.integer("consign_task_num")
	for _, pair := range effects {
		if len(pair) != 2 {
			return 0, 0, errors.New("委托效果格式错误")
		}
		var kind string
		var value int
		if json.Unmarshal(pair[0], &kind) != nil || json.Unmarshal(pair[1], &value) != nil {
			return 0, 0, errors.New("委托效果数值错误")
		}
		if kind == "3" {
			teams += value
		}
		if kind == "13" {
			tasks += value
		}
	}
	return teams, tasks, nil
}

func consignPick(p *Progress, poolID int) (int, error) {
	stars := remainingData("consign_level", p.AvatarLevel)
	cumulative, values := stars.ids("star_rates"), stars.ids("task_stars")
	weights := make([]int, len(cumulative))
	prev := 0
	for i, n := range cumulative {
		weights[i] = n - prev
		prev = n
	}
	var pool map[int]struct {
		Tasks []struct {
			ID     int `json:"task_id"`
			Weight int `json:"task_rate"`
		} `json:"tasks"`
	}
	if json.Unmarshal(androidRemainingGameplay["consign_task_pool"][strconv.Itoa(poolID)], &pool) != nil {
		return 0, errors.New("委托任务库结构无效")
	}
	// 同一模板不能重复占槽；已耗尽某星级时仅在仍可用的原星级权重中抽取。
	for i, value := range values {
		available := false
		for _, entry := range pool[value].Tasks {
			available = available || p.RemainingGameplay.Consign.Tasks[entry.ID] == nil
		}
		if !available {
			weights[i] = 0
		}
	}
	star, err := freeStageChoose(values, weights)
	if err != nil {
		return 0, err
	}
	ids, weights := []int{}, []int{}
	for _, entry := range pool[star].Tasks {
		if p.RemainingGameplay.Consign.Tasks[entry.ID] == nil {
			ids = append(ids, entry.ID)
			weights = append(weights, entry.Weight)
		}
	}
	return freeStageChoose(ids, weights)
}

func consignPoolAt(p Progress, index int, now time.Time) (int, error) {
	local := now.In(time.FixedZone("北京时间", 28800))
	base := remainingData("consign_base", 1)
	if local.Hour() >= base.integer("special_task_refresh_time") || local.Hour() < base.integer("task_refresh_time") {
		pools := base.ids("task_pool_ids")
		if len(pools) != 1 {
			return 0, errors.New("委托夜池配置无效")
		}
		return pools[0], nil
	}
	teams, _, err := consignLimits(p)
	if err != nil {
		return 0, err
	}
	// 首次日间生成用原表第一轮槽位池；重置单槽及以后日界用下一轮标准池。
	round := 1
	if p.RemainingGameplay.Consign.GenerationCount > 0 {
		round = 2
	}
	var pools []int
	if json.Unmarshal(androidRemainingGameplay["consign_refresh_pool"][strconv.Itoa(teams*1000+round)], &pools) != nil || len(pools) == 0 {
		return 0, errors.New("委托日池配置缺失")
	}
	if index <= 0 {
		return 0, errors.New("委托任务槽位无效")
	}
	if index > len(pools) {
		return 3, nil
	}
	return pools[index-1], nil
}

func consignRecoverRate(p Progress) int64 {
	base := remainingData("consign_base", 1)
	rate := int64(base.integer("default_recover_rate"))
	if p.Collection != nil {
		room := p.Collection.Rooms[androidCollection.Facilities[5].Room]
		values := base.ids("recover_rate")
		n := len(room.Slots)
		if n > len(values) {
			n = len(values)
		}
		if n > 0 {
			rate = int64(values[n-1])
		}
	}
	return rate
}

// 未找到原服起算方法；补任务和5点日界由显式本服政策恢复，默认不开启。
func refreshConsignState(p *Progress, now time.Time) error {
	w := p.RemainingGameplay.Consign
	if w.Tasks == nil {
		w.Tasks = map[int]*ConsignTaskState{}
	}
	if w.PhaseRecv == nil {
		w.PhaseRecv = map[int]int64{}
	}
	if w.PhaseTotal == nil {
		w.PhaseTotal = map[int]map[int]int64{}
	}
	if now.Unix() < w.PointAt {
		return errors.New("委托时间回拨")
	}
	if !remainingPolicyEnabled() {
		return nil
	}
	if p.Collection == nil {
		if err := ensureCollection(p, now); err != nil {
			return err
		}
	}
	_, maxTasks, err := consignLimits(*p)
	if err != nil {
		return err
	}
	local := now.In(time.FixedZone("北京时间", 28800))
	day := local.Add(-5 * time.Hour).Format("2006-01-02")
	if w.Day != "" && day < w.Day {
		return errors.New("委托日期回拨")
	}
	if w.PolicyVersion != "local-20261008-v1" {
		w.PolicyVersion = "local-20261008-v1"
		w.RefreshPoint = 5000
		w.PointAt = now.Unix()
	}
	if day > w.Day {
		w.Day = day
		w.FinishTimes = 0
		w.RefreshTimes = 0
		for id, task := range w.Tasks {
			if task.StartTime == 0 {
				delete(w.Tasks, id)
			}
		}
		next := time.Date(local.Year(), local.Month(), local.Day(), 5, 0, 0, 0, local.Location())
		if !next.After(local) {
			next = next.Add(24 * time.Hour)
		}
		w.NextRefresh = next.Unix()
		for len(w.Tasks) < maxTasks {
			index := len(w.Tasks) + 1
			pool, err := consignPoolAt(*p, index, now)
			if err != nil {
				return err
			}
			id, err := consignPick(p, pool)
			if err != nil {
				return err
			}
			w.Tasks[id] = &ConsignTaskState{Index: index, Level: p.AvatarLevel, Cards: []int{}}
		}
		w.GenerationCount++
	}
	seconds := now.Unix() - w.PointAt
	nextRate := consignRecoverRate(*p)
	rate := w.RecoveryRate
	if rate == 0 {
		rate = nextRate
	}
	if seconds > 0 {
		if rate <= 0 || seconds > math.MaxInt64/rate {
			return errors.New("委托刷新恢复溢出")
		}
		if w.PointRemainder < 0 || w.PointRemainder >= 3600 || seconds*rate > math.MaxInt64-w.PointRemainder {
			return errors.New("委托刷新余数无效")
		}
		credit := seconds*rate + w.PointRemainder
		recovered := credit / 3600
		w.PointRemainder = credit % 3600
		w.PointAt = now.Unix()
		if w.RefreshPoint == 5000 {
			w.PointRemainder = 0
		}
		if recovered > 0 {
			if recovered > 5000-w.RefreshPoint {
				w.RefreshPoint = 5000
				w.PointAt = now.Unix()
				w.PointRemainder = 0
			} else {
				w.RefreshPoint += recovered
			}
		}
	}
	w.RecoveryRate = nextRate
	return nil
}

func consignCardTags(p Progress, cid int) ([]int, error) {
	rules, err := loadIntimacyCatalog()
	if err != nil {
		return nil, err
	}
	level := intimacyLevel(p.Intimacy[cid], rules)
	tags := []int{}
	for _, row := range androidCollection.Cards[cid].Tags {
		if len(row) == 2 && level >= row[1] {
			tags = append(tags, row[0])
		}
	}
	return tags, nil
}
func consignDuration(p Progress, cards []int, task activityRow) (int64, error) {
	var hours float64
	if json.Unmarshal(task["task_time"], &hours) != nil || hours <= 0 {
		return 0, errors.New("委托耗时配置错误")
	}
	profits, err := collectionFacilityTagProfit(p, 5)
	if err != nil {
		return 0, err
	}
	rate := profits["consign_time_reduce"][0]
	if _, unlocked := p.UnlockSystems["house_character_tag"]; unlocked {
		for _, cid := range cards {
			tags, err := consignCardTags(p, cid)
			if err != nil {
				return 0, err
			}
			rows := androidCollection.Cards[cid].Tags
			if len(rows) < 3 || !containsInt(tags, rows[2][0]) {
				continue
			}
			for _, tid := range []int{rows[2][0]} {
				if androidCollection.Tags[tid].Affect == 0 {
					continue
				}
				for _, eid := range androidCollection.Tags[tid].Effects {
					effect := androidCollection.TagEffects[eid]
					if effect.Effect == "consign_time_reduce" && len(effect.Data) == 2 && effect.Data[0] == 1 && effect.Data[1] > rate {
						rate = effect.Data[1]
					}
				}
			}
		}
	}
	if rate < 0 || rate >= 1 {
		return 0, errors.New("委托时间减免无效")
	}
	base, ok := new(big.Rat).SetString(strconv.FormatFloat(hours, 'f', -1, 64))
	if !ok {
		return 0, errors.New("委托时间无法精确计算")
	}
	reduction, ok := new(big.Rat).SetString(strconv.FormatFloat(rate, 'f', -1, 64))
	if !ok {
		return 0, errors.New("委托减免无法精确计算")
	}
	base.Mul(base, big.NewRat(3600, 1))
	base.Mul(base, new(big.Rat).Sub(big.NewRat(1, 1), reduction))
	if !base.IsInt() || !base.Num().IsInt64() || base.Num().Int64() <= 0 {
		return 0, errors.New("委托耗时出现未确认的秒数舍入")
	}
	return base.Num().Int64(), nil
}
