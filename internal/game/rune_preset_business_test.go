package game

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRunePresetRPCPersistenceAndEmbed(t *testing.T) {
	accounts, service, conn, avatar, cards := layoutTestSetup(t)
	ctx := context.Background()
	if _, err := accounts.UpdateProgress(ctx, avatar.OID, func(p *Progress) error {
		p.Runes = map[string]Rune{
			"00112233445566778899aabb": {UUID: "00112233445566778899aabb", RuneID: 110101, Star: 5, Position: 1, Suit: 1101, Level: 1},
			"00112233445566778899aacc": {UUID: "00112233445566778899aacc", RuneID: 110102, Star: 5, Position: 2, Suit: 1101, Level: 1},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pushes, err := service.Handle(ctx, conn, "add_runes_templates", []json.RawMessage{
		json.RawMessage(`1`), json.RawMessage(`null`), json.RawMessage(`["00112233445566778899aabb","00112233445566778899aacc"]`), json.RawMessage(`"常用"`),
	})
	if err != nil || len(pushes) < 2 {
		t.Fatalf("新增契印预设失败: %v %#v", err, pushes)
	}
	p, _ := accounts.UpdateProgress(ctx, avatar.OID, func(*Progress) error { return nil })
	if len(p.RuneTemplates) != 1 {
		t.Fatal("契印预设未持久化")
	}
	var templateID string
	for id := range p.RuneTemplates {
		templateID = id
	}
	if _, err := service.Handle(ctx, conn, "embed_rune_by_template", []json.RawMessage{json.RawMessage(`2`), json.RawMessage(`"` + templateID + `"`), json.RawMessage(`"` + cards[0].UUID + `"`)}); err != nil {
		t.Fatal("应用契印预设失败：", err)
	}
	after, _ := accounts.UpdateProgress(ctx, avatar.OID, func(*Progress) error { return nil })
	if after.Runes["00112233445566778899aabb"].CardUUID != cards[0].UUID || after.Runes["00112233445566778899aacc"].CardUUID != cards[0].UUID {
		t.Fatal("应用预设没有替换契印归属")
	}
	if _, err := service.Handle(ctx, conn, "change_runes_templates_name", []json.RawMessage{json.RawMessage(`3`), json.RawMessage(`"` + templateID + `"`), json.RawMessage(`"新名"`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(ctx, conn, "delete_runes_templates", []json.RawMessage{json.RawMessage(`4`), json.RawMessage(`"` + templateID + `"`)}); err != nil {
		t.Fatal(err)
	}
	last, _ := accounts.UpdateProgress(ctx, avatar.OID, func(*Progress) error { return nil })
	if len(last.RuneTemplates) != 0 {
		t.Fatal("删除契印预设失败")
	}
}
