package dbstore

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"hs-server/internal/game"
)

// 学会展示投影只传输关系与公开资料，不传输其他玩家的背包、契印或战斗存档。
// jsonb_to_record 一次解析所需字段，避免对大存档重复解压。
const leagueSnapshotSQL = `SELECT a.avatar_oid,a.uid,a.hostnum,a.nickname,a.level,a.head_id,a.head_box_id,a.custom_head_image_url,
jsonb_build_object('server_league_id',r.server_league_id,'server_owned_leagues',r.server_owned_leagues,
'server_league_protect',r.server_league_protect,'selected_head_id',r.selected_head_id,
'selected_head_box_id',r.selected_head_box_id,'custom_head_cleared',r.custom_head_cleared,'sync_pvp_score',r.sync_pvp_score)
FROM avatars a JOIN avatar_progress p ON p.avatar_oid=a.avatar_oid
CROSS JOIN LATERAL jsonb_to_record(p.state) AS r(server_league_id jsonb,server_owned_leagues jsonb,
server_league_protect jsonb,selected_head_id jsonb,selected_head_box_id jsonb,custom_head_cleared jsonb,sync_pvp_score jsonb)
ORDER BY a.avatar_oid`

// 所有学会写事务先获取同一专属事务锁，保证名称与全局编号唯一；
// 再按统一顺序只锁当前角色、学会主档和显式目标。普通玩家写入不使用此专属锁。
func (s *Store) UpdateLeagueSocial(ctx context.Context, q game.LeagueTransaction, fn func(map[string]*game.Avatar) error) ([]game.Avatar, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	options := pgx.TxOptions{}
	if q.ReadOnly {
		options.AccessMode = pgx.ReadOnly
	}
	tx, err := s.pool.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout = '1s'`); err != nil {
		return nil, err
	}
	if !q.ReadOnly {
		if err = lockSocialDependency(ctx, tx); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(current_schema() || ':学会事务',7142))`); err != nil {
			return nil, err
		}
	}
	rows, err := tx.Query(ctx, leagueSnapshotSQL)
	if err != nil {
		return nil, err
	}
	avatars := map[string]*game.Avatar{}
	for rows.Next() {
		var av game.Avatar
		var raw []byte
		if err = rows.Scan(&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Info.HeadID, &av.Info.HeadBoxID, &av.Info.CustomHeadImageURL, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal(raw, &av.Progress); err != nil {
			rows.Close()
			return nil, err
		}
		av.Progress.AvatarLevel = av.Info.Level
		avatars[hex.EncodeToString(av.OID)] = &av
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	self := avatars[q.SelfID]
	if self == nil {
		return nil, errors.New("学会事务当前角色不存在")
	}
	locked := map[string]bool{}
	if !q.ReadOnly {
		locked[q.SelfID] = true
		if avatars[q.TargetMember] != nil {
			locked[q.TargetMember] = true
		}
		for id, av := range avatars {
			for leagueID, state := range av.Progress.OwnedLeagues {
				if state != nil && !state.Dissolved && (leagueID == self.Progress.LeagueID || leagueID == q.TargetLeague) {
					locked[id] = true
				}
			}
		}
	}
	ids := make([]string, 0, len(locked))
	for id := range locked {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	// 先锁全部相关 avatars，再读取相关 progress，与其他业务的锁顺序一致。
	for _, id := range ids {
		var level int
		if err = tx.QueryRow(ctx, `SELECT level FROM avatars WHERE avatar_oid=$1 FOR UPDATE`, avatars[id].OID).Scan(&level); err != nil {
			return nil, err
		}
	}
	for _, id := range ids {
		av, e := loadSocialAvatar(ctx, tx, avatars[id].OID)
		if e != nil {
			return nil, e
		}
		avatars[id] = &av
	}
	original := make(map[string][]byte, len(avatars))
	for id, av := range avatars {
		original[id], err = json.Marshal(av.Progress)
		if err != nil {
			return nil, err
		}
	}
	if err = fn(avatars); err != nil {
		return nil, err
	}
	if len(avatars) != len(original) {
		return nil, errors.New("学会事务不得增删角色")
	}
	// 展示投影必须保持只读；遗漏锁目标时整笔拒绝，不能部分保存。
	for id, av := range avatars {
		if av == nil || hex.EncodeToString(av.OID) != id || original[id] == nil {
			return nil, errors.New("学会事务不得替换角色标识")
		}
		current, e := json.Marshal(av.Progress)
		if e != nil {
			return nil, e
		}
		if bytes.Equal(original[id], current) {
			continue
		}
		if !locked[id] {
			return nil, errors.New("学会事务不得修改未锁定角色的展示投影")
		}
		if err = saveSocialAvatar(ctx, tx, av); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	out := make([]game.Avatar, 0, len(ids))
	for _, id := range ids {
		out = append(out, *avatars[id])
	}
	return out, nil
}

var _ game.LeagueAccounts = (*Store)(nil)
