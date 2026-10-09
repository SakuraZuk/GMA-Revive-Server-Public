package dbstore

import (
	"context"
	"errors"
	"hs-server/internal/game"
)

func (s *Store) RefreshHumanPresence(ctx context.Context, oid []byte, token string, until int64) error {
	if len(oid) != 12 || until <= 0 {
		return errors.New("真人在线租约参数无效")
	}
	if _, err := game.SocialOID(token); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `INSERT INTO hs_game_presence(session_id,avatar_oid,expires_at) VALUES($1,$2,$3) ON CONFLICT(session_id) DO UPDATE SET expires_at=EXCLUDED.expires_at WHERE hs_game_presence.avatar_oid=EXCLUDED.avatar_oid`, token, oid, until)
	if err == nil && tag.RowsAffected() != 1 {
		return errors.New("在线租约token不能转移至其他角色")
	}
	return err
}
func (s *Store) RemoveHumanPresence(ctx context.Context, token string) error {
	if _, err := game.SocialOID(token); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM hs_game_presence WHERE session_id=$1`, token)
	return err
}
func (s *Store) HumanOnlineIDs(ctx context.Context, now int64) ([]string, error) {
	// 有效连接可跨游戏进程；清理只有已过期租约，不触及玩家/其他服务。
	if _, err := s.pool.Exec(ctx, `DELETE FROM hs_game_presence WHERE expires_at<=$1`, now); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT encode(avatar_oid,'hex') FROM hs_game_presence WHERE expires_at>$1 ORDER BY 1`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

var _ game.HumanPresenceAccounts = (*Store)(nil)
