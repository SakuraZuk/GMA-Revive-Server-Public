package game

import (
	"reflect"
	"testing"
	"time"
)

func TestGenerateRuneAndroidTemplates(t *testing.T) {
	tables, err := loadRuneTables()
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range tables.Tables.Templates {
		for _, level := range []int{tables.Tables.Bounds.Min, tables.Tables.Bounds.Max} {
			r, err := GenerateRune(RuneSpec{Suit: 1101, Position: template.Position, Star: template.Star, Level: level}, time.Unix(1700000000, 0))
			if err != nil {
				t.Fatalf("星级%d位置%d等级%d: %v", template.Star, template.Position, level, err)
			}
			if !validObjectID(r.UUID) || len(r.BaseAttrs) != template.BaseCount || r.CreateTime != 1700000000 {
				t.Fatalf("生成字段错误: %#v", r)
			}
			if template.Position == 1 && r.ExtraSuit != r.Suit || template.Position != 1 && r.ExtraSuit != 0 {
				t.Fatal("额外套装位置错误")
			}
			seen := map[int]bool{}
			for _, id := range r.BaseAttrs {
				if seen[id] {
					t.Fatal("重复基础属性")
				}
				seen[id] = true
				allowed := false
				for _, candidate := range template.BaseIDs {
					allowed = allowed || candidate == id
				}
				if !allowed {
					t.Fatal("基础属性不在 Android 模板中")
				}
			}
			for _, attr := range tables.Tables.Attrs {
				if seen[attr.ID] && attr.Mutex != nil && seen[*attr.Mutex] {
					t.Fatal("互斥基础属性同时生成")
				}
			}
			lib := map[int]bool{}
			for _, id := range r.ExtraAttrsLib {
				if lib[id] {
					t.Fatal("重复词条库")
				}
				lib[id] = true
			}
			roots := 0
			for _, id := range template.ExtraIDs {
				if lib[id] {
					roots++
					for _, attr := range tables.Tables.Attrs {
						if attr.ID == id && attr.Bind != nil && !lib[*attr.Bind] {
							t.Fatal("词条库缺少绑定属性")
						}
					}
				}
			}
			if roots != template.ExtraCount {
				t.Fatal("词条库根数量与星级表不一致")
			}
			target := 0
			for _, row := range tables.Tables.UnlockAttrs {
				if row.Unlock != 0 && row.Level <= level {
					target++
				}
			}
			if target > r.ExtraAttrsCount {
				target = r.ExtraAttrsCount
			}
			if len(r.ExtraAttrs) != target {
				t.Fatal("生成未填足已解锁词条")
			}
			for _, id := range r.ExtraAttrs {
				if !lib[id] {
					t.Fatal("额外属性不在词条库")
				}
			}
		}
	}
	locked, err := GenerateRune(RuneSpec{Suit: 1901, Position: 1, Star: 5, Level: 1}, time.Now())
	if err != nil || !locked.Locked {
		t.Fatalf("套装自动锁定未生效: %v", err)
	}
	for _, spec := range []RuneSpec{{Suit: 99999, Position: 1, Star: 5, Level: 1}, {Suit: 1101, Position: 2, Star: 5, Level: 1, ExtraSuit: 1102}, {Suit: 1101, Position: 1, Star: 6, Level: 1}} {
		if _, err := GenerateRune(spec, time.Now()); err == nil {
			t.Fatalf("非法生成被接受: %#v", spec)
		}
	}
}

func TestRuneMigrationDeterministicIdempotentAndQuarantined(t *testing.T) {
	id := "00112233445566778899aabb"
	brokenID := "00112233445566778899aabc"
	p := NewProgress(1, time.Unix(1700000000, 0))
	p.Runes = map[string]Rune{id: {UUID: id, Suit: 1101, Star: 5, Position: 1, Level: 11, BaseAttrs: []int{10501}, ExtraAttrs: []int{20504}, ExtraAttrsCount: 5, ExtraAttrsFactor: map[int]int{20504: 8}}, brokenID: {UUID: brokenID, RuneID: 131101, Level: 1}}
	p = CloneProgress(p)
	independent := CloneProgress(p)
	if err := MigrateRunes(&p); err != nil {
		t.Fatal(err)
	}
	if err := MigrateRunes(&independent); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, independent) {
		t.Fatal("同一旧存档迁移非确定性")
	}
	if p.Runes[id].BaseAttrs[0] != 10501 || p.Runes[id].ExtraAttrs[0] != 20504 || p.Runes[id].ExtraAttrsFactor[20504] != 8 {
		t.Fatal("有效属性或因子未保留")
	}
	if len(p.Runes[id].ExtraAttrs) != 5 || len(p.RuneQuarantine) != 1 || p.RuneQuarantine[brokenID].Record.RuneID != 131101 {
		t.Fatal("迁移补齐或原件隔离错误")
	}
	if _, ok := p.Runes[brokenID]; ok {
		t.Fatal("未知套装记录仍下发给客户端")
	}
	p = CloneProgress(p)
	before := CloneProgress(p)
	if err := MigrateRunes(&p); err != nil || !reflect.DeepEqual(before, p) {
		t.Fatal("重复登录重抽或覆盖原件")
	}
	if p.RuneMigrationOriginals[id].BaseAttrs[0] != 10501 || len(p.RuneMigrationOriginals[id].BaseAttrs) != 1 {
		t.Fatal("原始存档被迁移覆盖")
	}
}

func TestGrantRuneCapacityAndMigrationFutureVersion(t *testing.T) {
	tables, err := loadRuneTables()
	if err != nil {
		t.Fatal(err)
	}
	p := NewProgress(1, time.Now())
	for i := 0; i < tables.Tables.Limits.Max; i++ {
		p.Runes[itoa(i)] = Rune{}
	}
	p = CloneProgress(p)
	before := CloneProgress(p)
	if _, err := GrantRune(&p, RuneSpec{Suit: 1101, Position: 1, Star: 5, Level: 1}, time.Now()); err == nil || !reflect.DeepEqual(before, p) {
		t.Fatal("满背包仍生成或写入")
	}
	p.RuneSchemaVersion = CurrentRuneSchemaVersion + 1
	if err := MigrateRunes(&p); err == nil {
		t.Fatal("未来版本被降级迁移")
	}
}

func TestRuneGrantReceiptSurvivesDecomposeAndRejectsConflicts(t *testing.T) {
	p := NewProgress(1, time.Now())
	spec := RuneSpec{Suit: 1101, Position: 1, Star: 5, Level: 1}
	first, err := GrantRuneOnce(&p, "副本:10001:首通:契印:0", spec, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	delete(p.Runes, first.UUID)
	p = CloneProgress(p)
	again, err := GrantRuneOnce(&p, "副本:10001:首通:契印:0", spec, time.Now())
	if err != nil || !reflect.DeepEqual(first, again) || len(p.Runes) != 0 {
		t.Fatal("已分解的同一奖励重试重复发放", err)
	}
	conflict := spec
	conflict.Star = 4
	if _, err := GrantRuneOnce(&p, "副本:10001:首通:契印:0", conflict, time.Now()); err == nil || len(p.Runes) != 0 {
		t.Fatal("凭据可重用为其他奖励")
	}
}

func TestRuneMigrationRepairsOwnerAndDuplicateSlot(t *testing.T) {
	p := NewProgress(1, time.Now())
	card := p.Cards[0].UUID
	ids := []string{"00112233445566778899aabb", "00112233445566778899aabc", "00112233445566778899aabd"}
	for i, id := range ids {
		owner := card
		if i == 2 {
			owner = "ffffffffffffffffffffffff"
		}
		p.Runes[id] = Rune{UUID: id, Suit: 1101, Star: 1, Position: 1, Level: 1, CardUUID: owner}
	}
	if err := MigrateRunes(&p); err != nil {
		t.Fatal(err)
	}
	if p.Runes[ids[0]].CardUUID != card || p.Runes[ids[1]].CardUUID != "" || p.Runes[ids[2]].CardUUID != "" {
		t.Fatal("迁移后仍有重复卡槽或未拥有卡牌")
	}
}
