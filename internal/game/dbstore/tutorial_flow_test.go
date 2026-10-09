package dbstore

import (
	"context"
	"hs-server/internal/game"
	"testing"
	"time"
)

// 实库验证完整教学状态链；不等价于客户端动作验收。
func TestPostgresFullTutorialRecovery(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "完整教学持久化验收", Password: "pw", Hostnum: 1}
	id, err := s.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	oid := id.Avatars[0].OID
	now := time.Now()
	for task := 1000; task <= 1013; task++ {
		if _, err = s.UpdateProgress(ctx, oid, func(p *game.Progress) error { return game.AdvanceGuide(p, task, now, 1) }); err != nil {
			t.Fatal(task, err)
		}
	}
	for _, step := range []struct{ task, dungeon int }{{1014, 504}, {2001, 506}, {2002, 509}, {2003, 510}} {
		if _, err = s.UpdateProgress(ctx, oid, func(p *game.Progress) error { return game.AdvanceGuide(p, step.task, now, 1) }); err == nil {
			t.Fatal("未通关不得越级完成", step.task)
		}
		if _, err = s.UpdateProgress(ctx, oid, func(p *game.Progress) error { p.ClearedDungeons = append(p.ClearedDungeons, step.dungeon); return nil }); err != nil {
			t.Fatal(err)
		}
		// 模拟旧版本停在等待态；新Store登录应在行锁内迁移并落库。
		reloaded, loginErr := New(s.pool).QuickLogin(ctx, info)
		if loginErr != nil || reloaded.Avatars[0].Progress.GuideTasks[step.task].Status != 1 {
			t.Fatal("重登未恢复教学执行态", step.task, loginErr)
		}
		for retry := 0; retry < 2; retry++ {
			if _, err = s.UpdateProgress(ctx, oid, func(p *game.Progress) error { return game.AdvanceGuide(p, step.task, now, 1) }); err != nil {
				t.Fatal(err)
			}
		}
		reloaded, loginErr = New(s.pool).QuickLogin(ctx, info)
		if loginErr != nil || reloaded.Avatars[0].Progress.GuideTasks[step.task].Status != 2 {
			t.Fatal("完成状态未持久化", step.task, loginErr)
		}
	}
}
