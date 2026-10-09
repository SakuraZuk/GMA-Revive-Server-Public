package game

import (
	"context"
	"reflect"
	"testing"
)

func TestRuneUnlockUsesAndroidLevelsAndRetainsFactors(t *testing.T) {
	tables, err := loadRuneTables()
	if err != nil {
		t.Fatal(err)
	}
	r := Rune{Star: 5, Position: 1, ExtraAttrsCount: 5, ExtraAttrsLib: []int{20504}, ExtraAttrsFactor: map[int]int{20504: 8}}
	for level := 1; level <= 11; level++ {
		r.Level = level
		if err := fillRuneExtraAttrs(&r, tables); err != nil {
			t.Fatal(err)
		}
		want := (level - 1) / 2
		if len(r.ExtraAttrs) != want || r.ExtraAttrsFactor[20504] != 8 {
			t.Fatalf("等级%d词条=%v 因子=%v", level, r.ExtraAttrs, r.ExtraAttrsFactor)
		}
		for _, id := range r.ExtraAttrs {
			if id != 20504 {
				t.Fatal("抽出了词条库之外的属性")
			}
		}
	}
	before := append([]int(nil), r.ExtraAttrs...)
	if err := fillRuneExtraAttrs(&r, tables); err != nil || !reflect.DeepEqual(before, r.ExtraAttrs) {
		t.Fatal("重复调用重抽已有词条")
	}
}

func TestRuneUpgradeUnlockAndInvalidLibraryRollback(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	for i := range c.identity.Avatars {
		c.identity.Avatars[i].Info.Level = 100
	}
	id := "00112233445566778899aabb"
	tables, err := loadRuneTables()
	if err != nil {
		t.Fatal(err)
	}
	for _, library := range [][]int{{20504}, {999999}} {
		before, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error {
			p.Runes = map[string]Rune{id: {UUID: id, Star: 5, Position: 1, Level: 2, ExtraAttrsCount: 2, ExtraAttrsLib: library}}
			for _, row := range tables.Tables.Levels {
				if row.Star == 5 && row.Level == 2 {
					for _, cost := range row.Upgrade {
						p.Materials[cost[0]] = Material{ID: cost[0], Count: int64(cost[1]) * 2, Total: int64(cost[1]) * 2}
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		pushes, err := s.Handle(ctx, c, "up_level_rune", runeTestArgs(11, id, 1))
		if err != nil {
			t.Fatal(err)
		}
		after, err := accounts.UpdateProgress(ctx, av.OID, func(p *Progress) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		if library[0] == 999999 {
			if businessPushCount(pushes) != 1 || !reflect.DeepEqual(before, after) {
				t.Fatal("非法词条库升级没有完整回滚")
			}
		} else {
			if businessPushCount(pushes) != 4 || after.Runes[id].Level != 3 || !reflect.DeepEqual(after.Runes[id].ExtraAttrs, []int{20504}) {
				t.Fatalf("升级未解锁词条: %#v %#v", pushes, after.Runes[id])
			}
			for materialID, material := range before.Materials {
				if after.Materials[materialID].Total != material.Total {
					t.Fatal("升级倒扣累计获得")
				}
			}
		}
	}
}
