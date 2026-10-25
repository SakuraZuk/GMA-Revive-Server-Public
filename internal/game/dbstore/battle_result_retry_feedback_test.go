package dbstore

import (
	"context"
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"hs-server/internal/game"
)

// 在真实PG事务业务完成后注入SQLSTATE，验证存档回滚与Tick恢复。
// 此处是确定性的SQL错误注入，不宣称制造了实际PG死锁。
type feedbackSQLRollbackStore struct {
	*Store
	code string
	left int
}

func (s *feedbackSQLRollbackStore) UpdateProgress(ctx context.Context, oid []byte, fn func(*game.Progress) error) (game.Progress, error) {
	return s.Store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		if err := fn(p); err != nil {
			return err
		}
		if s.left > 0 && p.Battle != nil && p.Battle.Finished {
			s.left--
			return &pgconn.PgError{Code: s.code, Message: "结算事务故障注入"}
		}
		return nil
	})
}

func TestPostgresFeedbackResultSQLRollbackTickAndColdReceipt(t *testing.T) {
	for _, code := range []string{"55P03", "40P01", "40001"} {
		t.Run(code, func(t *testing.T) {
			store := testStore(t)
			ctx := context.Background()
			now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
			info := game.ClientInfo{Account: "回滚" + code, Password: "pw", Hostnum: 1}
			svc, c, av := repairPlayer(t, store, info, &now)
			defer svc.Detach(c)
			if _, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
				p.AvatarLevel = 60
				p.GuideTasks = map[int]game.GuideTask{}
				p.Power.Value = 100
				p.ClearedDungeons = append(p.ClearedDungeons, 509)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			pgRemainingRPC(t, svc, c, "enter_dungeon", 1, 6101, map[string]any{})
			pgRemainingRPC(t, svc, c, "load_entity_finish")
			pgRemainingRPC(t, svc, c, "battle_fighting", map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})
			uuid := c.SelectedAvatarUnsafe().Progress.Battle.UUID
			emit := func(seq int, kind string, data map[string]any) []game.Push {
				return pgRemainingRPC(t, svc, c, "do_command", "__battle_event__", []any{map[string]any{"battle_uuid": uuid, "sequence": seq, "kind": kind, "data": data}})
			}
			emit(1, "ready", map[string]any{"version": 1})
			emit(2, "started", map[string]any{"units": []any{map[string]any{"eid": "1", "role": 1, "hex": []int{0, 0, 0}, "kind": "ally"}}})
			before := game.CloneProgress(c.SelectedAvatarUnsafe().Progress)
			svc.Accounts = &feedbackSQLRollbackStore{Store: store, code: code, left: 1}
			result := map[string]any{"winner_eids": []string{hex.EncodeToString(av.OID)}}
			emit(3, "result", result)
			read := func() game.Progress {
				identity, err := New(store.pool).QuickLogin(ctx, info)
				if err != nil {
					t.Fatal(err)
				}
				return identity.Avatars[0].Progress
			}
			frozen := read()
			if !frozen.Battle.RewardPlanFrozen || frozen.Battle.Finished || !reflect.DeepEqual(before.Runes, frozen.Runes) {
				t.Fatal("真实PG回滚泄漏资产或没有冻结奖励")
			}
			now = now.Add(time.Second)
			out, err := svc.Tick(ctx, c)
			settled := read()
			found := false
			for _, item := range out {
				found = found || item.Method == "battle_result"
			}
			if err != nil || !found || !settled.Battle.Finished || settled.Battle.UUID != uuid || !reflect.DeepEqual(frozen.Battle.RewardBonusPlan, settled.Battle.RewardBonusPlan) {
				t.Fatal("SQL冲突后Tick未兑现原冻结奖励", err)
			}
			emit(3, "result", result)
			if !reflect.DeepEqual(settled, read()) {
				t.Fatal("重复结果改变真实PG资产或收据")
			}
		})
	}
}
