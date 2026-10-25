package dbstore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"hs-server/internal/game"
)

// 预选资格只依赖指定卡和双向关系；契印留到最终十人锁内再读。
// jsonb_to_record一次提取，避免每个字段反复解压同一完整存档。
const assistCandidateRecordSQL = ` CROSS JOIN LATERAL jsonb_to_record(p.state) AS r(
server_avatar_level int,selected_head_id int,selected_head_box_id int,
custom_head_cleared boolean,sync_pvp_score int,assist_card_uuid text,cards jsonb,server_social jsonb)`
const assistCandidateProjectionSQL = `jsonb_build_object(
'server_avatar_level',r.server_avatar_level,'selected_head_id',r.selected_head_id,
'selected_head_box_id',r.selected_head_box_id,'custom_head_cleared',r.custom_head_cleared,
'sync_pvp_score',r.sync_pvp_score,'assist_card_uuid',r.assist_card_uuid,
'cards',(SELECT COALESCE(jsonb_agg(card),'[]'::jsonb) FROM jsonb_array_elements(COALESCE(r.cards,'[]'::jsonb)) card WHERE card->>'uuid'=r.assist_card_uuid),
'server_social',jsonb_build_object('friends',r.server_social->'friends','blacklist',r.server_social->'blacklist'))`

func (s *Store) AssistCandidates(ctx context.Context, host, limit int) ([]game.Avatar, error) {
	if host <= 0 || limit < 1 || limit > 1000 {
		return nil, errors.New("助战资格预选服区或数量无效")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `SELECT a.avatar_oid,a.uid,a.hostnum,a.nickname,a.level,a.head_id,a.head_box_id,a.custom_head_image_url,`+assistCandidateProjectionSQL+` FROM avatars a JOIN avatar_progress p USING(avatar_oid)`+assistCandidateRecordSQL+` WHERE a.hostnum=$1 AND COALESCE(r.assist_card_uuid,'')<>'' ORDER BY a.avatar_oid LIMIT $2`, host, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []game.Avatar{}
	for rows.Next() {
		var av game.Avatar
		var raw []byte
		if err = rows.Scan(&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Info.HeadID, &av.Info.HeadBoxID, &av.Info.CustomHeadImageURL, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &av.Progress); err != nil {
			return nil, err
		}
		out = append(out, av)
	}
	return out, rows.Err()
}
