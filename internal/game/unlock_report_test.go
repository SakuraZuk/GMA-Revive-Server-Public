package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestUnlockReportRechecksOriginalConditionsAndStorySuccessors(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.GuideTasks = map[int]GuideTask{}
		p.Power.Value = 9998
		p.ClearedDungeons = []int{506, 509, 510}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{507, 511} {
		out, err := s.Handle(ctx, c, "enter_dungeon", rawArgs(71, id, map[string]any{}))
		if err != nil || battleTestResult(out) == nil {
			t.Fatal("剧情解锁节点未推进", id, err)
		}
	}
	// 服务端按原表授权恢复已满足但旧存档未记录的解锁，不信任玩家直接点亮名称。
	p := c.SelectedAvatarUnsafe().Progress
	var eligible, locked string
	for _, def := range clientBaseline.SystemUnlocks {
		raw, _ := json.Marshal(def.Conditions)
		err := (activityRow{"unlock_condition": raw}).conditions("unlock_condition", p, p.AvatarLevel)
		if err == nil && len(def.Conditions) > 0 && eligible == "" {
			eligible = def.System
		}
		if err != nil && locked == "" {
			locked = def.System
		}
	}
	if eligible == "" || locked == "" {
		t.Fatal("解锁表没有可验证的已满足和未满足条件")
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error { delete(p.UnlockSystems, eligible); return nil }); err != nil {
		t.Fatal(err)
	}
	out, err := s.Handle(ctx, c, "on_unlock_system", rawArgs([]string{eligible}))
	if err != nil || len(out) != 1 || c.SelectedAvatarUnsafe().Progress.UnlockSystems[eligible] == 0 {
		t.Fatal("已满足条件未补回解锁", err, out)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if _, err := s.Handle(ctx, c, "on_unlock_system", rawArgs([]string{locked})); err == nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("未满足条件被客户端越权解锁", err)
	}
	for _, id := range []int{508, 512} {
		if err := s.updateProgress(ctx, c, func(p *Progress) error { p.Battle = nil; return nil }); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Handle(ctx, c, "enter_dungeon", rawArgs(72, id, map[string]any{})); err != nil {
			t.Fatal("1-7/1-11后继入口失败", id, err)
		}
	}
}
