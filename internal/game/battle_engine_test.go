package game

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// 教学战斗引擎八要素测试：校验、伤害、回合推进、波次、胜负与重复输入。
// syncArgs 取 sync_battle_method 推送的实参列表首项。
func syncArgs(push Push) any {
	args, _ := push.Args[1].([]any)
	if len(args) == 0 {
		return nil
	}
	return args[0]
}

func startGuideBattle(t *testing.T, ctx context.Context, service *Service, c *Connection, now *time.Time) {
	t.Helper()
	args := []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`10001`), json.RawMessage(`{}`)}
	if _, err := service.Handle(ctx, c, "enter_dungeon", args); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(ctx, c, "load_entity_finish", nil); err != nil {
		t.Fatal(err)
	}
	startArgs := []json.RawMessage{json.RawMessage(`{"fighting_cards":[0],"support_cards":[],"storyline_cards":[]}`)}
	if _, err := service.Handle(ctx, c, "battle_fighting", startArgs); err != nil {
		t.Fatal(err)
	}
	// 客户端权威改造后 battle_fighting 不再构建权威实体；旧权威引擎链的
	// 回归测试在此手动重建实体并推进到等待输入状态。
	if err := service.updateProgress(ctx, c, func(p *Progress) error {
		b := p.Battle
		// 旧权威研究回归的随机伤害必须可重现；生产会话仍由crypto/rand播种。
		// 此夹具只验证固定输入链，不能断言所有随机战斗都由玩家获胜。
		b.Seed = 123456
		entities, err := buildGuideBattleEntities(b.BattleID)
		if err != nil {
			return err
		}
		b.Entities = entities
		if err := b.armStartTriggers(); err != nil {
			return err
		}
		b.Bootstrap = phaseFirstInput
		b.DueAt = 0
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func newBattleConnection(t *testing.T, ctx context.Context, accounts *FixtureAccounts, service *Service) (*Connection, *Avatar) {
	t.Helper()
	identity, err := accounts.Register(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	av, err := accounts.SetNicknameGender(ctx, identity.Avatars[0].OID, "战斗馆长", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error { return AdvanceGuide(p, 1000, av.CreatedAt, 1) }); err != nil {
		t.Fatal(err)
	}
	c := NewConnection()
	c.identity = Identity{Account: identity.Account, Avatars: []Avatar{av}}
	c.hostnum = 1
	c.phase = Playing
	c.battleStartSent = true
	return c, &av
}

func TestGuideBattleDoCommandValidationAndDamage(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 5, 19, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, accounts, service)
	startGuideBattle(t, ctx, service, c, &now)

	selected, _ := c.SelectedAvatar()
	entities := selected.Progress.Battle.Entities
	if len(entities) != 4 || entities[0].EID != "1" || entities[0].RoleID != 4403 || !entities[0].Master {
		t.Fatal("权威实体构建错误", entities)
	}
	if entities[0].HP != 283 || entities[0].ATK != 56 {
		t.Fatal("主角属性错误", entities[0])
	}
	if entities[1].RoleID != 202 || entities[1].HP != 31 || entities[1].ATK != 45 {
		t.Fatal("敌方属性必须应用 battle_info enemy 因子", entities[1])
	}
	if entities[1].Camp != 2 || entities[2].Camp != 2 || entities[3].Camp != 2 {
		t.Fatal("敌方阵营错误", entities)
	}
	if entities[0].AP != 500 {
		t.Fatal("instance_event 10001011 set_ap 未落实", entities[0])
	}

	// 非法用例：未知技能、敌方做施法者。
	bad := []json.RawMessage{json.RawMessage(`"use_skill"`), json.RawMessage(`["ck_skill","1",999999,"2"]`)}
	if _, err := service.doCommandAuthoritative(ctx, c, bad); err == nil {
		t.Fatal("未知技能被放行")
	}
	bad = []json.RawMessage{json.RawMessage(`"use_skill"`), json.RawMessage(`["ck_skill","2",510106,"1"]`)}
	if _, err := service.doCommandAuthoritative(ctx, c, bad); err == nil {
		t.Fatal("敌方做施法者被放行")
	}

	// 实机上行形态：["use_skill",["ck_monster","1",440301,"2"]]。
	command := []json.RawMessage{json.RawMessage(`"use_skill"`), json.RawMessage(`["ck_monster","1",440301,"2"]`)}
	if _, err := service.doCommandAuthoritative(ctx, c, command); err != nil {
		t.Fatal(err)
	}
	selected, _ = c.SelectedAvatar()
	battle := selected.Progress.Battle
	if battle.Bootstrap != phaseActionPlay || battle.ActionEID != "1" || battle.AttackTarget != "2" {
		t.Fatal("攻击后阶段错误", battle)
	}
	if battle.entity("2").Alive || battle.entity("2").HP != 0 {
		t.Fatalf("一击伤害 %d 未击杀 31 血敌方", battle.AttackDamage)
	}
	// 客户端公式（effect_change_hp + defence + get_expect_value）：
	// 56 × 0.64 × (1000/(1000+138)) = 31.4938…，round_func 的 .5±0.01 边界分支 int() 截断 → 31。
	if battle.AttackDamage != 31 {
		t.Fatal("伤害必须等于 roundFunc(56*0.64*1000/1138)=31", battle.AttackDamage)
	}
	if battle.entity("1").AP != -500 {
		t.Fatal("行动必须消耗 1000 AP", battle.entity("1"))
	}
	// 行动演出期间重复输入被拒。
	if _, err := service.doCommandAuthoritative(ctx, c, command); err == nil {
		t.Fatal("演出期间重复输入被放行")
	}

	// 演出到期 → 主角行动结束：1000106 引导标记 + 敌方 "2" 已死不行动，下一个行动者必须为敌方。
	now = now.Add(1500 * time.Millisecond)
	round, err := service.Tick(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(round) < 3 || round[0].Args[0] != "real_round_end" || round[1].Args[0] != "real_start_next_round" {
		t.Fatal("回合跳转消息错误", round)
	}
	if syncArgs(round[0]) != "1" {
		t.Fatal("round_end 必须携带行动者 eid", round[0])
	}
	// "2" 已阵亡，按 AP 行动模型下一个行动者是存活敌方 "3"。
	if round[2].Args[0] != "after_pre_play" || syncArgs(round[2]) != "3" {
		t.Fatal("敌方行动必须以 after_pre_play 同步", round[2])
	}
	selected, _ = c.SelectedAvatar()
	battle = selected.Progress.Battle
	// 敌方 "3"（201，演出1.0秒）到期后 round_end 才结算波次。
	now = now.Add(1200 * time.Millisecond)
	if _, err := service.Tick(ctx, c); err != nil {
		t.Fatal(err)
	}
	selected, _ = c.SelectedAvatar()
	battle = selected.Progress.Battle
	if battle.Triggers == nil || !battle.Triggers.Fired[1000102] || len(battle.Entities) != 10 {
		t.Fatal("敌方首次行动结束必须触发 1000102 波次", len(battle.Entities))
	}
	// 客户端按事件序先创建 camp2 增援（'5','6','7'=202），再创建 camp1（'8'=107 带 init_ap=900）。
	if battle.entity("5") == nil || battle.entity("5").RoleID != 202 || battle.entity("5").Camp != 2 {
		t.Fatal("增援 camp2 必须先占 '5','6','7'", battle.entity("5"))
	}
	if battle.entity("8") == nil || battle.entity("8").RoleID != 107 || battle.entity("8").AP != 900 {
		t.Fatal("增援 camp1 必须带 init_ap=900", battle.entity("8"))
	}

	// 空挥契约：客户端攻击服务端已死亡的目标时不得拒绝（否则客户端本地行动等待回合同步而冻结）。
	// 等回合回到主角输入态。
	deadline := 0
	for {
		selected, _ = c.SelectedAvatar()
		if waitingForInput(selected.Progress.Battle.Bootstrap) || selected.Progress.Battle.Bootstrap == phaseBattleOver {
			break
		}
		now = now.Add(1500 * time.Millisecond)
		if _, err := service.Tick(ctx, c); err != nil {
			t.Fatal(err)
		}
		deadline++
		if deadline > 40 {
			t.Fatal("回合未回到主角输入态")
		}
	}
	if selected.Progress.Battle.Bootstrap == phaseBattleOver {
		t.Fatal("空挥验证前战斗不应结束")
	}
	whiff := []json.RawMessage{json.RawMessage(`"use_skill"`), json.RawMessage(`["ck_skill","1",440301,"2"]`)}
	if _, err := service.doCommandAuthoritative(ctx, c, whiff); err != nil {
		t.Fatal("攻击已死亡目标被拒绝", err)
	}
	selected, _ = c.SelectedAvatar()
	battle = selected.Progress.Battle
	if battle.Bootstrap != phaseActionPlay || battle.AttackDamage != 0 {
		t.Fatal("空挥必须推进回合且无伤害", battle.Bootstrap, battle.AttackDamage)
	}
}

// 完整战斗循环：玩家逐个击杀，服务端推进到胜利结算。
func TestGuideBattleFullLoopToVictory(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, accounts, service)
	startGuideBattle(t, ctx, service, c, &now)

	command := []json.RawMessage{json.RawMessage(`"use_skill"`), json.RawMessage(`["ck_monster","1",440301,"2"]`)}
	attack := func() {
		t.Helper()
		if _, err := service.doCommandAuthoritative(ctx, c, command); err != nil {
			t.Fatal(err)
		}
	}
	// 至多 400 次推进：等待玩家输入则攻击，否则推进 1.5 秒演出。
	for step := 0; step < 400; step++ {
		selected, _ := c.SelectedAvatar()
		battle := selected.Progress.Battle
		if battle.Bootstrap == phaseBattleOver {
			break
		}
		if waitingForInput(battle.Bootstrap) {
			// 找一个存活敌方为目标（客户端高亮哪个就打哪个；此处取 eid 最小者）。
			target := ""
			for i := range battle.Entities {
				if battle.Entities[i].Camp == enemyCamp && battle.Entities[i].Alive {
					target = battle.Entities[i].EID
					break
				}
			}
			if target == "" {
				t.Fatal("等待输入但没有敌方存活")
			}
			command = []json.RawMessage{json.RawMessage(`"use_skill"`), json.RawMessage([]byte(`["ck_monster","1",440301,"` + target + `"]`))}
			attack()
		}
		now = now.Add(1500 * time.Millisecond)
		if _, err := service.Tick(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	selected, _ := c.SelectedAvatar()
	battle := selected.Progress.Battle
	if battle.Bootstrap != phaseBattleOver || battle.Status != "结束" {
		t.Fatal("战斗未在限步内结束", battle.Bootstrap)
	}
	// 胜利来源：instance_event.battle_end=victory（T1000105：107 第二次行动结束）
	// 或敌方全灭兜底，二者均为合法胜利。
	// 主角在 283 血下经历整场战斗应存活。
	if !battle.entity("1").Alive {
		t.Fatal("主角意外死亡", battle.entity("1"))
	}
}

// 数据驱动触发器链：敌方第一波全灭触发 T1000104 第二波增援（202×4+201×2 + 201 补位 + 注册 T1000105）；
// 随后 107 第二次行动结束触发 T1000105 → battle_end=victory（教学真实胜利条件，允许敌方存活）。
func TestGuideBattleTriggerChainSecondWaveAndVictory(t *testing.T) {
	b := &BattleSession{BattleID: 10001}
	entities, err := buildGuideBattleEntities(b.BattleID)
	if err != nil {
		t.Fatal(err)
	}
	b.Entities = entities
	if err := b.armStartTriggers(); err != nil {
		t.Fatal(err)
	}
	if !b.Triggers.Armed[1000102] || !b.Triggers.Armed[1] || !b.Triggers.Armed[1000106] {
		t.Fatal("启动链必须保留待触发器 T1/T1000102/T1000106", b.Triggers.Armed)
	}
	if b.entity("1").AP != 500 {
		t.Fatal("EV10001011 必须将 4403 AP 置 500", b.entity("1").AP)
	}
	// 敌方首次行动结束 → 第一波增援（camp2 '5','6','7' + camp1 '8','9','10'）。
	if _, err := b.onActionEnd(b.entity("2")); err != nil {
		t.Fatal(err)
	}
	if len(b.Entities) != 10 {
		t.Fatal("T1000102 增援后实体应为 10", len(b.Entities))
	}
	// 主角首次 turn_start → T1000103 → EV10001040 注册 T1000104。
	if _, err := b.onActionBegin(b.entity("1")); err != nil {
		t.Fatal(err)
	}
	if !b.Triggers.Armed[1000104] {
		t.Fatal("T1000103 必须注册 T1000104", b.Triggers.Armed)
	}
	// 杀光敌方（含第一波增援）→ T1000104：第二波 202×4+201×2 + 201 + 注册 T1000105。
	for i := range b.Entities {
		if b.Entities[i].Camp == enemyCamp {
			b.Entities[i].Alive = false
		}
	}
	end, err := b.onEntityReduce()
	if err != nil {
		t.Fatal(err)
	}
	if end != "" {
		t.Fatal("T1000104 不应直接结束战斗", end)
	}
	roleCount := map[int]int{}
	for i := range b.Entities {
		if b.Entities[i].Camp == enemyCamp && b.Entities[i].Alive {
			roleCount[b.Entities[i].RoleID]++
		}
	}
	if roleCount[202] != 4 || roleCount[201] != 3 {
		t.Fatal("T1000104 必须增援第二波 202×4+201×2+201", roleCount)
	}
	if !b.Triggers.Armed[1000105] {
		t.Fatal("EV10001050 必须注册 T1000105", b.Triggers.Armed)
	}
	// 107 第一次行动结束：未达 action_end_counter=2，不触发。
	if end, err = b.onActionEnd(b.entity("8")); err != nil || end != "" {
		t.Fatal("107 第一次行动结束不应胜利", end, err)
	}
	// 107 第二次行动结束 → battle_end=victory（此时敌方存活也判胜）。
	end, err = b.onActionEnd(b.entity("8"))
	if err != nil {
		t.Fatal(err)
	}
	if end != "victory" {
		t.Fatal("T1000105 必须判胜", end)
	}
	// 玩家全灭 → T1 fail。
	b.Entities = entities[:1]
	for i := range b.Entities {
		b.Entities[i].Alive = false
	}
	b.ensureTriggers()
	b.Triggers.Armed[1] = true
	if end, err = b.onEntityReduce(); err != nil || end != "fail" {
		t.Fatal("玩家全灭必须判负", end, err)
	}
}

// 回归：T1000105 注册（第一波全灭）前 107 已多次行动。旧实现用全局累计计数（role:107
// 早已 >2）导致 ==2 永不成立、victory 永不触发；实例计数语义必须以注册时刻归零。
func TestGuideBattleVictoryCountsFromActivation(t *testing.T) {
	b := &BattleSession{BattleID: 10001}
	entities, err := buildGuideBattleEntities(b.BattleID)
	if err != nil {
		t.Fatal(err)
	}
	b.Entities = entities
	if err := b.armStartTriggers(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.onActionEnd(b.entity("2")); err != nil { // 第一波增援
		t.Fatal(err)
	}
	if _, err := b.onActionBegin(b.entity("1")); err != nil { // 注册 T1000104
		t.Fatal(err)
	}
	// 全灭前 107 已行动两次（若按全局计数，阈值 2 在注册前就已被越过）。
	if _, err := b.onActionEnd(b.entity("8")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.onActionEnd(b.entity("9")); err != nil {
		t.Fatal(err)
	}
	for i := range b.Entities {
		if b.Entities[i].Camp == enemyCamp {
			b.Entities[i].Alive = false
		}
	}
	end, err := b.onEntityReduce() // T1000104：第二波 + 注册 T1000105（计数归零）
	if err != nil || end != "" {
		t.Fatal("T1000104 不应结束战斗", end, err)
	}
	if !b.Triggers.Armed[1000105] || b.Triggers.TriggerCnt[1000105] != 0 {
		t.Fatal("T1000105 必须以计数 0 注册", b.Triggers.TriggerCnt[1000105], b.Triggers.Armed)
	}
	if end, err = b.onActionEnd(b.entity("8")); err != nil || end != "" {
		t.Fatal("注册起第 1 次 107 行动不应胜利", end, err)
	}
	if end, err = b.onActionEnd(b.entity("9")); err != nil || end != "victory" {
		t.Fatal("注册起第 2 次 107 行动必须判胜", end, err)
	}
}

// 回归：第一波先于 T1000104 注册（主角第二次行动开始）被打光时，注册瞬间必须
// 依 triggered_when_add 立即补触发，第二波与 T1000105 才不会被永久错过。
func TestGuideBattleEntityReduceFiresWhenRegisteredLate(t *testing.T) {
	b := &BattleSession{BattleID: 10001}
	entities, err := buildGuideBattleEntities(b.BattleID)
	if err != nil {
		t.Fatal(err)
	}
	b.Entities = entities
	if err := b.armStartTriggers(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.onActionEnd(b.entity("2")); err != nil { // 第一波增援
		t.Fatal(err)
	}
	// 主角尚未有第二次行动开始，T1000104 未注册时敌方已被打光。
	for i := range b.Entities {
		if b.Entities[i].Camp == enemyCamp {
			b.Entities[i].Alive = false
		}
	}
	if end, err := b.onEntityReduce(); err != nil || end != "" {
		t.Fatal("未注册的 T1000104 不应有动作", end, err)
	}
	// 主角第二次行动开始 → T1000103 → 注册 T1000104 → 条件已成立立即结算。
	if _, err := b.onActionBegin(b.entity("1")); err != nil {
		t.Fatal(err)
	}
	if !b.Triggers.Fired[1000104] || !b.Triggers.Armed[1000105] {
		t.Fatal("迟到注册的 T1000104 必须立即触发并注册 T1000105", b.Triggers.Fired, b.Triggers.Armed)
	}
	roleCount := map[int]int{}
	for i := range b.Entities {
		if b.Entities[i].Camp == enemyCamp && b.Entities[i].Alive {
			roleCount[b.Entities[i].RoleID]++
		}
	}
	if roleCount[202] != 4 || roleCount[201] != 3 {
		t.Fatal("立即触发必须生成第二波 202×4+201×2+201", roleCount)
	}
}

// 10002/10201 的触发器闭包在基线内完整（实体可建、启动链可结算）。
func TestBattleTriggerClosuresForLaterTeachBattles(t *testing.T) {
	for bid := range clientBaseline.Battles {
		b := &BattleSession{BattleID: bid}
		entities, err := buildGuideBattleEntities(bid)
		if err != nil {
			t.Fatalf("战斗 %d 实体构建失败：%v", bid, err)
		}
		b.Entities = entities
		if err := b.armStartTriggers(); err != nil {
			t.Fatalf("战斗 %d 启动链失败：%v", bid, err)
		}
	}
}

// 引擎上线前的旧会话（等待首次输入、无实体）必须能补建并继续战斗。
func TestGuideBattleLegacySessionMigration(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 5, 21, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, av := newBattleConnection(t, ctx, accounts, service)
	if _, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Battle = &BattleSession{UUID: "legacy0000000000000000ff", DungeonID: 10001, BattleID: 10001,
			Seed: 123456, Status: "准备", CreatedAt: now.Unix(), Loaded: true, Started: true,
			Bootstrap: phaseFirstInput}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// 补建属于 Tick/输入路径，不依赖 battle_fighting 重复上行。
	command := []json.RawMessage{json.RawMessage(`"use_skill"`), json.RawMessage(`["ck_monster","1",440301,"2"]`)}
	if _, err := service.doCommandAuthoritative(ctx, c, command); err != nil {
		t.Fatal(err)
	}
	selected, _ := c.SelectedAvatar()
	battle := selected.Progress.Battle
	if len(battle.Entities) != 4 || battle.entity("2").Alive {
		t.Fatal("旧会话补建失败", len(battle.Entities))
	}
	if battle.Bootstrap != phaseActionPlay {
		t.Fatal("补建后攻击未生效", battle.Bootstrap)
	}
}

// move_to 语义：仅当前输入角色可移动；接受后消耗 AP 并推进回合，避免客户端本地等待冻结。
func TestGuideBattleMoveToContract(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Date(2026, 10, 5, 22, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, accounts, service)
	startGuideBattle(t, ctx, service, c, &now)

	bad := []json.RawMessage{json.RawMessage(`"move_to"`), json.RawMessage(`["2",[0,3,-3]]`)}
	if _, err := service.doCommandAuthoritative(ctx, c, bad); err == nil {
		t.Fatal("非当前输入角色移动被放行")
	}
	good := []json.RawMessage{json.RawMessage(`"move_to"`), json.RawMessage(`["1",[0,3,-3]]`)}
	if _, err := service.doCommandAuthoritative(ctx, c, good); err != nil {
		t.Fatal(err)
	}
	selected, _ := c.SelectedAvatar()
	battle := selected.Progress.Battle
	if battle.Bootstrap != phaseActionPlay || battle.entity("1").AP != -500 {
		t.Fatal("移动必须消耗 AP 并进入行动演出", battle.Bootstrap, battle.entity("1").AP)
	}
	now = now.Add(1200 * time.Millisecond)
	round, err := service.Tick(ctx, c)
	if err != nil || len(round) < 3 {
		t.Fatal("移动后回合未推进", round, err)
	}
}
