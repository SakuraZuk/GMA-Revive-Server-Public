package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// ActivityRankAccounts 覆盖全服真实成绩，列表限制不限制个人名次查询。
type ActivityRankAccounts interface {
	ActivityRanking(context.Context, int, int, []byte, int, int) ([]ActivityRankEntry, int, error)
}

type ActivityRankEntry struct {
	Avatar   Avatar
	Rank     int
	Score    []int64
	Cards    []any
	SubRanks [3]int
}

func ValidateActivityRank(kind, subID, host, limit int) error {
	if host <= 0 || limit < 1 || limit > 1000 {
		return errors.New("活动排行服务器或数量无效")
	}
	switch kind {
	case 8:
		if subID < 1 || subID > 7 {
			return errors.New("山海排行关卡无效")
		}
	case 10:
		if subID == 0 {
			if !remainingPolicyEnabled() {
				return errors.New("年兽综合榜本服规则未启用")
			}
			return nil
		}
		if subID != 20200001 && subID != 20200002 && subID != 20200003 {
			return errors.New("年兽分榜副本无效")
		}
	default:
		return errors.New("活动排行类型未实现")
	}
	return nil
}

func activityRankValue(av Avatar, kind, subID int) (ActivityRankEntry, bool) {
	if kind == 10 && subID == 0 {
		return FrozenNianTotalRank(av)
	}
	var d ActivityProgress
	if kind == 8 {
		if av.Progress.Activities.Mountain == nil {
			return ActivityRankEntry{}, false
		}
		d = av.Progress.Activities.Mountain.Dungeons[subID]
		if !d.Ranked || d.RankHard <= 0 || d.Actions < 0 {
			return ActivityRankEntry{}, false
		}
		return ActivityRankEntry{Avatar: av, Score: []int64{int64(d.RankHard), -int64(d.Actions)}, Cards: d.Cards}, true
	}
	d = av.Progress.Activities.Nian[subID]
	if !d.Ranked || d.RankDamage < 0 || (d.RankDamage == 0 && (!remainingPolicyEnabled() || d.RankAt <= 0)) {
		return ActivityRankEntry{}, false
	}
	return ActivityRankEntry{Avatar: av, Score: []int64{d.RankDamage}, Cards: d.RankCards}, true
}

func (a *FixtureAccounts) ActivityRanking(ctx context.Context, kind, subID int, oid []byte, host, limit int) ([]ActivityRankEntry, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if err := ValidateActivityRank(kind, subID, host, limit); err != nil {
		return nil, 0, err
	}
	if len(oid) != 12 {
		return nil, 0, errors.New("活动排行角色标识无效")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	rows := []ActivityRankEntry{}
	for _, rec := range a.records {
		for _, av := range rec.Avatars {
			if av.Hostnum == host {
				if entry, ok := activityRankValue(av, kind, subID); ok {
					rows = append(rows, entry)
				}
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		for k := range rows[i].Score {
			if rows[i].Score[k] != rows[j].Score[k] {
				return rows[i].Score[k] > rows[j].Score[k]
			}
		}
		return hexOf(rows[i].Avatar.OID) < hexOf(rows[j].Avatar.OID)
	})
	own := 0
	for i := range rows {
		rows[i].Rank = i + 1
		if string(rows[i].Avatar.OID) == string(oid) {
			own = i + 1
		}
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	for i := range rows {
		rows[i].Avatar.Progress = CloneProgress(rows[i].Avatar.Progress)
	}
	return rows, own, nil
}

func (s *Service) activityRankRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if len(args) != 2 {
		return nil, errors.New("活动排行参数数量无效")
	}
	var kind, subID, cb int
	if method == "query_mountain_sea_own_rank" {
		kind = 8
		if json.Unmarshal(args[0], &cb) != nil || cb <= 0 || json.Unmarshal(args[1], &subID) != nil {
			return nil, errors.New("山海个人名次参数无效")
		}
	} else if json.Unmarshal(args[0], &kind) != nil || json.Unmarshal(args[1], &subID) != nil {
		return nil, errors.New("活动排行参数必须为整数")
	}
	av := c.SelectedAvatarUnsafe()
	if len(av.OID) != 12 {
		return nil, errors.New("活动排行需要登录角色")
	}
	if err := ValidateActivityRank(kind, subID, av.Hostnum, 1000); err != nil {
		return nil, err
	}
	act := 203
	if kind == 10 {
		act = 328
	}
	if _, err := activityOpen(av.Progress, act, av.Info.Level, s.Now()); err != nil {
		return nil, err
	}
	store, ok := s.Accounts.(ActivityRankAccounts)
	if !ok {
		return nil, errors.New("存储未提供全量活动排行")
	}
	rows, own, err := store.ActivityRanking(ctx, kind, subID, av.OID, av.Hostnum, 1000)
	if err != nil {
		return nil, err
	}
	if method == "query_mountain_sea_own_rank" {
		return []Push{Callback(cb, []any{own})}, nil
	}
	infos := []any{}
	for _, entry := range rows {
		info := s.socialInfo(socialProfile(entry.Avatar))
		info["score"] = entry.Score
		if kind == 10 && subID == 0 {
			info["score"] = []int64{-entry.Score[0]}
			info["sub_ranks"] = entry.SubRanks
		}
		info["cards_info"] = entry.Cards
		infos = append(infos, info)
	}
	return []Push{push("Avatar", "on_query_rank_list", fmt.Sprintf("%d&%d", kind, subID), infos)}, nil
}

var _ ActivityRankAccounts = (*FixtureAccounts)(nil)
