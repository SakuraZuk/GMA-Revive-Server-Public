package game

import (
	"context"
	"encoding/json"
	"errors"
	"log"
)

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
			var enabled bool
			if len(args) != 2 || json.Unmarshal(args[1], &enabled) != nil {
				return errors.New("随机看板设置必须为布尔值")
			}
			p.GirlRandomEnable = &enabled
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
	case "set_card_vo":
		property, value = "card_common_mgr", cardCommonMgrPropertiesWithProgress(p)
	case "set_show_cards":
		property, value = "show_cards", showCardsProperties(p)
	case "set_girl_random_enable":
		property, value = "girl_random_enable", *p.GirlRandomEnable
	case "set_explore_auto_agent":
		property, value = "activity_explore_agent", p.ExploreAutoAgent
	}
	code := any(RetSuccess)
	if method == "set_explore_auto_agent" {
		code = true
	}
	return []Push{push("Avatar", "client_prop_changed", []any{property, value}), Callback(cb, []any{code})}, nil
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
