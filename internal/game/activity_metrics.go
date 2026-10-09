package game

import (
	"errors"
	"math"
	"time"
)

// FBBE590A 返回的两个原生统计，必须来自当前冻结会话的 result，禁止命令数推测。
type ActivityBattleStatistics struct {
	AP     int64 `json:"total_ap_statistics"`
	Damage int64 `json:"total_damaged_statistics"`
}

func recordActivityBattleStatistics(b *BattleSession, data map[string]any) error {
	if b.ActivityContext == nil {
		return nil
	}
	bc := b.ActivityContext
	if bc.DungeonID != b.DungeonID || bc.PreparedAt <= 0 || b.RewardGranted {
		return errors.New("活动战报会话不匹配或已结算")
	}
	ap, hasAP := data["total_ap_statistics"]
	damage, hasDamage := data["total_damaged_statistics"]
	if !hasAP && !hasDamage {
		// 原桥兼容：没有真实统计时仅保留关卡进度，不参与排行。
		bc.Statistics = nil
		return nil
	}
	parse := func(raw any) (int64, error) {
		var value float64
		switch v := raw.(type) {
		case float64:
			value = v
		case int:
			value = float64(v)
		case int64:
			if v < 0 || v > 9007199254740991 {
				return 0, errors.New("活动战报统计超界")
			}
			return v, nil
		default:
			return 0, errors.New("活动战报统计必须为整数")
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 9007199254740991 || math.Trunc(value) != value {
			return 0, errors.New("活动战报统计必须为有限非负整数")
		}
		return int64(value), nil
	}
	if !hasAP || !hasDamage {
		return errors.New("活动战报两个原生统计必须同时提供")
	}
	a, err := parse(ap)
	if err != nil {
		return err
	}
	d, err := parse(damage)
	if err != nil {
		return err
	}
	if a > math.MaxInt32 {
		return errors.New("山海行动统计超界")
	}
	bc.Statistics = &ActivityBattleStatistics{AP: a, Damage: d}
	return nil
}

// A7E8977E avatar_item.set_info 的原生tuple字段是卡模板、等级、品阶、外观、援护。
// 在最终阵容冻结时调用，结算不读取可能随后升级的玩家卡牌。
func freezeActivityRankCards(p *Progress, b *BattleSession) error {
	if b.ActivityContext == nil {
		return nil
	}
	rows := []any{}
	support := map[string]bool{}
	if b.Layout != nil {
		for _, id := range b.Layout.Support {
			support[id] = true
		}
	}
	for _, uuid := range b.Team {
		if uuid == "" {
			continue
		}
		card := battleFormationCard(p, uuid)
		if card == nil {
			return errors.New("活动排行阵容没有合法卡牌实例")
		}
		rows = append(rows, []any{card.CardID, card.Level, card.Grade, card.Dress, support[uuid]})
	}
	b.ActivityContext.RankCards = rows
	return nil
}

func recordNianBattleDamage(p *Progress, bc *ActivityBattleContext, now time.Time) error {
	if bc.Statistics == nil {
		return nil
	}
	if p.Activities.Nian == nil || len(activityData("monster_nian_dungeon", bc.DungeonID)) == 0 {
		return errors.New("年兽伤害战报没有合法副本状态")
	}
	d := p.Activities.Nian[bc.DungeonID]
	damage := bc.Statistics.Damage
	if damage < 0 || damage > math.MaxInt64-d.TotalDamage {
		return errors.New("年兽累计伤害溢出")
	}
	d.ID = bc.DungeonID
	d.TotalDamage += damage
	if damage > d.MaxDamage {
		d.MaxDamage = damage
		d.Cards = append([]any{}, bc.RankCards...)
	}
	// 本版活动说明明确：跨零点完成的成绩不计榜。个人战斗记录仍保存真实伤害。
	if bc.PreparedAt <= 0 || now.Unix() < bc.PreparedAt {
		return errors.New("年兽战报时间无效")
	}
	validDay := time.Unix(bc.PreparedAt, 0).In(shanghaiZone).Format("2006-01-02") == now.In(shanghaiZone).Format("2006-01-02")
	// 已批准本服参赛规则：有实际result的0伤害是参赛；旧无收据的0字段不是。
	firstResult := remainingPolicyEnabled() && (!d.Ranked || d.RankAt <= 0)
	if validDay && (firstResult || damage > d.RankDamage) {
		if remainingPolicyEnabled() {
			recordNianScoreHour(p, bc.DungeonID, d)
		}
		d.Ranked = true
		d.RankDamage = damage
		d.RankCards = append([]any{}, bc.RankCards...)
		d.RankAt = now.Unix()
		if remainingPolicyEnabled() {
			recordNianScoreHour(p, bc.DungeonID, d)
		}
	}
	p.Activities.Nian[bc.DungeonID] = d
	return nil
}
