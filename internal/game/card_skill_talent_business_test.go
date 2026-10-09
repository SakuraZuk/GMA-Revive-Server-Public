package game

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"
)

func talentResultCode(t *testing.T, pushes []Push) int {
	t.Helper()
	for _, item := range pushes {
		if item.Method == "on_upgrade_talent_node" {
			if len(item.Args) != 4 {
				t.Fatal("原生潜质回调参数数目错误")
			}
			code, ok := item.Args[0].(int)
			if !ok {
				t.Fatal("原生潜质回调代码类型错误")
			}
			return code
		}
	}
	t.Fatal("缺少原生潜质升级回调")
	return -1
}

func cardSkillTalentWorld(t *testing.T, cardID int) (*Service, *Connection, *FixtureAccounts, string) {
	t.Helper()
	ctx := context.Background()
	store := NewFixtureAccounts(nil)
	s := New(store, nil)
	s.Now = func() time.Time { return time.Unix(1791360000, 0) }
	c, av := newBattleConnection(t, ctx, store, s)
	uuid := "00112233445566778899aabb"
	if _, err := store.UpdateProgress(ctx, av.OID, func(p *Progress) error {
		card := newCard(cardID, 60, s.Now())
		card.UUID = uuid
		card.Grade = 5
		p.Cards = []Card{card}
		for _, id := range []int{12, 211, 212, 213, 214, 221, 222, 223, 224, 231, 232, 233, 234, 241, 242, 243, 244, 251, 252, 253, 254, 261, 262, 263, 264, 314, 324, 334, 344, 354, 364} {
			p.Materials[id] = Material{ID: id, Count: 10000000, Total: 10000000}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return s, c, store, uuid
}

func TestCardSkillActualHandleCostsOwnershipAndTransactionRollback(t *testing.T) {
	s, c, store, uuid := cardSkillTalentWorld(t, 4401)
	ctx := context.Background()
	before, err := store.UpdateProgress(ctx, selectedOID(c), func(p *Progress) error { p.Materials[251] = Material{ID: 251, Count: 14, Total: 14}; return nil })
	if err != nil {
		t.Fatal(err)
	}
	args := socialArgs(1, uuid, 440101)
	pushes, err := s.Handle(ctx, c, "upgrade_card_skill", args)
	if err != nil {
		t.Fatal(err)
	}
	if socialCallback(t, pushes)[0] == 0 {
		t.Fatal("技能材料不足仍成功")
	}
	after, err := store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("技能第二项材料不足后金币或初始化状态部分提交", err)
	}
	if _, err = store.UpdateProgress(ctx, selectedOID(c), func(p *Progress) error { p.Materials[251] = Material{ID: 251, Count: 15, Total: 15}; return nil }); err != nil {
		t.Fatal(err)
	}
	pushes, err = s.Handle(ctx, c, "upgrade_card_skill", args)
	if err != nil || socialCallback(t, pushes)[0] != 0 {
		t.Fatal("技能实际Handle未按原生签名成功", err, pushes)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Cards[0].Skills[0].Level != 2 || p.Materials[12].Count != before.Materials[12].Count-20000 || p.Materials[251].Count != 0 || p.Materials[251].Total != 15 || p.Achievements[304001].Targets[304001] != 1 {
		t.Fatal("技能1到2成本、累计量或成就未按原生表提交")
	}
	before = CloneProgress(p)
	pushes, err = s.Handle(ctx, c, "upgrade_card_skill", socialArgs(2, uuid, 99999))
	if err != nil || socialCallback(t, pushes)[0] == 0 {
		t.Fatal("外部技能编号未拒绝", err)
	}
	if !reflect.DeepEqual(before, CloneProgress(c.SelectedAvatarUnsafe().Progress)) {
		t.Fatal("非法技能仍修改玩家状态")
	}
}

func TestCardSkillFullFiveNativeWireAndMaximumRepeat(t *testing.T) {
	s, c, store, uuid := cardSkillTalentWorld(t, 4401)
	ctx := context.Background()
	for _, id := range []int{440101, 440102, 440103} {
		for count := 0; count < 4; count++ {
			pushes, err := s.Handle(ctx, c, "upgrade_card_skill", socialArgs(1, uuid, id))
			if err != nil || socialCallback(t, pushes)[0] != 0 {
				t.Fatal(id, count, err, pushes)
			}
		}
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Achievements[304002].Targets[304002] != 1 || p.Achievements[304003].Targets[304006] != 1 {
		t.Fatal("全部三项战斗技能五级没有推进原生27/24目标")
	}
	wire := cardMgrProperties(p.Cards)[uuid].(map[string]any)
	skills, ok := wire["skill_mgr"].([]CardSkill)
	if !ok || len(skills) != 3 || skills[0].ID != 440101 || skills[0].Level != 5 {
		t.Fatal("原生技能CustomList下发缺失")
	}
	before := CloneProgress(p)
	pushes, err := s.Handle(ctx, c, "upgrade_card_skill", socialArgs(1, uuid, 440101))
	if err != nil || socialCallback(t, pushes)[0] == 0 {
		t.Fatal("满级重复技能升级仍成功", err)
	}
	after, err := store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("满级重复仍扣材料或推进成就", err)
	}
	var restored Progress
	raw, _ := json.Marshal(after)
	if json.Unmarshal(raw, &restored) != nil || !reflect.DeepEqual(after.Cards, restored.Cards) {
		t.Fatal("技能与潜质状态JSON往返丢失")
	}
}

func TestCardTalentActualHandlePrerequisiteQuotaAchievementAndDuplicate(t *testing.T) {
	s, c, store, uuid := cardSkillTalentWorld(t, 1601)
	ctx := context.Background()
	before, err := store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	pushes, err := s.Handle(ctx, c, "upgrade_talent_node", socialArgs(uuid, 101, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if talentResultCode(t, pushes) == 0 {
		t.Fatal("没有完成前置仍升级指定潜质")
	}
	after, _ := store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, after) {
		t.Fatal("潜质前置失败仍提交节点初始化")
	}
	// 本版洛伦佐普通分支0三个节点分别达到3级，才解锁101技能分支。
	for index := 0; index < 3; index++ {
		for level := 0; level < 3; level++ {
			pushes, err = s.Handle(ctx, c, "upgrade_talent_node", socialArgs(uuid, 0, index, level))
			if err != nil || talentResultCode(t, pushes) != 0 {
				t.Fatal(index, level, err)
			}
		}
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Achievements[3011601].Targets[3011601] != 0 {
		t.Fatal("普通潜质升级冒充指定祝佑辉光")
	}
	if _, err = store.UpdateProgress(ctx, selectedOID(c), func(p *Progress) error { p.Materials[264] = Material{ID: 264, Count: 130, Total: 130}; return nil }); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		pushes, err = s.Handle(ctx, c, "upgrade_talent_node", socialArgs(uuid, 101, index, 0))
		if err != nil || talentResultCode(t, pushes) != 0 {
			t.Fatal(index, err)
		}
	}
	p = c.SelectedAvatarUnsafe().Progress
	if p.Materials[264].Count != 0 || p.Materials[264].Total != 130 || p.Achievements[3011601].Targets[3011601] != 1 || p.Cards[0].TalentTree[101][1].ID != 31006 || p.Cards[0].TalentTree[101][1].Level != 1 {
		t.Fatal("指定潜质成本或一基成就节点映射错误")
	}
	before = CloneProgress(p)
	pushes, err = s.Handle(ctx, c, "upgrade_talent_node", socialArgs(uuid, 101, 1, 0))
	if err != nil || talentResultCode(t, pushes) == 0 {
		t.Fatal("旧currentLevel重复请求没有拒绝", err)
	}
	after, _ = store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, after) {
		t.Fatal("同节点旧等级重播再次扣费或完成事件")
	}
	card := p.Cards[0]
	card.Grade = 3
	card.EnhanceCount = 0
	limit, err := nativeTalentUpgradeLimit(p, card)
	if err != nil || limit != 5 {
		t.Fatal("原生升品额度按grade前项求和错误", limit, err)
	}
	if cardMgrProperties(p.Cards)[uuid].(map[string]any)["talent_tree"].(map[int][]CardTalentNode)[101][1].Level != 1 {
		t.Fatal("原生Int键潜质树未下发已升级状态")
	}
}

func TestCardTalentInsufficientQuotaAndMaterialWholeRollback(t *testing.T) {
	s, c, store, uuid := cardSkillTalentWorld(t, 1601)
	ctx := context.Background()
	before, err := store.UpdateProgress(ctx, selectedOID(c), func(p *Progress) error {
		p.Cards[0].Grade = 3
		p.Cards[0].TalentTree = nativeCardTalentTree(p.Cards[0])
		p.Cards[0].TalentTree[0][0].Level = 5
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pushes, err := s.Handle(ctx, c, "upgrade_talent_node", socialArgs(uuid, 1, 0, 0))
	if err != nil || talentResultCode(t, pushes) == 0 {
		t.Fatal("全树真实额度用尽仍升级", err)
	}
	after, _ := store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, after) {
		t.Fatal("额度拒绝后仍部分提交")
	}
	before, err = store.UpdateProgress(ctx, selectedOID(c), func(p *Progress) error {
		p.Cards[0].Grade = 5
		p.Cards[0].TalentTree = nil
		p.Materials[261] = Material{ID: 261, Count: 9, Total: 9}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pushes, err = s.Handle(ctx, c, "upgrade_talent_node", socialArgs(uuid, 0, 0, 0))
	if err != nil || talentResultCode(t, pushes) == 0 {
		t.Fatal("潜质材料不足仍升级", err)
	}
	after, _ = store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, after) {
		t.Fatal("潜质材料失败后节点或事件部分提交")
	}
}

func TestCardTalentAll34OriginalAchievementNodesThroughActualRPC(t *testing.T) {
	ids := []int{}
	for id, target := range androidAchievements.Targets {
		if target.Type == 59 {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	if len(ids) != 34 {
		t.Fatal("本版指定潜质成就目标数改变", len(ids))
	}
	for _, targetID := range ids {
		t.Run(fmt.Sprint(targetID), func(t *testing.T) {
			target := androidAchievements.Targets[targetID]
			var cardID int
			if len(target.Params) != 1 || json.Unmarshal(target.Params[0], &cardID) != nil {
				t.Fatal("原生潜质成就参数无效")
			}
			s, c, _, uuid := cardSkillTalentWorld(t, cardID)
			ctx := context.Background()
			rule := androidCardSkillTalent.Talents[cardID]
			if len(rule.AchievementNode) != 2 {
				t.Fatal("指定潜质目标缺少原生节点坐标", cardID)
			}
			branch, index := rule.AchievementNode[0], rule.AchievementNode[1]-1
			if branch != 101 {
				branch--
			}
			baseline := nativeCardTalentTree(Card{CardID: cardID})
			upgrade := func(branch, index, times int) {
				for level := 0; level < times; level++ {
					pushes, err := s.Handle(ctx, c, "upgrade_talent_node", socialArgs(uuid, branch, index, level))
					if err != nil || talentResultCode(t, pushes) != 0 {
						t.Fatal("原生指定节点前置或升级失败", cardID, branch, index, level, err)
					}
				}
			}
			if branch == 101 {
				// 原生get_previous_talent_node对101首节点接受任一普通分支末节点的完成阈值。
				for position, node := range baseline[0] {
					upgrade(0, position, androidCardSkillTalent.Nodes[node.ID].UnlockNext)
				}
			}
			for position := 0; position < index; position++ {
				node := baseline[branch][position]
				upgrade(branch, position, androidCardSkillTalent.Nodes[node.ID].UnlockNext)
			}
			if c.SelectedAvatarUnsafe().Progress.Achievements[targetID].Targets[targetID] != 0 {
				t.Fatal("前置升级冒充指定成就节点")
			}
			upgrade(branch, index, 1)
			p := c.SelectedAvatarUnsafe().Progress
			if p.Achievements[targetID].Targets[targetID] != 1 || p.Cards[0].TalentTree[branch][index].Level != 1 {
				t.Fatal("指定节点真实升级没有推进对应原生成就")
			}
		})
	}
}

func TestCardSkillNewCardGradeOriginalCapsAndStoredGradePreserved(t *testing.T) {
	for _, example := range []struct{ level, grade int }{{1, 0}, {10, 0}, {11, 1}, {20, 1}, {21, 2}, {30, 2}, {31, 3}, {40, 3}, {41, 4}, {50, 4}, {51, 5}, {60, 5}, {70, 5}} {
		card := newCard(4401, example.level, time.Unix(1791360000, 0))
		if card.Grade != example.grade {
			t.Fatal("新建卡品阶不符原生creator默认上限映射", example, card.Grade)
		}
	}
	// 旧存档可能确实已突破；读取/下发不从当前低等级反推降品阶。
	stored := Card{CardID: 4401, Level: 1, Grade: 1}
	nativeCardSkills(stored)
	if stored.Grade != 1 {
		t.Fatal("兼容读取覆盖旧品阶")
	}
	newborn := newCard(4401, 1, time.Unix(1791360000, 0))
	if _, ok := nativeCardSkillRule(newborn, 440103, 1); ok {
		t.Fatal("品阶0新卡提前解锁原生第三技能")
	}
	newborn.Grade = 1
	if _, ok := nativeCardSkillRule(newborn, 440103, 1); !ok {
		t.Fatal("真实首次突破后原生第三技能仍锁定")
	}
}
