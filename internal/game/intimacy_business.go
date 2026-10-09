package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
)

//go:embed intimacy_catalog.json
var intimacyCatalogJSON []byte

type IntimacyCommon struct {
	RingID       int  `json:"ring_id"`
	RewardLevel  int  `json:"reward_intimacy_level"`
	LikeGift     bool `json:"like_gift"`
	SpecialCount int  `json:"special_gift_count"`
}

type intimacyCatalog struct {
	Cards map[int]struct {
		Bonuses    []int `json:"bonuses"`
		Rarity     int   `json:"rarity"`
		Disabled   int   `json:"disabled"`
		NotInStat  int   `json:"not_in_stat"`
		FavoredTag *int  `json:"favored_tag"`
	} `json:"cards"`
	Levels map[int]struct {
		Min     int  `json:"min_value"`
		Max     *int `json:"max_value"`
		Support int  `json:"support_skill_level"`
	} `json:"levels"`
	Bonuses map[int]struct {
		Fixed       [][]int           `json:"fixed_items"`
		Inner       json.RawMessage   `json:"inner_bonus_id"`
		RandomItems []json.RawMessage `json:"random_items"`
		RandomRunes []json.RawMessage `json:"random_runes"`
		RandomLibs  []json.RawMessage `json:"random_item_libs"`
	} `json:"bonuses"`
	Materials map[int]struct {
		Type   int `json:"type"`
		Target int `json:"target_id"`
	} `json:"materials"`
	Heads map[int]struct {
		LimitDays float64 `json:"limit_days"`
	} `json:"head_boxes"`
	Fragments     map[int]int    `json:"card_to_fragment"`
	Constants     map[string]int `json:"constants"`
	Errors        map[string]int `json:"errors"`
	SupportHidden int            `json:"support_skill_hidden"`
	MaxIntimacy   int            `json:"max_intimacy"`
	Gifts         map[int]struct {
		Intimacy              int         `json:"intimacy"`
		FavoredIntimacy       int         `json:"favored_intimacy"`
		LikeTag               *int        `json:"like_tag"`
		ForbidSpecial         int         `json:"forbid_special_gift"`
		SpecialAmounts        map[int]int `json:"special_amounts"`
		SpecialFavoredAmounts map[int]int `json:"special_favored_amounts"`
		ReturnRunes           []struct {
			Spec  RuneSpec `json:"spec"`
			Marks []string `json:"marks"`
		} `json:"return_runes"`
	} `json:"gifts"`
	SpecialGiftRules map[int]struct {
		ChoiceParams    map[int]float64 `json:"choice_params"`
		SingleCardTimes int             `json:"single_card_times"`
		TotalTimes      int             `json:"total_times"`
	} `json:"special_gift_rule"`
	SpecialPresents map[int]struct {
		Chats []int `json:"character_chat_id"`
	} `json:"special_presents"`
}

var intimacyOnce sync.Once
var intimacyRules intimacyCatalog
var intimacyLoadError error

func loadIntimacyCatalog() (intimacyCatalog, error) {
	intimacyOnce.Do(func() { intimacyLoadError = json.Unmarshal(intimacyCatalogJSON, &intimacyRules) })
	return intimacyRules, intimacyLoadError
}

type intimacyBusinessError struct{ name, message string }

func (e *intimacyBusinessError) Error() string  { return e.message }
func intimacyReject(name, message string) error { return &intimacyBusinessError{name, message} }

func intimacyLevel(value int, rules intimacyCatalog) int {
	level := 0
	for id, row := range rules.Levels {
		if value >= row.Min && (row.Max == nil || value <= *row.Max) && id > level {
			level = id
		}
	}
	return level
}

func autoIntimacyClaim(cardID int, rules intimacyCatalog) bool {
	return rules.Cards[cardID].Rarity == rules.Constants["RARITY_N"] || rules.SupportHidden != 0
}

// claimIntimacyChapter 只在玩家行锁事务内调用；callback ID 不是持久幂等凭据。
func claimIntimacyChapter(p *Progress, cardID int, rules intimacyCatalog) error {
	card, exists := rules.Cards[cardID]
	if !exists || !ownsCardID(*p, cardID) {
		return intimacyReject("RET_CARD_NOT_EXIST", "幻书不存在")
	}
	common := p.IntimacyCommons[cardID]
	current := intimacyLevel(p.Intimacy[cardID], rules)
	if common.RewardLevel < 0 || common.RewardLevel > len(card.Bonuses) {
		return errors.New("好感度章节存档无效")
	}
	if autoIntimacyClaim(cardID, rules) || common.RewardLevel >= current || common.RewardLevel >= len(card.Bonuses) {
		return intimacyReject("RET_CARD_NO_INTIMACY_BONUS", "没有可领取的好感度章节")
	}
	next := common.RewardLevel + 1
	bonus, ok := rules.Bonuses[card.Bonuses[next-1]]
	if !ok || len(bonus.RandomItems)+len(bonus.RandomRunes)+len(bonus.RandomLibs) != 0 || (len(bonus.Inner) != 0 && string(bonus.Inner) != "null") {
		return errors.New("好感度章节奖励表缺失或类型未实现")
	}
	changes := map[int]int64{}
	heads := map[int]bool{}
	level, exists := rules.Levels[next]
	if !exists {
		return errors.New("好感度章节等级表缺失")
	}
	for _, row := range bonus.Fixed {
		if len(row) != 2 || row[0] <= 0 || row[1] <= 0 {
			return errors.New("好感度章节奖励条目无效")
		}
		id := row[0]
		mat, exists := rules.Materials[id]
		if !exists {
			return errors.New("好感度奖励材料不存在")
		}
		if mat.Type == rules.Constants["MATERIAL_TYPE_SPECIAL_FRAGE"] {
			id = rules.Fragments[cardID]
			if id == 0 {
				id = cardID
			} // 原生girl_memory为缺失映射回退card_id。
			if _, ok := rules.Materials[id]; !ok {
				return errors.New("幻书残页材料缺失")
			}
		} else if mat.Type == rules.Constants["MATERIAL_TYPE_HEADBOX"] {
			head, exists := rules.Heads[mat.Target]
			if !exists || mat.Target <= 0 || head.LimitDays != 0 {
				return errors.New("好感度头像框配置无效")
			}
			heads[mat.Target] = true
			continue
		}
		if changes[id] > math.MaxInt64-int64(row[1]) {
			return errors.New("好感度奖励数量溢出")
		}
		changes[id] += int64(row[1])
	}
	// 先验证全部资产，随后写入；事务仍保证持久化失败整体回滚。
	for id, amount := range changes {
		m := p.Materials[id]
		if m.Count < 0 || m.Total < 0 || m.Count > math.MaxInt64-amount || m.Total > math.MaxInt64-amount {
			return errors.New("好感度奖励超过材料容量")
		}
	}
	if p.Materials == nil {
		p.Materials = map[int]Material{}
	}
	for id, amount := range changes {
		m := p.Materials[id]
		m.ID = id
		m.Count += amount
		m.Total += amount
		p.Materials[id] = m
	}
	if len(heads) != 0 && p.OwnedHeadBox == nil {
		p.OwnedHeadBox = map[int]float64{}
	}
	for id := range heads {
		p.OwnedHeadBox[id] = 0
	}
	common.RewardLevel = next
	if p.IntimacyCommons == nil {
		p.IntimacyCommons = map[int]IntimacyCommon{}
	}
	p.IntimacyCommons[cardID] = common
	for i := range p.Cards {
		if p.Cards[i].CardID == cardID && p.Cards[i].SupportSkillLevel < level.Support {
			p.Cards[i].SupportSkillLevel = level.Support
		}
	}
	return nil
}

func (s *Service) receiveIntimacyBonus(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("好感度领取参数无效")
	}
	var callback, cardID int
	if json.Unmarshal(args[0], &callback) != nil || json.Unmarshal(args[1], &cardID) != nil || string(args[1]) == "null" || cardID <= 0 {
		return nil, errors.New("好感度领取参数无效")
	}
	rules, err := loadIntimacyCatalog()
	if err != nil {
		return nil, err
	}
	err = s.updateProgress(ctx, c, func(p *Progress) error { return claimIntimacyChapter(p, cardID, rules) })
	if err != nil {
		var reject *intimacyBusinessError
		if errors.As(err, &reject) {
			code, exists := rules.Errors[reject.name]
			if !exists {
				return nil, fmt.Errorf("好感度错误码缺失：%s", reject.name)
			}
			return []Push{Callback(callback, []any{code})}, nil
		}
		return nil, err
	}
	p := c.SelectedAvatarUnsafe().Progress
	heads := p.OwnedHeadBox
	if heads == nil {
		heads = map[int]float64{}
	}
	return []Push{
		push("Avatar", "client_prop_changed", []any{"card_common_mgr", cardCommonMgrPropertiesWithProgress(p)}),
		cardMgrPush(c), materialManagerPush(c),
		push("Avatar", "client_prop_changed", []any{"owned_head_box", heads}),
		Callback(callback, []any{0}),
	}, nil
}
