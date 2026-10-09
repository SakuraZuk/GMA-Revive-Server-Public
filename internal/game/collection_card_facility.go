package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
)

func (s *Service) collectionCardFacilityRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("收藏室培育需要玩家状态")
	}
	need := 1
	if method == "facility_card_upgrade" {
		need = 3
	} else if method == "get_facility_card_reward" {
		need = 2
	}
	if len(args) != need {
		return nil, errors.New("培育设施参数数量无效")
	}
	values := make([]int64, need)
	for i, raw := range args {
		if json.Unmarshal(raw, &values[i]) != nil || values[i] <= 0 {
			return nil, errors.New("培育设施参数必须为正整数")
		}
	}
	id := int(values[0])
	box := map[string]any{"__custom_type": "box.box", "materials": map[int]int64{}}
	e := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureCollection(p, s.Now()); e != nil {
			return e
		}
		f, owned := p.Collection.Facilities[id]
		if !owned {
			return runeReject("RET_HOUSE_FACILITY_LOCKED", "培育设施尚未解锁")
		}
		if androidCollection.Facilities[id].Sheet != "facility_card" {
			return runeReject("RET_HOUSE_FACILITY_NO_CARD_UPGRADE", "此设施不能培育")
		}
		rule, known := androidCollection.Levels[id][f.Level]
		if !known || rule.Count <= 0 || f.Recycle < 0 || int64(f.Recycle) > rule.Count {
			return errors.New("培育设施存档或目录无效")
		}
		switch method {
		case "facility_card_upgrade":
			if values[1] != int64(rule.Material) {
				return runeReject("RET_HOUSE_FACILITY_CARD_NO_UPGRADE_MATERIAL", "培育材料编号错误")
			}
			if values[2] > rule.Count-int64(f.Recycle) {
				return runeReject("RET_HOUSE_FACILITY_CARD_UPGRADE_ENOUGH", "培育材料超出本轮剩余进度")
			}
			if p.Materials[rule.Material].Count < values[2] {
				return runeReject("RET_HOUSE_FACILITY_CARD_NO_ENOUGH_UPGRADE_MATERIAL", "培育材料不足")
			}
			if rule.UnitBonusCount <= 0 || math.IsNaN(rule.UnitBonusCount) || math.IsInf(rule.UnitBonusCount, 0) || rule.UnitBonusCount != math.Trunc(rule.UnitBonusCount) {
				return errors.New("培育单份奖励规则无效")
			}
			if rule.UnitBonusCount != 1 {
				return errors.New("培育单份奖励换算规则尚未取证")
			}
			if e := collectionSpend(p, rule.Material, values[2]); e != nil {
				return e
			}
			var e error
			box, e = grantNativeBonus(p, rule.UnitBonus, values[2], p.AvatarLevel, s.Now())
			if e != nil {
				return e
			}
			f.Recycle += int(values[2])
		case "get_facility_card_reward":
			rank := int(values[1])
			bid := 0
			for _, stage := range rule.StageRewards {
				if len(stage) != 2 {
					return errors.New("培育阶段奖励结构无效")
				}
				if stage[0] == rank {
					bid = stage[1]
				}
			}
			if bid <= 0 {
				return runeReject("RET_HOUSE_FACILITY_CARD_NO_STAGE_REWARD", "不存在该培育阶段奖励")
			}
			if f.Recycle < rank {
				return runeReject("RET_HOUSE_FACILITY_CARD_STAGE_REWARD_NO_GET", "培育阶段进度不足")
			}
			if f.CardReward[rank] {
				return runeReject("RET_HOUSE_FACILITY_CARD_STAGE_REWARD_HAD_GET", "培育阶段奖励已领取")
			}
			var e error
			box, e = grantNativeBonus(p, bid, 1, p.AvatarLevel, s.Now())
			if e != nil {
				return e
			}
			if f.CardReward == nil {
				f.CardReward = map[int]bool{}
			}
			f.CardReward[rank] = true
		case "upgrade_house_facility_card":
			if int64(f.Recycle) < rule.Count {
				return runeReject("RET_HOUSE_FACILITY_CARD_NO_GET_REWARD", "本轮培育尚未完成")
			}
			for _, stage := range rule.StageRewards {
				if len(stage) != 2 || !f.CardReward[stage[0]] {
					return runeReject("RET_HOUSE_FACILITY_CARD_NO_GET_REWARD", "请先领取所有培育阶段奖励")
				}
			}
			next, known := androidCollection.Levels[id][f.Level+1]
			if !known {
				return runeReject("RET_HOUSE_FACILITY_MAX_LEVEL", "培育设施已经完成全部轮次")
			}
			f.Level++
			f.Recycle = 0
			f.RecycleKeep = 0
			f.CardReward = map[int]bool{}
			for _, stage := range next.StageRewards {
				if len(stage) != 2 {
					return errors.New("下一轮培育阶段目录无效")
				}
				f.CardReward[stage[0]] = false
			}
		default:
			return errors.New("培育设施接口不存在")
		}
		p.Collection.Facilities[id] = f
		reconcileCollectionAchievements(p, s.Now())
		return nil
	})
	code := RetSuccess
	if e != nil {
		var reject *runeBusinessError
		if !errors.As(e, &reject) {
			return nil, e
		}
		var known bool
		code, known = androidCollection.Errors[reject.Name]
		if !known {
			return nil, errors.New("培育设施错误码缺失")
		}
	}
	out := []Push{}
	if e == nil {
		out = append(out, materialManagerPush(c), cardMgrPush(c), runePush(c), knowledgePush(c))
		out = append(out, collectionPushes(c.SelectedAvatarUnsafe().Progress, s.Now(), false)...)
	}
	switch method {
	case "facility_card_upgrade":
		out = append(out, push("Avatar", "on_facility_card_upgrade", code, id, box))
	case "get_facility_card_reward":
		out = append(out, push("Avatar", "on_get_facility_card_reward", code, box))
	case "upgrade_house_facility_card":
		out = append(out, push("Avatar", "on_upgrade_house_facility_card", code))
	}
	return out, nil
}
