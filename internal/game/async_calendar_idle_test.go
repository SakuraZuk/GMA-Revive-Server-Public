package game

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestAsyncFirstEntryDoesNotAwardDormantDays(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	now := time.Date(2026, 10, 11, 13, 1, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	id := hexOf(selectedOID(cs[0]))
	_, err := store.UpdateSocial(context.Background(), []string{id}, func(v map[string]*Avatar) error {
		p := &v[id].Progress
		p.AsyncPvp = AsyncPvpState{RewardDay: "2026-10-01", RewardWeek: "2026-09-27"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Handle(context.Background(), cs[0], "enter_asyn_pvp", nil); err != nil {
		t.Fatal(err)
	}
	p := socialSaved(t, store, cs[0]).Progress
	if p.AsyncPvp.Score != 1000 || p.AsyncPvp.RewardDay != "2026-10-11" || p.AsyncPvp.RewardWeek != "2026-10-11" {
		t.Fatal("实际入口没有建立首次参赛基线", p.AsyncPvp)
	}
	if err = s.refreshPvpAwards(context.Background(), cs[0]); err != nil {
		t.Fatal(err)
	}
	if len(socialSaved(t, store, cs[0]).Progress.ShortMailInfo) != 0 {
		t.Fatal("首次参赛补发了休眠期间奖励")
	}
	now = now.AddDate(0, 0, 1)
	if err = s.refreshPvpAwards(context.Background(), cs[0]); err != nil {
		t.Fatal(err)
	}
	if len(socialSaved(t, store, cs[0]).Progress.ShortMailInfo) != 1 {
		t.Fatal("参赛后的下一日正常奖励丢失")
	}
}

// 跨21点不为从未参赛的角色改写空奖励日期；首次参与仍建立真实基线。
func TestAsyncIdleCalendarNoWriteAndFirstParticipation(t *testing.T) {
	now := time.Date(2026, 10, 11, 13, 1, 0, 0, time.UTC)
	s := New(nil, nil)
	for _, old := range []string{"", "2026-10-01"} {
		p := NewProgress(10, now)
		p.AsyncPvp = AsyncPvpState{RewardDay: old, RewardWeek: old}
		p.Social.Assist.Day = socialDay(now)
		p.Social.Assist.Active = map[string]int{}
		before, _ := json.Marshal(p)
		av := &Avatar{Progress: p}
		day, week := pvpAwardCutoffs(now)
		if err := s.settlePvpCalendar(map[string]*Avatar{"角色": av}, now, day, week); err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(av.Progress)
		if string(before) != string(after) {
			t.Fatal("从未参赛的角色被空奖励结算修改")
		}
		ensureAsyncCalendar(&av.Progress.AsyncPvp, now)
		ensureAsync(&av.Progress.AsyncPvp)
		if av.Progress.AsyncPvp.RewardDay != day || av.Progress.AsyncPvp.RewardWeek != week {
			t.Fatal("首次参赛没有替换休眠期间的旧基线")
		}
		// 已参赛后重进不得推进日期，避免吞掉下一轮应发奖励。
		ensureAsyncCalendar(&av.Progress.AsyncPvp, now.AddDate(0, 0, 8))
		if av.Progress.AsyncPvp.RewardDay != day || av.Progress.AsyncPvp.RewardWeek != week {
			t.Fatal("已有积分的奖励基线被重进覆盖")
		}
	}
}

func TestAsyncIdleCalendarStillRejectsClockRollback(t *testing.T) {
	now := time.Date(2026, 10, 11, 13, 1, 0, 0, time.UTC)
	av := &Avatar{Progress: Progress{AsyncPvp: AsyncPvpState{RewardDay: "2099-01-01"}}}
	ensureAsyncCalendar(&av.Progress.AsyncPvp, now)
	if av.Progress.AsyncPvp.RewardDay != "2099-01-01" {
		t.Fatal("首次参赛覆盖了异常未来日期")
	}
	day, week := pvpAwardCutoffs(now)
	if err := New(nil, nil).settlePvpCalendar(map[string]*Avatar{"角色": av}, now, day, week); err == nil {
		t.Fatal("零积分绕过了已有日期回拨检查")
	}
}
