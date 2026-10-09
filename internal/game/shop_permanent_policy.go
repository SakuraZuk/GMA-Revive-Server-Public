package game

// 2026-10-08用户明确指定这五个开服限时商品永久开放。
// 仅移除本版已确认的开服天数门槛；价格、限购、解锁和失效条件仍由原生目录检查。
// 按编号与原天数双重匹配，目录出现新商品或规则漂移时仍走严格开服时间校验。
func shopPermanentOpenDays(r commodityRule) bool {
	if r.OpenDays == nil {
		return false
	}
	switch r.ID {
	case 1071011, 1071012, 1071013, 1071014:
		return *r.OpenDays == 15
	case 2010010:
		return *r.OpenDays == 7
	}
	return false
}
