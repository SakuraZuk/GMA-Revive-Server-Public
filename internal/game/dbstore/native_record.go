package dbstore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"hs-server/internal/game"
)

func (s *Store) UpdateNativeRecord(ctx context.Context, oid []byte, uuid string, fn func(*game.Avatar, *game.NativeBattleRecord) error) (game.NativeBattleRecord, error) {
	if _, err := game.SocialOID(uuid); err != nil {
		return game.NativeBattleRecord{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return game.NativeBattleRecord{}, err
	}
	defer rollback(tx)
	av, err := loadSocialAvatar(ctx, tx, oid)
	if err != nil {
		return game.NativeBattleRecord{}, err
	}
	// 先玩家后录像的固定锁顺序；同UUID另一个上传者必须拒绝，不可覆盖文件。
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7143))`, uuid); err != nil {
		return game.NativeBattleRecord{}, err
	}
	record := game.NativeBattleRecord{}
	var raw, owner []byte
	err = tx.QueryRow(ctx, `SELECT avatar_oid,state FROM native_battle_records WHERE battle_uuid=$1 FOR UPDATE`, uuid).Scan(&owner, &raw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return record, err
	}
	if err == nil {
		if hex.EncodeToString(owner) != hex.EncodeToString(oid) {
			return record, errors.New("录像上传者归属冲突")
		}
		if err = json.Unmarshal(raw, &record); err != nil {
			return record, err
		}
	}
	if err = fn(&av, &record); err != nil {
		return record, err
	}
	raw, err = json.Marshal(record)
	if err != nil {
		return record, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO native_battle_records(battle_uuid,avatar_oid,state) VALUES($1,$2,$3::jsonb) ON CONFLICT(battle_uuid) DO UPDATE SET state=EXCLUDED.state,updated_at=now() WHERE native_battle_records.avatar_oid=EXCLUDED.avatar_oid`, uuid, oid, string(raw)); err != nil {
		return record, err
	}
	if err = tx.Commit(ctx); err != nil {
		return record, err
	}
	return record, nil
}
func (s *Store) ReadNativeRecord(ctx context.Context, oid []byte, uuid string) (game.NativeBattleRecord, error) {
	var record game.NativeBattleRecord
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT state FROM native_battle_records WHERE battle_uuid=$1 AND state->'viewers' ? $2`, uuid, hex.EncodeToString(oid)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return record, nil
	}
	if err != nil {
		return record, err
	}
	err = json.Unmarshal(raw, &record)
	return record, err
}

var _ game.NativeBattleRecordAccounts = (*Store)(nil)
