package game

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"time"
)

// Android box.get_card_exps将材料3完整数量赋予每个接收UUID，不在成员之间平分。
// 接收者来自服务端已冻结的真实拥有阵容；剧情及外部助战不会写入本玩家卡库存。
func grantCardBattleExp(p *Progress, uuids []string, amount int64) ([]string, error) {
	if amount < 0 || amount > math.MaxInt32 {
		return nil, errors.New("战斗幻书经验无效")
	}
	receivers := []string{}
	seen := map[string]bool{}
	for _, uuid := range uuids {
		if uuid == "" || seen[uuid] {
			continue
		}
		seen[uuid] = true
		_, card := findCard(p, uuid)
		if card == nil {
			continue
		}
		if err := checkCardGrowth(card); err != nil {
			return nil, err
		}
		cap, err := cardLevelCap(card)
		if err != nil {
			return nil, err
		}
		if card.Level < 1 || card.Level > cap || card.Exp < 0 || int64(card.Exp) > math.MaxInt32-amount {
			return nil, errors.New("幻书经验存档无效或溢出")
		}
		level, exp := card.Level, int64(card.Exp)+amount
		for level < cap && cardLevelUnlocked(level+1, p.AvatarLevel) {
			row, ok := androidOath.Levels[level]
			if !ok || row.Exp <= 0 {
				return nil, errors.New("幻书经验目录无效")
			}
			if exp < row.Exp {
				break
			}
			exp -= row.Exp
			level++
		}
		// 原生hero_item显示馆主门槛在当前等级max_exp-1，品阶满级不继续累计经验。
		if level == cap {
			exp = 0
		} else {
			max := androidOath.Levels[level].Exp
			if max <= 0 {
				return nil, errors.New("幻书经验上限无效")
			}
			if exp >= max {
				exp = max - 1
			}
		}
		card.Level, card.Exp = level, int(exp)
		receivers = append(receivers, card.UUID)
	}
	return receivers, nil
}

// 奖励盒的经验接收成员必须在资产事务内一起写入，随机奖励及接收列表随后固化为收据。
func applyBattleCardExp(p *Progress, b *BattleSession, box map[string]any) error {
	if box == nil {
		return nil
	}
	amounts, ok := box["materials"].(map[int]int64)
	if !ok {
		return errors.New("战斗奖励材料结构无效")
	}
	amount := amounts[3]
	if amount == 0 {
		return nil
	}
	receivers, err := grantCardBattleExp(p, b.Team, amount)
	if err != nil {
		return err
	}
	wire := make([]any, 0, len(receivers))
	for _, uuid := range receivers {
		wire = append(wire, ObjectID(uuid))
	}
	box["card_exp_receiver"] = wire
	return nil
}

// D81B4FE0.get_success_bonus_id：每日奖励以周一为第一项，按北京时间选取。
// 装备副本的bonus为空，不能因此漏掉everyday_bonus。
func ordinaryDailyBonus(row activityRow, now time.Time) (int, error) {
	var daily []int
	if raw := row["everyday_bonus"]; len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &daily); err != nil {
			return 0, errors.New("副本每日奖励目录无效")
		}
	}
	weekday := (int(now.In(shanghaiZone).Weekday()) + 6) % 7
	if weekday < len(daily) {
		return daily[weekday], nil
	}
	return row.integer("bonus"), nil
}

// 普通与剧情副本按原生首通、每日/常规、失败奖励发完整盒；失败返还只走失败分支。
func settleOrdinaryDungeonRewards(p *Progress, b *BattleSession, win bool, now time.Time) (map[string]any, error) {
	if b.RewardPlanFrozen && b.RewardPlannedAt > 0 {
		now = time.Unix(b.RewardPlannedAt, 0)
	}
	return withBattleOverflowMail(p, b, now, func() (map[string]any, error) {
		return settleOrdinaryDungeonRewardsAssets(p, b, win, now)
	})
}

func settleOrdinaryDungeonRewardsAssets(p *Progress, b *BattleSession, win bool, now time.Time) (map[string]any, error) {
	if b.RewardGranted {
		return emptyActivityBox(), nil
	}
	row := activityData("dungeons", b.DungeonID)
	if len(row) == 0 {
		return nil, errors.New("副本原生奖励目录不存在")
	}
	bonuses := []int{}
	if win {
		if !containsInt(p.ClearedDungeons, b.DungeonID) {
			bonuses = append(bonuses, row.integer("first_bonus"))
		}
		daily, err := ordinaryDailyBonus(row, now)
		if err != nil {
			return nil, err
		}
		if daily > 0 && (!ordinaryDailyRewardSupported(daily) || androidShop.Bonuses[daily].Lucky != 0 && dungeonCatalog[b.DungeonID].Type != 8) {
			// 只接入已完整实现的每日盒，避免把旧版高层空奖励升级成结果事务拒绝/卡住。
			// 未经授权的动态规则仍不接入；已授权装备本基础奖励不受幸运保底字段阻断。
			log.Printf("副本每日奖励尚未完整实现 dungeon=%d bonus=%d 动态规则待取证，保留旧版常规奖励", b.DungeonID, daily)
			daily = row.integer("bonus")
		}
		bonuses = append(bonuses, daily)
	} else {
		bonuses = append(bonuses, row.integer("fail_bonus"))
	}
	box := emptyActivityBox()
	if b.RewardPlanFrozen {
		// 首通与每日来源采用已提交的抽奖收据，重试不随日期或其他通关变化。
		bonuses = nil
		plan := b.RewardBonusPlan
		if b.RewardExtraPlanStart != nil {
			if *b.RewardExtraPlanStart < 0 || *b.RewardExtraPlanStart > len(plan) {
				return nil, errors.New("战斗奖励收据分界无效")
			}
			plan = plan[:*b.RewardExtraPlanStart]
		}
		for _, receipt := range plan {
			bonuses = append(bonuses, receipt.BonusID)
		}
	}
	for _, id := range bonuses {
		if id <= 0 {
			continue
		}
		previous := p.battleBaseBonusID
		if dungeonCatalog[b.DungeonID].Type == 8 {
			daily, e := ordinaryDailyBonus(row, now)
			if e != nil {
				return nil, e
			}
			if id == daily {
				p.battleBaseBonusID = id
			}
		}
		next, err := grantNativeBonus(p, id, remainingDungeonRewardAmount(b, win), p.AvatarLevel, now)
		p.battleBaseBonusID = previous
		if err != nil {
			return nil, err
		}
		if err = mergeActivityBox(box, next); err != nil {
			return nil, err
		}
	}
	if !win {
		returned := row.integer("return_power")
		// D81B4FE0.get_fail_return_power：双倍失败返还也翻倍，沿用本场已支付冻结倍率。
		if remainingExtraInt(b.Extra, "server_dungeon_bonus_double") == 2 && b.PaidMaterials[29] > 0 {
			if returned < 0 || returned > math.MaxInt32/2 {
				return nil, errors.New("双倍失败返还体力无效或溢出")
			}
			returned *= 2
		}
		if int64(returned) > b.PaidPower {
			returned = int(b.PaidPower)
		}
		if returned < 0 || int64(returned) > math.MaxInt32-int64(p.currentPower(now)) {
			return nil, errors.New("失败返还体力无效或溢出")
		}
		if returned > 0 {
			p.settlePowerRecovery(now)
			p.Power.Value += returned
			box["materials"].(map[int]int64)[1] += int64(returned)
		}
		var returns [][]int64
		if raw := row["return_materials"]; len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &returns); err != nil {
				return nil, errors.New("副本返还材料结构无效")
			}
		}
		for _, pair := range returns {
			if len(pair) != 2 || pair[0] <= 0 || pair[1] < 0 || pair[1] > b.PaidMaterials[int(pair[0])] {
				return nil, errors.New("副本返还超过已支付材料")
			}
			if pair[1] == 0 {
				continue
			}
			part := map[int]int64{}
			cards := []string{}
			if err := grantNativeItem(p, int(pair[0]), pair[1], p.AvatarLevel, now, part, &cards, 0); err != nil {
				return nil, err
			}
			if err := mergeActivityBox(box, map[string]any{"materials": part, "cards": cardListWire(p, cards)}); err != nil {
				return nil, err
			}
		}
	}
	if err := applyBattleCardExp(p, b, box); err != nil {
		return nil, err
	}
	b.RewardGranted = true
	return box, nil
}

func ordinaryDailyRewardSupported(id int) bool {
	b, known := androidShop.Bonuses[id]
	if !known || len(b.Inner) > 0 && string(b.Inner) != "null" {
		return false
	}
	for _, pair := range b.Fixed {
		if len(pair) != 2 {
			return false
		}
	}
	for _, raw := range b.RandomItems {
		var fields []json.RawMessage
		var materialID int
		if json.Unmarshal(raw, &fields) != nil || len(fields) != 3 || json.Unmarshal(fields[0], &materialID) != nil {
			return false
		}
	}
	return true
}

func prepareOrdinaryDungeonCosts(p *Progress, id int, now time.Time) (map[int]int64, error) {
	row := activityData("dungeons", id)
	if len(row) == 0 || row.flag("_dungeon_disable") && !remainingAuthorizedTaskDungeon(p, id) && !authorizedReopenedSpecialDrill(id) {
		return nil, runeReject("RET_DUNGEON_CLOSED", "副本不存在或已停用")
	}
	if err := ordinaryDungeonConditions(row, *p, now); err != nil {
		return nil, err
	}
	if err := ordinaryDungeonConditions(activityData("activity_type", row.integer("dungeon_type")), *p, now); err != nil {
		return nil, err
	}
	return activityMaterialCosts(p, row["need_materials"])
}
