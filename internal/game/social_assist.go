package game

import (
	"context"
	"encoding/json"
	"errors"
	"hs-server/internal/mobileproto"
	"math"
	"sort"
	"time"
)

// FriendAssist 保存真实提供者的卡与契印快照，不写入借用者的卡牌资产。
type FriendAssist struct {
	Profile    SocialProfile   `json:"profile"`
	Card       Card            `json:"card"`
	Runes      map[string]Rune `json:"runes,omitempty"`
	BattleUUID string          `json:"battle_uuid,omitempty"`
}
type FriendAssistState struct {
	Current     *FriendAssist           `json:"current,omitempty"`
	Frozen      *FriendAssist           `json:"frozen,omitempty"`
	Cards       map[string]FriendAssist `json:"cards,omitempty"`
	Strangers   map[string]FriendAssist `json:"strangers,omitempty"`
	Day         string                  `json:"day,omitempty"`
	Active      map[string]int          `json:"active,omitempty"`
	Passive     int                     `json:"passive"`
	RewardTimes int                     `json:"reward_times"`
	LastBattle  string                  `json:"last_battle,omitempty"`
}

func (a FriendAssist) cardWire() any {
	mgr := cardMgrPropertiesWithRunes([]Card{a.Card}, a.Runes)
	return mgr[a.Card.UUID]
}
func (a FriendAssist) wire() map[string]any {
	return map[string]any{"assist_avatar": a.Profile.wire(), "assist_card": a.cardWire()}
}
func friendAssistProperties(v FriendAssistState) map[string]any {
	cards, active := mobileproto.Map{}, mobileproto.Map{}
	ids := []string{}
	for id := range v.Cards {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		cards = append(cards, mobileproto.Pair{Key: ObjectID(id), Value: v.Cards[id].cardWire()})
	}
	ids = ids[:0]
	for id := range v.Active {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		active = append(active, mobileproto.Pair{Key: ObjectID(id), Value: v.Active[id]})
	}
	current := map[string]any{}
	if v.Current != nil {
		current = v.Current.wire()
	}
	strangers := mobileproto.Map{}
	ids = ids[:0]
	for id := range v.Strangers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		strangers = append(strangers, mobileproto.Pair{Key: ObjectID(id), Value: v.Strangers[id].wire()})
	}
	return map[string]any{"all_friend_assist_cards": cards, "all_stranger_assist": strangers, "current_assist": current, "assist_active_use_times": active, "assist_passive_use_times": v.Passive, "assist_reward_times": v.RewardTimes}
}
func snapshotFriendAssist(provider Avatar, uuid string) (FriendAssist, error) {
	_, card := findCard(&provider.Progress, uuid)
	if card == nil {
		return FriendAssist{}, errors.New("好友助战卡不存在")
	}
	rule, ok := androidCardAppearances[card.CardID]
	if !ok || containsInt(rule.Forbid, androidBattleForbidden) {
		return FriendAssist{}, errors.New("好友助战卡禁止战斗")
	}
	p := CloneProgress(provider.Progress)
	result := FriendAssist{Profile: socialProfile(provider), Card: *card, Runes: map[string]Rune{}}
	for id, rune := range p.Runes {
		if rune.CardUUID == uuid {
			result.Runes[id] = rune
		}
	}
	return result, nil
}

// 日界线是本复刻服统一北京时间政策；Android只证明次数/金额，不证明原厂清零时刻。
func ensureFriendAssistDay(p *Progress, now time.Time) error {
	v := &p.Social.Assist
	day := socialDay(now)
	if v.Day > day {
		return errors.New("好友助战时钟回拨")
	}
	if v.Day != day {
		v.Day = day
		v.Active = map[string]int{}
		v.Passive = 0
		v.RewardTimes = 0
	}
	if v.Active == nil {
		v.Active = map[string]int{}
	}
	return nil
}
func assistDungeonAllowed(p Progress, dungeon int) error {
	if p.UnlockSystems["assist"] <= 0 {
		return errors.New("助战系统未解锁")
	}
	var row struct {
		Limit int `json:"assist_limit_type"`
	}
	raw, ok := socialCatalog.Tables["dungeons"][intString(dungeon)]
	if !ok || json.Unmarshal(raw, &row) != nil {
		return errors.New("助战副本目录缺失")
	}
	switch row.Limit {
	case socialCatalog.Constants["ASSIST_LIMIT_ANY"]:
		return nil
	case socialCatalog.Constants["ASSIST_LIMIT_PASS_FIRST"]:
		if containsInt(p.ClearedDungeons, dungeon) {
			return nil
		}
	}
	return errors.New("当前副本不允许该好友助战")
}
func selectedFriendCard(p *Progress, uuid string) *Card {
	if p.Battle != nil && p.Social.Assist.Frozen != nil && p.Social.Assist.Frozen.BattleUUID == p.Battle.UUID && p.Social.Assist.Frozen.Card.UUID == uuid {
		return &p.Social.Assist.Frozen.Card
	}
	if p.Social.Assist.Current != nil && p.Social.Assist.Current.Card.UUID == uuid {
		return &p.Social.Assist.Current.Card
	}
	return nil
}
func battleFormationCard(p *Progress, uuid string) *Card {
	_, card := findCard(p, uuid)
	if card == nil {
		return selectedFriendCard(p, uuid)
	}
	return card
}
func battleCardListWire(p *Progress, uuids []string) []any {
	out := []any{}
	own := cardMgrPropertiesWithRunes(p.Cards, p.Runes)
	for _, id := range uuids {
		if card, ok := own[id]; ok {
			out = append(out, card)
			continue
		}
		assist := p.Social.Assist.Frozen
		if assist == nil || p.Battle == nil || assist.BattleUUID != p.Battle.UUID || assist.Card.UUID != id {
			assist = p.Social.Assist.Current
		}
		if assist != nil && assist.Card.UUID == id {
			out = append(out, assist.cardWire())
		}
	}
	return out
}
func acceptSocialRows(c *Connection, rows []Avatar) {
	for _, av := range rows {
		if string(av.OID) == string(selectedOID(c)) {
			for i := range c.identity.Avatars {
				if string(c.identity.Avatars[i].OID) == string(av.OID) {
					c.identity.Avatars[i] = av
				}
			}
		}
		c.pendingSocialOIDs = append(c.pendingSocialOIDs, append([]byte{}, av.OID...))
	}
}
func (s *Service) friendAssistRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("好友助战需要玩家状态")
	}
	if err := s.refreshPvpAwards(ctx, c); err != nil {
		return nil, err
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil, errors.New("账号存储不支持好友助战事务")
	}
	cb := -1
	uuid, provider := "", ""
	if method == "refresh_assist_use_times" {
		if len(args) != 0 {
			return nil, errors.New("刷新助战次数不带参数")
		}
	} else {
		var valid bool
		cb, valid = callbackArg(args)
		need := 2
		if method == "select_assist_card" {
			need = 3
		}
		if !valid || len(args) != need || json.Unmarshal(args[1], &uuid) != nil || !validObjectID(uuid) {
			return nil, errors.New("好友助战回调或卡号无效")
		}
		if method == "select_assist_card" {
			var info struct {
				EID  string `json:"eid"`
				Host int    `json:"hostnum"`
			}
			if json.Unmarshal(args[2], &info) != nil || !validObjectID(info.EID) || info.Host <= 0 {
				return nil, errors.New("好友助战提供者无效")
			}
			provider = info.EID
			found, e := store.SocialAvatars(ctx, SocialSearch{OIDs: []string{provider}, Hostnum: info.Host, Limit: 1})
			if e != nil {
				return nil, e
			}
			if len(found) != 1 {
				return []Push{Callback(cb, []any{socialCatalog.Errors["RET_CARD_NOT_EXIST"]})}, nil
			}
		}
	}
	selfID := hexOf(selectedOID(c))
	ids := []string{selfID}
	own, e := store.SocialAvatars(ctx, SocialSearch{OIDs: ids, Limit: 1})
	if e != nil {
		return nil, e
	}
	if len(own) != 1 {
		return nil, errors.New("当前角色不存在")
	}
	if method == "refresh_assist_use_times" {
		for id := range own[0].Progress.Social.Friends {
			ids = append(ids, id)
		}
		candidates, err := store.SocialAvatars(ctx, SocialSearch{Hostnum: own[0].Hostnum, Limit: 1000})
		if err != nil {
			return nil, err
		}
		// 查询窗口先随机抽最多10名，行锁事务重新验证这些候选。
		// 不锁住窗口内全部玩家，也不以任意客户端UUID扩大授权。
		sampleSelf := own[0]
		sampleSelf.Progress = CloneProgress(sampleSelf.Progress)
		sample := map[string]*Avatar{selfID: &sampleSelf}
		for i := range candidates {
			peer := candidates[i]
			if hexOf(peer.OID) == selfID {
				continue
			}
			sample[hexOf(peer.OID)] = &peer
		}
		if err := refreshStrangerAssistCandidates(sampleSelf, sample); err != nil {
			return nil, err
		}
		for id := range sampleSelf.Progress.Social.Assist.Strangers {
			ids = append(ids, id)
		}
	} else if provider != "" {
		ids = append(ids, provider)
	}
	rows, err := store.UpdateSocial(ctx, ids, func(avatars map[string]*Avatar) error {
		self := avatars[selfID]
		p := &self.Progress
		if e := ensureFriendAssistDay(p, s.Now()); e != nil {
			return e
		}
		switch method {
		case "set_assist_card", "del_assist_card":
			_, card := findCard(p, uuid)
			if card == nil {
				return errors.New("助战卡不属于当前角色")
			}
			if method == "set_assist_card" {
				if _, e := snapshotFriendAssist(*self, uuid); e != nil {
					return e
				}
				p.AssistCardUUID = uuid
			} else {
				if p.AssistCardUUID != uuid {
					return errors.New("删除的不是指定助战卡")
				}
				p.AssistCardUUID = ""
			}
		case "select_assist_card":
			if p.UnlockSystems["assist"] <= 0 {
				return errors.New("助战系统未解锁")
			}
			peer := avatars[provider]
			if peer == nil {
				return errors.New("助战提供者不存在")
			}
			relation, e := authorizedPlayerAssist(*self, *peer, uuid)
			if e != nil {
				return e
			}
			var rule struct {
				Limit int `json:"assist_limit_num"`
			}
			if json.Unmarshal(socialCatalog.Tables["assist_rule"][intString(relation)], &rule) != nil || rule.Limit <= 0 {
				return errors.New("好友助战次数目录缺失")
			}
			if p.Social.Assist.Active[uuid] >= rule.Limit {
				return errors.New("好友助战次数已用完")
			}
			snap, e := snapshotFriendAssist(*peer, uuid)
			if e != nil {
				return e
			}
			p.Social.Assist.Current = &snap
		case "refresh_assist_use_times":
			p.Social.Assist.Cards = map[string]FriendAssist{}
			for id := range p.Social.Friends {
				peer := avatars[id]
				if peer == nil || peer.Progress.AssistCardUUID == "" {
					continue
				}
				if AuthorizedFriendAssist(*self, *peer, peer.Progress.AssistCardUUID) != nil {
					continue
				}
				snap, e := snapshotFriendAssist(*peer, peer.Progress.AssistCardUUID)
				if e != nil {
					continue
				}
				p.Social.Assist.Cards[id] = snap
				p.Social.Friends[id] = SocialFriend{Info: snap.Profile, Time: p.Social.Friends[id].Time, Intimacy: p.Social.Friends[id].Intimacy, Message: p.Social.Friends[id].Message}
			}
			if err := refreshStrangerAssistCandidates(*self, avatars); err != nil {
				return err
			}
			if current := p.Social.Assist.Current; current != nil {
				peer := avatars[current.Profile.EID]
				_, authErr := authorizedPlayerAssist(*self, valueAvatar(peer), current.Card.UUID)
				if peer == nil || authErr != nil {
					p.Social.Assist.Current = nil
				} else {
					snap, e := snapshotFriendAssist(*peer, current.Card.UUID)
					if e != nil {
						return e
					}
					p.Social.Assist.Current = &snap
				}
			}
		default:
			return errors.New("未知好友助战请求")
		}
		p.Social.Revision++
		return nil
	})
	if err != nil {
		if cb >= 0 {
			return []Push{Callback(cb, []any{socialCatalog.Errors["RET_FAILED"]})}, nil
		}
		return nil, err
	}
	acceptSocialRows(c, rows)
	pushes := s.socialLivePushes(c.SelectedAvatarUnsafe().Progress.Social)
	if method == "set_assist_card" || method == "del_assist_card" {
		value := any(nil)
		if id := c.SelectedAvatarUnsafe().Progress.AssistCardUUID; id != "" {
			value = ObjectID(id)
		}
		pushes = append(pushes, push("Avatar", "client_prop_changed", []any{"assist_card_uuid", value}))
	}
	if cb >= 0 {
		pushes = append(pushes, Callback(cb, []any{RetSuccess}))
	}
	return pushes, nil
}

// updateBattleFormation 将实时好友授权与实际开战写入同一双角色事务。
// consume=false仅布阵/保存预设，不增加使用次数；consume=true按战斗UUID仅结算一次。
func (s *Service) updateBattleFormation(ctx context.Context, c *Connection, layout BattleLayout, dungeon int, consume bool, fn func(*Progress) error) error {
	if err := s.refreshPvpAwards(ctx, c); err != nil {
		return err
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return s.updateProgress(ctx, c, fn)
	}
	selfID := hexOf(selectedOID(c))
	rows, e := store.SocialAvatars(ctx, SocialSearch{OIDs: []string{selfID}, Limit: 1})
	if e != nil {
		return e
	}
	if len(rows) != 1 {
		return errors.New("当前角色不存在")
	}
	p := rows[0].Progress
	external := ""
	for _, uuid := range layout.team() {
		_, card := findCard(&p, uuid)
		if card == nil {
			if external != "" && external != uuid {
				return errors.New("一次出战只能选择一张好友助战")
			}
			external = uuid
		}
	}
	if external == "" {
		return s.updateProgress(ctx, c, func(current *Progress) error {
			for _, uuid := range layout.team() {
				if _, card := findCard(current, uuid); card == nil {
					return errors.New("阵容归属在读取后改变，请重新提交")
				}
			}
			return fn(current)
		})
	}
	if p.Battle != nil && p.Social.Assist.Frozen != nil && p.Social.Assist.Frozen.BattleUUID == p.Battle.UUID && p.Social.Assist.Frozen.Card.UUID == external && (p.Battle.Started || consume && p.Social.Assist.LastBattle == p.Battle.UUID) {
		// 恢复过程迁移Frozen及LastBattle到新UUID；它继承旧场已消费快照，不重新授权/计数。
		observedUUID := p.Battle.UUID
		return s.updateProgress(ctx, c, func(current *Progress) error {
			b, frozen := current.Battle, current.Social.Assist.Frozen
			if b == nil || b.UUID != observedUUID || frozen == nil || frozen.BattleUUID != b.UUID || frozen.Card.UUID != external || !(b.Started || consume && current.Social.Assist.LastBattle == b.UUID) {
				return errors.New("助战冻结场次在读取后改变，请重新提交")
			}
			return fn(current)
		})
	}
	current := p.Social.Assist.Current
	if current == nil || current.Card.UUID != external {
		return errors.New("外部卡牌尚未选择授权助战")
	}
	provider := current.Profile.EID
	rows, e = store.UpdateSocial(ctx, []string{selfID, provider}, func(avatars map[string]*Avatar) error {
		own, peer := avatars[selfID], avatars[provider]
		if own == nil || peer == nil {
			return errors.New("助战角色不存在")
		}
		p := &own.Progress
		if p.Social.Assist.Current == nil || p.Social.Assist.Current.Profile.EID != provider || p.Social.Assist.Current.Card.UUID != external {
			return errors.New("助战选择已变更")
		}
		relation, authErr := authorizedPlayerAssist(*own, *peer, external)
		if authErr != nil {
			return authErr
		}
		if e := ensureFriendAssistDay(p, s.Now()); e != nil {
			return e
		}
		if dungeon == 0 && p.Battle != nil {
			dungeon = p.Battle.DungeonID
		}
		if dungeon > 0 {
			if e := assistDungeonAllowed(*p, dungeon); e != nil {
				return e
			}
		} else if p.UnlockSystems["assist"] <= 0 {
			return errors.New("助战系统未解锁")
		}
		snap, e := snapshotFriendAssist(*peer, external)
		if e != nil {
			return e
		}
		p.Social.Assist.Current = &snap
		if consume {
			if p.Battle == nil || !p.Battle.Loaded || p.Battle.Finished {
				return errors.New("好友助战尚无已加载战斗")
			}
			if p.Social.Assist.LastBattle != p.Battle.UUID {
				var rule struct {
					Limit int `json:"assist_limit_num"`
				}
				if json.Unmarshal(socialCatalog.Tables["assist_rule"][intString(relation)], &rule) != nil || rule.Limit <= 0 {
					return errors.New("好友助战次数目录缺失")
				}
				if p.Social.Assist.Active[external] >= rule.Limit {
					return errors.New("好友助战次数已用完")
				}
				if e := ensureFriendAssistDay(&peer.Progress, s.Now()); e != nil {
					return e
				}
				if peer.Progress.Social.Assist.Passive == math.MaxInt32 {
					return errors.New("被动助战次数溢出")
				}
				var bonus struct {
					ID    int `json:"active_bonus_id"`
					Limit int `json:"active_bonus_limit"`
				}
				if json.Unmarshal(socialCatalog.Tables["assist_active_bonus"]["1"], &bonus) != nil || bonus.ID <= 0 || bonus.Limit <= 0 {
					return errors.New("好友助战奖励目录缺失")
				}
				if p.Social.Assist.RewardTimes < bonus.Limit {
					if _, e := grantPvpFixedBonus(p, bonus.ID, s.Now()); e != nil {
						return e
					}
					p.Social.Assist.RewardTimes++
				}
				p.Social.Assist.Active[external]++
				peer.Progress.Social.Assist.Passive++
				peer.Progress.Social.Revision++
				p.Social.Assist.LastBattle = p.Battle.UUID
				// base_target402001 的参数1是助战事件参数，区别于关系类型 FRIEND=3。
				advanceAchievementEvent(p, 22, 1, s.Now())
			}
			snap.BattleUUID = p.Battle.UUID
			p.Social.Assist.Frozen = &snap
		}
		if e := fn(p); e != nil {
			return e
		}
		reconcileAchievementState(p, p.AvatarLevel, s.Now())
		reconcileAchievementState(&peer.Progress, peer.Progress.AvatarLevel, s.Now())
		p.Social.Revision++
		return nil
	})
	if e != nil {
		return e
	}
	acceptSocialRows(c, rows)
	return nil
}
