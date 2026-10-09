package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"time"
)

//go:embed profile_catalog.json
var profileCatalogRaw []byte

type profileHead struct {
	Kind       int     `json:"head_type"`
	LimitHours float64 `json:"limit_days"`
	Target     int     `json:"unlock_target_id"`
	Dress      int     `json:"unlock_dress_id"`
	Gender     int     `json:"bind_gender"`
}

var androidProfile = func() struct {
	Heads           map[int]profileHead `json:"heads"`
	ClaimTimeFrames map[int]int         `json:"claim_time_frames"`
	HeadTargets     map[int]struct {
		Type   int   `json:"target_type"`
		Params []int `json:"target_params"`
	} `json:"head_targets"`
	CountedCards []int `json:"counted_cards"`
	MainDungeons []int `json:"main_dungeons"`
} {
	var catalog struct {
		Heads           map[int]profileHead `json:"heads"`
		ClaimTimeFrames map[int]int         `json:"claim_time_frames"`
		HeadTargets     map[int]struct {
			Type   int   `json:"target_type"`
			Params []int `json:"target_params"`
		} `json:"head_targets"`
		CountedCards []int `json:"counted_cards"`
		MainDungeons []int `json:"main_dungeons"`
	}
	if json.Unmarshal(profileCatalogRaw, &catalog) != nil || len(catalog.Heads) != 236 {
		panic("Android头像统计目录无效")
	}
	return catalog
}()

// Android owned_head_box 是 Int→Float 领取时刻；界面以该时刻+limit_days*3600计算期限。
// 六种PVP限时框材料均明确“以领取奖励时刻开始计时”。没有原服多份叠加算法，
// 单次只接受一份有限期奖励；再次领取重新记录本次时刻，不叠加剩余时长。
func grantProfileHeadBox(p *Progress, materialID, target int, amount int64, now time.Time) error {
	if remainingPolicyEnabled() {
		return grantRemainingHeadFrame(p, materialID, target, amount, now)
	}
	head, known := androidProfile.Heads[target]
	if !known || head.Kind != 2 || head.LimitHours < 0 || math.IsNaN(head.LimitHours) || math.IsInf(head.LimitHours, 0) || amount <= 0 {
		return runeReject("RET_SHOP_INVALID", "头像框奖励模板或数量无效")
	}
	acquired := float64(0)
	if head.LimitHours > 0 {
		if androidProfile.ClaimTimeFrames[materialID] != target {
			return runeReject("RET_SHOP_INVALID", "限时头像框的领取起算来源尚未取证")
		}
		if amount != 1 {
			return runeReject("RET_SHOP_BATCH_BUY_LIMIT", "限时头像框多份叠加期限尚未取证")
		}
		acquired = float64(now.UnixNano()) / 1e9
		if acquired <= 0 || math.IsNaN(acquired) || math.IsInf(acquired+head.LimitHours*3600, 0) {
			return errors.New("头像框领取时刻或期限无效")
		}
		if previous, owned := p.OwnedHeadBox[target]; owned {
			if previous < 0 || math.IsNaN(previous) || math.IsInf(previous, 0) || previous > acquired {
				return errors.New("头像框存档时间无效或时钟回拨")
			}
			// 0由原生update_limit_text按永久拥有处理，重复有限期奖励不能降为临时拥有。
			if previous == 0 {
				return nil
			}
		}
	}
	if p.OwnedHeadBox == nil {
		p.OwnedHeadBox = map[int]float64{}
	}
	p.OwnedHeadBox[target] = acquired
	return nil
}

func countedCardCount(p Progress) int {
	seen := map[int]bool{}
	for _, card := range p.Cards {
		if containsInt(androidProfile.CountedCards, card.CardID) {
			seen[card.CardID] = true
		}
	}
	return len(seen)
}

// 初始化仅承接旧选择、默认头像框及实际拥有外观，不解锁未获取的活动/付费资源。
func ensureProfileCosmetics(p *Progress, legacy AvatarInfo, now time.Time) {
	stamp := float64(now.UnixNano()) / 1e9
	if p.OwnedHeadBox == nil {
		p.OwnedHeadBox = map[int]float64{}
	}
	p.OwnedHeadBox[1] = 0
	p.OwnedHeadBox[3] = 0
	if p.SelectedHeadID == 0 {
		p.SelectedHeadID = legacy.HeadID
		if androidProfile.Heads[p.SelectedHeadID].Kind != 1 {
			p.SelectedHeadID = 1
		}
		if _, ok := p.OwnedHeadBox[p.SelectedHeadID]; !ok {
			p.OwnedHeadBox[p.SelectedHeadID] = 0
		}
	}
	if p.SelectedHeadBoxID == 0 {
		p.SelectedHeadBoxID = legacy.HeadBoxID
		if androidProfile.Heads[p.SelectedHeadBoxID].Kind != 2 {
			p.SelectedHeadBoxID = 3
		}
		if _, ok := p.OwnedHeadBox[p.SelectedHeadBoxID]; !ok {
			p.OwnedHeadBox[p.SelectedHeadBoxID] = 0
		}
	}
	for id, rule := range androidProfile.Heads {
		// 本版有目标的头像全部绑定base_target 1000（登录事件1/参数1）。
		// 此方法由已鉴权玩家初始化调用；只按本版已确认目标解锁，不能按lock_type全解锁。
		if target, known := androidProfile.HeadTargets[rule.Target]; known && target.Type == 1 && len(target.Params) == 1 && target.Params[0] == 1 {
			if _, exists := p.OwnedHeadBox[id]; !exists {
				if rule.LimitHours == 0 {
					p.OwnedHeadBox[id] = 0
				} else {
					p.OwnedHeadBox[id] = float64(now.Unix())
				}
			}
		}
		if rule.Dress == 0 || rule.Target != 0 {
			continue
		}
		for _, card := range p.Cards {
			if containsInt(ownedCardDresses(*p, card.CardID), rule.Dress) {
				if _, exists := p.OwnedHeadBox[id]; !exists {
					if rule.LimitHours == 0 {
						p.OwnedHeadBox[id] = 0
					} else {
						p.OwnedHeadBox[id] = float64(now.Unix())
					}
				}
				break
			}
		}
	}
	for id, acquired := range p.OwnedHeadBox {
		rule, known := androidProfile.Heads[id]
		if !known || acquired < 0 || math.IsNaN(acquired) || math.IsInf(acquired, 0) || (rule.LimitHours > 0 && acquired > 0 && acquired+rule.LimitHours*3600 <= stamp) {
			delete(p.OwnedHeadBox, id)
		}
	}
	if _, ok := p.OwnedHeadBox[p.SelectedHeadID]; !ok {
		p.SelectedHeadID = 1
	}
	if _, ok := p.OwnedHeadBox[p.SelectedHeadBoxID]; !ok {
		p.SelectedHeadBoxID = 3
	}
}

func profileCosmeticProperties(av Avatar, now time.Time) map[string]any {
	p := CloneProgress(av.Progress)
	ensureProfileCosmetics(&p, av.Info, now)
	url := av.Info.CustomHeadImageURL
	if av.Progress.CustomHeadCleared {
		url = ""
	}
	return map[string]any{"owned_head_box": p.OwnedHeadBox, "head_id": p.SelectedHeadID, "preset_head_id": p.SelectedHeadID, "head_box_id": p.SelectedHeadBoxID, "custom_head_image_url": url}
}

func (s *Service) profileCosmeticRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("头像操作需要玩家状态")
	}
	callback, id := 0, 0
	if method == "update_head_box_by_client" {
		if len(args) != 0 {
			return nil, errors.New("头像过期刷新不接受参数")
		}
	} else {
		var ok bool
		callback, ok = callbackArg(args)
		if !ok || len(args) != 2 || json.Unmarshal(args[1], &id) != nil || id <= 0 {
			return nil, errors.New("头像选择参数无效")
		}
	}
	legacy := c.SelectedAvatarUnsafe().Info
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		ensureProfileCosmetics(p, legacy, s.Now())
		if method == "update_head_box_by_client" {
			return nil
		}
		kind, label := 1, "HEAD"
		if method == "change_head_box" {
			kind, label = 2, "HEAD_BOX"
		}
		if androidProfile.Heads[id].Kind != kind {
			return runeReject("RET_AVATAR_"+label+"_NOT_EXIST", "头像资源类型无效")
		}
		if _, owned := p.OwnedHeadBox[id]; !owned {
			return runeReject("RET_AVATAR_"+label+"_NOT_OWNED", "头像资源尚未拥有或已经过期")
		}
		if kind == 1 {
			p.SelectedHeadID = id
			p.CustomHeadCleared = true
		} else {
			p.SelectedHeadBoxID = id
		}
		return nil
	})
	if err != nil {
		return growthCallbackError(callback, err)
	}
	props := profileCosmeticProperties(c.SelectedAvatarUnsafe(), s.Now())
	out := []Push{}
	for _, name := range []string{"owned_head_box", "head_id", "preset_head_id", "head_box_id", "custom_head_image_url"} {
		out = append(out, push("Avatar", "client_prop_changed", []any{name, props[name]}))
	}
	if method != "update_head_box_by_client" {
		out = append(out, Callback(callback, []any{RetSuccess}))
	}
	return out, nil
}

// 旧存档仅能迁移当前拥有卡；已在旧版删除且无历史记录的卡无法凭空恢复首次获取历史。
func ensureObtainedCardHistory(p *Progress) {
	if p.ObtainedCardIDs == nil {
		p.ObtainedCardIDs = map[int]int64{}
	}
	for _, card := range p.Cards {
		if _, known := p.ObtainedCardIDs[card.CardID]; !known {
			p.ObtainedCardIDs[card.CardID] = card.Time
		}
	}
}

func appendOwnedCard(p *Progress, card Card) bool {
	ensureObtainedCardHistory(p)
	_, known := p.ObtainedCardIDs[card.CardID]
	if !known {
		p.ObtainedCardIDs[card.CardID] = card.Time
	}
	p.Cards = append(p.Cards, card)
	return !known
}
