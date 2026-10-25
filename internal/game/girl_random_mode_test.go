package game

import (
	"context"
	"encoding/json"
	"testing"
)

func TestGirlRandomNativeModesAndColdProjection(t *testing.T) {
	ctx := context.Background()
	store := NewFixtureAccounts(nil)
	s := New(store, nil)
	c, av := newBattleConnection(t, ctx, store, s)
	for _, mode := range []int{1, 2, 0, 2} {
		out, err := s.Handle(ctx, c, "set_girl_random_enable", rawArgs(81, mode))
		if err != nil || len(out) != 3 || out[2].Args[1].([]any)[0] != RetSuccess {
			t.Fatal("原生整数模式被拒绝", mode, err, out)
		}
		if pair := out[0].Args[0].([]any); pair[0] != "girl_random_type" || pair[1] != mode {
			t.Fatal("模式未先于开关回推")
		}
		identity, err := store.QuickLogin(ctx, ClientInfo{Account: av.Account, Password: "pw", Hostnum: 1})
		if err != nil {
			t.Fatal(err)
		}
		props := identity.Avatars[0].InitialProperties(av.Account)
		if props["girl_random_type"] != mode || props["girl_random_enable"] != (mode != 0) {
			t.Fatal("冷登录丢失随机范围", props["girl_random_type"])
		}
	}
	for _, invalid := range []any{-1, 3, 1.5, nil, "2"} {
		before, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
		out, err := s.Handle(ctx, c, "set_girl_random_enable", rawArgs(82, invalid))
		after, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
		if err != nil || out[len(out)-1].Args[1].([]any)[0] == RetSuccess || string(before) != string(after) {
			t.Fatal("无效随机模式未完整拒绝", invalid)
		}
	}
	for _, enabled := range []bool{false, true} {
		legacy := Progress{GirlRandomEnable: &enabled}
		want := 0
		if enabled {
			want = 1
		}
		if girlRandomMode(legacy) != want {
			t.Fatal("旧布尔存档兼容错误")
		}
	}
}
