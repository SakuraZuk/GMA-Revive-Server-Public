package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"
)

// 抽奖与兑现分两次短事务：预演只保存来源收据，资产/进度/邮件在后续事务共同提交。
func (s *Service) freezeOrdinaryBattleRewardPlan(ctx context.Context, c *Connection, envelope *battleEnvelope) error {
	return s.updateProgress(ctx, c, func(p *Progress) error {
		b := p.Battle
		if b == nil || b.Finished || b.UUID != envelope.BattleUUID {
			return errors.New("奖励冻结战斗归属不符")
		}
		if b.RewardPlanFrozen {
			original, _ := json.Marshal(b.RewardResult)
			incoming, _ := json.Marshal(envelope)
			if string(original) != string(incoming) {
				return errors.New("冻结战斗结果不得变更")
			}
			return nil
		}
		// 活动包含独立的随机事件与状态转换，本次只冻结普通副本奖励。
		if b.ActivityContext != nil {
			return nil
		}
		scratch := CloneProgress(*p)
		trial := scratch.Battle
		if err := acceptBattleEvent(trial, envelope); err != nil {
			return err
		}
		winners, _ := battleWinnerEIDs(envelope.Data)
		player, _ := envelope.Data["player_eid"].(string)
		if player == "" {
			player = hexOf(selectedOID(c))
		}
		win := false
		for _, id := range winners {
			if id == player {
				win = true
			}
		}
		if outcome, ok := envelope.Data["outcome"].(string); ok {
			win = outcome == "win"
		}
		var tasks []int
		var err error
		if raw := envelope.Data["finished_task_list"]; raw != nil {
			tasks, err = battleTaskList(raw)
			if err != nil {
				return err
			}
		}
		if trial.ActivityContext != nil {
			if err := recordLeagueProtectStatistics(trial, envelope.Data); err != nil {
				return err
			}
			if err := recordActivityBattleStatistics(trial, envelope.Data); err != nil {
				return err
			}
			_, err = settleActivityDungeon(&scratch, trial, win, tasks, s.Now())
		} else {
			if err := recordSpecialDrillAchievements(&scratch, trial, win, tasks, s.Now()); err != nil {
				return err
			}
			_, err = withBattleOverflowMail(&scratch, trial, s.Now(), func() (map[string]any, error) {
				box, err := settleOrdinaryDungeonRewards(&scratch, trial, win, s.Now())
				if err != nil {
					return nil, err
				}
				start := len(trial.RewardBonusPlan)
				trial.RewardExtraPlanStart = &start
				if err := settleRemainingDungeon(&scratch, trial, win, tasks, box, s.Now()); err != nil {
					return nil, err
				}
				return box, nil
			})
		}
		if err != nil {
			return err
		}
		b.RewardBonusPlan, b.RewardPlanFrozen = trial.RewardBonusPlan, true
		b.RewardExtraPlanStart = trial.RewardExtraPlanStart
		copy := *envelope
		copy.Data = cloneEventData(envelope.Data)
		b.RewardResult, b.RewardPlannedAt = &copy, s.Now().Unix()
		return nil
	})
}

type BattleBonusReceipt struct {
	BonusID   int             `json:"bonus_id"`
	Amount    int64           `json:"amount"`
	Materials map[int]int64   `json:"materials,omitempty"`
	Cards     []Card          `json:"cards,omitempty"`
	Runes     map[string]Rune `json:"runes,omitempty"`
}

func grantFrozenBattleBonus(p *Progress, id int, amount int64, level int, now time.Time) (map[string]any, error) {
	b, index := p.battleRewardSession, p.battleRewardCursor
	p.battleRewardCursor++
	if index < len(b.RewardBonusPlan) {
		r := b.RewardBonusPlan[index]
		if r.BonusID != id || r.Amount != amount {
			return nil, errors.New("战斗冻结奖励来源不匹配")
		}
		changes := map[int]int64{}
		cards := []string{}
		ids := []int{}
		for mid := range r.Materials {
			ids = append(ids, mid)
		}
		sort.Ints(ids)
		for _, mid := range ids {
			if err := grantNativeItem(p, mid, r.Materials[mid], level, now, changes, &cards, 0); err != nil {
				return nil, err
			}
		}
		if len(r.Cards) > androidCompose.MaxCards-len(p.Cards) {
			return nil, runeReject("RET_CARD_COUNT_REACH_MAX", "冻结战斗幻书超过容量")
		}
		limits, err := loadRuneTables()
		if err != nil {
			return nil, err
		}
		if len(r.Runes) > limits.Tables.Limits.Max-len(p.Runes) {
			return nil, runeReject("RET_RUNE_MAX_COUNT_EXCEED", "冻结战斗契印超过容量")
		}
		for _, card := range r.Cards {
			if _, existing := findCard(p, card.UUID); existing != nil {
				return nil, errors.New("冻结战斗幻书编号冲突")
			}
			appendOwnedCard(p, card)
			cards = append(cards, card.UUID)
		}
		if p.Runes == nil {
			p.Runes = map[string]Rune{}
		}
		for uuid, rune := range r.Runes {
			if _, exists := p.Runes[uuid]; exists {
				return nil, errors.New("冻结战斗契印编号冲突")
			}
			p.Runes[uuid] = rune
		}
		box := map[string]any{"__custom_type": "box.box", "materials": changes, "cards": cardListWire(p, cards)}
		if len(r.Runes) > 0 {
			box["runes"] = runeMgrProperties(r.Runes)
		}
		return box, nil
	}
	if b.RewardPlanFrozen {
		return nil, errors.New("冻结战斗奖励收据缺失")
	}
	beforeCards, beforeRunes := map[string]bool{}, map[string]bool{}
	for _, card := range p.Cards {
		beforeCards[card.UUID] = true
	}
	for uuid := range p.Runes {
		beforeRunes[uuid] = true
	}
	beforeOverflow := map[int]int64{}
	for mid, n := range p.battleRewardOverflow {
		beforeOverflow[mid] = n
	}
	box, err := grantNativeBonusRaw(p, id, amount, level, now)
	if err != nil {
		return nil, err
	}
	r := BattleBonusReceipt{BonusID: id, Amount: amount, Materials: map[int]int64{}, Runes: map[string]Rune{}}
	for mid, n := range box["materials"].(map[int]int64) {
		r.Materials[mid] = n
	}
	for mid, n := range p.battleRewardOverflow {
		extra := n - beforeOverflow[mid]
		if extra < 0 || r.Materials[mid] > math.MaxInt64-extra {
			return nil, errors.New("冻结奖励材料计数溢出")
		}
		if extra > 0 {
			r.Materials[mid] += extra
		}
	}
	for _, card := range p.Cards {
		if !beforeCards[card.UUID] {
			r.Cards = append(r.Cards, card)
		}
	}
	for uuid, rune := range p.Runes {
		if !beforeRunes[uuid] {
			r.Runes[uuid] = rune
		}
	}
	b.RewardBonusPlan = append(b.RewardBonusPlan, r)
	return box, nil
}

func promoteBattleOverflowMailbox(p *Progress) error {
	ids := []int{}
	for id := range p.BattleOverflowMailbox {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		if len(p.ShortMailInfo) >= 500 {
			break
		}
		if err := insertMail(p, p.BattleOverflowMailbox[id]); err != nil {
			return err
		}
		delete(p.BattleOverflowMailbox, id)
	}
	return nil
}
