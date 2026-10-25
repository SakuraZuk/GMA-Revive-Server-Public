package game

import (
	"context"
	"encoding/json"
)

// LeagueTransaction 限定学会写入涉及的角色。其他角色仅供名称、编号和成员资料查询。
// 返回值只能包含完整存档，展示投影不得替换在线连接或写回数据库。
type LeagueTransaction struct {
	SelfID       string
	TargetLeague string
	TargetMember string
	ReadOnly     bool
}

type LeagueAccounts interface {
	UpdateLeagueSocial(context.Context, LeagueTransaction, func(map[string]*Avatar) error) ([]Avatar, error)
}

func leagueTransaction(self, method string, args []json.RawMessage) LeagueTransaction {
	q := LeagueTransaction{SelfID: self}
	q.ReadOnly = method == "query_total_league_info" || method == "search_league"
	if len(args) > 0 {
		var id string
		if json.Unmarshal(args[0], &id) == nil && validObjectID(id) {
			switch method {
			case "apply_league", "query_total_league_info":
				q.TargetLeague = id
			case "agree_league_apply", "refuse_league_apply", "appoint_league_member":
				q.TargetMember = id
			}
		}
	}
	return q
}

func updateLeagueSocial(ctx context.Context, store SocialAccounts, q LeagueTransaction, fn func(map[string]*Avatar) error) ([]Avatar, error) {
	if scoped, ok := store.(LeagueAccounts); ok {
		return scoped.UpdateLeagueSocial(ctx, q, fn)
	}
	// 内存夹具沿用独占互斥事务；生产数据库必须实现限定角色事务。
	return store.UpdateAllSocial(ctx, fn)
}
