package game

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// init_config init_cards=[[4401,1]]：新进度必须有初始卡，且经 InitialProperties
// 组装成 card_mgr 卡字典（client save 通道）。
func TestNewProgressInitialCards(t *testing.T) {
	p := NewProgress(1, time.Unix(1700000000, 0))
	if len(p.Cards) != 1 || p.Cards[0].CardID != 4401 || p.Cards[0].Level != 1 || p.Cards[0].Grade != 0 {
		t.Fatal("初始卡 4401 缺失或等级/品阶错误", p.Cards)
	}
	if len(p.Cards[0].UUID) != 24 {
		t.Fatal("卡 uuid 必须是 12 字节 hex", p.Cards[0].UUID)
	}
	if p.Cards[0].Dress != androidCardAppearances[4401].DefaultDress {
		t.Fatal("新卡必须保存原默认装帧，角色界面不能索引0")
	}
	avatar := Avatar{Progress: p}
	props := avatar.InitialProperties("账号")
	mgr, ok := props["card_mgr"].(map[string]any)
	if !ok || len(mgr) != 1 {
		t.Fatal("card_mgr 必须下发", props["card_mgr"])
	}
	card, ok := mgr[p.Cards[0].UUID].(map[string]any)
	if !ok {
		t.Fatal("card_mgr 键必须是卡 uuid", mgr)
	}
	if card["card_id"] != 4401 || card["level"] != 1 || card["grade"] != 0 || card["support_skill_level"] != 1 {
		t.Fatal("卡字典字段错误", card)
	}
	// card_common_mgr：sound_mgr.init_vo_language_map 读 card_common_mgr[card_id].card_vo，
	// 缺条目会把 on_login_success 打断（实机 KeyError 4401 → 过场后卡死）。
	common, ok := props["card_common_mgr"].(map[string]any)
	if !ok || len(common) != 1 {
		t.Fatal("card_common_mgr 必须下发", props["card_common_mgr"])
	}
	c4401, ok := common["4401"].(map[string]any)
	if !ok {
		t.Fatal("card_common_mgr 键必须是 card_id 字符串", common)
	}
	if c4401["card_id"] != 4401 || c4401["card_vo"] != 0 || c4401["level"] != 1 || c4401["count"] != 1 {
		t.Fatal("card_common 字段错误", c4401)
	}
}

// 编队校验：占位 0 与拥有 uuid 放行（占位路径由 TestGuideBattle* 覆盖），未拥有 uuid 拒绝。
func TestBattleFightingValidatesOwnedCardUUID(t *testing.T) {
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	now := time.Unix(1700000000, 0)
	service.Now = func() time.Time { return now }
	ctx := context.Background()
	c, av := newBattleConnection(t, ctx, accounts, service)
	// 给角色一张卡。
	updated, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = append(p.Cards, newCard(4401, 1, now))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := range c.identity.Avatars {
		if c.identity.Avatars[i].Hostnum == 1 {
			c.identity.Avatars[i].Progress = updated
		}
	}
	// 前置：enter_dungeon + load_entity_finish（battle_fighting 校验的前置状态）。
	args := []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`10001`), json.RawMessage(`{}`)}
	if _, err := service.Handle(ctx, c, "enter_dungeon", args); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(ctx, c, "load_entity_finish", nil); err != nil {
		t.Fatal(err)
	}
	start := func(fighting string) error {
		args := []json.RawMessage{json.RawMessage(`{"fighting_cards":` + fighting + `,"support_cards":[],"storyline_cards":[]}`)}
		_, err := service.Handle(ctx, c, "battle_fighting", args)
		return err
	}
	if err := start(`["deadbeefdeadbeefdeadbeef"]`); err == nil {
		t.Fatal("未拥有 uuid 被放行")
	}
	if err := start(`["` + updated.Cards[0].UUID + `"]`); err != nil {
		t.Fatal("拥有卡 uuid 阵容被拒", err)
	}
}

// 体力：扣费、恢复结算与不足拒绝。
func TestConsumePowerWithRecovery(t *testing.T) {
	start := time.Unix(1700000000, 0)
	p := NewProgress(1, start)
	p.Power.Max = 100
	if !p.consumePower(10, start) || p.Power.Value != 90 {
		t.Fatal("扣费失败", p.Power)
	}
	// 经过 300 秒恢复 1 点。
	later := start.Add(650 * time.Second)
	if got := p.currentPower(later); got != 92 {
		t.Fatal("恢复结算错误", got)
	}
	if !p.consumePower(92, later) || p.Power.Value != 0 {
		t.Fatal("全额扣费失败", p.Power)
	}
	if p.consumePower(1, later) {
		t.Fatal("不足时应拒绝")
	}
	// 超上限封顶。
	full := p.currentPower(start.Add(300 * 300 * time.Second))
	if full != 100 {
		t.Fatal("恢复超上限未封顶", full)
	}
}

// new_task 计数推进（表 101：target 20102 累计通关 1 次）。
func TestAdvanceNewTaskByDungeonClear(t *testing.T) {
	now := time.Unix(1700000000, 0)
	p := NewProgress(1, now)
	p.NewTasks = map[int]NewTaskProgress{101: {TaskID: 101, Status: 1, FinishedTargets: map[int]int{}}}
	if !p.advanceNewTask(20102, now) {
		t.Fatal("通关推进必须命中任务")
	}
	task := p.NewTasks[101]
	if task.FinishedTargets[20102] != 1 || task.Status != 1 {
		t.Fatal("任务计数/状态错误", task)
	}
	if p.advanceNewTask(20102, now) {
		t.Fatal("已完成任务不得重复推进")
	}
	if p.advanceNewTask(10001, now) {
		t.Fatal("无关副本不得推进")
	}
}
