package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// 克苏鲁状态与属性名来自Android 2907B608，额外回合凭证只保存在服务端。
type CthulhuState struct {
	Maps        map[int]*CthulhuMap `json:"maps"`
	Initialized bool                `json:"initialized"`
	Title       string              `json:"title"`
}
type CthulhuItem struct {
	ID           int                  `json:"item_id"`
	Index        int                  `json:"item_idx"`
	Priority     int                  `json:"item_priority"`
	Status       int                  `json:"status"`
	Cycles       int                  `json:"cycle_count"`
	Checks       int                  `json:"item_check_count"`
	CheckReceipt *CthulhuCheckReceipt `json:"server_check_receipt,omitempty" wire:"-"`
}
type CthulhuGrid struct {
	Index  int          `json:"grid_idx"`
	ID     int          `json:"grid_id"`
	Status int          `json:"status"`
	Item   *CthulhuItem `json:"current_item"`
}
type CthulhuMap struct {
	ID         int                          `json:"map_id"`
	Layer      int                          `json:"current_layer"`
	Layers     map[int]int                  `json:"layer_status"`
	Intro      map[int]int                  `json:"layer_init_item"`
	LayerItems map[int]*CthulhuItem         `json:"layer_items"`
	Finished   int                          `json:"finished"`
	Clues      map[int]int                  `json:"clue_times"`
	State      int                          `json:"state"`
	Trunk      int                          `json:"trunk_grid_idx"`
	Trunks     map[int]*CthulhuGrid         `json:"trunk_grid_infos"`
	Branch     int                          `json:"branch_grid_idx"`
	Branches   map[int]*CthulhuGrid         `json:"branch_grid_infos"`
	Libraries  map[int]map[int]*CthulhuItem `json:"current_item_libs"`
	FirstCheck bool                         `json:"first_check"`
	Moves      int                          `json:"move_count"`
	Checks     int                          `json:"check_count"`
	Init       *CthulhuItem                 `json:"init_item"`
	Cycle      int                          `json:"server_cycle,omitempty" wire:"-"`
}

func ensureCthulhu(p *Progress) *CthulhuState {
	if p.Materials == nil {
		p.Materials = map[int]Material{}
	}
	if p.Activities.Cthulhu == nil {
		var title string
		_ = json.Unmarshal(activityData("cthulhu", 206)["default_sub_name"], &title)
		p.Activities.Cthulhu = &CthulhuState{Maps: map[int]*CthulhuMap{}, Title: title}
	}
	return p.Activities.Cthulhu
}
func cthulhuItem(id, index, priority int) *CthulhuItem {
	return &CthulhuItem{ID: id, Index: index, Priority: priority, Status: -1}
}
func (m *CthulhuMap) grid() *CthulhuGrid {
	if m.Branch > 0 {
		return m.Branches[m.Branch]
	}
	return m.Trunks[m.Trunk]
}
func (m *CthulhuMap) ended() bool { return m.Trunk == len(m.Trunks)-1 && m.Branch == 0 }
func (m *CthulhuMap) canonical(item *CthulhuItem, gridID int) *CthulhuItem {
	if item == nil {
		return cthulhuItem(0, -1, 0)
	}
	if item.Index >= 0 {
		if x := m.Libraries[gridID][item.Index]; x != nil && x.ID == item.ID {
			return x
		}
	}
	return item
}
func (m *CthulhuMap) restore() {
	for _, g := range m.Trunks {
		g.Item = m.canonical(g.Item, g.ID)
	}
	for _, g := range m.Branches {
		g.Item = m.canonical(g.Item, g.ID)
	}
}
func cthulhuItemDone(x *CthulhuItem) bool { return x == nil || x.Status == -1 || x.Status == 3 }
func cthulhuVisitItem(p *Progress, x *CthulhuItem, now time.Time) (map[string]any, error) {
	if x == nil || x.ID == 0 {
		return emptyActivityBox(), nil
	}
	if x.Status == 3 {
		return emptyActivityBox(), nil
	}
	r := activityData("cthulhu_items", x.ID)
	switch r.integer("item_type") {
	case 2:
		box, e := grantCthulhuBonus(p, []int{r.integer("bonus_id")}, now)
		if e != nil {
			return nil, e
		}
		x.Status = 3
		return box, nil
	case 1, 3, 4:
		x.Status = 1
		return emptyActivityBox(), nil
	default:
		return nil, errors.New("克苏鲁事件类型无效")
	}
}
func cthulhuVisitGrid(p *Progress, g *CthulhuGrid, now time.Time) (map[string]any, error) {
	if g == nil {
		return nil, errors.New("克苏鲁格子不存在")
	}
	r := activityData("cthulhu_grids", g.ID)
	if r.integer("grid_type") == 1 {
		if g.Status == 2 {
			return emptyActivityBox(), nil
		}
		box, e := grantCthulhuBonus(p, []int{r.integer("bonus_id")}, now)
		if e != nil {
			return nil, e
		}
		g.Status = 2
		return box, nil
	}
	g.Status = 2
	if r.integer("grid_type") == 2 {
		return emptyActivityBox(), nil
	}
	return cthulhuVisitItem(p, g.Item, now)
}
func cthulhuChooseItem(m *CthulhuMap, gridID, day int, otherLayers ...int) (*CthulhuItem, error) {
	pool := []*CthulhuItem{}
	weights := []int{}
	priority := math.MaxInt32
	keys := []int{}
	for k := range m.Libraries[gridID] {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		x := m.Libraries[gridID][k]
		if x.Status != -1 {
			continue
		}
		r := activityData("cthulhu_items", x.ID)
		if days := r.ids("open_days"); len(days) > 0 && !containsInt(days, day) {
			continue
		}
		// 原生1EBB86C4/cthulhu_item.check_unlock：open_layer是单个层编号，传入所有地图的当前层。
		if layer := r.integer("open_layer"); layer > 0 {
			if layer != m.Layer && m.Layers[layer] < 1 && !containsInt(otherLayers, layer) {
				continue
			}
		}
		if x.Priority < priority {
			pool = nil
			weights = nil
			priority = x.Priority
		}
		if x.Priority == priority {
			pool = append(pool, x)
			weights = append(weights, r.integer("item_weight"))
		}
	}
	if len(pool) > 0 {
		i, e := activityWeighted(weights)
		if e != nil {
			return nil, e
		}
		pool[i].Status = 0
		return pool[i], nil
	}
	r := activityData("cthulhu_grids", gridID)
	defaults := r.ids("default_item")
	if len(defaults) > 0 {
		w := make([]int, len(defaults))
		for i := range w {
			w[i] = 1
		}
		i, e := activityWeighted(w)
		if e != nil {
			return nil, e
		}
		x := cthulhuItem(defaults[i], -1, 0)
		x.Status = 0
		return x, nil
	}
	return cthulhuItem(0, -1, 0), nil
}
func cthulhuLibrary(gridID int) map[int]*CthulhuItem {
	r := activityData("cthulhu_grids", gridID)
	var groups [][]int
	_ = json.Unmarshal(r["item_libs"], &groups)
	out := map[int]*CthulhuItem{}
	idx := 0
	for pri, g := range groups {
		for _, id := range g {
			out[idx] = cthulhuItem(id, idx, pri+1)
			idx++
		}
	}
	return out
}
func cthulhuNewMap(id, day int, otherLayers ...int) (*CthulhuMap, error) {
	r := activityData("cthulhu_maps", id)
	layers := r.ids("layer_list")
	if len(layers) == 0 {
		return nil, errors.New("克苏鲁地图层为空")
	}
	m := &CthulhuMap{ID: id, Layer: layers[0], Layers: map[int]int{}, Intro: map[int]int{}, LayerItems: map[int]*CthulhuItem{}, Clues: map[int]int{}, Trunks: map[int]*CthulhuGrid{}, Branches: map[int]*CthulhuGrid{}, Libraries: map[int]map[int]*CthulhuItem{}, Init: cthulhuItem(r.integer("init_item_id"), -1, 0)}
	if m.Init.ID > 0 {
		m.Init.Status = 0
	}
	for _, l := range layers {
		m.Layers[l] = 0
		x := cthulhuItem(activityData("cthulhu_layers", l).integer("finished_item"), -1, 0)
		if x.ID > 0 {
			x.Status = 0
		}
		m.LayerItems[l] = x
	}
	trunk := append([]int{r.integer("end_grid_id")}, r.ids("trunk_grids")...)
	trunk = append(trunk, r.integer("end_grid_id"))
	for idx, gid := range trunk {
		if m.Libraries[gid] == nil {
			m.Libraries[gid] = cthulhuLibrary(gid)
		}
		x, e := cthulhuChooseItem(m, gid, day, otherLayers...)
		if e != nil {
			return nil, e
		}
		g := &CthulhuGrid{Index: idx, ID: gid, Status: 1, Item: x}
		m.Trunks[idx] = g
		gr := activityData("cthulhu_grids", gid)
		if gr.integer("grid_type") == 2 {
			for bi, bgid := range gr.ids("branch_grids") {
				if _, exists := m.Branches[bi+1]; exists {
					return nil, errors.New("克苏鲁地图存在多个未编码分支")
				}
				if m.Libraries[bgid] == nil {
					m.Libraries[bgid] = cthulhuLibrary(bgid)
				}
				bx, e := cthulhuChooseItem(m, bgid, day, otherLayers...)
				if e != nil {
					return nil, e
				}
				m.Branches[bi+1] = &CthulhuGrid{Index: idx, ID: bgid, Status: 1, Item: bx}
			}
		}
	}
	m.Trunk = r.integer("init_grid_idx")
	if m.Trunks[m.Trunk] == nil {
		return nil, errors.New("克苏鲁初始格无效")
	}
	m.Trunks[m.Trunk].Status = 2
	if old := m.Trunks[m.Trunk].Item; old != nil && old.Index >= 0 {
		old.Status = -1
	}
	m.Trunks[m.Trunk].Item = cthulhuItem(0, -1, 0)
	return m, nil
}
func cthulhuCanMove(m *CthulhuMap) bool {
	g := m.grid()
	if g == nil || m.ended() || g.Status <= 0 {
		return false
	}
	if activityData("cthulhu_grids", g.ID).integer("grid_type") != 3 {
		return true
	}
	return g.Item == nil || g.Item.Status >= 2 || g.Item.Status == -1
}
func cthulhuUnlocked(m *CthulhuMap, flag string) bool {
	for l, s := range m.Layers {
		if s >= 1 && activityData("cthulhu_layers", l).flag(flag) {
			return true
		}
	}
	return false
}

// tryMove复现原生分支跳转和强制停留；preview只读取，真实移动才增加经过事件cycle_count。
func cthulhuTryMove(m *CthulhuMap, step int, real bool) (int, int, int, error) {
	t, b, rest := m.Trunk, m.Branch, step
	g := m.grid()
	if g == nil {
		return 0, 0, rest, errors.New("克苏鲁当前格无效")
	}
	advance := func(ng *CthulhuGrid) bool {
		if ng == nil {
			return false
		}
		rest--
		x := ng.Item
		if x != nil && x.ID > 0 {
			row := activityData("cthulhu_items", x.ID)
			var limit *int
			_ = json.Unmarshal(row["force_item_count"], &limit)
			if limit != nil && x.Cycles >= *limit && !cthulhuItemDone(x) {
				return false
			}
			if real {
				x.Cycles++
			}
		}
		return true
	}
	if activityData("cthulhu_grids", g.ID).integer("grid_type") == 2 || b > 0 {
		for rest > 0 {
			ng := m.Branches[b+1]
			if ng == nil {
				break
			}
			b++
			if !advance(ng) {
				return t, b, rest, nil
			}
		}
	}
	for rest > 0 {
		next := t + 1
		if b > 0 {
			next = t + activityData("cthulhu_grids", m.Trunks[t].ID).integer("branch_endpoint")
			b = 0
		}
		ng := m.Trunks[next]
		if ng == nil {
			break
		}
		t = next
		if !advance(ng) {
			break
		}
		if t == len(m.Trunks)-1 {
			break
		}
	}
	return t, b, rest, nil
}
func cthulhuConsume(p *Progress, pair []int) error {
	if len(pair) != 2 || pair[0] <= 0 || pair[1] <= 0 {
		return errors.New("克苏鲁材料费用无效")
	}
	m := p.Materials[pair[0]]
	if m.Count < int64(pair[1]) {
		return errors.New("克苏鲁材料不足")
	}
	m.Count -= int64(pair[1])
	p.Materials[pair[0]] = m
	return nil
}
func cthulhuRefresh(p *Progress, mapID, day int, now time.Time) (bool, error) {
	s := ensureCthulhu(p)
	r := activityData("cthulhu_maps", mapID)
	if len(r) == 0 || !containsInt(activityData("cthulhu", 206).ids("map_list"), mapID) {
		return false, errors.New("克苏鲁地图不存在")
	}
	if days := r.ids("open_days"); len(days) > 0 && !containsInt(days, day) {
		return false, errors.New("克苏鲁地图未到开放日")
	}
	if last := r.integer("last_map_id"); last > 0 {
		if s.Maps[last] == nil || s.Maps[last].Finished <= 0 {
			return false, errors.New("克苏鲁上一地图未完成")
		}
	}
	m := s.Maps[mapID]
	otherLayers := []int{}
	for _, other := range s.Maps {
		otherLayers = append(otherLayers, other.Layer)
	}
	if m == nil {
		var e error
		m, e = cthulhuNewMap(mapID, day, otherLayers...)
		if e != nil {
			return false, e
		}
		s.Maps[mapID] = m
		first := !s.Initialized
		if first {
			var init [][]int64
			if json.Unmarshal(activityData("cthulhu", 206)["init_materials"], &init) != nil {
				return false, errors.New("克苏鲁初始材料无效")
			}
			for _, pair := range init {
				if len(pair) != 2 || pair[0] <= 0 || pair[1] < 0 {
					return false, errors.New("克苏鲁初始材料无效")
				}
				mid := int(pair[0])
				mat := p.Materials[mid]
				mat.ID = mid
				mat.Count += pair[1]
				mat.Total += pair[1]
				p.Materials[mid] = mat
			}
			s.Initialized = true
		}
		return first, nil
	}
	m.restore()
	if !m.ended() {
		return false, errors.New("克苏鲁必须回到起点后刷新")
	}
	lr := activityData("cthulhu_layers", m.Layer)
	ready := len(lr.ids("unlock_materials")) > 0
	for _, mid := range lr.ids("unlock_materials") {
		if p.Materials[mid].Count <= 0 {
			ready = false
		}
	}
	if ready {
		if !cthulhuItemDone(m.LayerItems[m.Layer]) {
			return false, errors.New("克苏鲁本层结束事件未完成")
		}
		m.Layers[m.Layer] = 1
		layers := r.ids("layer_list")
		idx := intIndex(layers, m.Layer)
		if idx < 0 {
			return false, errors.New("克苏鲁存档层不存在")
		}
		if idx+1 < len(layers) {
			m.Layer = layers[idx+1]
		} else {
			m.Finished = 1
		}
	}
	for _, set := range []map[int]*CthulhuGrid{m.Trunks, m.Branches} {
		for _, g := range set {
			if activityData("cthulhu_grids", g.ID).integer("grid_type") == 3 {
				if cthulhuItemDone(g.Item) || g.Item.Index < 0 {
					x, e := cthulhuChooseItem(m, g.ID, day, otherLayers...)
					if e != nil {
						return false, e
					}
					g.Item = x
				} else {
					g.Item.Checks = 0
				}
			}
			g.Status = 1
		}
	}
	m.Trunk = 0
	m.Branch = 0
	m.Cycle++
	return false, nil
}
func intIndex(list []int, id int) int {
	for i, v := range list {
		if v == id {
			return i
		}
	}
	return -1
}
func cthulhuBattleItem(m *CthulhuMap, bc *ActivityBattleContext) *CthulhuItem {
	if m.Cycle != bc.Cycle || m.Layer != bc.Layer || m.Trunk != bc.Trunk || m.Branch != bc.Branch {
		return nil
	}
	switch bc.Place {
	case "init":
		return m.Init
	case "layer":
		return m.LayerItems[m.Layer]
	case "grid":
		if g := m.grid(); g != nil {
			return g.Item
		}
	}
	return nil
}
func prepareCthulhuDungeon(p *Progress, bc *ActivityBattleContext) error {
	state := p.Activities.Cthulhu
	if state == nil {
		return errors.New("克苏鲁地图尚未进入")
	}
	count := 0
	for _, m := range state.Maps {
		m.restore()
		candidates := map[string]*CthulhuItem{"init": m.Init}
		if g := m.grid(); g != nil {
			candidates["grid"] = g.Item
		}
		if m.ended() {
			candidates["layer"] = m.LayerItems[m.Layer]
		}
		for place, x := range candidates {
			if x == nil || x.Status != 1 && x.Status != 2 {
				continue
			}
			r := activityData("cthulhu_items", x.ID)
			if kind := r.integer("item_type"); kind != 1 && kind != 4 {
				continue
			}
			var params [][]int
			_ = json.Unmarshal(r["dungeon_params"], &params)
			for _, param := range params {
				if len(param) > 0 && param[0] == bc.DungeonID {
					count++
					bc.MapID = m.ID
					bc.NodeID = x.ID
					bc.Place = place
					bc.Layer = m.Layer
					bc.Cycle = m.Cycle
					bc.Trunk = m.Trunk
					bc.Branch = m.Branch
					break
				}
			}
		}
	}
	if count != 1 {
		return errors.New("克苏鲁副本没有唯一已触发事件上下文")
	}
	return nil
}
func finishCthulhuDungeon(p *Progress, bc *ActivityBattleContext, now time.Time) (map[string]any, error) {
	if p.Activities.Cthulhu == nil {
		return nil, errors.New("克苏鲁战斗地图缺失")
	}
	m := p.Activities.Cthulhu.Maps[bc.MapID]
	if m == nil {
		return nil, errors.New("克苏鲁战斗地图缺失")
	}
	m.restore()
	x := cthulhuBattleItem(m, bc)
	if x == nil || x.ID != bc.NodeID || x.Status != 1 && x.Status != 2 {
		return nil, errors.New("克苏鲁战斗事件已变化")
	}
	box, e := grantCthulhuBonus(p, []int{activityData("cthulhu_items", x.ID).integer("bonus_id")}, now)
	if e != nil {
		return nil, e
	}
	x.Status = 3
	return box, nil
}
func mergeActivityBox(dst, src map[string]any) error {
	if src == nil {
		return nil
	}
	a, ok := dst["materials"].(map[int]int64)
	if !ok {
		return errors.New("活动奖励盒材料结构无效")
	}
	b, ok := src["materials"].(map[int]int64)
	if !ok {
		return errors.New("活动奖励盒材料结构无效")
	}
	for mid, n := range b {
		if n < 0 || a[mid] > math.MaxInt64-n {
			return errors.New("活动奖励盒溢出")
		}
		a[mid] += n
	}
	if srcCards, ok := src["cards"].([]any); ok {
		dstCards, ok := dst["cards"].([]any)
		if !ok {
			return errors.New("活动奖励盒幻书结构无效")
		}
		dst["cards"] = append(activityCardRows(dstCards), activityCardRows(srcCards)...)
	}
	if value, ok := src["runes"]; ok {
		incoming, ok := value.(map[string]any)
		if !ok {
			return errors.New("活动奖励盒契印结构无效")
		}
		present, ok := dst["runes"].(map[string]any)
		if !ok {
			present = map[string]any{}
		}
		for id, row := range incoming {
			if _, exists := present[id]; exists {
				return errors.New("活动奖励盒契印重复")
			}
			present[id] = row
		}
		dst["runes"] = present
	}
	return nil
}

func activityCardRows(rows []any) []any {
	if len(rows) == 2 {
		if nested, ok := rows[0].([]any); ok {
			if marker, ok := rows[1].([]any); ok && len(marker) == 2 && marker[0] == "card.card_list" && marker[1] == "__custom_type" {
				return nested
			}
		}
	}
	if len(rows) > 0 {
		if marker, ok := rows[len(rows)-1].([]any); ok && len(marker) == 2 && marker[0] == "card.card_list" && marker[1] == "__custom_type" {
			return rows[:len(rows)-1]
		}
	}
	return rows
}

func (s *Service) cthulhuRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	now := s.Now()
	level := c.SelectedAvatarUnsafe().Info.Level
	cb := 0
	box := emptyActivityBox()
	first := false
	result := map[string]any{}
	direct := method == "cthulhu_item_check" || method == "cthulhu_check_all_in" || method == "set_cthulhu_title"
	if !direct {
		if len(args) < 1 || json.Unmarshal(args[0], &cb) != nil {
			return nil, errors.New("克苏鲁回调参数无效")
		}
		args = args[1:]
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		day, e := activityOpen(*p, 206, level, now)
		if e != nil {
			return e
		}
		state := ensureCthulhu(p)
		if method == "set_cthulhu_title" {
			if len(args) != 1 {
				return errors.New("克苏鲁名称参数无效")
			}
			var title string
			if json.Unmarshal(args[0], &title) != nil || !utf8.ValidString(title) || utf8.RuneCountInString(title) > 16 || strings.TrimSpace(title) == "" || strings.ContainsAny(title, "\r\n\x00") {
				return errors.New("克苏鲁名称无效")
			}
			state.Title = title
			return nil
		}
		if method == "cthulhu_item_check" || method == "cthulhu_check_all_in" {
			if !remainingPolicyEnabled() {
				return errors.New("克苏鲁检定本服规则未启用")
			}
			if len(args) != 2 {
				return errors.New("克苏鲁检定需要事件及地图编号")
			}
			var itemID, mapID int
			if json.Unmarshal(args[0], &itemID) != nil || json.Unmarshal(args[1], &mapID) != nil {
				return errors.New("克苏鲁检定编号无效")
			}
			var receipt *CthulhuCheckReceipt
			receipt, e = performCthulhuCheck(p, mapID, itemID, method == "cthulhu_check_all_in", now, nil)
			if e != nil {
				return e
			}
			result = receipt.result()
			box, e = battleSettlementBoxWire(receipt.Box)
			return e
		}
		if method == "add_cthulhu_point" {
			if len(args) != 2 {
				return errors.New("克苏鲁加点参数无效")
			}
			var mid int
			var targets map[int]int
			if json.Unmarshal(args[0], &mid) != nil || json.Unmarshal(args[1], &targets) != nil {
				return errors.New("克苏鲁加点参数无效")
			}
			var init [][]int
			_ = json.Unmarshal(activityData("cthulhu", 206)["init_materials"], &init)
			if len(init) == 0 || len(init[0]) != 2 || mid != init[0][0] {
				return errors.New("克苏鲁加点来源无效")
			}
			total := 0
			for target, n := range targets {
				allowed := false
				for _, v := range init {
					if len(v) == 2 && v[0] == target {
						allowed = true
					}
				}
				if !allowed || target == mid || n <= 0 || n > activityData("cthulhu", 206).integer("init_max_point") {
					return errors.New("克苏鲁加点目标或数量无效")
				}
				row := activityData("materials", target)
				limit := row.integer("limit_count")
				if limit <= 0 || p.Materials[target].Count+int64(n) > int64(limit) {
					return errors.New("克苏鲁属性超过原生上限")
				}
				total += n
			}
			if total <= 0 {
				return errors.New("克苏鲁加点为空")
			}
			if e = cthulhuConsume(p, []int{mid, total}); e != nil {
				return e
			}
			for target, n := range targets {
				mat := p.Materials[target]
				mat.ID = target
				mat.Count += int64(n)
				mat.Total += int64(n)
				p.Materials[target] = mat
			}
			return nil
		}
		if len(args) < 1 {
			return errors.New("克苏鲁地图参数缺失")
		}
		var mapID int
		if json.Unmarshal(args[0], &mapID) != nil {
			return errors.New("克苏鲁地图参数无效")
		}
		if method == "refresh_cthulhu_activity" {
			if len(args) != 1 {
				return errors.New("克苏鲁刷新参数无效")
			}
			first, e = cthulhuRefresh(p, mapID, day, now)
			return e
		}
		m := state.Maps[mapID]
		if m == nil {
			return errors.New("克苏鲁地图尚未解锁")
		}
		m.restore()
		switch method {
		case "cthulhu_visit_init_item":
			if len(args) != 1 {
				return errors.New("克苏鲁初始事件参数无效")
			}
			box, e = cthulhuVisitItem(p, m.Init, now)
			return e
		case "cthulhu_visit_layer_init_item":
			if len(args) != 2 {
				return errors.New("克苏鲁层事件参数无效")
			}
			var layer int
			if json.Unmarshal(args[1], &layer) != nil || layer != m.Layer {
				return errors.New("克苏鲁层事件不属于当前层")
			}
			if _, ok := m.Intro[layer]; ok {
				return errors.New("克苏鲁层开场已访问")
			}
			item := cthulhuItem(activityData("cthulhu_layers", layer).integer("init_item_id"), -1, 0)
			if kind := activityData("cthulhu_items", item.ID).integer("item_type"); item.ID > 0 && kind != 2 {
				return errors.New("克苏鲁层开场战斗上下文未取证")
			}
			box, e = cthulhuVisitItem(p, item, now)
			if e == nil {
				m.Intro[layer] = 3
			}
			return e
		case "cthulhu_visit_layer_finish_item":
			if len(args) != 1 || !m.ended() {
				return errors.New("克苏鲁结束事件仅可在起点触发")
			}
			clues := activityData("cthulhu_layers", m.Layer).ids("unlock_materials")
			if len(clues) == 0 {
				return errors.New("克苏鲁层线索为空")
			}
			for _, mid := range clues {
				if p.Materials[mid].Count <= 0 {
					return errors.New("克苏鲁调查线索未收齐")
				}
			}
			box, e = cthulhuVisitItem(p, m.LayerItems[m.Layer], now)
			return e
		case "cthulhu_giveup":
			if len(args) != 1 {
				return errors.New("克苏鲁撤退参数无效")
			}
			g := m.grid()
			if g == nil || g.Item == nil || activityData("cthulhu_items", g.Item.ID).integer("item_type") != 4 || g.Item.Status != 1 {
				return errors.New("克苏鲁只有已触发挑战事件可撤退")
			}
			g.Item.Status = 2
			return nil
		case "cthulhu_move", "cthulhu_ctrl_move":
			if !cthulhuCanMove(m) || !cthulhuItemDone(m.Init) {
				return errors.New("克苏鲁当前事件未处理，无法前进")
			}
			introID := activityData("cthulhu_layers", m.Layer).integer("init_item_id")
			if introID > 0 && m.Intro[m.Layer] != 3 {
				return errors.New("克苏鲁本层开场尚未完成")
			}
			step := 0
			ctrl := method == "cthulhu_ctrl_move"
			if ctrl {
				if len(args) != 2 || json.Unmarshal(args[1], &step) != nil || step < 1 || step > 6 {
					return errors.New("克苏鲁控制步数无效")
				}
				if !cthulhuUnlocked(m, "finished_unlock_ctrl_move") {
					return errors.New("克苏鲁控制骰子尚未解锁")
				}
			} else {
				if len(args) != 1 {
					return errors.New("克苏鲁移动参数无效")
				}
				n, e := activityAmount([]int64{1, 6})
				if e != nil {
					return e
				}
				step = int(n)
			}
			if fixed := activityData("cthulhu_maps", m.ID).ids("move_steps"); m.Moves < len(fixed) {
				step = fixed[m.Moves]
			}
			if ctrl {
				_, _, rest, e := cthulhuTryMove(m, step, false)
				if e != nil {
					return e
				}
				if rest > 0 {
					return errors.New("克苏鲁控制步数跨过强制停留格")
				}
				if e = cthulhuConsume(p, activityData("cthulhu", 206).ids("ctrl_need_materials")); e != nil {
					return e
				}
			} else {
				cost := activityData("cthulhu", 206).integer("need_power")
				if cost < 0 || !p.consumePower(cost, now) {
					return errors.New("克苏鲁体力不足")
				}
			}
			t, b, rest, e := cthulhuTryMove(m, step, true)
			if e != nil {
				return e
			}
			m.Trunk = t
			m.Branch = b
			m.Moves++
			box, e = cthulhuVisitGrid(p, m.grid(), now)
			if e != nil {
				return e
			}
			result = map[string]any{"ended": m.ended(), "trunk_idx": t, "branch_idx": b, "random_step": step, "step": step - rest}
			return nil
		default:
			return errors.New("克苏鲁接口未知")
		}
	})
	if direct {
		if method == "set_cthulhu_title" {
			if err != nil {
				return []Push{nativeErrorPush(activityErrorCode(err))}, nil
			}
			return activityPushes(c, now), nil
		}
		if err != nil {
			return []Push{push("Avatar", "on_cthulhu_item_check", activityErrorCode(err), map[string]any{}, emptyActivityBox())}, nil
		}
		out := append(activityPushes(c, now), materialManagerPush(c), cardMgrPush(c), knowledgePush(c))
		return append(out, push("Avatar", "on_cthulhu_item_check", RetSuccess, result, box)), nil
	}
	success := err == nil
	msg := ""
	if err != nil {
		msg = err.Error()
		box = emptyActivityBox()
	}
	payload := []any{success, msg}
	switch method {
	case "refresh_cthulhu_activity":
		payload = []any{success, first, msg}
	case "cthulhu_visit_init_item", "cthulhu_visit_layer_init_item", "cthulhu_visit_layer_finish_item":
		payload = []any{success, box, msg}
	case "cthulhu_move", "cthulhu_ctrl_move":
		payload = []any{success, result, box, msg}
	}
	out := []Push{}
	if success {
		out = append(out, activityPushes(c, now)...)
		out = append(out, materialManagerPush(c), cardMgrPush(c))
		out = append(out, knowledgePush(c))
	}
	return append(out, Callback(cb, payload)), nil
}
