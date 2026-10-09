package game

import (
	"context"
	"encoding/json"
	"errors"
	"hs-server/internal/mobileproto"
	"sort"
	"strconv"
	"strings"
	"time"
)

type MikuNode struct {
	ID      int   `json:"node_id"`
	Passed  bool  `json:"pass_flag"`
	Bonuses []int `json:"bless_bonus_ids"`
	Event   int   `json:"event_id"`
	Angry   bool  `json:"can_set_angry"`
}
type MikuHandbookItem struct {
	ID    int `json:"item_id"`
	State int `json:"state"`
}
type MikuHandbook struct {
	ID    int                        `json:"handbook_id"`
	Items map[int][]MikuHandbookItem `json:"handbook_items"`
}
type MikuStory struct {
	ID       int  `json:"story_id"`
	Finished bool `json:"finished"`
}
type MikuMap struct {
	ID               int               `json:"map_id"`
	Nodes            map[int]*MikuNode `json:"nodes"`
	Current          int               `json:"cur_node_id"`
	Path             []int             `json:"explore_path"`
	Bonus            map[string]any    `json:"explore_bonus_box"`
	Auto             int               `json:"auto_path_id"`
	AutoHard         bool              `json:"is_auto_path_hard"`
	Treasure         map[string]int    `json:"treasure_state"`
	Score            int               `json:"explore_score"`
	Coins            map[int]int64     `json:"explore_coins"`
	Stories          map[int]MikuStory `json:"story_list"`
	Handbook         MikuHandbook      `json:"handbook_info"`
	ReceiptMaterials map[int]int64     `json:"receipt_materials,omitempty" wire:"-"`
	ReceiptCards     []string          `json:"receipt_cards,omitempty" wire:"-"`
	ReceiptRunes     []string          `json:"receipt_runes,omitempty" wire:"-"`
	CompletedRuns    int               `json:"server_completed_runs,omitempty" wire:"-"`
	SurpriseClaimed  bool              `json:"server_surprise_claimed,omitempty" wire:"-"`
	SurpriseBox      map[string]any    `json:"server_surprise_box,omitempty" wire:"-"`
}
type MikuAchievement struct {
	ID      int         `json:"achv_id"`
	Targets map[int]int `json:"finished_targets"`
	Claimed bool        `json:"is_bonus"`
}
type MikuState struct {
	Maps         map[int]*MikuMap        `json:"map_infos"`
	Achievements map[int]MikuAchievement `json:"achv_info"`
	Tasks        map[string]bool         `json:"tasks,omitempty"`
	Like         bool                    `json:"like_song_flag"`
	Visits       map[string]int          `json:"server_visits,omitempty"`
	Runs         int                     `json:"server_runs,omitempty"`
	Ends         int                     `json:"server_ends,omitempty"`
	Battles      map[int]int             `json:"server_battles,omitempty"`
	Gifts        map[int]int             `json:"server_gifts,omitempty"`
}

func ensureMiku(p *Progress) *MikuState {
	if p.Activities.Miku == nil {
		p.Activities.Miku = &MikuState{Maps: map[int]*MikuMap{}, Achievements: map[int]MikuAchievement{}, Tasks: map[string]bool{}}
	}
	w := p.Activities.Miku
	// 旧JSON可能有容器对象但将各映射存成null，不能只在对象nil时初始化。
	if w.Maps == nil {
		w.Maps = map[int]*MikuMap{}
	}
	if w.Achievements == nil {
		w.Achievements = map[int]MikuAchievement{}
	}
	if w.Tasks == nil {
		w.Tasks = map[string]bool{}
	}
	if w.Visits == nil {
		w.Visits = map[string]int{}
	}
	if w.Battles == nil {
		w.Battles = map[int]int{}
	}
	if w.Gifts == nil {
		w.Gifts = map[int]int{}
	}
	for _, m := range w.Maps {
		if m == nil {
			continue
		}
		if m.Nodes == nil {
			m.Nodes = map[int]*MikuNode{}
		}
		if m.Treasure == nil {
			m.Treasure = map[string]int{}
		}
		if m.Coins == nil {
			m.Coins = map[int]int64{}
		}
		if m.Stories == nil {
			m.Stories = map[int]MikuStory{}
		}
		if m.Handbook.Items == nil {
			m.Handbook.Items = map[int][]MikuHandbookItem{}
		}
		if m.ReceiptMaterials == nil {
			m.ReceiptMaterials = map[int]int64{}
		}
		if m.Bonus == nil {
			m.Bonus = emptyActivityBox()
		}
	}
	return p.Activities.Miku
}
func mikuProperties(w *MikuState) map[string]any {
	keys := []string{}
	for k := range w.Tasks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	tasks := mobileproto.Map{}
	for _, k := range keys {
		parts := strings.Split(k, ":")
		if len(parts) != 2 {
			continue
		}
		d, e1 := strconv.Atoi(parts[0])
		tid, e2 := strconv.Atoi(parts[1])
		if e1 == nil && e2 == nil {
			tasks = append(tasks, mobileproto.Pair{Key: []any{d, tid}, Value: w.Tasks[k]})
		}
	}
	return map[string]any{"map_infos": w.Maps, "achv_info": w.Achievements, "miku_task_info": tasks, "like_song_flag": w.Like}
}
func initMikuNode(id int) (*MikuNode, error) {
	r := activityData("miku_nodes", id)
	if len(r) == 0 {
		return nil, errors.New("初音节点不存在")
	}
	n := &MikuNode{ID: id, Bonuses: []int{}}
	var choices [][]int
	if json.Unmarshal(r["event_ids"], &choices) != nil {
		return nil, errors.New("初音节点事件配置无效")
	}
	if len(choices) == 0 {
		return n, nil
	}
	ids := []int{}
	weights := []int{}
	for _, pair := range choices {
		if len(pair) != 2 {
			return nil, errors.New("初音节点事件权重结构无效")
		}
		ids = append(ids, pair[0])
		weights = append(weights, pair[1])
	}
	i, err := activityWeighted(weights)
	if err != nil {
		return nil, err
	}
	n.Event = ids[i]
	switch r.integer("event_type") {
	case 1:
		e := activityData("miku_battle_events", n.Event)
		var chance float64
		if json.Unmarshal(e["angry_prob"], &chance) != nil {
			return nil, errors.New("初音愤怒概率无效")
		}
		n.Angry, err = activityChance(chance)
		if err != nil {
			return nil, err
		}
	case 2:
		e := activityData("miku_blessing_events", n.Event)
		pool := append([]int{}, e.ids("choice_bonus")...)
		for len(pool) > 0 && len(n.Bonuses) < 3 {
			weights = make([]int, len(pool))
			for i := range weights {
				weights[i] = 1
			}
			i, err = activityWeighted(weights)
			if err != nil {
				return nil, err
			}
			n.Bonuses = append(n.Bonuses, pool[i])
			pool = append(pool[:i], pool[i+1:]...)
		}
	default:
		return nil, errors.New("初音节点事件类型尚未接线")
	}
	return n, nil
}
func openMikuMap(p *Progress, id, day, level int) (*MikuMap, error) {
	r := activityData("miku_map", id)
	if len(r) == 0 || day < r.integer("open_day") {
		return nil, errors.New("初音地图未开放")
	}
	if err := r.conditions("unlock_condition", *p, level); err != nil {
		return nil, err
	}
	w := ensureMiku(p)
	if m := w.Maps[id]; m != nil {
		return m, nil
	}
	start := 0
	for key := range androidActivities["miku_nodes"] {
		nid, _ := strconv.Atoi(key)
		n := activityData("miku_nodes", nid)
		if n.integer("map_id") == id && n.integer("site_type") == 1 {
			if start > 0 {
				return nil, errors.New("初音地图有重复起点")
			}
			start = nid
		}
	}
	if start <= 0 {
		return nil, errors.New("初音地图起点不存在")
	}
	m := &MikuMap{ID: id, Nodes: map[int]*MikuNode{}, Current: start, Path: []int{}, Bonus: emptyActivityBox(), Treasure: map[string]int{"treasure_id": 0, "covering_count": 0}, Coins: map[int]int64{}, Stories: map[int]MikuStory{}, Handbook: MikuHandbook{ID: r.integer("handbook_id"), Items: map[int][]MikuHandbookItem{}}}
	for _, sid := range r.ids("story_list") {
		m.Stories[sid] = MikuStory{ID: sid, Finished: containsInt(p.ClearedDungeons, activityData("miku_story", sid).integer("dungeon_id"))}
	}
	var groups [][]int
	if json.Unmarshal(activityData("handbook", m.Handbook.ID)["handbook_content"], &groups) != nil || len(groups) == 0 || len(groups[0]) == 0 {
		return nil, errors.New("初音手册分组无效")
	}
	for i, group := range groups {
		items := []MikuHandbookItem{}
		for _, item := range group {
			state := 0
			if i == 0 && item == groups[0][0] {
				state = 1
			}
			items = append(items, MikuHandbookItem{item, state})
		}
		m.Handbook.Items[i] = items
	}
	for _, nid := range append([]int{start}, activityData("miku_nodes", start).ids("children")...) {
		n, err := initMikuNode(nid)
		if err != nil {
			return nil, err
		}
		m.Nodes[nid] = n
	}
	w.Maps[id] = m
	return m, nil
}
func checkMikuNodes(m *MikuMap, condition []int) bool {
	if len(condition) == 0 {
		return true
	}
	if len(condition) < 2 || (condition[0] != 1 && condition[0] != 2) {
		return false
	}
	matched := condition[0] == 1
	for _, id := range condition[1:] {
		n := m.Nodes[id]
		pass := n != nil && n.Passed
		if condition[0] == 1 {
			matched = matched && pass
		} else {
			matched = matched || pass
		}
	}
	return matched
}
func mikuStoryGate(p *Progress, sid int) (int, error) {
	r := activityData("miku_story", sid)
	if len(r) == 0 || p.Activities.Miku == nil {
		return 0, errors.New("初音剧情尚不可用")
	}
	mid := 0
	for key := range androidActivities["miku_map"] {
		id, _ := strconv.Atoi(key)
		if containsInt(activityData("miku_map", id).ids("story_list"), sid) {
			mid = id
			break
		}
	}
	m := p.Activities.Miku.Maps[mid]
	if m == nil {
		return 0, errors.New("初音剧情地图尚未进入")
	}
	if need := r.integer("storyline_condition"); need > 0 && !containsInt(p.ClearedDungeons, activityData("miku_story", need).integer("dungeon_id")) {
		return 0, errors.New("初音前置剧情未完成")
	}
	if cond := r.ids("progress_condition"); len(cond) > 0 {
		if len(cond) != 2 || p.Activities.Miku.Maps[cond[0]] == nil || p.Activities.Miku.Maps[cond[0]].Score < cond[1] {
			return 0, errors.New("初音探索进度不足")
		}
	}
	var groups map[int][]int
	if json.Unmarshal(r["node_condition"], &groups) != nil {
		return 0, errors.New("初音剧情节点条件无效")
	}
	for mode, ids := range groups {
		if !checkMikuNodes(m, append([]int{mode}, ids...)) {
			return 0, errors.New("初音前置探索节点未完成")
		}
	}
	return mid, nil
}
func activateMikuHandbook(p *Progress, mid, item int) error {
	w := p.Activities.Miku
	if w == nil || w.Maps[mid] == nil {
		return errors.New("初音手册地图未进入")
	}
	m := w.Maps[mid]
	r := activityData("handbook_item", item)
	group, index := -1, -1
	for g, rows := range m.Handbook.Items {
		for i, row := range rows {
			if row.ID == item {
				group, index = g, i
			}
		}
	}
	if group < 0 || m.Handbook.Items[group][index].State != 1 {
		return errors.New("初音手册项目未解锁或已激活")
	}
	if cond := r.ids("progress_condition"); len(cond) > 0 && (len(cond) != 2 || m.Score < cond[1]) {
		return errors.New("初音手册探索进度不足")
	}
	need := r.ids("need_materials")
	if len(need) != 2 || need[0] <= 0 || need[1] <= 0 {
		return errors.New("初音手册材料配置无效")
	}
	mat := p.Materials[need[0]]
	if mat.Count < int64(need[1]) {
		return errors.New("初音手册材料不足")
	}
	mat.Count -= int64(need[1])
	p.Materials[need[0]] = mat
	m.Handbook.Items[group][index].State = 2
	for _, child := range r.ids("unlock_handbook_items") {
		cr := activityData("handbook_item", child)
		condition := cr.ids("progress_condition")
		if len(condition) > 0 && (len(condition) != 2 || m.Score < condition[1]) {
			continue
		}
		for g, rows := range m.Handbook.Items {
			for i, row := range rows {
				if row.ID == child && row.State == 0 {
					m.Handbook.Items[g][i].State = 1
				}
			}
		}
	}
	return nil
}
func prepareMikuDungeon(p *Progress, bc *ActivityBattleContext, day, level int, now time.Time) error {
	// 剧情与诅咒依据原生映射，普通探索战斗还须当前已生成事件；不得按副本编号猜地图。
	for key := range androidActivities["miku_story"] {
		sid, _ := strconv.Atoi(key)
		if activityData("miku_story", sid).integer("dungeon_id") == bc.DungeonID {
			mid, err := mikuStoryGate(p, sid)
			if err != nil {
				return err
			}
			bc.MapID, bc.NodeID = mid, sid
			bc.Place = "story"
			return nil
		}
	}
	var pair []int
	if json.Unmarshal(androidActivities["miku_abyss_dungeon_node"][strconv.Itoa(bc.DungeonID)], &pair) == nil && len(pair) == 2 {
		node := pair[0]
		n := activityData("miku_nodes", node)
		mid := n.integer("map_id")
		w := p.Activities.Miku
		if w == nil || w.Maps[mid] == nil || !checkMikuNodes(w.Maps[mid], n.ids("unlock_conditions")) {
			return errors.New("初音诅咒节点未解锁")
		}
		bc.MapID, bc.NodeID = mid, node
		bc.Place = "curse"
		_ = json.Unmarshal(androidActivities["miku_dungeon_task"][strconv.Itoa(bc.DungeonID)], &bc.Tasks)
		return nil
	}
	for key := range androidActivities["miku_battle_events"] {
		id, _ := strconv.Atoi(key)
		r := activityData("miku_battle_events", id)
		if r.integer("normal_dungeon_id") == bc.DungeonID || r.integer("hard_dungeon_id") == bc.DungeonID {
			bc.Place = "explore"
			return nil
		}
	}
	return errors.New("初音副本缺少原生地图事件映射")
}
func finishMikuDungeon(p *Progress, bc *ActivityBattleContext, tasks []int, now time.Time, boxes ...map[string]any) error {
	defer reconcileMikuAchievements(p)
	w := p.Activities.Miku
	if w == nil || w.Maps[bc.MapID] == nil {
		return errors.New("初音结算地图丢失")
	}
	m := w.Maps[bc.MapID]
	if bc.Place == "explore" {
		if len(boxes) != 1 {
			return errors.New("初音探索奖励盒缺失")
		}
		return finishMikuExploration(p, bc, boxes[0], now)
	}
	if r := activityData("miku_story", bc.NodeID); r.integer("dungeon_id") == bc.DungeonID {
		story := m.Stories[bc.NodeID]
		story.Finished = true
		m.Stories[bc.NodeID] = story
		return nil
	}
	for _, task := range tasks {
		if !containsInt(bc.Tasks, task) {
			return errors.New("初音诅咒任务不在注入列表")
		}
		key := strconv.Itoa(bc.DungeonID) + ":" + strconv.Itoa(task)
		if _, ok := w.Tasks[key]; !ok {
			w.Tasks[key] = false
		}
	}
	return nil
}
func (s *Service) mikuRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	switch method {
	case "enter_miku_node", "leave_miku_map", "receive_miku_task_bonus", "receive_miku_like_song_bonus", "receive_miku_surprise_bonus", "receive_miku_achv_bonus":
		return s.mikuExplorationRPC(ctx, c, method, args)
	}
	lens := map[string]int{"enter_miku_map": 1, "activate_handbook_item": 2, "set_miku_auto_path": 3, "enter_story_dungeon": 2, "enter_curse_abyss_dungeon": 3}
	if len(args) != lens[method] {
		return nil, errors.New("初音接口参数数量无效")
	}
	var id, item, cb int
	if json.Unmarshal(args[0], &id) != nil {
		return nil, errors.New("初音编号无效")
	}
	if len(args) > 1 && json.Unmarshal(args[1], &item) != nil {
		return nil, errors.New("初音第二编号无效")
	}
	if method == "enter_curse_abyss_dungeon" {
		cb = id
		id = item
		if cb <= 0 || json.Unmarshal(args[2], &item) != nil {
			return nil, errors.New("初音诅咒副本参数无效")
		}
		var pair []int
		if json.Unmarshal(androidActivities["miku_abyss_dungeon_node"][strconv.Itoa(id)], &pair) != nil || len(pair) != 2 || activityData("miku_nodes", pair[0]).integer("map_id") != item {
			return nil, errors.New("初音诅咒地图不匹配")
		}
		return s.enterDungeon(ctx, c, []json.RawMessage{args[0], args[1], json.RawMessage("{}")})
	}
	if method == "enter_story_dungeon" {
		r := activityData("miku_story", item)
		if len(r) == 0 {
			return []Push{push("Avatar", "on_enter_story_dungeon", activityErrorCode(errors.New("剧情不存在")))}, nil
		}
		progress := c.SelectedAvatarUnsafe().Progress
		mid, e := mikuStoryGate(&progress, item)
		if e != nil || mid != id {
			return []Push{push("Avatar", "on_enter_story_dungeon", activityErrorCode(errors.New("剧情地图不匹配")))}, nil
		}
		out, err := s.enterDungeon(ctx, c, []json.RawMessage{json.RawMessage("0"), json.RawMessage(strconv.Itoa(r.integer("dungeon_id"))), json.RawMessage("{}")})
		if err != nil {
			return []Push{push("Avatar", "on_enter_story_dungeon", activityErrorCode(err))}, nil
		}
		filtered := []Push{push("Avatar", "on_enter_story_dungeon", RetSuccess)}
		for _, v := range out {
			if v.Method != "call_client_callback" {
				filtered = append(filtered, v)
			}
		}
		return filtered, nil
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		defer reconcileMikuAchievements(p)
		day, err := activityOpen(*p, 208, c.SelectedAvatarUnsafe().Info.Level, s.Now())
		if err != nil {
			return err
		}
		switch method {
		case "enter_miku_map":
			_, err := openMikuMap(p, id, day, c.SelectedAvatarUnsafe().Info.Level)
			return err
		case "activate_handbook_item":
			return activateMikuHandbook(p, id, item)
		case "set_miku_auto_path":
			var hard bool
			if json.Unmarshal(args[2], &hard) != nil {
				return errors.New("初音自动寻路模式必须为布尔值")
			}
			w := p.Activities.Miku
			if w == nil || w.Maps[id] == nil {
				return errors.New("初音寻路地图未进入")
			}
			m := w.Maps[id]
			if item != 0 {
				r := activityData("miku_roots", item)
				if r.integer("map_id") != id {
					return errors.New("初音自动路径地图不匹配")
				}
				for _, nid := range r.ids("recommend_path") {
					if activityData("miku_nodes", nid).integer("progress") > 0 {
						if n := m.Nodes[nid]; n == nil || !n.Passed {
							return errors.New("初音自动路径尚未完整探索")
						}
					}
				}
			}
			m.Auto, m.AutoHard = item, hard
			return nil
		}
		return errors.New("初音接口不存在")
	})
	switch method {
	case "enter_miku_map":
		if err != nil {
			return []Push{push("Avatar", "on_enter_miku_map", activityErrorCode(err), id)}, nil
		}
		return append(activityPushes(c, s.Now()), push("Avatar", "on_enter_miku_map", RetSuccess, id)), nil
	case "activate_handbook_item":
		if err != nil {
			return []Push{push("Avatar", "on_activate_handbook_item", activityErrorCode(err))}, nil
		}
		return append(activityPushes(c, s.Now()), materialManagerPush(c), push("Avatar", "on_activate_handbook_item", RetSuccess)), nil
	}
	return activityPushes(c, s.Now()), err
}
