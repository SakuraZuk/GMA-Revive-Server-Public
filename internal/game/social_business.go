package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"hs-server/internal/mobileproto"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

//go:embed social_catalog.json
var socialCatalogData []byte
var socialCatalog = func() struct {
	Constants map[string]int                        `json:"constants"`
	Errors    map[string]int                        `json:"errors"`
	Tables    map[string]map[string]json.RawMessage `json:"tables"`
} {
	var v struct {
		Constants map[string]int                        `json:"constants"`
		Errors    map[string]int                        `json:"errors"`
		Tables    map[string]map[string]json.RawMessage `json:"tables"`
	}
	if json.Unmarshal(socialCatalogData, &v) != nil || v.Constants["MAX_FRIEND_COUNT"] <= 0 {
		panic("Android社交目录无效")
	}
	return v
}()

type SocialProfile struct {
	EID        string `json:"eid"`
	UID        int64  `json:"uid"`
	Hostnum    int    `json:"hostnum"`
	Nickname   string `json:"nickname"`
	Level      int    `json:"level"`
	HeadID     int    `json:"head_id"`
	HeadBoxID  int    `json:"head_box_id"`
	CustomHead string `json:"custom_head_image_url"`
	Score      int    `json:"sync_pvp_score"`
}
type SocialFriend struct {
	Info     SocialProfile `json:"info"`
	Time     int64         `json:"time"`
	Intimacy int           `json:"intimacy"`
	Message  string        `json:"msg,omitempty"`
}
type SocialState struct {
	Revision      int64                   `json:"revision"`
	Friends       map[string]SocialFriend `json:"friends,omitempty"`
	Requests      map[string]SocialFriend `json:"requests,omitempty"`
	Blacklist     map[string]SocialFriend `json:"blacklist,omitempty"`
	RequestFlag   bool                    `json:"request_flag"`
	Comments      map[string]CardComment  `json:"comments,omitempty"`
	CommentDay    string                  `json:"comment_day,omitempty"`
	CommentCounts map[int]int             `json:"comment_counts,omitempty"`
	Remarks       map[int]AvatarRemark    `json:"remarks,omitempty"`
	LikedComments map[string]int          `json:"liked_comments,omitempty"`
	Assist        FriendAssistState       `json:"friend_assist,omitempty"`
	Challenges    map[string]SocialFriend `json:"challenges,omitempty"`
	ChallengeSent map[string]int64        `json:"challenge_sent,omitempty"`
	HumanRoom     *HumanPvpRoom           `json:"human_room,omitempty"`
	HumanRefusals []HumanRefusal          `json:"human_refusals,omitempty"`
}
type AvatarRemark struct {
	Like       bool     `json:"like_flag"`
	Tags       []int    `json:"tags"`
	CommentIDs []string `json:"comment_ids"`
}
type CardComment struct {
	ID        string          `json:"comment_id"`
	Owner     string          `json:"owner"`
	CardID    int             `json:"card_id"`
	Content   string          `json:"content"`
	CreatedAt int64           `json:"time"`
	Profile   SocialProfile   `json:"comment_avatar"`
	Likes     map[string]bool `json:"likes,omitempty"`
	Deleted   bool            `json:"deleted,omitempty"`
}
type socialError struct {
	code    int
	message string
}

func (e socialError) Error() string { return e.message }
func socialFailure(name, msg string) error {
	code := socialCatalog.Errors[name]
	if code <= 0 {
		code = socialCatalog.Errors["RET_FRIEND_NOT_EXIST"]
	}
	return socialError{code, msg}
}
func ensureSocial(v *SocialState) {
	if v.Friends == nil {
		v.Friends = map[string]SocialFriend{}
	}
	if v.Requests == nil {
		v.Requests = map[string]SocialFriend{}
	}
	if v.Blacklist == nil {
		v.Blacklist = map[string]SocialFriend{}
	}
	if v.CommentCounts == nil {
		v.CommentCounts = map[int]int{}
	}
	if v.Remarks == nil {
		v.Remarks = map[int]AvatarRemark{}
	}
	if v.LikedComments == nil {
		v.LikedComments = map[string]int{}
	}
	if v.Comments == nil {
		v.Comments = map[string]CardComment{}
	}
	if v.Challenges == nil {
		v.Challenges = map[string]SocialFriend{}
	}
	if v.ChallengeSent == nil {
		v.ChallengeSent = map[string]int64{}
	}
}
func socialProfile(av Avatar) SocialProfile {
	info := av.Info
	if av.Progress.AvatarLevel > 0 {
		info.Level = av.Progress.AvatarLevel
	}
	if av.Progress.SelectedHeadID > 0 {
		info.HeadID = av.Progress.SelectedHeadID
	}
	if av.Progress.SelectedHeadBoxID > 0 {
		info.HeadBoxID = av.Progress.SelectedHeadBoxID
	}
	if av.Progress.CustomHeadCleared {
		info.CustomHeadImageURL = ""
	}
	return SocialProfile{hexOf(av.OID), av.UID, av.Hostnum, info.Nickname, info.Level, info.HeadID, info.HeadBoxID, info.CustomHeadImageURL, av.Progress.SyncPvpScore}
}
func (p SocialProfile) wire() map[string]any {
	return map[string]any{"eid": ObjectID(p.EID), "uid": p.UID, "hostnum": p.Hostnum, "nickname": p.Nickname, "level": p.Level, "head_id": p.HeadID, "head_box_id": p.HeadBoxID, "custom_head_image_url": p.CustomHead, "online": false, "extra": map[string]any{"sync_pvp_score": p.Score}}
}
func SocialProperties(v SocialState) map[string]any {
	dictionary := func(entries map[string]SocialFriend, request bool) mobileproto.Map {
		out := mobileproto.Map{}
		ids := []string{}
		for id := range entries {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			f := entries[id]
			row := map[string]any{"eid": ObjectID(id), "info": f.Info.wire(), "time": float64(f.Time), "intimacy": f.Intimacy, "crossword": map[string]any{}}
			if request {
				row["msg"] = f.Message
			}
			out = append(out, mobileproto.Pair{Key: ObjectID(id), Value: row})
		}
		return out
	}
	remarks := map[string]any{}
	for card, remark := range v.Remarks {
		remarks[intString(card)] = map[string]any{"like_flag": remark.Like, "tags": remark.Tags}
	}
	likes := mobileproto.Map{}
	for id, card := range v.LikedComments {
		likes = append(likes, mobileproto.Pair{Key: ObjectID(id), Value: card})
	}
	blacklist := mobileproto.Map{}
	for id, f := range v.Blacklist {
		blacklist = append(blacklist, mobileproto.Pair{Key: ObjectID(id), Value: f.Info.wire()})
	}
	flag := 0
	if v.RequestFlag {
		flag = 1
	}
	props := map[string]any{"friend_dict": dictionary(v.Friends, false), "friend_request_dict": dictionary(v.Requests, true), "black_list": blacklist, "friend_request_flag": flag, "archive_card_2_comment_times": v.CommentCounts, "archive_card_2_remark": remarks, "archive_like_comment_ids": likes}
	props["challenge_dict"] = dictionary(v.Challenges, false)
	for key, value := range friendAssistProperties(v.Assist) {
		props[key] = value
	}
	return props
}
func (c CardComment) wire() map[string]any {
	return map[string]any{"comment_id": ObjectID(c.ID), "content": c.Content, "time": c.CreatedAt, "like_count": len(c.Likes), "comment_avatar": c.Profile.wire(), "recommend_rate": float64(0)}
}
func intString(n int) string { b, _ := json.Marshal(n); return string(b) }
func socialPushes(v SocialState) []Push {
	props := SocialProperties(v)
	keys := []string{"friend_dict", "friend_request_dict", "black_list", "friend_request_flag", "archive_card_2_comment_times", "archive_card_2_remark", "archive_like_comment_ids", "all_friend_assist_cards", "current_assist", "assist_active_use_times", "assist_passive_use_times", "assist_reward_times"}
	out := []Push{}
	for _, key := range keys {
		out = append(out, push("Avatar", "client_prop_changed", []any{key, props[key]}))
	}
	return out
}

// socialRPC 只使用鉴权角色作为操作者；客户端上传头像、昵称不能覆盖目标资料。
func (s *Service) socialRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("社交请求需要玩家状态")
	}
	switch method {
	case "challenge_friend", "agree_challenge", "cancel_challenge", "refuse_challenge":
		return s.humanChallengeRPC(ctx, c, method, args)
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil, errors.New("账号存储不支持社交事务")
	}
	if method == "query_card_remark" || method == "like_card_remark" || method == "unlike_card_remark" || method == "select_card_tags" {
		return s.cardRemarkRPC(ctx, c, store, method, args)
	}
	if strings.Contains(method, "card_comment") {
		return s.commentRPC(ctx, c, store, method, args)
	}
	cb, ok := callbackArg(args)
	if method == "clear_friend_request_flag" {
		if len(args) != 0 {
			return nil, errors.New("清除申请标记不带参数")
		}
		cb = -1
		ok = true
	}
	if !ok {
		return nil, errors.New("社交回调参数无效")
	}
	fail := func(err error) ([]Push, error) {
		var se socialError
		if errors.As(err, &se) {
			if cb < 0 {
				return nil, err
			}
			return []Push{Callback(cb, []any{se.code})}, nil
		}
		return nil, err
	}
	selfID := hexOf(selectedOID(c))
	snapshot, err := store.SocialAvatars(ctx, SocialSearch{OIDs: []string{selfID}, Limit: 1})
	if err != nil || len(snapshot) != 1 {
		if err == nil {
			err = errors.New("社交角色不存在")
		}
		return nil, err
	}
	own := snapshot[0]
	ensureSocial(&own.Progress.Social)
	if method == "search_friend" || method == "get_recommend_friends" || method == "get_player_details" {
		return s.socialQuery(ctx, c, store, method, args, cb, own)
	}
	ids := []string{selfID}
	target := ""
	message := ""
	switch method {
	case "apply_friend":
		if len(args) != 5 || json.Unmarshal(args[1], &target) != nil || json.Unmarshal(args[2], &message) != nil {
			return nil, errors.New("好友申请签名为回调、角色、留言、服务器、来源")
		}
		var host, src int
		if json.Unmarshal(args[3], &host) != nil || host <= 0 || json.Unmarshal(args[4], &src) != nil {
			return nil, errors.New("好友申请来源参数无效")
		}
		if !utf8.ValidString(message) || utf8.RuneCountInString(message) > 100 {
			return nil, errors.New("好友留言最长一百字符")
		}
		peers, e := store.SocialAvatars(ctx, SocialSearch{OIDs: []string{target}, Limit: 1})
		if e != nil {
			return nil, e
		}
		if len(peers) != 1 || peers[0].Hostnum != host {
			return fail(socialFailure("RET_FRIEND_NOT_EXIST", "申请目标与服务器不一致"))
		}
	case "agree_apply_friend", "refuse_apply_friend", "delete_friend", "delete_black_list":
		if len(args) != 2 || json.Unmarshal(args[1], &target) != nil {
			return nil, errors.New("好友关系签名为回调、角色")
		}
	case "add_black_list":
		if len(args) != 2 {
			return nil, errors.New("添加黑名单签名为回调、角色资料")
		}
		var info map[string]json.RawMessage
		if json.Unmarshal(args[1], &info) != nil {
			return nil, errors.New("黑名单资料无效")
		}
		if json.Unmarshal(info["eid"], &target) != nil {
			if json.Unmarshal(info["_id"], &target) != nil {
				return nil, errors.New("黑名单角色标识缺失")
			}
		}
	case "agree_all_apply_friend", "refuse_all_apply_friend":
		if len(args) != 1 {
			return nil, errors.New("批量申请操作只有回调")
		}
		for id := range own.Progress.Social.Requests {
			ids = append(ids, id)
		}
	case "clear_friend_request_flag":
	default:
		return nil, errors.New("社交接口尚无原生业务证据")
	}
	if target != "" {
		if _, e := SocialOID(target); e != nil {
			return nil, e
		}
		ids = append(ids, target)
	}
	rows, err := store.UpdateSocial(ctx, ids, func(avatars map[string]*Avatar) error {
		self := avatars[selfID]
		if self == nil {
			return errors.New("没有鉴权社交角色")
		}
		ensureSocial(&self.Progress.Social)
		state := &self.Progress.Social
		transition := func(peerID string) error {
			peer := avatars[peerID]
			if peer == nil {
				return socialFailure("RET_FRIEND_NOT_EXIST", "目标角色不存在")
			}
			ensureSocial(&peer.Progress.Social)
			other := &peer.Progress.Social
			if peerID == selfID {
				return socialFailure("RET_FRIEND_CAN_NOT_SEND_SELF", "不能操作自身好友关系")
			}
			switch method {
			case "apply_friend":
				if _, yes := state.Friends[peerID]; yes {
					return socialFailure("RET_FRIEND_ALREADY_FRIEND", "双方已经是好友")
				}
				if len(state.Friends) >= socialCatalog.Constants["MAX_FRIEND_COUNT"] || len(other.Friends) >= socialCatalog.Constants["MAX_FRIEND_COUNT"] {
					return socialFailure("RET_FRIEND_COUNT_REACH_MAX", "好友达到上限")
				}
				if _, yes := state.Blacklist[peerID]; yes {
					return socialFailure("RET_FRIEND_IN_BLACK", "目标在黑名单")
				}
				if _, yes := other.Blacklist[selfID]; yes {
					return socialFailure("RET_FRIEND_HAS_IN_BLACK_LIST", "目标禁止该申请")
				}
				if _, exists := other.Requests[selfID]; !exists {
					if len(other.Requests) >= socialCatalog.Constants["MAX_FRIEND_COUNT"] {
						return socialFailure("RET_FRIEND_FORBID_ADD", "申请队列达到上限")
					}
					other.Requests[selfID] = SocialFriend{Info: socialProfile(*self), Time: s.Now().Unix(), Message: message}
					other.RequestFlag = true
				}
			case "agree_apply_friend", "agree_all_apply_friend":
				if _, yes := state.Friends[peerID]; yes {
					delete(state.Requests, peerID)
					return nil
				}
				if _, yes := state.Requests[peerID]; !yes {
					return socialFailure("RET_FRIEND_NOT_IN_REQUEST", "没有该角色申请")
				}
				if _, yes := state.Blacklist[peerID]; yes {
					return socialFailure("RET_FRIEND_IN_BLACK", "目标在黑名单")
				}
				if _, yes := other.Blacklist[selfID]; yes {
					return socialFailure("RET_FRIEND_HAS_IN_BLACK_LIST", "对方已拉黑")
				}
				if len(state.Friends) >= socialCatalog.Constants["MAX_FRIEND_COUNT"] || len(other.Friends) >= socialCatalog.Constants["MAX_FRIEND_COUNT"] {
					return socialFailure("RET_FRIEND_COUNT_REACH_MAX", "双方好友容量不足")
				}
				state.Friends[peerID] = SocialFriend{Info: socialProfile(*peer), Time: s.Now().Unix()}
				other.Friends[selfID] = SocialFriend{Info: socialProfile(*self), Time: s.Now().Unix()}
				delete(state.Requests, peerID)
				delete(other.Requests, selfID)
			case "refuse_apply_friend", "refuse_all_apply_friend":
				delete(state.Requests, peerID)
			case "delete_friend":
				delete(state.Friends, peerID)
				delete(other.Friends, selfID)
			case "add_black_list":
				if _, exists := state.Blacklist[peerID]; !exists {
					if len(state.Blacklist) >= socialCatalog.Constants["MAX_BLACK_LIST_COUNT"] {
						return socialFailure("RET_FRIEND_BLACK_COUNT_REACH_MAX", "黑名单达到上限")
					}
					state.Blacklist[peerID] = SocialFriend{Info: socialProfile(*peer), Time: s.Now().Unix()}
				}
				delete(state.Friends, peerID)
				delete(other.Friends, selfID)
				delete(state.Requests, peerID)
				delete(other.Requests, selfID)
			case "delete_black_list":
				delete(state.Blacklist, peerID)
			}
			other.Revision++
			return nil
		}
		if target != "" {
			if e := transition(target); e != nil {
				return e
			}
		} else if method == "clear_friend_request_flag" {
			state.RequestFlag = false
		} else {
			peers := append([]string{}, ids[1:]...)
			sort.Strings(peers)
			for _, id := range peers {
				if _, yes := state.Requests[id]; yes {
					if e := transition(id); e != nil {
						return e
					}
				}
			}
		}
		state.Revision++
		for _, av := range avatars {
			reconcileSocialAchievements(av, s.Now())
		}
		return nil
	})
	if err != nil {
		return fail(err)
	}
	for _, av := range rows {
		c.pendingSocialOIDs = append(c.pendingSocialOIDs, append([]byte{}, av.OID...))
	}
	for _, av := range rows {
		if hexOf(av.OID) == selfID {
			for i, current := range c.identity.Avatars {
				if current.Hostnum == c.hostnum {
					c.identity.Avatars[i].Progress = av.Progress
				}
			}
			result := s.socialLivePushes(av.Progress.Social)
			if cb >= 0 {
				result = append(result, Callback(cb, []any{RetSuccess}))
			}
			return result, nil
		}
	}
	return nil, errors.New("社交事务未返回本人状态")
}

// 好友成就按服务端当前真实好友数量投影，重复删除/重加不会累加事件次数。
func reconcileSocialAchievements(av *Avatar, now time.Time) {
	ensureAchievements(&av.Progress)
	for id, rule := range androidAchievements.Rules {
		a := av.Progress.Achievements[id]
		for _, group := range rule.Targets {
			for _, tid := range group {
				target := androidAchievements.Targets[tid]
				if target.Type != 11 || len(target.Params) != 1 || string(target.Params[0]) != "1" {
					continue
				}
				count := int64(len(av.Progress.Social.Friends))
				if count > rule.Need {
					count = rule.Need
				}
				if a.Targets == nil {
					a.Targets = map[int]int64{}
				}
				if count > a.Targets[tid] {
					a.Targets[tid] = count
				}
			}
		}
		if a.Time == 0 && achievementCount(a, rule) >= rule.Need {
			a.Time = now.Unix()
		}
		av.Progress.Achievements[id] = a
	}
}
func (s *Service) socialQuery(ctx context.Context, c *Connection, store SocialAccounts, method string, args []json.RawMessage, cb int, own Avatar) ([]Push, error) {
	switch method {
	case "search_friend":
		var host int
		var uid int64
		var name string
		var valid bool
		if len(args) != 5 || json.Unmarshal(args[1], &host) != nil || host < 0 || json.Unmarshal(args[2], &uid) != nil || uid < 0 || json.Unmarshal(args[3], &name) != nil || json.Unmarshal(args[4], &valid) != nil || !valid || utf8.RuneCountInString(name) > 100 {
			return nil, errors.New("好友搜索签名或文本无效")
		}
		if uid == 0 && strings.TrimSpace(name) == "" {
			return nil, errors.New("好友搜索条件不能为空")
		}
		rows, err := store.SocialAvatars(ctx, SocialSearch{Hostnum: host, UID: uid, Nickname: name, Limit: 100})
		if err != nil {
			return nil, err
		}
		list := []any{}
		for _, av := range rows {
			if string(av.OID) != string(own.OID) {
				list = append(list, s.socialInfo(socialProfile(av)))
			}
		}
		ret := RetSuccess
		if len(list) == 0 {
			ret = socialCatalog.Errors["RET_FRIEND_NOT_EXIST"]
		}
		return []Push{Callback(cb, []any{ret, list})}, nil
	case "get_recommend_friends":
		var force bool
		if len(args) != 2 || json.Unmarshal(args[1], &force) != nil {
			return nil, errors.New("好友推荐签名无效")
		}
		rows, err := store.SocialAvatars(ctx, SocialSearch{Hostnum: c.hostnum, Limit: 1000})
		if err != nil {
			return nil, err
		}
		out := mobileproto.Map{}
		for _, av := range rows {
			id := hexOf(av.OID)
			if id == hexOf(own.OID) {
				continue
			}
			if _, yes := own.Progress.Social.Friends[id]; yes {
				continue
			}
			if _, yes := own.Progress.Social.Blacklist[id]; yes {
				continue
			}
			if _, yes := av.Progress.Social.Blacklist[hexOf(own.OID)]; yes {
				continue
			}
			if len(av.Progress.Social.Friends) >= socialCatalog.Constants["MAX_FRIEND_COUNT"] {
				continue
			}
			out = append(out, mobileproto.Pair{Key: ObjectID(id), Value: s.socialInfo(socialProfile(av))})
			if len(out) >= 20 {
				break
			}
		}
		return []Push{Callback(cb, []any{out})}, nil
	case "get_player_details":
		var id string
		var host int
		if len(args) != 3 || json.Unmarshal(args[1], &id) != nil || json.Unmarshal(args[2], &host) != nil {
			return nil, errors.New("角色详情签名无效")
		}
		rows, err := store.SocialAvatars(ctx, SocialSearch{OIDs: []string{id}, Hostnum: host, Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(rows) != 1 {
			return nil, errors.New("目标角色不存在")
		}
		av := rows[0]
		info := s.socialInfo(socialProfile(av))
		info["card_mgr"] = cardMgrPropertiesWithRunes(av.Progress.Cards, av.Progress.Runes)
		info["rune_mgr"] = runeMgrProperties(av.Progress.Runes)
		info["assist_card_uuid"] = av.Progress.AssistCardUUID
		// 本版player_information直接访问这些公开资料，缺字段会中断页面。
		info["show_cards"], info["show_medals"], info["signature"] = showCardsProperties(av.Progress), []any{}, ""
		info["gender"] = av.Gender
		if av.Gender == 0 {
			info["gender"] = 1
		}
		for key, value := range profileStatisticsProperties(av.Progress) {
			info[key] = value
		}
		return []Push{Callback(cb, []any{info})}, nil
	}
	return nil, errors.New("未知社交查询")
}

// AuthorizedFriendAssist 同事务使用的助战授权：双方关系、双向黑名单与提供者指定卡全部匹配。
func AuthorizedFriendAssist(requester, provider Avatar, uuid string) error {
	if err := AuthorizedFriendRelationship(requester, provider); err != nil {
		return err
	}
	if provider.Progress.AssistCardUUID != uuid {
		return errors.New("该卡不是好友指定助战")
	}
	for _, card := range provider.Progress.Cards {
		if card.UUID == uuid {
			return nil
		}
	}
	return errors.New("好友助战卡不存在")
}
func AuthorizedFriendRelationship(requester, provider Avatar) error {
	id := hexOf(provider.OID)
	own := hexOf(requester.OID)
	if _, ok := requester.Progress.Social.Friends[id]; !ok {
		return errors.New("助战来源不是好友")
	}
	if _, ok := provider.Progress.Social.Friends[own]; !ok {
		return errors.New("好友关系不一致")
	}
	if _, ok := requester.Progress.Social.Blacklist[id]; ok {
		return errors.New("助战来源在黑名单")
	}
	if _, ok := provider.Progress.Social.Blacklist[own]; ok {
		return errors.New("提供者禁止助战")
	}
	return nil
}
func socialDay(now time.Time) string {
	return now.In(time.FixedZone("北京时间", 8*3600)).Format("2006-01-02")
}
