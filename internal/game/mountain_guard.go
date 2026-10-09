package game

import (
	"encoding/json"
	"errors"
	"math"
)

// 原生EFCB6F9A的充能比例、守护值与技能等级直接冻结到prepare。
// A091551F和6D5412CA让客户端原引擎计算buff与结界技能，不在Go重写属性公式。
func freezeMountainGuardEffects(p *Progress, bc *ActivityBattleContext, row activityRow) error {
	ids := row.ids("guard_active_id")
	if len(ids) == 0 {
		return nil
	}
	w := ensureMountain(p)
	total := activityData("mountain_game_base_rule", 1).integer("total_process_value")
	if total <= 0 {
		return errors.New("山海守护充能总量无效")
	}
	bc.NativeBuffs = map[int][]ActivityNativeBuff{}
	bc.NativeMFields = map[int][][3]int{}
	seen := map[int]bool{}
	owned := map[int]bool{}
	for _, card := range p.Cards {
		owned[card.CardID] = true
	}
	for _, siteID := range ids {
		if seen[siteID] {
			return errors.New("山海守护重复激活槽")
		}
		seen[siteID] = true
		site := activityData("mountain_game_guard_site", siteID)
		direction := site.integer("guard_direction")
		circle := w.Circles[direction]
		if circle == nil {
			return errors.New("山海守护法阵丢失")
		}
		var sites []int
		if json.Unmarshal(androidActivities["mountain_game_guard"][intString(direction)], &sites) != nil {
			return errors.New("山海守护槽配置无效")
		}
		index := -1
		for i, id := range sites {
			if id == siteID {
				index = i
				break
			}
		}
		if index < 0 || index >= len(circle.Cards) || index >= len(circle.Unlocked) {
			return errors.New("山海守护槽位置无效")
		}
		card := circle.Cards[index]
		if card == 0 {
			continue
		}
		if !circle.Unlocked[index] || !owned[card] || circle.Progress < 0 || circle.Progress > total || !containsInt(site.ids("guard_card"), card) {
			return errors.New("山海守护存档资格无效")
		}
		valueRow := activityData("mountain_game_guard_cards", card)
		var base, process float64
		if json.Unmarshal(valueRow["base_effect_value"], &base) != nil || json.Unmarshal(valueRow["process_effect_value"], &process) != nil || math.IsNaN(base) || math.IsInf(base, 0) || math.IsNaN(process) || math.IsInf(process, 0) || base < 0 || process < 0 {
			return errors.New("山海守护数值无效")
		}
		var effects []json.RawMessage
		if json.Unmarshal(site["guard_effect"], &effects) != nil || len(effects) != 2 {
			return errors.New("山海守护效果结构无效")
		}
		var kind int
		var targets []int
		if json.Unmarshal(effects[0], &kind) != nil || json.Unmarshal(effects[1], &targets) != nil {
			return errors.New("山海守护效果编号无效")
		}
		per := float64(circle.Progress) / float64(total)
		switch kind {
		case 1:
			if remainingPolicyEnabled() {
				if bc.MaterialRates == nil {
					bc.MaterialRates = map[int]float64{}
				}
				for _, id := range targets {
					bc.MaterialRates[id] += base + process*per
				}
			}
		case 2:
			property := base + process*per
			for _, id := range targets {
				bc.NativeBuffs[1] = append(bc.NativeBuffs[1], ActivityNativeBuff{ID: id, Property: property})
			}
		case 3:
			if math.Trunc(base) != base || base > math.MaxInt32 || process*per > math.MaxInt32-1 {
				return errors.New("山海结界强化等级无效")
			}
			for _, id := range targets {
				bc.NativeMFields[1] = append(bc.NativeMFields[1], [3]int{id, 1 + int(process*per), int(base)})
			}
		default:
			return errors.New("山海守护效果类型未实现")
		}
	}
	return nil
}
