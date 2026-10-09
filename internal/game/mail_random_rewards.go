package game

import (
	"errors"
	"math"
	"sort"
	"time"
)

// 签发时只在隔离的奖励规划副本抽取，真实玩家直到领取才获得资产。
// 原生池的资格只消费通关、等级和累计材料/活动账本；清空副本背包余额
// 避免收件人当前容量不足阻止邮件签发，领取仍检查全部真实容量。
func freezeMailRandomRewards(p Progress, m *Mail, now time.Time) error {
	ids := []int{}
	for id := range m.Attachments {
		if androidShop.Materials[id].Type == 7 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Ints(ids)
	scratch := CloneProgress(p)
	scratch.Cards = nil
	scratch.Runes = nil
	scratch.OwnedHeadBox = nil
	scratch.Power.Value = 0
	for id, material := range scratch.Materials {
		material.Count = 0
		scratch.Materials[id] = material
	}
	changes := map[int]int64{}
	cardIDs := []string{}
	for _, id := range ids {
		if err := grantNativeItem(&scratch, id, m.Attachments[id], max(p.AvatarLevel, 1), now, changes, &cardIDs, 0); err != nil {
			return err
		}
	}
	if len(scratch.Cards)+len(m.Cards) > 100 || len(scratch.Runes)+len(m.Runes) > 100 {
		return errors.New("冻结邮件随机资产超过附件上限")
	}
	m.SourceAttachments = map[int]int64{}
	actual := map[int]int64{}
	for id, n := range m.Attachments {
		m.SourceAttachments[id] = n
		if androidShop.Materials[id].Type != 7 {
			actual[id] = n
		}
	}
	for id, n := range changes {
		if n <= 0 || actual[id] > math.MaxInt64-n {
			return errors.New("冻结邮件奖励数量无效或溢出")
		}
		if err := validateMailItemDefinition(id, n, now); err != nil {
			return err
		}
		actual[id] += n
	}
	m.Attachments = actual
	m.Cards = append(m.Cards, scratch.Cards...)
	if len(scratch.Runes) > 0 {
		if m.Runes == nil {
			m.Runes = map[string]Rune{}
		}
		for id, rune := range scratch.Runes {
			if _, exists := m.Runes[id]; exists {
				return errors.New("冻结邮件契印编号冲突")
			}
			m.Runes[id] = rune
		}
	}
	m.RewardsFrozen = true
	return validateMailAssets(*m)
}
