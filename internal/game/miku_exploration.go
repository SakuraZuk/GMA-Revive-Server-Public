package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"
)

// 普通探索只接受服务端已生成、当前节点的相邻事件；客户端不能自行指定副本或奖励。
func mikuNodeContext(p *Progress, nodeID int, extra map[string]any) (*ActivityBattleContext, int, error) {
	r := activityData("miku_nodes", nodeID)
	w := p.Activities.Miku
	if len(r) == 0 || w == nil || w.Maps[r.integer("map_id")] == nil {
		return nil, 0, errors.New("初音节点地图尚未进入")
	}
	m := w.Maps[r.integer("map_id")]
	node := m.Nodes[nodeID]
	if node == nil || containsInt(m.Path, nodeID) || !containsInt(activityData("miku_nodes", m.Current).ids("children"), nodeID) || !checkMikuNodes(m, r.ids("unlock_conditions")) || r.integer("site_type") == 1 || r.integer("site_type") == 3 {
		return nil, 0, errors.New("初音节点不在可达前沿")
	}
	bc := &ActivityBattleContext{ActivityID: 208, MapID: m.ID, NodeID: nodeID, Trunk: m.Current, Cycle: len(m.Path), Resource: node.Event, Place: "explore"}
	if r.integer("site_type") == 2 {
		return bc, 0, nil
	}
	switch r.integer("event_type") {
	case 1:
		event := activityData("miku_battle_events", node.Event)
		normal, nok := extra["normal_battle"].(bool)
		hard, hok := extra["hard_battle"].(bool)
		angry, aok := extra["is_angry"].(bool)
		if len(event) == 0 || normal == hard || (!nok && !hok) || (!aok && extra["is_angry"] != nil) || (angry && !node.Angry) {
			return nil, 0, errors.New("初音战斗模式或愤怒资格无效")
		}
		bc.DungeonID = event.integer("normal_dungeon_id")
		if hard {
			bc.DungeonID = event.integer("hard_dungeon_id")
		}
		bc.Angry = angry
		return bc, 0, nil
	case 2:
		event := activityData("miku_blessing_events", node.Event)
		chosen := 0
		if len(event) == 0 || len(node.Bonuses) == 0 {
			return nil, 0, errors.New("初音奖励事件没有已生成选项")
		}
		if kind := event.integer("blessing_type"); kind == 1 || kind == 3 {
			raw, _ := json.Marshal(extra["choice_id"])
			var i int
			if json.Unmarshal(raw, &i) != nil || i < 0 || i >= len(node.Bonuses) {
				return nil, 0, errors.New("初音奖励选项无效")
			}
			chosen = node.Bonuses[i]
		} else if kind == 2 {
			weights := make([]int, len(node.Bonuses))
			for i := range weights {
				weights[i] = 1
			}
			i, e := activityWeighted(weights)
			if e != nil {
				return nil, 0, e
			}
			chosen = node.Bonuses[i]
		} else {
			return nil, 0, errors.New("初音祝福事件类型无效")
		}
		if event.integer("bonus_type") == 1 {
			if len(activityData("miku_treasure", chosen)) == 0 {
				return nil, 0, errors.New("初音秘宝不存在")
			}
			bc.Layer = chosen
			return bc, 0, nil
		}
		if event.integer("bonus_type") != 2 {
			return nil, 0, errors.New("初音祝福奖励类型无效")
		}
		return bc, chosen, nil
	}
	return nil, 0, errors.New("初音节点事件不存在")
}
func validateMikuBattleExtra(p *Progress, bc *ActivityBattleContext, extra map[string]any) error {
	var node int
	raw, _ := json.Marshal(extra["node_id"])
	if json.Unmarshal(raw, &node) != nil {
		return errors.New("初音探索必须由当前节点入口发起")
	}
	valid, _, e := mikuNodeContext(p, node, extra)
	if e != nil {
		return e
	}
	if valid.DungeonID != bc.DungeonID {
		return errors.New("初音探索事件副本不匹配")
	}
	bc.MapID, bc.NodeID, bc.Trunk, bc.Cycle, bc.Resource, bc.Angry = valid.MapID, valid.NodeID, valid.Trunk, valid.Cycle, valid.Resource, valid.Angry
	return nil
}
func mikuContextValid(p *Progress, bc *ActivityBattleContext) bool {
	w := p.Activities.Miku
	if w == nil {
		return false
	}
	m := w.Maps[bc.MapID]
	if m == nil {
		return false
	}
	node := m.Nodes[bc.NodeID]
	return node != nil && m.Current == bc.Trunk && len(m.Path) == bc.Cycle && node.Event == bc.Resource && !containsInt(m.Path, bc.NodeID) && containsInt(activityData("miku_nodes", m.Current).ids("children"), bc.NodeID)
}
func updateMikuHandbook(m *MikuMap) {
	unlocked := map[int]bool{}
	for _, group := range m.Handbook.Items {
		for _, item := range group {
			if item.State != 2 {
				continue
			}
			for _, child := range activityData("handbook_item", item.ID).ids("unlock_handbook_items") {
				r := activityData("handbook_item", child)
				cond := r.ids("progress_condition")
				if len(cond) == 0 || (len(cond) == 2 && m.Score >= cond[1]) {
					unlocked[child] = true
				}
			}
		}
	}
	for g, group := range m.Handbook.Items {
		for i, item := range group {
			if item.State == 0 && unlocked[item.ID] {
				m.Handbook.Items[g][i].State = 1
			}
		}
	}
}
func advanceMikuNode(p *Progress, bc *ActivityBattleContext) error {
	if !mikuContextValid(p, bc) {
		return errors.New("初音探索节点上下文已经变化")
	}
	w := ensureMiku(p)
	m := w.Maps[bc.MapID]
	n := m.Nodes[bc.NodeID]
	r := activityData("miku_nodes", bc.NodeID)
	if !n.Passed {
		amount := r.integer("progress")
		if amount < 0 || amount > 100 {
			return errors.New("初音节点进度无效")
		}
		m.Score += amount
		if m.Score > 100 {
			m.Score = 100
		}
		n.Passed = true
	}
	if len(m.Path) == 0 {
		w.Runs++
	}
	m.Current = bc.NodeID
	m.Path = append(m.Path, bc.NodeID)
	if r.integer("event_type") == 1 && m.Treasure["covering_count"] > 0 {
		m.Treasure["covering_count"]--
		if m.Treasure["covering_count"] == 0 {
			m.Treasure["treasure_id"] = 0
		}
	}
	if bc.Layer > 0 {
		m.Treasure = map[string]int{"treasure_id": bc.Layer, "covering_count": activityData("miku_treasure", bc.Layer).integer("cover_nodes")}
	}
	w.Visits[strconv.Itoa(r.integer("event_type"))+":"+strconv.Itoa(n.Event)] = 1
	for _, id := range r.ids("children") {
		old := m.Nodes[id]
		generated, e := initMikuNode(id)
		if e != nil {
			return e
		}
		if old != nil {
			generated.Passed = old.Passed
		}
		m.Nodes[id] = generated
	}
	updateMikuHandbook(m)
	return nil
}
func mikuReceiptBox(p *Progress, m *MikuMap) map[string]any {
	box := emptyActivityBox()
	materials := box["materials"].(map[int]int64)
	for id, n := range m.ReceiptMaterials {
		materials[id] = n
	}
	box["cards"] = cardListWire(p, m.ReceiptCards)
	runes := map[string]Rune{}
	for _, id := range m.ReceiptRunes {
		if r, ok := p.Runes[id]; ok {
			runes[id] = r
		}
	}
	if len(runes) > 0 {
		box["runes"] = runeMgrProperties(runes)
	}
	return box
}
func recordMikuBox(p *Progress, m *MikuMap, box map[string]any) error {
	if m.ReceiptMaterials == nil {
		m.ReceiptMaterials = map[int]int64{}
	}
	materials, ok := box["materials"].(map[int]int64)
	if !ok {
		return errors.New("初音探索奖励盒材料无效")
	}
	for id, n := range materials {
		if n < 0 || m.ReceiptMaterials[id] > math.MaxInt64-n {
			return errors.New("初音探索奖励累计溢出")
		}
		m.ReceiptMaterials[id] += n
		if containsInt(activityData("miku_args", 1).ids("miku_coin_materials"), id) {
			if m.Coins[id] > math.MaxInt64-n {
				return errors.New("初音探索币累计溢出")
			}
			m.Coins[id] += n
		}
	}
	if rows, ok := box["cards"].([]any); ok {
		for _, row := range rows {
			if r, ok := row.(map[string]any); ok {
				if id, ok := r["uuid"].(ObjectID); ok {
					m.ReceiptCards = append(m.ReceiptCards, string(id))
				}
			}
		}
	}
	if rows, ok := box["runes"].(map[string]any); ok {
		for id := range rows {
			m.ReceiptRunes = append(m.ReceiptRunes, id)
		}
	}
	m.Bonus = mikuReceiptBox(p, m)
	return nil
}
func applyMikuHandbookMaterials(p *Progress, m *MikuMap, box map[string]any, now time.Time) error {
	ratios := map[int]float64{}
	for _, group := range m.Handbook.Items {
		for _, item := range group {
			if item.State != 2 {
				continue
			}
			var pair []float64
			raw := activityData("handbook_item", item.ID)["material_addition_effect"]
			if len(raw) == 0 || string(raw) == "null" {
				continue
			}
			if json.Unmarshal(raw, &pair) != nil || len(pair) != 2 || pair[0] <= 0 || pair[1] < 0 {
				return errors.New("初音手册奖励加成配置无效")
			}
			ratios[int(pair[0])] += pair[1]
		}
	}
	materials := box["materials"].(map[int]int64)
	for mid, ratio := range ratios {
		base := materials[mid]
		if base == 0 {
			continue
		}
		increase := float64(base) * ratio
		if math.IsNaN(increase) || math.IsInf(increase, 0) || increase > math.MaxInt32 {
			return errors.New("初音手册奖励加成溢出")
		}
		n := int64(increase)
		if n <= 0 {
			continue
		}
		changes := map[int]int64{}
		cards := []string{}
		if e := grantNativeItem(p, mid, n, p.AvatarLevel, now, changes, &cards, 0); e != nil {
			return e
		}
		materials[mid] += n
	}
	return nil
}
func finishMikuExploration(p *Progress, bc *ActivityBattleContext, box map[string]any, now time.Time) error {
	if !mikuContextValid(p, bc) {
		return errors.New("初音战斗事件凭证已经变化")
	}
	m := p.Activities.Miku.Maps[bc.MapID]
	ids := []int{activityData("miku_nodes", bc.NodeID).integer("bonus_id")}
	if bc.Angry {
		ids = append(ids, activityData("miku_battle_events", bc.Resource).integer("extra_bonus_id"))
	}
	if tid := m.Treasure["treasure_id"]; tid > 0 && m.Treasure["covering_count"] > 0 && activityData("miku_treasure", tid).integer("effect_type") == 2 {
		ids = append(ids, activityData("dungeons", bc.DungeonID).integer("bonus"))
	}
	extra, e := grantActivityBonus(p, ids, now)
	if e != nil {
		return e
	}
	if e = mergeActivityBox(box, extra); e != nil {
		return e
	}
	if e = applyMikuHandbookMaterials(p, m, box, now); e != nil {
		return e
	}
	if e = recordMikuBox(p, m, box); e != nil {
		return e
	}
	if p.Battle != nil {
		seen := map[int]bool{}
		for _, uuid := range p.Battle.Team {
			_, card := findCard(p, uuid)
			if card != nil && !seen[card.CardID] {
				seen[card.CardID] = true
				p.Activities.Miku.Battles[card.CardID]++
			}
		}
	}
	err := advanceMikuNode(p, bc)
	if err == nil {
		reconcileMikuAchievements(p)
	}
	return err
}
func endMikuRun(p *Progress, m *MikuMap, fromEnd bool, now time.Time) ([]any, error) {
	if len(m.Path) == 0 {
		return nil, errors.New("初音没有进行中的探索")
	}
	coins := map[int]int64{}
	raise := map[int]int64{}
	for id, n := range m.Coins {
		coins[id] = n
	}
	if fromEnd {
		var ratio float64
		_ = json.Unmarshal(activityData("miku_args", 1)["finish_coin_raise_ratio"], &ratio)
		if ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			return nil, errors.New("初音终点加成比例无效")
		}
		for id, n := range coins {
			v := float64(n) * ratio / 100
			if v > math.MaxInt32 {
				return nil, errors.New("初音终点奖励溢出")
			}
			raise[id] = int64(v)
			if raise[id] > 0 {
				changes := map[int]int64{}
				cards := []string{}
				if e := grantNativeItem(p, id, raise[id], p.AvatarLevel, now, changes, &cards, 0); e != nil {
					return nil, e
				}
			}
		}
		p.Activities.Miku.Ends++
		if m.CompletedRuns == math.MaxInt32 {
			return nil, errors.New("初音完整探索次数溢出")
		}
		m.CompletedRuns++
	}
	box := mikuReceiptBox(p, m)
	path := append([]int{}, m.Path...)
	start := 0
	for key := range androidActivities["miku_nodes"] {
		id, _ := strconv.Atoi(key)
		r := activityData("miku_nodes", id)
		if r.integer("map_id") == m.ID && r.integer("site_type") == 1 {
			start = id
		}
	}
	if start <= 0 {
		return nil, errors.New("初音探索起点无效")
	}
	m.Current = start
	m.Path = []int{}
	m.Coins = map[int]int64{}
	m.ReceiptMaterials = map[int]int64{}
	m.ReceiptCards = nil
	m.ReceiptRunes = nil
	m.Bonus = emptyActivityBox()
	m.Treasure = map[string]int{"treasure_id": 0, "covering_count": 0}
	m.Auto = 0
	m.AutoHard = false
	for _, id := range append([]int{start}, activityData("miku_nodes", start).ids("children")...) {
		old := m.Nodes[id]
		n, e := initMikuNode(id)
		if e != nil {
			return nil, e
		}
		if old != nil {
			n.Passed = old.Passed
		}
		m.Nodes[id] = n
	}
	return []any{m.ID, coins, raise, box, fromEnd, path}, nil
}

func (s *Service) mikuExplorationRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	lengths := map[string]int{"enter_miku_node": 2, "leave_miku_map": 1, "receive_miku_task_bonus": 3, "receive_miku_like_song_bonus": 0, "receive_miku_surprise_bonus": 1, "receive_miku_achv_bonus": 1}
	if len(args) != lengths[method] {
		return nil, errors.New("初音探索接口参数数量无效")
	}
	id, item, cb := 0, 0, 0
	extra := map[string]any{}
	if len(args) > 0 && json.Unmarshal(args[0], &id) != nil {
		return nil, errors.New("初音探索编号无效")
	}
	if method == "receive_miku_task_bonus" {
		cb = id
		if cb <= 0 || json.Unmarshal(args[1], &id) != nil || json.Unmarshal(args[2], &item) != nil {
			return nil, errors.New("初音任务领奖参数无效")
		}
	}
	if method == "enter_miku_node" {
		if json.Unmarshal(args[1], &extra) != nil || extra == nil {
			return nil, errors.New("初音探索附加参数无效")
		}
		p := c.SelectedAvatarUnsafe().Progress
		bc, _, e := mikuNodeContext(&p, id, extra)
		if e == nil && bc.DungeonID > 0 {
			extra["node_id"] = id
			raw, _ := json.Marshal(extra)
			out, e := s.enterDungeon(ctx, c, []json.RawMessage{json.RawMessage("0"), json.RawMessage(strconv.Itoa(bc.DungeonID)), raw})
			if e != nil {
				return []Push{push("Avatar", "on_enter_miku_node", activityErrorCode(e))}, nil
			}
			ret := RetSuccess
			filtered := []Push{}
			for _, v := range out {
				if v.Method == "call_client_callback" {
					if len(v.Args) > 1 {
						if result, ok := v.Args[1].([]any); ok && len(result) > 0 {
							if n, ok := result[0].(int); ok {
								ret = n
							}
						}
					}
				} else {
					filtered = append(filtered, v)
				}
			}
			return append([]Push{push("Avatar", "on_enter_miku_node", ret)}, filtered...), nil
		}
	}
	box := emptyActivityBox()
	treasure := 0
	var ended []any
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		defer reconcileMikuAchievements(p)
		if _, e := activityOpen(*p, 208, p.AvatarLevel, s.Now()); e != nil {
			return e
		}
		w := ensureMiku(p)
		if p.Battle != nil && !p.Battle.Finished && p.Battle.ActivityContext != nil && p.Battle.ActivityContext.ActivityID == 208 {
			return errors.New("初音当前战斗尚未结束")
		}
		switch method {
		case "enter_miku_node":
			bc, chosen, e := mikuNodeContext(p, id, extra)
			if e != nil {
				return e
			}
			m := w.Maps[bc.MapID]
			if activityData("miku_nodes", id).integer("site_type") == 2 {
				if e = advanceMikuNode(p, bc); e != nil {
					return e
				}
				ended, e = endMikuRun(p, m, true, s.Now())
				return e
			}
			if bc.DungeonID > 0 {
				return errors.New("初音战斗节点必须建立战斗会话")
			}
			box, e = grantActivityBonus(p, []int{activityData("miku_nodes", id).integer("bonus_id"), chosen}, s.Now())
			if e != nil {
				return e
			}
			if e = applyMikuHandbookMaterials(p, m, box, s.Now()); e != nil {
				return e
			}
			if e = recordMikuBox(p, m, box); e != nil {
				return e
			}
			treasure = bc.Layer
			return advanceMikuNode(p, bc)
		case "leave_miku_map":
			m := w.Maps[id]
			if m == nil {
				return errors.New("初音地图未进入")
			}
			var e error
			ended, e = endMikuRun(p, m, false, s.Now())
			return e
		case "receive_miku_like_song_bonus":
			if w.Like {
				return errors.New("初音歌曲奖励已领取")
			}
			var e error
			box, e = grantActivityBonus(p, []int{activityData("miku_args", 1).integer("like_song_bonus_id")}, s.Now())
			if e != nil {
				return e
			}
			w.Like = true
			return nil
		case "receive_miku_task_bonus":
			key := strconv.Itoa(id) + ":" + strconv.Itoa(item)
			value, ok := w.Tasks[key]
			if !ok || value {
				return errors.New("初音任务未完成或已领取")
			}
			var allowed []int
			_ = json.Unmarshal(androidActivities["miku_dungeon_task"][strconv.Itoa(id)], &allowed)
			if !containsInt(allowed, item) {
				return errors.New("初音任务不属于该副本")
			}
			var e error
			box, e = grantActivityBonus(p, []int{activityData("tower_task", item).integer("bonus_id")}, s.Now())
			if e != nil {
				return e
			}
			w.Tasks[key] = true
			return nil
		case "receive_miku_surprise_bonus":
			var e error
			box, e = receiveMikuLocalSurprise(p, id, s.Now())
			return e
		case "receive_miku_achv_bonus":
			reconcileMikuAchievements(p)
			a, ok := w.Achievements[id]
			r := activityData("miku_achv", id)
			if !ok || a.Claimed || mikuAchievementCount(a, r) < r.integer("target_need_count") {
				return errors.New("初音成就未完成或已领取")
			}
			var e error
			box, e = grantActivityBonus(p, []int{r.integer("bonus_id")}, s.Now())
			if e != nil {
				return e
			}
			a.Claimed = true
			w.Achievements[id] = a
			return nil
		}
		return errors.New("初音探索接口未知")
	})
	ret := RetSuccess
	if err != nil {
		ret = activityErrorCode(err)
		box = emptyActivityBox()
	}
	out := []Push{}
	if err == nil {
		out = append(out, activityPushes(c, s.Now())...)
		out = append(out, materialManagerPush(c), cardMgrPush(c), runePush(c), knowledgePush(c))
	}
	if ended != nil && err == nil {
		out = append(out, push("Avatar", "on_end_miku_map", ended...))
	}
	switch method {
	case "enter_miku_node":
		out = append(out, push("Avatar", "on_enter_miku_node", ret))
		if err == nil && ended == nil {
			out = append(out, push("Avatar", "on_finish_miku_node", box, treasure, id))
		}
	case "leave_miku_map":
		out = append(out, push("Avatar", "on_leave_miku_map", ret))
	case "receive_miku_task_bonus":
		out = append(out, Callback(cb, []any{ret, box}))
	case "receive_miku_achv_bonus":
		// 本版上行叫bonus，下行注册却叫reward，不能用on_拼接。
		out = append(out, push("Avatar", "on_receive_miku_achv_reward", ret, box))
	default:
		out = append(out, push("Avatar", "on_"+method, ret, box))
	}
	return out, nil
}

// 战斗结果先推battle_result，再通知原生探索UI刷新；root可在共享结算推送末尾调用。
func activitySettlementPushes(c *Connection, now time.Time) []Push {
	p := c.SelectedAvatarUnsafe().Progress
	b := p.Battle
	if b == nil || !b.Finished || b.Outcome != "win" || b.ActivityContext == nil {
		return nil
	}
	bc := b.ActivityContext
	if bc.ActivityID != 208 || bc.Place != "explore" {
		return nil
	}
	return []Push{push("Avatar", "on_finish_miku_node", activityWire(b.SettlementBox), 0, bc.NodeID)}
}
