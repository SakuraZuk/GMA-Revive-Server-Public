package game

import (
	"context"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// formation_preset_catalog.json 是 Android 1.0.128 表的受控导出，禁止把参考项目数值写回这里。
//
//go:embed formation_preset_catalog.json
var formationPresetCatalogRaw []byte

type formationCardRule struct {
	Position *int  `json:"position"`
	Disable  int   `json:"disable"`
	GMFlag   int   `json:"gm_flag"`
	Forbid   []int `json:"forbid"`
}

type formationDutyRule struct {
	LimitPosition  *int `json:"duty_limit_card_position"`
	AdvicePosition *int `json:"duty_advice_card_position"`
}

type formationScoreRule struct {
	MinScore  int `json:"min_score"`
	MaxScore  int `json:"max_score"`
	LineupNum int `json:"lineup_num"`
}

type formationPresetCatalog struct {
	Cards            map[string]formationCardRule  `json:"cards"`
	MiniDuty         map[string]formationDutyRule  `json:"mini_duty"`
	SyncPvpScoreRule map[string]formationScoreRule `json:"sync_pvp_score_rule"`
}

var androidFormationPresetCatalog = func() formationPresetCatalog {
	var catalog formationPresetCatalog
	if err := json.Unmarshal(formationPresetCatalogRaw, &catalog); err != nil {
		panic("阵容预设 Android 目录无效")
	}
	if len(catalog.Cards) == 0 || len(catalog.MiniDuty) != 6 || len(catalog.SyncPvpScoreRule) == 0 {
		panic("阵容预设 Android 目录不完整")
	}
	return catalog
}()

const (
	presetRecordNum     = 20
	presetRecordNameLen = 7
)

// PresetRecord 对应 Android preset_cards_record 的一条记录。
// cards 保留三组列表和空槽，剧情卡只作为上下文，不写入玩家卡牌资产。
type PresetRecord struct {
	PresetID string       `json:"preset_id"`
	Name     string       `json:"preset_name"`
	Cards    BattleLayout `json:"cards"`
}

func newPresetID(now time.Time, used map[string]bool) string {
	for {
		id := hex.EncodeToString(NewAvatarOID(now))
		if !used[id] {
			used[id] = true
			return id
		}
	}
}

// ensurePresetState 只初始化缺失槽位，绝不每次登录重建已有ID。
func ensurePresetState(p *Progress, now time.Time) {
	if p.PresetCardsRecord == nil {
		p.PresetCardsRecord = map[string]PresetRecord{}
	}
	if p.ActivityPresetIDs == nil {
		p.ActivityPresetIDs = map[string]string{}
	}
	used := map[string]bool{}
	ids := make([]string, 0, presetRecordNum)
	for _, id := range p.PresetIDs {
		if validObjectID(id) && !used[strings.ToLower(id)] {
			id = strings.ToLower(id)
			used[id] = true
			ids = append(ids, id)
		}
		if len(ids) == presetRecordNum {
			break
		}
	}
	for id := range p.PresetCardsRecord {
		if validObjectID(id) {
			used[strings.ToLower(id)] = true
		}
	}
	for len(ids) < presetRecordNum {
		ids = append(ids, newPresetID(now, used))
	}
	p.PresetIDs = ids
	cleanSync := make([]string, 0, len(p.SyncPvpPresetIDs))
	for _, id := range p.SyncPvpPresetIDs {
		if validObjectID(id) {
			id = strings.ToLower(id)
			cleanSync = append(cleanSync, id)
			used[id] = true
		}
	}
	p.SyncPvpPresetIDs = cleanSync
	for name, id := range p.ActivityPresetIDs {
		if !validObjectID(id) {
			delete(p.ActivityPresetIDs, name)
		}
	}
}

func presetProperties(p Progress) map[string]any {
	copy := CloneProgress(p)
	ensurePresetState(&copy, time.Now())
	ids := make([]any, len(copy.PresetIDs))
	for i, id := range copy.PresetIDs {
		ids[i] = ObjectID(id)
	}
	syncIDs := make([]any, len(copy.SyncPvpPresetIDs))
	for i, id := range copy.SyncPvpPresetIDs {
		syncIDs[i] = ObjectID(id)
	}
	activity := map[string]any{}
	for name, id := range copy.ActivityPresetIDs {
		activity[name] = ObjectID(id)
	}
	records := map[ObjectID]any{}
	for id, record := range copy.PresetCardsRecord {
		records[ObjectID(id)] = map[string]any{
			"preset_id": ObjectID(id), "preset_name": record.Name,
			"cards": map[string]any{
				"fighting_cards":  battleSlotWire(record.Cards.Fighting),
				"support_cards":   battleSlotWire(record.Cards.Support),
				"storyline_cards": battleSlotWire(record.Cards.Storyline),
			},
		}
	}
	return map[string]any{
		"preset_ids": ids, "preset_cards_record": records,
		"sync_pvp_preset_ids": syncIDs, "activity_preset_ids": activity,
	}
}

func parsePresetID(raw json.RawMessage) (string, error) {
	var value string
	if json.Unmarshal(raw, &value) != nil || !validObjectID(value) {
		return "", errors.New("预设ObjectID无效")
	}
	return strings.ToLower(value), nil
}

func parsePresetCards(raw json.RawMessage, allowEmpty bool) (BattleLayout, error) {
	return parsePresetCardsMode(raw, allowEmpty, false)
}

func parsePresetCardsMode(raw json.RawMessage, allowEmpty, activity bool) (BattleLayout, error) {
	if !allowEmpty {
		if !activity {
			return parseBattleLayout(raw)
		}
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return BattleLayout{}, errors.New("预设卡组必须为字典")
	}
	parse := func(name string, limit int) ([]string, error) {
		value, ok := object[name]
		if !ok || string(value) == "null" {
			return []string{}, nil
		}
		var slots []any
		if json.Unmarshal(value, &slots) != nil || len(slots) > limit {
			return nil, errors.New("预设卡组数量或类型无效")
		}
		out := make([]string, len(slots))
		for i, item := range slots {
			if item == nil {
				continue
			}
			if number, ok := item.(float64); ok && number == 0 {
				continue
			}
			id, ok := item.(string)
			if !ok || !validObjectID(id) {
				return nil, errors.New("预设卡组元素必须为ObjectID或空槽")
			}
			out[i] = strings.ToLower(id)
		}
		return out, nil
	}
	fightingLimit := androidFightingSlots
	if activity {
		fightingLimit = 6
	}
	fighting, err := parse("fighting_cards", fightingLimit)
	if err != nil {
		return BattleLayout{}, err
	}
	support, err := parse("support_cards", androidSupportSlots)
	if err != nil {
		return BattleLayout{}, err
	}
	story, err := parse("storyline_cards", 1024)
	if err != nil {
		return BattleLayout{}, err
	}
	return BattleLayout{Fighting: fighting, Support: support, Storyline: story}, nil
}

func validatePresetCards(p Progress, cards BattleLayout, allowEmpty bool) error {
	seenCardIDs := map[int]bool{}
	seenAny := false
	if !allowEmpty && len(cards.Fighting) == 0 {
		return errors.New("预设卡组不能全空")
	}
	if !allowEmpty && len(cards.Support) > 0 && p.UnlockSystems["support"] == 0 {
		return errors.New("援护系统尚未解锁")
	}
	for _, group := range [][]string{cards.Fighting, cards.Support} {
		for _, uuid := range group {
			if uuid == "" {
				continue
			}
			seenAny = true
			_, card := findCard(&p, uuid)
			if card == nil {
				return errors.New("预设卡牌不存在")
			}
			if seenCardIDs[card.CardID] {
				return errors.New("预设不能重复使用同名卡牌")
			}
			seenCardIDs[card.CardID] = true
			rule, ok := androidCardAppearances[card.CardID]
			if !ok || containsInt(rule.Forbid, androidBattleForbidden) {
				return errors.New("预设卡牌禁止战斗")
			}
		}
	}
	if !allowEmpty && !seenAny {
		return errors.New("预设卡组不能全空")
	}
	return nil
}

// 普通预设可保存已选好友助战引用；实际保存/开战由双角色事务再次授权。
func validateNormalPresetCards(p Progress, cards BattleLayout) error {
	copy := CloneProgress(p)
	for _, uuid := range cards.team() {
		if _, owned := findCard(&copy, uuid); owned == nil {
			if card := selectedFriendCard(&p, uuid); card != nil {
				copy.Cards = append(copy.Cards, *card)
			}
		}
	}
	return validatePresetCards(copy, cards, false)
}

func validateActivityPresetCards(p Progress, cards BattleLayout) error {
	if len(cards.Fighting) == 0 {
		return errors.New("活动预设缺少出战卡")
	}
	seen := map[int]bool{}
	for index, uuid := range cards.Fighting {
		if uuid == "" {
			return errors.New("活动预设出战槽不能留空")
		}
		_, card := findCard(&p, uuid)
		if card == nil {
			return errors.New("活动预设卡牌不存在")
		}
		rule, ok := androidFormationPresetCatalog.Cards[strconv.Itoa(card.CardID)]
		if !ok || rule.Disable != 0 || containsInt(rule.Forbid, androidBattleForbidden) {
			return errors.New("活动预设卡牌禁止战斗")
		}
		if seen[card.CardID] {
			return errors.New("活动预设不能重复使用同名卡牌")
		}
		seen[card.CardID] = true
		if duty, ok := androidFormationPresetCatalog.MiniDuty[strconv.Itoa(index+1)]; ok && duty.LimitPosition != nil && rule.Position != nil && *duty.LimitPosition != *rule.Position {
			return errors.New("活动预设卡牌职责位置不匹配")
		}
	}
	return nil
}

func syncPresetLimit(score int) int {
	for _, rule := range androidFormationPresetCatalog.SyncPvpScoreRule {
		if score >= rule.MinScore && score <= rule.MaxScore && rule.LineupNum > 0 {
			return rule.LineupNum
		}
	}
	return 1
}

func presetCallback(args []json.RawMessage) (int, error) {
	if len(args) == 0 {
		return 0, errors.New("预设缺少callback")
	}
	var callback int
	if json.Unmarshal(args[0], &callback) != nil || callback < 0 {
		return 0, errors.New("预设callback无效")
	}
	return callback, nil
}

func (s *Service) formationPresetRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("预设操作需要玩家状态")
	}
	callback, err := presetCallback(args)
	if err != nil {
		return nil, err
	}
	update := func(fn func(*Progress) error) error { return s.updateProgress(ctx, c, fn) }
	if method == "cover_preset_record" {
		if len(args) != 3 {
			return nil, errors.New("覆盖预设参数无效")
		}
		layout, e := parsePresetCards(args[2], false)
		if e != nil {
			return nil, e
		}
		update = func(fn func(*Progress) error) error { return s.updateBattleFormation(ctx, c, layout, -1, false, fn) }
	}
	if err := update(func(p *Progress) error {
		ensurePresetState(p, s.Now())
		if len(args) < 2 {
			return errors.New("预设参数不足")
		}
		switch method {
		case "cover_preset_record":
			if len(args) != 3 {
				return errors.New("覆盖预设参数无效")
			}
			id, e := parsePresetID(args[1])
			if e != nil {
				return e
			}
			cards, e := parsePresetCards(args[2], false)
			if e != nil {
				return e
			}
			if err := validateNormalPresetCards(*p, cards); err != nil {
				return err
			}
			if !containsString(p.PresetIDs, id) {
				return errors.New("预设槽位不存在")
			}
			previous := p.PresetCardsRecord[id]
			p.PresetCardsRecord[id] = PresetRecord{PresetID: id, Name: previous.Name, Cards: cloneBattleLayout(cards)}
		case "del_preset_record":
			if len(args) != 2 {
				return errors.New("删除预设参数无效")
			}
			id, e := parsePresetID(args[1])
			if e != nil || !containsString(p.PresetIDs, id) {
				return errors.New("预设槽位不存在")
			}
			delete(p.PresetCardsRecord, id)
		case "update_preset_name":
			if len(args) != 3 {
				return errors.New("预设名称参数无效")
			}
			id, e := parsePresetID(args[1])
			if e != nil || !containsString(p.PresetIDs, id) {
				return errors.New("预设槽位不存在")
			}
			var name string
			if json.Unmarshal(args[2], &name) != nil || len([]rune(name)) > presetRecordNameLen {
				return errors.New("预设名称过长或无效")
			}
			record := p.PresetCardsRecord[id]
			record.PresetID, record.Name = id, name
			p.PresetCardsRecord[id] = record
		case "update_preset_index":
			if len(args) != 4 {
				return errors.New("预设排序参数无效")
			}
			src, e1 := parsePresetID(args[1])
			dest, e2 := parsePresetID(args[2])
			var after bool
			if e1 != nil || e2 != nil || json.Unmarshal(args[3], &after) != nil || !containsString(p.PresetIDs, src) || !containsString(p.PresetIDs, dest) {
				return errors.New("预设排序参数无效")
			}
			if src != dest {
				p.PresetIDs = removeString(p.PresetIDs, src)
				index := indexString(p.PresetIDs, dest)
				if after {
					index++
				}
				if index > len(p.PresetIDs) {
					index = len(p.PresetIDs)
				}
				p.PresetIDs = append(p.PresetIDs, "")
				copy(p.PresetIDs[index+1:], p.PresetIDs[index:])
				p.PresetIDs[index] = src
			}
		case "set_sync_pvp_preset_record":
			if len(args) != 3 {
				return errors.New("同步PVP预设参数无效")
			}
			var index int
			if json.Unmarshal(args[1], &index) != nil || index < 0 || index >= syncPresetLimit(p.SyncPvpScore) {
				return errors.New("同步PVP预设槽位无效")
			}
			cards, e := parsePresetCards(args[2], true)
			if e != nil {
				return e
			}
			if err := validatePresetCards(*p, cards, true); err != nil {
				return err
			}
			for len(p.SyncPvpPresetIDs) <= index {
				used := map[string]bool{}
				for _, id := range p.PresetIDs {
					used[id] = true
				}
				p.SyncPvpPresetIDs = append(p.SyncPvpPresetIDs, newPresetID(s.Now(), used))
			}
			id := p.SyncPvpPresetIDs[index]
			record := p.PresetCardsRecord[id]
			record.PresetID, record.Cards = id, cloneBattleLayout(cards)
			p.PresetCardsRecord[id] = record
		case "activity_set_preset_record":
			if len(args) != 3 {
				return errors.New("活动预设参数无效")
			}
			var name string
			if json.Unmarshal(args[1], &name) != nil || name == "" {
				return errors.New("活动预设名称无效")
			}
			cards, e := parsePresetCardsMode(args[2], false, true)
			if e != nil {
				return e
			}
			if err := validateActivityPresetCards(*p, cards); err != nil {
				return err
			}
			id := p.ActivityPresetIDs[name]
			if !validObjectID(id) {
				used := map[string]bool{}
				for _, value := range p.PresetIDs {
					used[value] = true
				}
				id = newPresetID(s.Now(), used)
				p.ActivityPresetIDs[name] = id
			}
			p.PresetCardsRecord[id] = PresetRecord{PresetID: id, Name: name, Cards: cloneBattleLayout(cards)}
		default:
			return fmt.Errorf("未知预设方法：%s", method)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	properties := presetProperties(c.SelectedAvatarUnsafe().Progress)
	pushes := make([]Push, 0, len(properties)+1)
	for key, value := range properties {
		pushes = append(pushes, push("Avatar", "client_prop_changed", []any{key, value}))
	}
	pushes = append(pushes, Callback(callback, []any{RetSuccess}))
	return pushes, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func indexString(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return len(values)
}

func removeString(values []string, target string) []string {
	index := indexString(values, target)
	if index == len(values) {
		return values
	}
	return append(values[:index], values[index+1:]...)
}
