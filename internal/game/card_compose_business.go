package game

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"math/big"
)

//go:embed card_compose_catalog.json
var cardComposeCatalogRaw []byte

type composeCandidate struct {
	CardID int   `json:"card_id"`
	Weight int64 `json:"weight"`
}
type composeRecipe struct {
	Need              int64              `json:"need"`
	Cards             []composeCandidate `json:"cards"`
	UnverifiedPromise bool               `json:"unverified_promise"`
}

var androidCompose = func() struct {
	Recipes    map[int]composeRecipe `json:"recipes"`
	MaxCards   int                   `json:"max_cards"`
	MaxCompose int                   `json:"max_compose"`
} {
	var c struct {
		Recipes    map[int]composeRecipe `json:"recipes"`
		MaxCards   int                   `json:"max_cards"`
		MaxCompose int                   `json:"max_compose"`
	}
	if json.Unmarshal(cardComposeCatalogRaw, &c) != nil || len(c.Recipes) == 0 || c.MaxCards <= 0 || c.MaxCompose <= 0 {
		panic("Android碎片合成目录无效")
	}
	return c
}()

func (s *Service) composeCardsRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("合成幻书需要回调与碎片编号列表")
	}
	cb, ok := callbackArg(args)
	if !ok {
		return nil, errors.New("合成回调无效")
	}
	var materials intList
	if json.Unmarshal(args[1], &materials) != nil {
		return nil, errors.New("合成碎片列表无效")
	}
	var obtained []string
	first := []int{}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if len(materials) == 0 {
			return runeReject("RET_CARD_FRAGMENT_NOT_CHOOSE", "未选择碎片")
		}
		if len(materials) > len(androidCompose.Recipes) {
			return errors.New("合成材料列表过长")
		}
		remaining := androidCompose.MaxCards - len(p.Cards)
		if remaining <= 0 {
			return runeReject("RET_CARD_COUNT_REACH_MAX", "幻书背包已满")
		}
		if remaining > androidCompose.MaxCompose {
			remaining = androidCompose.MaxCompose
		}
		seen := map[int]bool{}
		known := map[int]bool{}
		ensureObtainedCardHistory(p)
		for id := range p.ObtainedCardIDs {
			known[id] = true
		}
		type plan struct {
			id     int
			recipe composeRecipe
			count  int64
		}
		plans := []plan{}
		for _, mid := range materials {
			if seen[mid] {
				return errors.New("合成材料编号重复")
			}
			seen[mid] = true
			r, ok := androidCompose.Recipes[mid]
			if !ok {
				return runeReject("RET_CARD_FRAGMENT_NOT_EXIST", "不是可合成的幻书碎片")
			}
			if r.UnverifiedPromise {
				return errors.New("此随机碎片的等级保底语义尚未取证，拒绝扣料")
			}
			if r.Need <= 0 || len(r.Cards) == 0 {
				return errors.New("合成配方无效")
			}
			n := p.Materials[mid].Count / r.Need
			if n <= 0 {
				return runeReject("RET_CARD_FRAGMENT_NOT_ENOUGH", "碎片不足")
			}
			if n > int64(remaining) {
				n = int64(remaining)
			}
			remaining -= int(n)
			plans = append(plans, plan{mid, r, n})
		}
		for _, plan := range plans {
			var total int64
			for _, entry := range plan.recipe.Cards {
				if entry.Weight <= 0 || entry.CardID <= 0 {
					return errors.New("合成权重无效")
				}
				if total > math.MaxInt64-entry.Weight {
					return errors.New("合成权重溢出")
				}
				total += entry.Weight
			}
			for n := int64(0); n < plan.count; n++ {
				pick, err := rand.Int(rand.Reader, big.NewInt(total))
				if err != nil {
					return err
				}
				value := pick.Int64()
				cid := 0
				for _, entry := range plan.recipe.Cards {
					if value < entry.Weight {
						cid = entry.CardID
						break
					}
					value -= entry.Weight
				}
				card := newCard(cid, 1, s.Now())
				appendOwnedCard(p, card)
				obtained = append(obtained, card.UUID)
				if !known[cid] {
					first = append(first, cid)
					known[cid] = true
				}
			}
			m := p.Materials[plan.id]
			m.Count -= plan.count * plan.recipe.Need
			p.Materials[plan.id] = m
		}
		return nil
	})
	if err != nil {
		var refusal *runeBusinessError
		if errors.As(err, &refusal) {
			rules, e := loadIntimacyCatalog()
			if e != nil {
				return nil, e
			}
			code, ok := rules.Errors[refusal.Name]
			if !ok {
				return nil, errors.New("合成错误码缺失")
			}
			return []Push{Callback(cb, []any{code, []any{}, []int{}})}, nil
		}
		return nil, err
	}
	p := c.SelectedAvatarUnsafe().Progress
	cards := append(cardListWire(&p, obtained), "card.card_list", "__custom_type")
	return []Push{materialManagerPush(c), cardMgrPush(c), Callback(cb, []any{RetSuccess, cards, first})}, nil
}
