package game

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"sort"
	"time"
)

type shopItemLibrary struct {
	Item   int     `json:"item_id"`
	Counts []int64 `json:"item_count"`
}

// 调用者必须处于玩家事务。D44DBBA7的固定、随机材料/契印、素材库共用解释器。
// 未知解锁、幸运保底、没有明确套装池的契印拒绝，不能以预览奖励代替。
func grantNativeBonus(p *Progress, id int, amount int64, level int, now time.Time) (map[string]any, error) {
	changes := map[int]int64{}
	cards := []string{}
	before := map[string]bool{}
	for uuid := range p.Runes {
		before[uuid] = true
	}
	if err := grantNativeBonusAssets(p, id, amount, level, now, changes, &cards, 0); err != nil {
		return nil, err
	}
	box := map[string]any{"__custom_type": "box.box", "materials": changes, "cards": cardListWire(p, cards)}
	runes := map[string]Rune{}
	for uuid, r := range p.Runes {
		if !before[uuid] {
			runes[uuid] = r
		}
	}
	if len(runes) > 0 {
		box["runes"] = runeMgrProperties(runes)
	}
	return box, nil
}

// 原生bonus_utils.select_material_id只选择固定奖励中的实际条目。
func grantNativeSelectedBonus(p *Progress, id int, amount int64, selected, level int, now time.Time, changes map[int]int64, cards *[]string, depth int) error {
	if depth > 8 || amount <= 0 || amount > 10000 {
		return errors.New("自选奖励批量无效")
	}
	id = resolveShopBonus(id, now.Unix())
	b, known := androidShop.Bonuses[id]
	if !known || b.Lucky != 0 || len(b.RandomItems)+len(b.RandomRunes)+len(b.RandomLibs) > 0 || (len(b.Inner) > 0 && string(b.Inner) != "null") {
		return errors.New("自选奖励包含未取证动态规则")
	}
	matched := false
	for _, row := range b.Fixed {
		if len(row) != 2 || row[0] <= 0 || row[1] <= 0 || row[1] > math.MaxInt64/amount {
			return errors.New("自选奖励目录无效")
		}
		if row[0] == int64(selected) {
			matched = true
			if e := grantNativeItem(p, selected, row[1]*amount, level, now, changes, cards, depth+1); e != nil {
				return e
			}
		}
	}
	if !matched {
		return runeReject("RET_SHOP_INVALID", "选择材料不在商品原生奖励中")
	}
	return nil
}

func nativeRandomCount(counts []int64) (int64, error) {
	if len(counts) == 0 {
		return 0, errors.New("奖励数量区间为空")
	}
	minimum, maximum := counts[0], counts[0]
	for _, n := range counts {
		if n < 0 || n > math.MaxInt32 {
			return 0, errors.New("奖励数量区间无效")
		}
		if n < minimum {
			minimum = n
		}
		if n > maximum {
			maximum = n
		}
	}
	draw, e := rand.Int(rand.Reader, big.NewInt(maximum-minimum+1))
	if e != nil {
		return 0, e
	}
	return minimum + draw.Int64(), nil
}
func grantNativeRandomItems(p *Progress, rows []json.RawMessage, amount int64, level int, now time.Time, changes map[int]int64, cards *[]string, depth int) error {
	for _, raw := range rows {
		var fields []json.RawMessage
		var id int
		var counts []int64
		var chance float64
		if json.Unmarshal(raw, &fields) != nil || len(fields) != 3 || json.Unmarshal(fields[0], &id) != nil || json.Unmarshal(fields[1], &counts) != nil || json.Unmarshal(fields[2], &chance) != nil || id <= 0 {
			return errors.New("随机材料结构无效")
		}
		for batch := int64(0); batch < amount; batch++ {
			chosen, e := shopProbabilityChoice(chance)
			if e != nil {
				return e
			}
			if !chosen {
				continue
			}
			count, e := nativeRandomCount(counts)
			if e != nil {
				return e
			}
			if count == 0 {
				continue
			}
			if e := grantNativeItem(p, id, count, level, now, changes, cards, depth); e != nil {
				return e
			}
		}
	}
	return nil
}

func nativeLibraryWeights(p Progress, library int, level int) ([]int, []int64, error) {
	data, known := androidShop.LibraryData[library]
	if !known {
		return nil, nil, errors.New("素材库预计算权重目录不存在")
	}
	keys := []int{}
	for id := range data {
		keys = append(keys, id)
	}
	sort.Ints(keys)
	ids := []int{}
	weights := []int64{}
	for _, id := range keys {
		var fields []json.RawMessage
		var weight int64
		var conditionEntries []json.RawMessage
		if json.Unmarshal(data[id], &fields) != nil || len(fields) != 2 || json.Unmarshal(fields[0], &weight) != nil || json.Unmarshal(fields[1], &conditionEntries) != nil || weight < 0 {
			return nil, nil, errors.New("素材库权重结构无效")
		}
		if _, known := androidShop.ItemLibraries[id]; !known {
			return nil, nil, errors.New("素材库材料目录不存在")
		}
		allowed := true
		for _, raw := range conditionEntries {
			var entry []json.RawMessage
			var conditions [][][]json.RawMessage
			if json.Unmarshal(raw, &entry) != nil || len(entry) != 2 || json.Unmarshal(entry[0], &conditions) != nil {
				return nil, nil, errors.New("素材库解锁条件无效")
			}
			ok, e := shopConditions(conditions, p, level)
			if e != nil {
				return nil, nil, e
			}
			if !ok {
				allowed = false
				break
			}
		}
		if allowed && weight > 0 {
			ids = append(ids, id)
			weights = append(weights, weight)
		}
	}
	return ids, weights, nil
}
func grantNativeItemLibraries(p *Progress, rows []json.RawMessage, amount int64, level int, now time.Time, changes map[int]int64, cards *[]string, depth int) error {
	for _, raw := range rows {
		var fields []json.RawMessage
		var library int
		var count, repeats int64
		var chance float64
		if json.Unmarshal(raw, &fields) != nil || len(fields) != 4 || json.Unmarshal(fields[0], &library) != nil || json.Unmarshal(fields[1], &count) != nil || json.Unmarshal(fields[2], &chance) != nil || json.Unmarshal(fields[3], &repeats) != nil || library <= 0 || count < 0 || repeats < 0 || count > 10000 || repeats > 10000 || (count > 0 && repeats > 10000/count) || (repeats > 0 && amount > 10000/repeats) {
			return errors.New("随机素材库结构或批量无效")
		}
		if count == 0 || repeats == 0 {
			continue
		}
		for round := int64(0); round < repeats*amount; round++ {
			chosen, e := shopProbabilityChoice(chance)
			if e != nil {
				return e
			}
			if !chosen {
				continue
			}
			ids, weights, e := nativeLibraryWeights(*p, library, level)
			if e != nil {
				return e
			}
			for pick := int64(0); pick < count && len(ids) > 0; pick++ {
				index, e := shopWeightedIndex(weights)
				if e != nil {
					return e
				}
				rule := androidShop.ItemLibraries[ids[index]]
				quantity, e := nativeRandomCount(rule.Counts)
				if e != nil {
					return e
				}
				if quantity > 0 {
					if e := grantNativeItem(p, rule.Item, quantity, level, now, changes, cards, depth); e != nil {
						return e
					}
				}
				ids = append(ids[:index], ids[index+1:]...)
				weights = append(weights[:index], weights[index+1:]...)
			}
		}
	}
	return nil
}
