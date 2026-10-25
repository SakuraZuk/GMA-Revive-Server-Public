package game

// CollectionRankCardIDs 与Android图鉴的disable/not_in_stat过滤相同。
// 此目录82种卡均有cards_archive记录且branch_num为1至5，排除素材和剧情变体。
func CollectionRankCardIDs() []int {
	return append([]int(nil), androidProfile.CountedCards...)
}

// 收集榜按已获取的不同卡种计数，不按背包副本数计数。
// 历史取得记录保留已分解卡；旧存档仅补当前实际拥有卡，不能重建不存在的历史。
func CollectionRankCount(p Progress) int {
	seen := map[int]bool{}
	for id := range p.ObtainedCardIDs {
		if containsInt(androidProfile.CountedCards, id) {
			seen[id] = true
		}
	}
	for _, card := range p.Cards {
		if containsInt(androidProfile.CountedCards, card.CardID) {
			seen[card.CardID] = true
		}
	}
	return len(seen)
}
