package game

import (
	"context"
	"encoding/json"
	"fmt"
	"hs-server/internal/hotfix"
	"hs-server/internal/mobileproto"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func remainingRPC(t *testing.T, s *Service, c *Connection, method string, values ...any) []Push {
	t.Helper()
	args := []json.RawMessage{}
	for _, v := range values {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		args = append(args, raw)
	}
	out, err := s.Handle(context.Background(), c, method, args)
	if err != nil {
		t.Fatal(method, err)
	}
	return out
}

func TestRemainingConsignDayNightRefreshAndExactPointRecovery(t *testing.T) {
	s, c, now := remainingFixture(t)
	p := c.SelectedAvatarUnsafe().Progress
	if len(p.RemainingGameplay.Consign.Tasks) != 6 {
		t.Fatal("原设施一级每日任务数量错误")
	}
	if pool, err := consignPoolAt(p, 1, time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)); err != nil || pool != 9 {
		t.Fatal("北京时间20点没有夜池", pool, err)
	}
	if pool, err := consignPoolAt(p, 1, time.Date(2026, 10, 8, 21, 0, 0, 0, time.UTC)); err != nil || pool != 3 {
		t.Fatal("次日5点没有日池", pool, err)
	}
	for i := 0; i < 5; i++ {
		ids := []int{}
		for id := range c.SelectedAvatarUnsafe().Progress.RemainingGameplay.Consign.Tasks {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		out := remainingRPC(t, s, c, "refresh_consign_task_one", 9, ids[0])
		cb := remainingFindPush(out, "call_client_callback")
		if cb == nil || cb.Args[1].([]any)[0] != RetSuccess {
			t.Fatal("真实刷新失败", out)
		}
	}
	p = c.SelectedAvatarUnsafe().Progress
	if p.RemainingGameplay.Consign.RefreshPoint != 0 {
		t.Fatal("5次刷新没有消耗5000")
	}
	id := 0
	for key := range p.RemainingGameplay.Consign.Tasks {
		id = key
		break
	}
	before, _ := json.Marshal(p)
	out := remainingRPC(t, s, c, "refresh_consign_task_one", 9, id)
	cb := remainingFindPush(out, "call_client_callback")
	if cb.Args[1].([]any)[0] != 4012 {
		t.Fatal("无刷新点原生错误码错误", cb)
	}
	after, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
	if string(before) != string(after) {
		t.Fatal("刷新不足改变资产或任务")
	}
	if err := s.updateProgress(context.Background(), c, func(p *Progress) error {
		roomID := androidCollection.Facilities[5].Room
		room := newCollectionRoom(roomID, *now)
		room.Slots = map[int]int{1: 4401}
		room.Cards = map[int]int{4401: 1}
		p.Collection.Rooms[roomID] = room
		return ensureRemainingGameplay(p, *now)
	}); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 3600; n++ {
		*now = now.Add(time.Second)
		if err := s.updateProgress(context.Background(), c, func(p *Progress) error { return ensureRemainingGameplay(p, *now) }); err != nil {
			t.Fatal(err)
		}
	}
	p = c.SelectedAvatarUnsafe().Progress
	if p.RemainingGameplay.Consign.RefreshPoint != 260 || p.RemainingGameplay.Consign.PointRemainder != 0 {
		t.Fatal("1住客按260/小时恢复出现取整漂移", p.RemainingGameplay.Consign)
	}
	raw, _ := json.Marshal(p)
	var restored Progress
	if json.Unmarshal(raw, &restored) != nil || !reflect.DeepEqual(p.RemainingGameplay.Consign, restored.RemainingGameplay.Consign) {
		t.Fatal("委托状态重登变化")
	}
}

func TestRemainingTaskTowerForeignTasksRollbackAndExplorationDoubleCost(t *testing.T) {
	s, c, _ := remainingFixture(t)
	remainingRPC(t, s, c, "enter_task_tower", 10, 101, 1, map[string]any{})
	b := c.SelectedAvatarUnsafe().Progress.Battle
	p := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	before, _ := json.Marshal(p)
	if err := settleTaskTower(&p, b, true, []int{999999}, emptyActivityBox(), s.Now()); err == nil {
		t.Fatal("绝密外国任务被奖励")
	}
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("非法绝密任务改变资产或成就")
	}
	if err := s.updateProgress(context.Background(), c, func(p *Progress) error { p.Battle = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	remainingRPC(t, s, c, "enter_free_stage", 10101, true)
	remainingRPC(t, s, c, "unlock_chapter", 2, 2)
	if c.SelectedAvatarUnsafe().Progress.UnlockedChapters[2] != 2 {
		t.Fatal("原生章节已看标记未持久化")
	}
	p = c.SelectedAvatarUnsafe().Progress
	node := 0
	for id, site := range p.RemainingGameplay.Stages[10101].Sites {
		if site.State == 1 {
			node = id
			break
		}
	}
	site := p.RemainingGameplay.Stages[10101].Sites[node]
	did := freeStageSiteDungeon(site)
	power := p.Power.Value
	remainingRPC(t, s, c, "set_free_stage_power", 9, 1)
	out := remainingRPC(t, s, c, "set_auto_free_stage_power", 9, map[int]int{node: 2})
	cb := remainingFindPush(out, "call_client_callback")
	if cb == nil || cb.Args[1].([]any)[0] != RetSuccess {
		t.Fatal("原生以节点为键的自动倍率被拒绝")
	}
	remainingRPC(t, s, c, "set_auto_list", []int{node})
	remainingRPC(t, s, c, "set_auto_fighting", true)
	if !reflect.DeepEqual(c.SelectedAvatarUnsafe().Progress.RemainingGameplay.AutoList, []int{node}) {
		t.Fatal("原生节点路径没有保存")
	}
	remainingRPC(t, s, c, "enter_stage_site", node, 0)
	b = c.SelectedAvatarUnsafe().Progress.Battle
	if b == nil || b.PaidPower != int64(dungeonCatalog[did].Power*2) || c.SelectedAvatarUnsafe().Progress.Power.Value != power-dungeonCatalog[did].Power*2 || remainingDungeonRewardAmount(b, true) != 2 {
		t.Fatal("探索双倍没有使用实际冻结倍率", b)
	}
	remainingFinishBattle(t, s, c, nil, false)
	p = c.SelectedAvatarUnsafe().Progress
	if p.RemainingGameplay.Stages[10101].Sites[node].State != 1 || p.Achievements[201001].Time != 0 {
		t.Fatal("探索失败提前完成节点或成就")
	}
	// 原盒JSON重载需保留整数键和超2^53精度，不能把累计材料转换成字符串键。
	st := p.RemainingGameplay.Stages[10101]
	st.Bonus = emptyActivityBox()
	st.Bonus["materials"] = map[int]int64{12: 9007199254740993}
	raw, _ := json.Marshal(st)
	var restored FreeStageState
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	wire := freeStageStatesWire(map[int]*FreeStageState{10101: &restored}).(mobileproto.Map)
	box := wire[0].Value.(map[string]any)["cur_bonus_box"].(map[string]any)
	materials := box["materials"].(mobileproto.Map)
	if fmt.Sprint(materials[0].Value) != "9007199254740993" {
		t.Fatal("探索累计材料整数精度丢失", materials)
	}
}
func remainingFindPush(out []Push, method string) *Push {
	for i := range out {
		if out[i].Method == method {
			return &out[i]
		}
	}
	return nil
}
func remainingHasProp(out []Push, name string) bool {
	for _, p := range out {
		if p.Method == "client_prop_changed" && len(p.Args) == 1 {
			if pair, ok := p.Args[0].([]any); ok && len(pair) == 2 && pair[0] == name {
				return true
			}
		}
	}
	return false
}
func remainingFixture(t *testing.T) (*Service, *Connection, *time.Time) {
	t.Helper()
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	t.Setenv("HS_RUNE_FALLBACK_POOLS", approvedGameplayValue(t, "HS_RUNE_FALLBACK_POOLS"))
	now := time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC)
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	s.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, context.Background(), a, s)
	if err := s.updateProgress(context.Background(), c, func(p *Progress) error {
		p.AvatarLevel = 60
		p.GuideTasks = map[int]GuideTask{}
		p.Power = Power{Value: 10000, Max: 10000, Interval: 300, PerValue: 1, LastTime: float64(now.Unix())}
		p.ClearedDungeons = []int{514, 619, 714, 118, 213, 319, 417, 814, 911}
		for _, last := range []int{514, 619, 714, 118, 213, 319, 417, 814, 911} {
			for id := last/100*100 + 1; id <= last; id++ {
				if !containsInt(p.ClearedDungeons, id) {
					p.ClearedDungeons = append(p.ClearedDungeons, id)
				}
			}
		}
		p.Cards = []Card{newCard(4401, 1, now)}
		p.Lineup = []string{p.Cards[0].UUID}
		p.UnlockSystems["house_character_tag"] = 1
		if err := ensureCollection(p, now); err != nil {
			return err
		}
		p.Collection.Facilities[5] = CollectionFacility{ID: 5, Level: 1}
		p.RemainingGameplay = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// 委托初始状态必须来自真实quick_login事务，禁止夹具手动ensure掩盖漏接。
	catalog := &hotfix.Catalog{}
	if err := catalog.Load(filepath.Join("..", "..", "deploy", "data", "hotfix.json")); err != nil {
		t.Fatal(err)
	}
	s.Hotfix = catalog
	c = NewConnection()
	pushes := remainingRPC(t, s, c, "quick_login", ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if len(pushes) != 3 || pushes[0].Method != "login_result" || pushes[0].Args[0] != RetSuccess || c.phase != Authenticated {
		t.Fatal("真实登录委托初始化失败", pushes)
	}
	if _, err := s.BecomePlayer(c); err != nil {
		t.Fatal(err)
	}
	if c.SelectedAvatarUnsafe().Progress.RemainingGameplay == nil || c.SelectedAvatarUnsafe().Progress.RemainingGameplay.Consign == nil {
		t.Fatal("真实登录未持久化委托容器")
	}
	return s, c, &now
}
func remainingFinishBattle(t *testing.T, s *Service, c *Connection, tasks []int, win bool) []Push {
	t.Helper()
	if tasks == nil {
		tasks = []int{}
	}
	remainingRPC(t, s, c, "load_entity_finish")
	remainingRPC(t, s, c, "battle_fighting", map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})
	b := c.SelectedAvatarUnsafe().Progress.Battle
	if b == nil {
		t.Fatal("实际战斗入口未产生会话")
	}
	uuid := b.UUID
	observeBattleEvent(t, context.Background(), s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":1,"kind":"ready","data":{"version":1}}`, uuid))
	observeBattleEvent(t, context.Background(), s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":2,"kind":"started","data":{"units":[]}}`, uuid))
	winners := []string{}
	if win {
		winners = append(winners, fmt.Sprintf("%x", selectedOID(c)))
	}
	raw, _ := json.Marshal(map[string]any{"battle_uuid": uuid, "sequence": 3, "kind": "result", "data": map[string]any{"winner_eids": winners, "finished_task_list": tasks}})
	return observeBattleEvent(t, context.Background(), s, c, string(raw))
}
func TestRemainingTaskTowerNativeEntranceActual15TasksEachAndPersistence(t *testing.T) {
	s, c, _ := remainingFixture(t)
	for _, tower := range []int{101, 102} {
		for floor := 1; floor <= 5; floor++ {
			remainingRPC(t, s, c, "enter_task_tower", 10, tower, floor, map[string]any{"tower_task_list": []int{999999}, "server_task_tower": []int{999, 999}})
			b := c.SelectedAvatarUnsafe().Progress.Battle
			if b == nil {
				t.Fatal("绝密入口没有建立真实副本")
			}
			var frozen []int
			raw, _ := json.Marshal(b.Extra["tower_task_list"])
			if json.Unmarshal(raw, &frozen) != nil || len(frozen) != 3 || containsInt(frozen, 999999) {
				t.Fatal("未权威冻结本层三任务", b.Extra)
			}
			out := remainingFinishBattle(t, s, c, frozen, true)
			if battleTestResult(out) == nil {
				t.Fatal("绝密没有原生真实结算")
			}
			p := c.SelectedAvatarUnsafe().Progress
			state := p.RemainingGameplay.TaskTowers[tower].Floors[floor]
			if len(state.Finished) != 3 || len(state.Received) != 3 || p.RemainingGameplay.PendingTaskDungeon != 0 {
				t.Fatal("任务完成奖励或授权消费错误", state)
			}
			before, _ := json.Marshal(p)
			b = p.Battle
			winners := fmt.Sprintf(`["%x"]`, selectedOID(c))
			observeBattleEvent(t, context.Background(), s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":4,"kind":"result","data":{"winner_eids":%s,"finished_task_list":%s}}`, b.UUID, winners, raw))
			after, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
			if string(before) != string(after) {
				t.Fatal("重复绝密result重复发奖")
			}
		}
		id := 208103
		if tower == 102 {
			id = 208106
		}
		p := c.SelectedAvatarUnsafe().Progress
		if achievementCount(p.Achievements[id], androidAchievements.Rules[id]) != 15 || p.Achievements[id].Time == 0 {
			t.Fatal("绝密15个真实任务没有驱动对应成就", id, p.Achievements[id])
		}
	}
	p := c.SelectedAvatarUnsafe().Progress
	raw, _ := json.Marshal(p)
	var restored Progress
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.RemainingGameplay.TaskTowers, restored.RemainingGameplay.TaskTowers) {
		t.Fatal("绝密重登任务或领奖记录丢失")
	}
}
func TestRemainingExplorationAllNineChaptersRealNodeProgressAndAchievement(t *testing.T) {
	s, c, _ := remainingFixture(t)
	ids := []int{10101, 10201, 10301, 10401, 10501, 10601, 10701, 10801, 10901}
	for _, stageID := range ids {
		remainingRPC(t, s, c, "enter_free_stage", stageID, true)
		p := c.SelectedAvatarUnsafe().Progress
		if p.RemainingGameplay.CurrentStage != stageID {
			t.Fatal("探索真实章节入口被拒绝", stageID)
		}
		ended := false
		for steps := 0; steps < 20; steps++ {
			st := c.SelectedAvatarUnsafe().Progress.RemainingGameplay.Stages[stageID]
			if st.State == 3 {
				ended = true
				break
			}
			available := []int{}
			for id, site := range st.Sites {
				if site.State == 1 {
					available = append(available, id)
				}
			}
			sort.Ints(available)
			if len(available) == 0 {
				t.Fatal("探索出现无可探索节点", stageID)
			}
			node := available[0]
			did := freeStageSiteDungeon(st.Sites[node])
			remainingRPC(t, s, c, "enter_stage_site", node, 0)
			if did > 0 {
				out := remainingFinishBattle(t, s, c, nil, true)
				if battleTestResult(out) == nil {
					t.Fatal("探索战斗未真实结算")
				}
				if remainingFindPush(out, "on_finish_stage_site") == nil {
					t.Fatal("探索战斗胜利没有原生节点完成推送")
				}
			}
			if c.SelectedAvatarUnsafe().Progress.RemainingGameplay.Stages[stageID].Sites[node].State != 3 {
				t.Fatal("真实节点未完成", stageID, node)
			}
		}
		if !ended {
			t.Fatal("探索未到实际boss终点", stageID)
		}
		id := 201001 + (stageID-10101)/100
		p = c.SelectedAvatarUnsafe().Progress
		if p.Achievements[id].Time == 0 || achievementCount(p.Achievements[id], androidAchievements.Rules[id]) != 1 {
			t.Fatal("章节完成未驱动真实成就", stageID, id, p.Achievements[id])
		}
	}
	before := remainingGameplayProperties(c.SelectedAvatarUnsafe().Progress)
	raw, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
	var restored Progress
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	after := remainingGameplayProperties(restored)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("探索累计盒或节点重载协议变化")
	}
}
func TestRemainingConsignNativeFullLifecyclePhaseFreezeAndTwoAchievements(t *testing.T) {
	s, c, now := remainingFixture(t)
	for n := 0; n < 100; n++ {
		var id int
		if err := s.updateProgress(context.Background(), c, func(p *Progress) error {
			w := p.RemainingGameplay.Consign
			ids := []int{}
			for key, task := range w.Tasks {
				if task.StartTime == 0 {
					ids = append(ids, key)
				}
			}
			sort.Ints(ids)
			if len(ids) == 0 {
				*now = now.Add(24 * time.Hour)
				if err := ensureRemainingGameplay(p, *now); err != nil {
					return err
				}
				for key, task := range w.Tasks {
					if task.StartTime == 0 {
						ids = append(ids, key)
					}
				}
				sort.Ints(ids)
			}
			id = ids[0]
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		out := remainingRPC(t, s, c, "start_consign_task", 10, id, []int{4401})
		if out[len(out)-1].Args[1].([]any)[0] != RetSuccess {
			t.Fatal("真实委托开始失败", out[len(out)-1])
		}
		task := c.SelectedAvatarUnsafe().Progress.RemainingGameplay.Consign.Tasks[id]
		if task.FinishTime <= task.StartTime {
			t.Fatal("委托未冻结时间")
		}
		if n == 0 {
			before, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
			remainingRPC(t, s, c, "commit_consign_task", id)
			after, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
			if string(before) != string(after) {
				t.Fatal("未完成委托提交改变资产")
			}
		}
		*now = time.Unix(task.FinishTime, 0)
		out = remainingRPC(t, s, c, "commit_consign_task", id)
		if remainingFindPush(out, "commit_consign_task_success") == nil {
			t.Fatal("真实委托结算失败", out[len(out)-1])
		}
		if !remainingHasProp(out, "achves") || !remainingHasProp(out, "achv_value") {
			t.Fatal("实际委托完成没有当场同步成就和积分")
		}
		p := c.SelectedAvatarUnsafe().Progress
		if p.RemainingGameplay.Consign.Tasks[id] != nil || len(p.RemainingGameplay.Consign.PhaseRecv) == 0 {
			t.Fatal("委托奖励或阶段真实收据未保存")
		}
		before, _ := json.Marshal(p)
		remainingRPC(t, s, c, "commit_consign_task", id)
		after, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
		if string(before) != string(after) {
			t.Fatal("重复委托提交改变资产")
		}
	}
	p := c.SelectedAvatarUnsafe().Progress
	for _, id := range []int{205001, 205004} {
		if p.Achievements[id].Time == 0 {
			t.Fatal("100次真实委托未驱动成就", id, p.Achievements[id])
		}
	}
	pending := p.RemainingGameplay.Consign.PhaseRecv
	counts := map[int]int64{}
	for mid := range pending {
		counts[mid] = p.Materials[mid].Count
	}
	out := remainingRPC(t, s, c, "reward_consign_phase_bonus")
	last := *remainingFindPush(out, "on_reward_consign_phase_bonus")
	if last.Method != "on_reward_consign_phase_bonus" || last.Args[0] != RetSuccess || last.Args[1].(map[string]any)["__custom_type"] != "bonus.bonus" {
		t.Fatal("阶段奖原生bonus类型不正确", last)
	}
	p = c.SelectedAvatarUnsafe().Progress
	for mid, n := range pending {
		if p.Materials[mid].Count != counts[mid]+n {
			t.Fatal("阶段真实累计奖励未入账", mid)
		}
	}
	if len(p.RemainingGameplay.Consign.PhaseRecv) != 0 {
		t.Fatal("阶段已领未消费")
	}
	before, _ := json.Marshal(p)
	remainingRPC(t, s, c, "reward_consign_phase_bonus")
	after, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
	if string(before) != string(after) {
		t.Fatal("重复阶段领奖改变资产")
	}
}

// 冷登录必须保留首批候选和生成等级；policy关闭不得自动建立新运营状态。
func TestRemainingConsignActualQuickLoginInitializationAndColdRetry(t *testing.T) {
	s, c, _ := remainingFixture(t)
	w := c.SelectedAvatarUnsafe().Progress.RemainingGameplay.Consign
	if len(w.Tasks) != 6 || w.RefreshPoint != 5000 || w.GenerationCount != 1 {
		t.Fatal("真实首次登录未冻结原设施6候选与初始点", w)
	}
	for _, task := range w.Tasks {
		if task.Level != 60 || task.StartTime != 0 || task.FinishTime != 0 {
			t.Fatal("真实登录候选生成等级/初始时间错误", task)
		}
	}
	stored, err := s.Accounts.(*FixtureAccounts).UpdateProgress(context.Background(), selectedOID(c), func(*Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(w, stored.RemainingGameplay.Consign) {
		t.Fatal("登录仅内存初始化而未真实保存", err)
	}
	before, _ := json.Marshal(w)
	cold := NewConnection()
	remainingRPC(t, s, cold, "quick_login", ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	after, _ := json.Marshal(cold.SelectedAvatarUnsafe().Progress.RemainingGameplay.Consign)
	if string(before) != string(after) {
		t.Fatal("同日真实冷登录重抽或重置初始委托")
	}
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "")
	if _, err := s.Accounts.(*FixtureAccounts).UpdateProgress(context.Background(), selectedOID(c), func(p *Progress) error { p.RemainingGameplay = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	cold = NewConnection()
	pushes := remainingRPC(t, s, cold, "quick_login", ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if pushes[0].Args[0] != RetSuccess || cold.SelectedAvatarUnsafe().Progress.RemainingGameplay != nil {
		t.Fatal("默认关闭登录自动启用运营委托", pushes)
	}
}
