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
	"time"
)

//go:embed collection_remaining_catalog.json
var collectionRemainingCatalogRaw []byte

var androidCollectionRemaining = func() struct {
	Windows map[int]struct {
		Begin int `json:"begin_time"`
		End   int `json:"end_time"`
		Count int `json:"random_num"`
	} `json:"windows"`
	Rules map[int][]struct {
		Mood      int `json:"mood"`
		Bonus     int `json:"bonus"`
		RawSecond int `json:"raw_second"`
	} `json:"rules"`
	Cards map[int]struct {
		Max        float64   `json:"mood_max"`
		Rate       float64   `json:"mood_transform_rate"`
		Thresholds []float64 `json:"mood_threshold"`
	} `json:"cards"`
	EnergyCosts    map[int]map[int]float64 `json:"energy_costs"`
	EnergyPerCard  float64                 `json:"energy_per_card"`
	EnergyInterval float64                 `json:"energy_interval"`
} {
	var c struct {
		Windows map[int]struct {
			Begin int `json:"begin_time"`
			End   int `json:"end_time"`
			Count int `json:"random_num"`
		} `json:"windows"`
		Rules map[int][]struct {
			Mood      int `json:"mood"`
			Bonus     int `json:"bonus"`
			RawSecond int `json:"raw_second"`
		} `json:"rules"`
		Cards map[int]struct {
			Max        float64   `json:"mood_max"`
			Rate       float64   `json:"mood_transform_rate"`
			Thresholds []float64 `json:"mood_threshold"`
		} `json:"cards"`
		EnergyCosts    map[int]map[int]float64 `json:"energy_costs"`
		EnergyPerCard  float64                 `json:"energy_per_card"`
		EnergyInterval float64                 `json:"energy_interval"`
	}
	if json.Unmarshal(collectionRemainingCatalogRaw, &c) != nil || len(c.Rules) != 79 || len(c.Windows) != 2 {
		panic("收藏室住客与能量原生目录无效")
	}
	return c
}()

type CollectionResidentGift struct {
	Card      int  `json:"card"`
	Room      int  `json:"room"`
	Bonus     int  `json:"bonus"`
	RawSecond int  `json:"raw_second"`
	Frozen    Mail `json:"frozen"`
	Claimed   bool `json:"claimed"`
}

func collectionResidentWindow(now time.Time) int {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	seconds := local.Hour()*3600 + local.Minute()*60 + local.Second()
	for id, w := range androidCollectionRemaining.Windows {
		if seconds >= w.Begin && seconds <= w.End {
			return id
		}
	}
	return 0
}

func collectionMoodStatus(p Progress, room, cid int) int {
	c, known := androidCollectionRemaining.Cards[cid]
	if !known {
		return 1
	}
	mood := math.Min(c.Max, math.Max(0, float64(collectionComfort(p, room))*c.Rate))
	last := -1
	for i, threshold := range c.Thresholds {
		if mood < threshold {
			break
		}
		if threshold >= 0 {
			last = i
		}
	}
	if last >= 0 {
		return last + 2
	}
	return 1
}

func collectionFreezeResidentBonus(p Progress, bonus int, now time.Time) (Mail, error) {
	scratch := CloneProgress(p)
	scratch.Cards = nil
	scratch.Runes = nil
	scratch.OwnedHeadBox = nil
	scratch.Power.Value = 0
	for id, m := range scratch.Materials {
		m.Count = 0
		scratch.Materials[id] = m
	}
	box, err := grantNativeBonus(&scratch, bonus, 1, max(p.AvatarLevel, 1), now)
	if err != nil {
		return Mail{}, err
	}
	changes, ok := box["materials"].(map[int]int64)
	if !ok {
		return Mail{}, errors.New("住客奖励冻结材料类型无效")
	}
	reward := Mail{State: MailUnclaimed, Attachments: changes, Cards: scratch.Cards, Runes: scratch.Runes, RewardsFrozen: true}
	if err := validateMailAssets(reward); err != nil {
		return Mail{}, err
	}
	return reward, nil
}

// 每个窗口只规划一次，实际随机箱在规划事务内冻结，背包不足的重试也不会重新抽取。
func collectionGenerateResidentGifts(p *Progress, now time.Time) error {
	if !remainingPolicyEnabled() {
		return nil
	}
	st := p.Collection
	day := collectionDay(now)
	if st.ResidentDay > day {
		return errors.New("住客奖励日期回拨")
	}
	if st.ResidentDay != day {
		st.ResidentDay = day
		st.ResidentWindows = map[int]map[int]CollectionResidentGift{}
	}
	if st.ResidentWindows == nil {
		st.ResidentWindows = map[int]map[int]CollectionResidentGift{}
	}
	window := collectionResidentWindow(now)
	if window == 0 {
		return nil
	}
	if _, planned := st.ResidentWindows[window]; planned {
		return nil
	}
	st.ResidentWindows[window] = map[int]CollectionResidentGift{}
	if _, unlocked := p.UnlockSystems["house_daily_random_reward"]; !unlocked {
		return nil
	}
	candidates := []CollectionResidentGift{}
	seen := map[int]bool{}
	for room, r := range st.Rooms {
		for _, cid := range r.Slots {
			if seen[cid] || !ownsCardID(*p, cid) {
				continue
			}
			seen[cid] = true
			mood := collectionMoodStatus(*p, room, cid)
			selected := -1
			for i, rule := range androidCollectionRemaining.Rules[cid] {
				if rule.Mood <= mood && (selected < 0 || rule.Mood > androidCollectionRemaining.Rules[cid][selected].Mood) {
					selected = i
				}
			}
			if selected >= 0 {
				rule := androidCollectionRemaining.Rules[cid][selected]
				candidates = append(candidates, CollectionResidentGift{Card: cid, Room: room, Bonus: rule.Bonus, RawSecond: rule.RawSecond})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Card < candidates[j].Card })
	count := min(androidCollectionRemaining.Windows[window].Count, len(candidates))
	for i := 0; i < count; i++ {
		draw, err := rand.Int(rand.Reader, big.NewInt(int64(len(candidates))))
		if err != nil {
			return err
		}
		index := int(draw.Int64())
		gift := candidates[index]
		gift.Frozen, err = collectionFreezeResidentBonus(*p, gift.Bonus, now)
		if err != nil {
			return err
		}
		st.ResidentWindows[window][gift.Card] = gift
		candidates = append(candidates[:index], candidates[index+1:]...)
	}
	return nil
}

func collectionResidentProperties(p Progress, now time.Time) map[int]map[int][]any {
	result := map[int]map[int][]any{}
	if p.Collection == nil || p.Collection.ResidentDay != collectionDay(now) {
		return result
	}
	for window, gifts := range p.Collection.ResidentWindows {
		result[window] = map[int][]any{}
		for cid, g := range gifts {
			state := 1
			if g.Claimed {
				state = 0
			}
			result[window][cid] = []any{state}
		}
	}
	return result
}

func (s *Service) collectionResidentRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 || !remainingPolicyEnabled() {
		return nil, errors.New("住客随机礼物接口需要已启用的玩家状态")
	}
	var window, cid int
	if json.Unmarshal(args[0], &window) != nil || json.Unmarshal(args[1], &cid) != nil || window <= 0 || cid <= 0 {
		return nil, errors.New("住客窗口与幻书参数无效")
	}
	now := s.Now()
	var reward Mail
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if err := ensureCollection(p, now); err != nil {
			return err
		}
		st := p.Collection
		gift, exists := st.ResidentWindows[window][cid]
		if !exists || st.ResidentDay != collectionDay(now) || window != collectionResidentWindow(now) || gift.Claimed {
			return errors.New("住客随机礼物不存在、过期或已领取")
		}
		room, exists := st.Rooms[gift.Room]
		living := false
		for _, id := range room.Slots {
			living = living || id == cid
		}
		if !exists || !living || !ownsCardID(*p, cid) {
			return errors.New("礼物幻书已经离开原房间")
		}
		if st.ResidentGiftCount < 0 || st.ResidentGiftCount == math.MaxInt64 {
			return errors.New("住客奖励累计次数溢出")
		}
		reward = gift.Frozen
		if err := claimMail(p, &gift.Frozen, now.Unix()); err != nil {
			return err
		}
		st = p.Collection
		gift.Claimed = true
		st.ResidentWindows[window][cid] = gift
		st.ResidentGiftCount++
		for _, threshold := range []int{1, 30, 100} {
			if st.ResidentGiftCount == int64(threshold) {
				advanceAchievementEvent(p, 1006, threshold, now)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := []Push{materialManagerPush(c), cardMgrPush(c), runePush(c), knowledgePush(c)}
	out = append(out, collectionPushes(c.SelectedAvatarUnsafe().Progress, now, false)...)
	return append(out, push("Avatar", "on_player_get_house_random_reward", RetSuccess, mailAttachmentBox(reward))), nil
}
