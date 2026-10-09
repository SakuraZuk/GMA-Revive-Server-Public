package game

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

const maxCardLevel = 70

func findCard(p *Progress, uuid string) (int, *Card) {
	for i := range p.Cards {
		if strings.EqualFold(p.Cards[i].UUID, uuid) {
			return i, &p.Cards[i]
		}
	}
	return -1, nil
}

func cardMgrPush(c *Connection) Push {
	av := c.SelectedAvatarUnsafe()
	return push("Avatar", "client_prop_changed", []any{"card_mgr", cardMgrPropertiesWithRunes(av.Progress.Cards, av.Progress.Runes)})
}

func (s *Service) cardGrowthRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("卡牌操作需要玩家状态")
	}
	if len(args) < 2 {
		return nil, errors.New("卡牌 callback 或 uuid 缺失")
	}
	var callbackID int
	if json.Unmarshal(args[0], &callbackID) != nil {
		return nil, errors.New("卡牌 callback 无效")
	}
	var uuid string
	if json.Unmarshal(args[1], &uuid) != nil || uuid == "" {
		return nil, errors.New("卡牌 uuid 无效")
	}
	switch method {
	case "up_level_card":
		if len(args) != 3 {
			return nil, errors.New("升级卡牌需要 fast 参数")
		}
		var fast bool
		if json.Unmarshal(args[2], &fast) != nil {
			return nil, errors.New("升级 fast 参数无效")
		}
		err := s.updateProgress(ctx, c, func(p *Progress) error {
			_, card := findCard(p, uuid)
			if err := checkCardGrowth(card); err != nil {
				return err
			}
			cap, err := cardLevelCap(card)
			if err != nil {
				return err
			}
			if card.Level < 1 || card.Exp < 0 {
				return errors.New("幻书等级经验存档无效")
			}
			if card.Level >= cap {
				return runeReject("RET_CARD_REACH_MAX_LEVEL", "幻书已达到当前品阶上限")
			}
			if !cardLevelUnlocked(card.Level+1, p.AvatarLevel) {
				return runeReject("RET_CARD_LIMIT_BY_MASTER", "馆主等级不足")
			}
			ink := p.Materials[4]
			if ink.Count < 0 {
				return errors.New("知识储备存档无效")
			}
			level, exp, spent := card.Level, int64(card.Exp), int64(0)
			for level < cap && cardLevelUnlocked(level+1, p.AvatarLevel) {
				row, ok := androidOath.Levels[level]
				if !ok || row.Exp <= 0 || exp > row.Exp {
					return errors.New("幻书升级经验表或存档无效")
				}
				need := row.Exp - exp
				if need > ink.Count-spent {
					break
				}
				spent += need
				level++
				exp = 0
				if !fast {
					break
				}
			}
			if level == card.Level {
				return runeReject("RET_CARD_EXP_NOT_ENOUGH", "知识储备不足")
			}
			card.Level, card.Exp = level, int(exp)
			ink.Count -= spent
			p.Materials[4] = ink
			advanceAchievementAmount(p, 33, method, 1, s.Now())
			return nil
		})
		if err != nil {
			return growthCallbackError(callbackID, err)
		}
		return []Push{cardMgrPush(c), materialManagerPush(c), knowledgePush(c), Callback(callbackID, []any{0})}, nil
	case "upgrade_card":
		if len(args) != 2 {
			return nil, errors.New("升品卡牌参数无效")
		}
		err := s.updateProgress(ctx, c, func(p *Progress) error {
			_, card := findCard(p, uuid)
			if err := checkCardGrowth(card); err != nil {
				return err
			}
			if card.Grade >= androidOath.MaxGrade {
				return runeReject("RET_CARD_REACH_MAX_GRADE", "幻书已达到最高品阶")
			}
			cap, err := cardLevelCap(card)
			if err != nil {
				return err
			}
			if card.Level < cap {
				return runeReject("RET_CARD_NOT_REACH_MAX_LEVEL", "幻书未达到当前品阶满级")
			}
			rows := androidOath.Cards[card.CardID].Upgrade
			if card.Grade < 0 || card.Grade >= len(rows) || len(rows[card.Grade]) == 0 {
				return errors.New("幻书升品材料表缺失")
			}
			costs := map[int]int64{}
			for _, row := range rows[card.Grade] {
				if len(row) != 2 || row[0] <= 0 || row[1] <= 0 {
					return errors.New("幻书升品材料配置无效")
				}
				costs[row[0]] += int64(row[1])
			}
			for id, cost := range costs {
				if p.Materials[id].Count < cost {
					return runeReject("RET_MATERIAL_NOT_ENOUGH", "幻书升品材料不足")
				}
			}
			for id, cost := range costs {
				material := p.Materials[id]
				material.Count -= cost
				p.Materials[id] = material
			}
			card.Grade++
			return nil
		})
		if err != nil {
			return growthCallbackError(callbackID, err)
		}
		return []Push{cardMgrPush(c), materialManagerPush(c), Callback(callbackID, []any{0})}, nil
	case "card_enhance":
		if len(args) != 3 {
			return nil, errors.New("卡牌补完参数无效")
		}
		var materialUUIDs []string
		if json.Unmarshal(args[2], &materialUUIDs) != nil {
			return nil, errors.New("补完材料无效")
		}
		var box map[string]any
		err := s.updateProgress(ctx, c, func(p *Progress) error {
			var e error
			box, e = enhanceCardNative(p, uuid, materialUUIDs, s.Now())
			return e
		})
		if err != nil {
			var rejected *runeBusinessError
			if !errors.As(err, &rejected) {
				return nil, err
			}
			rules, e := loadIntimacyCatalog()
			if e != nil {
				return nil, e
			}
			code, known := rules.Errors[rejected.Name]
			if !known {
				return nil, errors.New("补完错误码缺失")
			}
			return []Push{Callback(callbackID, []any{code, map[string]any{"__custom_type": "box.box", "materials": map[int]int64{}}})}, nil
		}
		return []Push{cardMgrPush(c), materialManagerPush(c), runePush(c), push("Avatar", "client_prop_changed", []any{"card_common_mgr", cardCommonMgrPropertiesWithProgress(c.SelectedAvatarUnsafe().Progress)}), Callback(callbackID, []any{RetSuccess, box})}, nil
	default:
		return nil, errors.New("卡牌方法未实现")
	}
}
