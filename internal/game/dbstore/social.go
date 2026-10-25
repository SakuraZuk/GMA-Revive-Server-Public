package dbstore

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"hs-server/internal/game"
	"sort"
	"time"
)

// 社交事务结果会替换连接中的完整Avatar，必须保留账号读取的取名、性别和创建时间。
func (s *Store) SocialAvatars(ctx context.Context, q game.SocialSearch) ([]game.Avatar, error) {
	limit := q.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	ids := [][]byte{}
	for _, id := range q.OIDs {
		b, e := game.SocialOID(id)
		if e != nil {
			return nil, e
		}
		ids = append(ids, b)
	}
	rows, err := s.pool.Query(ctx, `SELECT a.avatar_oid,a.uid,a.hostnum,a.nickname,a.level,a.head_id,a.head_box_id,a.custom_head_image_url,a.account,a.gender,a.nickname_set,a.created_at,CASE WHEN $8::boolean THEN jsonb_build_object('server_avatar_level',p.state->'server_avatar_level','selected_head_id',p.state->'selected_head_id','selected_head_box_id',p.state->'selected_head_box_id','custom_head_cleared',p.state->'custom_head_cleared','sync_pvp_score',p.state->'sync_pvp_score','server_social',jsonb_build_object('friends',p.state->'server_social'->'friends','blacklist',p.state->'server_social'->'blacklist')) WHEN $9::boolean THEN jsonb_build_object('server_avatar_level',p.state->'server_avatar_level','selected_head_id',p.state->'selected_head_id','selected_head_box_id',p.state->'selected_head_box_id','custom_head_cleared',p.state->'custom_head_cleared','sync_pvp_score',p.state->'sync_pvp_score','assist_card_uuid',p.state->'assist_card_uuid','cards',(SELECT COALESCE(jsonb_agg(card),'[]'::jsonb) FROM jsonb_array_elements(COALESCE(NULLIF(p.state->'cards','null'::jsonb),'[]'::jsonb)) card WHERE card->>'uuid'=p.state->>'assist_card_uuid'),'rune_mgr',(SELECT COALESCE(jsonb_object_agg(r.key,r.value),'{}'::jsonb) FROM jsonb_each(COALESCE(NULLIF(p.state->'rune_mgr','null'::jsonb),'{}'::jsonb)) r WHERE r.value->>'card_uuid'=p.state->>'assist_card_uuid'),'server_social',jsonb_build_object('friends',p.state->'server_social'->'friends','blacklist',p.state->'server_social'->'blacklist')) ELSE p.state END FROM avatars a JOIN avatar_progress p ON p.avatar_oid=a.avatar_oid WHERE (cardinality($1::bytea[])=0 OR a.avatar_oid=ANY($1)) AND ($2::bigint=0 OR a.uid=$2) AND ($3::int=0 OR a.hostnum=$3) AND ($4::text='' OR a.nickname=$4) AND (NOT $6::boolean OR coalesce(p.state->>'assist_card_uuid','')<>'') AND (NOT $7::boolean OR coalesce(p.state->'server_owned_leagues','{}'::jsonb) NOT IN ('{}'::jsonb,'null'::jsonb)) ORDER BY a.avatar_oid LIMIT $5`, ids, q.UID, q.Hostnum, q.Nickname, limit, q.AssistOnly, q.LeagueOwnersOnly, q.FriendSummaryOnly, q.AssistSummaryOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []game.Avatar{}
	for rows.Next() {
		var av game.Avatar
		var raw []byte
		if e := rows.Scan(&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Info.HeadID, &av.Info.HeadBoxID, &av.Info.CustomHeadImageURL, &av.Account, &av.Gender, &av.NicknameSet, &av.CreatedAt, &raw); e != nil {
			return nil, e
		}
		if e := json.Unmarshal(raw, &av.Progress); e != nil {
			return nil, e
		}
		out = append(out, av)
	}
	return out, rows.Err()
}
func (s *Store) UpdateSocial(ctx context.Context, ids []string, fn func(map[string]*game.Avatar) error) ([]game.Avatar, error) {
	return s.updateSocial(ctx, ids, "", fn)
}

// 助战刷新只改请求者。提供者保持原锁顺序与实时授权读取，但不解码无关库存。
func (s *Store) UpdateAssistRefresh(ctx context.Context, self string, ids []string, fn func(map[string]*game.Avatar) error) ([]game.Avatar, error) {
	if _, err := game.SocialOID(self); err != nil {
		return nil, err
	}
	found := false
	for _, id := range ids {
		found = found || id == self
	}
	if !found {
		return nil, errors.New("助战刷新缺少请求者")
	}
	return s.updateSocial(ctx, ids, self, fn)
}

func (s *Store) updateSocial(ctx context.Context, ids []string, assistSelf string, fn func(map[string]*game.Avatar) error) ([]game.Avatar, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout = '1s'`); err != nil {
		return nil, err
	}
	ids = append([]string{}, ids...)
	if err = lockSocialDependency(ctx, tx); err != nil {
		return nil, err
	}
	sort.Strings(ids)
	ids = uniqueSocialIDs(ids)
	// 双玩家写入先按OID锁全部avatars，再按同序读取进度；与管理和普通写入一致。
	for _, id := range ids {
		oid, e := game.SocialOID(id)
		if e != nil {
			return nil, e
		}
		var level int
		if e = tx.QueryRow(ctx, `SELECT level FROM avatars WHERE avatar_oid=$1 FOR UPDATE`, oid).Scan(&level); e != nil {
			return nil, e
		}
	}
	avatars := map[string]*game.Avatar{}
	original := map[string][]byte{}
	for _, id := range ids {
		oid, e := game.SocialOID(id)
		if e != nil {
			return nil, e
		}
		var av game.Avatar
		if assistSelf != "" && id != assistSelf {
			av, e = loadAssistSocialAvatar(ctx, tx, oid)
		} else {
			av, e = loadSocialAvatar(ctx, tx, oid)
		}
		if e != nil {
			return nil, e
		}
		avatars[id] = &av
		original[id], err = json.Marshal(av.Progress)
		if err != nil {
			return nil, err
		}
	}
	if err = fn(avatars); err != nil {
		return nil, err
	}
	out := []game.Avatar{}
	for _, id := range ids {
		av := avatars[id]
		current, e := json.Marshal(av.Progress)
		if e != nil {
			return nil, e
		}
		if assistSelf != "" && id != assistSelf {
			if !bytes.Equal(original[id], current) {
				return nil, errors.New("助战刷新不能修改提供者资产或关系")
			}
			continue
		}
		// 只读取的助战/社交对手不重写整份JSON，也不抬高版本引发在线全量回载。
		if !bytes.Equal(original[id], current) {
			if err = saveSocialAvatar(ctx, tx, av); err != nil {
				return nil, err
			}
		}
		out = append(out, *av)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}
func uniqueSocialIDs(ids []string) []string {
	out := []string{}
	for _, id := range ids {
		if len(out) == 0 || out[len(out)-1] != id {
			out = append(out, id)
		}
	}
	return out
}
func loadSocialAvatar(ctx context.Context, tx pgx.Tx, oid []byte) (game.Avatar, error) {
	var av game.Avatar
	var raw []byte
	var level int
	if err := tx.QueryRow(ctx, `SELECT level FROM avatars WHERE avatar_oid=$1 FOR UPDATE`, oid).Scan(&level); err != nil {
		return av, err
	}
	err := tx.QueryRow(ctx, `SELECT a.avatar_oid,a.uid,a.hostnum,a.nickname,a.level,a.head_id,a.head_box_id,a.custom_head_image_url,a.account,a.gender,a.nickname_set,a.created_at,p.state FROM avatars a JOIN avatar_progress p ON p.avatar_oid=a.avatar_oid WHERE a.avatar_oid=$1 FOR UPDATE OF p`, oid).Scan(&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Info.HeadID, &av.Info.HeadBoxID, &av.Info.CustomHeadImageURL, &av.Account, &av.Gender, &av.NicknameSet, &av.CreatedAt, &raw)
	if err != nil {
		return av, err
	}
	err = json.Unmarshal(raw, &av.Progress)
	av.Progress.AvatarLevel = level
	return av, err
}
func saveSocialAvatar(ctx context.Context, tx pgx.Tx, av *game.Avatar) error {
	raw, e := json.Marshal(av.Progress)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE avatar_progress SET state=$2::jsonb,revision=revision+1,updated_at=now() WHERE avatar_oid=$1`, av.OID, string(raw))
	if e == nil && av.Progress.AvatarLevel != av.Info.Level {
		_, e = tx.Exec(ctx, `UPDATE avatars SET level=$2 WHERE avatar_oid=$1`, av.OID, av.Progress.AvatarLevel)
		if e == nil {
			av.Info.Level = av.Progress.AvatarLevel
		}
	}
	return e
}
func (s *Store) QueryComments(ctx context.Context, card, start, order int) ([]game.CardComment, error) {
	if start < 0 || start > 10000 || (order != 1 && order != 2) {
		return nil, errors.New("评论分页越界")
	}
	rows, err := s.pool.Query(ctx, `SELECT state FROM card_comments WHERE card_id=$1 AND deleted=false ORDER BY CASE WHEN $2::int=2 THEN like_count ELSE 0 END DESC,created_at DESC,comment_oid ASC LIMIT 20 OFFSET $3`, card, order, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []game.CardComment{}
	for rows.Next() {
		var raw []byte
		var row game.CardComment
		if e := rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e := decodeSocialComment(raw, &row); e != nil {
			return nil, e
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// 零票的likes因omitempty不会入JSONB；新建与读回都向可写回调提供非nil投票表。
// 只补空表，不替换已有投票，也不修改持久化格式或投票计数。
func decodeSocialComment(raw []byte, row *game.CardComment) error {
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, row); err != nil {
			return err
		}
	}
	if row.Likes == nil {
		row.Likes = map[string]bool{}
	}
	return nil
}

func (s *Store) UpdateComment(ctx context.Context, oid []byte, id string, fn func(*game.Avatar, *game.CardComment) error) (game.Avatar, game.CardComment, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	commentOID, err := game.SocialOID(id)
	if err != nil {
		return game.Avatar{}, game.CardComment{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return game.Avatar{}, game.CardComment{}, err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout = '1s'`); err != nil {
		return game.Avatar{}, game.CardComment{}, err
	}
	// 评论锁先于任何玩家行锁，各点赞/编辑/删除使用同一顺序。
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7142))`, id); err != nil {
		return game.Avatar{}, game.CardComment{}, err
	}
	row := game.CardComment{ID: id}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT state FROM card_comments WHERE comment_oid=$1 FOR UPDATE`, commentOID).Scan(&raw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return game.Avatar{}, row, err
	}
	existed := err == nil
	if err = decodeSocialComment(raw, &row); err != nil {
		return game.Avatar{}, row, err
	}
	originalOwner, originalCard := row.Owner, row.CardID
	av, err := loadSocialAvatar(ctx, tx, oid)
	if err != nil {
		return av, row, err
	}
	if err = fn(&av, &row); err != nil {
		return av, row, err
	}
	if row.ID != id || (existed && (row.Owner != originalOwner || row.CardID != originalCard)) {
		return av, row, errors.New("既有评论标识、作者或幻书归属不可修改")
	}
	if err = decodeSocialComment(nil, &row); err != nil {
		return av, row, err
	}
	if row.Owner == "" {
		return av, row, errors.New("评论作者缺失")
	}
	owner, e := hex.DecodeString(row.Owner)
	if e != nil {
		return av, row, e
	}
	raw, e = json.Marshal(row)
	if e != nil {
		return av, row, e
	}
	written, err := tx.Exec(ctx, `INSERT INTO card_comments(comment_oid,avatar_oid,card_id,created_at,like_count,deleted,state) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb) ON CONFLICT(comment_oid) DO UPDATE SET like_count=EXCLUDED.like_count,deleted=EXCLUDED.deleted,state=EXCLUDED.state WHERE card_comments.avatar_oid=EXCLUDED.avatar_oid AND card_comments.card_id=EXCLUDED.card_id`, commentOID, owner, row.CardID, row.CreatedAt, len(row.Likes), row.Deleted, string(raw))
	if err != nil {
		return av, row, err
	}
	if written.RowsAffected() != 1 {
		return av, row, errors.New("评论写入未影响唯一记录，玩家进度整笔回滚")
	}
	if err = saveSocialAvatar(ctx, tx, &av); err != nil {
		return av, row, err
	}
	if err = tx.Commit(ctx); err != nil {
		return av, row, err
	}
	return av, row, nil
}

var _ game.SocialAccounts = (*Store)(nil)

func (s *Store) AsyncPvpRank(ctx context.Context, oid []byte, host int) (int, error) {
	var own int
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE((state->'server_async_pvp'->>'score')::int,0) FROM avatar_progress WHERE avatar_oid=$1`, oid).Scan(&own); err != nil {
		return 0, err
	}
	if own <= 0 {
		return 0, nil
	}
	var rank int
	err := s.pool.QueryRow(ctx, `SELECT 1+COUNT(*) FROM avatar_progress p JOIN avatars a ON a.avatar_oid=p.avatar_oid WHERE ($3::int=0 OR a.hostnum=$3) AND (COALESCE((p.state->'server_async_pvp'->>'score')::int,0)>$2 OR (COALESCE((p.state->'server_async_pvp'->>'score')::int,0)=$2 AND p.avatar_oid<$1))`, oid, own, host).Scan(&rank)
	return rank, err
}
func (s *Store) CardRemark(ctx context.Context, card int) (game.CardRemarkStats, error) {
	r := game.CardRemarkStats{CardID: card, Tags: map[int]int{}}
	keyRaw, _ := json.Marshal(card)
	key := string(keyRaw)
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM card_comments WHERE card_id=$1 AND deleted=false`, card).Scan(&r.Comments); err != nil {
		return r, err
	}
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM avatar_progress WHERE COALESCE((state->'server_social'->'remarks'->$1->>'like_flag')::boolean,false)=true`, key).Scan(&r.Likes); err != nil {
		return r, err
	}
	rows, err := s.pool.Query(ctx, `SELECT tag::int,COUNT(*) FROM avatar_progress p CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(NULLIF(p.state->'server_social'->'remarks'->$1->'tags','null'::jsonb),'[]'::jsonb)) tag GROUP BY tag`, key)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var tag, count int
		if e := rows.Scan(&tag, &count); e != nil {
			return r, e
		}
		r.Tags[tag] = count
	}
	return r, rows.Err()
}
