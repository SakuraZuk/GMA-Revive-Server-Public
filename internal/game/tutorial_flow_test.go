package game

import (
	"encoding/json"
	"testing"
	"time"
)

// 验证原生教学串联、等待触发、持久化重载与重复完成；非实机验收。
func TestFullTutorialGuidePersistence(t *testing.T) {
	now := time.Unix(1791456000, 0)
	p := NewProgress(1, now)
	for id := 1000; id <= 1013; id++ {
		if err := AdvanceGuide(&p, id, now, 1); err != nil {
			t.Fatalf("教学%d不能推进：%v", id, err)
		}
	}
	for _, step := range []struct{ guide, dungeon, next int }{
		{1014, 504, 2001}, {2001, 506, 2002}, {2002, 509, 2003}, {2003, 510, 0},
	} {
		if p.GuideTasks[step.guide].Status != 0 {
			t.Fatalf("教学%d必须等待真实触发", step.guide)
		}
		before, _ := json.Marshal(p)
		if err := AdvanceGuide(&p, step.guide, now, 1); err == nil {
			t.Fatalf("教学%d未通关不应放行", step.guide)
		}
		after, _ := json.Marshal(p)
		if string(before) != string(after) {
			t.Fatal("拒绝完成不应修改进度")
		}
		p.ClearedDungeons = append(p.ClearedDungeons, step.dungeon)
		p = CloneProgress(p)
		if !ActivateGuideTriggers(&p) || p.GuideTasks[step.guide].Status != 1 {
			t.Fatal("真实通关必须持久激活教学")
		}
		if ActivateGuideTriggers(&p) {
			t.Fatal("重复激活必须幂等")
		}
		if err := AdvanceGuide(&p, step.guide, now, 1); err != nil {
			t.Fatalf("教学%d真实通关后仍被拒绝：%v", step.guide, err)
		}
		before, _ = json.Marshal(p)
		if err := AdvanceGuide(&p, step.guide, now, 1); err != nil {
			t.Fatal(err)
		}
		after, _ = json.Marshal(p)
		if string(before) != string(after) {
			t.Fatal("重复完成不能重置下一项")
		}
		if step.next != 0 && p.GuideTasks[step.next].Status != 0 {
			t.Fatal("下一条件教学不应提前执行")
		}
	}
	if err := AdvanceGuide(&p, 6001, now, 1); err == nil {
		t.Fatal("不存在的任务不能通过通关越级创建")
	}
}
