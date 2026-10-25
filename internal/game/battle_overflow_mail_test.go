package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestBattleOverflowFullMailboxQueueAndCollision(t *testing.T) {
	now := time.Unix(1791813600, 0)
	p := NewProgress(60, now)
	for i := 1; i <= 500; i++ {
		p.ShortMailInfo[i] = Mail{MID: i, UUID: fmt.Sprintf("%024x", i), State: MailFinal}
	}
	limit := androidShop.Materials[29].Limit
	p.Materials[29] = Material{ID: 29, Count: limit, Total: limit}
	b := &BattleSession{UUID: "00112233445566778899aabb", DungeonID: 6101}
	settle := func() (map[string]any, error) {
		err := grantNativeItem(&p, 29, 7, 60, now, map[int]int64{}, new([]string), 0)
		return emptyActivityBox(), err
	}
	if _, err := withBattleOverflowMail(&p, b, now, settle); err != nil || len(p.BattleOverflowMailbox) != 1 || len(p.ShortMailInfo) != 500 {
		t.Fatal("满邮箱结算丢奖", err)
	}
	queued := CloneProgress(p)
	if _, err := withBattleOverflowMail(&p, b, now, settle); err == nil {
		t.Fatal("满邮箱绕过重复编号校验")
	}
	p = queued
	av := Avatar{Progress: p}
	if _, leaked := av.InitialProperties("验收账号")["server_battle_overflow_mailbox"]; leaked {
		t.Fatal("内部待投递邮箱下发为未知原生属性")
	}
	delete(p.ShortMailInfo, 1)
	if err := promoteBattleOverflowMailbox(&p); err != nil || len(p.ShortMailInfo) != 500 || len(p.BattleOverflowMailbox) != 0 {
		t.Fatal("释放槽位未投递", err)
	}
	var mail Mail
	for _, m := range p.ShortMailInfo {
		if m.Attachments[29] == 7 {
			mail = m
		}
	}
	if mail.MID == 0 {
		t.Fatal("暂存附件丢失")
	}
	if _, err := withBattleOverflowMail(&p, b, now, settle); err == nil {
		t.Fatal("满邮箱绕过已投递编号校验")
	}
}

func TestBattleOverflowExtraTaskRewardReceiptBoundary(t *testing.T) {
	s, c, _ := remainingFixture(t)
	remainingRPC(t, s, c, "enter_task_tower", 10, 101, 1, map[string]any{})
	b := c.SelectedAvatarUnsafe().Progress.Battle
	var tasks []int
	raw, _ := json.Marshal(b.Extra["tower_task_list"])
	if err := json.Unmarshal(raw, &tasks); err != nil || len(tasks) != 3 {
		t.Fatal("真实三任务快照缺失", err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	out := remainingFinishBattle(t, s, c, tasks, true)
	p := c.SelectedAvatarUnsafe().Progress
	if battleTestResult(out) == nil || !p.Battle.RewardPlanFrozen || p.Battle.RewardExtraPlanStart == nil {
		t.Fatal("额外任务未进入同一冻结结算链")
	}
	if len(p.Battle.RewardBonusPlan)-*p.Battle.RewardExtraPlanStart != 3 {
		t.Fatal("三项任务奖励未完整冻结")
	}
	expected := map[int]int64{}
	for _, receipt := range p.Battle.RewardBonusPlan {
		for mid, amount := range receipt.Materials {
			expected[mid] += amount
		}
	}
	for _, mid := range []int{11, 12, 27} {
		if p.Materials[mid].Count-before.Materials[mid].Count != expected[mid] {
			t.Fatal("副本与任务整笔收据兑现不一致", mid)
		}
	}
}

func TestBattleOverflowFrozenRandomPlanRollbackColdAndCrossDay(t *testing.T) {
	t.Setenv("HS_RUNE_FALLBACK_POOLS", approvedGameplayValue(t, "HS_RUNE_FALLBACK_POOLS"))
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	c, av := newBattleConnection(t, ctx, a, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.GuideTasks = map[int]GuideTask{}
		p.AvatarLevel = 60
		p.Power.Value = 100
		p.ClearedDungeons = append(p.ClearedDungeons, 509)
		p.Materials[29] = Material{ID: 29, Count: 100, Total: 100}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, call := range []struct {
		name string
		args []json.RawMessage
	}{
		{"enter_dungeon", rawArgs(1, 6101, map[string]any{})}, {"load_entity_finish", nil},
		{"battle_fighting", rawArgs(map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})},
	} {
		if _, err := s.Handle(ctx, c, call.name, call.args); err != nil {
			t.Fatal(err)
		}
	}
	uuid := c.SelectedAvatarUnsafe().Progress.Battle.UUID
	observeBattleEvent(t, ctx, s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":1,"kind":"ready","data":{"version":1}}`, uuid))
	observeBattleEvent(t, ctx, s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":2,"kind":"started","data":{"units":[{"eid":"1","role":1,"hex":[0,0,0],"kind":"ally"}]}}`, uuid))
	e := battleEnvelope{BattleUUID: uuid, Sequence: 3, Kind: "result", Data: map[string]any{"winner_eids": []any{hexOf(av.OID)}}}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if err := s.freezeOrdinaryBattleRewardPlan(ctx, c, &e); err != nil {
		t.Fatal(err)
	}
	frozen := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if !frozen.Battle.RewardPlanFrozen || len(frozen.Battle.RewardBonusPlan) == 0 || !reflect.DeepEqual(before.Runes, frozen.Runes) {
		t.Fatal("抽奖阶段提前写资产或缺少收据")
	}
	var expected map[string]Rune = map[string]Rune{}
	for _, r := range frozen.Battle.RewardBonusPlan {
		for id, rune := range r.Runes {
			expected[id] = rune
		}
	}
	if len(expected) == 0 {
		t.Fatal("未冻结真实随机契印")
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if _, err := settleOrdinaryDungeonRewards(p, p.Battle, true, now); err != nil {
			return err
		}
		return errors.New("模拟提交前失败")
	}); err == nil {
		t.Fatal("模拟回滚未发生")
	}
	coldID, err := a.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	cold := NewConnection()
	cold.identity = coldID
	cold.hostnum = 1
	cold.phase = Playing
	if !reflect.DeepEqual(frozen.Runes, cold.SelectedAvatarUnsafe().Progress.Runes) {
		t.Fatal("失败事务泄漏资产")
	}
	now = now.Add(24 * time.Hour)
	out, err := s.Handle(ctx, cold, "client_need_recover_battle", nil)
	if err != nil || battleTestResult(out) == nil {
		t.Fatal("冷恢复未兑现已冻结结果", err)
	}
	settled := CloneProgress(cold.SelectedAvatarUnsafe().Progress)
	if !settled.Battle.Finished || settled.Battle.UUID != uuid {
		t.Fatal("冷恢复重开已抽奖战斗")
	}
	for id, rune := range expected {
		if !reflect.DeepEqual(rune, settled.Runes[id]) {
			t.Fatal("跨日冷重试重新抽奖")
		}
	}
	raw, _ := json.Marshal(e)
	observeBattleEvent(t, ctx, s, cold, string(raw))
	if cold.pendingOrdinaryResult != nil || !reflect.DeepEqual(settled, CloneProgress(cold.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("重复结束结果再次排队或发奖")
	}
}

func TestBattleOverflowMailSplitColdClaimAndDuplicate(t *testing.T) {
	now := time.Unix(1791813600, 0)
	p := NewProgress(60, now)
	limit := androidShop.Materials[29].Limit
	p.Materials[29] = Material{ID: 29, Count: limit - 2, Total: limit - 2}
	b := &BattleSession{UUID: "00112233445566778899aabb", DungeonID: 6101}
	settle := func() (map[string]any, error) {
		if b.RewardGranted {
			return emptyActivityBox(), nil
		}
		changes := map[int]int64{}
		if err := grantNativeItem(&p, 29, limit+7, 60, now, changes, new([]string), 0); err != nil {
			return nil, err
		}
		b.RewardGranted = true
		return map[string]any{"materials": changes}, nil
	}
	box, err := withBattleOverflowMail(&p, b, now, settle)
	if err != nil || p.Materials[29].Count != limit || box["materials"].(map[int]int64)[29] != 2 || len(p.ShortMailInfo) != 2 {
		t.Fatal("未将容量内与溢出正确拆分", err)
	}
	before := CloneProgress(p)
	if _, err := withBattleOverflowMail(&p, b, now, settle); err != nil || !reflect.DeepEqual(before, CloneProgress(p)) {
		t.Fatal("重复结果改变资产或邮件", err)
	}
	raw, _ := json.Marshal(p)
	var cold Progress
	if err := json.Unmarshal(raw, &cold); err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, mail := range cold.ShortMailInfo {
		if mail.ExpiresAt != 0 || !mail.RewardsFrozen || mail.Attachments[29] > limit {
			t.Fatal("邮件无法完整领取或会过期")
		}
		full := CloneProgress(cold)
		if err := claimMail(&cold, &mail, now.Unix()); err == nil || !reflect.DeepEqual(full, CloneProgress(cold)) {
			t.Fatal("容量未释放仍领取或部分写入")
		}
		cold.Materials[29] = Material{ID: 29, Total: cold.Materials[29].Total}
		amount := mail.Attachments[29]
		if err := claimMail(&cold, &mail, now.Unix()); err != nil || cold.Materials[29].Count != amount {
			t.Fatal("释放容量后未领取", err)
		}
		total += amount
		if err := claimMail(&cold, &mail, now.Unix()); err == nil {
			t.Fatal("附件重复领取")
		}
		cold.Materials[29] = Material{ID: 29, Count: limit, Total: cold.Materials[29].Total}
	}
	if total != limit+5 {
		t.Fatal("溢出材料丢失", total)
	}
	ordinary := NewProgress(60, now)
	ordinary.Materials[29] = Material{ID: 29, Count: limit, Total: limit}
	if err := grantNativeItem(&ordinary, 29, 1, 60, now, map[int]int64{}, new([]string), 0); err == nil {
		t.Fatal("非战斗容量保护被取消")
	}
}
