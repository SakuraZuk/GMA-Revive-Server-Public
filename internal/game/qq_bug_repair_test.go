package game

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"testing"
)

func TestQQBugGlobalVoiceRefreshAndLegacyProjection(t *testing.T) {
	s, c, store, _ := basicFixture(t)
	ctx := context.Background()
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.CardVoices = map[int]int{4401: 2}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out, err := s.Handle(ctx, c, "set_global_vo", rawArgs(771, 1))
	if err != nil || len(out) != 4 || out[3].Method != "call_client_callback" {
		t.Fatal("默认中配没有先同步再回调", err, out)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.GlobalVO != 1 || p.GlobalSetVO != 1 || effectiveCardVoice(p, 4401) != 1 {
		t.Fatal("旧单卡日配覆盖了新的全局中配")
	}
	for _, card := range p.Cards {
		common := cardCommonMgrPropertiesWithProgress(p)[strconv.Itoa(card.CardID)].(map[string]any)
		if common["card_vo"] == 0 {
			t.Fatal("缺省单卡语种0覆盖了全局中文选择")
		}
	}
	if _, err := s.Handle(ctx, c, "set_card_vo", rawArgs(772, 4401, 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Handle(ctx, c, "set_global_vo", rawArgs(773, 0)); err != nil {
		t.Fatal(err)
	}
	p = c.SelectedAvatarUnsafe().Progress
	if effectiveGlobalVoice(p) != 1 || effectiveCardVoice(p, 4401) != 2 {
		t.Fatal("单卡模式丢失默认中配或单卡日配")
	}
	cold, err := store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if err != nil || effectiveGlobalVoice(cold) != 1 || effectiveCardVoice(cold, 4401) != 2 {
		t.Fatal("配音没有冷持久化", err)
	}
	legacy := Progress{GlobalVO: 1}
	if effectiveCardVoice(legacy, 4401) != 1 || (Avatar{Progress: legacy}).InitialProperties("")["global_set_vo"] != 1 {
		t.Fatal("旧中配存档未兼容")
	}
}

func TestQQBugAwakenThresholdAndPaidDressIsolation(t *testing.T) {
	count := 0
	for id, rule := range androidCardAppearances {
		if rule.AwakenGrade <= 0 || rule.AwakenedDress == 0 || rule.AwakenedDress == rule.DefaultDress {
			continue
		}
		card := Card{CardID: id, UUID: "00112233445566778899aabb", Grade: rule.AwakenGrade - 1}
		p := Progress{Cards: []Card{card}}
		if effectiveCardAwakened(card) != 0 || containsInt(ownedCardDresses(p, id), rule.AwakenedDress) {
			t.Fatal("提前解锁觉醒装帧", id)
		}
		p.Cards[0].Grade++
		if !validOwnedDress(p, id, rule.AwakenedDress) || cardMgrProperties(p.Cards)[card.UUID].(map[string]any)["awakened"] != 1 {
			t.Fatal("达到原表位格没有解锁", id)
		}
		for _, dress := range rule.Dresses {
			if dress != rule.DefaultDress && dress != rule.AwakenedDress && validOwnedDress(p, id, dress) {
				t.Fatal("越权授予额外装帧", id, dress)
			}
		}
		if p.Cards[0].Awakened != 0 {
			t.Fatal("只读投影修改旧存档")
		}
		count++
	}
	if count < 50 {
		t.Fatal("未覆盖实际卡牌目录", count)
	}
}

func TestQQBugExploreDailyCompletionClaimAndResultPush(t *testing.T) {
	s, c, _ := remainingFixture(t)
	remainingRPC(t, s, c, "enter_free_stage", 10101, true)
	for steps := 0; steps < 20 && c.SelectedAvatarUnsafe().Progress.RemainingGameplay.Stages[10101].State != 3; steps++ {
		st := c.SelectedAvatarUnsafe().Progress.RemainingGameplay.Stages[10101]
		available := []int{}
		for id, site := range st.Sites {
			if site.State == 1 {
				available = append(available, id)
			}
		}
		sort.Ints(available)
		if len(available) == 0 {
			t.Fatal("没有可推进节点")
		}
		node := available[0]
		did := freeStageSiteDungeon(st.Sites[node])
		remainingRPC(t, s, c, "enter_stage_site", node, 0)
		if did > 0 {
			out := remainingFinishBattle(t, s, c, nil, true)
			seen, result := false, false
			for _, item := range out {
				if item.Method == "client_prop_changed" && item.Args[0].([]any)[0] == "daily_tasks" {
					seen = true
				}
				if item.Method == "battle_result" {
					result = true
					if !seen {
						t.Fatal("战斗结果前缺日常进度同步")
					}
				}
			}
			if !result {
				t.Fatal("缺实际战斗结算")
			}
		}
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.RemainingGameplay.Stages[10101].State != 3 || p.BasicRewards.Daily[4].Targets[1013] != 1 {
		t.Fatal("探索完成未推进日常目标", p.BasicRewards.Daily[4])
	}
	for _, id := range []int{105, 106, 107, 108} {
		if p.NewTasks[id].FinishedTargets[20101] != 1 {
			t.Fatal("首次完整探索未推进第一天探索任务", id, p.NewTasks[id])
		}
	}
	if basicReply(t, s, c, "receive_new_task_bonus", 773, 105)[0] != RetSuccess {
		t.Fatal("第一天探索一次奖励无法领取")
	}
	if basicReply(t, s, c, "receive_daily_task_active", 774, 4)[0] != true {
		t.Fatal("探索日常无法领取")
	}
	p = CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if !p.BasicRewards.Daily[4].Claimed || p.BasicRewards.DailyActive != 20 {
		t.Fatal("领奖态或活跃度错误")
	}
	if basicReply(t, s, c, "receive_daily_task_active", 775, 4)[0] != false || !reflect.DeepEqual(p, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("重复日常领奖不幂等")
	}
	raw, _ := json.Marshal(p)
	var cold Progress
	if json.Unmarshal(raw, &cold) != nil || !cold.BasicRewards.Daily[4].Claimed {
		t.Fatal("冷存档领取态丢失")
	}
}
