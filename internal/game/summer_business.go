package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"time"
)

type SummerMap struct {
	ID       int   `json:"map_id"`
	Unlocked []int `json:"unlock_node_list"`
	Starts   []int `json:"start_node_list"`
	Frontier []int `json:"can_unlock_node_list"`
	Finished []int `json:"finish_node_list"`
	Total    int   `json:"all_finish_node_num"`
	Boat     int   `json:"boat_node_id"`
	Removed  int   `json:"remove_count"`
}
type PendingFishing struct {
	Site    int     `json:"site_id"`
	Fish    int     `json:"fish_id"`
	Length  float64 `json:"fish_length"`
	Started int64   `json:"started_at"`
}
type SummerState struct {
	SupplyDay string             `json:"server_supply_day,omitempty" wire:"-"`
	Maps      map[int]*SummerMap `json:"map_infos"`
	SP        int                `json:"compass_sp"`
	Sites     []int              `json:"unlock_fishing_sites"`
	Fish      map[int][2]float64 `json:"fish_info"`
	Handbook  []int              `json:"fish_handbook_bonus"`
	Items     []int              `json:"item_list"`
	Choose    int                `json:"choose_time"`
	Used      map[int]int        `json:"use_item_data"`
	LastItem  int                `json:"last_use_item_id"`
	Banners   []int              `json:"banner_bonus"`
	Completed []int              `json:"completed_dungeons"`
	Pending   *PendingFishing    `json:"pending_fishing,omitempty"`
}

func ensureSummer(p *Progress) *SummerState {
	if p.Activities.Summer == nil {
		p.Activities.Summer = &SummerState{Maps: map[int]*SummerMap{}, SP: activityData("summer_general_args", 1).integer("max_compass_sp"), Sites: []int{}, Fish: map[int][2]float64{}, Handbook: []int{}, Items: []int{}, Used: map[int]int{}, Banners: []int{}, Completed: []int{}}
	}
	return p.Activities.Summer
}

// 原生error_data72023和new_system_tips.activity_summer_map：每日不足上限补满，副本额外值可超过上限。
// 日界沿用本项目原生REFRESH_HMS=(0,0,0)，UTC+8；只保存日期，不累计离线天数的能量。
func refreshSummerDaily(w *SummerState, now time.Time) error {
	day := now.In(time.FixedZone("北京时间", 28800)).Format("2006-01-02")
	if day < w.SupplyDay {
		return errors.New("夏日冒险能量存档时间回拨")
	}
	if day > w.SupplyDay {
		limit := activityData("summer_general_args", 1).integer("max_compass_sp")
		if w.SP < 0 || limit <= 0 {
			return errors.New("夏日冒险能量存档或上限无效")
		}
		if w.SP < limit {
			w.SP = limit
		}
		w.SupplyDay = day
	}
	return nil
}
func summerProperties(w *SummerState) map[string]any {
	return map[string]any{"map_infos": w.Maps, "compass_sp": w.SP, "unlock_fishing_sites": w.Sites, "fish_info": w.Fish, "fish_handbook_bonus": w.Handbook, "item_list": w.Items, "choose_time": w.Choose, "use_item_data": w.Used, "last_use_item_id": w.LastItem, "banner_bonus": w.Banners}
}

// Android21400154.get_material_add_info对所有已获得宝藏累加material_add_rate，起始倍率1。
func applySummerMaterialBonus(p *Progress, box map[string]any, now time.Time) error {
	w := p.Activities.Summer
	if w == nil {
		return errors.New("夏日奖励状态丢失")
	}
	mid := activityData("summer_general_args", 1).integer("material_id")
	materials, ok := box["materials"].(map[int]int64)
	if !ok {
		return errors.New("夏日奖励盒结构无效")
	}
	base := materials[mid]
	if base <= 0 {
		return nil
	}
	rate := float64(0)
	for _, id := range w.Items {
		var v float64
		r := activityData("summer_item", id)
		if len(r) == 0 {
			continue
		}
		if json.Unmarshal(r["material_add_rate"], &v) != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("夏日宝藏收益倍率无效")
		}
		rate += v
	}
	if math.IsNaN(rate) || math.IsInf(rate, 0) || float64(base)*rate > math.MaxInt32 {
		return errors.New("夏日宝藏收益溢出")
	}
	added := int64(float64(base) * rate)
	if added <= 0 {
		return nil
	}
	changes := map[int]int64{}
	cards := []string{}
	if err := grantNativeItem(p, mid, added, p.AvatarLevel, now, changes, &cards, 0); err != nil {
		return err
	}
	materials[mid] += added
	return nil
}
func summerNeighbours(nodeID int) []int {
	n := activityData("summer_node", nodeID)
	out := []int{}
	for key := range androidActivities["summer_node"] {
		id, _ := strconv.Atoi(key)
		r := activityData("summer_node", id)
		if r.integer("map_id") == n.integer("map_id") && absInt(r.integer("x")-n.integer("x"))+absInt(r.integer("y")-n.integer("y")) == 1 {
			out = append(out, id)
		}
	}
	sort.Ints(out)
	return out
}
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// 原生summer_map.setup按四邻域扩展已开格，前沿只能从可达格产生。
func summerSetup(m *SummerMap) {
	m.Starts = []int{}
	m.Frontier = []int{}
	start := activityData("summer_map", m.ID).integer("start_node")
	if start <= 0 {
		return
	}
	queue := []int{start}
	seen := map[int]bool{}
	front := map[int]bool{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		m.Starts = append(m.Starts, id)
		for _, next := range summerNeighbours(id) {
			if containsInt(m.Unlocked, next) {
				queue = append(queue, next)
			} else {
				front[next] = true
			}
		}
	}
	for id := range front {
		m.Frontier = append(m.Frontier, id)
	}
	sort.Ints(m.Frontier)
}
func openSummerMap(p *Progress, id, level int) (*SummerMap, error) {
	r := activityData("summer_map", id)
	if len(r) == 0 || r.flag("_disable") {
		return nil, errors.New("夏日地图不存在")
	}
	if err := r.conditions("unlock_condition", *p, level); err != nil {
		return nil, err
	}
	w := ensureSummer(p)
	if m := w.Maps[id]; m != nil {
		return m, nil
	}
	m := &SummerMap{ID: id, Boat: r.integer("start_node"), Unlocked: []int{}, Finished: []int{}, Starts: []int{}, Frontier: []int{}}
	for _, nid := range r.ids("node_list") {
		n := activityData("summer_node", nid)
		if n.integer("locked") == 0 {
			m.Unlocked = append(m.Unlocked, nid)
			if site := n.integer("fishing_site_id"); site > 0 && !containsInt(w.Sites, site) {
				w.Sites = append(w.Sites, site)
			}
		}
		if n.integer("need_statistics") > 0 {
			m.Total++
		}
	}
	summerSetup(m)
	w.Maps[id] = m
	return m, nil
}
func canSummerNode(p *Progress, id, level int) error {
	n := activityData("summer_node", id)
	w := p.Activities.Summer
	if len(n) == 0 || n.flag("_disable") || w == nil || w.Maps[n.integer("map_id")] == nil {
		return runeReject("RET_SUMMER_MAP_LOCKED", "夏日地图未进入")
	}
	m := w.Maps[n.integer("map_id")]
	if !containsInt(m.Unlocked, id) {
		return runeReject("RET_SUMMER_NODE_LOCKED", "夏日节点仍有迷雾")
	}
	if pre := n.integer("pre_node"); pre > 0 && !containsInt(m.Finished, pre) {
		return runeReject("RET_SUMMER_PRE_NODE_LOCKED", "夏日前置节点未完成")
	}
	return n.conditions("unlock_condition", *p, level)
}
func prepareSummerDungeon(p *Progress, bc *ActivityBattleContext, level int, now time.Time) error {
	if p.Activities.Summer != nil {
		if err := refreshSummerDaily(p.Activities.Summer, now); err != nil {
			return err
		}
	}
	var node int
	if json.Unmarshal(androidActivities["summer_dungeon_to_node"][strconv.Itoa(bc.DungeonID)], &node) != nil || node <= 0 {
		return errors.New("夏日副本与节点映射不存在")
	}
	n := activityData("summer_node", node)
	bc.MapID, bc.NodeID = n.integer("map_id"), node
	if err := canSummerNode(p, node, level); err != nil {
		return err
	}
	return nil
}
func finishSummerNode(w *SummerState, node int) error {
	n := activityData("summer_node", node)
	m := w.Maps[n.integer("map_id")]
	if m == nil {
		return errors.New("夏日结算地图丢失")
	}
	if !containsInt(m.Finished, node) {
		m.Finished = append(m.Finished, node)
		if containsInt(activityData("summer_map", m.ID).ids("key_node_list"), node) {
			m.Removed = 0
		}
	}
	return nil
}
func finishSummerDungeon(p *Progress, bc *ActivityBattleContext, now time.Time) error {
	w := p.Activities.Summer
	if w == nil {
		return errors.New("夏日活动状态丢失")
	}
	r := activityData("summer_dungeons", bc.DungeonID)
	if err := finishSummerNode(w, bc.NodeID); err != nil {
		return err
	}
	if !containsInt(w.Completed, bc.DungeonID) {
		w.Completed = append(w.Completed, bc.DungeonID)
		n := r.integer("choose_item")
		if n < 0 || w.Choose > math.MaxInt32-n {
			return errors.New("夏日选择道具次数溢出")
		}
		w.Choose += n
	}
	sp := r.integer("bonus_sp")
	if sp < 0 || w.SP > math.MaxInt32-sp {
		return errors.New("夏日冒险能量溢出")
	}
	w.SP += sp
	if r.flag("unlock_all") {
		m := w.Maps[bc.MapID]
		m.Unlocked = append([]int{}, activityData("summer_map", bc.MapID).ids("node_list")...)
		summerSetup(m)
	}
	return nil
}

func (s *Service) summerRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	lengths := map[string]int{"summer_game_enter_map": 2, "summer_game_move_to": 3, "summer_game_move_boat": 3, "summer_game_enter_node": 3, "summer_game_choose_item": 2, "summer_game_clear_sp": 1, "start_fishing": 2, "check_fishing_result": 2, "receive_fish_handbook_bonus": 2, "receive_all_fish_handbook_bonus": 1, "receive_summer_banner_bonus": 2}
	if len(args) != lengths[method] {
		return nil, errors.New("夏日接口参数数量无效")
	}
	var cb, id, node int
	if json.Unmarshal(args[0], &cb) != nil || cb <= 0 {
		return nil, errors.New("夏日回调编号无效")
	}
	if len(args) > 1 && method != "check_fishing_result" {
		if json.Unmarshal(args[1], &id) != nil {
			return nil, errors.New("夏日编号无效")
		}
	}
	var extra map[string]json.RawMessage
	if method == "summer_game_enter_node" || method == "summer_game_move_to" {
		if json.Unmarshal(args[2], &extra) != nil || extra == nil {
			return nil, errors.New("夏日附加字典无效")
		}
	}
	if method == "summer_game_move_boat" && json.Unmarshal(args[2], &node) != nil {
		return nil, errors.New("夏日划船节点无效")
	}
	if method == "summer_game_enter_node" {
		n := activityData("summer_node", id)
		dungeon := n.integer("dungeon_id")
		if v := extra["dungeon_id"]; len(v) > 0 {
			if json.Unmarshal(v, &dungeon) != nil {
				return nil, errors.New("夏日副本编号无效")
			}
			if !containsInt(n.ids("daily_dungeon"), dungeon) && dungeon != n.integer("dungeon_id") {
				return nil, errors.New("夏日节点副本不匹配")
			}
		}
		if dungeon > 0 {
			return s.enterDungeon(ctx, c, []json.RawMessage{args[0], json.RawMessage(strconv.Itoa(dungeon)), args[2]})
		}
	}
	box := emptyActivityBox()
	reply := []any{RetSuccess}
	if method == "start_fishing" {
		reply = []any{RetSuccess, 0, 0.0}
	}
	hasBox := method == "summer_game_enter_node" || method == "summer_game_clear_sp" || method == "check_fishing_result" || method == "receive_fish_handbook_bonus" || method == "receive_all_fish_handbook_bonus" || method == "receive_summer_banner_bonus"
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		level := c.SelectedAvatarUnsafe().Info.Level
		if _, e := activityOpen(*p, 211, level, s.Now()); e != nil {
			return e
		}
		if p.Activities.LastTime > s.Now().Unix() {
			return errors.New("夏日存档时间回拨")
		}
		p.Activities.LastTime = s.Now().Unix()
		w := ensureSummer(p)
		if err := refreshSummerDaily(w, s.Now()); err != nil {
			return err
		}
		general := activityData("summer_general_args", 1)
		switch method {
		case "summer_game_enter_map":
			_, e := openSummerMap(p, id, level)
			return e
		case "summer_game_move_to":
			n := activityData("summer_node", id)
			if len(n) == 0 || n.flag("_disable") {
				return errors.New("夏日节点不存在或已禁用")
			}
			if err := n.conditions("unlock_condition", *p, level); err != nil {
				return err
			}
			m := w.Maps[n.integer("map_id")]
			if m == nil {
				return errors.New("夏日地图未进入")
			}
			if containsInt(m.Starts, id) {
				return nil
			}
			if !containsInt(m.Frontier, id) {
				return runeReject("RET_SUMMER_CLOUD_LOCKED", "夏日迷雾不在可达前沿")
			}
			cost := general.integer("remove_fog_sp")
			if cost <= 0 || w.SP < cost {
				return runeReject("RET_SUMMER_SP_NOT_ENOUGH", "夏日冒险能量不足")
			}
			w.SP -= cost
			m.Unlocked = append(m.Unlocked, id)
			m.Removed++
			summerSetup(m)
			if site := n.integer("fishing_site_id"); site > 0 && !containsInt(w.Sites, site) {
				w.Sites = append(w.Sites, site)
			}
			return nil
		case "summer_game_move_boat":
			m := w.Maps[id]
			if m == nil || !containsInt(m.Starts, node) {
				return errors.New("夏日划船目标不可达")
			}
			m.Boat = node
			return nil
		case "summer_game_enter_node":
			if e := canSummerNode(p, id, level); e != nil {
				return e
			}
			n := activityData("summer_node", id)
			if n.integer("site_type") != 4 {
				return errors.New("夏日节点需要对应副本或钓鱼入口")
			}
			m := w.Maps[n.integer("map_id")]
			if containsInt(m.Finished, id) {
				return errors.New("夏日事件奖励已经领取")
			}
			var choice int
			if json.Unmarshal(extra["choice_id"], &choice) != nil {
				return errors.New("夏日事件选择无效")
			}
			r := activityData("summer_events", n.integer("event_id"))
			var opts [][]json.RawMessage
			if json.Unmarshal(r["opts"], &opts) != nil || choice < 0 || choice >= len(opts) || len(opts[choice]) != 2 {
				return errors.New("夏日事件选项不存在")
			}
			var bid int
			if json.Unmarshal(opts[choice][1], &bid) != nil {
				return errors.New("夏日事件奖励无效")
			}
			var e error
			box, e = grantActivityBonus(p, []int{bid}, s.Now(), level)
			if e != nil {
				return e
			}
			return finishSummerNode(w, id)
		case "summer_game_choose_item":
			if w.Choose <= 0 || containsInt(w.Items, id) || len(activityData("summer_item", id)) == 0 {
				return errors.New("夏日道具不可选择")
			}
			available := []int{}
			for key := range androidActivities["summer_item"] {
				n, _ := strconv.Atoi(key)
				if !containsInt(w.Items, n) {
					available = append(available, n)
				}
			}
			sort.Ints(available)
			if len(available) > 2 {
				available = available[:2]
			}
			if !containsInt(available, id) {
				return errors.New("夏日道具不在本次选项")
			}
			w.Items = append(w.Items, id)
			w.Choose--
			return nil
		case "start_fishing":
			if !containsInt(w.Sites, id) || len(activityData("summer_fishing_site", id)) == 0 {
				return errors.New("钓鱼场所尚未解锁")
			}
			if w.Pending != nil {
				if w.Pending.Site != id {
					return errors.New("已有另一场钓鱼")
				}
				reply = []any{RetSuccess, w.Pending.Fish, w.Pending.Length}
				return nil
			}
			site := activityData("summer_fishing_site", id)
			ids := site.ids("fish_id")
			weights := site.ids("fish_weight")
			if len(ids) != len(weights) {
				return errors.New("钓鱼权重配置无效")
			}
			i, e := activityWeighted(weights)
			if e != nil {
				return e
			}
			fish := activityData("summer_fish", ids[i])
			if !remainingPolicyEnabled() {
				return errors.New("鱼长分布本服规则未启用")
			}
			var bounds []float64
			if json.Unmarshal(fish["body_form"], &bounds) != nil || len(bounds) != 2 || bounds[0] <= 0 || bounds[1] < bounds[0] {
				return errors.New("鱼长度原生区间无效")
			}
			n, e := activityAmount([]int64{int64(math.Ceil(bounds[0] * 100)), int64(math.Floor(bounds[1] * 100))})
			if e != nil {
				return e
			}
			bait := general.integer("stosh_material_id")
			m := p.Materials[bait]
			if m.Count < 1 {
				return errors.New("鱼饵不足")
			}
			m.Count--
			p.Materials[bait] = m
			w.Pending = &PendingFishing{Site: id, Fish: ids[i], Length: float64(n) / 100, Started: s.Now().UnixMilli()}
			reply = []any{RetSuccess, w.Pending.Fish, w.Pending.Length}
			return nil
		case "check_fishing_result":
			var success bool
			if json.Unmarshal(args[1], &success) != nil {
				return errors.New("钓鱼结果必须为布尔值")
			}
			pending := w.Pending
			if pending == nil {
				return errors.New("没有待结算钓鱼")
			}
			fish := activityData("summer_fish", pending.Fish)
			var wait float64
			_ = json.Unmarshal(fish["wait_time"], &wait)
			if s.Now().UnixMilli() < pending.Started+int64(wait*1000) {
				return errors.New("钓鱼等待时间未结束")
			}
			key := "failed_bonus"
			if success {
				key = "success_bonus"
			}
			var e error
			box, e = grantActivityBonus(p, []int{fish.integer(key)}, s.Now(), level)
			if e != nil {
				return e
			}
			if success {
				old := w.Fish[pending.Fish]
				if old[0] >= math.MaxInt32 {
					return errors.New("捕鱼数量溢出")
				}
				old[0]++
				old[1] = math.Max(old[1], pending.Length)
				w.Fish[pending.Fish] = old
			}
			w.Pending = nil
			return nil
		case "receive_fish_handbook_bonus", "receive_all_fish_handbook_bonus":
			var tiers [][]int
			if json.Unmarshal(general["handbook_bonus"], &tiers) != nil {
				return errors.New("钓鱼手册奖励配置无效")
			}
			claimed := []int{}
			bonuses := []int{}
			for _, tier := range tiers {
				if len(tier) != 2 {
					return errors.New("钓鱼手册奖励结构无效")
				}
				if tier[0] <= len(w.Fish) && !containsInt(w.Handbook, tier[0]) && (method == "receive_all_fish_handbook_bonus" || tier[0] == id) {
					claimed = append(claimed, tier[0])
					bonuses = append(bonuses, tier[1])
				}
			}
			if len(bonuses) == 0 {
				return errors.New("钓鱼手册没有可领奖项")
			}
			var e error
			box, e = grantActivityBonus(p, bonuses, s.Now(), level)
			if e != nil {
				return e
			}
			w.Handbook = append(w.Handbook, claimed...)
			return nil
		case "receive_summer_banner_bonus":
			bonus := general.ids("banner_bonus")
			if id < 0 || id >= len(bonus) || containsInt(w.Banners, id) {
				return errors.New("夏日横幅奖励编号无效或已领取")
			}
			var e error
			box, e = grantActivityBonus(p, []int{bonus[id]}, s.Now(), level)
			if e != nil {
				return e
			}
			w.Banners = append(w.Banners, id)
			return nil
		case "summer_game_clear_sp":
			if e := general.conditions("unlock_condition", *p, level); e != nil {
				return e
			}
			if w.SP <= 0 || w.SP > 100000 {
				return errors.New("夏日冒险能量转换数量无效")
			}
			bonuses := make([]int, w.SP)
			for i := range bonuses {
				bonuses[i] = general.integer("bonus_per_sp")
			}
			var e error
			box, e = grantActivityBonus(p, bonuses, s.Now(), level)
			if e != nil {
				return e
			}
			w.SP = 0
			return nil
		}
		return errors.New("夏日业务不存在")
	})
	if err != nil {
		reply[0] = activityErrorCode(err)
		if hasBox {
			reply = []any{reply[0], emptyActivityBox()}
		}
		return []Push{Callback(cb, reply)}, nil
	}
	out := append(activityPushes(c, s.Now()), materialManagerPush(c), cardMgrPush(c), knowledgePush(c))
	if hasBox {
		reply = []any{RetSuccess, box}
	}
	return append(out, Callback(cb, reply)), nil
}
