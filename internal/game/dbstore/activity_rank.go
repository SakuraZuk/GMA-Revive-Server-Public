package dbstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"hs-server/internal/game"
)

// 山海：真实已通关hard优先、实际AP次优；年兽：只选同日有效成绩。
// COUNT与页面在同一repeatable-read快照中，限制页面不能截断全服个人名次。
func (s *Store) ActivityRanking(ctx context.Context, kind, subID int, oid []byte, host, limit int) ([]game.ActivityRankEntry, int, error) {
	if err := game.ValidateActivityRank(kind, subID, host, limit); err != nil {
		return nil, 0, err
	}
	if len(oid) != 12 {
		return nil, 0, errors.New("活动排行角色标识无效")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, 0, err
	}
	defer rollback(tx)
	if kind == 10 && subID == 0 {
		path := []string{"server_activities", "rank_calendar", "nian_total"}
		var own int
		if err = tx.QueryRow(ctx, `SELECT COALESCE((p.state #> $1::text[] ->> 'rank')::int,0) FROM avatar_progress p WHERE avatar_oid=$2`, path, oid).Scan(&own); err != nil {
			return nil, 0, err
		}
		rows, err := tx.Query(ctx, `SELECT a.avatar_oid,a.uid,a.hostnum,a.nickname,a.level,a.head_id,a.head_box_id,a.custom_head_image_url,p.state
		 FROM avatars a JOIN avatar_progress p ON p.avatar_oid=a.avatar_oid WHERE a.hostnum=$1 AND COALESCE((p.state #> $2::text[] ->> 'rank')::int,0)>0
		 ORDER BY (p.state #> $2::text[] ->> 'rank')::int ASC,a.avatar_oid ASC LIMIT $3`, host, path, limit)
		if err != nil {
			return nil, 0, err
		}
		out := []game.ActivityRankEntry{}
		for rows.Next() {
			var av game.Avatar
			var raw []byte
			if err = rows.Scan(&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Info.HeadID, &av.Info.HeadBoxID, &av.Info.CustomHeadImageURL, &raw); err != nil {
				rows.Close()
				return nil, 0, err
			}
			if err = json.Unmarshal(raw, &av.Progress); err != nil {
				rows.Close()
				return nil, 0, err
			}
			entry, ok := game.FrozenNianTotalRank(av)
			if !ok {
				rows.Close()
				return nil, 0, errors.New("年兽冻结总榜结构无效")
			}
			out = append(out, entry)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, 0, err
		}
		if err = tx.Commit(ctx); err != nil {
			return nil, 0, err
		}
		return out, own, nil
	}
	path := []string{"server_activities", "nian", fmt.Sprint(subID)}
	first, second := "server_rank_damage", "total_action"
	if kind == 8 {
		path = []string{"server_activities", "mountain", "mountain_dungeon", fmt.Sprint(subID)}
		first = "server_rank_hard"
	}
	eligible := "score>0"
	if kind == 10 && game.ValidateActivityRank(10, 0, host, limit) == nil {
		eligible = "(score>0 OR (score=0 AND ranked_at>0))"
	}
	// 字段名只由上述两个已校验分支生成，用户参数仅作为SQL绑定值。
	cte := fmt.Sprintf(`WITH all_scores AS (
	 SELECT a.avatar_oid,a.uid,a.hostnum,a.nickname,a.level,a.head_id,a.head_box_id,a.custom_head_image_url,p.state,
	 COALESCE((p.state #> $1::text[] ->> '%s')::bigint,0) AS score,
	 COALESCE((p.state #> $1::text[] ->> '%s')::bigint,0) AS actions,
	 COALESCE((p.state #> $1::text[] ->> 'server_rank_updated_at')::bigint,0) AS ranked_at
	 FROM avatars a JOIN avatar_progress p ON p.avatar_oid=a.avatar_oid
	 WHERE a.hostnum=$2 AND COALESCE((p.state #> $1::text[] ->> 'server_ranked')::boolean,false)
	), candidates AS (SELECT * FROM all_scores WHERE %s AND actions>=0) `, first, second, eligible)
	var own int
	err = tx.QueryRow(ctx, cte+`SELECT CASE WHEN EXISTS(SELECT 1 FROM candidates WHERE avatar_oid=$3) THEN
	 1+(SELECT COUNT(*) FROM candidates c, candidates self WHERE self.avatar_oid=$3 AND
	 (c.score>self.score OR (c.score=self.score AND $4::int=8 AND c.actions<self.actions) OR
	 (c.score=self.score AND ($4::int=10 OR c.actions=self.actions) AND c.avatar_oid<self.avatar_oid))) ELSE 0 END`, path, host, oid, kind).Scan(&own)
	if err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, cte+`SELECT avatar_oid,uid,hostnum,nickname,level,head_id,head_box_id,custom_head_image_url,state,score,actions
	 FROM candidates ORDER BY score DESC,CASE WHEN $3::int=8 THEN actions ELSE 0 END ASC,avatar_oid ASC LIMIT $4`, path, host, kind, limit)
	if err != nil {
		return nil, 0, err
	}
	out := []game.ActivityRankEntry{}
	for rows.Next() {
		var entry game.ActivityRankEntry
		var raw []byte
		var score, actions int64
		av := &entry.Avatar
		if err = rows.Scan(&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Info.HeadID, &av.Info.HeadBoxID, &av.Info.CustomHeadImageURL, &raw, &score, &actions); err != nil {
			rows.Close()
			return nil, 0, err
		}
		if err = json.Unmarshal(raw, &av.Progress); err != nil {
			rows.Close()
			return nil, 0, err
		}
		entry.Rank = len(out) + 1
		entry.Score = []int64{score}
		if kind == 8 {
			entry.Score = append(entry.Score, -actions)
			entry.Cards = av.Progress.Activities.Mountain.Dungeons[subID].Cards
		} else {
			entry.Cards = av.Progress.Activities.Nian[subID].RankCards
		}
		out = append(out, entry)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return out, own, nil
}

var _ game.ActivityRankAccounts = (*Store)(nil)
