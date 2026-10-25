package dbstore

import (
	"context"
	"hs-server/internal/game"
	"reflect"
	"testing"
	"time"
)

func TestPostgresDisableRuneDecomposeColdPersistence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	info := game.ClientInfo{Account: "契印关闭验收", Password: "pw", Hostnum: 1}
	svc, c, _ := repairPlayer(t, s, info, &now)
	defer svc.Detach(c)
	for _, call := range []struct {
		name string
		args []any
	}{
		{"save_rune_auto_decompose_stars", []any{1, []int{1, 4}}},
		{"remove_rune_auto_decompose_stars", []any{2}},
		{"remove_rune_auto_decompose_stars", []any{3}},
	} {
		before := game.CloneProgress(c.SelectedAvatarUnsafe().Progress)
		out, err := svc.Handle(ctx, c, call.name, pgSocialArgs(call.args...))
		if err != nil || len(out) != 2 || out[1].Args[1].([]any)[0] != game.RetSuccess {
			t.Fatal("设置或关闭实库调用失败", err, out)
		}
		if call.name == "remove_rune_auto_decompose_stars" {
			id, err := New(s.pool).QuickLogin(ctx, info)
			if err != nil {
				t.Fatal(err)
			}
			p := id.Avatars[0].Progress
			if p.RuneAutoDecomposeStars == nil || len(*p.RuneAutoDecomposeStars) != 0 || !reflect.DeepEqual(id.Avatars[0].InitialProperties(info.Account)["rune_auto_decompose_stars"], []int{}) {
				t.Fatal("关闭状态未持久化为显式空列表")
			}
			before.RuneAutoDecomposeStars = p.RuneAutoDecomposeStars
			if !reflect.DeepEqual(before, p) {
				t.Fatal("关闭意外修改资产或其他状态")
			}
		}
	}
}
