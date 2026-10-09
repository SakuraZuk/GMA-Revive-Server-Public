package game

import (
	"context"
	"encoding/hex"
	"log"
)

// 通知只做失效标记，不用管理请求的旧快照覆盖客户端随后提交的状态。
func (s *Service) publishPlayerRefresh(oid []byte) {
	s.onlineMu.Lock()
	connections := []*Connection{}
	for c := range s.online[hex.EncodeToString(oid)] {
		connections = append(connections, c)
	}
	s.onlineMu.Unlock()
	for _, c := range connections {
		c.mu.Lock()
		if c.phase == Playing {
			c.pendingPlayerReload = true
		}
		c.mu.Unlock()
	}
}

// 调用时持有连接锁；账号存储事务已释放全部玩家行锁。
func (s *Service) flushPlayerRefresh(ctx context.Context, c *Connection) []Push {
	if !c.pendingPlayerReload || c.phase != Playing {
		return nil
	}
	store, ok := s.Accounts.(AdminAccounts)
	if !ok {
		return nil
	}
	av, err := store.AdminPlayer(ctx, selectedOID(c))
	if err != nil {
		log.Printf("在线管理同步读取失败：%v", err)
		return nil
	}
	c.pendingPlayerReload = false
	for i, old := range c.identity.Avatars {
		if old.Hostnum == c.hostnum {
			av.Account = old.Account
			c.identity.Avatars[i] = av
		}
	}
	props := s.AvatarProperties(av, c.identity.Account)
	keys := []string{"nickname", "nickname_flag", "gender", "level", "exp", "power", "material_mgr", "exp_pool", "card_mgr", "card_common_mgr", "rune_mgr", "runes_templates", "cards_count", "last_main_chapter_dungeon_id", "achves", "achv_value", "owned_head_box", "head_id", "preset_head_id", "head_box_id"}
	keys = append(keys, "friend_dict", "friend_request_dict", "black_list", "friend_request_flag", "archive_card_2_comment_times", "archive_card_2_remark", "archive_like_comment_ids")
	keys = append(keys, "all_friend_assist_cards", "all_stranger_assist", "current_assist", "assist_active_use_times", "assist_passive_use_times", "assist_reward_times")
	keys = append(keys, "league_id", "last_leave_league_time", "league_activity_weekly_info", "house_likes", "house_likes_map", "daily_house_likes", "house_daily_random_reward")
	keys = append(keys, "recommend_gifts", "commodity_detail_info")
	for key := range pvpExtraProperties(av.Progress) {
		keys = append(keys, key)
	}
	result := []Push{}
	for _, key := range keys {
		if value, exists := props[key]; exists {
			result = append(result, push("Avatar", "client_prop_changed", []any{key, value}))
		}
	}
	return result
}
