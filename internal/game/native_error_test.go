package game

import (
	"context"
	"reflect"
	"testing"
)

func TestNativeErrorRPCExactContractAndFailedBusinessRollback(t *testing.T) {
	expected := Push{Target: "Avatar", Method: "handle_error_msg", Args: []any{123, []any{}}}
	if out := nativeErrorPush(123); !reflect.DeepEqual(out, expected) {
		t.Fatal("原生错误入口必须为两个位置参数Int和非nil空Tuple", out)
	}
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, s)
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	for _, call := range []func() ([]Push, error){
		func() ([]Push, error) { return s.freeStageRPC(ctx, c, "enter_free_stage", rawArgs(999999, false)) },
		func() ([]Push, error) { return s.freeStageSettingsRPC(ctx, c, "set_auto_list", rawArgs([]int{999999})) },
		func() ([]Push, error) { return s.cthulhuRPC(ctx, c, "set_cthulhu_title", rawArgs("")) },
	} {
		out, err := call()
		if err != nil || len(out) != 1 || out[0].Target != "Avatar" || out[0].Method != "handle_error_msg" || len(out[0].Args) != 2 || !reflect.DeepEqual(out[0].Args[1], []any{}) {
			t.Fatal("真实失败分支必须走注册错误RPC", out, err)
		}
		if !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
			t.Fatal("失败事务不得修改进度或资产")
		}
	}
}
