package game

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"
)

// 原生materials206001明确“战斗失败返还”；return_supply不得当成胜利奖励。
func refundWangyanSupply(p *Progress, bc *ActivityBattleContext, now time.Time) error {
	w := p.Activities.Wangyan
	if w == nil {
		return errors.New("妄言失败返还状态缺失")
	}
	if e := refreshWangyanSupply(w, now); e != nil {
		return e
	}
	amount := activityData("wangyan_dungeons", bc.DungeonID).integer("return_supply")
	if amount < 0 || amount > bc.Resource || w.Supply.Value > math.MaxInt32-amount {
		return errors.New("妄言返还补给超过实际扣费")
	}
	_, err := grantWangyanSupply(p, int64(amount), now)
	return err
}

// E27E0BCF详情页直接展示所有stype21物品总数乘bonus20703025，3B原生first任务须全部已完成。
func submitWangyanSpecial(p *Progress, r activityRow, now time.Time) (map[string]any, error) {
	w := p.Activities.Wangyan
	if w == nil {
		return nil, errors.New("妄言委托状态缺失")
	}
	for key := range androidActivities["wangyan_game"] {
		id, _ := strconv.Atoi(key)
		if activityData("wangyan_game", id).integer("task_type") == 1 {
			if _, ok := w.Mini[id]; !ok {
				return nil, errors.New("妄言首轮委托尚未全部完成")
			}
		}
	}
	total := int64(0)
	inputs := []int{}
	for id, mat := range p.Materials {
		if activityData("materials", id).integer("stype") == 21 && mat.Count > 0 {
			if total > math.MaxInt64-mat.Count {
				return nil, errors.New("妄言特殊委托输入溢出")
			}
			total += mat.Count
			inputs = append(inputs, id)
		}
	}
	if total <= 0 {
		return nil, errors.New("妄言特殊委托材料不足")
	}
	// 本Android奖励只有固定妄言币；数量一次乘总库存，不能随意截断为一份或丢弃多余物品。
	b := activityData("bonus", r.integer("bonus_id"))
	var rows [][]int64
	if json.Unmarshal(b["fixed_items"], &rows) != nil || len(rows) == 0 || len(b.ids("random_items"))+len(b.ids("random_runes"))+len(b.ids("random_item_libs")) > 0 {
		return nil, errors.New("妄言特殊委托奖励需要新的原生规则")
	}
	box := emptyActivityBox()
	changes := box["materials"].(map[int]int64)
	cards := []string{}
	for _, pair := range rows {
		if len(pair) != 2 || pair[0] <= 0 || pair[1] <= 0 || pair[1] > math.MaxInt64/total {
			return nil, errors.New("妄言特殊委托奖励数量无效")
		}
		if e := grantNativeItem(p, int(pair[0]), pair[1]*total, p.AvatarLevel, now, changes, &cards, 0); e != nil {
			return nil, e
		}
	}
	for _, id := range inputs {
		mat := p.Materials[id]
		mat.Count = 0
		p.Materials[id] = mat
	}
	box["cards"] = cardListWire(p, cards)
	return box, nil
}

// 原生wangyan_general_args.area_bonus与activity_fool_map规则：10/30/50领地加成10%/20%/30%。
func applyWangyanMaterialBonus(p *Progress, box map[string]any, now time.Time) error {
	w := p.Activities.Wangyan
	if w == nil {
		return errors.New("妄言奖励状态缺失")
	}
	r := activityData("wangyan_general_args", 1)
	var tiers [][]float64
	if json.Unmarshal(r["area_bonus"], &tiers) != nil {
		return errors.New("妄言领地倍率配置无效")
	}
	rate := float64(0)
	for _, pair := range tiers {
		if len(pair) != 2 || pair[0] < 0 || pair[1] < 0 {
			return errors.New("妄言领地倍率结构无效")
		}
		if float64(w.Area) >= pair[0] {
			rate = pair[1]
		}
	}
	materials := box["materials"].(map[int]int64)
	for _, mid := range r.ids("addition_material_id") {
		base := materials[mid]
		if base <= 0 {
			continue
		}
		n := float64(base) * rate
		if math.IsNaN(n) || math.IsInf(n, 0) || n > math.MaxInt32 {
			return errors.New("妄言领地收益溢出")
		}
		added := int64(n)
		if added <= 0 {
			continue
		}
		changes := map[int]int64{}
		cards := []string{}
		if e := grantNativeItem(p, mid, added, p.AvatarLevel, now, changes, &cards, 0); e != nil {
			return e
		}
		materials[mid] += added
	}
	return nil
}
