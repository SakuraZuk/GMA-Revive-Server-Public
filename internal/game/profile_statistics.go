package game

// 统计字段来源Android Avatar属性与common_const.GET_PLAYER_DETAILS_INFO。
// 唯一卡数只计算当前真实持有且参与统计的幻书，不使用获取历史冒充当前库存。
func profileStatisticsProperties(p Progress) map[string]any {
	completed := map[int]bool{}
	for _, id := range p.ClearedDungeons {
		completed[id] = true
	}
	last := 0
	// Android目录原始顺序与主线地图的关卡序列一致；不从活动副本编号推导主线进度。
	for _, id := range androidProfile.MainDungeons {
		if completed[id] {
			last = id
		}
	}
	return map[string]any{"cards_count": countedCardCount(p), "last_main_chapter_dungeon_id": last, "achv_value": achievementPoints(p)}
}

func profileStatisticsPushes(p Progress) []Push {
	props := profileStatisticsProperties(p)
	out := []Push{}
	for _, field := range []string{"cards_count", "last_main_chapter_dungeon_id", "achv_value"} {
		out = append(out, push("Avatar", "client_prop_changed", []any{field, props[field]}))
	}
	return out
}
