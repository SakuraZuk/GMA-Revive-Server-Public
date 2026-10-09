package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
)

//go:embed card_oath_catalog.json
var cardOathCatalogRaw []byte

type oathLevel struct {
	Exp        int64     `json:"exp"`
	Conditions [][][]int `json:"conditions"`
}
type oathCatalog struct {
	Cards map[int]struct {
		Forbid  []int     `json:"forbid"`
		Upgrade [][][]int `json:"upgrade"`
	} `json:"cards"`
	Rings             map[int]int       `json:"rings"`
	Materials         map[int]int       `json:"materials"`
	Caps              map[int]int       `json:"caps"`
	Levels            map[int]oathLevel `json:"levels"`
	RingIntimacyLevel int               `json:"ring_intimacy_level"`
	MaxGrade          int               `json:"max_grade"`
	NormalMaxLevel    int               `json:"normal_max_level"`
}

var androidOath = func() oathCatalog {
	var rules oathCatalog
	if json.Unmarshal(cardOathCatalogRaw, &rules) != nil || len(rules.Levels) != 70 || rules.MaxGrade != 5 || rules.RingIntimacyLevel <= 0 {
		panic("Android誓约目录无效")
	}
	return rules
}()

func checkCardGrowth(card *Card) error {
	if card == nil {
		return runeReject("RET_CARD_NOT_EXIST", "幻书不存在")
	}
	rule, ok := androidOath.Cards[card.CardID]
	if !ok {
		return errors.New("幻书成长目录缺失")
	}
	if containsInt(rule.Forbid, 2) {
		return runeReject("RET_CARD_FORBID_GROWUP", "此幻书禁止成长")
	}
	return nil
}

func cardLevelCap(card *Card) (int, error) {
	cap, ok := androidOath.Caps[card.Grade]
	if !ok {
		return 0, errors.New("幻书品阶存档无效")
	}
	if card.Grade == androidOath.MaxGrade && card.IsUpdateLevelOne && androidOath.Rings[card.RingID] > 0 {
		cap = androidOath.Rings[card.RingID]
	}
	return cap, nil
}

func cardLevelUnlocked(level, avatarLevel int) bool {
	row, ok := androidOath.Levels[level]
	if !ok {
		return false
	}
	if len(row.Conditions) == 0 {
		return true
	}
	for _, group := range row.Conditions {
		if len(group) == 0 {
			continue
		}
		match := false
		for _, condition := range group {
			if len(condition) == 2 && condition[0] == 2 && avatarLevel >= condition[1] {
				match = true
			}
		}
		if !match {
			return false
		}
	}
	return true
}

func growthCallbackError(callback int, err error) ([]Push, error) {
	var reject *runeBusinessError
	if !errors.As(err, &reject) {
		return nil, err
	}
	rules, loadErr := loadIntimacyCatalog()
	if loadErr != nil {
		return nil, loadErr
	}
	code, ok := rules.Errors[reject.Name]
	if !ok {
		return nil, errors.New("幻书成长错误码缺失")
	}
	return []Push{Callback(callback, []any{code})}, nil
}

func knowledgePush(c *Connection) Push {
	return push("Avatar", "client_prop_changed", []any{"exp_pool", c.SelectedAvatarUnsafe().Progress.Materials[4].Count})
}

func (s *Service) cardOathRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("誓约操作需要玩家状态")
	}
	callback, ok := callbackArg(args)
	if !ok {
		return nil, errors.New("誓约回调无效")
	}
	if (method == "consume_ring" && len(args) != 3) || (method == "update_level_one" && len(args) != 2) {
		return nil, errors.New("誓约参数数量无效")
	}
	var cardID, materialID int
	var uuid string
	if method == "consume_ring" {
		if json.Unmarshal(args[1], &cardID) != nil || json.Unmarshal(args[2], &materialID) != nil || cardID <= 0 || materialID <= 0 {
			return nil, errors.New("誓约幻书或材料编号无效")
		}
	} else if json.Unmarshal(args[1], &uuid) != nil || uuid == "" {
		return nil, errors.New("突破幻书编号无效")
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if method == "consume_ring" {
			var card *Card
			for i := range p.Cards {
				if p.Cards[i].CardID == cardID {
					card = &p.Cards[i]
					break
				}
			}
			if err := checkCardGrowth(card); err != nil {
				return err
			}
			ringID := androidOath.Materials[materialID]
			if androidOath.Rings[ringID] == 0 {
				return runeReject("RET_MATERIAL_NOT_EXIST", "不是誓约材料")
			}
			common := p.IntimacyCommons[cardID]
			if common.RingID != 0 {
				return runeReject("RET_CARD_HAS_GET_RING", "幻书已接受誓约")
			}
			rules, err := loadIntimacyCatalog()
			if err != nil {
				return err
			}
			level := intimacyLevel(p.Intimacy[cardID], rules)
			rewardLevel := common.RewardLevel
			if autoIntimacyClaim(cardID, rules) {
				rewardLevel = level
			}
			if level < androidOath.RingIntimacyLevel || rewardLevel < androidOath.RingIntimacyLevel {
				return runeReject("RET_CARD_INTIMACY_LEVEL_NOT_ENOUGH", "好感度或已领取章节不足")
			}
			material := p.Materials[materialID]
			if material.Count < 1 {
				return runeReject("RET_MATERIAL_NOT_ENOUGH", "誓约材料不足")
			}
			material.Count--
			p.Materials[materialID] = material
			common.RingID = ringID
			if p.IntimacyCommons == nil {
				p.IntimacyCommons = map[int]IntimacyCommon{}
			}
			p.IntimacyCommons[cardID] = common
			return nil
		}
		_, card := findCard(p, uuid)
		if err := checkCardGrowth(card); err != nil {
			return err
		}
		if card.IsUpdateLevelOne {
			return runeReject("RET_CARD_HAS_UPDATE_LEVEL_ONE", "幻书已突破等级上限")
		}
		ringID := p.IntimacyCommons[card.CardID].RingID
		if androidOath.Rings[ringID] == 0 {
			return runeReject("RET_CARD_NOT_GIVE_RING", "幻书尚未接受誓约")
		}
		if card.Grade != androidOath.MaxGrade || card.Level != androidOath.NormalMaxLevel {
			return runeReject("RET_CARD_NOT_REACH_MAX_LEVEL", "幻书需达到普通满级满品阶")
		}
		card.IsUpdateLevelOne, card.RingID = true, ringID
		return nil
	})
	if err != nil {
		return growthCallbackError(callback, err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	return []Push{materialManagerPush(c), cardMgrPush(c), push("Avatar", "client_prop_changed", []any{"card_common_mgr", cardCommonMgrPropertiesWithProgress(p)}), Callback(callback, []any{RetSuccess})}, nil
}
