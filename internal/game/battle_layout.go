package game

import (
	"encoding/hex"
	"encoding/json"
	"errors"
)

// BattleLayout 保留三个原生列表的顺序和空槽；内部空字符串在wire中转换为0。
// 剧情卡是客户端剧情上下文，不能因上传就写入玩家卡牌资产。
type BattleLayout struct {
	Fighting  []string `json:"fighting_cards"`
	Support   []string `json:"support_cards"`
	Storyline []string `json:"storyline_cards"`
}

func parseBattleLayout(raw json.RawMessage) (BattleLayout, error) {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return BattleLayout{}, errors.New("布阵必须为原生battle_cards字典")
	}
	parse := func(name string, limit int) ([]string, error) {
		value, exists := object[name]
		if !exists {
			return []string{}, nil
		}
		var slots []any
		if string(value) == "null" || json.Unmarshal(value, &slots) != nil || (limit > 0 && len(slots) > limit) {
			return nil, errors.New("布阵列表类型或数量无效：" + name)
		}
		out := make([]string, len(slots))
		for index, slot := range slots {
			if slot == nil {
				continue
			}
			if number, ok := slot.(float64); ok && number == 0 {
				continue
			}
			uuid, ok := slot.(string)
			if !ok || len(uuid) != 24 {
				return nil, errors.New("布阵元素必须为ObjectID或空槽")
			}
			bytes, err := hex.DecodeString(uuid)
			if err != nil || len(bytes) != 12 {
				return nil, errors.New("布阵ObjectID无效")
			}
			out[index] = hex.EncodeToString(bytes)
		}
		return out, nil
	}
	var layout BattleLayout
	var err error
	if layout.Fighting, err = parse("fighting_cards", androidFightingSlots); err != nil {
		return layout, err
	}
	if layout.Support, err = parse("support_cards", androidSupportSlots); err != nil {
		return layout, err
	}
	// 原生未声明剧情列表数量上限，不把出战容量套到剧情上下文。
	if layout.Storyline, err = parse("storyline_cards", 0); err != nil {
		return layout, err
	}
	if len(layout.Fighting) == 0 {
		return layout, errors.New("布阵缺少出战槽位")
	}
	return layout, nil
}

func (layout BattleLayout) team() []string {
	out := []string{}
	for _, slots := range [][]string{layout.Fighting, layout.Support} {
		for _, uuid := range slots {
			if uuid != "" {
				out = append(out, uuid)
			}
		}
	}
	return out
}

func validateBattleLayout(p Progress, layout BattleLayout) error {
	seen := map[string]bool{}
	for _, uuid := range layout.team() {
		if seen[uuid] {
			return errors.New("出战与援护阵容重复使用同一实例")
		}
		seen[uuid] = true
		card := battleFormationCard(&p, uuid)
		if card == nil {
			return errors.New("布阵卡牌未拥有或没有有效外部助战授权")
		}
		rule, exists := androidCardAppearances[card.CardID]
		if !exists || containsInt(rule.Forbid, androidBattleForbidden) {
			return errors.New("该卡牌禁止战斗")
		}
	}
	return nil
}

func cloneBattleLayout(layout BattleLayout) BattleLayout {
	return BattleLayout{append([]string{}, layout.Fighting...), append([]string{}, layout.Support...), append([]string{}, layout.Storyline...)}
}

func savedBattleLayout(p Progress, dungeonID int) BattleLayout {
	if layout, exists := p.BattleLayouts[dungeonID]; exists {
		layout = cloneBattleLayout(layout)
		// 卡牌分解后空出原槽位，不把后续卡牌向前挤。
		for _, slots := range [][]string{layout.Fighting, layout.Support} {
			for index, uuid := range slots {
				if uuid == "" {
					continue
				}
				card := battleFormationCard(&p, uuid)
				if card == nil {
					slots[index] = ""
					continue
				}
				rule, exists := androidCardAppearances[card.CardID]
				if !exists || containsInt(rule.Forbid, androidBattleForbidden) {
					slots[index] = ""
				}
			}
		}
		return layout
	}
	// 旧扁平数组可能混有援护；长度不能证明分组，不把它猜成出战阵容。
	// 原数据仍保留，客户端首次重新布阵后写入有分组证据的新模型。
	return BattleLayout{}
}

func battleSlotWire(slots []string) []any {
	out := make([]any, len(slots))
	for index, uuid := range slots {
		if uuid == "" {
			out[index] = 0
		} else {
			out[index] = ObjectID(uuid)
		}
	}
	return out
}
