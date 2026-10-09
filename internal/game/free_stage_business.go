package game

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"
)

func (w *FreeStageState) UnmarshalJSON(raw []byte) error {
	type plain FreeStageState
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode((*plain)(w))
}
func freeStageStatesWire(stages map[int]*FreeStageState) any {
	out := map[int]any{}
	for id, w := range stages {
		box, err := battleSettlementBoxWire(w.Bonus)
		if err != nil {
			panic("探索奖励存档无效：" + err.Error())
		}
		out[id] = map[string]any{"stage_id": w.ID, "state": w.State, "site_info": activityWire(w.Sites), "cur_bonus_box": box, "boss_double_bonus_flag": w.BossDouble}
	}
	return activityWire(out)
}

type FreeStageSite struct {
	ID         int `json:"site_id"`
	Type       int `json:"site_type"`
	State      int `json:"site_state"`
	Event      int `json:"random_event_type"`
	EventParam int `json:"random_event_param"`
	Monster    int `json:"monster_dungeon_id"`
	Elite      int `json:"elite_dungeon_id"`
	Boss       int `json:"boss_dungeon_id"`
}
type FreeStageState struct {
	ID         int                    `json:"stage_id"`
	State      int                    `json:"state"`
	Sites      map[int]*FreeStageSite `json:"site_info"`
	Bonus      map[string]any         `json:"cur_bonus_box"`
	BossDouble bool                   `json:"boss_double_bonus_flag"`
	Cycle      int64                  `json:"server_cycle" wire:"-"`
}

func freeStageNodes(id int) []int {
	var nodes map[int][]int
	_ = json.Unmarshal(androidRemainingGameplay["free_stage_nodes"]["free_stage_nodes"], &nodes)
	return nodes[id]
}
func freeStageChoose(values, weights []int) (int, error) {
	if len(values) == 0 || len(values) != len(weights) {
		return 0, errors.New("探索权重配置无效")
	}
	i, err := activityWeighted(weights)
	if err != nil {
		return 0, err
	}
	return values[i], nil
}
func newFreeStage(id int, cycle int64) (*FreeStageState, error) {
	nodes := freeStageNodes(id)
	if len(nodes) == 0 {
		return nil, errors.New("探索章节不存在")
	}
	s := &FreeStageState{ID: id, Sites: map[int]*FreeStageSite{}, Bonus: emptyActivityBox(), Cycle: cycle}
	starts := []int{}
	for _, node := range nodes {
		r := remainingData("free_stage_site", node)
		monster, err := freeStageChoose(r.ids("monster_dungeon_id"), r.ids("monster_dungeon_weights"))
		if err != nil {
			return nil, err
		}
		elite, err := freeStageChoose(r.ids("elite_dungeon_id"), r.ids("elite_dungeon_weights"))
		if err != nil {
			return nil, err
		}
		boss, err := houseFrageChoice(r.ids("boss_dungeon_id"))
		if err != nil {
			return nil, err
		}
		kind, err := freeStageChoose(r.ids("site_type_range"), r.ids("site_type_weights"))
		if err != nil {
			return nil, err
		}
		site := &FreeStageSite{ID: node, State: 2, Type: kind, Monster: monster, Elite: elite, Boss: boss}
		if kind == 3 {
			e := remainingData("free_stage_site_event", r.integer("event_id"))
			values := [][2]int{{3, 0}, {4, 0}}
			weights := []int{e.integer("translate_to_treasure_weight"), e.integer("double_boss_bonus_weight")}
			for i, did := range e.ids("dungeon_ids") {
				if i >= len(e.ids("dungeon_weights")) {
					return nil, errors.New("探索事件战斗配置不完整")
				}
				values = append(values, [2]int{1, did})
				weights = append(weights, e.ids("dungeon_weights")[i])
			}
			for i, bid := range e.ids("bonus_ids") {
				if i >= len(e.ids("bonus_weights")) {
					return nil, errors.New("探索事件奖励配置不完整")
				}
				values = append(values, [2]int{2, bid})
				weights = append(weights, e.ids("bonus_weights")[i])
			}
			index, err := activityWeighted(weights)
			if err != nil {
				return nil, err
			}
			site.Event, site.EventParam = values[index][0], values[index][1]
		}
		s.Sites[node] = site
		if r.integer("start_point_flag") != 0 {
			starts = append(starts, node)
		}
	}
	start, err := houseFrageChoice(starts)
	if err != nil {
		return nil, err
	}
	end, err := houseFrageChoice(remainingData("free_stage_site", start).ids("end_points_set"))
	if err != nil {
		return nil, err
	}
	if s.Sites[end] == nil {
		return nil, errors.New("探索终点不属于本章")
	}
	s.Sites[start].State, s.Sites[start].Type = 1, 1
	s.Sites[end].Type = 4
	return s, nil
}
func freeStageUnlocked(p Progress, id int) error {
	if len(freeStageNodes(id)) == 0 {
		return errors.New("探索章节不存在")
	}
	if err := activityData("activity_type", 2).conditions("unlock_condition", p, p.AvatarLevel); err != nil {
		return err
	}
	chapter := remainingData("sites", id).integer("chapter_id")
	if err := remainingChapterUnlocked(p, chapter); err != nil {
		return err
	}
	if chapter > 1 && !remainingChapterFinished(p, chapter-1) {
		return errors.New("探索前一章节尚未全部完成")
	}
	var mapping map[int][]int
	_ = json.Unmarshal(androidRemainingGameplay["chapter_site_dungeon"]["site_unlock_dict"], &mapping)
	prior := mapping[id]
	if len(prior) == 0 {
		return errors.New("探索章节主线解锁来源缺失")
	}
	for _, site := range prior {
		if !containsInt(p.ClearedDungeons, site) {
			return errors.New("探索章节主线前置未通关")
		}
	}
	return nil
}
func freeStageSiteDungeon(site *FreeStageSite) int {
	switch site.Type {
	case 1:
		return site.Monster
	case 4:
		return site.Boss
	case 5:
		return site.Elite
	case 3:
		if site.Event == 1 {
			return site.EventParam
		}
	}
	return 0
}
func finishFreeStageSite(p *Progress, s *FreeStageState, node int, now time.Time) error {
	site := s.Sites[node]
	if site == nil || site.State != 1 {
		return errors.New("探索节点不是可完成态")
	}
	site.State = 3
	for _, next := range remainingData("free_stage_site", node).ids("unlock_sites") {
		if s.Sites[next] == nil {
			return errors.New("探索相邻节点不属于本章")
		}
		if s.Sites[next].State == 2 {
			s.Sites[next].State = 1
		}
	}
	if site.Type == 4 {
		s.State = 3
		advanceAchievementEvent(p, 6, s.ID, now)
	}
	return nil
}
func addFreeStageBox(s *FreeStageState, box map[string]any) error {
	if s.Bonus == nil {
		s.Bonus = emptyActivityBox()
	}
	// JSONB重载将materials键转为文本，事务内按真实整数材料恢复后合并。
	if _, ok := s.Bonus["materials"].(map[int]int64); !ok {
		raw, err := json.Marshal(s.Bonus["materials"])
		if err != nil {
			return err
		}
		var materials map[int]int64
		if err = json.Unmarshal(raw, &materials); err != nil {
			return errors.New("探索累计奖励材料存档无效")
		}
		if materials == nil {
			materials = map[int]int64{}
		}
		s.Bonus["materials"] = materials
	}
	return mergeActivityBox(s.Bonus, box)
}
func settleFreeStage(p *Progress, b *BattleSession, win bool, box map[string]any, now time.Time) error {
	if dungeonCatalog[b.DungeonID].Type != 2 {
		return nil
	}
	w := ensureRemainingState(p)
	id, node := remainingExtraInt(b.Extra, "server_free_stage_id"), remainingExtraInt(b.Extra, "server_free_stage_node")
	s := w.Stages[id]
	if s == nil || node <= 0 || s.Sites[node] == nil || freeStageSiteDungeon(s.Sites[node]) != b.DungeonID {
		return errors.New("探索结算没有原节点授权")
	}
	if !win {
		return nil
	}
	if err := addFreeStageBox(s, box); err != nil {
		return err
	}
	if err := finishFreeStageSite(p, s, node, now); err != nil {
		return err
	}
	w.PendingSite, w.PendingDungeon = 0, 0
	return nil
}
func (s *Service) freeStageRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("探索需要玩家状态")
	}
	var id int
	var refresh bool
	if method == "set_free_stage_power" || method == "set_auto_free_stage_power" || method == "set_auto_state" || method == "set_auto_fighting" || method == "set_auto_list" || method == "unlock_chapter" {
		return s.freeStageSettingsRPC(ctx, c, method, args)
	}
	if method == "enter_free_stage" {
		if len(args) != 2 || json.Unmarshal(args[0], &id) != nil || json.Unmarshal(args[1], &refresh) != nil {
			return nil, errors.New("探索进入参数无效")
		}
		if err := s.updateProgress(ctx, c, func(p *Progress) error {
			if err := freeStageUnlocked(*p, id); err != nil {
				return err
			}
			if p.Battle != nil && !p.Battle.Finished {
				return errors.New("战斗中不能刷新探索")
			}
			w := ensureRemainingState(p)
			old := w.Stages[id]
			if old == nil || refresh || old.State == 3 {
				cycle := int64(1)
				if old != nil {
					if old.Cycle == math.MaxInt64 {
						return errors.New("探索周次溢出")
					}
					cycle = old.Cycle + 1
				}
				fresh, err := newFreeStage(id, cycle)
				if err != nil {
					return err
				}
				w.Stages[id] = fresh
			}
			w.CurrentStage = id
			if old == nil || refresh {
				w.AutoList = []int{}
			}
			w.PendingSite, w.PendingDungeon = 0, 0
			return nil
		}); err != nil {
			return []Push{nativeErrorPush(activityErrorCode(err))}, nil
		}
		return append(remainingGameplayPushes(c.SelectedAvatarUnsafe().Progress), push("Avatar", "on_enter_free_stage", id, refresh)), nil
	}
	if method == "leave_free_stage" {
		if len(args) != 0 {
			return nil, errors.New("离开探索参数无效")
		}
		if err := s.updateProgress(ctx, c, func(p *Progress) error {
			if p.Battle != nil && !p.Battle.Finished {
				return errors.New("战斗中不能离开探索")
			}
			w := ensureRemainingState(p)
			w.CurrentStage, w.PendingSite, w.PendingDungeon = 0, 0, 0
			return nil
		}); err != nil {
			return nil, err
		}
		return remainingGameplayPushes(c.SelectedAvatarUnsafe().Progress), nil
	}
	if method == "enter_stage_site" {
		var delay int
		if len(args) != 2 || json.Unmarshal(args[0], &id) != nil || json.Unmarshal(args[1], &delay) != nil || delay < 0 {
			return nil, errors.New("探索节点参数无效")
		}
		var dungeon int
		var box map[string]any
		var ended bool
		err := s.updateProgress(ctx, c, func(p *Progress) error {
			w := ensureRemainingState(p)
			st := w.Stages[w.CurrentStage]
			if st == nil || st.State == 3 || st.Sites[id] == nil || st.Sites[id].State != 1 {
				return errors.New("探索节点不可用")
			}
			if p.Battle != nil && !p.Battle.Finished {
				return errors.New("已有未结束战斗")
			}
			site := st.Sites[id]
			dungeon = freeStageSiteDungeon(site)
			w.PendingSite, w.PendingDungeon = id, dungeon
			if dungeon > 0 {
				return nil
			}
			row := remainingData("free_stage_site", id)
			cost := row.integer("treasure_need_power")
			bonus := row.integer("treasure_bonus_id")
			if site.Type == 3 {
				cost = row.integer("event_need_power")
				if site.Event == 2 {
					bonus = site.EventParam
				} else if site.Event == 3 {
					site.Type = 2
				} else if site.Event == 4 {
					st.BossDouble = true
					bonus = 0
				} else {
					return errors.New("探索事件类型无效")
				}
			}
			multiple := w.PowerType
			if multiple == 0 {
				multiple = 1
			}
			if w.AutoFighting {
				if n := w.PowerTypes[id]; n > 0 {
					multiple = n
				}
			}
			if multiple != 1 && multiple != 2 {
				return errors.New("探索体力倍率无效")
			}
			if cost < 0 || !p.consumePower(cost*multiple, s.Now()) {
				return errors.New("探索事件体力不足")
			}
			box = emptyActivityBox()
			if bonus > 0 {
				var err error
				box, err = grantNativeBonus(p, bonus, int64(multiple), p.AvatarLevel, s.Now())
				if err != nil {
					return err
				}
			}
			if err := addFreeStageBox(st, box); err != nil {
				return err
			}
			if err := finishFreeStageSite(p, st, id, s.Now()); err != nil {
				return err
			}
			ended = st.State == 3
			w.PendingSite, w.PendingDungeon = 0, 0
			return nil
		})
		if err != nil {
			return []Push{push("Avatar", "on_enter_stage_site", id, activityErrorCode(err))}, nil
		}
		out := append(remainingGameplayPushes(c.SelectedAvatarUnsafe().Progress), push("Avatar", "on_enter_stage_site", id, RetSuccess))
		if dungeon > 0 {
			battle, err := s.enterDungeon(ctx, c, []json.RawMessage{json.RawMessage("0"), json.RawMessage(strconv.Itoa(dungeon)), json.RawMessage(`{}`)})
			if err != nil {
				return append(out, nativeErrorPush(activityErrorCode(err))), nil
			}
			return append(out, battle...), nil
		}
		out = append(out, materialManagerPush(c), cardMgrPush(c), push("Avatar", "on_get_treasure_in_site", box), push("Avatar", "on_finish_stage_site", id))
		if ended {
			out = append(out, push("Avatar", "on_finish_free_stage", c.SelectedAvatarUnsafe().Progress.RemainingGameplay.CurrentStage))
		}
		return out, nil
	}
	return nil, errors.New("探索接口尚未接线")
}
