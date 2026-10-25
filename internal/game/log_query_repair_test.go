package game

import (
	"context"
	"reflect"
	"testing"
)

func TestLogQueryNativeShapesAndFocusKeepsBattleState(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	for _, row := range []struct {
		method string
		args   []any
		reply  string
	}{
		{"query_phone_info", nil, "on_query_phone_info"},
		{"request_chat_channel", nil, "on_change_channel"},
		{"home_enter_quit_log", []any{1, 2, -1}, ""},
		{"share_click_log", []any{"summon", "channel"}, ""},
		{"notice_share", nil, ""},
		{"receive_package_check", []any{map[string]any{"client_name": "com.netease.hsqsl"}}, ""},
		{"close_asyn_pvp_ui", nil, ""},
	} {
		out, err := s.Handle(ctx, c, row.method, rawArgs(row.args...))
		if err != nil {
			t.Fatal(row.method, err)
		}
		if row.reply != "" && (len(out) == 0 || out[len(out)-1].Method != row.reply) {
			t.Fatal(row.method, out)
		}
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.Battle = &BattleSession{UUID: "会话", DungeonID: 601, Started: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	out, err := s.Handle(ctx, c, "set_focus_target", rawArgs("target", 1))
	if err != nil || len(out) != 1 || out[0].Method != "sync_battle_method" || out[0].Args[0] != "set_focus_target" || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("聚焦改变存档或原生下行丢失", err, out)
	}
	if _, err = s.Handle(ctx, c, "set_focus_target", rawArgs(123, 1)); err == nil {
		t.Fatal("聚焦接受错误类型")
	}
	for _, target := range []string{"", "1", "0123456789abcdef01234567"} {
		out, err = s.Handle(ctx, c, "set_focus_target", rawArgs(target, 1))
		if err != nil {
			t.Fatal(err)
		}
		values := out[0].Args[1].([]any)
		if _, ok := values[0].(ObjectID); !ok {
			t.Fatal("操作者缺原生实体类型")
		}
		if len(target) == 24 {
			if values[1] != ObjectID(target) {
				t.Fatal("对象目标缺原生实体类型")
			}
		} else if values[1] != target {
			t.Fatal("取消或槽位编号被改变")
		}
	}
}

func TestSearchFriendUsesNativeNicknameUIDServerOrder(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	out, err := s.Handle(ctx, c, "search_friend", rawArgs(91, "不存在的角色", 0, 1, true))
	if err != nil || len(out) != 1 || out[0].Method != "call_client_callback" || len(out[0].Args[1].([]any)) != 2 {
		t.Fatal("原生搜索参数顺序仍被拒绝", err, out)
	}
}
