package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"hs-server/internal/mobileproto"
)

//go:embed collection_social_catalog.json
var collectionSocialCatalogData []byte

var androidCollectionSocial = func() struct {
	DailyLikes int `json:"house_likes_daily_limit"`
} {
	var rule struct {
		DailyLikes int `json:"house_likes_daily_limit"`
	}
	if json.Unmarshal(collectionSocialCatalogData, &rule) != nil || rule.DailyLikes != 99 {
		panic("Android收藏室访问目录无效")
	}
	return rule
}()

// 每日边界沿用本服 UTC+8 运营策略。只有新一天首次合法事务才清当日去重表。
func collectionLikeDay(st *CollectionState, now time.Time) error {
	day := collectionDay(now)
	if st.LikeDay > day {
		return errors.New("收藏室点赞日期回拨")
	}
	if st.LikeDay != day {
		st.LikeDay = day
		st.LikedOwners = map[string]bool{}
	}
	if st.LikedOwners == nil {
		st.LikedOwners = map[string]bool{}
	}
	if st.DailyReceivedLikes == nil {
		st.DailyReceivedLikes = map[int]int64{}
	}
	if st.ReceivedLikes < 0 {
		return errors.New("收藏室获赞存档无效")
	}
	return nil
}

func collectionSocialProperties(p Progress, now time.Time) map[string]any {
	st := p.Collection
	likes := int64(0)
	owners := mobileproto.Map{}
	daily := map[int]int64{}
	if st != nil {
		likes = st.ReceivedLikes
		for day, count := range st.DailyReceivedLikes {
			daily[day] = count
		}
		if st.LikeDay == collectionDay(now) {
			for id, liked := range st.LikedOwners {
				owners = append(owners, mobileproto.Pair{Key: ObjectID(id), Value: liked})
			}
		}
	}
	return map[string]any{"house_likes": likes, "house_likes_map": owners, "daily_house_likes": activityWire(daily)}
}

func collectionVisitAllowed(self, peer *Avatar) error {
	if self == nil || peer == nil || string(self.OID) == string(peer.OID) || self.Hostnum != peer.Hostnum {
		return runeReject("RET_HOUSE_VISIT_NO_THIS_FRIEND", "收藏室访问目标无效")
	}
	if self.Progress.UnlockSystems["real_house_visit"] <= 0 && self.Progress.UnlockSystems["house_visit"] <= 0 {
		return runeReject("RET_HOUSE_VISIT_NOT_UNLOCK", "收藏室访问系统未解锁")
	}
	selfID, peerID := hexOf(self.OID), hexOf(peer.OID)
	if _, yes := self.Progress.Social.Blacklist[peerID]; yes {
		return runeReject("RET_HOUSE_VISIT_NOT_FRIEND", "访问目标已屏蔽")
	}
	if _, yes := peer.Progress.Social.Blacklist[selfID]; yes {
		return runeReject("RET_HOUSE_VISIT_NOT_FRIEND", "对方已屏蔽访问者")
	}
	st := peer.Progress.Collection
	if st == nil || len(st.Rooms) == 0 {
		return runeReject("RET_HOUSE_VISIT_NOT_UNLOCK", "对方收藏室尚未解锁")
	}
	if _, yes := st.Rooms[st.VisitRoom]; !yes {
		return runeReject("RET_HOUSE_VISIT_NOT_UNLOCK", "对方访问房间尚未解锁")
	}
	_, ours := self.Progress.Social.Friends[peerID]
	_, theirs := peer.Progress.Social.Friends[selfID]
	if !st.AllowVisitors && !(ours && theirs) {
		return runeReject("RET_HOUSE_VISIT_NOT_FRIEND", "对方仅允许好友访问")
	}
	return nil
}

func collectionVisitWire(peer Avatar, now time.Time) map[string]any {
	p := peer.Progress
	props := collectionProperties(p, now)
	info := socialProfile(peer).wire()
	info["__custom_type"] = "avatar_info.avatar_info"
	info["house_likes"] = p.Collection.ReceivedLikes
	return map[string]any{"__custom_type": "restroom.house_total_info", "restroom_grid_info": props["restroom_grid_info"], "restroom_wallpapers": props["restroom_wallpapers"], "restroom_info": props["restroom_info"], "facility_info": props["facility_info"], "can_be_visited_by_stranger": p.Collection.AllowVisitors, "room_id_for_visited": p.Collection.VisitRoom, "player_info": info, "captain": []int{effectiveCaptainID(p), p.CaptainDresses[effectiveCaptainID(p)]}}
}

func (s *Service) collectionSocialRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("收藏室访问需要玩家状态")
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil, errors.New("账号存储不支持收藏室双角色事务")
	}
	now := s.Now()
	selfID := hexOf(selectedOID(c))
	target := ""
	host := c.hostnum
	switch method {
	case "query_friend_dormitory":
		if len(args) != 2 || json.Unmarshal(args[0], &target) != nil || json.Unmarshal(args[1], &host) != nil || host <= 0 {
			return nil, errors.New("收藏室访问签名为角色和服务器")
		}
	case "like_friend_house":
		if len(args) != 1 || json.Unmarshal(args[0], &target) != nil {
			return nil, errors.New("收藏室点赞签名为角色")
		}
	case "end_visiting_house":
		if len(args) != 0 {
			return nil, errors.New("结束收藏室访问不接受参数")
		}
	default:
		return nil, errors.New("收藏室访问接口无效")
	}
	ids := []string{selfID}
	if target != "" {
		if _, e := SocialOID(target); e != nil {
			return nil, e
		}
		ids = append(ids, target)
	}
	var visited map[string]any
	rows, err := store.UpdateSocial(ctx, ids, func(avatars map[string]*Avatar) error {
		self := avatars[selfID]
		if self == nil {
			return errors.New("没有鉴权收藏室角色")
		}
		if e := ensureCollection(&self.Progress, now); e != nil {
			return e
		}
		st := self.Progress.Collection
		if method == "end_visiting_house" {
			st.VisitOwner = ""
			st.VisitTime = 0
			return nil
		}
		peer := avatars[target]
		if peer == nil || peer.Hostnum != host {
			return runeReject("RET_HOUSE_VISIT_NO_THIS_FRIEND", "收藏室目标与服务器不一致")
		}
		if e := collectionVisitAllowed(self, peer); e != nil {
			return e
		}
		if method == "query_friend_dormitory" {
			if e := collectionLikeDay(st, now); e != nil {
				return e
			}
			st.VisitOwner = target
			st.VisitTime = float64(now.UnixNano()) / 1e9
			visited = collectionVisitWire(*peer, now)
			return nil
		}
		if st.VisitOwner != target || st.VisitTime <= 0 || st.VisitTime > float64(now.UnixNano())/1e9 {
			return runeReject("RET_HOUSE_VISITING_NO_THIS_FRIEND", "尚未授权访问此收藏室")
		}
		if e := collectionLikeDay(st, now); e != nil {
			return e
		}
		if st.LikedOwners[target] {
			return runeReject("RET_HOUSE_LIKE_ALREADY", "今天已为此收藏室点赞")
		}
		if len(st.LikedOwners) >= androidCollectionSocial.DailyLikes {
			return runeReject("RET_HOUSE_LIKE_DAILY_LIMIT", "已达每日点赞次数上限")
		}
		other := peer.Progress.Collection
		if e := collectionLikeDay(other, now); e != nil {
			return e
		}
		dateKey, e := strconv.Atoi(strings.ReplaceAll(collectionDay(now), "-", ""))
		if e != nil {
			return errors.New("收藏室日界键无效")
		}
		if other.ReceivedLikes == math.MaxInt64 || other.DailyReceivedLikes[dateKey] == math.MaxInt64 {
			return errors.New("收藏室获赞计数溢出")
		}
		st.LikedOwners[target] = true
		other.ReceivedLikes++
		other.DailyReceivedLikes[dateKey]++
		advanceAchievementEvent(&self.Progress, 1009, 2, now)
		advanceAchievementEvent(&peer.Progress, 1009, 1, now)
		reconcileAchievementState(&self.Progress, self.Progress.AvatarLevel, now)
		reconcileAchievementState(&peer.Progress, peer.Progress.AvatarLevel, now)
		return nil
	})
	code := RetSuccess
	if err != nil {
		var refusal *runeBusinessError
		if !errors.As(err, &refusal) {
			return nil, err
		}
		code = androidCollection.Errors[refusal.Name]
		if code <= 0 {
			return nil, errors.New("收藏室访问错误码缺失")
		}
	} else {
		acceptSocialRows(c, rows)
	}
	out := []Push{}
	if err == nil {
		props := collectionSocialProperties(c.SelectedAvatarUnsafe().Progress, now)
		for _, field := range []string{"house_likes", "house_likes_map", "daily_house_likes"} {
			out = append(out, push("Avatar", "client_prop_changed", []any{field, props[field]}))
		}
	}
	switch method {
	case "query_friend_dormitory":
		if visited == nil {
			visited = map[string]any{"__custom_type": "restroom.house_total_info"}
		}
		return append(out, push("Avatar", "on_query_friend_dormitory", code, ObjectID(target), visited)), nil
	case "like_friend_house":
		return append(out, push("Avatar", "on_like_friend_house", ObjectID(target), code)), nil
	default:
		return append(out, push("Avatar", "on_end_visiting_house", code)), nil
	}
}
