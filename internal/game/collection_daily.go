package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"
)

type collectionDailyRule struct {
	Materials map[int]int `json:"materials"`
	Rules     []struct {
		Dungeon  int               `json:"dungeon"`
		Priority int               `json:"priority"`
		Formula  []json.RawMessage `json:"formula"`
	} `json:"rules"`
}

func collectionDay(now time.Time) string {
	return now.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
}

// Android 855ADC1E 对已通关规则按 -priority 排序，再取首项。
// 表中首列是副本编号，不能把它当材料编号或随机权重。
func collectionDailyAmounts(p Progress, roomID int) (map[int]int64, error) {
	rule, exists := androidCollection.DailyRewards[androidCollection.Rooms[roomID].Daily]
	if !exists {
		return nil, nil
	}
	selected := -1
	for i, row := range rule.Rules {
		if !containsInt(p.ClearedDungeons, row.Dungeon) {
			continue
		}
		if selected < 0 || row.Priority > rule.Rules[selected].Priority {
			selected = i
		}
	}
	if selected < 0 {
		return nil, nil
	}
	formula := rule.Rules[selected].Formula
	if len(formula) < 2 {
		return nil, errors.New("每日收藏室奖励公式缺失")
	}
	var kind string
	if json.Unmarshal(formula[0], &kind) != nil {
		return nil, errors.New("每日收藏室奖励公式类型无效")
	}
	values := []float64{}
	for _, raw := range formula[1:] {
		var value float64
		if json.Unmarshal(raw, &value) != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("每日收藏室奖励公式数值无效")
		}
		values = append(values, value)
	}
	comfort := float64(collectionComfort(p, roomID))
	amount := float64(0)
	switch kind {
	case "constant":
		if len(values) != 1 {
			return nil, errors.New("每日固定奖励公式参数无效")
		}
		amount = values[0]
	case "comfort":
		if len(values) != 1 {
			return nil, errors.New("每日舒适度奖励公式参数无效")
		}
		amount = comfort * values[0]
	case "comfort_offset":
		if len(values) != 3 || values[0] <= 0 {
			return nil, errors.New("每日舒适度增量奖励公式参数无效")
		}
		amount = (comfort/values[0] + values[1]) * values[2]
	default:
		return nil, errors.New("每日收藏室奖励公式未取证")
	}
	amount = math.Max(1, math.Floor(amount))
	if amount > math.MaxInt32 {
		return nil, errors.New("每日收藏室奖励超出原生整数范围")
	}
	materials := map[int]int64{}
	for rewardType, id := range rule.Materials {
		if rewardType != 1 {
			return nil, errors.New("每日收藏室奖励类型尚未取证")
		}
		if id <= 0 {
			return nil, errors.New("每日收藏室奖励目标无效")
		}
		materials[id] = int64(amount)
	}
	return materials, nil
}

func collectionDailyAvailability(p Progress, now time.Time) map[int]bool {
	available := map[int]bool{}
	if p.Collection == nil {
		return available
	}
	if _, unlocked := p.UnlockSystems["house_daily_reward"]; !unlocked {
		return available
	}
	day := collectionDay(now)
	for roomID := range p.Collection.Rooms {
		amounts, e := collectionDailyAmounts(p, roomID)
		if e == nil && len(amounts) > 0 {
			available[roomID] = p.Collection.DailyClaims[roomID] != day
		}
	}
	return available
}

func (s *Service) collectionDailyRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 0 {
		return nil, errors.New("每日收藏室领奖不接受参数")
	}
	e := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureCollection(p, s.Now()); e != nil {
			return e
		}
		if _, unlocked := p.UnlockSystems["house_daily_reward"]; !unlocked {
			return runeReject("RET_SYSTEM_LOCKED", "每日收藏室奖励尚未开放")
		}
		day := collectionDay(s.Now())
		rooms := []int{}
		for id := range p.Collection.Rooms {
			rooms = append(rooms, id)
		}
		sort.Ints(rooms)
		claims := 0
		for _, roomID := range rooms {
			last := p.Collection.DailyClaims[roomID]
			if last > day {
				return errors.New("每日收藏室领奖存档时间回拨")
			}
			if last == day {
				continue
			}
			amounts, e := collectionDailyAmounts(*p, roomID)
			if e != nil {
				return e
			}
			if len(amounts) == 0 {
				continue
			}
			changes := map[int]int64{}
			cards := []string{}
			for id, count := range amounts {
				if e := grantNativeItem(p, id, count, p.AvatarLevel, s.Now(), changes, &cards, 0); e != nil {
					return e
				}
			}
			p.Collection.DailyClaims[roomID] = day
			claims++
		}
		if claims == 0 {
			return runeReject("RET_HOUSE_DAILY_REWARD_GOTTON", "当前没有可领取的收藏室每日奖励")
		}
		return nil
	})
	if e != nil {
		var reject *runeBusinessError
		if !errors.As(e, &reject) {
			return nil, e
		}
		code, known := androidCollection.Errors[reject.Name]
		if !known {
			return nil, errors.New("每日收藏室领奖错误码缺失")
		}
		return []Push{push("Avatar", "on_get_house_reward", []int{code})}, nil
	}
	p := c.SelectedAvatarUnsafe().Progress
	out := []Push{materialManagerPush(c), knowledgePush(c)}
	out = append(out, collectionPushes(p, s.Now(), false)...)
	return append(out, push("Avatar", "on_get_house_reward", []int{RetSuccess})), nil
}
