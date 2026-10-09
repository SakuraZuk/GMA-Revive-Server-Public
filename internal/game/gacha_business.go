package game

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/big"
	"strings"
)

// handleGachaContract records the client-side contract boundary without
// inventing Android probabilities. The catalog is deliberately a deployment
// dependency; until it is installed, the caller receives a clear failure.
func (s *Service) handleGachaContract(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 3 {
		return nil, errors.New("抽卡参数需要callback_id、pool_id、count")
	}
	var callbackID, poolID, count int
	if json.Unmarshal(args[0], &callbackID) != nil || json.Unmarshal(args[1], &poolID) != nil || json.Unmarshal(args[2], &count) != nil {
		return nil, errors.New("抽卡参数必须是整数")
	}
	if callbackID < 0 || poolID <= 0 || (count != 1 && count != 10) {
		return drawFailureCallback(callbackID, 1), nil
	}
	cfg, ok := androidGachaPools[poolID]
	if !ok || len(cfg.Cards) == 0 {
		log.Printf("抽卡拒绝 uid=%d pool=%d count=%d 原因=卡池未配置", c.SelectedAvatarUnsafe().UID, poolID, count)
		return drawFailureCallback(callbackID, 28007), nil
	}
	var cards []Card
	var firstIDs []int
	bonus := map[string]any{"__custom_type": "box.box", "materials": map[string]any{}}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if pool, exists := androidNativeDrawPools[poolID]; exists && pool.Activity > 0 {
			if _, err := activityOpen(*p, pool.Activity, p.AvatarLevel, s.Now()); err != nil {
				return runeReject("RET_CARD_POOL_UPDATED", "抽卡活动未开放或未解锁")
			}
		}
		if len(p.Cards) > 2000-count {
			return runeReject("RET_CARD_COUNT_REACH_MAX", "幻书背包容量不足")
		}
		if p.RandomCardsRecord == nil {
			p.RandomCardsRecord = map[int]RandomCardRecord{}
		}
		if p.PromiseCardsRecord == nil {
			p.PromiseCardsRecord = map[int]PromiseCardRecord{}
		}
		record := p.RandomCardsRecord[poolID]
		if record.PoolID == 0 {
			record.PoolID = poolID
		}
		if cfg.LimitCount > 0 && record.RandomCount+count > cfg.LimitCount {
			return runeReject("RET_CARD_POOL_UPDATED", "抽卡已达到卡池次数上限")
		}
		mat, exists := p.Materials[cfg.CostMaterial]
		cost := int64(cfg.CostCount * count)
		if !exists || mat.Count < cost {
			return runeReject("RET_MATERIAL_NOT_ENOUGH", "抽卡材料不足")
		}
		first := poolID == 1 && record.RandomCount == 0 && count == 1
		for i := 0; i < count; i++ {
			force := 0
			minimumRarity := 0
			if first {
				force = androidFirstDrawCardID
			}
			if count == 10 && i == count-1 {
				found := false
				for _, c := range cards {
					if gachaCardRarity(cfg, c.CardID) >= cfg.GuaranteeRarity {
						found = true
						break
					}
				}
				if !found {
					minimumRarity = cfg.GuaranteeRarity
				}
			}
			cid, err := chooseGachaCardWithRarity(cfg, record.RandomCount+i, force, minimumRarity)
			if err != nil {
				return err
			}
			card := newCard(cid, 1, s.Now())
			cards = append(cards, card)
			firstObtained := appendOwnedCard(p, card)
			advanceAchievementEvent(p, 9, gachaCardRarity(cfg, cid), s.Now())
			if first || firstObtained {
				firstIDs = append(firstIDs, cid)
			}
		}
		mat.Count -= cost
		mat.Total += 0
		p.Materials[cfg.CostMaterial] = mat
		record.RandomCount += count
		p.RandomCardsRecord[poolID] = record
		if first {
			bonus["materials"] = map[string]any{fmt.Sprint(androidFirstDrawBonusMaterial): androidFirstDrawBonusCount}
			rule := p.PromiseCardsRecord[androidFirstDrawRuleID]
			rule.RuleID = androidFirstDrawRuleID
			rule.Count++
			rule.FinishCount++
			p.PromiseCardsRecord[androidFirstDrawRuleID] = rule
		}
		if first {
			if p.Materials[androidFirstDrawBonusMaterial].ID == 0 {
				p.Materials[androidFirstDrawBonusMaterial] = Material{ID: androidFirstDrawBonusMaterial}
			}
			bonusMat := p.Materials[androidFirstDrawBonusMaterial]
			if bonusMat.Count > math.MaxInt64-androidFirstDrawBonusCount || bonusMat.Total > math.MaxInt64-androidFirstDrawBonusCount {
				return errors.New("首抽奖励资产溢出")
			}
			bonusMat.Count += androidFirstDrawBonusCount
			bonusMat.Total += androidFirstDrawBonusCount
			p.Materials[androidFirstDrawBonusMaterial] = bonusMat
		}
		return nil
	})
	if err != nil {
		code := 1
		var refusal *runeBusinessError
		if errors.As(err, &refusal) {
			if rules, e := loadIntimacyCatalog(); e == nil {
				if value, exists := rules.Errors[refusal.Name]; exists {
					code = value
				}
			}
		}
		log.Printf("抽卡拒绝 uid=%d pool=%d count=%d 原因=%v", c.SelectedAvatarUnsafe().UID, poolID, count, err)
		return drawFailureCallback(callbackID, code), nil
	}
	cardValues := make([]any, 0, len(cards))
	for _, card := range cards {
		cardValues = append(cardValues, cardMgrProperties([]Card{card})[card.UUID])
	}
	// 回调不经过card_mgr属性转换，必须携带原生card_list标记才能得到card对象。
	cardValues = append(cardValues, "card.card_list", "__custom_type")
	return []Push{push("Avatar", "client_prop_changed", []any{"material_mgr", materialProperties(c.identity.Avatars, c.hostnum)}), cardMgrPush(c), Callback(callbackID, []any{0, cardValues, firstIDs, false, bonus})}, nil
}

func drawFailureCallback(callbackID, code int) []Push {
	return []Push{Callback(callbackID, []any{code, []any{"card.card_list", "__custom_type"}, []int{}, false, map[string]any{"__custom_type": "box.box", "materials": map[int]int64{}}})}
}

func chooseGachaCard(cfg gachaPoolConfig, count, forced int) (int, error) {
	return chooseGachaCardWithRarity(cfg, count, forced, 0)
}

func gachaCardRarity(cfg gachaPoolConfig, id int) int {
	for _, card := range cfg.Cards {
		if card.CardID == id {
			return card.Rarity
		}
	}
	return 0
}

func chooseGachaCardWithRarity(cfg gachaPoolConfig, count, forced, minimumRarity int) (int, error) {
	if forced > 0 {
		for _, c := range cfg.Cards {
			if c.CardID == forced {
				return forced, nil
			}
		}
		return 0, errors.New("抽卡保底卡不在卡池")
	}
	if pool, ok := androidNativeDrawPools[cfg.PoolID]; ok {
		weights := pool.Weights[count+1]
		if len(weights) == 0 {
			minimum := -1
			for step := range pool.Weights {
				if minimum < 0 || step < minimum {
					minimum = step
				}
			}
			weights = pool.Weights[minimum]
		}
		var total int64
		for cid, weight := range weights {
			if gachaCardRarity(cfg, cid) >= minimumRarity {
				if weight < 0 || total > math.MaxInt64-weight {
					return 0, errors.New("抽卡原生权重溢出")
				}
				total += weight
			}
		}
		if total <= 0 {
			return 0, errors.New("抽卡原生权重为空")
		}
		n, err := rand.Int(rand.Reader, big.NewInt(total))
		if err != nil {
			return 0, err
		}
		pick := n.Int64()
		for cid, weight := range weights {
			if gachaCardRarity(cfg, cid) >= minimumRarity {
				if pick < weight {
					return cid, nil
				}
				pick -= weight
			}
		}
		return 0, errors.New("抽卡原生权重选择失败")
	}
	weights := cfg.Weights[count+1]
	if len(weights) == 0 {
		weights = cfg.Weights[88]
	}
	if minimumRarity > 0 {
		groups := map[int]bool{}
		for _, card := range cfg.Cards {
			if card.Rarity >= minimumRarity {
				groups[card.Group] = true
			}
		}
		filtered := make([]gachaWeight, 0, len(weights))
		for _, w := range weights {
			if groups[w.Group] {
				filtered = append(filtered, w)
			}
		}
		weights = filtered
	}
	total := 0
	for _, w := range weights {
		total += w.Weight
	}
	if total <= 0 {
		return 0, errors.New("抽卡概率表为空")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(total)))
	if err != nil {
		return 0, err
	}
	pick := int(n.Int64())
	group := 0
	for _, w := range weights {
		if pick < w.Weight {
			group = w.Group
			break
		}
		pick -= w.Weight
	}
	var eligible []int
	for _, c := range cfg.Cards {
		if c.Group == group && c.Rarity > 0 && c.Rarity >= minimumRarity {
			eligible = append(eligible, c.CardID)
		}
	}
	if len(eligible) == 0 {
		return 0, fmt.Errorf("抽卡概率组 %d 无卡牌", group)
	}
	n, err = rand.Int(rand.Reader, big.NewInt(int64(len(eligible))))
	if err != nil {
		return 0, err
	}
	return eligible[n.Int64()], nil
}

func materialProperties(avatars []Avatar, hostnum int) map[string]any {
	for _, av := range avatars {
		if av.Hostnum == hostnum {
			out := map[string]any{}
			for id, m := range av.Progress.Materials {
				out[fmt.Sprint(id)] = map[string]any{"material_id": m.ID, "count": m.Count, "total": m.Total}
			}
			return out
		}
	}
	return map[string]any{}
}

func (s *Service) setAssistCard(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 1 {
		return nil, errors.New("助战卡参数无效")
	}
	var uuid string
	if err := json.Unmarshal(args[0], &uuid); err != nil || len(uuid) != 24 {
		return nil, errors.New("助战卡 uuid 无效")
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		for _, card := range p.Cards {
			if strings.EqualFold(card.UUID, uuid) {
				p.AssistCardUUID = card.UUID
				return nil
			}
		}
		return errors.New("助战卡不属于当前角色")
	})
	if err != nil {
		return nil, err
	}
	return []Push{push("Avatar", "client_prop_changed", []any{"assist_card_uuid", uuid}), push("Avatar", "call_client_callback", 0, []any{0})}, nil
}

func (s *Service) setDungeonSkipEditState(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 3 {
		return nil, errors.New("副本跳过编辑参数无效")
	}
	var callbackID, dungeonID int
	var locked bool
	if json.Unmarshal(args[0], &callbackID) != nil || callbackID <= 0 {
		return nil, errors.New("副本跳过编辑回调编号无效")
	}
	if json.Unmarshal(args[1], &dungeonID) != nil || json.Unmarshal(args[2], &locked) != nil || dungeonID <= 0 {
		return nil, errors.New("副本跳过编辑参数类型无效")
	}
	if _, known := dungeonCatalog[dungeonID]; !known {
		return []Push{Callback(callbackID, []any{false})}, nil
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if p.DungeonSkipEditState == nil {
			p.DungeonSkipEditState = map[int]bool{}
		}
		p.DungeonSkipEditState[dungeonID] = locked
		return nil
	})
	if err != nil {
		return []Push{Callback(callbackID, []any{false})}, nil
	}
	return []Push{Callback(callbackID, []any{true})}, nil
}
