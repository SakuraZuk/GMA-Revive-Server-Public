package dbstore

import (
	"context"
	"errors"
	"time"

	"hs-server/internal/game"
)

func (s *Store) SaveReconnect(ctx context.Context, oid []byte, device string, hash []byte, expires time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO avatar_reconnect(avatar_oid,device_id,token_hash,expires_at)
	 VALUES($1,$2,$3,$4) ON CONFLICT(avatar_oid) DO UPDATE SET
	 device_id=EXCLUDED.device_id,token_hash=EXCLUDED.token_hash,expires_at=EXCLUDED.expires_at`, oid, device, hash, expires)
	return err
}

func (s *Store) Resume(ctx context.Context, oid []byte, device string, hash []byte, now time.Time) (game.Identity, error) {
	var account string
	var hostnum int
	err := s.pool.QueryRow(ctx, `SELECT a.account,a.hostnum FROM avatars a JOIN avatar_reconnect r USING(avatar_oid)
	 WHERE a.avatar_oid=$1 AND r.device_id=$2 AND r.token_hash=$3 AND r.expires_at>$4`, oid, device, hash, now).Scan(&account, &hostnum)
	if err != nil {
		return game.Identity{}, errors.New("重连凭证无效或已过期")
	}
	return s.loadIdentity(ctx, account, hostnum)
}

var _ game.ReconnectAccounts = (*Store)(nil)
