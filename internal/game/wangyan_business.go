package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"
)

type WangyanMap struct {
	ID       int   `json:"map_id"`
	Occupied []int `json:"occupy_node_list"`
	Area     []int `json:"area_list"`
}
type WangyanState struct {
	Power     int                 `json:"combat_power"`
	OldPower  int                 `json:"old_combat_power"`
	Supply    Power               `json:"supply"`
	Maps      map[int]*WangyanMap `json:"map_infos"`
	Area      int                 `json:"area_num"`
	BossPower map[int]int         `json:"boss_site_combat_power"`
	Regain    int                 `json:"supply_regain_addition"`
	Mini      map[int]int         `json:"mini_game_info"`
	Rewarded  []int               `json:"rewarded_nodes"`
	Tasks     []int               `json:"claimed_tasks"`
}

func ensureWangyan(p *Progress, now time.Time) *WangyanState {
	if p.Activities.Wangyan == nil {
		r := activityData("wangyan_general_args", 1)
		p.Activities.Wangyan = &WangyanState{Power: r.integer("init_combat_power"), OldPower: r.integer("init_combat_power"), Supply: Power{Value: r.integer("init_supply"), Max: r.integer("supply_max_limit"), Interval: r.integer("supply_interval"), PerValue: r.integer("supply_per_value"), LastTime: float64(now.UnixNano()) / 1e9}, Maps: map[int]*WangyanMap{}, BossPower: map[int]int{}, Mini: map[int]int{}}
	}
	w := p.Activities.Wangyan
	if w.Maps == nil {
		w.Maps = map[int]*WangyanMap{}
	}
	if w.BossPower == nil {
		w.BossPower = map[int]int{}
	}
	if w.Mini == nil {
		w.Mini = map[int]int{}
	}
	for _, id := range w.Tasks {
		if _, ok := w.Mini[id]; !ok {
			w.Mini[id] = 1
		}
	}
	return w
}
func refreshWangyanSupply(w *WangyanState, now time.Time) error {
	t := float64(now.UnixNano()) / 1e9
	if math.IsNaN(w.Supply.LastTime) || w.Supply.LastTime > t {
		return errors.New("妄言补给存档时间回拨")
	}
	if w.Supply.Interval <= 0 || w.Supply.PerValue <= 0 || w.Supply.Max <= 0 || w.Supply.Value < 0 {
		return errors.New("妄言补给配置无效")
	}
	if w.Supply.Value >= w.Supply.Max {
		w.Supply.LastTime = t
		return nil
	}
	steps := int64((t - w.Supply.LastTime) / float64(w.Supply.Interval))
	if steps > 0 {
		need := w.Supply.Max - w.Supply.Value
		v := int64(w.Supply.PerValue + w.Regain)
		if v <= 0 {
			return errors.New("妄言补给恢复数无效")
		}
		if steps > int64(need)/v {
			w.Supply.Value = w.Supply.Max
			w.Supply.LastTime = t
		} else {
			w.Supply.Value += int(steps * v)
			w.Supply.LastTime += float64(steps * int64(w.Supply.Interval))
		}
	}
	return nil
}

// Android 0FA7FEEC time_auto_attr.add 的默认 auto=False：显式获得可超过
// max_limit，只有自然回复 auto=True 才封顶。补给不是普通材料库存。
func grantWangyanSupply(p *Progress, amount int64, now time.Time) (int64, error) {
	if amount < 0 || amount > math.MaxInt32 {
		return 0, errors.New("妄言补给奖励数量无效")
	}
	w := ensureWangyan(p, now)
	if err := refreshWangyanSupply(w, now); err != nil {
		return 0, err
	}
	if amount > int64(math.MaxInt32-w.Supply.Value) {
		return 0, errors.New("妄言补给奖励溢出")
	}
	w.Supply.Value += int(amount)
	if w.Supply.Value >= w.Supply.Max {
		w.Supply.LastTime = float64(now.UnixNano()) / 1e9
	}
	return amount, nil
}
func wangyanProperties(w *WangyanState, now time.Time) map[string]any {
	copy := *w
	_ = refreshWangyanSupply(&copy, now)
	maps := map[int]any{}
	for id, m := range w.Maps {
		maps[id] = map[string]any{"map_id": m.ID, "occupy_node_list": m.Occupied}
	}
	return map[string]any{"combat_power": w.Power, "old_combat_power": w.OldPower, "supply": copy.Supply, "map_infos": maps, "area_num": w.Area, "boss_site_combat_power": w.BossPower, "supply_regain_addition": w.Regain, "mini_game_info": w.Mini}
}
func openWangyanMap(p *Progress, id, level int, now time.Time) (*WangyanMap, error) {
	r := activityData("wangyan_map", id)
	if len(r) == 0 || r.flag("_disable") {
		return nil, errors.New("妄言地图不存在")
	}
	w := ensureWangyan(p, now)
	if w.Power < r.integer("map_unlock_combat_power") {
		return nil, errors.New("妄言力未达到地图门槛")
	}
	if err := r.conditions("unlock_condition", *p, level); err != nil {
		return nil, err
	}
	if m := w.Maps[id]; m != nil {
		return m, nil
	}
	mr := activityData("wangyan_map_node", r.integer("map_node_id"))
	start := mr.integer("start_node")
	if start <= 0 {
		return nil, errors.New("妄言地图起点无效")
	}
	m := &WangyanMap{ID: id, Occupied: []int{start}, Area: []int{start}}
	w.Maps[id] = m
	w.Area++
	return m, nil
}
func prepareWangyanDungeon(p *Progress, bc *ActivityBattleContext, level int, now time.Time) error {
	r := activityData("wangyan_dungeons", bc.DungeonID)
	if len(r) == 0 || r.flag("_disable") {
		return errors.New("妄言副本配置不存在")
	}
	bc.MapID, bc.NodeID = r.integer("map_id"), r.integer("site_id")
	m, err := openWangyanMap(p, bc.MapID, level, now)
	if err != nil {
		return err
	}
	node := activityData("wangyan_node", bc.NodeID)
	if len(node) == 0 {
		return errors.New("妄言节点配置不存在")
	}
	entered := containsInt(m.Occupied, bc.NodeID)
	for _, n := range node.ids("neighbour_nodes") {
		entered = entered || containsInt(m.Occupied, n)
	}
	if !entered || (r.integer("activity_dungeon_type") != 1 && !containsInt(m.Occupied, bc.NodeID)) {
		return errors.New("妄言节点尚不可达")
	}
	w := ensureWangyan(p, now)
	if err := refreshWangyanSupply(w, now); err != nil {
		return err
	}
	cost := r.integer("supply_consume")
	if cost < 0 || w.Supply.Value < cost {
		return errors.New("妄言补给不足")
	}
	w.Supply.Value -= cost
	bc.Resource = cost
	return nil
}
func finishWangyanDungeon(p *Progress, bc *ActivityBattleContext, now time.Time) error {
	w := p.Activities.Wangyan
	if w == nil || w.Maps[bc.MapID] == nil {
		return errors.New("妄言结算地图丢失")
	}
	r := activityData("wangyan_dungeons", bc.DungeonID)
	m := w.Maps[bc.MapID]
	addition := r.integer("combat_power_addition")
	switch r.integer("activity_dungeon_type") {
	case 1:
		if containsInt(w.Rewarded, bc.NodeID) {
			addition = 0
		} else {
			w.Rewarded = append(w.Rewarded, bc.NodeID)
		}
		if !containsInt(m.Occupied, bc.NodeID) {
			m.Occupied = append(m.Occupied, bc.NodeID)
		}
		if !containsInt(m.Area, bc.NodeID) {
			m.Area = append(m.Area, bc.NodeID)
			w.Area++
		}
	case 2:
		limit := activityData("wangyan_node", bc.NodeID).integer("combat_power_addition_limit")
		remaining := limit - w.BossPower[bc.NodeID]
		if remaining < 0 {
			remaining = 0
		}
		if addition > remaining {
			addition = remaining
		}
		w.BossPower[bc.NodeID] += addition
	case 3:
	default:
		return errors.New("妄言副本玩法类型未取证")
	}
	if addition < 0 || w.Power > math.MaxInt32-addition {
		return errors.New("妄言力溢出")
	}
	w.OldPower = w.Power
	w.Power += addition
	_, err := grantWangyanSupply(p, int64(r.integer("supply_addition")), now)
	return err
}
func (s *Service) wangyanRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	direct := method == "submit_wangyan_consign_task"
	expected := map[string]int{"enter_wangyan_game": 1, "wangyan_game_enter_map": 2, "wangyan_game_enter_dungeon": 4, "submit_wangyan_consign_task": 1}[method]
	if len(args) != expected {
		return nil, errors.New("妄言接口参数数量无效")
	}
	var cb int
	offset := 0
	if !direct {
		if json.Unmarshal(args[0], &cb) != nil || cb <= 0 {
			return nil, errors.New("妄言回调编号无效")
		}
		offset = 1
	}
	var id, node int
	if len(args) > offset && json.Unmarshal(args[offset], &id) != nil {
		return nil, errors.New("妄言编号无效")
	}
	if method == "wangyan_game_enter_dungeon" {
		if json.Unmarshal(args[2], &node) != nil {
			return nil, errors.New("妄言节点编号无效")
		}
		var extra map[string]json.RawMessage
		if json.Unmarshal(args[3], &extra) != nil || extra == nil {
			return nil, errors.New("妄言额外参数无效")
		}
		nr := activityData("wangyan_node", node)
		dungeon := nr.integer("main_dungeon_id")
		if v := extra["dungeon_id"]; len(v) > 0 {
			if json.Unmarshal(v, &dungeon) != nil {
				return nil, errors.New("妄言副本编号无效")
			}
		}
		dr := activityData("wangyan_dungeons", dungeon)
		if dr.integer("map_id") != id || dr.integer("site_id") != node {
			return []Push{Callback(cb, []any{activityErrorCode(errors.New("节点不符"))})}, nil
		}
		converted := []json.RawMessage{args[0], json.RawMessage(strconv.Itoa(dungeon)), args[3]}
		return s.enterDungeon(ctx, c, converted)
	}
	var box map[string]any
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		level := c.SelectedAvatarUnsafe().Info.Level
		if _, err := activityOpen(*p, 207, level, s.Now()); err != nil {
			return err
		}
		w := ensureWangyan(p, s.Now())
		if err := refreshWangyanSupply(w, s.Now()); err != nil {
			return err
		}
		switch method {
		case "enter_wangyan_game":
			return nil
		case "wangyan_game_enter_map":
			_, err := openWangyanMap(p, id, level, s.Now())
			return err
		case "submit_wangyan_consign_task":
			r := activityData("wangyan_game", id)
			if len(r) == 0 || (r.integer("task_type") != 1 && r.integer("task_type") != 2) {
				return errors.New("妄言委托类型尚未取证")
			}
			if r.integer("task_type") == 2 {
				var e error
				box, e = submitWangyanSpecial(p, r, s.Now())
				return e
			}
			if w.Power < r.integer("map_unlock_combat_power") || containsInt(w.Tasks, id) {
				return errors.New("妄言委托未解锁或已领取")
			}
			need := r.ids("need_materials")
			if len(need) != 2 || need[0] <= 0 || need[1] <= 0 {
				return errors.New("妄言委托材料配置无效")
			}
			m := p.Materials[need[0]]
			if m.Count < int64(need[1]) {
				return errors.New("妄言委托材料不足")
			}
			m.Count -= int64(need[1])
			p.Materials[need[0]] = m
			var e error
			box, e = grantActivityBonus(p, []int{r.integer("bonus_id")}, s.Now())
			if e != nil {
				return e
			}
			w.Tasks = append(w.Tasks, id)
			w.Mini[id] = 1
			return nil
		}
		return errors.New("妄言接口不存在")
	})
	if err != nil {
		if direct {
			return []Push{push("Avatar", "on_submit_wangyan_consign_task", activityErrorCode(err), emptyActivityBox())}, nil
		}
		return []Push{Callback(cb, []any{activityErrorCode(err)})}, nil
	}
	out := activityPushes(c, s.Now())
	if direct {
		return append(out, materialManagerPush(c), cardMgrPush(c), push("Avatar", "on_submit_wangyan_consign_task", RetSuccess, box)), nil
	}
	return append(out, Callback(cb, []any{RetSuccess})), nil
}
