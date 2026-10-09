package game

import (
	"crypto/rand"
	"testing"
	"time"
)

func TestMarkedRuneRewardAndroidOrderAndEquipValidity(t *testing.T) {
	tables, err := loadRuneTables()
	if err != nil {
		t.Fatal(err)
	}
	for pos := 1; pos <= 4; pos++ {
		template, err := runeTemplateFor(tables, 5, pos)
		if err != nil {
			t.Fatal(err)
		}
		for sample := 0; sample < 20; sample++ {
			r, err := GenerateRuneWithMarks(RuneSpec{Suit: 1901, Position: pos, Star: 5, Level: 1}, []string{"atk", "hp"}, time.Unix(1700000000, 0))
			if err != nil {
				t.Fatal(err)
			}
			if err := validateRuneForEquip(r, tables); err != nil {
				t.Fatalf("回礼契印不能佩戴：位置%d：%v", pos, err)
			}
			if !r.Locked || len(r.BaseAttrs) != template.BaseCount {
				t.Fatal("回礼套装锁定或属性容量错误")
			}
			// 原生顺序是先攻击再生命；当前模板不存在该属性时才跳过。
			available := map[string]bool{}
			names := map[int]string{}
			for _, attr := range tables.Tables.Attrs {
				names[attr.ID] = attr.Name
				for _, id := range template.BaseIDs {
					if attr.ID == id && attr.Weight > 0 {
						available[attr.Name] = true
					}
				}
			}
			index := 0
			for _, name := range []string{"atk", "hp"} {
				if available[name] && index < len(r.BaseAttrs) {
					if names[r.BaseAttrs[index]] != name {
						t.Fatalf("位置%d缺少顺序标记%s：%v", pos, name, r.BaseAttrs)
					}
					index++
				}
			}
		}
	}
}

func TestMarkedRuneNoMatchAndDuplicateRespectCapacity(t *testing.T) {
	tables, err := loadRuneTables()
	if err != nil {
		t.Fatal(err)
	}
	template, err := runeTemplateFor(tables, 5, 4)
	if err != nil {
		t.Fatal(err)
	}
	attrs, err := markedRuneAttrs(template.BaseIDs, template.BaseCount, nil, []string{"不存在的属性", "atk", "atk", "hp"}, tables, rand.Reader)
	if err != nil || len(attrs) != template.BaseCount {
		t.Fatalf("未知标记或重复标记影响容量：%v %v", attrs, err)
	}
	seen := map[int]bool{}
	for _, id := range attrs {
		if seen[id] {
			t.Fatal("重复属性")
		}
		seen[id] = true
	}
	for _, attr := range tables.Tables.Attrs {
		if seen[attr.ID] && attr.Mutex != nil && seen[*attr.Mutex] {
			t.Fatal("重复标记绕过互斥")
		}
	}
}
