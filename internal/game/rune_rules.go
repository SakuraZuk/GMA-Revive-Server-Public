package game

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"sort"
)

// fillRuneExtraAttrs 按 Android 解锁表补齐词条。已有词条、权重因子保持不变，
// 每次抽选不移除候选，允许重复属性；调用者必须持有玩家事务。
func fillRuneExtraAttrs(r *Rune, tables runeTables) error {
	return fillRuneExtraAttrsWithReader(r, tables, rand.Reader)
}

func fillRuneExtraAttrsWithReader(r *Rune, tables runeTables, entropy io.Reader) error {
	target := 0
	for _, row := range tables.Tables.UnlockAttrs {
		if row.Unlock != 0 && row.Level <= r.Level {
			target++
		}
	}
	if r.ExtraAttrsCount < 0 {
		return errors.New("契印附加词条数量无效")
	}
	if target > r.ExtraAttrsCount {
		target = r.ExtraAttrsCount
	}
	if len(r.ExtraAttrs) > target {
		return errors.New("契印已有词条超过解锁数量")
	}
	if len(r.ExtraAttrs) == target {
		return nil
	}
	// 独立复制，避免失败事务通过共享切片或字典污染原存档。
	extras := append([]int(nil), r.ExtraAttrs...)
	factors := make(map[int]int, len(r.ExtraAttrsFactor))
	for id, factor := range r.ExtraAttrsFactor {
		factors[id] = factor
	}
	weights := map[int]int64{}
	for _, id := range r.ExtraAttrsLib {
		found := false
		for _, attr := range tables.Tables.Attrs {
			if attr.ID != id {
				continue
			}
			found = true
			factor := 1
			if len(r.ExtraAttrsFactor) != 0 {
				if saved, ok := factors[id]; ok {
					factor = saved
				}
			} else {
				var err error
				// 未取证全局计数，使用原生方法 count=0 的基础范围。
				factor, err = runeFactorWithReader(attr.FactorRanges, 0, entropy)
				if err != nil {
					return err
				}
				factors[id] = factor
			}
			if factor <= 0 || attr.Weight < 0 || (attr.Weight > 0 && int64(factor) > math.MaxInt64/int64(attr.Weight)) {
				return errors.New("契印权重因子无效")
			}
			weights[id] = int64(attr.Weight) * int64(factor)
			break
		}
		if !found {
			return errors.New("契印词条库引用不存在的属性")
		}
	}
	for _, id := range extras {
		if _, ok := weights[id]; !ok {
			return errors.New("契印已有词条不在词条库中")
		}
	}
	ids := make([]int, 0, len(weights))
	var total int64
	for id, weight := range weights {
		if weight > 0 {
			if total > math.MaxInt64-weight {
				return errors.New("契印抽选权重超过容量")
			}
			total += weight
			ids = append(ids, id)
		}
	}
	if total == 0 {
		return errors.New("契印没有可抽选的附加属性")
	}
	sort.Ints(ids)
	for len(extras) < target {
		n, err := rand.Int(entropy, big.NewInt(total))
		if err != nil {
			return err
		}
		value := n.Int64()
		for _, id := range ids {
			value -= weights[id]
			if value < 0 {
				extras = append(extras, id)
				break
			}
		}
	}
	r.ExtraAttrs, r.ExtraAttrsFactor = extras, factors
	return nil
}

// runeFactorAtCount 对应原生 get_factor_range：阈值排序后依序抽取整数。
func runeFactorAtCount(raw [][]json.RawMessage, count int) (int, error) {
	return runeFactorWithReader(raw, count, rand.Reader)
}

func runeFactorWithReader(raw [][]json.RawMessage, count int, entropy io.Reader) (int, error) {
	type factorRange struct {
		threshold int
		bounds    []int
	}
	ranges := make([]factorRange, 0, len(raw))
	for _, row := range raw {
		var item factorRange
		if len(row) != 2 || json.Unmarshal(row[0], &item.threshold) != nil || json.Unmarshal(row[1], &item.bounds) != nil || len(item.bounds) == 0 {
			return 0, errors.New("契印权重范围表无效")
		}
		ranges = append(ranges, item)
	}
	sort.SliceStable(ranges, func(i, j int) bool { return ranges[i].threshold < ranges[j].threshold })
	factor := 1
	for _, row := range ranges {
		if row.threshold > count {
			break
		}
		low, high := row.bounds[0], row.bounds[0]
		for _, bound := range row.bounds {
			if bound < low {
				low = bound
			}
			if bound > high {
				high = bound
			}
		}
		span := new(big.Int).Sub(big.NewInt(int64(high)), big.NewInt(int64(low)))
		span.Add(span, big.NewInt(1))
		n, err := rand.Int(entropy, span)
		if err != nil {
			return 0, err
		}
		n.Add(n, big.NewInt(int64(low)))
		factor = int(n.Int64())
	}
	if factor <= 0 {
		factor = 1
	}
	return factor, nil
}
