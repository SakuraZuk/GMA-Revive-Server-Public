package dbstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"hs-server/internal/game"
)

func pgRemainingRPC(t *testing.T, s *game.Service, c *game.Connection, method string, args ...any) []game.Push {
	t.Helper()
	out, err := s.Handle(context.Background(), c, method, pgSocialArgs(args...))
	if err != nil {
		t.Fatal(method, err)
	}
	return out
}
func pgRemainingPush(out []game.Push, method string) *game.Push {
	for i := range out {
		if out[i].Method == method {
			return &out[i]
		}
	}
	return nil
}
func pgRemainingFinish(t *testing.T, s *game.Service, c *game.Connection, oid []byte, tasks []int) {
	t.Helper()
	pgRemainingRPC(t, s, c, "load_entity_finish")
	pgRemainingRPC(t, s, c, "battle_fighting", map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})
	av, _ := c.SelectedAvatar()
	uuid := av.Progress.Battle.UUID
	for seq, kind := range []string{"ready", "started", "result"} {
		data := map[string]any{"version": 1}
		if kind == "started" {
			data = map[string]any{"units": []any{}}
		}
		if kind == "result" {
			data = map[string]any{"winner_eids": []string{fmt.Sprintf("%x", oid)}, "finished_task_list": tasks}
		}
		env := map[string]any{"battle_uuid": uuid, "sequence": seq + 1, "kind": kind, "data": data}
		out := pgRemainingRPC(t, s, c, "do_command", "__battle_event__", []any{env})
		if kind == "result" && pgRemainingPush(out, "battle_result") == nil {
			t.Fatal("真实库原生战斗结果未提交", out)
		}
	}
}

func TestPostgresRemainingConsignTaskTowerAndExplorationAtomicReload(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	store := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	s := pgActivityService(t, store, func() time.Time { return now })
	info := game.ClientInfo{Account: "新玩法真实持久事务", Password: "pw", Hostnum: 10001}
	identity, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	oid := identity.Avatars[0].OID
	// 先完成真实创建资料RPC，不能先清Guide1000而留下nickname_set=false。
	// enterDungeon要求真实avatars命名标志；高级玩法数据基线不代替创建状态。
	creation := pgActivityLogin(t, s, info)
	if creationAvatar, ok := creation.SelectedAvatar(); !ok || creationAvatar.Progress.GuideTasks[1000].Status != 1 || creationAvatar.NicknameSet {
		t.Fatal("新账号未处于真实原生创建取名节点")
	}
	naming := pgRemainingRPC(t, s, creation, "set_nickname_gender", 19, "新玩法事务", 1)
	callback := pgRemainingPush(naming, "call_client_callback")
	if callback == nil || len(callback.Args) != 2 || callback.Args[0] != 19 {
		t.Fatal("真实创建取名回调缺失", naming)
	}
	result, ok := callback.Args[1].([]any)
	if !ok || len(result) != 1 || result[0] != game.RetSuccess {
		t.Fatal("真实创建取名失败", naming)
	}
	named := pgHumanRead(t, New(store.pool), oid)
	if !named.NicknameSet || named.Info.Nickname != "新玩法事务" || named.Gender != 1 {
		t.Fatal("真实创建资料未同时冷持久到avatars")
	}
	if _, err = store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		if err := game.AdvanceGuide(p, 1000, now, 10001); err != nil {
			return err
		}
		p.GuideTasks = map[int]game.GuideTask{}
		p.AvatarLevel = 60
		p.RemainingGameplay = nil
		p.Power = game.Power{Value: 10000, Max: 10000, Interval: 300, PerValue: 1, LastTime: float64(now.Unix())}
		p.ClearedDungeons = []int{514, 619, 714, 118, 213, 319, 417, 814, 911}
		for _, last := range []int{514, 619, 714, 118, 213, 319, 417, 814, 911} {
			for id := last/100*100 + 1; id <= last; id++ {
				p.ClearedDungeons = append(p.ClearedDungeons, id)
			}
		}
		p.Collection = &game.CollectionState{Rooms: map[int]game.CollectionRoom{}, Facilities: map[int]game.CollectionFacility{5: {ID: 5, Level: 1}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	c := pgActivityLogin(t, s, info)
	p := pgHumanRead(t, New(store.pool), oid).Progress
	if p.RemainingGameplay == nil || p.RemainingGameplay.Consign == nil {
		t.Fatal("真实quick_login事务没有保存委托初始化状态")
	}
	ids := []int{}
	for id := range p.RemainingGameplay.Consign.Tasks {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	if len(ids) != 6 || len(p.Cards) == 0 {
		t.Fatal("真实登录未初始化原生委托或幻书")
	}
	id := ids[0]
	pgRemainingRPC(t, s, c, "start_consign_task", 20, id, []int{p.Cards[0].CardID})
	p = pgHumanRead(t, New(store.pool), oid).Progress
	task := p.RemainingGameplay.Consign.Tasks[id]
	if task.StartTime <= 0 || task.FinishTime <= task.StartTime {
		t.Fatal("真实库委托时间未冻结")
	}
	before, _ := json.Marshal(p)
	pgRemainingRPC(t, s, c, "commit_consign_task", id)
	after, _ := json.Marshal(pgHumanRead(t, New(store.pool), oid).Progress)
	if !bytes.Equal(before, after) {
		t.Fatal("真实库未完成委托部分提交")
	}
	now = time.Unix(task.FinishTime, 0)
	out := pgRemainingRPC(t, s, c, "commit_consign_task", id)
	if pgRemainingPush(out, "commit_consign_task_success") == nil {
		t.Fatal("真实库委托完整奖励失败", out)
	}
	p = pgHumanRead(t, New(store.pool), oid).Progress
	if p.RemainingGameplay.Consign.Tasks[id] != nil || p.Achievements[205001].Time == 0 || len(p.RemainingGameplay.Consign.PhaseRecv) == 0 {
		t.Fatal("真实库委托任务、成就和累计奖励不同事务")
	}
	pending := p.RemainingGameplay.Consign.PhaseRecv
	counts := map[int]int64{}
	for mid := range pending {
		counts[mid] = p.Materials[mid].Count
	}
	out = pgRemainingRPC(t, s, c, "reward_consign_phase_bonus")
	if pgRemainingPush(out, "on_reward_consign_phase_bonus") == nil {
		t.Fatal("真实库阶段领奖回包缺失")
	}
	p = pgHumanRead(t, New(store.pool), oid).Progress
	for mid, n := range pending {
		if p.Materials[mid].Count != counts[mid]+n {
			t.Fatal("真实库阶段奖励没入账", mid)
		}
	}
	before, _ = json.Marshal(p)
	pgRemainingRPC(t, s, c, "reward_consign_phase_bonus")
	after, _ = json.Marshal(pgHumanRead(t, New(store.pool), oid).Progress)
	if !bytes.Equal(before, after) {
		t.Fatal("真实库重复阶段领奖修改状态")
	}
	pgRemainingRPC(t, s, c, "enter_task_tower", 21, 101, 1, map[string]any{"tower_task_list": []int{999999}})
	av, _ := c.SelectedAvatar()
	raw, _ := json.Marshal(av.Progress.Battle.Extra["tower_task_list"])
	var tasks []int
	if json.Unmarshal(raw, &tasks) != nil || len(tasks) != 3 {
		t.Fatal("真实库绝密冻结清单无效")
	}
	pgRemainingFinish(t, s, c, oid, tasks)
	p = pgHumanRead(t, New(store.pool), oid).Progress
	floor := p.RemainingGameplay.TaskTowers[101].Floors[1]
	if len(floor.Finished) != 3 || len(floor.Received) != 3 || p.RemainingGameplay.PendingTaskDungeon != 0 || p.Achievements[208103].Targets[208103] != 3 {
		t.Fatal("真实库绝密奖励/授权/成就没原子持久化")
	}
	pgRemainingRPC(t, s, c, "enter_free_stage", 10101, true)
	p = pgHumanRead(t, New(store.pool), oid).Progress
	node := 0
	for key, site := range p.RemainingGameplay.Stages[10101].Sites {
		if site.State == 1 {
			node = key
			break
		}
	}
	if node <= 0 {
		t.Fatal("真实库探索未生成原生起点")
	}
	pgRemainingRPC(t, s, c, "enter_stage_site", node, 0)
	pgRemainingFinish(t, s, c, oid, []int{})
	p = pgHumanRead(t, New(store.pool), oid).Progress
	if p.RemainingGameplay.Stages[10101].Sites[node].State != 3 || p.RemainingGameplay.PendingDungeon != 0 {
		t.Fatal("真实库探索胜利未推进节点或消费授权")
	}
	c2 := pgActivityLogin(t, s, info)
	reloaded, _ := c2.SelectedAvatar()
	if reloaded.Progress.RemainingGameplay.Stages[10101].Sites[node].State != 3 || len(reloaded.Progress.RemainingGameplay.TaskTowers[101].Floors[1].Finished) != 3 {
		t.Fatal("真实鉴权重登丢失新玩法节点或任务")
	}
}
