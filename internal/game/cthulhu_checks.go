package game

import (
	"errors"
	"math"
	"time"
)

// 原生配置和回包字段保留；2/12优先及一次重掷账本为已批准本服规则。
// Moves、Cycle、Layer、格位、事件库索引绑定此次停留，失败离开后可未来再检定。
type CthulhuCheckReceipt struct {
	Visit   int                  `json:"visit"`
	Cycle   int                  `json:"cycle"`
	Layer   int                  `json:"layer"`
	Trunk   int                  `json:"trunk"`
	Branch  int                  `json:"branch"`
	Dice    [2]int               `json:"dice"`
	Success bool                 `json:"success"`
	Big     bool                 `json:"big"`
	AllIn   bool                 `json:"all_in"`
	Box     map[string]any       `json:"box"`
	First   *CthulhuCheckReceipt `json:"first_attempt,omitempty"`
}

func (r *CthulhuCheckReceipt) sameVisit(m *CthulhuMap) bool {
	return r != nil && r.Visit == m.Moves && r.Cycle == m.Cycle && r.Layer == m.Layer && r.Trunk == m.Trunk && r.Branch == m.Branch
}
func (r *CthulhuCheckReceipt) result() map[string]any {
	code := 2
	if r.Success {
		code = 1
	}
	return map[string]any{"dice1": r.Dice[0], "dice2": r.Dice[1], "check_res": code, "big_enable": r.Big}
}

func performCthulhuCheck(p *Progress, mapID, itemID int, allIn bool, now time.Time, fixedDice *[2]int) (*CthulhuCheckReceipt, error) {
	if !remainingPolicyEnabled() {
		return nil, errors.New("克苏鲁检定本服规则未启用")
	}
	w := p.Activities.Cthulhu
	if w == nil || w.Maps[mapID] == nil {
		return nil, errors.New("克苏鲁检定地图未进入")
	}
	m := w.Maps[mapID]
	m.restore()
	g := m.grid()
	if g == nil || g.Item == nil || g.Item.ID != itemID || activityData("cthulhu_items", itemID).integer("item_type") != 3 {
		return nil, errors.New("克苏鲁检定事件不属于当前格位")
	}
	x := g.Item
	old := x.CheckReceipt
	if old.sameVisit(m) && !allIn && old.AllIn {
		if old.First == nil || !old.First.sameVisit(m) || old.First.AllIn {
			return nil, errors.New("克苏鲁重掷账本缺少首次检定收据")
		}
		return old.First, nil
	}
	if old.sameVisit(m) && ((!allIn && !old.AllIn) || (allIn && old.AllIn)) {
		return old, nil
	}
	if allIn {
		if !old.sameVisit(m) || old.Success || old.AllIn || x.Status != 2 {
			return nil, errors.New("克苏鲁没有可重掷的本次失败")
		}
		unlocked := false
		for _, other := range w.Maps {
			unlocked = unlocked || cthulhuUnlocked(other, "finished_unlock_all_in")
		}
		if !unlocked {
			return nil, errors.New("克苏鲁孤注一掷未解锁")
		}
		if err := cthulhuConsume(p, activityData("cthulhu", 206).ids("random_need_materials")); err != nil {
			return nil, err
		}
	} else if x.Status != 1 {
		return nil, errors.New("克苏鲁检定事件未触发或已经领取")
	}
	var dice [2]int
	if fixedDice != nil {
		dice = *fixedDice
	} else {
		for i := range dice {
			n, err := activityAmount([]int64{1, 6})
			if err != nil {
				return nil, err
			}
			dice[i] = int(n)
		}
	}
	if dice[0] < 1 || dice[0] > 6 || dice[1] < 1 || dice[1] > 6 {
		return nil, errors.New("克苏鲁骰值不合法")
	}
	r := activityData("cthulhu_items", itemID)
	attribute := p.Materials[r.integer("check_material")].Count
	if attribute < 0 || attribute > 12 {
		return nil, errors.New("克苏鲁检定属性存档无效")
	}
	sum := dice[0] + dice[1]
	big := sum == 2 || sum == 12
	success := int64(sum+r.integer("fix_check")) <= attribute
	if big {
		success = sum == 2
	}
	box := emptyActivityBox()
	if big {
		pair := activityData("cthulhu", 206).ids("fail_materials")
		if success {
			pair = activityData("cthulhu", 206).ids("success_materials")
		}
		if len(pair) != 2 || pair[0] != 53 || pair[1] <= 0 {
			return nil, errors.New("克苏鲁理智变更配置无效")
		}
		mat := p.Materials[pair[0]]
		limit := int64(activityData("materials", pair[0]).integer("limit_count"))
		if mat.Count < 0 || mat.Count > limit || mat.Total < mat.Count {
			return nil, errors.New("克苏鲁理智存档无效")
		}
		if success {
			added := int64(pair[1])
			if added > limit-mat.Count {
				added = limit - mat.Count
			}
			if mat.Total > math.MaxInt64-added {
				return nil, errors.New("克苏鲁理智累计溢出")
			}
			mat.Count += added
			mat.Total += added
			if added > 0 {
				box["materials"].(map[int]int64)[pair[0]] = added
			}
		} else {
			mat.Count -= int64(pair[1])
			if mat.Count < 0 {
				mat.Count = 0
			}
		}
		p.Materials[pair[0]] = mat
	}
	if success {
		part, err := grantCthulhuBonus(p, []int{r.integer("bonus_id")}, now)
		if err != nil {
			return nil, err
		}
		if err = mergeActivityBox(box, part); err != nil {
			return nil, err
		}
		x.Status = 3
	} else {
		x.Status = 2
	}
	if x.Checks == math.MaxInt32 || m.Checks == math.MaxInt32 {
		return nil, errors.New("克苏鲁检定次数溢出")
	}
	x.Checks++
	m.Checks++
	m.FirstCheck = true
	receipt := &CthulhuCheckReceipt{Visit: m.Moves, Cycle: m.Cycle, Layer: m.Layer, Trunk: m.Trunk, Branch: m.Branch, Dice: dice, Success: success, Big: big, AllIn: allIn, Box: box}
	if allIn {
		receipt.First = old
	}
	x.CheckReceipt = receipt
	return receipt, nil
}
