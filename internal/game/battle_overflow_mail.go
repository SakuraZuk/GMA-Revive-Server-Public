package game

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"
)

// 用户确认的运营规则：容量内到账，超出材料保留邮件，不丢弃奖励。
// 按单种材料上限拆分附件，避免单封邮件永久无法领取；不设置过期日。
func withBattleOverflowMail(p *Progress, b *BattleSession, now time.Time, settle func() (map[string]any, error)) (map[string]any, error) {
	if b.RewardGranted || b.ActivityContext == nil && b.DungeonID == 0 {
		return settle()
	}
	if p.battleRewardOverflow != nil {
		if p.battleRewardSession == b {
			return settle()
		}
		return nil, errors.New("战斗奖励溢出规划不得嵌套")
	}
	p.battleRewardOverflow = map[int]int64{}
	p.battleRewardSession, p.battleRewardCursor = b, 0
	defer func() { p.battleRewardOverflow = nil; p.battleRewardSession = nil; p.battleRewardCursor = 0 }()
	box, err := settle()
	if err != nil {
		return nil, err
	}
	if b.RewardPlanFrozen && p.battleRewardCursor != len(b.RewardBonusPlan) {
		return nil, errors.New("冻结战斗奖励收据未完整兑现")
	}
	ids := []int{}
	for id := range p.battleRewardOverflow {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		left := p.battleRewardOverflow[id]
		limit := androidShop.Materials[id].Limit
		if limit <= 0 || left <= 0 || !validObjectID(b.UUID) {
			return nil, errors.New("战斗溢出邮件收据无效")
		}
		for part := 0; left > 0; part++ {
			amount := min(left, limit)
			digest := sha256.Sum256([]byte(fmt.Sprintf("战斗溢出:%s:%d:%d", b.UUID, id, part)))
			mid := int(binary.BigEndian.Uint32(digest[:4]) & 0x7fffffff)
			if mid == 0 {
				return nil, errors.New("战斗溢出邮件编号无效")
			}
			mail := Mail{MID: mid, UUID: hex.EncodeToString(digest[4:16]), Title: "战斗奖励暂存", Content: "材料容量不足，超出部分已保留。释放材料容量后领取附件。", Sender: "阿克夏书馆", Attachments: map[int]int64{id: amount}, RewardsFrozen: true, CreatedAt: now.Unix()}
			if err := validateMailAssets(mail); err != nil {
				return nil, err
			}
			// 容量判断之前检查两个邮箱，不能把编号冲突当成邮箱已满。
			for _, mailbox := range []map[int]Mail{p.ShortMailInfo, p.BattleOverflowMailbox} {
				for oldID, old := range mailbox {
					if oldID == mid || old.oid() == mail.oid() {
						return nil, errors.New("战斗溢出邮件编号冲突")
					}
				}
			}
			if err := insertMail(p, mail); err != nil {
				// 邮箱500封上限保留；奖励存于待投递邮箱，释放邮件槽后自动投递。
				if len(p.ShortMailInfo) < 500 {
					return nil, err
				}
				if p.BattleOverflowMailbox == nil {
					p.BattleOverflowMailbox = map[int]Mail{}
				}
				if _, exists := p.BattleOverflowMailbox[mid]; exists {
					return nil, errors.New("待投递奖励邮件编号重复")
				}
				p.BattleOverflowMailbox[mid] = mail
			}
			left -= amount
			b.RewardOverflowMailed = true
		}
	}
	return box, nil
}
