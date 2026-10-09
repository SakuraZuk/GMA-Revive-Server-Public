package game

import (
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"time"
)

//go:embed avatar_growth_catalog.json
var avatarGrowthRaw []byte

var androidAvatarGrowth = func() struct {
	Limits struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"limits"`
	Levels map[int]struct {
		Exp   int64 `json:"level_max_exp"`
		Bonus int   `json:"level_up_bonus_id"`
	} `json:"levels"`
} {
	var out struct {
		Limits struct {
			Min int `json:"min"`
			Max int `json:"max"`
		} `json:"limits"`
		Levels map[int]struct {
			Exp   int64 `json:"level_max_exp"`
			Bonus int   `json:"level_up_bonus_id"`
		} `json:"levels"`
	}
	if json.Unmarshal(avatarGrowthRaw, &out) != nil || len(out.Levels) != out.Limits.Max || out.Limits.Min != 1 {
		panic("Android馆主经验目录无效")
	}
	return out
}()

// level与exp原生字段分别为馆主等级和该等级内经验；经验不作为材料库存。
// avatars.level与Progress.AvatarLevel由账号存储在同一事务内写入。
func grantAvatarExp(p *Progress, amount int64, now time.Time) error {
	if amount < 0 || p.AvatarExp < 0 || p.AvatarExp > math.MaxInt32-amount {
		return errors.New("馆主经验无效或溢出")
	}
	if p.AvatarLevel < androidAvatarGrowth.Limits.Min || p.AvatarLevel > androidAvatarGrowth.Limits.Max {
		return errors.New("馆主等级未绑定有效存档")
	}
	exp := p.AvatarExp + amount
	level := p.AvatarLevel
	for level < androidAvatarGrowth.Limits.Max {
		rule, ok := androidAvatarGrowth.Levels[level]
		if !ok || rule.Exp <= 0 || rule.Bonus != 0 {
			return errors.New("馆主升级规则尚未取证")
		}
		if exp < rule.Exp {
			break
		}
		exp -= rule.Exp
		level++
	}
	if max := androidAvatarGrowth.Levels[level].Exp; max <= 0 || exp > max {
		return errors.New("馆主满级经验超过原生上限")
	}
	// 先结算当前恢复时间，再提高上限；不凭空加满体力。
	if level != p.AvatarLevel {
		p.settlePowerRecovery(now)
		p.Power.Max = clientBaseline.LevelPower[level].Max
	}
	p.AvatarLevel, p.AvatarExp = level, exp
	return nil
}

func (p *Progress) settlePowerRecovery(now time.Time) {
	before := p.Power.Value
	value := p.currentPower(now)
	if value > before && p.Power.PerValue > 0 && p.Power.Interval > 0 {
		p.Power.LastTime += float64((value - before) / p.Power.PerValue * p.Power.Interval)
	}
	p.Power.Value = value
	if p.Power.Max > 0 && value >= p.Power.Max {
		p.Power.LastTime = float64(now.UnixNano()) / 1e9
	}
}
