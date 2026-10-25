package game

import (
	"context"
	"errors"
)

// 仅使用已鉴权角色的真实待处理邀请，双方删除及通知记录在同一事务提交。
func (s *Service) refusePendingChallenges(ctx context.Context, c *Connection, cb, reason int) ([]Push, error) {
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil, errors.New("存储不支持邀请事务")
	}
	ownID := hexOf(selectedOID(c))
	ids := []string{ownID}
	// 邀请可能来自其他连接，先读取持久化本人记录，不能依赖旧连接缓存。
	snapshot, err := store.SocialAvatars(ctx, SocialSearch{OIDs: []string{ownID}, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(snapshot) != 1 {
		return nil, errors.New("邀请本人不存在")
	}
	for id := range snapshot[0].Progress.Social.Challenges {
		if id != ownID {
			ids = append(ids, id)
		}
	}
	if len(ids) == 1 {
		return []Push{Callback(cb, []any{RetSuccess})}, nil
	}
	rows, err := store.UpdateSocial(ctx, ids, func(v map[string]*Avatar) error {
		own := v[ownID]
		if own == nil {
			return errors.New("邀请本人不存在")
		}
		ensureSocial(&own.Progress.Social)
		changed := false
		for _, id := range ids[1:] {
			if _, exists := own.Progress.Social.Challenges[id]; !exists {
				continue
			}
			peer := v[id]
			if peer == nil {
				return errors.New("邀请方不存在")
			}
			ensureSocial(&peer.Progress.Social)
			delete(own.Progress.Social.Challenges, id)
			delete(peer.Progress.Social.ChallengeSent, ownID)
			changed = true
			sequence := int64(1)
			if n := len(peer.Progress.Social.HumanRefusals); n > 0 {
				sequence = peer.Progress.Social.HumanRefusals[n-1].Sequence + 1
			}
			peer.Progress.Social.HumanRefusals = append(peer.Progress.Social.HumanRefusals, HumanRefusal{Sequence: sequence, Profile: socialProfile(*own), Reason: reason, Created: s.Now().Unix()})
			if n := len(peer.Progress.Social.HumanRefusals); n > 32 {
				peer.Progress.Social.HumanRefusals = peer.Progress.Social.HumanRefusals[n-32:]
			}
			peer.Progress.Social.Revision++
		}
		if changed {
			own.Progress.Social.Revision++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	acceptSocialRows(c, rows)
	return append(s.socialLivePushes(c.SelectedAvatarUnsafe().Progress.Social), Callback(cb, []any{RetSuccess})), nil
}
