package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"
)

func collectionFacilityTagProfit(p Progress, id int) (map[string][2]float64, error) {
	profits := map[string][2]float64{}
	if _, ok := p.UnlockSystems["house_character_tag"]; !ok {
		return profits, nil
	}
	roomID := androidCollection.Facilities[id].Room
	room := p.Collection.Rooms[roomID]
	intimacy, e := loadIntimacyCatalog()
	if e != nil {
		return nil, e
	}
	cards := []int{}
	for _, cid := range room.Slots {
		cards = append(cards, cid)
	}
	sort.Ints(cards)
	for _, cid := range cards {
		rows := androidCollection.Cards[cid].Tags
		if len(rows) < 3 || len(rows[2]) != 2 || intimacyLevel(p.Intimacy[cid], intimacy) < rows[2][1] {
			continue
		}
		tag := androidCollection.Tags[rows[2][0]]
		if tag.Affect == 0 {
			continue
		}
		for _, effectID := range tag.Effects {
			effect, known := androidCollection.TagEffects[effectID]
			if !known {
				return nil, errors.New("收藏室性格效果模板缺失")
			}
			matched := effect.Condition == 2 && effect.Room == roomID && effect.Facility == id
			if effect.Condition == 3 && effect.Room == roomID && effect.Facility == id {
				for _, other := range cards {
					matched = matched || other == effect.Card
				}
			}
			if !matched {
				continue
			}
			if len(effect.Data) != 2 || (effect.Data[0] != 1 && effect.Data[0] != 2) {
				continue
			}
			kind := int(effect.Data[0])
			value := effect.Data[1]
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return nil, errors.New("收藏室性格效果数值无效")
			}
			// 原生process_character_tag_data按效果+数值类型取最大，不相加。
			profit := profits[effect.Effect]
			if value > profit[kind-1] {
				profit[kind-1] = value
			}
			profits[effect.Effect] = profit
		}
	}
	return profits, nil
}

// 原生produce_rate=实际单位产量/实际单位时间，入住人数收益由设施lambda乘4。
func collectionFacilityRate(p Progress, f CollectionFacility) (float64, float64, error) {
	row, known := androidCollection.Levels[f.ID][f.Level]
	if !known {
		return 0, 0, errors.New("设施等级目录缺失")
	}
	if row.ProduceType == 0 || row.Produce <= 0 {
		return 0, 0, nil
	}
	if row.Unit <= 0 || row.ProduceNum < 0 || row.Storage < 0 {
		return 0, 0, errors.New("设施生产数值无效")
	}
	roomID := androidCollection.Facilities[f.ID].Room
	room := p.Collection.Rooms[roomID]
	profit := 0.0
	n := len(room.Slots)
	values := androidCollection.Rooms[roomID].CardProfit
	if n > 0 && n <= len(values) {
		profit = values[n-1] * 4
	}
	tags, e := collectionFacilityTagProfit(p, f.ID)
	if e != nil {
		return 0, 0, e
	}
	output := ""
	switch f.ID {
	case 2:
		output = "building_material_output"
	case 3:
		output = "furniture_coin_output"
	case 4:
		output = "gift_output"
	default:
		return 0, 0, errors.New("设施生产编号未取证")
	}
	amount := row.ProduceNum * (1 + tags[output][0])
	unit := row.Unit / (1 + profit) / (1 + tags["efficiency_improve"][0])
	storage := row.Storage*(1+tags["storage"][0]) + tags["storage"][1]
	return amount / unit, storage, nil
}
func collectionRefreshFacilities(p *Progress, now time.Time) error {
	if p.Collection == nil {
		return nil
	}
	stamp := float64(now.UnixNano()) / 1e9
	for id, f := range p.Collection.Facilities {
		if f.ID != id || f.Level < 1 || f.Keep < 0 || f.Rate < 0 || f.Storage < 0 || math.IsNaN(f.Keep+f.Rate+f.Storage+f.ProduceStart) || math.IsInf(f.Keep+f.Rate+f.Storage+f.ProduceStart, 0) || f.ProduceStart > stamp {
			return errors.New("设施生产存档无效或时间回拨")
		}
		if f.ProduceStart > 0 && f.Rate > 0 {
			f.Keep = math.Min(f.Storage, f.Keep+(stamp-f.ProduceStart)*f.Rate)
		}
		rate, storage, e := collectionFacilityRate(*p, f)
		if e != nil {
			return e
		}
		f.Rate = rate
		f.Storage = storage
		f.ProduceStart = stamp
		if f.Keep > storage {
			f.Keep = storage
		}
		p.Collection.Facilities[id] = f
	}
	return nil
}

func (s *Service) collectionProductionRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 1 {
		return nil, errors.New("收获生产设施需要设施编号")
	}
	var id int
	if json.Unmarshal(args[0], &id) != nil || id <= 0 {
		return nil, errors.New("收获设施编号无效")
	}
	var box map[string]any
	var info []int64
	e := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureCollection(p, s.Now()); e != nil {
			return e
		}
		f, owned := p.Collection.Facilities[id]
		if !owned {
			return runeReject("RET_HOUSE_FACILITY_LOCKED", "设施尚未解锁")
		}
		row := androidCollection.Levels[id][f.Level]
		if row.ProduceType != 1 && row.ProduceType != 2 {
			return errors.New("此设施没有生产材料")
		}
		count := int64(math.Floor(f.Keep))
		if count <= 0 {
			return runeReject("RET_HOUSE_FACILITY_GATHER_PRODUCE_ERROR", "没有可收获产物")
		}
		if row.ProduceType == 1 {
			changes := map[int]int64{}
			cards := []string{}
			if e := grantNativeItem(p, row.Produce, count, p.AvatarLevel, s.Now(), changes, &cards, 0); e != nil {
				return e
			}
			box = map[string]any{"__custom_type": "box.box", "materials": changes, "cards": cardListWire(p, cards)}
		} else {
			var e error
			box, e = grantNativeBonus(p, row.Produce, count, p.AvatarLevel, s.Now())
			if e != nil {
				return e
			}
		}
		info = []int64{int64(row.Produce), count}
		f.Keep -= float64(count)
		p.Collection.Facilities[id] = f
		advanceAchievementAmount(p, 33, "gather_produce_material", 1, s.Now())
		return nil
	})
	if e != nil {
		var reject *runeBusinessError
		if !errors.As(e, &reject) {
			return nil, e
		}
		code, known := androidCollection.Errors[reject.Name]
		if !known {
			return nil, errors.New("设施收获错误码缺失")
		}
		return []Push{push("Avatar", "on_gather_produce_material", code, id, []int64{}, map[string]any{"__custom_type": "box.box", "materials": map[int]int64{}})}, nil
	}
	p := c.SelectedAvatarUnsafe().Progress
	out := []Push{materialManagerPush(c), cardMgrPush(c), runePush(c), knowledgePush(c)}
	out = append(out, collectionPushes(p, s.Now(), false)...)
	return append(out, push("Avatar", "on_gather_produce_material", RetSuccess, id, info, box)), nil
}
