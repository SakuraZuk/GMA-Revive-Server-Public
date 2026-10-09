package game

// syncPvpResultPushes 是原生 Int,Int,Bool,Int,Int,Int 下行，客户端不能提交发奖指令。
func syncPvpResultPushes(c *Connection, result SyncPvpResult) []Push {
	p := c.SelectedAvatarUnsafe().Progress
	values := map[string]int{"sync_pvp_score": p.SyncPvpScore, "sync_pvp_highest_score": p.SyncPvpHighestScore,
		"sync_pvp_continues_win_count": p.SyncPvpWinStreak, "sync_weekly_win_count": p.SyncPvpWeeklyWins}
	keys := []string{"sync_pvp_score", "sync_pvp_highest_score", "sync_pvp_continues_win_count", "sync_weekly_win_count"}
	pushes := make([]Push, 0, 6)
	for _, key := range keys {
		pushes = append(pushes, push("Avatar", "client_prop_changed", []any{key, values[key]}))
	}
	pushes = append(pushes, push("Avatar", "battle_result", p.Battle.Outcome == "win", map[string]any{}, map[string]any{}, nativeSoloResultExtra(p.Battle, map[string]any{"client_authoritative": true, "verified": false, "dungeon_id": p.Battle.DungeonID})))
	return append(pushes, push("Avatar", "sync_pvp_battle_result", result.DeltaScore, result.DeltaCoin,
		result.WinContinuously, result.DivisionUpdated, result.OwnScore, result.EnemyScore))
}
