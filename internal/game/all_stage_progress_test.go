package game

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"testing"
)

// 枚举原表主线/支线纯剧情；用实际入口和客户端属性协议核验，不伪造战斗胜利。
func TestAllMainlineStoryNodesSyncBeforeResultAndColdProjection(t *testing.T) {
	t.Setenv("HS_NEW_AVATAR_ALL_HEROES", "0")
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, av := newBattleConnection(t, ctx, a, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.GuideTasks = map[int]GuideTask{}
		p.AvatarLevel = 60
		p.Power.Value = 9998
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ids := []int{}
	for id, d := range dungeonCatalog {
		var disabled bool
		_ = json.Unmarshal(activityData("dungeons", id)["_dungeon_disable"], &disabled)
		if !disabled && d.BattleID <= 0 && (d.Type == 1 || d.Type == 17) {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	if !containsInt(ids, 603) {
		t.Fatal("2-3剧情节点未进入全量回归")
	}
	for _, id := range ids {
		t.Run(itoaStage(id), func(t *testing.T) {
			for repeat := 0; repeat < 2; repeat++ {
				out, err := s.Handle(ctx, c, "enter_dungeon", rawArgs(100+repeat, id, map[string]any{}))
				if err != nil {
					t.Fatal("原表剧情入口失败", id, err)
				}
				progressIndex, resultIndex := -1, -1
				for i, v := range out {
					if v.Method == "client_prop_set" {
						path, ok := v.Args[0].([]any)
						if ok && len(path) == 3 && path[0] == "dungeon_mgr" && path[1] == id {
							if path[2].(map[string]any)["finished"] != 1 {
								t.Fatal("剧情未同步已完成状态")
							}
							progressIndex = i
						}
					}
					if v.Method == "battle_result" {
						resultIndex = i
					}
				}
				if len(out) == 0 || out[0].Method != "call_client_callback" || progressIndex < 0 || resultIndex <= progressIndex {
					t.Fatal("剧情结果先于客户端真实进度", out)
				}
				p := c.SelectedAvatarUnsafe().Progress
				if p.Battle != nil || !containsInt(p.ClearedDungeons, id) {
					t.Fatal("剧情留下战斗会话或未提交通关")
				}
			}
		})
	}
	stored, err := a.UpdateProgress(ctx, av.OID, func(p *Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	props := (Avatar{Progress: stored}).InitialProperties("全关卡剧情验收")
	mgr := props["dungeon_mgr"].(map[string]any)
	for _, id := range ids {
		if mgr[itoaStage(id)].(map[string]any)["finished"] != 1 {
			t.Fatal("冷登录丢失剧情通关", id)
		}
	}
	t.Logf("主线及支线纯剧情节点%d项，每项入口/重复进入/结果顺序及冷属性已核验", len(ids))
}

func itoaStage(id int) string { return strconv.Itoa(id) }

func TestAllMainlineBattleNodesRepeatedEntryPreservesModel(t *testing.T) {
	t.Setenv("HS_NEW_AVATAR_ALL_HEROES", "0")
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	ids := []int{}
	for id, d := range dungeonCatalog {
		if !activityData("dungeons", id).flag("_dungeon_disable") && d.BattleID > 0 && (d.Type == 1 || d.Type == 17) {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	for _, id := range ids {
		t.Run(itoaStage(id), func(t *testing.T) {
			if err := s.updateProgress(ctx, c, func(p *Progress) error {
				p.GuideTasks = map[int]GuideTask{}
				p.AvatarLevel = 60
				p.Power.Value = 9998
				p.Battle = nil
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Handle(ctx, c, "enter_dungeon", rawArgs(200, id, map[string]any{})); err != nil {
				t.Fatal(err)
			}
			if err := s.updateProgress(ctx, c, func(p *Progress) error { p.Battle.Loaded = true; p.Battle.Started = true; return nil }); err != nil {
				t.Fatal(err)
			}
			before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
			out, err := s.Handle(ctx, c, "enter_dungeon", rawArgs(201, id, map[string]any{}))
			if err != nil || len(out) != 1 || out[0].Method != "call_client_callback" {
				t.Fatal("重复入场重建模型", err, out)
			}
			after := c.SelectedAvatarUnsafe().Progress
			if after.Battle.UUID != before.Battle.UUID || !after.Battle.Loaded || !after.Battle.Started || after.Power.Value != before.Power.Value {
				t.Fatal("重复入场重置战斗状态或重扣体力")
			}
		})
	}
	t.Logf("主线及支线战斗节点%d项，同UUID重复入口均保持客户端模型", len(ids))
}
