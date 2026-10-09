package game

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"
)

// 重连使用客户端已鉴权收尾提交的 auth_msg；数据库仅保存摘要，并绑定角色与设备。
// RE_CONNECTION 必须复用客户端已有 Avatar，不能创建新的 Account。
type ReconnectAccounts interface {
	SaveReconnect(context.Context, []byte, string, []byte, time.Time) error
	Resume(context.Context, []byte, string, []byte, time.Time) (Identity, error)
}

type reconnectRecord struct {
	identity Identity
	device   string
	hash     [32]byte
	expires  time.Time
}

func (a *FixtureAccounts) SaveReconnect(ctx context.Context, oid []byte, device string, hash []byte, expires time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	for account, record := range a.records {
		for _, av := range record.Avatars {
			if string(av.OID) == string(oid) {
				if a.reconnect == nil {
					a.reconnect = map[string]reconnectRecord{}
				}
				var digest [32]byte
				copy(digest[:], hash)
				a.reconnect[string(oid)] = reconnectRecord{Identity{Account: account, Avatars: []Avatar{av}}, device, digest, expires}
				return nil
			}
		}
	}
	return errors.New("角色不存在")
}

func (a *FixtureAccounts) Resume(ctx context.Context, oid []byte, device string, hash []byte, now time.Time) (Identity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Identity{}, err
	}
	record, ok := a.reconnect[string(oid)]
	if !ok || record.device != device || !now.Before(record.expires) || subtle.ConstantTimeCompare(hash, record.hash[:]) != 1 {
		return Identity{}, errors.New("重连凭证无效或已过期")
	}
	identity := Identity{Account: record.identity.Account, Avatars: append([]Avatar(nil), record.identity.Avatars...)}
	for i, av := range identity.Avatars {
		for _, current := range a.records[identity.Account].Avatars {
			if string(current.OID) == string(av.OID) {
				identity.Avatars[i] = current
				break
			}
		}
	}
	return identity, nil
}

func (c *Connection) SetDeviceID(device string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deviceID = device
}

// 调用者已经持有连接锁，提交成功后才能允许客户端进入重连路径。
func (s *Service) saveReconnect(ctx context.Context, c *Connection, raw json.RawMessage) error {
	var token string
	if json.Unmarshal(raw, &token) != nil || len(token) == 0 || len(token) > 512 {
		return errors.New("重连凭证必须是非空短字符串")
	}
	store, ok := s.Accounts.(ReconnectAccounts)
	if !ok {
		return errors.New("账号存储不支持重连")
	}
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			digest := sha256.Sum256([]byte(token))
			return store.SaveReconnect(ctx, av.OID, c.deviceID, digest[:], s.Now().Add(24*time.Hour))
		}
	}
	return errors.New("没有绑定角色")
}

// Resume校验凭据并复用原Avatar。普通未完成战斗丢失连接观察缓存，
// 必须同批下发新UUID、明确清场标记与prepare；PVP保留其独立原生恢复路线。
func (s *Service) Resume(ctx context.Context, c *Connection, oid []byte, device string, token []byte) (Avatar, []Push, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.phase != Connected || len(oid) != 12 || len(token) == 0 || len(token) > 512 {
		return Avatar{}, nil, errors.New("重连状态或参数无效")
	}
	store, ok := s.Accounts.(ReconnectAccounts)
	if !ok {
		return Avatar{}, nil, errors.New("账号存储不支持重连")
	}
	digest := sha256.Sum256(token)
	identity, err := store.Resume(ctx, oid, device, digest[:], s.Now())
	if err != nil {
		return Avatar{}, nil, err
	}
	for _, av := range identity.Avatars {
		if string(av.OID) == string(oid) {
			c.identity, c.hostnum, c.phase, c.deviceID = identity, av.Hostnum, Playing, device
			c.refreshLoginSent = true
			// 客户端权威：重连不再补发服务器战斗跳转；已开战会话仅恢复
			// battleStartSent 防止 battle_fighting 重复上行，未完成战斗由
			// 客户端 client_need_recover_battle 请求驱动重开准备阶段。
			if av.Progress.Battle != nil && av.Progress.Battle.Started {
				c.battleStartSent = true
			}
			p := av.Progress
			if b := p.Battle; b != nil && !b.Finished && b.NativeSolo == nil && p.SyncPvpMatch == nil && p.Social.HumanRoom == nil && (p.AsyncPvp.Match == nil || p.AsyncPvp.Match.UUID != b.UUID) {
				// 真正TCP重连丢失连接观察缓存与未送达帧，不能续用旧序号。
				// 先走已授权的普通冷恢复事务，新UUID与明确清场标记随connect_reply整批下发。
				replay, err := s.clientNeedRecoverBattle(ctx, c, nil)
				if err != nil {
					return Avatar{}, nil, err
				}
				return c.SelectedAvatarUnsafe(), replay, nil
			}
			return av, nil, nil
		}
	}
	return Avatar{}, nil, errors.New("重连角色不属于账号")
}
