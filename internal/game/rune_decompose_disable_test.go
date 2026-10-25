package game

import (
	"context"
	"reflect"
	"testing"
)

func TestDisableRuneDecomposePersistsExplicitEmpty(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	if _, err := s.Handle(ctx, c, "save_rune_auto_decompose_stars", rawArgs(10, []int{1, 4})); err != nil {
		t.Fatal(err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	for _, args := range [][]any{{0}, {11, 1}, {"非法"}} {
		if _, err := s.Handle(ctx, c, "remove_rune_auto_decompose_stars", rawArgs(args...)); err == nil {
			t.Fatal("非法关闭参数被接受")
		}
	}
	if !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("非法关闭改变存档")
	}
	for i := 0; i < 2; i++ {
		out, err := s.Handle(ctx, c, "remove_rune_auto_decompose_stars", rawArgs(12))
		if err != nil || len(out) != 2 || out[0].Method != "client_prop_changed" || out[1].Args[1].([]any)[0] != RetSuccess {
			t.Fatal("关闭未先同步再回调", err, out)
		}
		prop := out[0].Args[0].([]any)
		if prop[0] != "rune_auto_decompose_stars" || !reflect.DeepEqual(prop[1], []int{}) {
			t.Fatal("关闭未投影空列表")
		}
	}
	id, err := accounts.QuickLogin(ctx, ClientInfo{Account: av.Account, Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := id.Avatars[0].Progress
	if p.RuneAutoDecomposeStars == nil || len(*p.RuneAutoDecomposeStars) != 0 || !reflect.DeepEqual(id.Avatars[0].InitialProperties(av.Account)["rune_auto_decompose_stars"], []int{}) {
		t.Fatal("重登恢复成默认自动分解")
	}
	before.RuneAutoDecomposeStars = p.RuneAutoDecomposeStars
	if !reflect.DeepEqual(before, p) {
		t.Fatal("关闭改变其他存档或资产")
	}
}
