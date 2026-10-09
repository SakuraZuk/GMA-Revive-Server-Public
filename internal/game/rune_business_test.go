package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestRuneRPCEmbedLockUnembedAndDecompose(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, service)
	card := newCard(4401, 1, service.Now())
	runeID := "00112233445566778899aabb"
	generated, err := GenerateRune(RuneSpec{Suit: 1101, Position: 1, Star: 1, Level: 1}, service.Now())
	if err != nil {
		t.Fatal(err)
	}
	generated.UUID = runeID
	if _, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{card}
		p.Materials[4] = Material{ID: 4, Count: 100000, Total: 100000}
		p.Runes = map[string]Rune{runeID: generated}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(ctx, c, "embed_rune", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`"` + card.UUID + `"`), json.RawMessage(`"` + runeID + `"`)}); err != nil {
		t.Fatal(err)
	}
	got, _ := c.SelectedAvatar()
	if got.Progress.Runes[runeID].CardUUID != card.UUID {
		t.Fatalf("契印未镶嵌: %#v", got.Progress.Runes[runeID])
	}
	if _, err := service.Handle(ctx, c, "lock_rune", []json.RawMessage{json.RawMessage(`2`), json.RawMessage(`"` + runeID + `"`)}); err != nil {
		t.Fatal(err)
	}
	refused, err := service.Handle(ctx, c, "decompose_runes", []json.RawMessage{json.RawMessage(`3`), json.RawMessage(`[` + `"` + runeID + `"` + `]`)})
	if err != nil || len(refused) != 1 {
		t.Fatalf("拒绝分解未回调: %v %#v", err, refused)
	}
	got, _ = c.SelectedAvatar()
	if _, exists := got.Progress.Runes[runeID]; !exists {
		t.Fatal("已镶嵌/锁定契印不应分解")
	}
	if _, err := service.Handle(ctx, c, "unlock_rune", []json.RawMessage{json.RawMessage(`4`), json.RawMessage(`"` + runeID + `"`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(ctx, c, "unembed_rune", []json.RawMessage{json.RawMessage(`5`), json.RawMessage(`"` + runeID + `"`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(ctx, c, "decompose_runes", []json.RawMessage{json.RawMessage(`6`), json.RawMessage(`[` + `"` + runeID + `"` + `]`)}); err != nil {
		t.Fatal(err)
	}
	got, _ = c.SelectedAvatar()
	if _, ok := got.Progress.Runes[runeID]; ok {
		t.Fatal("分解后契印仍存在")
	}
}

func runeTestArgs(values ...any) []json.RawMessage {
	args := make([]json.RawMessage, len(values))
	for i, value := range values {
		args[i], _ = json.Marshal(value)
	}
	return args
}

func TestRuneDecomposeAggregatesDeduplicatesAndRollsBack(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	ids := []string{"00112233445566778899aabb", "00112233445566778899aabc"}
	before, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Runes = map[string]Rune{}
		for _, id := range ids {
			p.Runes[id] = Rune{UUID: id, Star: 1, Position: 1, Level: 1}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// 在前一枚已暂存入账后遇到缺失 UUID，全部资源和契印都必须回滚。
	refused, err := s.Handle(ctx, c, "decompose_runes", runeTestArgs(7, []string{ids[0], "00112233445566778899aabd"}))
	if err != nil || len(refused) != 1 {
		t.Fatalf("失败未返回原生空 box: %v %#v", err, refused)
	}
	after, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("失败分解部分写入资源")
	}
	pushes, err := s.Handle(ctx, c, "decompose_runes", runeTestArgs(8, []string{ids[0], ids[1], ids[0]}))
	if err != nil || len(pushes) != 4 {
		t.Fatalf("批量分解失败: %v %#v", err, pushes)
	}
	box := pushes[3].Args[1].([]any)[0].(map[string]any)
	if box["__custom_type"] != "box.box" {
		t.Fatal("分解回包缺少原生类型标记")
	}
	materials := box["materials"].(map[string]any)
	tables, _ := loadRuneTables()
	for _, row := range tables.Tables.Levels {
		if row.Star == 1 && row.Level == 1 {
			for _, gain := range row.Decompose {
				want := int64(gain[1]) * 2
				got := c.SelectedAvatarUnsafe().Progress.Materials[gain[0]].Count - before.Materials[gain[0]].Count
				if got != want || materials[itoa(gain[0])] != want {
					t.Fatalf("分解累计错误: 余额=%d box=%v 期望=%d", got, materials, want)
				}
			}
		}
	}
}

func TestRuneResetRejectsInjectedAttributesAndPreservesAcquiredTotal(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	id := "00112233445566778899aabb"
	before, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Runes = map[string]Rune{id: {UUID: id, Star: 5, Position: 1, Level: 3, ExtraAttrsCount: 1, ExtraAttrs: []int{20501}, ExtraAttrsLib: []int{20501, 20502}}}
		p.Materials[36] = Material{ID: 36, Count: 10, Total: 10}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, attr := range []int{999999, 20501} {
		want := 5128
		if attr == 20501 {
			want = 5129
		}
		pushes, err := s.Handle(ctx, c, "change_rune_extra_attr", runeTestArgs(9, id, 0, attr))
		if err != nil || len(pushes) != 1 || pushes[0].Args[1].([]any)[0] != want {
			t.Fatalf("非法洗练没有失败回调: %v %#v", err, pushes)
		}
		after, _ := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error { return nil })
		if !reflect.DeepEqual(before, after) {
			t.Fatal("非法洗练修改了词条或材料")
		}
	}
	pushes, err := s.Handle(ctx, c, "change_rune_extra_attr", runeTestArgs(10, id, 0, 20502))
	pushes = checkedCorePushes(t, pushes, c.SelectedAvatarUnsafe().Progress)
	if err != nil || len(pushes) != 4 {
		t.Fatalf("合法洗练失败: %v %#v", err, pushes)
	}
	got := c.SelectedAvatarUnsafe().Progress
	if got.Runes[id].ExtraAttrs[0] != 20502 || got.Materials[36].Count != 9 || got.Materials[36].Total != 10 {
		t.Fatalf("洗练资产错误: %#v", got)
	}
}

func TestRuneWireUsesNativeFieldsAndClearsEmptyCardSlots(t *testing.T) {
	r := Rune{UUID: "00112233445566778899aabb", Star: 5, Position: 1, Level: 3, Suit: 1001, Locked: true, BaseAttrs: []int{10501}, ExtraAttrsCount: 1}
	wire := runeProperties(r)
	if wire["lock"] != 1 || wire["card_uuid"] != nil {
		t.Fatalf("原生类型错误: %#v", wire)
	}
	for _, name := range []string{"base_attrs", "extra_attrs", "extra_attrs_lib"} {
		if _, ok := wire[name].([]int); !ok {
			t.Fatalf("%s 不是原生 List", name)
		}
	}
	card := newCard(4401, 1, time.Unix(1700000000, 0))
	if len(cardMgrPropertiesWithRunes([]Card{card}, nil)[card.UUID].(map[string]any)["embed_runes"].(map[string]any)) != 0 {
		t.Fatal("空槽无法清除")
	}
}
