package game

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
)

// Android 5FFFCFD8.handbook_item 的 battle_addition_effects 是完整 buff 序列，
// 2080021/2080001 每次添加一层。A091551F.create_avatar 原生消费
// change_attr_data.buff[camp] 的二元组；缺省 user_property 为 None。
// 仅新会话从所属地图已激活手册冻结，重入/恢复沿 ActivityBattleContext。
func freezeActivityConfirmedBuffs(p *Progress, bc *ActivityBattleContext) error {
	if bc == nil {
		return nil
	}
	if bc.ActivityID == 207 {
		return freezeWangyanConfirmedBuffs(p, bc)
	}
	if bc.ActivityID == 211 {
		bc.SummerClosedBeta = true
		return nil
	}
	if bc.ActivityID != 208 {
		return nil
	}
	if p.Activities.Miku == nil {
		return errors.New("初音手册战斗状态丢失")
	}
	m := p.Activities.Miku.Maps[bc.MapID]
	if m == nil {
		return errors.New("初音手册战斗地图丢失")
	}
	groups := make([]int, 0, len(m.Handbook.Items))
	for group := range m.Handbook.Items {
		groups = append(groups, group)
	}
	sort.Ints(groups)
	for _, group := range groups {
		for _, item := range m.Handbook.Items[group] {
			if item.State != 2 {
				continue
			}
			row := activityData("handbook_item", item.ID)
			if len(row) == 0 {
				return errors.New("初音已激活手册条目不存在")
			}
			for _, id := range row.ids("battle_addition_effects") {
				if id != 2080001 && id != 2080021 {
					return errors.New("初音手册战斗效果缺原生验证")
				}
				if bc.NativeBuffs == nil {
					bc.NativeBuffs = map[int][]ActivityNativeBuff{}
				}
				bc.NativeBuffs[1] = append(bc.NativeBuffs[1], ActivityNativeBuff{ID: id, NullProperty: true})
			}
		}
	}
	return nil
}

// 2BCB4D1C.get_node_affected_nodes/get_node_combat_power/get_correction_level，
// 2CC3F05D.on_bid_set 消费 enemy_level_added，非零修正最低等级为5。
// 原生UI文字明确未占领领地向魔神鬼将提供加成，占领后在后续新战斗消失。
func freezeWangyanConfirmedBuffs(p *Progress, bc *ActivityBattleContext) error {
	w := p.Activities.Wangyan
	if w == nil || w.Maps[bc.MapID] == nil {
		return errors.New("妄言战斗加成地图丢失")
	}
	node := activityData("wangyan_node", bc.NodeID)
	if len(node) == 0 || w.Power < 0 {
		return errors.New("妄言战斗战力或节点无效")
	}
	power := node.integer("main_combat_power")
	for _, id := range node.ids("affected_by_nodes") {
		if containsInt(w.Maps[bc.MapID].Occupied, id) {
			continue
		}
		affected := activityData("wangyan_node", id)
		if len(affected) == 0 {
			continue // 原生忽略已移除的影响节点。
		}
		added := affected.integer("combat_power_added")
		if added < 0 || power > math.MaxInt32-added {
			return errors.New("妄言节点战力加成溢出")
		}
		power += added
		if buff := affected.integer("buff_added"); buff > 0 {
			if buff < 6040174 || buff > 6040179 {
				return errors.New("妄言节点buff缺原生验证")
			}
			if bc.NativeBuffs == nil {
				bc.NativeBuffs = map[int][]ActivityNativeBuff{}
			}
			bc.NativeBuffs[2] = append(bc.NativeBuffs[2], ActivityNativeBuff{ID: buff, NullProperty: true})
		}
	}
	correction, err := wangyanCorrectionLevel(w.Power, power)
	if err != nil {
		return err
	}
	bc.EnemyLevelAdded = &correction
	return nil
}

func wangyanCorrectionLevel(player, enemy int) (int, error) {
	if player < 0 || enemy < 0 {
		return 0, errors.New("妄言战力不能为负数")
	}
	if enemy == 0 {
		return 0, nil
	}
	ratio := float64(player) / float64(enemy)
	keys := []int{}
	for key := range androidActivities["wangyan_level_correction"] {
		id, err := strconv.Atoi(key)
		if err != nil {
			return 0, errors.New("妄言等级修正编号无效")
		}
		keys = append(keys, id)
	}
	sort.Ints(keys)
	for _, id := range keys {
		row := activityData("wangyan_level_correction", id)
		var conditions [][]json.RawMessage
		if json.Unmarshal(row["compare_data"], &conditions) != nil {
			return 0, errors.New("妄言等级修正条件无效")
		}
		matched := len(conditions) > 0
		for _, pair := range conditions {
			var op string
			var bound float64
			if len(pair) != 2 || json.Unmarshal(pair[0], &op) != nil || json.Unmarshal(pair[1], &bound) != nil || math.IsNaN(bound) || math.IsInf(bound, 0) {
				return 0, errors.New("妄言等级修正比较无效")
			}
			var pass bool
			switch op {
			case "gt":
				pass = ratio > bound
			case "ge":
				pass = ratio >= bound
			case "lt":
				pass = ratio < bound
			case "le":
				pass = ratio <= bound
			case "eq":
				pass = ratio == bound
			case "ne":
				pass = ratio != bound
			default:
				return 0, errors.New("妄言等级修正比较符未实现")
			}
			matched = matched && pass
		}
		if matched {
			return row.integer("correction_level"), nil
		}
	}
	return 0, nil
}
