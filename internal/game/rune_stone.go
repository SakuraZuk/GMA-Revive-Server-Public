package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"
)

//go:embed rune_stone_catalog.json
var runeStoneCatalogRaw []byte

type runeStoneParams struct {
	OpenLimit     int     `json:"step_0_limit_count"`
	OpenMaterials [][]int `json:"step_0_materials"`
	PolishLimit   int     `json:"step_1_limit_count"`
	ValidHours    int64   `json:"valid_hour"`
	DailyLimit    int     `json:"daily_limit"`
}

var runeStoneCatalog = func() struct {
	Params map[int]runeStoneParams `json:"rune_stone_params"`
	Prices struct {
		Entries []struct {
			Key   []int `json:"键"`
			Value struct {
				Materials []int `json:"step_2_materials"`
			} `json:"值"`
		} `json:"键值条目"`
	} `json:"rune_stone_prices"`
} {
	var out struct {
		Params map[int]runeStoneParams `json:"rune_stone_params"`
		Prices struct {
			Entries []struct {
				Key   []int `json:"键"`
				Value struct {
					Materials []int `json:"step_2_materials"`
				} `json:"值"`
			} `json:"键值条目"`
		} `json:"rune_stone_prices"`
	}
	if json.Unmarshal(runeStoneCatalogRaw, &out) != nil || len(out.Prices.Entries) != 20 || out.Params[1].DailyLimit != 1 {
		panic("契印原石原表目录无效")
	}
	return out
}()

// 隐藏契印只存服务端；客户端按状态逐阶段看到原生规定的字段。
type RuneStoneState struct {
	MaterialID int             `json:"material_id"`
	Time       int64           `json:"time"`
	Count      int             `json:"count"`
	OpenCount  int             `json:"step_0_count"`
	States     map[string]int  `json:"rune_states"`
	Runes      map[string]Rune `json:"runes"`
	Day        string          `json:"server_day"`
}

func runeStoneValid(stone *RuneStoneState, now time.Time) bool {
	if stone == nil || stone.MaterialID <= 0 {
		return false
	}
	mat, ok := androidShop.Materials[stone.MaterialID]
	_, bonus := androidShop.Bonuses[mat.Target]
	return ok && mat.Type == 15 && bonus && now.Unix() <= stone.Time+runeStoneCatalog.Params[1].ValidHours*3600
}
func grantRuneStone(p *Progress, id int, now time.Time) (bool, error) {
	mat, ok := androidShop.Materials[id]
	if !ok || mat.Type != 15 || mat.Target <= 0 {
		return false, errors.New("契印原石材料目录无效")
	}
	if p.RuneStone == nil {
		p.RuneStone = &RuneStoneState{}
	}
	stone := p.RuneStone
	day := socialDay(now)
	if stone.Day > day {
		// 冻结的旧战斗可能在新日状态写入后兑现；不回拨每日次数，也不阻塞其他基础奖励。
		return false, nil
	}
	if stone.Day != day {
		stone.Day = day
		stone.Count = 0
	}
	if stone.Count < 0 {
		return false, errors.New("契印原石次数无效")
	}
	// 原版check_drop：当日次数未满且没有有效原石才能设置下一颗。
	if stone.Count >= runeStoneCatalog.Params[1].DailyLimit || runeStoneValid(stone, now) {
		return false, nil
	}
	stone.MaterialID = id
	stone.Time = now.Unix()
	stone.Count++
	stone.OpenCount = 0
	stone.States = map[string]int{}
	stone.Runes = map[string]Rune{}
	return true, nil
}
func runeStoneInfo(stone *RuneStoneState, ids []string) map[string]any {
	out := map[string]any{}
	if stone == nil {
		return out
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	for id, r := range stone.Runes {
		if ids != nil && !selected[id] {
			continue
		}
		state := stone.States[id]
		if state <= 0 {
			continue
		}
		info := map[string]any{"star": r.Star, "suit": r.Suit}
		if state > 1 {
			info["pos"] = r.Position
			info["extra_suit"] = r.ExtraSuit
		}
		if state > 2 {
			info = runeProperties(r)
		}
		out[id] = info
	}
	return out
}
func runeStoneProperties(stone *RuneStoneState) map[string]any {
	if stone == nil {
		stone = &RuneStoneState{}
	}
	states := stone.States
	if states == nil {
		states = map[string]int{}
	}
	return map[string]any{"__custom_type": "rune_stone.rune_stone", "material_id": stone.MaterialID, "time": stone.Time, "count": stone.Count, "step_0_count": stone.OpenCount, "rune_states": states, "client_runes": runeStoneInfo(stone, nil)}
}
func runeStonePush(c *Connection) Push {
	return push("Avatar", "client_prop_changed", []any{"current_rune_stone", runeStoneProperties(c.SelectedAvatarUnsafe().Progress.RuneStone)})
}

func openRuneStone(p *Progress, id int, now time.Time) (map[string]any, error) {
	stone := p.RuneStone
	if !runeStoneValid(stone, now) || stone.MaterialID != id {
		return nil, runeStoneReject("契印原石不存在、已过期或材料不符")
	}
	params := runeStoneCatalog.Params[1]
	if stone.OpenCount < 0 || stone.OpenCount >= params.OpenLimit {
		return nil, runeStoneReject("契印原石重开次数已满")
	}
	var cost []int
	for _, row := range params.OpenMaterials {
		if len(row) == 2 && p.Materials[row[0]].Count >= int64(row[1]) {
			cost = row
			break
		}
	}
	if cost == nil {
		return nil, runeStoneReject("开启原石材料不足")
	}
	scratch := CloneProgress(*p)
	scratch.Runes = map[string]Rune{}
	scratch.battleRewardSession = nil
	scratch.battleRewardOverflow = nil
	box, err := grantNativeBonusRaw(&scratch, androidShop.Materials[id].Target, 1, p.AvatarLevel, now)
	if err != nil {
		return nil, err
	}
	if len(scratch.Runes) == 0 || len(box["materials"].(map[int]int64)) != 0 || len(scratch.Cards) != len(p.Cards) {
		return nil, errors.New("原石目标奖励不是纯契印目录")
	}
	if err = runeMaterials(p, [][]int{cost}, -1); err != nil {
		return nil, err
	}
	stone.Runes = scratch.Runes
	stone.States = map[string]int{}
	stone.OpenCount++
	for uuid := range stone.Runes {
		stone.States[uuid] = 1
	}
	return runeStoneInfo(stone, nil), nil
}
func advanceRuneStone(p *Progress, ids []string, identify bool, now time.Time) (map[string]any, error) {
	stone := p.RuneStone
	if !runeStoneValid(stone, now) || len(ids) == 0 {
		return nil, runeStoneReject("契印原石已过期或选择为空")
	}
	seen := map[string]bool{}
	want, next := 1, 2
	if identify {
		want, next = 2, 3
	}
	replayed := true
	for _, id := range ids {
		if !validObjectID(id) || seen[id] {
			return nil, runeStoneReject("契印原石选择重复或编号无效")
		}
		seen[id] = true
		if _, ok := stone.Runes[id]; !ok {
			return nil, runeStoneReject("契印原石选择不属于本次开启")
		}
		if stone.States[id] != next {
			replayed = false
		}
	}
	if replayed {
		if identify {
			for _, id := range ids {
				if _, ok := p.Runes[id]; !ok {
					return nil, runeStoneReject("已鉴定契印不在库存，不能重复发奖")
				}
			}
		}
		return runeStoneInfo(stone, ids), nil
	}
	for _, id := range ids {
		if stone.States[id] != want {
			return nil, runeStoneReject("契印原石阶段不符")
		}
	}
	if !identify {
		count := 0
		for _, state := range stone.States {
			if state > 1 {
				count++
			}
		}
		if count+len(ids) > runeStoneCatalog.Params[1].PolishLimit {
			return nil, runeStoneReject("原石最多打磨原表规定的契印数量")
		}
	} else {
		limits, err := loadRuneTables()
		if err != nil {
			return nil, err
		}
		if len(ids) > limits.Tables.Limits.Max-len(p.Runes) {
			return nil, runeReject("RET_RUNE_MAX_COUNT_EXCEED", "契印库存容量不足")
		}
		costs := map[int]int{}
		for _, id := range ids {
			if _, exists := p.Runes[id]; exists {
				return nil, errors.New("契印原石鉴定编号冲突")
			}
			r := stone.Runes[id]
			var price []int
			for _, entry := range runeStoneCatalog.Prices.Entries {
				if len(entry.Key) == 2 && entry.Key[0] == r.Star && entry.Key[1] == r.Position {
					price = entry.Value.Materials
					break
				}
			}
			if len(price) != 2 || price[1] <= 0 || costs[price[0]] > math.MaxInt32-price[1] {
				return nil, errors.New("契印原石鉴定价格无效")
			}
			costs[price[0]] += price[1]
		}
		keys := []int{}
		for mid := range costs {
			keys = append(keys, mid)
		}
		sort.Ints(keys)
		rows := [][]int{}
		for _, mid := range keys {
			rows = append(rows, []int{mid, costs[mid]})
		}
		if err = runeMaterials(p, rows, -1); err != nil {
			return nil, err
		}
		if p.Runes == nil {
			p.Runes = map[string]Rune{}
		}
		for _, id := range ids {
			p.Runes[id] = stone.Runes[id]
		}
	}
	for _, id := range ids {
		stone.States[id] = next
	}
	return runeStoneInfo(stone, ids), nil
}
func (s *Service) runeStoneRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	callback, ok := callbackArg(args)
	if c.phase != Playing || !ok || len(args) != 2 {
		return nil, errors.New("契印原石需要回调和一个参数")
	}
	var info map[string]any
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		var err error
		if method == "step_0_rune_stone" {
			var mid int
			if json.Unmarshal(args[1], &mid) != nil {
				return runeStoneReject("契印原石材料编号无效")
			}
			info, err = openRuneStone(p, mid, s.Now())
		} else {
			var ids []string
			if json.Unmarshal(args[1], &ids) != nil {
				return runeStoneReject("契印原石选择列表无效")
			}
			info, err = advanceRuneStone(p, ids, method == "step_2_rune_stone", s.Now())
		}
		return err
	})
	if err != nil {
		return []Push{Callback(callback, []any{false, "契印原石操作失败，请检查期限、阶段、材料或库存容量", map[string]any{}})}, errRuneStoneCallback(err)
	}
	// RPC回调不经过CustomDict.load，必须显式编码ObjectId键；字符串键不能命中原生rune_mgr。
	wireInfo := map[ObjectID]any{}
	for uuid, value := range info {
		wireInfo[ObjectID(uuid)] = value
	}
	return []Push{materialManagerPush(c), runeStonePush(c), runePush(c), Callback(callback, []any{true, "", wireInfo})}, nil
}

// 业务拒绝有原生完整失败回调；存储故障仍向日志上报，不能伪造成功。
type runeStoneBusinessError struct{ message string }

func (e *runeStoneBusinessError) Error() string { return e.message }
func runeStoneReject(message string) error      { return &runeStoneBusinessError{message} }
func errRuneStoneCallback(err error) error {
	var stoneErr *runeStoneBusinessError
	var runeErr *runeBusinessError
	if errors.As(err, &stoneErr) || errors.As(err, &runeErr) {
		return nil
	}
	return err
}
