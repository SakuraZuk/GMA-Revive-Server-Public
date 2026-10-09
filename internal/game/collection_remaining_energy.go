package game

import (
	"errors"
	"math"
	"time"
)

func collectionEnergyNumber(energy map[string]any, key string) (float64, error) {
	var value float64
	switch v := energy[key].(type) {
	case nil:
	case int:
		value = float64(v)
	case int64:
		value = float64(v)
	case float64:
		value = v
	default:
		return 0, errors.New("收藏室能量存档类型无效")
	}
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("收藏室能量存档数值无效")
	}
	return value, nil
}

// 更换住客或设施前先按旧参数结算；耗尽不因读取或换人补满，空房再次入住才重启。
func collectionRefreshEnergy(p *Progress, now time.Time) error {
	if !remainingPolicyEnabled() || p.Collection == nil {
		return nil
	}
	stamp := float64(now.UnixNano()) / 1e9
	for rid, room := range p.Collection.Rooms {
		if room.Energy == nil {
			room.Energy = newCollectionRoom(rid, now).Energy
		}
		e := room.Energy
		// 在新启动或关闭前验证旧时刻，不能用重置掩盖回拨或损坏存档。
		for _, key := range []string{"last_get_time", "last_update_time"} {
			old, err := collectionEnergyNumber(e, key)
			if err != nil {
				return err
			}
			if old > stamp {
				return errors.New("收藏室能量时钟回拨")
			}
		}
		last, err := collectionEnergyNumber(e, "settled_time")
		if err != nil {
			return err
		}
		if last > stamp {
			return errors.New("收藏室能量时钟回拨")
		}
		value, err := collectionEnergyNumber(e, "value")
		if err != nil {
			return err
		}
		per, err := collectionEnergyNumber(e, "per_value")
		if err != nil {
			return err
		}
		started, _ := e["start_flag"].(bool)
		if started && last > 0 {
			value = math.Max(0, value-(stamp-last)*per)
		}
		previousEligible, _ := e["eligible"].(bool)
		count := len(room.Slots)
		fid := 0
		for id, f := range p.Collection.Facilities {
			if androidCollection.Facilities[id].Room == rid && (id == 2 || id == 3 || id == 4) && f.Level > 0 {
				fid = id
				break
			}
		}
		base, per, maxEnergy := 0.0, 0.0, float64(count)*androidCollectionRemaining.EnergyPerCard
		if fid > 0 && count > 0 {
			f := p.Collection.Facilities[fid]
			row := androidCollection.Levels[fid][f.Level]
			if row.Unit <= 0 {
				return errors.New("收藏室能量单位周期无效")
			}
			cost := androidCollectionRemaining.EnergyCosts[fid][f.Level]
			tags, err := collectionFacilityTagProfit(*p, fid)
			if err != nil {
				return err
			}
			moodProfit := 0.0
			profits := androidCollection.Rooms[rid].CardProfit
			if count <= len(profits) {
				moodProfit = profits[count-1] * 4
			}
			base = cost / row.Unit
			per = cost * (1 + moodProfit + tags["efficiency_improve"][0]) / row.Unit
			if !previousEligible {
				value = maxEnergy
				started = true
				e["last_update_time"] = stamp
				e["last_get_time"] = stamp
			}
			value = math.Min(value, maxEnergy)
			if value <= 0 {
				started = false
			}
		} else {
			// 原生关闭生产才清零；已经空房的读取时刻应保持到下一次get。
			if previousEligible || started {
				e["last_get_time"] = float64(0)
			}
			value = 0
			started = false
		}
		update, err := collectionEnergyNumber(e, "last_update_time")
		if err != nil {
			return err
		}
		if update > stamp {
			return errors.New("收藏室能量刷新时钟回拨")
		}
		interval := androidCollectionRemaining.EnergyInterval
		if stamp-update >= interval {
			e["last_update_time"] = update + math.Floor((stamp-update)/interval)*interval
		}
		e["start_flag"] = started
		e["value"] = value
		e["base_cost"] = base
		e["per_value"] = per
		e["interval"] = interval
		e["resident_count"] = float64(count)
		e["eligible"] = fid > 0 && count > 0
		e["settled_time"] = stamp
		room.Energy = e
		p.Collection.Rooms[rid] = room
	}
	return nil
}
