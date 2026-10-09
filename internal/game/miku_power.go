package game

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
)

// B0D07550.get_miku_power_addition：所属地图已激活手册battle_power_effect求和，倍率max(0,1-sum)。
// C13BDDDA.fix_dungeon_need_power：int(ceil(max(0,need_power*factor)))。
func activityPowerCost(p Progress, dungeonID, base int) (int, error) {
	if base < 0 {
		return 0, errors.New("副本体力配置无效")
	}
	if activityData("dungeons", dungeonID).integer("dungeon_type") != 208 || p.Activities.Miku == nil {
		return base, nil
	}
	var mapping []int
	if json.Unmarshal(androidActivities["miku_dungeon_map"][strconv.Itoa(dungeonID)], &mapping) != nil || len(mapping) == 0 {
		return base, nil
	}
	m := p.Activities.Miku.Maps[mapping[0]]
	if m == nil {
		return base, nil
	}
	addition := float64(0)
	for _, group := range m.Handbook.Items {
		for _, item := range group {
			if item.State != 2 {
				continue
			}
			r := activityData("handbook_item", item.ID)
			raw := r["battle_power_effect"]
			if len(raw) == 0 || string(raw) == "null" {
				continue
			}
			var v float64
			if json.Unmarshal(raw, &v) != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				return 0, errors.New("初音手册体力减免配置无效")
			}
			addition += v
		}
	}
	factor := math.Max(0, 1-addition)
	result := math.Ceil(math.Max(0, float64(base)*factor))
	if math.IsNaN(result) || math.IsInf(result, 0) || result > math.MaxInt32 {
		return 0, errors.New("初音体力费用溢出")
	}
	return int(result), nil
}
