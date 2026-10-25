package game

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
)

// 原生0F9F9ACB：无参上行，on_get_recommend_leagues接收真实league_info列表。
// 推荐采用既有搜索的UID升序/最多100条；只读主档候选窗口，不锁全服存档。
func (s *Service) recommendLeagues(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 0 {
		return nil, errors.New("学会推荐需要玩家状态且不带参数")
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil, errors.New("存储不支持学会查询")
	}
	rows, err := store.SocialAvatars(ctx, SocialSearch{Hostnum: c.hostnum, LeagueOwnersOnly: true, Limit: 1000})
	if err != nil {
		return nil, err
	}
	states := []*LeagueState{}
	ids := []string{}
	seen := map[string]bool{}
	for _, av := range rows {
		for _, state := range av.Progress.OwnedLeagues {
			if state == nil || state.Dissolved || state.Host != c.hostnum || len(state.Members) >= leagueData("league_level", state.Level).integer("member_limit") {
				continue
			}
			states = append(states, state)
			if !seen[state.President] {
				seen[state.President] = true
				ids = append(ids, state.President)
			}
		}
	}
	sort.Slice(states, func(i, j int) bool {
		if states[i].UID == states[j].UID {
			return states[i].ID < states[j].ID
		}
		return states[i].UID < states[j].UID
	})
	if len(states) > 100 {
		states = states[:100]
	}
	avatars := map[string]*Avatar{}
	if len(ids) > 0 {
		presidents, e := store.SocialAvatars(ctx, SocialSearch{OIDs: ids, Hostnum: c.hostnum, Limit: 1000})
		if e != nil {
			return nil, e
		}
		for i := range presidents {
			avatars[hexOf(presidents[i].OID)] = &presidents[i]
		}
	}
	list := []any{}
	for _, state := range states {
		// 推荐只带公开资料；不向非成员暴露内部公告、申请者和成员存档。
		public := *state
		public.PrivateNotice = ""
		public.Applications = nil
		public.Members = nil
		info := leagueInfoWire(&public, avatars)
		info["member_count"] = len(state.Members)
		list = append(list, info)
	}
	return []Push{push("Avatar", "on_get_recommend_leagues", list)}, nil
}
