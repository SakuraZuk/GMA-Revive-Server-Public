package game

import (
	_ "embed"
	"encoding/json"
	"errors"
)

//go:embed beginner_promise_catalog.json
var beginnerPromiseRaw []byte

type beginnerPromiseRule struct {
	ID    int   `json:"id"`
	At    int   `json:"at"`
	Cards []int `json:"cards"`
}

var beginnerPromiseRules = func() []beginnerPromiseRule {
	var doc struct {
		Rules []beginnerPromiseRule `json:"rules"`
	}
	if json.Unmarshal(beginnerPromiseRaw, &doc) != nil || len(doc.Rules) != 10 {
		panic("启程一次性保底目录无效")
	}
	for i, rule := range doc.Rules {
		if rule.ID != 1002+i || rule.At != i+1 || len(rule.Cards) == 0 {
			panic("启程保底节点无效")
		}
	}
	return doc.Rules
}()

func (r beginnerPromiseRule) matches(id int) bool {
	for _, candidate := range r.Cards {
		if candidate == id {
			return true
		}
	}
	return false
}

// 旧档没有逐抽来源：只接续尚未到期的节点，不把已过期的缺失记录伪造为已领取。
func prepareBeginnerPromises(p *Progress, drawn int) {
	for _, rule := range beginnerPromiseRules {
		if _, exists := p.PromiseCardsRecord[rule.ID]; !exists && drawn < rule.At {
			p.PromiseCardsRecord[rule.ID] = PromiseCardRecord{RuleID: rule.ID, Count: drawn}
		}
	}
}

func chooseBeginnerPromise(p *Progress, cfg gachaPoolConfig, drawn int) (int, error) {
	for _, rule := range beginnerPromiseRules {
		record, exists := p.PromiseCardsRecord[rule.ID]
		if !exists || record.FinishCount >= 1 || record.Count+1 < rule.At {
			continue
		}
		// 保留原生单卡权重，只过滤本节点指定的候选，不新增均匀抽样概率。
		filtered := cfg
		filtered.Cards = nil
		for _, card := range cfg.Cards {
			if rule.matches(card.CardID) {
				filtered.Cards = append(filtered.Cards, card)
			}
		}
		if len(filtered.Cards) == 0 {
			return 0, errors.New("启程保底候选为空")
		}
		return chooseGachaCardWithRarity(filtered, drawn, 0, 1)
	}
	return 0, nil
}

// 对最终入账卡刷新全部已知规则；先判保底再递增，单抽和十连共享持久计数。
func refreshBeginnerPromises(p *Progress, cardID int) {
	for _, rule := range beginnerPromiseRules {
		record, exists := p.PromiseCardsRecord[rule.ID]
		if !exists {
			continue
		}
		record.Count++
		if rule.matches(cardID) {
			record.FinishCount++
		}
		if record.FinishCount < 1 && record.Count >= rule.At {
			record.FinishCount = 1
		}
		p.PromiseCardsRecord[rule.ID] = record
	}
}
