package game

import (
	"context"
	"encoding/json"
	"errors"
)

func (s *Service) setCaptainCard(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("队长卡参数无效")
	}
	var cardID int
	var dress *int // 原生 None 表示沿用该卡的队长外观。
	if string(args[0]) == "null" || json.Unmarshal(args[0], &cardID) != nil || cardID <= 0 || json.Unmarshal(args[1], &dress) != nil {
		return nil, errors.New("队长卡参数无效")
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		rule, ok := androidCardAppearances[cardID]
		if !ok || !ownsCardID(*p, cardID) {
			return errors.New("队长卡不存在或不属于当前玩家")
		}
		if containsInt(rule.Forbid, androidCaptainForbidden) {
			return errors.New("该卡牌禁止担任队长")
		}
		selected := captainDress(*p, cardID)
		if dress != nil {
			selected = *dress
		}
		if !validOwnedDress(*p, cardID, selected) {
			return errors.New("队长外观不存在、不属于该卡、未拥有或未满足觉醒要求")
		}
		if p.CaptainDresses == nil {
			p.CaptainDresses = map[int]int{}
		}
		if p.OwnedDresses == nil {
			p.OwnedDresses = map[int][]int{}
		}
		p.OwnedDresses[cardID] = ownedCardDresses(*p, cardID)
		p.CaptainDresses[cardID] = selected
		p.CaptainID = cardID
		p.CaptainCardUUID = ""
		return nil
	}); err != nil {
		return nil, err
	}
	p := c.SelectedAvatarUnsafe().Progress
	return []Push{
		push("Avatar", "client_prop_changed", []any{"card_common_mgr", cardCommonMgrPropertiesWithProgress(p)}),
		push("Avatar", "client_prop_changed", []any{"captain_id", p.CaptainID}),
	}, nil
}

func (s *Service) setLayoutCards(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("布阵参数无效")
	}
	var dungeon int
	if json.Unmarshal(args[0], &dungeon) != nil || dungeon <= 0 {
		return nil, errors.New("布阵参数无效")
	}
	if _, exists := dungeonCatalog[dungeon]; !exists {
		return nil, errors.New("未知布阵副本")
	}
	layout, err := parseBattleLayout(args[1])
	if err != nil {
		return nil, err
	}
	if err := s.updateBattleFormation(ctx, c, layout, dungeon, false, func(p *Progress) error {
		if err := validateBattleLayout(*p, layout); err != nil {
			return err
		}
		if p.Battle != nil && p.Battle.DungeonID == dungeon && !p.Battle.Finished {
			if p.Battle.Started {
				return errors.New("战斗开始后不能改布阵")
			}
			copy := cloneBattleLayout(layout)
			p.Battle.Layout = &copy
			p.Battle.Team = layout.team()
		}
		if p.BattleLayouts == nil {
			p.BattleLayouts = map[int]BattleLayout{}
		}
		p.BattleLayouts[dungeon] = cloneBattleLayout(layout)
		p.Lineup = layout.team()
		return nil
	}); err != nil {
		return nil, err
	}
	return nil, nil // 原生无callback；不猜测不存在的lineup属性推送。
}
