package game

import (
	"context"
	"encoding/json"
	"errors"
)

func (s *Service) collectionWishlistRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("家具心愿单需要玩家状态")
	}
	callback, ok := callbackArg(args)
	if !ok {
		return nil, errors.New("家具心愿单缺少回调")
	}
	mid := 0
	switch method {
	case "set_up_furniture_material_id":
		if len(args) != 2 || json.Unmarshal(args[1], &mid) != nil || mid <= 0 {
			return nil, errors.New("家具心愿单目标无效")
		}
	case "reset_up_furniture_material_id":
		if len(args) != 1 {
			return nil, errors.New("清空家具心愿单不接受目标参数")
		}
	default:
		return nil, errors.New("家具心愿单接口不存在")
	}
	e := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureCollection(p, s.Now()); e != nil {
			return e
		}
		f, e := collectionWorkshop(p)
		if e != nil {
			return e
		}
		if !f.ComposeUnlocked {
			return runeReject("RET_HOUSE_COMPOSE_FURNITURE_NOT_UNLOCK", "限定家具融合尚未解锁")
		}
		if mid != 0 {
			mat, known := androidShop.Materials[mid]
			furniture, exists := androidCollection.Furniture[mat.Target]
			// 原生7D7F0010只把source=4家具列为融合心愿单。
			if !known || mat.Type != 6 || !exists || !containsInt(androidCraft.FurnitureSources[androidCraft.FurnitureUnlock[1].WishlistSource], mid) || furniture.Material != mid {
				return runeReject("RET_HOUSE_COMPOSE_FURNITURE_NOT_VALID", "心愿单目标不是原生融合限定家具")
			}
		}
		p.Collection.Wishlist = mid
		return nil
	})
	if e != nil {
		var reject *runeBusinessError
		if !errors.As(e, &reject) {
			return nil, e
		}
		code, known := androidCollection.Errors[reject.Name]
		if !known {
			return nil, errors.New("家具心愿单错误码缺失")
		}
		return []Push{Callback(callback, []any{code})}, nil
	}
	out := collectionPushes(c.SelectedAvatarUnsafe().Progress, s.Now(), false)
	return append(out, Callback(callback, []any{RetSuccess})), nil
}
