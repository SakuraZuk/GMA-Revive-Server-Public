package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"hs-server/internal/mobileproto"
)

//go:embed league_catalog.json
var leagueCatalogRaw []byte

var androidLeague = func() struct {
	Tables    map[string]map[string]json.RawMessage `json:"表"`
	Constants map[string]int                        `json:"常量"`
	Errors    map[string]int                        `json:"错误码"`
} {
	var result struct {
		Tables    map[string]map[string]json.RawMessage `json:"表"`
		Constants map[string]int                        `json:"常量"`
		Errors    map[string]int                        `json:"错误码"`
	}
	if json.Unmarshal(leagueCatalogRaw, &result) != nil || len(result.Tables["league_protect"]) != 3 {
		panic("原生学会目录无效")
	}
	return result
}()

func leagueData(table string, id int) activityRow {
	var result activityRow
	_ = json.Unmarshal(androidLeague.Tables[table][intString(id)], &result)
	return result
}

// 学会主档固定保存在创建者的玩家行锁存档，退会不删除主档。
// UpdateAllSocial同时处理主档、真实成员和申请者；事务失败全部回滚。
type LeagueState struct {
	ID            string                  `json:"id"`
	UID           int64                   `json:"uid"`
	Host          int                     `json:"hostnum"`
	Name          string                  `json:"name"`
	PublicNotice  string                  `json:"public_notice"`
	PrivateNotice string                  `json:"private_notice"`
	President     string                  `json:"president"`
	CardID        int                     `json:"card_id"`
	Level         int                     `json:"level"`
	Created       int64                   `json:"created_at"`
	AutoAgree     bool                    `json:"auto_agree"`
	Dissolved     bool                    `json:"dissolved"`
	Members       map[string]LeagueMember `json:"members"`
	Applications  map[string]int64        `json:"applications"`
}

type LeagueMember struct {
	Type   int   `json:"member_type"`
	Joined int64 `json:"join_time"`
}

func leagueMembershipProperties(p Progress) map[string]any {
	var id any
	if validObjectID(p.LeagueID) {
		id = ObjectID(p.LeagueID)
	}
	return map[string]any{"league_id": id, "last_leave_league_time": float64(p.LastLeaveLeague)}
}

func leagueOwner(avatars map[string]*Avatar, id string) (*Avatar, *LeagueState) {
	for _, av := range avatars {
		if state := av.Progress.OwnedLeagues[id]; state != nil && !state.Dissolved {
			return av, state
		}
	}
	return nil, nil
}

func leagueMemberWire(av *Avatar, member LeagueMember) map[string]any {
	result := socialProfile(*av).wire()
	result["member_type"] = member.Type
	result["join_time"] = float64(member.Joined)
	result["account_id"] = nil
	result["last_login_time"], result["last_logout_time"] = float64(0), float64(0)
	result["daily_active"], result["total_active"] = 0, 0
	result["last_receive_welfare_time"] = float64(0)
	result["league_activity_weekly_info"] = leagueProtectProperties(av.Progress)["league_activity_weekly_info"]
	result["league_boss_challenge_highest_damage"], result["league_pvp_personal_score"] = 0, 0
	result["udid"] = ""
	return result
}

func leagueInfoWire(state *LeagueState, avatars map[string]*Avatar) map[string]any {
	if state == nil {
		return map[string]any{}
	}
	members, applications := mobileproto.Map{}, mobileproto.Map{}
	ids := []string{}
	for id := range state.Members {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if av := avatars[id]; av != nil {
			members = append(members, mobileproto.Pair{Key: ObjectID(id), Value: leagueMemberWire(av, state.Members[id])})
		}
	}
	applyList := []any{}
	ids = ids[:0]
	for id := range state.Applications {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if av := avatars[id]; av != nil {
			applications = append(applications, mobileproto.Pair{Key: ObjectID(id), Value: leagueMemberWire(av, LeagueMember{Type: 4, Joined: state.Applications[id]})})
			applyList = append(applyList, []any{ObjectID(id), float64(state.Applications[id])})
		}
	}
	president := ""
	if av := avatars[state.President]; av != nil {
		president = av.Info.Nickname
	}
	cardData, _ := leagueGuardianWire(state, 0)
	return map[string]any{"eid": ObjectID(state.ID), "uid": state.UID, "name": state.Name, "hostnum": state.Host, "level": state.Level, "members": members, "member_count": len(state.Members), "president_name": president, "public_notice": state.PublicNotice, "private_notice": state.PrivateNotice, "auto_agree_flag": state.AutoAgree, "apply_map": applications, "apply_list": applyList, "min_apply_level": 0, "min_apply_active": 0, "daily_active_point": 0, "total_active_point": 0, "league_card": cardData, "weekly_explore_states": map[int]any{}, "weekly_explore_win_times": map[int]int{}, "trial_states": map[int]any{}, "trial_score": map[string]int{}, "boss_challenge_id": 0, "boss_end_time": float64(0), "boss_die_flag": 0, "boss_challenge_dungeon_id": 0, "boss_challenge_hps": []int{}, "boss_max_hps": []int{}, "boss_challenge_damages": map[string]any{}, "events_log": []any{}, "message_board": []any{}, "last_change_league_card_time": float64(state.Created), "buff_begin_time": map[int]float64{}, "league_tasks": map[int]any{}, "task_score": 0, "new_league_challenge": map[string]any{}, "new_pvp_boss_info": map[string]any{}}
}

func joinRealLeague(av *Avatar, state *LeagueState, now time.Time) error {
	id := hexOf(av.OID)
	if av.Hostnum != state.Host || av.Progress.LeagueID != "" {
		return runeReject("RET_LEAGUE_AGREE_TARGET_ALREADY_HAVE_LEAGUE", "申请者已有学会或不在同一服务器")
	}
	if av.Progress.AvatarLevel < leagueData("league_base", 1).integer("create_level_limit") {
		return runeReject("RET_LEAGUE_APPLY_LEVEL_NOT_ENOUGH", "学会系统尚未解锁")
	}
	var cooldown float64
	if json.Unmarshal(leagueData("league_base", 1)["join_league_cd"], &cooldown) != nil {
		return errors.New("学会冷却原生目录无效")
	}
	if now.Unix() < av.Progress.LastLeaveLeague+int64(cooldown*3600) {
		return runeReject("RET_LEAGUE_LEFTED_TIME_NOT_ENOUGH", "退会后申请冷却未结束")
	}
	if len(state.Members) >= leagueData("league_level", state.Level).integer("member_limit") {
		return runeReject("RET_LEAGUE_MEMBER_FULL", "学会成员已满")
	}
	state.Members[id] = LeagueMember{Type: androidLeague.Constants["LEAGUE_MEMBER_TYPE_NORMAL"], Joined: now.Unix()}
	av.Progress.LeagueID = state.ID
	delete(state.Applications, id)
	advanceAchievementEvent(&av.Progress, 26, 1, now)
	reconcileAchievementState(&av.Progress, av.Progress.AvatarLevel, now)
	return nil
}

func (s *Service) leagueRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("学会操作需要真实玩家状态")
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil, errors.New("存储不支持学会事务")
	}
	selfID := hexOf(selectedOID(c))
	var result []Push
	rows, err := store.UpdateAllSocial(ctx, func(avatars map[string]*Avatar) error {
		self := avatars[selfID]
		if self == nil {
			return errors.New("学会事务当前玩家不存在")
		}
		now := s.Now()
		if self.Progress.AvatarLevel < 6 {
			name := "RET_LEAGUE_LEVEL_NOT_ENOUGH"
			if method == "create_new_league" {
				name = "RET_LEAGUE_CREATE_LEVEL_NOT_ENOUGH"
			}
			return runeReject(name, "学会在馆主六级解锁")
		}
		_, state := leagueOwner(avatars, self.Progress.LeagueID)
		readID := func(index int) (string, error) {
			var id string
			if index >= len(args) || json.Unmarshal(args[index], &id) != nil || !validObjectID(id) {
				return "", errors.New("学会或成员编号无效")
			}
			return id, nil
		}
		admin := func() error {
			if state == nil {
				return runeReject("RET_LEAGUE_NOT_JOINED", "当前玩家尚未加入学会")
			}
			member := state.Members[selfID]
			if member.Type != 1 && member.Type != 2 {
				name := "RET_LEAGUE_POWER_ERROR"
				if method == "agree_league_apply" || method == "refuse_league_apply" {
					name = "RET_LEAGUE_AGREE_APPLY_NO_POWER"
				}
				return runeReject(name, "没有学会管理权限")
			}
			return nil
		}
		switch method {
		case "create_new_league":
			var name, notice string
			var cardID int
			if len(args) != 3 || json.Unmarshal(args[0], &name) != nil || json.Unmarshal(args[1], &notice) != nil || json.Unmarshal(args[2], &cardID) != nil || name != strings.TrimSpace(name) || utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > androidLeague.Constants["MAX_LEAGUE_NAME_LENGTH"] || utf8.RuneCountInString(notice) > androidLeague.Constants["MAX_LEAGUE_NOTICE_LENGTH"] {
				return errors.New("学会创建原生参数无效")
			}
			if self.Progress.LeagueID != "" {
				return runeReject("RET_LEAGUE_ALREADY_HAVE_LEAGUE", "已有学会不能再次创建")
			}
			uid := int64(0)
			for _, av := range avatars {
				for _, old := range av.Progress.OwnedLeagues {
					if old.UID > uid {
						uid = old.UID
					}
					if !old.Dissolved && old.Host == self.Hostnum && old.Name == name {
						return runeReject("RET_LEAGUE_ALREADY_EXIST_LEAGUE", "同服学会名称已被使用")
					}
				}
			}
			if uid == 9223372036854775807 {
				return errors.New("学会编号已达上限")
			}
			rarity := leagueData("cards", cardID).integer("rarity")
			costs := leagueData("cards_rarity", rarity).ids("league_card_select_cost")
			if len(costs) != 2 || costs[0] <= 0 || costs[1] <= 0 {
				return errors.New("学会幻书或创建成本无效")
			}
			if _, valid := androidCardSkillTalent.Cards[cardID]; !valid {
				return errors.New("学会幻书不存在")
			}
			if err := runeMaterials(&self.Progress, [][]int{costs}, -1); err != nil {
				var reject *runeBusinessError
				if errors.As(err, &reject) && reject.Name == "RET_MATERIAL_NOT_ENOUGH" {
					return runeReject("RET_LEAGUE_CREATE_COST_ENOUGH", "学会创建材料不足")
				}
				return err
			}
			state = &LeagueState{ID: newBattleUUID(now), UID: uid + 1, Host: self.Hostnum, Name: name, PublicNotice: notice, President: selfID, CardID: cardID, Level: 1, Created: now.Unix(), Members: map[string]LeagueMember{}, Applications: map[string]int64{}}
			if err := joinRealLeague(self, state, now); err != nil {
				return err
			}
			state.Members[selfID] = LeagueMember{Type: 1, Joined: now.Unix()}
			if self.Progress.OwnedLeagues == nil {
				self.Progress.OwnedLeagues = map[string]*LeagueState{}
			}
			self.Progress.OwnedLeagues[state.ID] = state
			result = []Push{push("Avatar", "client_prop_changed", []any{"material_mgr", materialProperties([]Avatar{*self}, self.Hostnum)}), push("Avatar", "on_create_new_league", RetSuccess)}
		case "apply_league":
			if len(args) != 2 {
				return errors.New("申请学会原生参数数量无效")
			}
			id, e := readID(0)
			if e != nil {
				return e
			}
			_, target := leagueOwner(avatars, id)
			if target == nil || target.Host != self.Hostnum {
				return runeReject("RET_LEAGUE_APPLY_ALREADY_DISSOLVED", "学会不存在、已解散或不在同一服")
			}
			if self.Progress.LeagueID != "" {
				return runeReject("RET_LEAGUE_ALREADY_HAVE_LEAGUE", "当前玩家已加入学会")
			}
			if target.AutoAgree {
				if err := joinRealLeague(self, target, now); err != nil {
					return err
				}
			} else {
				if now.Unix() < self.Progress.LastLeaveLeague+14400 {
					return runeReject("RET_LEAGUE_LEFTED_TIME_NOT_ENOUGH", "退会申请冷却尚未结束")
				}
				if len(target.Applications) >= 1000 {
					return runeReject("RET_LEAGUE_ALLPY_MAX_LIMIT", "学会申请队列达到服务保护上限")
				}
				if target.Applications == nil {
					target.Applications = map[string]int64{}
				}
				if _, exists := target.Applications[selfID]; !exists {
					target.Applications[selfID] = now.Unix()
				}
			}
			result = []Push{push("Avatar", "on_apply_league", RetSuccess, ObjectID(id))}
		case "agree_league_apply", "refuse_league_apply":
			if len(args) != 1 {
				return errors.New("学会审批参数数量无效")
			}
			if err := admin(); err != nil {
				return err
			}
			id, e := readID(0)
			if e != nil {
				return e
			}
			if _, exists := state.Applications[id]; !exists {
				return runeReject("RET_LEAGUE_AGREE_APPLY_NOT_EXIST", "该玩家没有待处理入会申请")
			}
			if method == "agree_league_apply" {
				if avatars[id] == nil {
					return runeReject("RET_LEAGUE_AGREE_APPLY_NOT_EXIST", "申请者不存在")
				}
				if err := joinRealLeague(avatars[id], state, now); err != nil {
					return err
				}
			} else {
				delete(state.Applications, id)
			}
			result = []Push{leagueApprovalPush(method, RetSuccess, id, leagueInfoWire(state, avatars))}
		case "league_setup_auto_agree":
			var flag bool
			if len(args) != 1 || json.Unmarshal(args[0], &flag) != nil {
				return errors.New("自动审批参数无效")
			}
			if err := admin(); err != nil {
				return err
			}
			state.AutoAgree = flag
			result = []Push{push("Avatar", "on_league_setup_auto_agree", RetSuccess, flag)}
		case "leave_league":
			if len(args) != 0 {
				return errors.New("退会不接受参数")
			}
			if state == nil {
				return runeReject("RET_LEAGUE_NOT_JOINED", "当前玩家尚未入会")
			}
			if state.President == selfID && len(state.Members) > 1 {
				return runeReject("RET_LEAGUE_PRESIDENT_CANT_LEAVE", "会长须先移交职位才能退会")
			}
			delete(state.Members, selfID)
			if len(state.Members) == 0 {
				state.Dissolved = true
			}
			self.Progress.LeagueID = ""
			self.Progress.LastLeaveLeague = now.Unix()
			result = []Push{push("Avatar", "on_leave_league", RetSuccess)}
		case "appoint_league_member":
			var appoint int
			if len(args) != 2 || json.Unmarshal(args[1], &appoint) != nil {
				return errors.New("学会任命参数无效")
			}
			if appoint != 1 || state == nil || state.President != selfID {
				return runeReject("RET_LEAGUE_APPOINT_INVALID", "会长移交须由当前会长指定职位1")
			}
			id, e := readID(0)
			if e != nil {
				return e
			}
			target := avatars[id]
			member, exists := state.Members[id]
			if id == selfID || !exists || target == nil || target.Hostnum != self.Hostnum || target.Progress.LeagueID != state.ID {
				return runeReject("RET_LEAGUE_APPOINT_NOT_EXIST", "新会长必须为同服当前真实学会成员")
			}
			old := state.Members[selfID]
			old.Type = androidLeague.Constants["LEAGUE_MEMBER_TYPE_NORMAL"]
			state.Members[selfID] = old
			member.Type = 1
			state.Members[id] = member
			state.President = id
			result = []Push{push("Avatar", "on_appoint_league_member", RetSuccess, ObjectID(id))}
		case "query_total_league_info":
			var host int
			if len(args) != 2 || json.Unmarshal(args[1], &host) != nil || host != self.Hostnum {
				return errors.New("学会查询不能跨服")
			}
			id, e := readID(0)
			if e != nil {
				return e
			}
			_, target := leagueOwner(avatars, id)
			if target == nil {
				return errors.New("查询学会不存在")
			}
			result = []Push{push("Avatar", "on_query_total_league_info", leagueInfoWire(target, avatars))}
		case "search_league":
			var name string
			var uid int64
			if len(args) != 2 || json.Unmarshal(args[0], &name) != nil || json.Unmarshal(args[1], &uid) != nil || uid < 0 || utf8.RuneCountInString(name) > 7 {
				return errors.New("学会搜索参数无效")
			}
			list := []any{}
			for _, av := range avatars {
				for _, target := range av.Progress.OwnedLeagues {
					if !target.Dissolved && target.Host == self.Hostnum && (uid == 0 || target.UID == uid) && (name == "" || strings.Contains(target.Name, name)) {
						list = append(list, leagueInfoWire(target, avatars))
					}
				}
			}
			sort.Slice(list, func(i, j int) bool {
				return list[i].(map[string]any)["uid"].(int64) < list[j].(map[string]any)["uid"].(int64)
			})
			if len(list) > 100 {
				list = list[:100]
			}
			result = []Push{push("Avatar", "on_search_league", list)}
		default:
			return errors.New("学会接口尚未实现")
		}
		for id, av := range avatars {
			if id == selfID || state != nil && state.Members[id].Joined > 0 {
				c.pendingSocialOIDs = append(c.pendingSocialOIDs, append([]byte{}, av.OID...))
			}
		}
		return nil
	})
	if err != nil {
		return leagueFailurePush(method, args, err)
	}
	for _, av := range rows {
		if hexOf(av.OID) == selfID {
			for i := range c.identity.Avatars {
				if c.identity.Avatars[i].Hostnum == c.hostnum {
					c.identity.Avatars[i] = av
				}
			}
			break
		}
	}
	for key, value := range leagueMembershipProperties(c.SelectedAvatarUnsafe().Progress) {
		result = append([]Push{push("Avatar", "client_prop_changed", []any{key, value})}, result...)
	}
	return result, nil
}

func leagueFailurePush(method string, args []json.RawMessage, err error) ([]Push, error) {
	var refusal *runeBusinessError
	if !errors.As(err, &refusal) {
		return nil, err
	}
	code, known := androidLeague.Errors[refusal.Name]
	if !known {
		return nil, err
	}
	name := "on_" + method
	switch method {
	case "create_new_league", "leave_league":
		return []Push{push("Avatar", name, code)}, nil
	case "agree_league_apply", "refuse_league_apply":
		var id string
		if len(args) != 1 || json.Unmarshal(args[0], &id) != nil || !validObjectID(id) {
			return nil, err
		}
		return []Push{leagueApprovalPush(method, code, id, nil)}, nil
	case "apply_league", "appoint_league_member":
		var id string
		if len(args) == 0 || json.Unmarshal(args[0], &id) != nil || !validObjectID(id) {
			return nil, err
		}
		return []Push{push("Avatar", name, code, ObjectID(id))}, nil
	case "league_setup_auto_agree":
		var flag bool
		if len(args) != 1 || json.Unmarshal(args[0], &flag) != nil {
			return nil, err
		}
		return []Push{push("Avatar", "on_league_setup_auto_agree", code, flag)}, nil
	}
	return nil, err
}

// Android0F9F9ACB审批回包成功与失败都需要完整位置参数。
// 同意为Int/ObjId/Dict成员快照；拒绝为Int/ObjId/List申请队列/Dict申请快照。
func leagueApprovalPush(method string, code int, id string, info map[string]any) Push {
	members, applications := any(mobileproto.Map{}), any(mobileproto.Map{})
	list := any([]any{})
	if info != nil {
		members = info["members"]
		applications = info["apply_map"]
		list = info["apply_list"]
	}
	if method == "agree_league_apply" {
		return push("Avatar", "on_agree_league_apply", code, ObjectID(id), members)
	}
	return push("Avatar", "on_refuse_league_apply", code, ObjectID(id), list, applications)
}
