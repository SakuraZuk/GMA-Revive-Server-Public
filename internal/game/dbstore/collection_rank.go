package dbstore

import (
	"context"
	"strconv"
	"time"

	"hs-server/internal/game"
)

func collectionRankIDStrings() []string {
	out := []string{}
	for _, id := range game.CollectionRankCardIDs() {
		out = append(out, strconv.Itoa(id))
	}
	return out
}

// 一条只读SQL在同一快照内完成去重、同服排序和个人全量名次。
// 只返回公开资料与分数，不向应用层传输全服卡牌/契印存档，也不获取行锁。
func (s *Store) collectionRanking(ctx context.Context, oid []byte, host, limit int) ([]game.ActivityRankEntry, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `WITH scores AS (
 SELECT a.avatar_oid,a.uid,a.hostnum,a.nickname,a.level,
 COALESCE(NULLIF(r.selected_head_id,0),a.head_id) AS head_id,
 COALESCE(NULLIF(r.selected_head_box_id,0),a.head_box_id) AS head_box_id,
 CASE WHEN COALESCE(r.custom_head_cleared,false) THEN '' ELSE a.custom_head_image_url END AS custom_head,
 (SELECT COUNT(DISTINCT card_id) FROM (
   SELECT key AS card_id FROM jsonb_object_keys(COALESCE(NULLIF(r.obtained_card_ids,'null'::jsonb),'{}'::jsonb)) AS key
   UNION ALL SELECT value->>'card_id' FROM jsonb_array_elements(COALESCE(NULLIF(r.cards,'null'::jsonb),'[]'::jsonb))
 ) owned WHERE card_id=ANY($4::text[])) AS score
 FROM avatars a JOIN avatar_progress p ON p.avatar_oid=a.avatar_oid
 CROSS JOIN LATERAL jsonb_to_record(p.state) AS r(obtained_card_ids jsonb,cards jsonb,selected_head_id int,selected_head_box_id int,custom_head_cleared boolean)
 WHERE a.hostnum=$2
), ranked AS (
 SELECT *,ROW_NUMBER() OVER (ORDER BY score DESC,avatar_oid) AS rank FROM scores WHERE score>0
), page AS (
 SELECT *,COALESCE(MAX(rank) FILTER (WHERE avatar_oid=$1) OVER (),0) AS own_rank FROM ranked
)
SELECT avatar_oid,uid,hostnum,nickname,level,head_id,head_box_id,custom_head,score,rank,own_rank
FROM page ORDER BY rank LIMIT $3`, oid, host, limit, collectionRankIDStrings())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []game.ActivityRankEntry{}
	own := 0
	for rows.Next() {
		var e game.ActivityRankEntry
		var score int64
		av := &e.Avatar
		if err := rows.Scan(&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Info.HeadID, &av.Info.HeadBoxID, &av.Info.CustomHeadImageURL, &score, &e.Rank, &own); err != nil {
			return nil, 0, err
		}
		e.Score = []int64{score}
		out = append(out, e)
	}
	return out, own, rows.Err()
}
