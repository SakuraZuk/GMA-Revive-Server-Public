package game

import (
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
)

//go:embed card_growth_catalog.json
var cardGrowthCatalogRaw []byte

var androidCardGrowth = func() struct {
	Cards map[int]struct {
		Rarity int   `json:"rarity"`
		Forbid []int `json:"forbid"`
	} `json:"cards"`
	Rarities map[int]struct {
		Sort   [][]int `json:"enhance_sort"`
		Common []int   `json:"common_enhance_card"`
		Cost   []int64 `json:"consume_materials"`
	} `json:"rarities"`
	Enhancements map[int]struct {
		Skill int `json:"enhance_skill"`
	} `json:"enhancements"`
} {
	var out struct {
		Cards map[int]struct {
			Rarity int   `json:"rarity"`
			Forbid []int `json:"forbid"`
		} `json:"cards"`
		Rarities map[int]struct {
			Sort   [][]int `json:"enhance_sort"`
			Common []int   `json:"common_enhance_card"`
			Cost   []int64 `json:"consume_materials"`
		} `json:"rarities"`
		Enhancements map[int]struct {
			Skill int `json:"enhance_skill"`
		} `json:"enhancements"`
	}
	if json.Unmarshal(cardGrowthCatalogRaw, &out) != nil || len(out.Rarities) != 5 || len(out.Enhancements) != 16 {
		panic("Android补完目录无效")
	}
	return out
}()

func nativeCardEnhanceIDs(card Card) []int {
	rule := androidCardGrowth.Rarities[androidCardGrowth.Cards[card.CardID].Rarity]
	ids := []int{}
	for i, row := range rule.Sort {
		if i >= card.EnhanceCount {
			break
		}
		ids = append(ids, row...)
	}
	return ids
}

func enhanceCardNative(p *Progress, uuid string, materials []string, now time.Time) (map[string]any, error) {
	_, target := findCard(p, uuid)
	if err := checkCardGrowth(target); err != nil {
		return nil, err
	}
	rarity := androidCardGrowth.Cards[target.CardID].Rarity
	rule, known := androidCardGrowth.Rarities[rarity]
	if !known || len(rule.Sort) == 0 {
		return nil, runeReject("RET_CARD_CAN_NOT_ENHANCE_SKILL_LIMIT", "幻书没有补完阶段")
	}
	if target.EnhanceCount < 0 {
		return nil, errors.New("幻书补完存档无效")
	}
	if target.EnhanceCount >= len(rule.Sort) {
		return nil, runeReject("RET_CARD_ENHANCE_FINISH", "幻书已完成全部补完")
	}
	if len(materials) == 0 {
		return nil, runeReject("RET_CARD_SAME_CARD_EMPTY", "补完材料为空")
	}
	remove := map[string]bool{}
	added := 0
	for _, id := range materials {
		key := strings.ToLower(id)
		if key == strings.ToLower(uuid) || remove[key] {
			return nil, runeReject("RET_CARD_SAME_CARD_REPEAT", "补完材料重复或包含目标")
		}
		_, material := findCard(p, id)
		if material == nil {
			return nil, runeReject("RET_CARD_NOT_EXIST", "补完材料幻书不存在")
		}
		if material.Lock != 0 {
			return nil, runeReject("RET_CARD_LOCKED_CAN_NOT_SELECT", "补完材料幻书已锁定")
		}
		contribution := 1 + material.EnhanceCount
		common := false
		for _, r := range androidCardGrowth.Rarities {
			if len(r.Common) == 2 && r.Common[0] == material.CardID {
				common = true
			}
		}
		if common {
			if len(rule.Common) != 2 || rule.Common[0] != material.CardID || rule.Common[1] <= 0 {
				return nil, runeReject("RET_CARD_NOT_SAME_CARD", "通用补完材料不适用此稀有度")
			}
			contribution = rule.Common[1]
		} else if material.CardID != target.CardID {
			return nil, runeReject("RET_CARD_NOT_SAME_CARD", "补完必须使用同名或原生通用幻书")
		}
		if contribution <= 0 || contribution > len(rule.Sort)-target.EnhanceCount-added {
			return nil, runeReject("RET_CARD_ENHANCE_FINISH", "材料补完阶段超出剩余阶段")
		}
		added += contribution
		remove[key] = true
	}
	if len(rule.Cost) != 2 || rule.Cost[0] <= 0 || rule.Cost[1] <= 0 || int64(added) > math.MaxInt64/rule.Cost[1] {
		return nil, errors.New("补完消耗目录无效")
	}
	mid := int(rule.Cost[0])
	need := rule.Cost[1] * int64(added)
	m := p.Materials[mid]
	if m.Count < need {
		return nil, runeReject("RET_MATERIAL_NOT_ENOUGH", "补完材料不足")
	}
	for _, row := range rule.Sort[target.EnhanceCount : target.EnhanceCount+added] {
		for _, id := range row {
			if androidCardGrowth.Enhancements[id].Skill != 0 {
				target.SkillEnhanceCount++
			}
		}
	}
	target.EnhanceCount += added
	m.Count -= need
	p.Materials[mid] = m
	filtered := make([]Card, 0, len(p.Cards)-len(remove))
	for _, card := range p.Cards {
		if !remove[strings.ToLower(card.UUID)] {
			filtered = append(filtered, card)
		} else {
			for rid, r := range p.Runes {
				if r.CardUUID == card.UUID {
					r.CardUUID = ""
					p.Runes[rid] = r
				}
			}
		}
	}
	p.Cards = filtered
	return map[string]any{"__custom_type": "box.box", "materials": map[int]int64{}}, nil
}
