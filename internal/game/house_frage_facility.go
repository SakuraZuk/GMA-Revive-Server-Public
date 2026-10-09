package game

import (
	"errors"
	"math"
)

// 原生514DD3DE.get_ssr_add_prob与facility_base6的lambda直接取实际设施房间
// 入住人数表；这里不乘生产设施的4倍，也不使用全收藏室人数或累计SSR次数。
func houseFrageFacilitySSRBonus(p *Progress) (float64, error) {
	if p.Collection == nil || p.Collection.Facilities[6].Level <= 0 {
		return 0, nil
	}
	roomID := androidCollection.Facilities[6].Room
	row, ok := androidCollection.Rooms[roomID]
	if roomID <= 0 || !ok {
		return 0, errors.New("骰子桌实际房间原生配置缺失")
	}
	n := len(p.Collection.Rooms[roomID].Slots)
	if n == 0 || n > len(row.CardProfit) {
		return 0, nil
	}
	value := row.CardProfit[n-1]
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, errors.New("骰子桌入住典藏残页加成无效")
	}
	return value, nil
}
