package game

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestMailExperiencePowerKnowledgeAtomicAndRetry(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, a, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error { p.Power.Value = p.Power.Max; return nil }); err != nil {
		t.Fatal(err)
	}
	oldPower := c.SelectedAvatarUnsafe().Progress.Power.Value
	if err := s.IssueMail(ctx, c, Mail{MID: 610, Title: "经验与体力", Attachments: map[int]int64{1: 50, 2: 1190, 4: 100}}); err != nil {
		t.Fatal(err)
	}
	id, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress.ShortMailInfo[610].oid())
	pushes, err := s.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage(`7`), id})
	if err != nil {
		t.Fatal(err)
	}
	av := c.SelectedAvatarUnsafe()
	p := av.Progress
	if av.Info.Level != 2 || p.AvatarExp != 0 || p.Power.Value != oldPower+50 || p.Materials[2].Count != 0 || p.Materials[1].Count != 0 || p.Materials[4].Count != 100 {
		t.Fatalf("附件语义错误:%d %+v", av.Info.Level, p)
	}
	seenLevel := false
	for _, push := range pushes {
		if push.Method == "on_avatar_level_up" {
			seenLevel = true
		}
	}
	if !seenLevel {
		t.Fatal("原生升级通知未发出")
	}
	if _, err = s.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage(`8`), id}); err == nil {
		t.Fatal("重复领取未拒绝")
	}
	if err = s.IssueMail(ctx, c, Mail{MID: 611, Title: "整笔回滚", Attachments: map[int]int64{1: 1, 2: 1230, 12: 1}}); err != nil {
		t.Fatal(err)
	}
	if err = s.updateProgress(ctx, c, func(p *Progress) error {
		p.Materials[12] = Material{ID: 12, Count: math.MaxInt64, Total: math.MaxInt64}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	id, _ = json.Marshal(before.ShortMailInfo[611].oid())
	if _, err = s.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage(`9`), id}); err == nil {
		t.Fatal("溢出未拒绝")
	}
	after := c.SelectedAvatarUnsafe().Progress
	if after.AvatarLevel != before.AvatarLevel || after.AvatarExp != before.AvatarExp || after.Power.Value != before.Power.Value || after.ShortMailInfo[611].State != before.ShortMailInfo[611].State {
		t.Fatal("失败邮件经验或体力未回滚")
	}
}
