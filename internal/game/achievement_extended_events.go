package game

import "time"

// 原生 dungeons 的 4/5 分别为异步/同步竞技；type4 目标使用相同枚举。
// 调用者必须在真实胜利结算的 UUID 收据去重之后，与积分/奖励同事务写入。
func recordAchievementCompetitiveWin(p *Progress, dungeonType int, now time.Time) {
	if dungeonType == 4 || dungeonType == 5 {
		advanceAchievementEvent(p, 4, dungeonType, now)
	}
}

// 以实际所属幻书和四个不同位置判断全身佩戴，不把背包数量或星级总和当成完成。
func achievementFullRuneCards(p Progress, star int) int64 {
	var count int64
	for _, card := range p.Cards {
		positions := map[int]bool{}
		for _, rune := range p.Runes {
			if rune.CardUUID == card.UUID && rune.Star >= star && rune.Position >= 1 && rune.Position <= 4 {
				positions[rune.Position] = true
			}
		}
		if len(positions) == 4 {
			count++
		}
	}
	return count
}
