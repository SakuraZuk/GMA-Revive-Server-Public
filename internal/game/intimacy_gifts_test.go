package game

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestIntimacyGiftRPCNativeBoxAndReturnRunes(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	_, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{newCard(4401, 1, time.Now())}
		p.Materials[523] = Material{ID: 523, Count: 2, Total: 2}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage(`19`), json.RawMessage(`4401`), json.RawMessage(`523`), json.RawMessage(`2`), json.RawMessage(`0`)}
	pushes, err := s.Handle(ctx, c, "consume_intimacy_gift", args)
	if err != nil || businessPushCount(pushes) != 5 {
		t.Fatal(err, pushes)
	}
	box := pushes[len(pushes)-1].Args[1].([]any)[1].(map[string]any)
	if box["__custom_type"] != "box.box" || len(box["runes"].(map[string]any)) != 8 || pushes[0].Method != "client_prop_changed" {
		t.Fatal("回礼box或原生推送顺序错误", pushes)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Intimacy[4401] != 1040 || p.Materials[523].Count != 0 || p.Materials[523].Total != 2 || len(p.Runes) != 8 {
		t.Fatal("送礼资产或好感度错误")
	}
	positions := map[int]int{}
	for _, r := range p.Runes {
		if r.Suit != 1901 || r.Star != 5 || !r.Locked {
			t.Fatal("回礼未按Android规格生成")
		}
		positions[r.Position]++
	}
	for pos := 1; pos <= 4; pos++ {
		if positions[pos] != 2 {
			t.Fatal("回礼位置数量错误")
		}
	}
	reloaded, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if !reflect.DeepEqual(p.Runes, reloaded.Runes) || reloaded.Intimacy[4401] != 1040 {
		t.Fatal("回礼或好感度未持久化")
	}
	before := CloneProgress(reloaded)
	pushes, err = s.Handle(ctx, c, "consume_intimacy_gift", args)
	rules, _ := loadIntimacyCatalog()
	if err != nil || businessPushCount(pushes) != 1 || pushes[0].Args[1].([]any)[0] != rules.Errors["RET_MATERIAL_NOT_ENOUGH"] {
		t.Fatal("缺材料未按原生拒绝", err, pushes)
	}
	after, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, after) {
		t.Fatal("失败重复请求仍发奖励")
	}
}

func TestIntimacyMultiGiftTransactionAndCapacityRollback(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	before, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{newCard(4401, 1, time.Now())}
		p.Materials[701] = Material{ID: 701, Count: 5, Total: 9}
		p.Materials[702] = Material{ID: 702, Count: 0, Total: 2}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage(`21`), json.RawMessage(`4401`), json.RawMessage(`{"701":2,"702":1}`)}
	pushes, err := s.Handle(ctx, c, "consume_multi_intimacy_gift", args)
	if err != nil || businessPushCount(pushes) != 1 {
		t.Fatal("资源不足没有业务拒绝", err, pushes)
	}
	after, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, after) {
		t.Fatal("多送材料不足部分扣费")
	}
	_, _ = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error { p.Materials[702] = Material{ID: 702, Count: 2, Total: 2}; return nil })
	pushes, err = s.Handle(ctx, c, "consume_multi_intimacy_gift", args)
	if err != nil || businessPushCount(pushes) != 5 || c.SelectedAvatarUnsafe().Progress.Intimacy[4401] != 90 {
		t.Fatal("多送没有累加原生增量", err)
	}
	tables, _ := loadRuneTables()
	before, _ = accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.RuneSchemaVersion = CurrentRuneSchemaVersion
		p.Runes = map[string]Rune{}
		for i := 1; i <= tables.Tables.Limits.Max-3; i++ {
			id := fmt.Sprintf("%024x", i)
			p.Runes[id] = Rune{UUID: id}
		}
		p.Materials[523] = Material{ID: 523, Count: 1, Total: 1}
		return nil
	})
	args[2] = json.RawMessage(`{"701":1,"523":1}`)
	pushes, err = s.Handle(ctx, c, "consume_multi_intimacy_gift", args)
	rules, _ := loadIntimacyCatalog()
	if err != nil || businessPushCount(pushes) != 1 || pushes[0].Args[1].([]any)[0] != rules.Errors["RET_RUNE_MAX_COUNT_EXCEED"] {
		t.Fatal("回礼背包容量没有拒绝", err, pushes)
	}
	after, _ = accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, after) {
		t.Fatal("回礼容量失败部分扣费或加好感")
	}
}

func TestIntimacyGiftPreferenceCapAndStrictInput(t *testing.T) {
	rules, err := loadIntimacyCatalog()
	if err != nil {
		t.Fatal(err)
	}
	cardID := 0
	for id, card := range rules.Cards {
		if card.FavoredTag != nil && *card.FavoredTag == *rules.Gifts[701].LikeTag && card.Disabled == 0 && card.NotInStat == 0 {
			cardID = id
			break
		}
	}
	if cardID == 0 {
		t.Fatal("Android偏好样本缺失")
	}
	for _, liked := range []bool{false, true} {
		p := NewProgress(1, time.Now())
		p.Cards = []Card{newCard(cardID, 1, time.Now())}
		p.IntimacyCommons = map[int]IntimacyCommon{cardID: {LikeGift: liked}}
		p.Materials[701] = Material{ID: 701, Count: 3, Total: 3}
		if _, err := consumeIntimacyGifts(&p, cardID, map[int]int64{701: 3}, rules, time.Now()); err != nil {
			t.Fatal(err)
		}
		want := 60
		if liked {
			want = 66
		}
		if p.Intimacy[cardID] != want {
			t.Fatal("偏好标记未按原生控制", liked, p.Intimacy[cardID])
		}
	}
	p := NewProgress(1, time.Now())
	p.Cards = []Card{newCard(cardID, 1, time.Now())}
	p.Intimacy = map[int]int{cardID: 31999}
	p.Materials[701] = Material{ID: 701, Count: math.MaxInt64, Total: math.MaxInt64}
	if _, err := consumeIntimacyGifts(&p, cardID, map[int]int64{701: math.MaxInt64}, rules, time.Now()); err != nil || p.Intimacy[cardID] != 32000 || p.Materials[701].Total != math.MaxInt64 {
		t.Fatal("大数量溢出或累计获得错误", err)
	}
	for _, raw := range []string{`{}`, `null`, `[]`, `{"701":1,"701":2}`, `{"0701":1}`, `{"701":null}`, `{"701":-1}`, `{"701":1.5}`, `{"701":true}`, `{"701":1}{}`} {
		if _, err := parseIntimacyGifts(json.RawMessage(raw)); err == nil {
			t.Fatal("非法送礼字典被接受", raw)
		}
	}
}

func TestIntimacyGiftGenerationFailureRollsBackAndSpecialChoiceUsesCatalog(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	before, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		p.Cards = []Card{newCard(4401, 1, time.Now())}
		p.Materials[523] = Material{ID: 523, Count: 1, Total: 1}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var broken intimacyCatalog
	if err := json.Unmarshal(intimacyCatalogJSON, &broken); err != nil {
		t.Fatal(err)
	}
	row := broken.Gifts[523]
	row.ReturnRunes[1].Spec.Suit = 999999
	broken.Gifts[523] = row
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		_, err := consumeIntimacyGifts(p, 4401, map[int]int64{523: 1}, broken, time.Now())
		return err
	}); err == nil {
		t.Fatal("中途生成失败未拒绝")
	}
	after, _ := accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, after) {
		t.Fatal("中途生成失败留下了第一枚契印")
	}
	args := []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`4401`), json.RawMessage(`523`), json.RawMessage(`1`), json.RawMessage(`3`)}
	if pushes, err := s.Handle(ctx, c, "consume_intimacy_gift", args); err != nil || businessPushCount(pushes) != 6 {
		t.Fatal("特殊礼物未按目录选择完成", err, pushes)
	}
	after, _ = accounts.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if reflect.DeepEqual(before, after) || after.Materials[523].Count != 0 || after.Intimacy[4401] <= 0 || after.IntimacyCommons[4401].SpecialCount != 1 {
		t.Fatal("特殊礼物未按事务扣除或记录次数")
	}
}
