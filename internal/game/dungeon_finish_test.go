package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestFinishedDungeonEventNativeSignatureNoRewards(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	for _, args := range [][]json.RawMessage{nil, {json.RawMessage("10001"), json.RawMessage("0")}, {json.RawMessage("999999999")}, {json.RawMessage("{}")}} {
		if _, err := s.Handle(ctx, c, "finished_dungeon_event", args); err == nil {
			t.Fatal("非法完成签名被接受")
		}
	}
	for i := 0; i < 2; i++ {
		pushes, err := s.Handle(ctx, c, "finished_dungeon_event", []json.RawMessage{json.RawMessage("10001")})
		if err != nil || len(pushes) != 0 {
			t.Fatal("原生完成上报失败", err)
		}
	}
	if !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("剧情上报修改结算或重复发奖")
	}
}
