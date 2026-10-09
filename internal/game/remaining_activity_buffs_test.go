package game

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestRemainingActivityMikuBuffLayersFrozenAndReload(t *testing.T) {
	p := NewProgress(40, time.Unix(1800000000, 0))
	p.Activities.Miku = &MikuState{Maps: map[int]*MikuMap{
		1: {ID: 1, Handbook: MikuHandbook{Items: map[int][]MikuHandbookItem{
			1: {{ID: 103, State: 2}, {ID: 104, State: 1}},
			0: {{ID: 101, State: 2}, {ID: 102, State: 0}},
		}}},
	}}
	bc := &ActivityBattleContext{ActivityID: 208, MapID: 1, DungeonID: 20810001}
	if err := freezeActivityConfirmedBuffs(&p, bc); err != nil {
		t.Fatal(err)
	}
	want := []ActivityNativeBuff{{ID: 2080021, NullProperty: true}, {ID: 2080021, NullProperty: true}, {ID: 2080021, NullProperty: true}, {ID: 2080001, NullProperty: true}}
	if !reflect.DeepEqual(bc.NativeBuffs[1], want) {
		t.Fatalf("手册已激活效果未保持组序/重复层叠：%+v", bc.NativeBuffs)
	}
	before, err := json.Marshal(bc)
	if err != nil {
		t.Fatal(err)
	}
	var restored ActivityBattleContext
	if err = json.Unmarshal(before, &restored); err != nil {
		t.Fatal(err)
	}
	p.Activities.Miku.Maps[1].Handbook.Items = nil
	extra := activityBattleExtra(&restored, map[string]any{"change_attr_data": map[string]any{"伪造": 1}})
	attrs := extra["change_attr_data"].(map[string]any)
	rows := attrs["buff"].(map[int][]any)[1]
	if len(rows) != len(want) || rows[0].([]any)[1] != nil || !reflect.DeepEqual(restored.NativeBuffs[1], want) {
		t.Fatal("冷恢复丢失原生None或重新取可变玩家手册", rows)
	}
}

func TestRemainingActivityWangyanCorrectionBoundaries(t *testing.T) {
	for _, row := range []struct{ player, enemy, correction int }{
		{151, 100, -20}, {150, 100, -10}, {110, 100, 0}, {100, 100, 0},
		{90, 100, 45}, {66, 100, 150}, {0, 100, 0}, {10, 0, 0},
	} {
		got, err := wangyanCorrectionLevel(row.player, row.enemy)
		if err != nil || got != row.correction {
			t.Fatalf("原表战力边界 %+v：%d %v", row, got, err)
		}
	}
}

func TestRemainingActivityWangyanOccupationAppliesNextBattle(t *testing.T) {
	p := NewProgress(40, time.Unix(1800000000, 0))
	p.Activities.Wangyan = &WangyanState{Power: 300, Maps: map[int]*WangyanMap{13: {ID: 13}}}
	bc := &ActivityBattleContext{ActivityID: 207, MapID: 13, NodeID: 1399, DungeonID: 20711399}
	if err := freezeActivityConfirmedBuffs(&p, bc); err != nil {
		t.Fatal(err)
	}
	if len(bc.NativeBuffs[2]) != 2 || bc.NativeBuffs[2][0].ID != 6040174 || bc.NativeBuffs[2][1].ID != 6040175 || bc.EnemyLevelAdded == nil {
		t.Fatal("未占领影响节点没有冻结敌方原buff", bc)
	}
	before, _ := json.Marshal(bc)
	p.Activities.Wangyan.Maps[13].Occupied = []int{1305, 1306}
	p.Activities.Wangyan.Power = 100000
	extra := activityBattleExtra(bc, map[string]any{"enemy_level": 1, "enemy_level_added": -100000, "enemy_hp_factor": 0.0001})
	after, _ := json.Marshal(bc)
	if !reflect.DeepEqual(before, after) || extra["enemy_level"] != nil || extra["enemy_hp_factor"] != nil || extra["enemy_level_added"] != *bc.EnemyLevelAdded {
		t.Fatal("当前冻结会话被占领/战力变化或客户端数值覆盖", extra)
	}
	next := &ActivityBattleContext{ActivityID: 207, MapID: 13, NodeID: 1399}
	if err := freezeActivityConfirmedBuffs(&p, next); err != nil {
		t.Fatal(err)
	}
	if len(next.NativeBuffs[2]) != 0 || next.EnemyLevelAdded == nil || *next.EnemyLevelAdded != -20 {
		t.Fatal("占领未在下一战移除加成并更新修正", next)
	}
}

func TestRemainingActivitySummerFlagCannotBeUploaded(t *testing.T) {
	extra := activityBattleExtra(nil, map[string]any{"hs_summer_closed_beta": true})
	if extra["hs_summer_closed_beta"] != nil {
		t.Fatal("普通副本允许上传夏活buff开关")
	}
	bc := &ActivityBattleContext{ActivityID: 211}
	if err := freezeActivityConfirmedBuffs(nil, bc); err != nil {
		t.Fatal(err)
	}
	extra = activityBattleExtra(bc, map[string]any{"hs_summer_closed_beta": false})
	if extra["hs_summer_closed_beta"] != true {
		t.Fatal("夏活服务端冻结开关可被客户端关闭")
	}
}
