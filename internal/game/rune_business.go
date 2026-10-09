package game

import (
	"context"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
)

// 契印规则只从 Android 导出的 rune_tables.json 读取，禁止在代码中硬编码成本。
//
//go:embed rune_tables.json
var runeTablesFS embed.FS

type runeTables struct {
	Tables struct {
		Errors map[string]int `json:"errors"`
		Bounds struct {
			Min int `json:"min"`
			Max int `json:"max"`
		} `json:"runes_level"`
		Templates []runeTemplate `json:"runes"`
		Suits     []struct {
			ID            int   `json:"_id"`
			AutoLockStars []int `json:"auto_lock_stars"`
		} `json:"runes_suits"`
		Levels []struct {
			Star      int       `json:"star"`
			Level     int       `json:"level"`
			Upgrade   [][]int   `json:"upgrade_material"`
			Decompose [][]int   `json:"decompose_material"`
			Unlock    [][][]int `json:"unlock_condition"`
			AutoLock  int       `json:"auto_lock"`
		} `json:"runes_levels"`
		Resets []struct {
			Star     int   `json:"star"`
			Material int   `json:"material_id"`
			Count    int64 `json:"item_num"`
		} `json:"runes_reset"`
		Attrs []struct {
			ID           int                 `json:"_id"`
			Name         string              `json:"attr_name"`
			Weight       int                 `json:"attr_weight"`
			Bind         *int                `json:"bind_extra_attr"`
			Mutex        *int                `json:"mutex_attr"`
			FactorRanges [][]json.RawMessage `json:"level_up_factor_range"`
		} `json:"runes_attrs"`
		UnlockAttrs []struct {
			Level  int `json:"level"`
			Unlock int `json:"unlock"`
		} `json:"runes_unlock_attrs"`
		Limits struct {
			Max int `json:"MAX_RUNE_COUNT"`
		} `json:"limits"`
	} `json:"tables"`
}

func loadRuneTables() (runeTables, error) {
	var out runeTables
	b, err := runeTablesFS.ReadFile("rune_tables.json")
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(b, &out)
	return out, err
}

func validObjectID(s string) bool {
	if len(s) != 24 {
		return false
	}
	b, err := hex.DecodeString(s)
	return err == nil && string(b) != string(make([]byte, 12))
}

func runeMaterials(p *Progress, changes [][]int, sign int) error {
	for _, row := range changes {
		if len(row) != 2 || row[0] <= 0 || row[1] < 0 || (sign != 1 && sign != -1) {
			return errors.New("契印材料表无效")
		}
		m := p.Materials[row[0]]
		amount := int64(row[1]) * int64(sign)
		if amount < 0 && m.Count < -amount {
			return runeReject("RET_MATERIAL_NOT_ENOUGH", "契印材料不足")
		}
		if amount > 0 && (m.Count > math.MaxInt64-amount || m.Total > math.MaxInt64-amount) {
			return errors.New("契印材料超过容量")
		}
		m.ID, m.Count = row[0], m.Count+amount
		// total 为累计获得，消耗不能倒扣累计值。
		if amount > 0 {
			m.Total += amount
		}
		p.Materials[row[0]] = m
	}
	return nil
}

func runePush(c *Connection) Push {
	return push("Avatar", "client_prop_changed", []any{"rune_mgr", runeMgrProperties(c.SelectedAvatarUnsafe().Progress.Runes)})
}

// 原生 custom_types/rune.py 的字段与类型；持久存储的布尔值转换为 Int。
func runeProperties(r Rune) map[string]any {
	lock := 0
	if r.Locked {
		lock = 1
	}
	list := func(ids []int) []int {
		if ids == nil {
			return []int{}
		}
		return ids
	}
	factors := r.ExtraAttrsFactor
	if factors == nil {
		factors = map[int]int{}
	}
	var owner any
	if r.CardUUID != "" {
		owner = r.CardUUID
	}
	return map[string]any{"uuid": r.UUID, "star": r.Star, "pos": r.Position, "suit": r.Suit, "extra_suit": r.ExtraSuit, "level": r.Level,
		"base_attrs": list(r.BaseAttrs), "extra_attrs_count": r.ExtraAttrsCount, "extra_attrs": list(r.ExtraAttrs), "extra_attrs_lib": list(r.ExtraAttrsLib),
		"extra_attrs_factor": factors, "card_uuid": owner, "lock": lock, "create_time": r.CreateTime}
}

func runeMgrProperties(runes map[string]Rune) map[string]any {
	manager := map[string]any{}
	for id, r := range runes {
		manager[id] = runeProperties(r)
	}
	return manager
}

func (s *Service) runeRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) < 2 {
		return nil, errors.New("契印操作参数无效")
	}
	var callback int
	if err := json.Unmarshal(args[0], &callback); err != nil {
		return nil, errors.New("契印 callback 无效")
	}
	var runeID string
	if method != "decompose_runes" {
		runeIndex := 1
		if method == "embed_rune" {
			runeIndex = 2
		}
		if len(args) <= runeIndex || json.Unmarshal(args[runeIndex], &runeID) != nil || !validObjectID(runeID) {
			return nil, errors.New("契印 UUID 无效")
		}
		runeID = strings.ToLower(runeID)
	}
	var cardID string
	if method == "embed_rune" {
		if len(args) != 3 || json.Unmarshal(args[1], &cardID) != nil || !validObjectID(cardID) {
			return nil, errors.New("契印卡牌 UUID 无效")
		}
		cardID = strings.ToLower(cardID)
	}
	var count, index, attrID int
	if method == "up_level_rune" && (len(args) != 3 || json.Unmarshal(args[2], &count) != nil || count < 1) {
		return nil, errors.New("契印升级次数无效")
	}
	if method == "change_rune_extra_attr" && (len(args) != 4 || json.Unmarshal(args[2], &index) != nil || json.Unmarshal(args[3], &attrID) != nil) {
		return nil, errors.New("契印词条参数无效")
	}
	if (method == "unembed_rune" || method == "lock_rune" || method == "unlock_rune" || method == "decompose_runes") && len(args) != 2 {
		return nil, errors.New("契印操作参数数量无效")
	}
	gains := map[int]int64{}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if p.Runes == nil {
			p.Runes = map[string]Rune{}
		}
		if p.RuneTemplates == nil {
			p.RuneTemplates = map[string]RuneTemplate{}
		}
		tables, err := loadRuneTables()
		if err != nil {
			return err
		}
		if method == "decompose_runes" {
			var ids []string
			if err := json.Unmarshal(args[1], &ids); err != nil || len(ids) == 0 {
				return runeReject("RET_DECOMPOSE_RUNE_EMPTY", "分解契印列表无效")
			}
			seen := map[string]bool{}
			for _, id := range ids {
				if !validObjectID(id) {
					return runeReject("RET_RUNE_NOT_EXIST", "分解契印 UUID 无效")
				}
				id = strings.ToLower(id)
				if seen[id] {
					continue
				}
				seen[id] = true
				row, exists := p.Runes[id]
				if !exists {
					return runeReject("RET_RUNE_NOT_EXIST", "契印不存在")
				}
				if row.Locked {
					return runeReject("RET_RUNE_LOCKED", "契印被锁定")
				}
				if row.CardUUID != "" {
					return runeReject("RET_RUNE_EMBED_OTHER_CARD", "契印已镶嵌")
				}
				var gain [][]int
				for _, x := range tables.Tables.Levels {
					if x.Star == row.Star && x.Level == row.Level {
						gain = x.Decompose
						break
					}
				}
				if gain == nil {
					return errors.New("契印分解表缺失")
				}
				if err := runeMaterials(p, gain, 1); err != nil {
					return err
				}
				delete(p.Runes, id)
				// Android 删除库存契印后，预设中的同一 UUID 也必须失效；
				// 清理槽位可避免登录下发悬空 ObjectID，后续应用预设再被拒绝。
				for templateID, template := range p.RuneTemplates {
					for pos, templateRuneID := range template.Runes {
						if templateRuneID == id {
							delete(template.Runes, pos)
						}
					}
					p.RuneTemplates[templateID] = template
				}
				for _, x := range gain {
					gains[x[0]] += int64(x[1])
				}
			}
			return nil
		}
		r, ok := p.Runes[runeID]
		if !ok {
			return runeReject("RET_RUNE_NOT_EXIST", "契印不存在")
		}
		switch method {
		case "embed_rune":
			if err := validateRuneForEquip(r, tables); err != nil {
				return runeReject("RET_RUNE_ATTR_INVALID", err.Error())
			}
			if _, card := findCard(p, cardID); card == nil {
				return runeReject("RET_CARD_NOT_EXIST", "卡牌不存在")
			}
			validSlot := false
			for _, template := range tables.Tables.Templates {
				if template.Star == r.Star && template.Position == r.Position {
					validSlot = true
					break
				}
			}
			if !validSlot {
				return errors.New("契印星级或位置不在 Android 表中")
			}
			for id, other := range p.Runes {
				if id != runeID && other.CardUUID == cardID && other.Position == r.Position {
					other.CardUUID = ""
					p.Runes[id] = other
				}
			}
			r.CardUUID = cardID
			p.Runes[runeID] = r
		case "unembed_rune":
			if r.CardUUID == "" {
				return runeReject("RET_RUNE_NOT_EMBED", "契印未镶嵌")
			}
			r.CardUUID = ""
			p.Runes[runeID] = r
		case "lock_rune", "unlock_rune":
			r.Locked = method == "lock_rune"
			p.Runes[runeID] = r
		case "up_level_rune":
			if r.Level < tables.Tables.Bounds.Min || r.Level > tables.Tables.Bounds.Max || count > tables.Tables.Bounds.Max-r.Level {
				return runeReject("RET_RUNE_REACH_MAX_LEVEL", "契印升级超过等级上限")
			}
			for i := 0; i < count; i++ {
				var cost [][]int
				nextFound := false
				nextAutoLock := false
				for _, row := range tables.Tables.Levels {
					if row.Star == r.Star && row.Level == r.Level+1 {
						nextFound = true
						for _, group := range row.Unlock {
							for _, condition := range group {
								if len(condition) != 2 || condition[0] != 2 || c.SelectedAvatarUnsafe().Info.Level < condition[1] {
									return runeReject("RET_RUNE_REACH_MAX_LEVEL", "玩家等级尚未解锁契印升级")
								}
							}
						}
						nextAutoLock = row.AutoLock != 0
					}
					if row.Star == r.Star && row.Level == r.Level {
						cost = row.Upgrade
					}
				}
				if len(cost) == 0 || !nextFound {
					return errors.New("契印已达等级上限或升级表缺失")
				}
				if err := runeMaterials(p, cost, -1); err != nil {
					return err
				}
				r.Level++
				if err := fillRuneExtraAttrs(&r, tables); err != nil {
					return runeReject("RET_RUNE_ATTR_INVALID", err.Error())
				}
				if nextAutoLock {
					r.Locked = true
				}
			}
			p.Runes[runeID] = r
			advanceAchievementAmount(p, 33, method, 1, s.Now())
		case "change_rune_extra_attr":
			if index < 0 || index >= len(r.ExtraAttrs) {
				return runeReject("RET_RUNE_CHANGE_ATTR_INDEX_ERROR", "契印词条位置无效")
			}
			allowed := false
			for _, id := range r.ExtraAttrsLib {
				if id == attrID {
					allowed = true
				}
			}
			if !allowed {
				return runeReject("RET_RUNE_ATTR_ID_ERROR", "契印洗练属性不在词条库")
			}
			if r.ExtraAttrs[index] == attrID {
				return runeReject("RET_RUNE_CANNOT_RESET_IT", "契印洗练属性未发生变化")
			}
			attrFound := false
			for _, attr := range tables.Tables.Attrs {
				if attr.ID == attrID {
					attrFound = true
					break
				}
			}
			if !attrFound {
				return runeReject("RET_RUNE_ATTR_ID_ERROR", "契印洗练属性表缺失")
			}
			resetFound := false
			for _, reset := range tables.Tables.Resets {
				if reset.Star == r.Star {
					resetFound = true
					if err := runeMaterials(p, [][]int{{reset.Material, int(reset.Count)}}, -1); err != nil {
						return err
					}
					break
				}
			}
			if !resetFound {
				return runeReject("RET_RUNE_CANNOT_RESET_IT", "该星级契印不能洗练")
			}
			r.ExtraAttrs = append([]int(nil), r.ExtraAttrs...)
			r.ExtraAttrs[index] = attrID
			p.Runes[runeID] = r
		default:
			return fmt.Errorf("契印方法未实现：%s", method)
		}
		return nil
	})
	if err != nil {
		log.Printf("契印操作拒绝 method=%s 原因=%v", method, err)
		var rejected *runeBusinessError
		if !errors.As(err, &rejected) {
			return nil, err
		}
		if method == "decompose_runes" {
			return []Push{Callback(callback, []any{map[string]any{"__custom_type": "box.box", "materials": map[string]any{}}})}, nil
		}
		tables, tableErr := loadRuneTables()
		if tableErr != nil {
			return nil, tableErr
		}
		code, exists := tables.Tables.Errors[rejected.Name]
		if !exists {
			return nil, fmt.Errorf("Android 契印错误码缺失：%s", rejected.Name)
		}
		return []Push{Callback(callback, []any{code})}, nil
	}
	result := []Push{runePush(c), cardMgrPush(c), push("Avatar", "client_prop_changed", []any{"material_mgr", materialProperties(c.identity.Avatars, c.hostnum)})}
	if method == "decompose_runes" {
		materials := map[string]any{}
		for id, amount := range gains {
			materials[strconv.Itoa(id)] = amount
		}
		result = append(result, Callback(callback, []any{map[string]any{"__custom_type": "box.box", "materials": materials}}))
	} else {
		result = append(result, Callback(callback, []any{0}))
	}
	return result, nil
}
