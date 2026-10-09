package dbstore

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"hs-server/internal/game"
	"time"
)

// 全量周期结算先顺序锁全部avatars，再顺序锁全部progress；写入使用批次且仅保存变动行。
func (s *Store) UpdateAllSocial(ctx context.Context, fn func(map[string]*game.Avatar) error) ([]game.Avatar, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	rows, err := tx.Query(ctx, `SELECT avatar_oid,uid,hostnum,nickname,level,head_id,head_box_id,custom_head_image_url,account,gender,nickname_set,created_at FROM avatars ORDER BY avatar_oid FOR UPDATE`)
	if err != nil {
		return nil, err
	}
	avatars := map[string]*game.Avatar{}
	ids := []string{}
	for rows.Next() {
		var av game.Avatar
		if err = rows.Scan(&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Info.HeadID, &av.Info.HeadBoxID, &av.Info.CustomHeadImageURL, &av.Account, &av.Gender, &av.NicknameSet, &av.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		id := hex.EncodeToString(av.OID)
		avatars[id] = &av
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.Query(ctx, `SELECT avatar_oid,state FROM avatar_progress ORDER BY avatar_oid FOR UPDATE`)
	if err != nil {
		return nil, err
	}
	original := map[string][]byte{}
	for rows.Next() {
		var oid, raw []byte
		if err = rows.Scan(&oid, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		id := hex.EncodeToString(oid)
		av := avatars[id]
		if av == nil {
			rows.Close()
			return nil, errors.New("周期进度存在孤立角色")
		}
		if err = json.Unmarshal(raw, &av.Progress); err != nil {
			rows.Close()
			return nil, err
		}
		av.Progress.AvatarLevel = av.Info.Level
		original[id], _ = json.Marshal(av.Progress)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// 早期版本曾留下“角色主表存在、进度表缺失”的遗留行。全服排行与周期
	// 结算只能使用有完整进度的角色；单个遗留行不得阻断所有正常玩家登录、
	// 教学副本或活动结算。这里在同一只读快照中隔离它们，不猜测或补造进度。
	validIDs := ids[:0]
	for _, id := range ids {
		if _, exists := original[id]; !exists {
			delete(avatars, id)
			continue
		}
		validIDs = append(validIDs, id)
	}
	ids = validIDs
	if err = fn(avatars); err != nil {
		return nil, err
	}
	batch := &pgx.Batch{}
	out := []game.Avatar{}
	for _, id := range ids {
		av := avatars[id]
		raw, e := json.Marshal(av.Progress)
		if e != nil {
			return nil, e
		}
		if bytes.Equal(raw, original[id]) {
			continue
		}
		batch.Queue(`UPDATE avatar_progress SET state=$2::jsonb,revision=revision+1,updated_at=now() WHERE avatar_oid=$1`, av.OID, string(raw))
		if av.Progress.AvatarLevel != av.Info.Level {
			batch.Queue(`UPDATE avatars SET level=$2 WHERE avatar_oid=$1`, av.OID, av.Progress.AvatarLevel)
			av.Info.Level = av.Progress.AvatarLevel
		}
		out = append(out, *av)
	}
	if batch.Len() > 0 {
		results := tx.SendBatch(ctx, batch)
		for i := 0; i < batch.Len(); i++ {
			if _, err = results.Exec(); err != nil {
				_ = results.Close()
				return nil, err
			}
		}
		if err = results.Close(); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}
