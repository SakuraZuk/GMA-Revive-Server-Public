package dbstore

import (
	"context"
	"encoding/json"
	"hs-server/internal/game"
	"time"
)

func (s *Store) AdminPlayers(ctx context.Context, q string, limit int) ([]game.Avatar, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `SELECT avatar_oid,uid,hostnum,nickname,level,gender FROM avatars WHERE $1='' OR position($1 in nickname)>0 OR uid::text=$1 OR encode(avatar_oid,'hex')=$1 ORDER BY uid LIMIT $2`, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []game.Avatar{}
	for rows.Next() {
		var av game.Avatar
		if err = rows.Scan(&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Gender); err != nil {
			return nil, err
		}
		out = append(out, av)
	}
	return out, rows.Err()
}
func (s *Store) AdminPlayer(ctx context.Context, oid []byte) (game.Avatar, error) {
	return s.adminPlayerTransaction(ctx, oid, nil)
}
func (s *Store) AdminUpdatePlayer(ctx context.Context, oid []byte, f func(*game.Avatar) error) (game.Avatar, error) {
	return s.adminPlayerTransaction(ctx, oid, f)
}
func (s *Store) adminPlayerTransaction(ctx context.Context, oid []byte, f func(*game.Avatar) error) (game.Avatar, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return game.Avatar{}, err
	}
	defer rollback(tx)
	var av game.Avatar
	query := `SELECT avatar_oid,uid,hostnum,nickname,level,gender,nickname_set,head_id,head_box_id,custom_head_image_url,created_at FROM avatars WHERE avatar_oid=$1`
	if f != nil {
		query += ` FOR UPDATE`
	} else {
		query += ` FOR SHARE`
	}
	if err = tx.QueryRow(ctx, query, oid).Scan(&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Gender, &av.NicknameSet, &av.Info.HeadID, &av.Info.HeadBoxID, &av.Info.CustomHeadImageURL, &av.CreatedAt); err != nil {
		return game.Avatar{}, err
	}
	query = `SELECT state FROM avatar_progress WHERE avatar_oid=$1`
	if f != nil {
		query += ` FOR UPDATE`
	} else {
		query += ` FOR SHARE`
	}
	var raw []byte
	if err = tx.QueryRow(ctx, query, oid).Scan(&raw); err != nil {
		return game.Avatar{}, err
	}
	if err = json.Unmarshal(raw, &av.Progress); err != nil {
		return game.Avatar{}, err
	}
	av.Progress.AvatarLevel = av.Info.Level
	if f != nil {
		if err = f(&av); err != nil {
			return game.Avatar{}, err
		}
		raw, err = json.Marshal(av.Progress)
		if err != nil {
			return game.Avatar{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE avatars SET nickname=$2,level=$3,gender=$4,nickname_set=$5 WHERE avatar_oid=$1`, oid, av.Info.Nickname, av.Info.Level, av.Gender, av.NicknameSet); err != nil {
			return game.Avatar{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE avatar_progress SET state=$2::jsonb,revision=revision+1,updated_at=now() WHERE avatar_oid=$1`, oid, string(raw)); err != nil {
			return game.Avatar{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return game.Avatar{}, err
	}
	return av, nil
}

var _ game.AdminAccounts = (*Store)(nil)
