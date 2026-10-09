package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"strings"
)

//go:embed card_return_catalog.json
var cardReturnCatalogRaw []byte

var androidCardReturn = func() struct {
	Cards map[int]struct {
		Materials     [][]int `json:"materials"`
		Allowed       bool    `json:"allowed"`
		BattleAllowed bool    `json:"battle_allowed"`
		Rarity        int     `json:"rarity"`
	} `json:"cards"`
	Levels map[int]struct {
		Exp    int64   `json:"exp"`
		Factor float64 `json:"factor"`
	} `json:"levels"`
} {
	var rules struct {
		Cards map[int]struct {
			Materials     [][]int `json:"materials"`
			Allowed       bool    `json:"allowed"`
			BattleAllowed bool    `json:"battle_allowed"`
			Rarity        int     `json:"rarity"`
		} `json:"cards"`
		Levels map[int]struct {
			Exp    int64   `json:"exp"`
			Factor float64 `json:"factor"`
		} `json:"levels"`
	}
	if json.Unmarshal(cardReturnCatalogRaw, &rules) != nil || len(rules.Cards) != 109 || len(rules.Levels) != 70 {
		panic("Android归还幻书目录无效")
	}
	return rules
}()

func (s *Service) decomposeCardsRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("归还幻书需要回调和UUID列表")
	}
	cb, ok := callbackArg(args)
	if !ok {
		return nil, errors.New("归还幻书回调无效")
	}
	var ids []string
	if json.Unmarshal(args[1], &ids) != nil || len(ids) == 0 || len(ids) > 2000 {
		return nil, errors.New("归还幻书列表无效")
	}
	gains := map[int]int64{}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		remove := map[string]bool{}
		for _, id := range ids {
			id = strings.ToLower(id)
			if !validObjectID(id) || remove[id] {
				return runeReject("RET_FAILED", "归还UUID无效或重复")
			}
			_, card := findCard(p, id)
			if card == nil {
				return runeReject("RET_CARD_NOT_EXIST", "幻书不属于当前角色")
			}
			rule, known := androidCardReturn.Cards[card.CardID]
			if !known || !rule.Allowed || len(rule.Materials) == 0 {
				return runeReject("RET_CARD_FORBID_GROWUP", "此幻书不可归还")
			}
			if card.Lock != 0 {
				return runeReject("RET_CARD_LOCK", "幻书已锁定")
			}
			if containsString(p.AsyncPvp.Defence, id) {
				return runeReject("RET_CARD_IN_DEFENCE_CAN_NOT_SELECT", "幻书在竞技防守阵容中")
			}
			if p.RemainingGameplay != nil && p.RemainingGameplay.Consign != nil {
				for _, task := range p.RemainingGameplay.Consign.Tasks {
					if task != nil && task.StartTime > 0 && containsInt(task.Cards, card.CardID) {
						return runeReject("RET_CARD_IN_CONSIGN_CAN_NOT_SELECT", "幻书正在委托")
					}
				}
			}
			if p.Battle != nil && !p.Battle.Finished && containsString(p.Battle.Team, id) {
				return runeReject("RET_FAILED", "幻书在进行中的战斗阵容中")
			}
			if card.EnhanceCount < 0 || card.EnhanceCount > 64 || card.Level < 1 || card.Level > 70 || card.Exp < 0 {
				return errors.New("归还幻书成长存档无效")
			}
			// 原生get_decompose_materials按本体表返回固定材料，不按补完次数倍增。
			for _, item := range rule.Materials {
				if len(item) != 2 || item[0] <= 0 || item[1] <= 0 {
					return errors.New("归还材料表无效")
				}
				gains[item[0]] += int64(item[1])
			}
			levelRule := androidCardReturn.Levels[card.Level]
			if levelRule.Factor <= 0 {
				return errors.New("归还经验系数无效")
			}
			exp := float64(card.Exp) * levelRule.Factor
			for level := 1; level < card.Level; level++ {
				row := androidCardReturn.Levels[level]
				exp += float64(row.Exp) * row.Factor
			}
			if exp < 0 || exp >= float64(math.MaxInt64) {
				return errors.New("归还经验溢出")
			}
			if int64(exp) > 0 {
				gains[4] += int64(exp)
			}
			remove[id] = true
		}
		leftBattle := 0
		for _, card := range p.Cards {
			if !remove[strings.ToLower(card.UUID)] && androidCardReturn.Cards[card.CardID].BattleAllowed {
				leftBattle++
			}
		}
		if leftBattle == 0 {
			return runeReject("RET_CARD_LEFT_ONE", "至少保留一本可战斗幻书")
		}
		if p.Materials == nil {
			p.Materials = map[int]Material{}
		}
		for id, n := range gains {
			m := p.Materials[id]
			if n < 0 || m.Count > math.MaxInt64-n || m.Total > math.MaxInt64-n {
				return errors.New("归还资产溢出")
			}
			m.ID = id
			m.Count += n
			m.Total += n
			p.Materials[id] = m
		}
		ensureObtainedCardHistory(p)
		kept := make([]Card, 0, len(p.Cards)-len(remove))
		for _, card := range p.Cards {
			if !remove[strings.ToLower(card.UUID)] {
				kept = append(kept, card)
			}
		}
		p.Cards = kept
		for id, r := range p.Runes {
			if remove[strings.ToLower(r.CardUUID)] {
				r.CardUUID = ""
				p.Runes[id] = r
			}
		}
		if remove[strings.ToLower(p.AssistCardUUID)] {
			p.AssistCardUUID = ""
		}
		// 阵容使用原槽位，删除UUID只留空，避免后续卡牌挤位。
		for i, id := range p.Lineup {
			if remove[strings.ToLower(id)] {
				p.Lineup[i] = ""
			}
		}
		clean := func(slots []string) {
			for i, id := range slots {
				if remove[strings.ToLower(id)] {
					slots[i] = ""
				}
			}
		}
		clean(p.ShowCards)
		for id, layout := range p.BattleLayouts {
			clean(layout.Fighting)
			clean(layout.Support)
			p.BattleLayouts[id] = layout
		}
		for id, record := range p.PresetCardsRecord {
			clean(record.Cards.Fighting)
			clean(record.Cards.Support)
			p.PresetCardsRecord[id] = record
		}
		if remove[strings.ToLower(p.CaptainCardUUID)] {
			p.CaptainCardUUID = ""
		}
		return nil
	})
	if err != nil {
		code := 1
		var refusal *runeBusinessError
		if errors.As(err, &refusal) {
			if catalog, e := loadIntimacyCatalog(); e == nil {
				if value, exists := catalog.Errors[refusal.Name]; exists {
					code = value
				}
			}
		}
		return []Push{nativeErrorPush(code), Callback(cb, []any{map[string]any{"__custom_type": "bonus.bonus", "contain_items": map[int]int64{}}})}, nil
	}
	return []Push{cardMgrPush(c), runePush(c), materialManagerPush(c), knowledgePush(c), push("Avatar", "client_prop_changed", []any{"assist_card_uuid", c.SelectedAvatarUnsafe().Progress.AssistCardUUID}), Callback(cb, []any{map[string]any{"__custom_type": "bonus.bonus", "contain_items": gains}})}, nil
}
