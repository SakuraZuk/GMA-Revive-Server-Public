package game

import (
	"errors"
	"math"
	"time"
)

// 已批准本服政策：领取起算，剩余期限与本次份数相加；原生wire仍为到期减单份小时数。
func grantRemainingHeadFrame(p *Progress, materialID, target int, amount int64, now time.Time) error {
	head, known := androidProfile.Heads[target]
	material := androidShop.Materials[materialID]
	if !known || head.Kind != 2 || material.Type != 9 || material.Target != target || amount <= 0 || head.LimitHours < 0 || math.IsNaN(head.LimitHours) || math.IsInf(head.LimitHours, 0) {
		return runeReject("RET_SHOP_INVALID", "头像框材料、模板或数量无效")
	}
	stamp := float64(now.UnixNano()) / 1e9
	last := p.HeadFrameGrantTime[target]
	if stamp <= 0 || last < 0 || last > stamp || math.IsNaN(last) || math.IsInf(last, 0) {
		return errors.New("头像框领取时间无效或时钟回拨")
	}
	previous, owned := p.OwnedHeadBox[target]
	if owned && (previous < 0 || math.IsNaN(previous) || math.IsInf(previous, 0)) {
		return errors.New("头像框存档时间无效")
	}
	if p.OwnedHeadBox == nil {
		p.OwnedHeadBox = map[int]float64{}
	}
	if p.HeadFrameGrantTime == nil {
		p.HeadFrameGrantTime = map[int]float64{}
	}
	if head.LimitHours == 0 || (owned && previous == 0) {
		p.OwnedHeadBox[target] = 0
		p.HeadFrameGrantTime[target] = stamp
		return nil
	}
	unit := head.LimitHours * 3600
	base := stamp
	if owned {
		base = math.Max(base, previous+unit)
	}
	expiry := base + float64(amount)*unit
	if math.IsNaN(expiry) || math.IsInf(expiry, 0) || expiry <= base || expiry-unit <= 0 || expiry > 253402300799 {
		return errors.New("头像框叠加期限溢出")
	}
	p.OwnedHeadBox[target] = expiry - unit
	p.HeadFrameGrantTime[target] = stamp
	return nil
}
