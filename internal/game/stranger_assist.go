package game

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
)

func valueAvatar(av *Avatar) Avatar {
	if av == nil {
		return Avatar{}
	}
	return *av
}

// 候选资格由服务器真实角色、双向黑名单、同服和指定卡决定。
// 名册最多10人/每卡每日1次来自本版assist_rule[1]，不是任意UUID授权。
func validStrangerAssistProvider(own, peer Avatar, uuid string) error {
	id, self := hexOf(peer.OID), hexOf(own.OID)
	if id == self || len(peer.OID) != 12 || own.Hostnum != peer.Hostnum || !validObjectID(uuid) {
		return errors.New("陌生助战角色或指定卡无效")
	}
	if _, ok := own.Progress.Social.Friends[id]; ok {
		return errors.New("好友不列入陌生助战")
	}
	if _, ok := peer.Progress.Social.Friends[self]; ok {
		return errors.New("好友关系尚未同步")
	}
	if _, ok := own.Progress.Social.Blacklist[id]; ok {
		return errors.New("助战提供者在黑名单")
	}
	if _, ok := peer.Progress.Social.Blacklist[self]; ok {
		return errors.New("提供者拒绝当前玩家")
	}
	if peer.Progress.AssistCardUUID != uuid {
		return errors.New("助战卡已被提供者更换")
	}
	_, err := snapshotFriendAssist(peer, uuid)
	return err
}

func authorizedPlayerAssist(own, peer Avatar, uuid string) (int, error) {
	id := hexOf(peer.OID)
	if _, friend := own.Progress.Social.Friends[id]; friend {
		return socialCatalog.Constants["RELATION_TYPE_FRIEND"], AuthorizedFriendAssist(own, peer, uuid)
	}
	if err := validStrangerAssistProvider(own, peer, uuid); err != nil {
		return 0, err
	}
	snapshot, exists := own.Progress.Social.Assist.Strangers[id]
	if !exists || snapshot.Card.UUID != uuid {
		return 0, errors.New("该陌生角色未进入服务端授权助战候选")
	}
	return socialCatalog.Constants["RELATION_TYPE_STRANGER"], nil
}

// 本服从查询窗口内所有合格真实角色等概率取候选，排序不承担随机选择。
func refreshStrangerAssistCandidates(own Avatar, peers map[string]*Avatar) error {
	var rule struct {
		Count int `json:"assist_card_num"`
	}
	if json.Unmarshal(socialCatalog.Tables["assist_rule"]["1"], &rule) != nil || rule.Count <= 0 {
		return errors.New("陌生助战名册原生目录缺失")
	}
	ids := []string{}
	for id, peer := range peers {
		if peer != nil && validStrangerAssistProvider(own, *peer, peer.Progress.AssistCardUUID) == nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	chosen := map[string]FriendAssist{}
	for len(ids) > 0 && len(chosen) < rule.Count {
		draw, err := rand.Int(rand.Reader, big.NewInt(int64(len(ids))))
		if err != nil {
			return err
		}
		i := int(draw.Int64())
		id := ids[i]
		peer := peers[id]
		snapshot, err := snapshotFriendAssist(*peer, peer.Progress.AssistCardUUID)
		if err != nil {
			return err
		}
		chosen[id] = snapshot
		ids = append(ids[:i], ids[i+1:]...)
	}
	self := peers[hexOf(own.OID)]
	if self == nil {
		return errors.New("助战刷新事务缺少本人")
	}
	self.Progress.Social.Assist.Strangers = chosen
	return nil
}
