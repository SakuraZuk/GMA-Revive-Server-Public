package game

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestLeagueProtectActualRecoveryMigratesConsumedUUIDAndLateWindowReceipt(t *testing.T) {
	s, c, _, now := leagueProtectFixture(t)
	remainingRPC(t, s, c, "league_protect_start", 1, 0)
	leagueProtectStartObserved(t, s, c)
	p := c.SelectedAvatarUnsafe().Progress
	old := p.Battle.UUID
	*now = now.Add(5 * 24 * time.Hour)
	remainingRPC(t, s, c, "client_need_recover_battle")
	p = c.SelectedAvatarUnsafe().Progress
	next := p.Battle.UUID
	if next == old || p.LeagueProtect.Consumed[next] != p.LeagueProtect.Consumed[old] {
		t.Fatal("真实恢复没有将旧窗口计次迁移新UUID")
	}
	leagueProtectStartObserved(t, s, c)
	p = c.SelectedAvatarUnsafe().Progress
	if p.LeagueProtect.TotalWeekly != 0 {
		t.Fatal("旧窗口已开始战斗恢复误扣新窗口")
	}
	out := leagueProtectResultObserved(t, s, c, 50, 0, true)
	if battleTestResult(out) == nil {
		t.Fatal("真实恢复后结果没有结算")
	}
	p = c.SelectedAvatarUnsafe().Progress
	if p.LeagueProtect.TotalWeekly != 0 || !p.LeagueProtect.Unlocked[1] || len(p.LeagueProtect.Results) != 1 {
		t.Fatal("迟到旧窗口结果污染新窗口次数或丢失结果")
	}
	before := CloneProgress(p)
	out = remainingRPC(t, s, c, "client_need_recover_battle")
	if battleTestResult(out) == nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("已完成恢复没有仅重放原生收据")
	}
	if _, err := s.Handle(context.Background(), c, "league_protect_start", socialArgs(2, 0)); err != nil {
		t.Fatal("当前新窗口资格被旧场消费", err)
	}
}
