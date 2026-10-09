// Package dbstore 实现 PostgreSQL 账号鉴权与角色持久化。
// 定义见 internal/game/db/schema.sql，三秒业务期限覆盖连接等待及事务。
package dbstore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"hs-server/internal/game"
	"hs-server/internal/game/db"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// EnsureSchema 使用唯一的嵌入式 SQL，在事务中幂等建表，失败不保留半套定义。
func (s *Store) EnsureSchema(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, db.Schema); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// QuickLogin 只校验既有账号密码（bcrypt），未知账号一律拒绝；
// 注册必须走显式 Register（AUDIT-2026-10-05 P0：随机账号不得当作用户登录）。
func (s *Store) QuickLogin(ctx context.Context, info game.ClientInfo) (game.Identity, error) {
	if info.Account == "" || len(info.Account) > 256 || info.Password == "" || len(info.Password) > 72 || info.Hostnum <= 0 {
		return game.Identity{}, errors.New("账号登录参数无效")
	}
	return s.loginWithPassword(ctx, info.Account, info.Password, info.Hostnum)
}

// Register 显式注册：建号、哈希口令并在所选服务器建默认角色，同一事务提交；
// 账号已存在时拒绝，不覆盖口令。
func (s *Store) Register(ctx context.Context, info game.ClientInfo) (game.Identity, error) {
	if info.Account == "" || len(info.Account) > 256 || info.Password == "" || len(info.Password) > 72 || info.Hostnum <= 0 {
		return game.Identity{}, errors.New("账号注册参数无效")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(info.Password), bcrypt.DefaultCost)
	if err != nil {
		return game.Identity{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return game.Identity{}, err
	}
	defer rollback(tx)
	inserted, err := tx.Exec(ctx, `INSERT INTO accounts(account,password_hash,last_login_at) VALUES($1,$2,now()) ON CONFLICT(account) DO NOTHING`, info.Account, string(hash))
	if err != nil {
		return game.Identity{}, err
	}
	if inserted.RowsAffected() == 0 {
		return game.Identity{}, game.ErrAccountExists
	}
	if err = s.seedAvatar(ctx, tx, info.Account, info.Hostnum); err != nil {
		return game.Identity{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return game.Identity{}, err
	}
	return s.loadIdentity(ctx, info.Account, info.Hostnum)
}

// SDKLogin 真实 SDK 票据鉴权未实现前一律拒绝；此前用占位哈希自动建号的
// 行为已按审计移除（sdk_info['UID'] 不能当作账号凭证，指令级证据：
// sdk_mgr.sdk_login_callback 实读 self.sdk_info.get('UID',”)）。
func (s *Store) SDKLogin(ctx context.Context, info game.ClientInfo, sdkInfo json.RawMessage) (game.Identity, error) {
	return game.Identity{}, game.ErrSDKUnavailable
}

func (s *Store) loginWithPassword(ctx context.Context, account, password string, hostnum int) (game.Identity, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return game.Identity{}, err
	}
	defer rollback(tx)
	var hash string
	if err = tx.QueryRow(ctx, `SELECT password_hash FROM accounts WHERE account=$1 FOR UPDATE`, account).Scan(&hash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return game.Identity{}, game.ErrAccountNotFound
		}
		return game.Identity{}, err
	}
	if err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return game.Identity{}, errors.New("账号鉴权失败")
	}
	if err = s.seedAvatar(ctx, tx, account, hostnum); err != nil {
		return game.Identity{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE accounts SET last_login_at=now() WHERE account=$1`, account); err != nil {
		return game.Identity{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return game.Identity{}, err
	}
	return s.loadIdentity(ctx, account, hostnum)
}

// loadIdentity 读取账号全部角色(不做口令校验)。
func (s *Store) loadIdentity(ctx context.Context, account string, hostnum int) (game.Identity, error) {
	rows, err := s.pool.Query(ctx, `SELECT avatar_oid,uid,hostnum,nickname,level,head_id,head_box_id,custom_head_image_url,gender,nickname_set,created_at
		FROM avatars WHERE account=$1 ORDER BY hostnum`, account)
	if err != nil {
		return game.Identity{}, err
	}
	identity := game.Identity{Account: account, Avatars: []game.Avatar{}}
	for rows.Next() {
		var av game.Avatar
		if err = rows.Scan(&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Info.HeadID, &av.Info.HeadBoxID, &av.Info.CustomHeadImageURL, &av.Gender, &av.NicknameSet, &av.CreatedAt); err != nil {
			rows.Close()
			return game.Identity{}, err
		}
		if len(av.OID) != 12 {
			rows.Close()
			return game.Identity{}, errors.New("数据库角色标识必须为十二字节")
		}
		identity.Avatars = append(identity.Avatars, av)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return game.Identity{}, err
	}
	for i := range identity.Avatars {
		av := &identity.Avatars[i]
		av.Account = account
		if av.Progress, err = s.loadProgress(ctx, av.OID, av.Info.Level); err != nil {
			return game.Identity{}, err
		}
	}
	return identity, nil
}

// seedAvatar 在事务内为 hostnum 建默认角色(已存在则跳过)。
func (s *Store) seedAvatar(ctx context.Context, tx pgx.Tx, account string, hostnum int) error {
	defaults := game.DefaultAvatarInfo(account)
	now := time.Now()
	oid := game.NewAvatarOID(now)
	inserted, err := tx.Exec(ctx, `INSERT INTO avatars(avatar_oid,account,hostnum,nickname,level,head_id,head_box_id)
		VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(account,hostnum) DO NOTHING`,
		oid, account, hostnum, defaults.Nickname, defaults.Level, defaults.HeadID, defaults.HeadBoxID)
	if err != nil || inserted.RowsAffected() == 0 {
		return err
	}
	// 建角与初始资产同一事务；已有角色和缺失进度的旧存档不触发测试赠送。
	raw, err := json.Marshal(game.NewAvatarProgress(defaults.Level, now))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO avatar_progress(avatar_oid,state) VALUES($1,$2::jsonb)`, oid, string(raw))
	return err
}

var _ game.Accounts = (*Store)(nil)
