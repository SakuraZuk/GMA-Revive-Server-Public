// Package dbstore 的同步 PVP 跨账号排行查询。
package dbstore

import (
	"context"
	"errors"
	"time"

	"hs-server/internal/game"
)

// SyncPvpRankings 从全体角色读取同步 PVP 积分榜（真账号，不含幽灵）。
// score 存于 avatar_progress.state JSONB 的 sync_pvp_score 键。
func (s *Store) SyncPvpRankings(ctx context.Context, limit int, hosts ...int) ([]game.SyncPvpRankEntry, error) {
	if limit <= 0 || limit > 1000 {
		return nil, errors.New("排行榜数量越界")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	hostnum := 0
	if len(hosts) > 0 {
		hostnum = hosts[0]
	}
	rows, err := s.pool.Query(ctx, `
		SELECT a.avatar_oid, a.uid, a.hostnum, a.nickname, a.level,
		       COALESCE((ap.state->>'sync_pvp_score')::int, 0) AS score
		FROM avatars a
		JOIN avatar_progress ap ON ap.avatar_oid = a.avatar_oid
		WHERE COALESCE((ap.state->>'sync_pvp_score')::int, 0) > 0 AND ($2=0 OR a.hostnum=$2)
		ORDER BY score DESC, a.avatar_oid
		LIMIT $1`, limit, hostnum)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]game.SyncPvpRankEntry, 0, limit)
	for rows.Next() {
		var entry game.SyncPvpRankEntry
		if err := rows.Scan(&entry.OID, &entry.UID, &entry.Hostnum, &entry.Nickname, &entry.Level, &entry.Score); err != nil {
			return nil, err
		}
		if entry.Score <= 0 {
			continue
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// SyncPvpRank 的同分次序与榜单相同；名次查询覆盖全部账号，不只第一页。
func (s *Store) SyncPvpRank(ctx context.Context, oid []byte, hostnum int) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var rank int
	err := s.pool.QueryRow(ctx, `WITH own AS (
	 SELECT a.avatar_oid, COALESCE((p.state->>'sync_pvp_score')::int,0) AS score
	 FROM avatars a JOIN avatar_progress p ON p.avatar_oid=a.avatar_oid WHERE a.avatar_oid=$1 AND ($2=0 OR a.hostnum=$2)
	) SELECT CASE WHEN EXISTS(SELECT 1 FROM own) THEN 1+COUNT(*) ELSE 0 END FROM avatars a JOIN avatar_progress p ON p.avatar_oid=a.avatar_oid, own
	 WHERE ($2=0 OR a.hostnum=$2) AND COALESCE((p.state->>'sync_pvp_score')::int,0)>0
	 AND (COALESCE((p.state->>'sync_pvp_score')::int,0)>own.score OR
	 (COALESCE((p.state->>'sync_pvp_score')::int,0)=own.score AND a.avatar_oid<own.avatar_oid))`, oid, hostnum).Scan(&rank)
	if err == nil && rank == 0 {
		return 0, errors.New("排行榜角色不存在")
	}
	return rank, err
}
