package game

import (
	"context"
	"reflect"
	"testing"
)

func TestCardDressNativeContractAndOwnership(t *testing.T) {
	s, c, store, _ := basicFixture(t)
	ctx := context.Background()
	uuid := "00112233445566778899aabb"
	seed := func(grade int, owned []int) {
		t.Helper()
		if err := s.updateProgress(ctx, c, func(p *Progress) error {
			p.Cards = []Card{{UUID: uuid, CardID: 4401, Level: 10, Grade: grade, Dress: 440101}}
			p.OwnedDresses = map[int][]int{4401: owned}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	call := func(id string, dress, want int) []Push {
		t.Helper()
		out, err := s.Handle(ctx, c, "change_card_dress", rawArgs(907, id, dress))
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range out {
			if item.Method == "call_client_callback" {
				if !reflect.DeepEqual(item.Args[1], []any{want}) {
					t.Fatal("原生换装返回码错误", item, want)
				}
				return out
			}
		}
		t.Fatal("换装缺回调")
		return nil
	}
	seed(1, nil)
	out := call(uuid, 440102, 0)
	if len(out) < 2 || out[0].Method != "client_prop_changed" || out[0].Args[0].([]any)[0] != "card_mgr" || out[1].Method != "call_client_callback" {
		t.Fatal("换装属性未先于回调", out)
	}
	cold, err := store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if err != nil || cold.Cards[0].Dress != 440102 {
		t.Fatal("换装未持久化", err)
	}
	call(uuid, 440102, 5021)
	call("ffffffffffffffffffffffff", 440101, 3)
	call(uuid, 340701, 5022)
	call(uuid, 44010101, 5022)
	if c.SelectedAvatarUnsafe().Progress.Cards[0].Dress != 440102 {
		t.Fatal("拒绝修改了装帧")
	}
	call(uuid, 440101, 0)
	seed(0, nil)
	call(uuid, 440102, 5022)
	seed(0, []int{440102})
	call(uuid, 440102, 5023)
	// 同卡另一副本已觉醒，只能获得共享归属，不能绕过目标卡的觉醒检查。
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.Cards = append(p.Cards, Card{UUID: "ffeeddccbbaa998877665544", CardID: 4401, Grade: 1})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	call(uuid, 440102, 5023)
}
