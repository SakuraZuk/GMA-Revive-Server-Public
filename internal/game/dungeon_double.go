package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
)

// C13BDDDA.set_dungeon_power_type的RPC回调是单个IntRet，不是布尔值。
// D81B4FE0.get_need_power/check_material_double确认普通双倍扣两倍体力及等量材料29。
// 探索和活动沿用各自独立倍率；本接口不覆盖其尚未确认的叠加规则。
func (s *Service) setDungeonBonusDouble(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	cb, valid := callbackArg(args)
	if c.phase != Playing || !valid || len(args) != 3 {
		return nil, errors.New("副本双倍选择需要登录角色、回调、副本和开关")
	}
	var id int
	var enabled bool
	if json.Unmarshal(args[1], &id) != nil || !nativeBinarySwitch(args[2], &enabled) {
		return []Push{Callback(cb, []any{1019})}, nil
	}
	_, known := dungeonCatalog[id]
	if !known {
		return []Push{Callback(cb, []any{1019})}, nil
	}
	ret := 0
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if enabled && !ordinaryDoubleSupported(id) {
			ret = 1029 // RET_DUNGEON_NOT_SUPPOR_DOUBLE
			return nil
		}
		if enabled && activityData("dungeons", id).integer("double_unlock_condition") != 0 && !containsInt(p.ClearedDungeons, id) {
			ret = 1030 // RET_DUNGEON_DOUBLE_NOT_PASS
			return nil
		}
		if p.DungeonBonusDouble == nil {
			p.DungeonBonusDouble = map[int]bool{}
		}
		p.DungeonBonusDouble[id] = enabled
		return nil
	})
	if err != nil {
		return []Push{Callback(cb, []any{13})}, nil // RET_UNKNOWN_ERROR
	}
	if ret != 0 {
		return []Push{Callback(cb, []any{ret})}, nil
	}
	return []Push{dungeonEntryPush(c.SelectedAvatarUnsafe().Progress, id), Callback(cb, []any{RetSuccess})}, nil
}

func prepareOrdinaryDoubleCosts(p *Progress, id, power int, paid map[int]int64) (int, error) {
	if !p.DungeonBonusDouble[id] {
		return power, nil
	}
	if !ordinaryDoubleSupported(id) || power <= 0 || power > math.MaxInt32/2 || paid == nil {
		return 0, errors.New("副本双倍扣费状态无效")
	}
	if activityData("dungeons", id).integer("double_unlock_condition") != 0 && !containsInt(p.ClearedDungeons, id) {
		return 0, errors.New("副本双倍尚未首通解锁")
	}
	power *= 2
	m := p.Materials[29]
	if m.Count < int64(power) {
		return 0, errors.New("副本双倍券不足")
	}
	m.Count -= int64(power)
	p.Materials[29] = m
	paid[29] += int64(power)
	return power, nil
}

func ordinaryDoubleSupported(id int) bool {
	d := dungeonCatalog[id]
	if d.Type == 9 {
		return true
	}
	if d.Type != 8 {
		return false
	}
	bonuses := activityData("dungeons", id).ids("everyday_bonus")
	if len(bonuses) != 7 {
		return false
	}
	for _, bonus := range bonuses {
		if !ordinaryDailyRewardSupported(bonus) {
			return false
		}
	}
	return true
}
