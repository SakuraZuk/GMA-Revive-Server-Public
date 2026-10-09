package game

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestActivityMikuActualRPCBattleReceiptAndLeave(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	activitySchedulesForTest(t, now)
	c, av := newBattleConnection(t, ctx, a, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		for id, g := range p.GuideTasks {
			g.Status = 2
			p.GuideTasks[id] = g
		}
		p.ClearedDungeons = append(p.ClearedDungeons, 601)
		p.Power.Value = 100
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	rpc := func(method string, values ...any) []Push {
		t.Helper()
		args := []json.RawMessage{}
		for _, v := range values {
			raw, _ := json.Marshal(v)
			args = append(args, raw)
		}
		out, e := s.Handle(ctx, c, method, args)
		if e != nil {
			t.Fatal(method, e)
		}
		return out
	}
	rpc("enter_miku_map", 1)
	m := c.SelectedAvatarUnsafe().Progress.Activities.Miku.Maps[1]
	if m == nil || m.Current != 200 || m.Nodes[201] == nil {
		t.Fatal("初音原生起点/事件未生成")
	}
	rpc("receive_miku_like_song_bonus")
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	rpc("receive_miku_like_song_bonus")
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
	if string(beforeJSON) != string(afterJSON) {
		t.Fatal("歌曲奖励重复入账")
	}
	rpc("enter_miku_node", 201, map[string]any{"normal_battle": true, "hard_battle": false, "is_angry": false})
	b := c.SelectedAvatarUnsafe().Progress.Battle
	if b == nil || b.ActivityContext == nil || b.DungeonID != 20811101 || b.ActivityContext.NodeID != 201 || b.ActivityContext.Place != "explore" {
		t.Fatal("初音事件资格未冻结", b)
	}
	uuid := b.UUID
	rpc("load_entity_finish")
	rpc("battle_fighting", map[string]any{"fighting_cards": []int{0}, "support_cards": []int{}, "storyline_cards": []int{}})
	observeBattleEvent(t, ctx, s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":1,"kind":"ready","data":{"version":1}}`, uuid))
	observeBattleEvent(t, ctx, s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":2,"kind":"started","data":{"units":[{"eid":"1","role":1,"hex":[0,0,0],"kind":"ally"}]}}`, uuid))
	out := observeBattleEvent(t, ctx, s, c, fmt.Sprintf(`{"battle_uuid":%q,"sequence":3,"kind":"result","data":{"winner_eids":["%x"],"finished_task_list":[]}}`, uuid, av.OID))
	p := c.SelectedAvatarUnsafe().Progress
	m = p.Activities.Miku.Maps[1]
	if !p.Battle.RewardGranted || m.Current != 201 || !m.Nodes[201].Passed || m.Score != 2 || len(m.ReceiptMaterials) == 0 {
		t.Fatal("初音探索结算未真正推进", m)
	}
	nativeFinish := false
	for _, v := range out {
		if v.Method == "on_finish_miku_node" {
			nativeFinish = true
		}
	}
	if !nativeFinish {
		t.Fatal("原生节点完成回调缺失")
	}
	clone := CloneProgress(p)
	activityProperties(clone, now)
	if !reflect.DeepEqual(m.ReceiptMaterials, clone.Activities.Miku.Maps[1].Bonus["materials"]) {
		t.Fatal("JSON重连奖励盒丢失整型材料字典")
	}
	rpc("leave_miku_map", 1)
	m = c.SelectedAvatarUnsafe().Progress.Activities.Miku.Maps[1]
	if m.Current != 200 || len(m.Path) != 0 || len(m.ReceiptMaterials) != 0 || !m.Nodes[201].Passed || m.Score != 2 {
		t.Fatal("提前离开未按原生清本轮并保留累计进度", m)
	}
	before = CloneProgress(c.SelectedAvatarUnsafe().Progress)
	rpc("leave_miku_map", 1)
	beforeJSON, _ = json.Marshal(before)
	afterJSON, _ = json.Marshal(c.SelectedAvatarUnsafe().Progress)
	if string(beforeJSON) != string(afterJSON) {
		t.Fatal("重复离开重新发探索奖励")
	}
}

func TestActivityNianFootprintPoolAndRollback(t *testing.T) {
	now := time.Unix(1800000000, 0)
	p := Progress{Materials: map[int]Material{522: {ID: 522, Count: 2, Total: 2}}}
	w := ensureNianFootprint(&p)
	w.Remaining = []int{1, 2}
	box, e := openNianGrid(&p, 0, now)
	if e != nil || len(box["materials"].(map[int]int64)) == 0 {
		t.Fatal("原生翻格奖励未入账", e, box)
	}
	if p.Materials[522].Count != 1 || w.Times != 1 || len(w.Opened) != 1 || len(w.Remaining) != 1 {
		t.Fatal("翻格票券/池/历史未同步推进")
	}
	before := CloneProgress(p)
	if _, e = openNianGrid(&p, 0, now); e == nil || !reflect.DeepEqual(before, p) {
		t.Fatal("已开格重复扣票或发奖")
	}
	w.Remaining = []int{0}
	box, e = openNianGrid(&p, 1, now)
	if e != nil || w.Site != 2 || w.Real != 2 || len(w.Opened) != 0 || w.Times != 2 || p.Materials[522].Count != 0 {
		t.Fatal("通层大奖与层刷新错误", e, box, w)
	}
	w.Site, w.Real, w.Times = 25, 25, 125
	if e = refreshNianFootprint(w); e != nil || w.Real != 21 {
		t.Fatal("无限模式5层循环错误", e, w)
	}
	w.Site, w.Times = 11, 100
	if e = refreshNianFootprint(w); e != nil || !containsInt(w.Remaining, 0) {
		t.Fatal("进度落后时原生保底池缺通层大奖", e, w)
	}
	w.Site, w.Times = 19, 0
	if e = refreshNianFootprint(w); e != nil || containsInt(w.Remaining, 0) || len(w.Remaining) != 8 {
		t.Fatal("进度过快时原生限制池错误", e, w)
	}
}
