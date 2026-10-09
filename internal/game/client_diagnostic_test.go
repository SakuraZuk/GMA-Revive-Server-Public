package game

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestFinishedUIGuidePersistenceAndNoGameplayAuthority(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error { return nil }); err != nil {
		t.Fatal(err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	for retry := 0; retry < 2; retry++ {
		out, err := s.Handle(ctx, c, "finished_guide", rawArgs(13))
		if err != nil || len(out) != 0 {
			t.Fatal("原生通知不得依赖callback", out, err)
		}
	}
	after := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if !reflect.DeepEqual(after.FinishedGuides, []int{13}) {
		t.Fatal("界面引导未保存或重复")
	}
	before.FinishedGuides = after.FinishedGuides
	if !reflect.DeepEqual(before, after) {
		t.Fatal("界面引导改变资产、教学或通关")
	}
	stored, _ := accounts.QuickLogin(ctx, ClientInfo{Account: av.Account, Password: "pw", Hostnum: av.Hostnum})
	if len(stored.Avatars) == 0 || !containsInt(stored.Avatars[0].Progress.FinishedGuides, 13) {
		t.Fatal("重登丢失界面引导")
	}
	for _, args := range [][]json.RawMessage{rawArgs(-1), rawArgs(999999), rawArgs(true), rawArgs(13, 1)} {
		if _, err := s.Handle(ctx, c, "finished_guide", args); err == nil {
			t.Fatal("非法界面引导被接受")
		}
	}
	if !canEnterDungeon(&Progress{GuideTasks: map[int]GuideTask{1014: {Status: 1}}}, 505) {
		t.Fatal("界面教学封锁普通副本")
	}
	if canEnterDungeon(&Progress{GuideTasks: map[int]GuideTask{1000: {Status: 1}}}, 505) {
		t.Fatal("初始教学越级")
	}
}

func TestClientDiagnosticRedactionAndNoAssets(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error { return nil }); err != nil {
		t.Fatal(err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	for i := 0; i < 65; i++ {
		out, err := s.Handle(ctx, c, "client_sa_log", rawArgs("hs_client_error", map[string]any{"message": "角色界面异常", "token": "不得输出"}))
		if err != nil || len(out) != 0 {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("诊断上报修改存档")
	}
	raw, _ := json.Marshal(redactDiagnostic(map[string]any{"nested": map[string]any{"password": "不得输出"}}))
	if strings.Contains(string(raw), "不得输出") {
		t.Fatal("诊断泄露凭据")
	}
}
