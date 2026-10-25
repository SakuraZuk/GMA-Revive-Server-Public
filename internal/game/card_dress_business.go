package game

import (
	"context"
	"encoding/json"
	"errors"
)

// 原生card_mgr发送(callback, uuid, dress_id)，回调仅含整数错误码。
// 归属和觉醒检查在同一存档事务中完成，属性必须先于成功回调推送。
func (s *Service) changeCardDress(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 3 {
		return nil, errors.New("换装需要玩家状态、回调、幻书编号和装帧编号")
	}
	var callback, dress int
	var uuid string
	if string(args[0]) == "null" || string(args[2]) == "null" || json.Unmarshal(args[0], &callback) != nil || json.Unmarshal(args[1], &uuid) != nil || uuid == "" || json.Unmarshal(args[2], &dress) != nil {
		return nil, errors.New("换装参数无效")
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		_, card := findCard(p, uuid)
		if card == nil {
			return runeReject("RET_CARD_NOT_EXIST", "幻书不属于当前角色")
		}
		if effectiveCardDress(*card) == dress {
			return runeReject("RET_CARD_HAS_DRESS", "幻书已穿着该装帧")
		}
		if !containsInt(ownedCardDresses(*p, card.CardID), dress) {
			return runeReject("RET_CARD_DRESS_NOT_EXIST", "装帧未拥有或不属于该幻书")
		}
		needsAwakened, exists := androidDressNeedsAwakened[dress]
		if !exists {
			return runeReject("RET_CARD_DRESS_NOT_EXIST", "装帧不存在")
		}
		if needsAwakened && effectiveCardAwakened(*card) == 0 {
			return runeReject("RET_CARD_DRESS_BY_AWAKE", "该幻书尚未达到装帧觉醒要求")
		}
		card.Dress = dress
		return nil
	})
	if err != nil {
		return growthCallbackError(callback, err)
	}
	return []Push{cardMgrPush(c), Callback(callback, []any{RetSuccess})}, nil
}
