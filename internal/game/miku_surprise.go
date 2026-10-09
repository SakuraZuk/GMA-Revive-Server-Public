package game

import (
	"errors"
	"time"
)

// 已批准本服政策：每图第一次完整到终点后10个对应和声，一图永久一次。
// APK仅包含代理/回包，没有惊喜奖励ID；此数值明确不是原厂恢复结论。
func receiveMikuLocalSurprise(p *Progress, mapID int, now time.Time) (map[string]any, error) {
	if !remainingPolicyEnabled() {
		return nil, errors.New("初音惊喜本服规则未启用")
	}
	if p.Activities.Miku == nil || p.Activities.Miku.Maps[mapID] == nil || mapID < 1 || mapID > 4 {
		return nil, errors.New("初音惊喜地图未进入")
	}
	m := p.Activities.Miku.Maps[mapID]
	if m.CompletedRuns <= 0 {
		return nil, errors.New("初音惊喜需要一次完整终点探索")
	}
	if m.SurpriseClaimed {
		if m.SurpriseBox == nil {
			return nil, errors.New("初音惊喜已领账本缺奖励快照")
		}
		return battleSettlementBoxWire(m.SurpriseBox)
	}
	ids := activityData("miku_args", 1).ids("miku_coin_materials")
	if len(ids) != 4 {
		return nil, errors.New("初音和声地图映射无效")
	}
	changes := map[int]int64{}
	cards := []string{}
	if err := grantNativeItem(p, ids[mapID-1], 10, p.AvatarLevel, now, changes, &cards, 0); err != nil {
		return nil, err
	}
	box := emptyActivityBox()
	box["materials"] = changes
	m.SurpriseClaimed = true
	m.SurpriseBox = box
	return box, nil
}
