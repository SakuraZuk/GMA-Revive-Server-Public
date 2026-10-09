package dbstore

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"hs-server/internal/game"
	"time"
)

// 首次取名在行锁下提交；重复同值可重试，更名消费属于另一个尚未实现的接口。
func (s *Store) SetNicknameGender(ctx context.Context, oid []byte, name string, gender int) (game.Avatar, error) {
	if game.ValidateNickname(name) != 0 || (gender != 1 && gender != 2) {
		return game.Avatar{}, errors.New("取名参数无效")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return game.Avatar{}, err
	}
	defer rollback(tx)
	var account, current string
	var currentGender, hostnum int
	var chosen bool
	if err = tx.QueryRow(ctx, `SELECT account,hostnum,nickname,gender,nickname_set FROM avatars WHERE avatar_oid=$1 FOR UPDATE`, oid).Scan(&account, &hostnum, &current, &currentGender, &chosen); err != nil {
		return game.Avatar{}, err
	}
	if chosen {
		if current != name || currentGender != gender {
			return game.Avatar{}, game.ErrNicknameExists
		}
	} else {
		var status int
		if err = tx.QueryRow(ctx, `SELECT (state->'guide_tasks'->'1000'->>'task_status')::integer FROM avatar_progress WHERE avatar_oid=$1`, oid).Scan(&status); err != nil {
			return game.Avatar{}, err
		}
		if status != 1 {
			return game.Avatar{}, game.ErrProfileState
		}
		if _, err = tx.Exec(ctx, `UPDATE avatars SET nickname=$2,gender=$3,nickname_set=true WHERE avatar_oid=$1`, oid, name, gender); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return game.Avatar{}, game.ErrNicknameExists
			}
			return game.Avatar{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return game.Avatar{}, err
	}
	identity, err := s.loadIdentity(ctx, account, hostnum)
	if err != nil {
		return game.Avatar{}, err
	}
	for _, av := range identity.Avatars {
		if av.Hostnum == hostnum {
			return av, nil
		}
	}
	return game.Avatar{}, game.ErrProfileState
}
