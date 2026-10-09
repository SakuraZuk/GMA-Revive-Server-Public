package game

import "sort"

// 原生角色界面直接索引cards_dress[card.dress]，旧存档0必须投影为原默认装帧。
func effectiveCardDress(card Card) int {
	if card.Dress != 0 {
		return card.Dress
	}
	return androidCardAppearances[card.CardID].DefaultDress
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func ownsCardID(p Progress, cardID int) bool {
	for _, card := range p.Cards {
		if card.CardID == cardID {
			return true
		}
	}
	return false
}

func cardAwakened(p Progress, cardID int) bool {
	for _, card := range p.Cards {
		if card.CardID == cardID && card.Awakened > 0 {
			return true
		}
	}
	return false
}

// 默认外观、觉醒解锁外观和已存外观归属分开处理，不自动解锁付费外观。
func ownedCardDresses(p Progress, cardID int) []int {
	rule, ok := androidCardAppearances[cardID]
	owned := []int{}
	if !ok {
		return owned
	}
	add := func(id int) {
		if id > 0 && containsInt(rule.Dresses, id) && !containsInt(owned, id) {
			owned = append(owned, id)
		}
	}
	add(rule.DefaultDress)
	if cardAwakened(p, cardID) {
		add(rule.AwakenedDress)
	}
	for _, id := range p.OwnedDresses[cardID] {
		add(id)
	}
	// 旧存档的实际穿着是已有归属证据；未知或跨卡外观不会迁移。
	for _, card := range p.Cards {
		if card.CardID == cardID {
			add(card.Dress)
		}
	}
	sort.Ints(owned)
	return owned
}

func validOwnedDress(p Progress, cardID, dress int) bool {
	needsAwakened, exists := androidDressNeedsAwakened[dress]
	return exists && containsInt(ownedCardDresses(p, cardID), dress) && (!needsAwakened || cardAwakened(p, cardID))
}

func captainDress(p Progress, cardID int) int {
	if dress := p.CaptainDresses[cardID]; validOwnedDress(p, cardID, dress) {
		return dress
	}
	return androidCardAppearances[cardID].DefaultDress
}

// 旧版本保存 UUID：登录按实际拥有卡映射，成功设置时写新字段并清旧 UUID。
func effectiveCaptainID(p Progress) int {
	id := p.CaptainID
	if id == 0 && p.CaptainCardUUID != "" {
		for _, card := range p.Cards {
			if card.UUID == p.CaptainCardUUID {
				id = card.CardID
				break
			}
		}
	}
	rule, exists := androidCardAppearances[id]
	if exists && ownsCardID(p, id) && !containsInt(rule.Forbid, androidCaptainForbidden) {
		return id
	}
	return 0
}

func cardCommonMgrPropertiesWithProgress(p Progress) map[string]any {
	mgr := cardCommonMgrProperties(p.Cards)
	for _, value := range mgr {
		common := value.(map[string]any)
		id := common["card_id"].(int)
		common["card_vo"] = p.CardVoices[id]
		common["captain_dress"] = captainDress(p, id)
		common["house_dress_id"] = collectionDress(p, id)
		common["owned_dresses"] = ownedCardDresses(p, id)
		common["intimacy"] = p.Intimacy[id]
		state := p.IntimacyCommons[id]
		common["reward_intimacy_level"] = state.RewardLevel
		common["like_gift"] = state.LikeGift
		common["special_gift_count"] = state.SpecialCount
		common["ring_id"] = state.RingID
		if cardAwakened(p, id) {
			common["awakened"] = 1
		}
	}
	return mgr
}
