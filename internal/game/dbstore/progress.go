package dbstore

import (
	"context"
	"encoding/json"
	"time"

	"hs-server/internal/game"
)

func (s *Store) loadProgress(ctx context.Context, oid []byte, level int) (game.Progress, error) {
	initial, _ := json.Marshal(game.NewProgress(level, time.Now()))
	if _, err := s.pool.Exec(ctx, `INSERT INTO avatar_progress(avatar_oid,state) VALUES($1,$2::jsonb) ON CONFLICT(avatar_oid) DO NOTHING`, oid, string(initial)); err != nil {
		return game.Progress{}, err
	}
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT state FROM avatar_progress WHERE avatar_oid=$1`, oid).Scan(&raw); err != nil {
		return game.Progress{}, err
	}
	p := game.Progress{GlobalVO: 2, StoryVO: 2}
	err := json.Unmarshal(raw, &p)
	if err == nil && (game.ActivateGuideTriggers(&p) || p.RuneSchemaVersion != game.CurrentRuneSchemaVersion) {
		// 读取后在行锁内重新取最新状态并迁移，避免登录快照覆盖并发资源更新。
		return s.UpdateProgress(ctx, oid, func(latest *game.Progress) error {
			game.ActivateGuideTriggers(latest)
			return game.MigrateRunes(latest)
		})
	}
	return p, err
}

// 玩家行锁下读取、校验并更新，避免两个连接用过期整份快照覆盖进度。
func (s *Store) UpdateProgress(ctx context.Context, oid []byte, update func(*game.Progress) error) (game.Progress, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return game.Progress{}, err
	}
	defer rollback(tx)
	// 全服周期结算按 OID 顺序锁全部角色。玩家请求若在其后排队到请求期限，
	// 连接一直不释放，池被占满后后续请求连锁超时。锁不到立即失败，客户端重试。
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout = '1s'`); err != nil {
		return game.Progress{}, err
	}
	var level int
	// 所有写入顺序统一为avatars再avatar_progress，避免与管理/双玩家事务互锁。
	if err = tx.QueryRow(ctx, `SELECT level FROM avatars WHERE avatar_oid=$1 FOR UPDATE`, oid).Scan(&level); err != nil {
		return game.Progress{}, err
	}
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT state FROM avatar_progress WHERE avatar_oid=$1 FOR UPDATE`, oid).Scan(&raw); err != nil {
		return game.Progress{}, err
	}
	p := game.Progress{GlobalVO: 2, StoryVO: 2}
	if err = json.Unmarshal(raw, &p); err != nil {
		return game.Progress{}, err
	}
	p.AvatarLevel = level
	if err = update(&p); err != nil {
		return game.Progress{}, err
	}
	raw, err = json.Marshal(p)
	if err != nil {
		return game.Progress{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE avatar_progress SET state=$2::jsonb,revision=revision+1,updated_at=now() WHERE avatar_oid=$1`, oid, string(raw)); err != nil {
		return game.Progress{}, err
	}
	if p.AvatarLevel != level {
		if _, err = tx.Exec(ctx, `UPDATE avatars SET level=$2 WHERE avatar_oid=$1`, oid, p.AvatarLevel); err != nil {
			return game.Progress{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return game.Progress{}, err
	}
	return p, nil
}

var _ game.ProgressAccounts = (*Store)(nil)
