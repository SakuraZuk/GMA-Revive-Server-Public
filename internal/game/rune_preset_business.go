package game

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

const runePresetMax = 100
const runePresetNameLen = 7

func ensureRunePresetState(p *Progress) {
	if p.RuneTemplates == nil {
		p.RuneTemplates = map[string]RuneTemplate{}
	}
	if p.RunePresetSchemaVersion == 0 {
		p.RunePresetSchemaVersion = 1
	}
	for id, row := range p.RuneTemplates {
		if !validObjectID(id) || row.UUID != id {
			delete(p.RuneTemplates, id)
			continue
		}
		if row.Runes == nil {
			row.Runes = map[int]string{}
		}
		if row.TopTimes == nil {
			row.TopTimes = map[int]int64{}
		}
		p.RuneTemplates[id] = row
	}
}

func runeTemplateProperties(p Progress) map[ObjectID]any {
	copy := CloneProgress(p)
	ensureRunePresetState(&copy)
	out := map[ObjectID]any{}
	for id, row := range copy.RuneTemplates {
		uuids := map[int]any{}
		for pos, runeID := range row.Runes {
			uuids[pos] = ObjectID(runeID)
		}
		tops := map[int]int64{}
		for cardID, stamp := range row.TopTimes {
			tops[cardID] = stamp
		}
		out[ObjectID(id)] = map[string]any{
			"uuids": uuids, "name": row.Name, "time": row.Time, "top_times": tops,
		}
	}
	return out
}

func parseRunePresetID(raw json.RawMessage) (string, error) {
	var id string
	if json.Unmarshal(raw, &id) != nil || !validObjectID(id) {
		return "", errors.New("契印预设ObjectID无效")
	}
	return strings.ToLower(id), nil
}

func parseRunePresetIDs(raw json.RawMessage, p Progress) (map[int]string, error) {
	var ids []string
	if json.Unmarshal(raw, &ids) != nil {
		return nil, errors.New("契印预设契印列表无效")
	}
	result := map[int]string{}
	for _, id := range ids {
		id = strings.ToLower(id)
		runeRow, ok := p.Runes[id]
		if !ok || !validObjectID(id) {
			return nil, errors.New("契印不存在")
		}
		if runeRow.Position < 1 || runeRow.Position > 4 {
			return nil, errors.New("契印位置无效")
		}
		if _, exists := result[runeRow.Position]; exists {
			return nil, errors.New("同一位置不能选择多个契印")
		}
		result[runeRow.Position] = id
	}
	return result, nil
}

func runePresetCallback(args []json.RawMessage) (int, error) {
	if len(args) == 0 {
		return 0, errors.New("契印预设缺少callback")
	}
	var callback int
	if json.Unmarshal(args[0], &callback) != nil || callback < 0 {
		return 0, errors.New("契印预设callback无效")
	}
	return callback, nil
}

func (s *Service) runePresetRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("契印预设操作需要玩家状态")
	}
	callback, err := runePresetCallback(args)
	if err != nil {
		return nil, err
	}
	var created string
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		ensureRunePresetState(p)
		now := s.Now()
		newID := func() string {
			for {
				id := hex.EncodeToString(NewAvatarOID(now))
				if _, exists := p.RuneTemplates[id]; !exists {
					return id
				}
			}
		}
		if len(args) < 2 {
			return errors.New("契印预设参数不足")
		}
		switch method {
		case "add_runes_templates":
			if len(args) != 4 {
				return errors.New("新增契印预设参数无效")
			}
			var requested string
			if string(args[1]) != "null" && string(args[1]) != "\"\"" {
				requested, err = parseRunePresetID(args[1])
				if err != nil {
					return err
				}
			}
			var name string
			if json.Unmarshal(args[3], &name) != nil || name == "" || len([]rune(name)) > runePresetNameLen || strings.Contains(name, "#") {
				return errors.New("契印预设名称无效")
			}
			slots, e := parseRunePresetIDs(args[2], *p)
			if e != nil {
				return e
			}
			id := requested
			if id == "" {
				if len(p.RuneTemplates) >= runePresetMax {
					return errors.New("契印预设数量达到上限")
				}
				id = newID()
			} else if _, exists := p.RuneTemplates[id]; !exists && len(p.RuneTemplates) >= runePresetMax {
				return errors.New("契印预设数量达到上限")
			}
			p.RuneTemplates[id] = RuneTemplate{UUID: id, Name: name, Runes: slots, TopTimes: map[int]int64{}, Time: now.Unix()}
			created = id
		case "change_runes_templates_name":
			if len(args) != 3 {
				return errors.New("改契印预设名称参数无效")
			}
			id, e := parseRunePresetID(args[1])
			if e != nil {
				return e
			}
			row, ok := p.RuneTemplates[id]
			if !ok {
				return errors.New("契印预设不存在")
			}
			var name string
			if json.Unmarshal(args[2], &name) != nil || name == "" || len([]rune(name)) > runePresetNameLen || strings.Contains(name, "#") {
				return errors.New("契印预设名称无效")
			}
			row.Name = name
			p.RuneTemplates[id] = row
		case "delete_runes_templates":
			if len(args) != 2 {
				return errors.New("删除契印预设参数无效")
			}
			id, e := parseRunePresetID(args[1])
			if e != nil {
				return e
			}
			if _, ok := p.RuneTemplates[id]; !ok {
				return errors.New("契印预设不存在")
			}
			delete(p.RuneTemplates, id)
		case "update_runes_templates":
			if len(args) != 4 {
				return errors.New("更新契印预设参数无效")
			}
			id, e := parseRunePresetID(args[1])
			if e != nil {
				return e
			}
			row, ok := p.RuneTemplates[id]
			if !ok {
				return errors.New("契印预设不存在")
			}
			var runeID string
			var pos int
			if json.Unmarshal(args[2], &runeID) != nil || json.Unmarshal(args[3], &pos) != nil || pos < 1 || pos > 4 {
				return errors.New("契印预设位置无效")
			}
			runeID = strings.ToLower(runeID)
			if runeID == "" {
				delete(row.Runes, pos)
				p.RuneTemplates[id] = row
				return nil
			}
			r, exists := p.Runes[runeID]
			if !exists || r.Position != pos {
				return errors.New("契印不存在或位置不匹配")
			}
			for existingPos, existingID := range row.Runes {
				if existingPos != pos && existingID == runeID {
					return errors.New("契印已在其他位置")
				}
			}
			row.Runes[pos] = runeID
			p.RuneTemplates[id] = row
		case "set_top_runes_templates":
			if len(args) != 4 {
				return errors.New("设置常用契印预设参数无效")
			}
			id, e := parseRunePresetID(args[1])
			if e != nil {
				return e
			}
			row, ok := p.RuneTemplates[id]
			if !ok {
				return errors.New("契印预设不存在")
			}
			var cardID int
			var untop bool
			if json.Unmarshal(args[2], &cardID) != nil || cardID <= 0 || json.Unmarshal(args[3], &untop) != nil {
				return errors.New("常用契印预设参数无效")
			}
			if !ownsCardID(*p, cardID) {
				return errors.New("卡牌不存在")
			}
			if untop {
				delete(row.TopTimes, cardID)
			} else {
				row.TopTimes[cardID] = now.Unix()
			}
			p.RuneTemplates[id] = row
		case "embed_rune_by_template":
			if len(args) != 3 && len(args) != 4 {
				return errors.New("应用契印预设参数无效")
			}
			if len(args) == 4 {
				var isBattleLayout bool
				if json.Unmarshal(args[3], &isBattleLayout) != nil {
					return errors.New("应用契印预设战斗布局参数无效")
				}
			}
			id, e := parseRunePresetID(args[1])
			if e != nil {
				return e
			}
			row, ok := p.RuneTemplates[id]
			if !ok {
				return errors.New("契印预设不存在")
			}
			var cardID string
			if json.Unmarshal(args[2], &cardID) != nil || !validObjectID(cardID) {
				return errors.New("卡牌不存在")
			}
			cardID = strings.ToLower(cardID)
			if _, card := findCard(p, cardID); card == nil {
				return errors.New("卡牌不存在")
			}
			selected := map[string]bool{}
			for pos, runeID := range row.Runes {
				r, exists := p.Runes[runeID]
				if !exists || r.Position != pos || (r.CardUUID != "" && r.CardUUID != cardID) {
					return errors.New("契印不存在或位置不匹配")
				}
				selected[runeID] = true
			}
			for runeID, r := range p.Runes {
				if r.CardUUID == cardID && !selected[runeID] {
					r.CardUUID = ""
					p.Runes[runeID] = r
				}
			}
			for runeID := range selected {
				r := p.Runes[runeID]
				r.CardUUID = cardID
				p.Runes[runeID] = r
			}
		default:
			return errors.New("契印预设方法未实现")
		}
		return nil
	}); err != nil {
		return []Push{Callback(callback, []any{false, err.Error()})}, nil
	}
	properties := runeTemplateProperties(c.SelectedAvatarUnsafe().Progress)
	pushes := []Push{push("Avatar", "client_prop_changed", []any{"runes_templates", properties})}
	if method == "embed_rune_by_template" {
		pushes = append(pushes, runePush(c), cardMgrPush(c))
	}
	if method == "add_runes_templates" {
		pushes = append(pushes, Callback(callback, []any{ObjectID(created), ""}))
	} else {
		pushes = append(pushes, Callback(callback, []any{true, ""}))
	}
	return pushes, nil
}
