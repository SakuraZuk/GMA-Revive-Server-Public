package game

import (
	"encoding/hex"
	"hs-server/internal/mobileproto"
	"sort"
)

// AvatarProperties补在线状态；create_entity与管理在线同步使用同一个原生投影。
func (s *Service) AvatarProperties(av Avatar, account string) map[string]any {
	props := av.InitialProperties(account)
	for key, value := range s.socialLiveProperties(av.Progress.Social) {
		props[key] = value
	}
	return props
}

// socialLiveProperties只读取注册表，不尝试获取另一连接锁；避免两个玩家互相刷新时死锁。
func (s *Service) socialLiveProperties(state SocialState) map[string]any {
	online := s.humanKnownOnline()
	props := SocialProperties(state)
	updateInfo := func(info map[string]any) {
		if id, ok := info["eid"].(ObjectID); ok {
			info["online"] = online[string(id)]
		}
	}
	for _, key := range []string{"friend_dict", "friend_request_dict", "black_list", "challenge_dict"} {
		entries, _ := props[key].(mobileproto.Map)
		for _, pair := range entries {
			row, _ := pair.Value.(map[string]any)
			if key == "black_list" {
				updateInfo(row)
			} else {
				if info, ok := row["info"].(map[string]any); ok {
					updateInfo(info)
				}
			}
		}
	}
	if assist, ok := props["current_assist"].(map[string]any); ok {
		if info, ok := assist["assist_avatar"].(map[string]any); ok {
			updateInfo(info)
		}
	}
	return props
}
func (s *Service) socialLivePushes(v SocialState) []Push {
	props := s.socialLiveProperties(v)
	// 原生all_friend_assist_cards setter会立即查friend_dict，先下发关系再下发助战名单。
	keys := []string{"friend_dict", "friend_request_dict", "black_list", "friend_request_flag", "challenge_dict", "archive_card_2_comment_times", "archive_card_2_remark", "archive_like_comment_ids", "all_friend_assist_cards", "all_stranger_assist", "current_assist", "assist_active_use_times", "assist_passive_use_times", "assist_reward_times"}
	result := []Push{}
	for _, key := range keys {
		result = append(result, push("Avatar", "client_prop_changed", []any{key, props[key]}))
	}
	return result
}
func (s *Service) socialInfo(v SocialProfile) map[string]any {
	info := v.wire()
	info["online"] = s.humanKnownOnline()[v.EID]
	return info
}
func socialFriendOIDs(v SocialState) [][]byte {
	ids := []string{}
	for id := range v.Friends {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := [][]byte{}
	for _, id := range ids {
		if oid, e := hex.DecodeString(id); e == nil && len(oid) == 12 {
			result = append(result, oid)
		}
	}
	return result
}
