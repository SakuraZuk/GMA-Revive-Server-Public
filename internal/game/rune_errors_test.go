package game

import (
	"context"
	"errors"
	"testing"
	"time"
)

type runeFailedProgressStore struct{ *FixtureAccounts }

func (r runeFailedProgressStore) UpdateProgress(context.Context, []byte, func(*Progress) error) (Progress, error) {
	return Progress{}, errors.New("数据库故障注入")
}

func TestRuneNativeErrorCodesAndInfrastructureFailure(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	id := "00112233445566778899aabb"
	r, err := GenerateRune(RuneSpec{Suit: 1101, Position: 1, Star: 5, Level: 11}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r.UUID = id
	_, err = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Runes = map[string]Rune{id: r}
		p.Materials[36] = Material{ID: 36, Count: 0}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		method string
		args   []any
		code   int
	}{
		{"unembed_rune", []any{7, id}, 5104},
		{"lock_rune", []any{7, "ffffffffffffffffffffffff"}, 5101},
		{"up_level_rune", []any{7, id, 1}, 5102},
		{"change_rune_extra_attr", []any{7, id, 99, 20504}, 5127},
		{"change_rune_extra_attr", []any{7, id, 0, 999999}, 5128},
		{"change_rune_extra_attr", []any{7, id, 0, r.ExtraAttrs[0]}, 5129},
		{"embed_rune", []any{7, "ffffffffffffffffffffffff", id}, 3},
	}
	for _, tc := range cases {
		pushes, err := s.Handle(ctx, c, tc.method, runeTestArgs(tc.args...))
		if err != nil || len(pushes) != 1 || pushes[0].Args[1].([]any)[0] != tc.code {
			t.Fatalf("%s 原生回码错误: %v %#v", tc.method, err, pushes)
		}
	}
	for _, attr := range r.ExtraAttrsLib {
		if attr != r.ExtraAttrs[0] {
			pushes, err := s.Handle(ctx, c, "change_rune_extra_attr", runeTestArgs(7, id, 0, attr))
			if err != nil || len(pushes) != 1 || pushes[0].Args[1].([]any)[0] != 5 {
				t.Fatalf("材料不足回码错误: %v %#v", err, pushes)
			}
			break
		}
	}
	s.Accounts = runeFailedProgressStore{accounts}
	pushes, err := s.Handle(ctx, c, "lock_rune", runeTestArgs(8, id))
	if err == nil || len(pushes) != 0 {
		t.Fatal("基础设施故障被伪装成业务回码")
	}
}
