package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"
)

type SyncPvpMeta struct {
	Period       int                   `json:"period"`
	BeginPeriod  int                   `json:"begin_period"`
	WeeklyClaims map[int]bool          `json:"weekly_claims,omitempty"`
	History      map[int]SyncPvpPeriod `json:"history,omitempty"`
}
type SyncPvpPeriod struct {
	Score         int  `json:"score"`
	Highest       int  `json:"highest"`
	Wins          int  `json:"wins"`
	DivisionBonus int  `json:"division_bonus"`
	Claimed       bool `json:"claimed"`
	Rank          int  `json:"rank,omitempty"`
	RankBonus     int  `json:"rank_bonus,omitempty"`
}

func ensureSyncPvpPeriod(p *Progress, now time.Time) error {
	period := syncPvpSeasonID(now)
	meta := &p.SyncPvpMeta
	if meta.Period > period {
		return errors.New("同步竞技赛季时钟回拨")
	}
	if meta.WeeklyClaims == nil {
		meta.WeeklyClaims = map[int]bool{}
	}
	if meta.History == nil {
		meta.History = map[int]SyncPvpPeriod{}
	}
	if meta.Period == 0 {
		meta.Period = period
		meta.BeginPeriod = period
		p.SyncPvpSeasonID = period
		return nil
	}
	if meta.Period != period {
		if humanRoomActive(p.Social.HumanRoom) && p.Social.HumanRoom.Rated && p.Battle != nil && !p.Battle.Finished {
			return nil
		}
		if p.SyncPvpMatch != nil && p.SyncPvpMatch.Status == syncPvpMatchReady && p.Battle != nil && !p.Battle.Finished {
			return nil
		}
		rule, err := syncPvpScoreRuleFor(p.SyncPvpScore)
		if err != nil {
			return err
		}
		if _, exists := meta.History[meta.Period]; !exists {
			meta.History[meta.Period] = SyncPvpPeriod{Score: p.SyncPvpScore, Highest: p.SyncPvpHighestScore, Wins: p.SyncPvpWeeklyWins, DivisionBonus: rule.DivisionBonus}
		}
		meta.Period = period
		meta.WeeklyClaims = map[int]bool{}
		p.SyncPvpWeeklyWins = 0
		p.SyncPvpWinStreak = 0
	}
	p.SyncPvpSeasonID = period
	return nil
}
func pvpExtraProperties(p Progress) map[string]any {
	bonusID := 0
	flag := false
	latest := 0
	for period, record := range p.SyncPvpMeta.History {
		if !record.Claimed && period > latest {
			latest = period
			bonusID = record.DivisionBonus
			flag = true
		}
	}
	return map[string]any{"sync_weekly_win_bonus": syncWeeklyClaimProperties(p.SyncPvpMeta.WeeklyClaims), "sync_pvp_season_bonus_id": bonusID, "sync_pvp_season_bonus_flag": flag, "asyn_pvp_cards": battleSlotWire(p.AsyncPvp.Defence), "asyn_pvp_score": p.AsyncPvp.Score, "asyn_pvp_max_score": p.AsyncPvp.MaxScore, "asyn_pvp_auto": p.AsyncPvp.Auto, "asyn_pvp_rank": p.AsyncPvp.Rank}
}
func syncWeeklyClaimProperties(v map[int]bool) map[int]int {
	out := map[int]int{}
	for id, claimed := range v {
		if claimed {
			out[id] = 1
		} else {
			out[id] = 0
		}
	}
	return out
}

// grantPvpFixedBonus 的金额只来自Android bonus.fixed_items。随机奖励不猜值。
func grantPvpFixedBonus(p *Progress, bid int, now time.Time) (map[int]int64, error) {
	raw, ok := socialCatalog.Tables["bonus"][intString(bid)]
	if !ok {
		return nil, errors.New("竞技奖励目录缺失")
	}
	var row struct {
		Fixed  [][]int64         `json:"fixed_items"`
		Random []json.RawMessage `json:"random_items"`
		Runes  []json.RawMessage `json:"random_runes"`
		Libs   []json.RawMessage `json:"random_item_libs"`
		Inner  json.RawMessage   `json:"inner_bonus_id"`
	}
	if json.Unmarshal(raw, &row) != nil || len(row.Random)+len(row.Runes)+len(row.Libs) > 0 || (len(row.Inner) > 0 && string(row.Inner) != "null") {
		return nil, errors.New("竞技动态奖励尚无取证")
	}
	rewards := map[int]int64{}
	cards := []string{}
	for _, entry := range row.Fixed {
		if len(entry) != 2 || entry[0] <= 0 || entry[1] <= 0 || rewards[int(entry[0])] > math.MaxInt64-entry[1] {
			return nil, errors.New("竞技奖励数值无效")
		}
		if err := grantShopItem(p, int(entry[0]), entry[1], now, rewards, &cards, 0); err != nil {
			return nil, err
		}
	}
	return rewards, nil
}
func (s *Service) receiveSyncPvpWeekly(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if err := s.refreshPvpAwards(ctx, c); err != nil {
		return nil, err
	}
	cb, ok := callbackArg(args)
	var count int
	if c.phase != Playing || !ok || len(args) != 2 || json.Unmarshal(args[1], &count) != nil {
		return nil, errors.New("周胜奖励签名为回调、胜利次数")
	}
	var row struct {
		Wins  int `json:"win_count"`
		Bonus int `json:"bonus_id"`
	}
	raw, known := socialCatalog.Tables["sync_pvp_weekly_win_bonus"][intString(count)]
	if !known || json.Unmarshal(raw, &row) != nil {
		return []Push{Callback(cb, []any{nil, "周胜奖励档位不存在"})}, nil
	}
	var rewards map[int]int64
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if err := ensureSyncPvpPeriod(p, s.Now()); err != nil {
			return err
		}
		if p.SyncPvpWeeklyWins < count {
			return errors.New("本期胜利次数不足")
		}
		if p.SyncPvpMeta.WeeklyClaims[count] {
			return errors.New("该周胜奖励已领取")
		}
		var err error
		rewards, err = grantPvpFixedBonus(p, row.Bonus, s.Now())
		if err != nil {
			return err
		}
		p.SyncPvpMeta.WeeklyClaims[count] = true
		return nil
	})
	if err != nil {
		return []Push{Callback(cb, []any{nil, err.Error()})}, nil
	}
	p := c.SelectedAvatarUnsafe().Progress
	return []Push{materialManagerPush(c), push("Avatar", "client_prop_changed", []any{"sync_weekly_win_bonus", syncWeeklyClaimProperties(p.SyncPvpMeta.WeeklyClaims)}), Callback(cb, []any{map[string]any{"__custom_type": "box.box", "materials": rewards}, ""})}, nil
}

// receiveSyncPvpSeason 发放已结束且本角色有参赛快照的分段奖；排名奖必须有结期全服快照，不伪造历史排名。
func (s *Service) receiveSyncPvpSeason(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if err := s.refreshPvpAwards(ctx, c); err != nil {
		return nil, err
	}
	if c.phase != Playing || len(args) != 0 {
		return nil, errors.New("同步赛季奖励请求不带参数")
	}
	rewards := map[int]int64{}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureSyncPvpPeriod(p, s.Now()); e != nil {
			return e
		}
		periods := []int{}
		for period, row := range p.SyncPvpMeta.History {
			if !row.Claimed && period < syncPvpSeasonID(s.Now()) {
				periods = append(periods, period)
			}
		}
		sort.Ints(periods)
		if len(periods) == 0 {
			return errors.New("没有未领取的已结束赛季奖励")
		}
		for _, period := range periods {
			row := p.SyncPvpMeta.History[period]
			bonuses := []int{row.DivisionBonus}
			if row.RankBonus > 0 {
				bonuses = append(bonuses, row.RankBonus)
			}
			for _, bonus := range bonuses {
				part, e := grantPvpFixedBonus(p, bonus, s.Now())
				if e != nil {
					return e
				}
				for id, count := range part {
					if rewards[id] > math.MaxInt64-count {
						return errors.New("赛季奖励合计溢出")
					}
					rewards[id] += count
				}
			}
			row.Claimed = true
			p.SyncPvpMeta.History[period] = row
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := []Push{materialManagerPush(c), push("Avatar", "client_prop_changed", []any{"card_common_mgr", cardCommonMgrPropertiesWithProgress(c.SelectedAvatarUnsafe().Progress)})}
	cosmetics := profileCosmeticProperties(c.SelectedAvatarUnsafe(), s.Now())
	keys := []string{}
	for key := range cosmetics {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, push("Avatar", "client_prop_changed", []any{key, cosmetics[key]}))
	}
	return append(out, push("Avatar", "client_prop_changed", []any{"sync_pvp_season_bonus_flag", false}), push("Avatar", "client_prop_changed", []any{"sync_pvp_season_bonus_id", 0}), push("Avatar", "on_receive_sync_pvp_season_bonus", map[string]any{"__custom_type": "box.box", "materials": rewards})), nil
}
