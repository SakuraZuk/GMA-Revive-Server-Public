package game

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"sort"
	"strconv"
	"time"
)

//go:embed achievement_catalog.json
var achievementCatalogRaw []byte

type Achievement struct {
	ID      int           `json:"achv_id"`
	Targets map[int]int64 `json:"finished_targets"`
	Claimed bool          `json:"is_bonus"`
	Time    int64         `json:"time"`
}
type achievementRule struct {
	Targets [][]int `json:"target_id"`
	Need    int64   `json:"target_need_count"`
	Bonus   int     `json:"bonus_id"`
	Value   int     `json:"achv_value"`
}
type achievementReward struct {
	Fixed  [][]int64 `json:"fixed"`
	Groups [][]struct {
		Weight int64 `json:"weight"`
		ID     int   `json:"item_id"`
		Count  int64 `json:"count"`
	} `json:"groups"`
}
type achievementCatalog struct {
	Rules   map[int]achievementRule `json:"achievements"`
	Targets map[int]struct {
		Type   int               `json:"target_type"`
		Params []json.RawMessage `json:"target_params"`
	} `json:"targets"`
	Rewards   map[int]achievementReward `json:"rewards"`
	Materials map[int]struct {
		Type   int `json:"type"`
		Sub    int `json:"stype"`
		Target int `json:"target"`
	} `json:"materials"`
}

var androidAchievements = func() achievementCatalog {
	var c achievementCatalog
	if json.Unmarshal(achievementCatalogRaw, &c) != nil || len(c.Rules) != 254 {
		panic("Android成就目录无效")
	}
	return c
}()

// 组内求和，组间取最小值，按原生achv.get_count执行。
func achievementCount(a Achievement, r achievementRule) int64 {
	if len(r.Targets) == 0 {
		return 0
	}
	minimum := int64(math.MaxInt64)
	for _, group := range r.Targets {
		sum := int64(0)
		for _, id := range group {
			v := a.Targets[id]
			if v < 0 {
				return 0
			}
			if sum > math.MaxInt64-v {
				return math.MaxInt64
			}
			sum += v
		}
		if sum < minimum {
			minimum = sum
		}
	}
	return minimum
}

func ensureAchievements(p *Progress) {
	if p.Achievements == nil {
		p.Achievements = map[int]Achievement{}
	}
	for id := range androidAchievements.Rules {
		if _, ok := p.Achievements[id]; !ok {
			p.Achievements[id] = Achievement{ID: id, Targets: map[int]int64{}}
		}
	}
}

func achievementProperties(p Progress) map[string]any {
	out := map[string]any{}
	for id := range androidAchievements.Rules {
		a, ok := p.Achievements[id]
		if !ok {
			a = Achievement{ID: id, Targets: map[int]int64{}}
		}
		out[strconv.Itoa(id)] = a
	}
	return out
}
func achievementPoints(p Progress) int {
	total := 0
	for id, a := range p.Achievements {
		r, ok := androidAchievements.Rules[id]
		if ok && achievementCount(a, r) >= r.Need {
			total += r.Value
		}
	}
	return total
}

// 事件参数保留原生整数/字符串类型，只由成功的服务端事务推进。
func advanceAchievementEvent(p *Progress, eventType, param int, now time.Time) bool {
	return advanceAchievementAmount(p, eventType, param, 1, now)
}

func advanceAchievementAmount(p *Progress, eventType int, param any, amount int64, now time.Time) bool {
	if amount <= 0 {
		return false
	}
	// nil 对应原生空 target_params；标量仍为单参数，切片显式传递完整参数序列。
	params := []any{}
	switch value := param.(type) {
	case nil:
	case []any:
		params = value
	case []int:
		for _, item := range value {
			params = append(params, item)
		}
	default:
		params = append(params, value)
	}
	want, err := json.Marshal(params)
	if err != nil {
		return false
	}
	advanceBasicRewardEvent(p, eventType, params, amount, now)
	ensureAchievements(p)
	changed := false
	for tid, target := range androidAchievements.Targets {
		actual, err := json.Marshal(target.Params)
		if target.Type != eventType || err != nil || string(actual) != string(want) {
			continue
		}
		for id, r := range androidAchievements.Rules {
			matched := false
			for _, group := range r.Targets {
				if containsInt(group, tid) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			a := p.Achievements[id]
			if a.Targets == nil {
				a.Targets = map[int]int64{}
			}
			if a.Targets[tid] < r.Need {
				if amount >= r.Need-a.Targets[tid] {
					a.Targets[tid] = r.Need
				} else {
					a.Targets[tid] += amount
				}
				changed = true
			}
			if a.Time == 0 && achievementCount(a, r) >= r.Need {
				a.Time = now.Unix()
			}
			p.Achievements[id] = a
		}
	}
	return changed
}

func recordAchievementLogin(p *Progress, now time.Time) error {
	day := now.In(time.FixedZone("北京时间", 8*3600)).Format("2006-01-02")
	if p.AchievementLoginDay > day {
		return errors.New("登录成就日界发生时钟回拨")
	}
	ensureAchievements(p)
	if p.AchievementLoginDay != day {
		advanceAchievementEvent(p, 1, 1, now)
		p.AchievementLoginDay = day
	}
	return nil
}

// 只投影原生表和描述已明确的拥有数量/首次阈值；已完成的历史不倒扣。
// 积分目标按实际完成积分迭代，最多每个成就新完成一次，不使用事件累加。
func reconcileAchievementState(p *Progress, avatarLevel int, now time.Time) {
	ensureAchievements(p)
	intimacy, err := loadIntimacyCatalog()
	if err != nil {
		panic("好感度目录无效")
	}
	for pass := 0; pass <= len(androidAchievements.Rules); pass++ {
		changed := false
		points := achievementPoints(*p)
		for id, r := range androidAchievements.Rules {
			a := p.Achievements[id]
			for _, group := range r.Targets {
				for _, tid := range group {
					target := androidAchievements.Targets[tid]
					var threshold int
					count := int64(0)
					if target.Type == 14 {
						// 本版三项目标均为 [0,星级]：任一自有幻书四位置全部佩戴到阈值。
						var cardID int
						if len(target.Params) != 2 || json.Unmarshal(target.Params[0], &cardID) != nil || cardID != 0 || json.Unmarshal(target.Params[1], &threshold) != nil || threshold <= 0 {
							continue
						}
						count = achievementFullRuneCards(*p, threshold)
					} else {
						if len(target.Params) != 1 || json.Unmarshal(target.Params[0], &threshold) != nil || threshold <= 0 {
							continue
						}
						switch target.Type {
						case 3:
							if threshold != 1 {
								continue
							}
							count = int64(avatarLevel)
						case 13:
							if threshold != 1 {
								continue
							}
							count = int64(points)
						case 15, 16:
							for _, card := range p.Cards {
								if (target.Type == 15 && card.Level >= threshold) || (target.Type == 16 && card.Grade >= threshold) {
									count++
								}
							}
						case 17:
							for _, rune := range p.Runes {
								if rune.Level-1 >= threshold {
									count++
								}
							}
						case 74:
							for cardID, value := range p.Intimacy {
								if ownsCardID(*p, cardID) && intimacyLevel(value, intimacy) >= threshold {
									count++
								}
							}
						default:
							continue
						}
					}
					if count > r.Need {
						count = r.Need
					}
					if count > a.Targets[tid] {
						if a.Targets == nil {
							a.Targets = map[int]int64{}
						}
						a.Targets[tid] = count
						changed = true
					}
				}
			}
			if a.Time == 0 && achievementCount(a, r) >= r.Need {
				a.Time = now.Unix()
			}
			p.Achievements[id] = a
		}
		if !changed {
			return
		}
	}
}

// 成就奖励完整展开即时礼物盒；扣领奖标记与全部资产在同一玩家事务。
func grantAchievementReward(p *Progress, bid int, now time.Time, materials map[int]int64, cardUUIDs *[]string, depth int) error {
	if depth > 8 {
		return errors.New("成就奖励嵌套超过限制")
	}
	r, ok := androidAchievements.Rewards[bid]
	if !ok {
		return errors.New("成就奖励目录缺失")
	}
	items := map[int]int64{}
	add := func(id int, n int64) error {
		if id <= 0 || n <= 0 || items[id] > math.MaxInt64-n {
			return errors.New("成就奖励数量无效")
		}
		items[id] += n
		return nil
	}
	for _, row := range r.Fixed {
		if len(row) != 2 {
			return errors.New("成就奖励结构无效")
		}
		if err := add(int(row[0]), row[1]); err != nil {
			return err
		}
	}
	for _, group := range r.Groups {
		total := int64(0)
		for _, candidate := range group {
			if candidate.Weight <= 0 || total > math.MaxInt64-candidate.Weight {
				return errors.New("成就奖励权重无效")
			}
			total += candidate.Weight
		}
		if total == 0 {
			return errors.New("成就奖励物品库为空")
		}
		draw, err := rand.Int(rand.Reader, big.NewInt(total))
		if err != nil {
			return err
		}
		value := draw.Int64()
		for _, candidate := range group {
			if value < candidate.Weight {
				if err := add(candidate.ID, candidate.Count); err != nil {
					return err
				}
				break
			}
			value -= candidate.Weight
		}
	}
	ids := []int{}
	for id := range items {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		n := items[id]
		m, ok := androidAchievements.Materials[id]
		if !ok {
			return errors.New("成就奖励材料缺失")
		}
		switch m.Type {
		case 7:
			if m.Sub != 1 || n > 1000 {
				return errors.New("成就礼物盒展开数量无效")
			}
			for i := int64(0); i < n; i++ {
				if err := grantAchievementReward(p, m.Target, now, materials, cardUUIDs, depth+1); err != nil {
					return err
				}
			}
		case 8:
			if n > int64(androidCompose.MaxCards-len(p.Cards)) || m.Target <= 0 {
				return runeReject("RET_CARD_COUNT_REACH_MAX", "成就幻书奖励容量不足")
			}
			if _, ok := androidOath.Cards[m.Target]; !ok {
				return errors.New("成就幻书奖励不存在")
			}
			for i := int64(0); i < n; i++ {
				card := newCard(m.Target, 1, now)
				appendOwnedCard(p, card)
				*cardUUIDs = append(*cardUUIDs, card.UUID)
			}
		case 9:
			head, ok := androidProfile.Heads[m.Target]
			if !ok {
				return errors.New("成就头像框奖励不存在")
			}
			if p.OwnedHeadBox == nil {
				p.OwnedHeadBox = map[int]float64{}
			}
			if head.LimitHours == 0 {
				p.OwnedHeadBox[m.Target] = 0
			} else {
				if _, owned := p.OwnedHeadBox[m.Target]; owned {
					return errors.New("限时头像框重复奖励续期语义待取证")
				}
				p.OwnedHeadBox[m.Target] = float64(now.Unix())
			}
		case 3, 4:
			mat := p.Materials[id]
			if mat.Count < 0 || mat.Total < 0 || mat.Count > math.MaxInt64-n || mat.Total > math.MaxInt64-n || materials[id] > math.MaxInt64-n {
				return errors.New("成就奖励资产溢出")
			}
			if p.Materials == nil {
				p.Materials = map[int]Material{}
			}
			mat.ID = id
			mat.Count += n
			mat.Total += n
			p.Materials[id] = mat
			materials[id] += n
		default:
			return errors.New("成就奖励材料类型尚未取证")
		}
	}
	return nil
}

func (s *Service) achievementRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("成就领取需要回调和编号")
	}
	callback, ok := callbackArg(args)
	if !ok {
		return nil, errors.New("成就回调无效")
	}
	ids := []int{}
	if method == "receive_achv_bonus" {
		var id int
		if json.Unmarshal(args[1], &id) != nil || id <= 0 {
			return nil, errors.New("成就编号无效")
		}
		ids = append(ids, id)
	} else {
		var list intList
		if json.Unmarshal(args[1], &list) != nil || len(list) == 0 || len(list) > len(androidAchievements.Rules) {
			return nil, errors.New("成就批量列表无效")
		}
		ids = []int(list)
	}
	materials := map[int]int64{}
	cardUUIDs := []string{}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		ensureAchievements(p)
		seen := map[int]bool{}
		for _, id := range ids {
			if seen[id] {
				return errors.New("成就批量编号重复")
			}
			seen[id] = true
			r, known := androidAchievements.Rules[id]
			a := p.Achievements[id]
			if !known {
				return runeReject("RET_ACHV_NOT_EXIST", "成就不存在")
			}
			if achievementCount(a, r) < r.Need {
				return runeReject("RET_ACHV_NOT_FINISH", "成就未完成")
			}
			if r.Bonus == 0 {
				return runeReject("RET_ACHV_NO_BONUS", "成就没有奖励")
			}
			if a.Claimed {
				return runeReject("RET_ACHV_HAS_RECV", "成就奖励已领取")
			}
			if err := grantAchievementReward(p, r.Bonus, s.Now(), materials, &cardUUIDs, 0); err != nil {
				return err
			}
			a.Claimed = true
			p.Achievements[id] = a
		}
		return nil
	})
	box := map[string]any{"__custom_type": "box.box", "materials": materials}
	if err != nil {
		pushes, e := growthCallbackError(callback, err)
		if e != nil {
			return nil, e
		}
		code := pushes[0].Args[1].([]any)[0]
		return []Push{Callback(callback, []any{code, map[string]any{"__custom_type": "box.box", "materials": map[int]int64{}}})}, nil
	}
	p := c.SelectedAvatarUnsafe().Progress
	box["cards"] = cardListWire(&p, cardUUIDs)
	return []Push{materialManagerPush(c), cardMgrPush(c), push("Avatar", "client_prop_changed", []any{"owned_head_box", p.OwnedHeadBox}), push("Avatar", "client_prop_changed", []any{"achves", achievementProperties(p)}), push("Avatar", "client_prop_changed", []any{"achv_value", achievementPoints(p)}), Callback(callback, []any{RetSuccess, box})}, nil
}
