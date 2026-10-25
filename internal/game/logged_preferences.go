package game

import (
	"context"
	"encoding/json"
	"errors"
	"log"
)

// 原生RANDOM_CAPTAIN_TYPE：0关闭、1全部、2收藏；旧布尔档映射为关闭/全部。
func girlRandomMode(p Progress) int {
	if p.GirlRandomType != nil {
		return *p.GirlRandomType
	}
	if p.GirlRandomEnable != nil && *p.GirlRandomEnable {
		return 1
	}
	return 0
}

// 原生C9287FA5展示槽保存卡UUID/None，归还或损坏旧引用投影为空槽。
func showCardsProperties(p Progress) []any {
	values := make([]any, len(p.ShowCards))
	for i, id := range p.ShowCards {
		for _, card := range p.Cards {
			if id != "" && card.UUID == id {
				values[i] = id
				break
			}
		}
	}
	return values
}

// 原生偏好只保存界面选择，不执行战斗、发奖或自动探索。
func (s *Service) loggedPreferenceRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) < 2 {
		return nil, errors.New("偏好设置需要已登录角色及回调")
	}
	var cb int
	if json.Unmarshal(args[0], &cb) != nil || cb <= 0 {
		return nil, errors.New("偏好回调编号无效")
	}
	fail := func(reason string) ([]Push, error) {
		log.Printf("偏好设置拒绝 uid=%d method=%s 原因=%s", c.SelectedAvatarUnsafe().UID, method, reason)
		if method == "set_explore_auto_agent" {
			return []Push{Callback(cb, []any{false})}, nil
		}
		return []Push{Callback(cb, []any{1})}, nil
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		switch method {
		case "set_card_favorites":
			var id int
			var favorite bool
			if len(args) != 3 || json.Unmarshal(args[1], &id) != nil || json.Unmarshal(args[2], &favorite) != nil || !ownsCardID(*p, id) {
				return errors.New("收藏设置需要本人卡牌及布尔开关")
			}
			if p.CardFavorites == nil {
				p.CardFavorites = map[int]bool{}
			}
			p.CardFavorites[id] = favorite
		case "set_card_vo":
			var id, voice int
			if len(args) != 3 || json.Unmarshal(args[1], &id) != nil || json.Unmarshal(args[2], &voice) != nil || !ownsCardID(*p, id) || !containsInt(androidCardAppearances[id].Voices, voice) {
				return errors.New("卡牌归属或原生配音选项无效")
			}
			if p.CardVoices == nil {
				p.CardVoices = map[int]int{}
			}
			p.CardVoices[id] = voice
		case "set_show_cards":
			var slots []*string
			if len(args) != 2 || json.Unmarshal(args[1], &slots) != nil || len(slots) > androidShowCardSlots {
				return errors.New("展示槽数量或参数无效")
			}
			values := make([]string, len(slots))
			seen := map[string]bool{}
			for i, slot := range slots {
				if slot == nil {
					continue
				}
				found := false
				for _, card := range p.Cards {
					if card.UUID == *slot {
						found = true
						break
					}
				}
				if !found || seen[*slot] {
					return errors.New("展示卡不属于玩家或重复")
				}
				values[i] = *slot
				seen[*slot] = true
			}
			p.ShowCards = values
		case "set_girl_random_enable":
			var mode int
			if len(args) != 2 {
				return errors.New("随机看板设置参数数量无效")
			}
			if string(args[1]) == "true" {
				mode = 1
			} else if string(args[1]) == "false" {
				mode = 0
			} else if string(args[1]) == "null" || json.Unmarshal(args[1], &mode) != nil || mode < 0 || mode > 2 {
				return errors.New("随机看板模式必须为原生0、1或2")
			}
			enabled := mode != 0
			p.GirlRandomEnable = &enabled
			p.GirlRandomType = &mode
		case "set_explore_auto_agent":
			var kind int
			var enabled bool
			if len(args) != 3 || json.Unmarshal(args[1], &kind) != nil || kind != androidMikuActivityType || json.Unmarshal(args[2], &enabled) != nil {
				return errors.New("自动探索仅接受原生初音类型及布尔开关")
			}
			if p.ExploreAutoAgent == nil {
				p.ExploreAutoAgent = map[int]bool{}
			}
			p.ExploreAutoAgent[kind] = enabled
		}
		return nil
	})
	if err != nil {
		return fail(err.Error())
	}
	p := c.SelectedAvatarUnsafe().Progress
	var property string
	var value any
	switch method {
	case "set_card_vo", "set_card_favorites":
		property, value = "card_common_mgr", cardCommonMgrPropertiesWithProgress(p)
	case "set_show_cards":
		property, value = "show_cards", showCardsProperties(p)
	case "set_girl_random_enable":
		return []Push{push("Avatar", "client_prop_changed", []any{"girl_random_type", girlRandomMode(p)}), push("Avatar", "client_prop_changed", []any{"girl_random_enable", *p.GirlRandomEnable}), Callback(cb, []any{RetSuccess})}, nil
	case "set_explore_auto_agent":
		property, value = "activity_explore_agent", p.ExploreAutoAgent
	}
	code := any(RetSuccess)
	if method == "set_explore_auto_agent" {
		code = true
	}
	return []Push{push("Avatar", "client_prop_changed", []any{property, value}), Callback(cb, []any{code})}, nil
}

// 原生评分提示只记录渠道与来源，避免重复弹窗；不代表商店实际评分或发奖。
func (s *Service) setGameScoreFlag(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	var channel string
	var source int
	if c.phase != Playing || len(args) != 2 || json.Unmarshal(args[0], &channel) != nil || (channel != "taptap" && channel != "ios") || json.Unmarshal(args[1], &source) != nil || source < 0 {
		return nil, errors.New("评分提示渠道或来源无效")
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if p.GameScoreFlags == nil {
			p.GameScoreFlags = map[string][]int{}
		}
		if !containsInt(p.GameScoreFlags[channel], source) {
			if len(p.GameScoreFlags[channel]) >= 64 {
				return errors.New("评分提示来源超过容量")
			}
			p.GameScoreFlags[channel] = append(p.GameScoreFlags[channel], source)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []Push{push("Avatar", "client_prop_changed", []any{"game_score_flag", c.SelectedAvatarUnsafe().Progress.GameScoreFlags})}, nil
}

const (
	supportHabitKey      = 1
	battleHabitKey       = 2
	aiBattleHabit        = 100
	maxChatChannel       = 999
	maxRuneDecomposeStar = 4
)

// 客户端 card_mgr 在战斗中上报本人卡的自动战斗或助战习惯，不等回调。
// player_input.on_set_auto_battle_habit 以 sync_method 接收，按 SUPPORT_HABIT=1、
// BATTLE_HABIT=2 写入；AI_BATTLE=100，普通值是技能下标。
func (s *Service) setAutoHabit(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("自动习惯需要已登录角色及卡牌和设置")
	}
	var uuid string
	if json.Unmarshal(args[0], &uuid) != nil {
		return nil, errors.New("自动习惯卡牌无效")
	}
	key := battleHabitKey
	value := 0
	if method == "set_auto_support_habit" {
		key = supportHabitKey
		var enabled bool
		if json.Unmarshal(args[1], &enabled) != nil {
			return nil, errors.New("助战习惯必须是布尔值")
		}
		if enabled {
			value = 1
		}
	} else if json.Unmarshal(args[1], &value) != nil || value < 0 || value > aiBattleHabit {
		return nil, errors.New("战斗习惯超出技能下标或自动值")
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		_, card := findCard(p, uuid)
		if card == nil {
			return errors.New("自动习惯卡牌不属于玩家")
		}
		if p.AutoBattleHabits == nil {
			p.AutoBattleHabits = map[string]map[int]int{}
		}
		habit := p.AutoBattleHabits[uuid]
		if habit == nil {
			habit = map[int]int{supportHabitKey: 1, battleHabitKey: aiBattleHabit}
		}
		habit[key] = value
		p.AutoBattleHabits[uuid] = habit
		card.Habit = &CardHabit{Support: habit[supportHabitKey] != 0, Battle: habit[battleHabitKey]}
		return nil
	})
	if err != nil {
		return nil, err
	}
	saved := c.SelectedAvatarUnsafe().Progress.AutoBattleHabits[uuid]
	return []Push{battleSync("on_set_auto_battle_habit", ObjectID(uuid), map[string]any{"__custom_type": "card.card_habit", "support_habit": saved[supportHabitKey] != 0, "battle_habit": saved[battleHabitKey]})}, nil
}

// 客户端设置界面保存可自动分解的契印星级。回调只有一个整数；
// 成功后客户端读取自身 rune_auto_decompose_stars 刷新开关，不从回调取列表。
func (s *Service) saveRuneAutoDecomposeStars(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("契印自动分解需要已登录角色、回调和星级")
	}
	var callbackID int
	if json.Unmarshal(args[0], &callbackID) != nil || callbackID <= 0 {
		return nil, errors.New("契印自动分解回调无效")
	}
	var stars []int
	if json.Unmarshal(args[1], &stars) != nil || len(stars) == 0 || len(stars) > maxRuneDecomposeStar {
		log.Printf("契印自动分解拒绝 uid=%d 原因=星级列表无效", c.SelectedAvatarUnsafe().UID)
		return []Push{Callback(callbackID, []any{1})}, nil
	}
	seen := map[int]bool{}
	clean := make([]int, 0, len(stars))
	for _, star := range stars {
		if star < 1 || star > maxRuneDecomposeStar || seen[star] {
			log.Printf("契印自动分解拒绝 uid=%d 原因=星级超出1至4或重复", c.SelectedAvatarUnsafe().UID)
			return []Push{Callback(callbackID, []any{1})}, nil
		}
		seen[star] = true
		clean = append(clean, star)
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.RuneAutoDecomposeStars = &clean
		return nil
	})
	if err != nil {
		log.Printf("契印自动分解拒绝 uid=%d 原因=%s", c.SelectedAvatarUnsafe().UID, err.Error())
		return []Push{Callback(callbackID, []any{1})}, nil
	}
	saved := append([]int{}, clean...)
	return []Push{push("Avatar", "client_prop_changed", []any{"rune_auto_decompose_stars", saved}), Callback(callbackID, []any{RetSuccess})}, nil
}

// 原生关闭入口只有回调编号。显式空列表表示已关闭，不能用nil恢复未设置默认值。
func (s *Service) removeRuneAutoDecomposeStars(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 1 {
		return nil, errors.New("关闭契印自动分解需要已登录角色和回调")
	}
	var callbackID int
	if json.Unmarshal(args[0], &callbackID) != nil || callbackID <= 0 {
		return nil, errors.New("关闭契印自动分解回调无效")
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		empty := []int{}
		p.RuneAutoDecomposeStars = &empty
		return nil
	}); err != nil {
		log.Printf("关闭契印自动分解失败 uid=%d 原因=%v", c.SelectedAvatarUnsafe().UID, err)
		return []Push{Callback(callbackID, []any{1})}, nil
	}
	return []Push{push("Avatar", "client_prop_changed", []any{"rune_auto_decompose_stars", []int{}}), Callback(callbackID, []any{RetSuccess})}, nil
}

// 客户端切频道前已按 1 至 999 校验。服务端持久化后回 on_change_channel，
// 客户端只在返回成功时用自己已保存的 chat_channel 刷新界面。
func (s *Service) changeChatChannel(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 1 {
		return nil, errors.New("切换频道需要已登录角色及一个频道")
	}
	var channel int
	if json.Unmarshal(args[0], &channel) != nil || channel < 1 || channel > maxChatChannel {
		return []Push{push("Avatar", "on_change_channel", 1)}, nil
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.ChatChannel = channel
		return nil
	})
	if err != nil {
		return []Push{push("Avatar", "on_change_channel", 1)}, nil
	}
	return []Push{push("Avatar", "client_prop_changed", []any{"chat_channel", channel}), push("Avatar", "on_change_channel", RetSuccess)}, nil
}

// 66D65F7F与71BB353A证明这些都是三参单向埋点，没有资产和回调。
func (s *Service) loggedClientTelemetry(c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 3 {
		return nil, errors.New("客户端埋点需要已登录角色及三个参数")
	}
	size := 0
	values := []any{}
	for _, arg := range args {
		size += len(arg)
		var value any
		if json.Unmarshal(arg, &value) != nil {
			return nil, errors.New("客户端埋点参数无效")
		}
		values = append(values, value)
	}
	if size > 16384 {
		return nil, errors.New("客户端埋点超过长度限制")
	}
	now := s.Now().Unix()
	if now-c.telemetryWindow >= 60 {
		c.telemetryWindow, c.telemetryCount = now, 0
	}
	if c.telemetryCount >= 60 {
		return nil, nil
	}
	c.telemetryCount++
	raw, _ := json.Marshal(redactDiagnostic(values))
	log.Printf("客户端埋点 uid=%d method=%s 参数=%s", c.SelectedAvatarUnsafe().UID, method, raw)
	return nil, nil
}
