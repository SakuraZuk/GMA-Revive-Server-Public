package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func completeAutoEnhanceChapter(t *testing.T, p *Progress, chapter int) {
	t.Helper()
	var chapters map[int]map[int][]int
	if err := json.Unmarshal(androidRemainingGameplay["chapter_site_dungeon"]["chapter_dict"], &chapters); err != nil {
		t.Fatal(err)
	}
	for site, ids := range chapters[chapter] {
		kind := remainingData("sites", site).integer("site_type")
		if kind == 3 || kind == 7 || kind == 9 || kind == 10 {
			continue
		}
		p.ClearedDungeons = append(p.ClearedDungeons, ids...)
	}
}

func TestMainlineAutoEnhanceActualStoryEntryBeforeResult(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.GuideTasks = map[int]GuideTask{}
		p.AvatarLevel = 60
		p.Power.Value = 9998
		p.Cards = []Card{newCard(4401, 1, s.Now())}
		completeAutoEnhanceChapter(t, p, 1)
		cleared := p.ClearedDungeons[:0]
		for _, id := range p.ClearedDungeons {
			if id != 714 {
				cleared = append(cleared, id)
			}
		}
		p.ClearedDungeons = cleared
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for repeat := range 2 {
		out, err := s.Handle(ctx, c, "enter_dungeon", rawArgs(100+repeat, 714, map[string]any{}))
		if err != nil {
			t.Fatal(err)
		}
		p := c.SelectedAvatarUnsafe().Progress
		if p.Cards[0].EnhanceCount != 2 || p.Cards[0].SkillEnhanceCount != 1 {
			t.Fatal("实际纯剧情入口没有提交自动补完", p.Cards[0])
		}
		cardIndex, resultIndex := -1, -1
		for i, v := range out {
			if v.Method == "client_prop_changed" {
				if values, ok := v.Args[0].([]any); ok && values[0] == "card_mgr" {
					cardIndex = i
				}
			}
			if v.Method == "battle_result" {
				resultIndex = i
			}
		}
		if repeat == 0 && (cardIndex < 0 || resultIndex <= cardIndex) {
			t.Fatal("原生补完演出前没有同步幻书状态", cardIndex, resultIndex)
		}
	}
}

func TestMainlineAutoEnhanceNativeChaptersAndColdDuplicate(t *testing.T) {
	p := NewProgress(1, time.Now())
	p.Cards = []Card{newCard(4401, 1, time.Now()), newCard(4401, 1, time.Now()), newCard(4402, 1, time.Now())}
	p.Cards[0].Level, p.Cards[0].Grade, p.Cards[0].Exp = 20, 2, 19
	before := CloneProgress(p)
	if err := ensureMainlineAutoEnhance(&p); err != nil || !reflect.DeepEqual(CloneProgress(p), before) {
		t.Fatal("未完成章不应自动补完", err)
	}
	completeAutoEnhanceChapter(t, &p, 1)
	last := p.ClearedDungeons[len(p.ClearedDungeons)-1]
	p.ClearedDungeons = p.ClearedDungeons[:len(p.ClearedDungeons)-1]
	if err := ensureMainlineAutoEnhance(&p); err != nil || p.Cards[0].EnhanceCount != 0 {
		t.Fatal("缺一个主线节点不得补完", err)
	}
	p.ClearedDungeons = append(p.ClearedDungeons, last)
	if err := ensureMainlineAutoEnhance(&p); err != nil || p.Cards[0].EnhanceCount != 2 || p.Cards[0].SkillEnhanceCount != 1 {
		t.Fatal("第一章原表两阶段未生效", err, p.Cards[0])
	}
	if p.Cards[1].EnhanceCount != 0 || p.Cards[2].EnhanceCount != 0 || !reflect.DeepEqual(p.Materials, before.Materials) || p.Cards[0].Exp != 19 {
		t.Fatal("自动补完影响其他实例或资产")
	}
	completeAutoEnhanceChapter(t, &p, 2)
	if err := ensureMainlineAutoEnhance(&p); err != nil || p.Cards[0].EnhanceCount != 4 || p.Cards[0].SkillEnhanceCount != 2 {
		t.Fatal("两章累计原表四阶段未生效", err, p.Cards[0])
	}
	saved := CloneProgress(p)
	for range 3 {
		p = CloneProgress(p)
		if err := ensureMainlineAutoEnhance(&p); err != nil || !reflect.DeepEqual(p, saved) {
			t.Fatal("冷恢复重复补完", err)
		}
	}
	levels := 0
	for _, skill := range nativeCardSkills(p.Cards[0]) {
		levels += skill.EnhanceLevel
	}
	if levels != 2 {
		t.Fatal("原生技能补完列表未反映四阶段中的两次技能补完", levels)
	}
}
