package game

import (
	"os"
	"sort"
	"strconv"
	"time"
)

// NewAvatarProgress 仅供真正的新建角色调用；旧存档恢复仍用 NewProgress。
// 测试开关只授予本版图鉴统计中的可用英雄，不授予禁用卡、素材卡或剧情变体。
func NewAvatarProgress(level int, now time.Time) Progress {
	p := NewProgress(level, now)
	enabled, _ := strconv.ParseBool(os.Getenv("HS_NEW_AVATAR_ALL_HEROES"))
	if !enabled {
		return p
	}
	owned := make(map[int]bool, len(p.Cards))
	for _, card := range p.Cards {
		owned[card.CardID] = true
	}
	ids := append([]int(nil), androidProfile.CountedCards...)
	sort.Ints(ids)
	for _, id := range ids {
		if !owned[id] {
			appendOwnedCard(&p, newCard(id, 1, now))
			owned[id] = true
		}
	}
	ensureObtainedCardHistory(&p)
	reconcileAchievementState(&p, level, now)
	return p
}
