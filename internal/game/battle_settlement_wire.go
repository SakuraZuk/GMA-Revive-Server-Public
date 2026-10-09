package game

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"hs-server/internal/mobileproto"
)

// 只给结算盒使用精确数字读取，其余战斗快照沿用既有解码契约。
// JSONB将整数键/ObjectID文本化；此处保存原始数量，发送时按本版box/card/rune类型恢复。
func (b *BattleSession) UnmarshalJSON(raw []byte) error {
	type plainBattle BattleSession
	var base plainBattle
	if err := json.Unmarshal(raw, &base); err != nil {
		return err
	}
	var fields struct {
		Box json.RawMessage `json:"settlement_box"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if len(fields.Box) > 0 && string(fields.Box) != "null" {
		decoder := json.NewDecoder(bytes.NewReader(fields.Box))
		decoder.UseNumber()
		if err := decoder.Decode(&base.SettlementBox); err != nil {
			return err
		}
	}
	*b = BattleSession(base)
	return nil
}

// 还原的是收据内原资产快照，绝不读取当前卡/契印重新生成，也不抽奖或发奖。
// 主证据：752AAF28 box、2948C748 card、69E1FFCA rune的字段声明。
func battleSettlementBoxWire(box map[string]any) (map[string]any, error) {
	if box == nil {
		return nil, nil
	}
	raw, err := json.Marshal(box)
	if err != nil {
		return nil, fmt.Errorf("结算奖励盒不能读取：%w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var saved map[string]any
	if err = decoder.Decode(&saved); err != nil {
		return nil, err
	}
	if kind, ok := saved["__custom_type"].(string); !ok || kind != "box.box" {
		return nil, errors.New("结算奖励盒类型无效")
	}
	out, err := battleReceiptValue(saved, "", 0)
	if err != nil {
		return nil, err
	}
	return out.(map[string]any), nil
}

func battleReceiptObjectID(raw any) (ObjectID, error) {
	text, ok := raw.(string)
	if !ok || !validObjectID(text) {
		return "", errors.New("结算奖励盒ObjectID无效")
	}
	return ObjectID(strings.ToLower(text)), nil
}

func battleReceiptValue(value any, field string, depth int) (any, error) {
	if depth > 32 {
		return nil, errors.New("结算奖励盒嵌套过深")
	}
	if value == nil {
		return nil, nil
	}
	if field == "uuid" || field == "card_uuid" {
		return battleReceiptObjectID(value)
	}
	if field == "create_time" {
		number, ok := value.(json.Number)
		if !ok {
			return nil, errors.New("结算契印创建时间无效")
		}
		stamp, err := number.Float64()
		if err != nil || math.IsNaN(stamp) || math.IsInf(stamp, 0) || stamp < 0 {
			return nil, errors.New("结算契印创建时间无效")
		}
		return stamp, nil
	}
	switch v := value.(type) {
	case json.Number:
		if integer, err := v.Int64(); err == nil {
			return integer, nil
		}
		decimal, err := v.Float64()
		if err != nil || math.IsNaN(decimal) || math.IsInf(decimal, 0) {
			return nil, errors.New("结算奖励盒数字无效")
		}
		return decimal, nil
	case map[string]any:
		if field == "materials" || field == "grow_materials" || field == "extra_attrs_factor" {
			out := mobileproto.Map{}
			for _, key := range sortedReceiptKeys(v) {
				id, err := strconv.Atoi(key)
				if err != nil || id <= 0 || strconv.Itoa(id) != key {
					return nil, errors.New("结算奖励盒整数键无效")
				}
				number, ok := v[key].(json.Number)
				if !ok {
					return nil, errors.New("结算奖励盒整数数量无效")
				}
				amount, err := number.Int64()
				if err != nil || amount < 0 {
					return nil, errors.New("结算奖励盒整数数量无效")
				}
				out = append(out, mobileproto.Pair{Key: id, Value: amount})
			}
			return out, nil
		}
		if field == "runes" || field == "embed_runes" || field == "skill_mgr" || field == "talent_tree" {
			out := mobileproto.Map{}
			for _, key := range sortedReceiptKeys(v) {
				var nativeKey any
				if field == "runes" {
					id, err := battleReceiptObjectID(key)
					if err != nil {
						return nil, err
					}
					nativeKey = id
				} else {
					id, err := strconv.Atoi(key)
					if err != nil || id < 0 || strconv.Itoa(id) != key {
						return nil, errors.New("结算卡实例整数键无效")
					}
					nativeKey = id
				}
				nativeValue, err := battleReceiptValue(v[key], "", depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, mobileproto.Pair{Key: nativeKey, Value: nativeValue})
			}
			return out, nil
		}
		out := make(map[string]any, len(v))
		for key, item := range v {
			converted, err := battleReceiptValue(item, key, depth+1)
			if err != nil {
				return nil, err
			}
			out[key] = converted
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for index, item := range v {
			var converted any
			var err error
			if field == "card_exp_receiver" {
				converted, err = battleReceiptObjectID(item)
			} else {
				converted, err = battleReceiptValue(item, "", depth+1)
			}
			if err != nil {
				return nil, err
			}
			out[index] = converted
		}
		return out, nil
	default:
		return value, nil
	}
}

func sortedReceiptKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
