package game

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestStorylineRPCPersistenceAndNoAssetAuthority(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error { return nil }); err != nil {
		t.Fatal(err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	for retry := 0; retry < 2; retry++ {
		out, err := s.Handle(ctx, c, "finished_storyline", rawArgs(71, "cthulhu_1_0_over"))
		if err != nil || len(out) != 2 || out[0].Method != "client_prop_set" || out[1].Method != "call_client_callback" || out[1].Args[0] != 71 {
			t.Fatal("剧情桶必须先于原生回调且重试幂等", out, err)
		}
		if !reflect.DeepEqual(out[0].Args, []any{[]any{"extra_info_mgr", "storyline", storylineProperties(c.SelectedAvatarUnsafe().Progress)}}) {
			t.Fatal("剧情桶参数不能多嵌套或丢字段", out[0])
		}
	}
	after := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if !reflect.DeepEqual(after.PlayedStorylines, []string{"cthulhu_1_0_over"}) {
		t.Fatal("剧情重复或缺失", after.PlayedStorylines)
	}
	before.PlayedStorylines = after.PlayedStorylines
	if !reflect.DeepEqual(before, after) {
		t.Fatal("观影记录不得修改资产、通关、引导或系统解锁")
	}
	stored, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(stored.PlayedStorylines, after.PlayedStorylines) {
		t.Fatal("剧情记录未持久化", stored.PlayedStorylines, err)
	}
	c.refreshLoginSent = false
	out, err := s.Handle(ctx, c, "set_reconnect_auth_msg", rawArgs("剧情冷登录"))
	if err != nil || len(out) < 2 || out[0].Method != "client_prop_set" || out[len(out)-1].Method != "on_refresh_login" {
		t.Fatal("冷登录必须先恢复剧情桶再启动场景", out, err)
	}
	props := (Avatar{Progress: stored}).InitialProperties("剧情验收")
	if _, leaked := props["server_played_storylines"]; leaked || props["extra_info_mgr"] == nil {
		t.Fatal("客户端只接收原生剧情结构", props)
	}
	copy := CloneProgress(stored)
	copy.PlayedStorylines[0] = "不污染存档"
	if stored.PlayedStorylines[0] != "cthulhu_1_0_over" {
		t.Fatal("剧情切片克隆未隔离")
	}
}

func TestStorylineRejectMalformedBeforeMutation(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, s)
	for _, args := range [][]json.RawMessage{rawArgs(1, ""), rawArgs(1, strings.Repeat("a", 257)), rawArgs(0, "故事"), rawArgs(1, "换行\n"), rawArgs(1, "\uFFFD"), rawArgs(1, 3), rawArgs(1)} {
		if _, err := s.Handle(ctx, c, "finished_storyline", args); err == nil {
			t.Fatal("非法剧情参数不能写入存档", args)
		}
	}
	if len(c.SelectedAvatarUnsafe().Progress.PlayedStorylines) != 0 {
		t.Fatal("非法上报污染剧情")
	}
}
