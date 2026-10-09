package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"math"
)

//go:embed craft_catalog.json
var craftCatalogRaw []byte

type craftCatalog struct {
	Recipes map[int]struct {
		Sources  [][]int64 `json:"src_material_ids"`
		Costs    [][]int64 `json:"cost_material_ids"`
		Material int       `json:"compose_material_id"`
	} `json:"recipes"`
	Unlocks map[int]struct {
		Materials []int `json:"material_ids"`
	} `json:"unlocks"`
	FurnitureUnlock map[int]struct {
		Material       int   `json:"material_id"`
		Count          int64 `json:"material_count"`
		WishlistSource int   `json:"wishlish_source_id"`
	} `json:"furniture_unlock"`
	Levels map[int]struct {
		Effects [][]json.RawMessage `json:"upgrade_effect"`
	} `json:"levels"`
	FurnitureSources map[int][]int                `json:"furniture_sources"`
	Fusion           map[int]collectionFusionRule `json:"fusion"`
	FusionInputCount int                          `json:"fusion_input_count"`
}

var androidCraft = func() craftCatalog {
	var c craftCatalog
	if json.Unmarshal(craftCatalogRaw, &c) != nil || len(c.Recipes) != 84 || len(c.Levels) != 5 {
		panic("Android加工配方目录无效")
	}
	return c
}()

func collectionWorkshop(p *Progress) (CollectionFacility, error) {
	for id, r := range androidCollection.Facilities {
		if r.Sheet == "facility_process" {
			f, owned := p.Collection.Facilities[id]
			if !owned {
				return CollectionFacility{}, runeReject("RET_HOUSE_FACILITY_LOCKED", "加工设施尚未解锁")
			}
			return f, nil
		}
	}
	return CollectionFacility{}, errors.New("原生加工设施目录缺失")
}
func collectionCraftUnlocked(f CollectionFacility, mid int) bool {
	for level, row := range androidCraft.Levels {
		if level > f.Level {
			continue
		}
		for _, effect := range row.Effects {
			if len(effect) != 2 {
				continue
			}
			var kind string
			var group int
			if json.Unmarshal(effect[0], &kind) != nil || (kind != "10" && kind != "18") || json.Unmarshal(effect[1], &group) != nil {
				continue
			}
			if containsInt(androidCraft.Unlocks[group].Materials, mid) {
				return true
			}
		}
	}
	return false
}
func collectionFurnitureUsed(p Progress, fid int) int64 {
	count := int64(0)
	for _, r := range p.Collection.Placements {
		if r[5] == fid {
			count++
		}
	}
	for _, list := range p.Collection.Wallpapers {
		for _, id := range list {
			if id == fid {
				count++
			}
		}
	}
	return count
}
func collectionConsumeCraft(p *Progress, mid int, count int64) error {
	mat, known := androidShop.Materials[mid]
	if !known || count < 0 {
		return errors.New("加工原材料目录无效")
	}
	if mat.Type != 6 {
		return collectionSpend(p, mid, count)
	}
	fid := mat.Target
	if collectionFurnitureCount(*p, fid)-collectionFurnitureUsed(*p, fid) < count {
		return runeReject("RET_MATERIAL_NOT_ENOUGH", "未摆放的家具原料不足")
	}
	direct := p.Collection.Furniture[fid]
	take := count
	if take > direct {
		take = direct
	}
	p.Collection.Furniture[fid] = direct - take
	count -= take
	if count > 0 {
		return collectionSpend(p, mid, count)
	}
	return nil
}

func (s *Service) craftRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if method != "unlock_furniture_compose" && method != "compose_furniture_normal" && method != "compose_material" {
		return nil, errors.New("加工接口不存在")
	}
	if c.phase != Playing {
		return nil, errors.New("加工需要玩家状态")
	}
	callback := 0
	mid := 0
	count := int64(0)
	if method == "unlock_furniture_compose" {
		if len(args) != 0 {
			return nil, errors.New("家具合成解锁不接受参数")
		}
	} else {
		var ok bool
		callback, ok = callbackArg(args)
		if !ok || len(args) != 3 || json.Unmarshal(args[1], &mid) != nil || json.Unmarshal(args[2], &count) != nil || mid <= 0 || count <= 0 || count > 10000 {
			return nil, errors.New("加工回调、目标材料或数量无效")
		}
	}
	box := map[string]any{"__custom_type": "box.box", "materials": map[int]int64{}}
	e := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureCollection(p, s.Now()); e != nil {
			return e
		}
		f, e := collectionWorkshop(p)
		if e != nil {
			return e
		}
		if method == "unlock_furniture_compose" {
			if f.Level < 4 {
				return runeReject("RET_HOUSE_PROCESS_FACILITY_LEVEL_NOT_ENOUGH", "家具合成需要原生4级加工设施")
			}
			if f.ComposeUnlocked {
				return runeReject("RET_HOUSE_COMPOSE_FURNITURE_UNLOCK", "家具合成已经解锁")
			}
			rule := androidCraft.FurnitureUnlock[1]
			if e := collectionSpend(p, rule.Material, rule.Count); e != nil {
				return e
			}
			f.ComposeUnlocked = true
			p.Collection.Facilities[f.ID] = f
			return nil
		}
		rule, known := androidCraft.Recipes[mid]
		if !known || rule.Material != mid {
			return errors.New("加工目标不在原生84份配方中")
		}
		if !collectionCraftUnlocked(f, mid) {
			return runeReject("RET_HOUSE_PROCESS_FACILITY_LEVEL_NOT_ENOUGH", "加工设施尚未开放目标配方")
		}
		furniture := androidShop.Materials[mid].Type == 6
		if method == "compose_furniture_normal" && (!furniture || !f.ComposeUnlocked) {
			return runeReject("RET_HOUSE_COMPOSE_FURNITURE_NOT_UNLOCK", "家具合成未解锁或目标不是家具")
		}
		if method == "compose_material" && furniture {
			return errors.New("材料合成不能绕过家具业务解锁")
		}
		roomID := androidCollection.Facilities[f.ID].Room
		n := len(p.Collection.Rooms[roomID].Slots)
		profit := 0.0
		profits := androidCollection.Rooms[roomID].CardProfit
		if n > 0 && n <= len(profits) {
			profit = profits[n-1] * -2
		}
		factor := 1 + profit
		if factor <= 0 {
			return errors.New("加工入住成本倍率无效")
		}
		costs := map[int]int64{}
		for groupIndex, rows := range [][][]int64{rule.Sources, rule.Costs} {
			for _, row := range rows {
				if len(row) != 2 || row[0] <= 0 || row[1] <= 0 {
					return errors.New("加工原生消耗配方无效")
				}
				unit := row[1]
				if groupIndex == 1 {
					unit = int64(math.Floor(factor * float64(unit)))
				}
				if unit < 0 || unit > math.MaxInt64/count || costs[int(row[0])] > math.MaxInt64-unit*count {
					return errors.New("加工消耗数量溢出")
				}
				costs[int(row[0])] += unit * count
			}
		}
		for id, n := range costs {
			if e := collectionConsumeCraft(p, id, n); e != nil {
				return e
			}
		}
		changes := map[int]int64{}
		cards := []string{}
		if e := grantNativeItem(p, mid, count, p.AvatarLevel, s.Now(), changes, &cards, 0); e != nil {
			return e
		}
		box = map[string]any{"__custom_type": "box.box", "materials": changes, "cards": cardListWire(p, cards)}
		syncCollectionHandbook(p)
		return nil
	})
	code := RetSuccess
	message := ""
	if e != nil {
		var reject *runeBusinessError
		if !errors.As(e, &reject) {
			return nil, e
		}
		var known bool
		code, known = androidCollection.Errors[reject.Name]
		if !known {
			return nil, errors.New("加工错误码缺失")
		}
		message = reject.Error()
	}
	out := []Push{}
	if e == nil {
		out = append(out, materialManagerPush(c))
		out = append(out, collectionPushes(c.SelectedAvatarUnsafe().Progress, s.Now(), false)...)
	}
	switch method {
	case "unlock_furniture_compose":
		return append(out, push("Avatar", "on_unlock_furniture_compose", code)), nil
	case "compose_furniture_normal":
		return append(out, Callback(callback, []any{code, box})), nil
	case "compose_material":
		if e != nil {
			return append(out, Callback(callback, []any{nil, message})), nil
		}
		return append(out, Callback(callback, []any{box, message})), nil
	}
	return nil, errors.New("加工接口尚未实现")
}
