package game

import "os"

// 本版全部12个旧演练DID停用；只有明确批准的本服恢复版本才能绕过该停用位。
// 原解锁条件、敌人、轮次、原生tower_task阈值及奖励仍走完整普通副本事务。
func specialDrillReopenEnabled() bool {
	return remainingPolicyEnabled() && os.Getenv("HS_SPECIAL_DRILL_REOPEN") == "timber-12-20261008"
}

func authorizedReopenedSpecialDrill(id int) bool {
	if !specialDrillReopenEnabled() {
		return false
	}
	switch id {
	case 2103, 2106, 2112, 2203, 2206, 2212, 2303, 2306, 2312, 2503, 2506, 2512:
		return dungeonCatalog[id].Type == 382
	}
	return false
}
