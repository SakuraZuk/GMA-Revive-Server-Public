package game

import (
	"encoding/json"
	"time"
)

// 收藏室目标1001/1002/1003/1004由Android成就说明、base_target参数及实际存档共同确定。
// 完成态保存历史；拆家具、离开房间不撤销首次阈值成就。
func reconcileCollectionAchievements(p *Progress, now time.Time) {
	if p.Collection == nil {
		return
	}
	ensureAchievements(p)
	for id, r := range androidAchievements.Rules {
		a := p.Achievements[id]
		if a.Targets == nil {
			a.Targets = map[int]int64{}
		}
		for _, group := range r.Targets {
			for _, tid := range group {
				t := androidAchievements.Targets[tid]
				params := []int{}
				valid := true
				for _, raw := range t.Params {
					var v int
					if json.Unmarshal(raw, &v) != nil || v <= 0 {
						valid = false
						break
					}
					params = append(params, v)
				}
				if !valid {
					continue
				}
				complete := false
				switch t.Type {
				case 1001:
					if len(params) == 1 {
						_, complete = p.Collection.Rooms[params[0]]
					}
				case 1002:
					if len(params) == 2 {
						room, owned := p.Collection.Rooms[params[1]]
						complete = owned && len(room.Cards) >= params[0]
					}
				case 1003:
					if len(params) == 2 {
						facility, owned := p.Collection.Facilities[params[1]]
						complete = owned && facility.Level >= params[0]
					}
				case 1004:
					if len(params) == 2 {
						_, owned := p.Collection.Rooms[params[1]]
						complete = owned && collectionComfort(*p, params[1]) >= params[0]
					}
				}
				if complete && a.Targets[tid] < 1 {
					a.Targets[tid] = 1
				}
			}
		}
		if a.Time == 0 && achievementCount(a, r) >= r.Need {
			a.Time = now.Unix()
		}
		p.Achievements[id] = a
	}
}
