package dbstore

import (
	"context"
	"encoding/json"
	"hs-server/internal/game"
	"time"
)

// 一条MVCC查询保持元数据与进度版本一致；无变化时不读取TOAST中的完整存档。
// 不使用管理事务或FOR SHARE，避免每个在线连接10Hz轮询争抢角色行锁。
func (s *Store) HumanAvatarSnapshot(ctx context.Context, oid []byte, since int64) (game.Avatar, int64, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var av game.Avatar
	var revision int64
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT a.avatar_oid,a.uid,a.hostnum,a.nickname,a.level,a.gender,a.nickname_set,a.head_id,a.head_box_id,a.custom_head_image_url,a.created_at,p.revision,
		CASE WHEN p.revision<>$2 THEN p.state ELSE NULL END
		FROM avatars a JOIN avatar_progress p USING(avatar_oid) WHERE a.avatar_oid=$1`, oid, since).Scan(
		&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Gender, &av.NicknameSet,
		&av.Info.HeadID, &av.Info.HeadBoxID, &av.Info.CustomHeadImageURL, &av.CreatedAt, &revision, &raw)
	if err != nil {
		return game.Avatar{}, 0, false, err
	}
	changed := raw != nil
	if changed {
		if err = json.Unmarshal(raw, &av.Progress); err != nil {
			return game.Avatar{}, 0, false, err
		}
		av.Progress.AvatarLevel = av.Info.Level
	}
	return av, revision, changed, nil
}

var _ game.HumanAvatarSnapshotAccounts = (*Store)(nil)
