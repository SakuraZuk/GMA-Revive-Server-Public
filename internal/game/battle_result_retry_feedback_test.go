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

// 模拟奖励已经冻结、结算闭包执行完毕，但SQL事务整体回滚的故障点。
// 必须从Handle完整进入，验证回调停止重发后Tick仍能提交同一份真实结果。
type feedbackResultRollbackStore struct {
	*FixtureAccounts
	failure error
	left    int
}

func (s *feedbackResultRollbackStore) UpdateProgress(ctx context.Context, oid []byte, fn func(*Progress) error) (Progress, error) {
	return s.FixtureAccounts.UpdateProgress(ctx, oid, func(p *Progress) error {
		if err := fn(p); err != nil {
			return err
		}
		if s.left > 0 && p.Battle != nil && p.Battle.Finished {
			s.left--
			return s.failure
		}
		return nil
	})
}

func TestFeedbackOrdinaryResultRollbackRetainsExactEnvelopeUntilCommit(t *testing.T) {
	for _, code := range []string{"55P03", "40P01", "40001"} {
		t.Run(code, func(t *testing.T) {
			ctx := context.Background()
			accounts := NewFixtureAccounts(nil)
			service := New(accounts, nil)
			now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
			service.Now = func() time.Time { return now }
			c, av := newBattleConnection(t, ctx, accounts, service)
			uuid := startObservedBattle(t, ctx, service, c)
			service.Accounts = &feedbackResultRollbackStore{FixtureAccounts: accounts, failure: basicSQLStateFailure{code: code}, left: 2}
			result := fmt.Sprintf(`{"battle_uuid":%q,"sequence":3,"kind":"result","data":{"winner_eids":["%x"],"finished_task_list":[]}}`, uuid, av.OID)
			if out := observeBattleEvent(t, ctx, service, c, result); battleTestResult(out) != nil || c.pendingOrdinaryResult == nil {
				t.Fatal("SQL回滚后提前报成功或丢弃结果", out)
			}
			frozen := CloneProgress(c.SelectedAvatarUnsafe().Progress)
			if frozen.Battle.Finished || !frozen.Battle.RewardPlanFrozen {
				t.Fatal("真实末态没有冻结，或回滚后仍提交通关")
			}
			if out, err := service.retryOrdinaryResult(ctx, c); err != nil || len(out) != 0 {
				t.Fatal("尚未到重试期限就重放结果", out, err)
			}
			now = now.Add(time.Second)
			if out, err := service.retryOrdinaryResult(ctx, c); err != nil || battleTestResult(out) != nil || c.pendingOrdinaryResult == nil || !reflect.DeepEqual(frozen, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
				t.Fatal("第二次SQL冲突丢失待重试结果或改变冻结资产", out, err)
			}
			now = now.Add(time.Second)
			out, err := service.retryOrdinaryResult(ctx, c)
			if err != nil || battleTestResult(out) == nil || c.pendingOrdinaryResult != nil || !c.SelectedAvatarUnsafe().Progress.Battle.Finished {
				t.Fatal("锁释放后未完成原结果结算", out, err)
			}
			committed := CloneProgress(c.SelectedAvatarUnsafe().Progress)
			observeBattleEvent(t, ctx, service, c, result)
			if !reflect.DeepEqual(committed, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
				t.Fatal("重复结果改变资产或奖励收据")
			}
			cold, err := accounts.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
			if err != nil || !cold.Battle.Finished || !reflect.DeepEqual(committed.Battle.SettlementBox, cold.Battle.SettlementBox) {
				t.Fatal("SQL冲突恢复的结算未持久化", err)
			}
		})
	}
}

func TestFeedbackOrdinaryResultPermanentFailureDoesNotLoopOrGrant(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, service)
	uuid := startObservedBattle(t, ctx, service, c)
	service.Accounts = &feedbackResultRollbackStore{FixtureAccounts: accounts, failure: errors.New("不可重试的存档错误"), left: 1}
	observeBattleEvent(t, ctx, service, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":3,"kind":"result","data":{"winner_eids":["%x"],"finished_task_list":[]}}`, uuid, av.OID))
	if c.pendingOrdinaryResult != nil || c.SelectedAvatarUnsafe().Progress.Battle.Finished {
		t.Fatal("永久错误自动重试或伪造通关")
	}
}

// 现网2026-10-14多次battle_fighting因55P03锁超时直接外抛，客户端无开战推送
// 卡在进战斗。与基础领奖同规则：可重放短锁冲突在同一请求内有界重试。
type battleStartFlakyStore struct {
	*FixtureAccounts
	failure error
	left    int
}

func (s *battleStartFlakyStore) UpdateProgress(ctx context.Context, oid []byte, fn func(*Progress) error) (Progress, error) {
	return s.FixtureAccounts.UpdateProgress(ctx, oid, func(p *Progress) error {
		was := p.Battle != nil && p.Battle.Started
		if err := fn(p); err != nil {
			return err
		}
		if s.left > 0 && !was && p.Battle != nil && p.Battle.Started {
			s.left--
			return s.failure
		}
		return nil
	})
}

func TestBattleFightingRetriesShortLockConflict(t *testing.T) {
	for _, code := range []string{"55P03", "40P01", "40001"} {
		t.Run(code, func(t *testing.T) {
			ctx := context.Background()
			accounts := NewFixtureAccounts(nil)
			service := New(accounts, nil)
			now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
			service.Now = func() time.Time { return now }
			c, _ := newBattleConnection(t, ctx, accounts, service)
			args := []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`10001`), json.RawMessage(`{}`)}
			if _, err := service.Handle(ctx, c, "enter_dungeon", args); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Handle(ctx, c, "load_entity_finish", nil); err != nil {
				t.Fatal(err)
			}
			service.Accounts = &battleStartFlakyStore{FixtureAccounts: accounts, failure: basicSQLStateFailure{code: code}, left: 1}
			start := []json.RawMessage{json.RawMessage(`{"fighting_cards":[0],"support_cards":[],"storyline_cards":[]}`)}
			out, err := service.Handle(ctx, c, "battle_fighting", start)
			if err != nil {
				t.Fatal("短锁冲突未在同一请求内重试", err)
			}
			started := false
			for _, item := range out {
				if item.Method == "sync_battle_method" && len(item.Args) > 0 && item.Args[0] == "start" {
					started = true
				}
			}
			if !started {
				t.Fatal("重试后缺少开战推送", out)
			}
			if !c.SelectedAvatarUnsafe().Progress.Battle.Started {
				t.Fatal("重试成功后战斗未开始")
			}
			if service.Accounts.(*battleStartFlakyStore).left != 0 {
				t.Fatal("失败注入未被消耗")
			}
			// 幂等：重复battle_fighting不再重开。
			if out, err := service.Handle(ctx, c, "battle_fighting", start); err != nil || len(out) != 0 {
				t.Fatal("已开始战斗的重复请求应被忽略", out, err)
			}
		})
	}
}

func TestBattleFightingPermanentStoreErrorPropagates(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, service)
	args := []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`10001`), json.RawMessage(`{}`)}
	if _, err := service.Handle(ctx, c, "enter_dungeon", args); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(ctx, c, "load_entity_finish", nil); err != nil {
		t.Fatal(err)
	}
	service.Accounts = &battleStartFlakyStore{FixtureAccounts: accounts, failure: errors.New("不可重试的存档错误"), left: 1}
	start := []json.RawMessage{json.RawMessage(`{"fighting_cards":[0],"support_cards":[],"storyline_cards":[]}`)}
	if _, err := service.Handle(ctx, c, "battle_fighting", start); err == nil {
		t.Fatal("永久存储错误被吞掉")
	}
	if c.SelectedAvatarUnsafe().Progress.Battle.Started {
		t.Fatal("失败事务后战斗被标记开始")
	}
}
