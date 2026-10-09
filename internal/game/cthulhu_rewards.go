package game

import (
	"errors"
	"math"
	"time"
)

// 克苏鲁属性不是普通库存容量：Android materials(53..57)给出理智100、属性12上限。
// 206的初始化、圈终点和事件奖励均引用这些材料；box只返回实际增加量。
// 在事务内临时从零解释原生奖励，再将实际增量封顶，随机奖励仍由统一原生解释器生成一次。
func grantCthulhuBonus(p *Progress, ids []int, now time.Time) (map[string]any, error) {
	box := emptyActivityBox()
	if p.Materials == nil {
		p.Materials = map[int]Material{}
	}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		old := map[int]Material{}
		existed := map[int]bool{}
		for mid := 53; mid <= 57; mid++ {
			mat, found := p.Materials[mid]
			limit := int64(activityData("materials", mid).integer("limit_count"))
			if limit <= 0 || mat.Count < 0 || mat.Count > limit || mat.Total < mat.Count || mat.Total > math.MaxInt64-limit {
				return nil, errors.New("克苏鲁属性存档或原生上限无效")
			}
			old[mid], existed[mid] = mat, found
			mat.Count = 0
			p.Materials[mid] = mat
		}
		part, err := grantNativeBonus(p, id, 1, p.AvatarLevel, now)
		if err != nil {
			for mid, mat := range old {
				if existed[mid] {
					p.Materials[mid] = mat
				} else {
					delete(p.Materials, mid)
				}
			}
			return nil, err
		}
		changes := part["materials"].(map[int]int64)
		for mid, before := range old {
			mat := p.Materials[mid]
			added := mat.Count
			limit := int64(activityData("materials", mid).integer("limit_count"))
			if added > limit-before.Count {
				added = limit - before.Count
			}
			mat.Count, mat.Total = before.Count+added, before.Total+added
			if added > 0 {
				mat.ID = mid
				changes[mid] = added
			} else {
				delete(changes, mid)
			}
			if existed[mid] || added > 0 {
				p.Materials[mid] = mat
			} else {
				delete(p.Materials, mid)
			}
		}
		if err = mergeActivityBox(box, part); err != nil {
			return nil, err
		}
	}
	return box, nil
}
