package dbstore

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"hs-server/internal/game"
)

// 与助战候选投影相同；提供者进度仍在有序行锁内读取，不使用预选旧快照。
const assistRefreshStateSQL = `jsonb_build_object(
'server_avatar_level',p.state->'server_avatar_level',
'selected_head_id',p.state->'selected_head_id','selected_head_box_id',p.state->'selected_head_box_id',
'custom_head_cleared',p.state->'custom_head_cleared','sync_pvp_score',p.state->'sync_pvp_score',
'assist_card_uuid',p.state->'assist_card_uuid',
'cards',(SELECT COALESCE(jsonb_agg(card),'[]'::jsonb) FROM jsonb_array_elements(COALESCE(NULLIF(p.state->'cards','null'::jsonb),'[]'::jsonb)) card WHERE card->>'uuid'=p.state->>'assist_card_uuid'),
'rune_mgr',(SELECT COALESCE(jsonb_object_agg(r.key,r.value),'{}'::jsonb) FROM jsonb_each(COALESCE(NULLIF(p.state->'rune_mgr','null'::jsonb),'{}'::jsonb)) r WHERE r.value->>'card_uuid'=p.state->>'assist_card_uuid'),
'server_social',jsonb_build_object('friends',p.state->'server_social'->'friends','blacklist',p.state->'server_social'->'blacklist'))`

func loadAssistSocialAvatar(ctx context.Context, tx pgx.Tx, oid []byte) (game.Avatar, error) {
	var av game.Avatar
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT a.avatar_oid,a.uid,a.hostnum,a.nickname,a.level,a.head_id,a.head_box_id,a.custom_head_image_url,a.account,a.gender,a.nickname_set,a.created_at,`+assistRefreshStateSQL+` FROM avatars a JOIN avatar_progress p ON p.avatar_oid=a.avatar_oid WHERE a.avatar_oid=$1 FOR UPDATE OF p`, oid).Scan(&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Info.HeadID, &av.Info.HeadBoxID, &av.Info.CustomHeadImageURL, &av.Account, &av.Gender, &av.NicknameSet, &av.CreatedAt, &raw)
	if err != nil {
		return av, err
	}
	err = json.Unmarshal(raw, &av.Progress)
	av.Progress.AvatarLevel = av.Info.Level
	return av, err
}
