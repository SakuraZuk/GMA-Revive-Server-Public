package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// 原生 skill_mgr 是 CustomList，talent_tree 是 Int→talent_branch(CustomList)。
type CardSkill struct {
	ID           int `json:"skill_id"`
	Level        int `json:"level"`
	EnhanceLevel int `json:"enhance_level"`
}
type CardTalentNode struct {
	ID    int `json:"node_id"`
	State int `json:"state"`
	Level int `json:"has_upgrade_time"`
}

//go:embed card_skill_talent_catalog.json
var cardSkillTalentCatalogRaw []byte

type nativeSkillRule struct {
	ID         int   `json:"id"`
	Grades     []int `json:"skill_grade"`
	CardGrades []int `json:"card_grade"`
	UpgradeID  int   `json:"upgrade_id"`
	Forbid     int   `json:"forbid"`
}
type nativeTalentRule struct {
	Branches        [][]int `json:"talent_branch"`
	SkillBranch     []int   `json:"skill_talent_branch"`
	UnlockLevel     int     `json:"unlock_level"`
	AchievementNode []int   `json:"achievement_node"`
}
type nativeTalentNodeRule struct {
	Forbid     int       `json:"node_forbid"`
	Times      int       `json:"talent_node_upgrade_times"`
	UnlockNext int       `json:"unlock_post_node_condition"`
	Costs      [][][]int `json:"talent_node_upgrade_material"`
	Replace    []int     `json:"replace_skill_id"`
}
type cardSkillTalentCatalog struct {
	Cards map[int]struct {
		RoleID int `json:"role_id"`
	} `json:"cards"`
	Roles map[int]struct {
		Skills  []int `json:"skill_list"`
		Enhance []int `json:"skill_enhance_sort"`
	} `json:"roles"`
	Skills   []nativeSkillRule `json:"skills"`
	Upgrades []struct {
		ID    int     `json:"id"`
		Level int     `json:"level"`
		Costs [][]int `json:"cost"`
	} `json:"upgrades"`
	Grades map[int]struct {
		Grade int `json:"skill_grade"`
	} `json:"grades"`
	Talents map[int]nativeTalentRule     `json:"talents"`
	Nodes   map[int]nativeTalentNodeRule `json:"nodes"`
	Limit   struct {
		Base     int   `json:"base_limit"`
		Level    []int `json:"level_provide_limit"`
		Grade    []int `json:"grade_provide_limit"`
		Intimacy []int `json:"intimacy_provide_limit"`
		Ring     int   `json:"ring_provide_limit"`
		Enhance  int   `json:"enhance_provide_limit"`
	} `json:"limit"`
}

var androidCardSkillTalent = func() cardSkillTalentCatalog {
	var out cardSkillTalentCatalog
	if json.Unmarshal(cardSkillTalentCatalogRaw, &out) != nil || len(out.Skills) != 1529 || len(out.Talents) != 54 || len(out.Grades) != 5 {
		panic("原生技能潜质目录无效")
	}
	return out
}()

func nativeCardSkills(card Card) []CardSkill {
	role := androidCardSkillTalent.Roles[androidCardSkillTalent.Cards[card.CardID].RoleID]
	result := slices.Clone(card.Skills)
	if len(result) == 0 {
		result = []CardSkill{}
		for _, id := range role.Skills {
			result = append(result, CardSkill{ID: id, Level: 1})
		}
	}
	// 旧存档有真实 SkillEnhanceCount；原生技能补完顺序一基索引映射到List位置。
	for i := range result {
		result[i].EnhanceLevel = 0
	}
	for _, index := range role.Enhance[:min(max(card.SkillEnhanceCount, 0), len(role.Enhance))] {
		if index > 0 && index <= len(result) {
			result[index-1].EnhanceLevel = min(result[index-1].EnhanceLevel+1, 2)
		}
	}
	return result
}

// 缺失状态只构造原生未升级节点；不能从资产或成就反推旧技能/潜质级别。
func nativeCardTalentTree(card Card) map[int][]CardTalentNode {
	result := map[int][]CardTalentNode{}
	rule, ok := androidCardSkillTalent.Talents[card.CardID]
	if !ok {
		return result
	}
	branches := map[int][]int{}
	for index, ids := range rule.Branches {
		branches[index] = ids
	}
	if len(rule.SkillBranch) > 0 {
		branches[101] = rule.SkillBranch
	}
	for branch, ids := range branches {
		for index, id := range ids {
			node := CardTalentNode{ID: id}
			if index < len(card.TalentTree[branch]) && card.TalentTree[branch][index].ID == id {
				node = card.TalentTree[branch][index]
			}
			result[branch] = append(result[branch], node)
		}
	}
	// 原生 update_talent_tree 的各普通分支首节点自动unlock，后续只按真实前置解锁。
	keys := []int{}
	for branch := range result {
		keys = append(keys, branch)
	}
	slices.Sort(keys) // 原生先普通分支、后101技能分支，解锁顺序不能受Go map遍历影响。
	for _, branch := range keys {
		nodes := result[branch]
		for index := range nodes {
			if index == 0 && branch != 101 {
				nodes[index].State = 1
			}
			if androidCardSkillTalent.Nodes[nodes[index].ID].Forbid != 0 {
				continue
			}
			unlock := index == 0 && branch != 101
			if index > 0 {
				pre := nodes[index-1]
				unlock = pre.State == 1 && pre.Level >= androidCardSkillTalent.Nodes[pre.ID].UnlockNext
			}
			if branch == 101 && index == 0 {
				for key, previous := range result {
					if key == 101 || len(previous) == 0 {
						continue
					}
					pre := previous[len(previous)-1]
					if pre.State == 1 && pre.Level >= androidCardSkillTalent.Nodes[pre.ID].UnlockNext {
						unlock = true
						break
					}
				}
			}
			if unlock {
				nodes[index].State = 1
			}
		}
		result[branch] = nodes
	}
	return result
}

func nativeCardSkillRule(card Card, id, level int) (nativeSkillRule, bool) {
	grade, ok := androidCardSkillTalent.Grades[level]
	if !ok {
		return nativeSkillRule{}, false
	}
	for _, row := range androidCardSkillTalent.Skills {
		if row.ID == id && containsInt(row.Grades, grade.Grade) && containsInt(row.CardGrades, card.Grade) {
			return row, true
		}
	}
	return nativeSkillRule{}, false
}

func validateCardSkillTalentState(card Card) error {
	role := androidCardSkillTalent.Roles[androidCardSkillTalent.Cards[card.CardID].RoleID]
	if len(card.Skills) > 0 {
		if len(card.Skills) != len(role.Skills) {
			return errors.New("技能组长度与原生角色不一致")
		}
		for index, skill := range card.Skills {
			if skill.ID != role.Skills[index] || skill.Level < 1 || skill.Level > len(androidCardSkillTalent.Grades) {
				return errors.New("技能编号、顺序或等级存档无效")
			}
		}
	}
	if card.SkillEnhanceCount < 0 || card.SkillEnhanceCount > len(role.Enhance) {
		return errors.New("技能补完次数存档无效")
	}
	if len(card.TalentTree) > 0 {
		baseline := nativeCardTalentTree(Card{CardID: card.CardID})
		if len(card.TalentTree) != len(baseline) {
			return errors.New("潜质分支与原生角色不一致")
		}
		for branch, nodes := range card.TalentTree {
			if len(nodes) != len(baseline[branch]) {
				return errors.New("潜质分支节点数量无效")
			}
			for index, node := range nodes {
				if node.ID != baseline[branch][index].ID || node.Level < 0 || node.Level > androidCardSkillTalent.Nodes[node.ID].Times || node.State < 0 || node.State > 1 {
					return errors.New("潜质节点编号、状态或等级存档无效")
				}
			}
		}
	}
	return nil
}

func upgradeCardSkillNative(p *Progress, uuid string, skillID int, now time.Time) error {
	_, card := findCard(p, uuid)
	if err := checkCardGrowth(card); err != nil {
		return err
	}
	if err := validateCardSkillTalentState(*card); err != nil {
		return err
	}
	card.Skills = nativeCardSkills(*card)
	card.TalentTree = nativeCardTalentTree(*card)
	// 客户端原生先把已激活替换技能ID还原为基础技能ID，服务端仍进行同一校验。
	for _, nodes := range card.TalentTree {
		for _, node := range nodes {
			rule := androidCardSkillTalent.Nodes[node.ID]
			if node.State == 1 && node.Level > 0 && len(rule.Replace) == 2 && rule.Replace[1] == skillID {
				skillID = rule.Replace[0]
			}
		}
	}
	index := -1
	for i, s := range card.Skills {
		if s.ID == skillID {
			index = i
			break
		}
	}
	if index < 0 {
		return runeReject("RET_CARD_REACH_MAX_SKILL_LEVEL", "该技能不属于幻书的可升级技能组")
	}
	skill := card.Skills[index]
	rule, ok := nativeCardSkillRule(*card, skill.ID, skill.Level)
	if !ok || skill.Level < 1 || skill.Level >= len(androidCardSkillTalent.Grades) || rule.Forbid != 0 {
		return runeReject("RET_CARD_REACH_MAX_SKILL_LEVEL", "技能未解锁或已达到上限")
	}
	var costs [][]int
	for _, row := range androidCardSkillTalent.Upgrades {
		if row.ID == rule.UpgradeID && row.Level == skill.Level {
			costs = row.Costs
			break
		}
	}
	// 原生无 upgrade_id 的援护/固定技能没有有据成本，拒绝写入而不免费升级。
	if rule.UpgradeID <= 0 || len(costs) == 0 {
		return runeReject("RET_CARD_REACH_MAX_SKILL_LEVEL", "技能没有原生可升级成本")
	}
	if err := runeMaterials(p, costs, -1); err != nil {
		return err
	}
	card.Skills[index].Level++
	for _, target := range androidAchievements.Targets {
		if target.Type != 27 || len(target.Params) != 1 {
			continue
		}
		var need int
		if json.Unmarshal(target.Params[0], &need) == nil && skill.Level < need && card.Skills[index].Level >= need {
			advanceAchievementEvent(p, 27, need, now)
		}
	}
	all := true
	for _, s := range card.Skills {
		if s.Level < 5 {
			all = false
			break
		}
	}
	if all && len(card.Skills) > 0 {
		advanceAchievementEvent(p, 24, 5, now)
	}
	return nil
}

func nativeTalentUpgradeLimit(p Progress, card Card) (int, error) {
	r := androidCardSkillTalent.Limit
	if len(r.Level) != 2 || card.Grade < 0 || card.Grade > len(r.Grade) || card.EnhanceCount < 0 {
		return 0, errors.New("潜质升级额度存档或原生表无效")
	}
	limit := r.Base + max(card.Level-r.Level[0], 0)*r.Level[1] + card.EnhanceCount*r.Enhance
	for _, value := range r.Grade[:card.Grade] {
		limit += value
	}
	common := p.IntimacyCommons[card.CardID]
	for index := 0; index < common.RewardLevel && index < len(r.Intimacy); index++ {
		limit += r.Intimacy[index]
	}
	if common.RingID != 0 {
		limit += r.Ring
	}
	return limit, nil
}

func upgradeTalentNodeNative(p *Progress, uuid string, branch, index, level int, now time.Time) error {
	_, card := findCard(p, uuid)
	if err := checkCardGrowth(card); err != nil {
		return err
	}
	if err := validateCardSkillTalentState(*card); err != nil {
		return err
	}
	rule, ok := androidCardSkillTalent.Talents[card.CardID]
	if !ok || card.Level < rule.UnlockLevel || card.Grade < rule.UnlockLevel/10 {
		return runeReject("RET_FAILED", "潜质树尚未解锁")
	}
	card.TalentTree = nativeCardTalentTree(*card)
	nodes, ok := card.TalentTree[branch]
	if !ok || index < 0 || index >= len(nodes) {
		return runeReject("RET_FAILED", "潜质节点不属于当前幻书")
	}
	node := nodes[index]
	nr := androidCardSkillTalent.Nodes[node.ID]
	if level != node.Level {
		return runeReject("RET_FAILED", "潜质节点等级已改变，请刷新后操作")
	}
	if node.State != 1 || nr.Forbid != 0 || node.Level < 0 || node.Level >= nr.Times || node.Level >= len(nr.Costs) {
		return runeReject("RET_FAILED", "潜质节点未解锁或达到上限")
	}
	limit, err := nativeTalentUpgradeLimit(*p, *card)
	if err != nil {
		return err
	}
	times := 0
	for _, list := range card.TalentTree {
		for _, n := range list {
			if n.Level < 0 {
				return errors.New("潜质节点等级存档无效")
			}
			times += n.Level
		}
	}
	if times >= limit {
		return runeReject("RET_FAILED", "潜质升级额度不足")
	}
	if len(nr.Costs[node.Level]) == 0 {
		return errors.New("潜质节点缺少有据材料成本")
	}
	if err = runeMaterials(p, nr.Costs[node.Level], -1); err != nil {
		return err
	}
	nodes[index].Level++
	card.TalentTree[branch] = nodes
	card.TalentTree = nativeCardTalentTree(*card)
	// achievement_node 表为一基位置，101为专用技能分支常量，其他分支减一。
	if len(rule.AchievementNode) == 2 {
		wantBranch := rule.AchievementNode[0]
		if wantBranch != 101 {
			wantBranch--
		}
		if wantBranch == branch && rule.AchievementNode[1]-1 == index {
			advanceAchievementEvent(p, 59, card.CardID, now)
		}
	}
	return nil
}

func (s *Service) cardSkillTalentRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("技能潜质操作需要玩家状态")
	}
	var uuid string
	if method == "upgrade_card_skill" {
		var callback, skillID int
		if len(args) != 3 || json.Unmarshal(args[0], &callback) != nil || json.Unmarshal(args[1], &uuid) != nil || !validObjectID(uuid) || json.Unmarshal(args[2], &skillID) != nil || skillID <= 0 {
			return nil, errors.New("技能升级原生参数无效")
		}
		if err := s.updateProgress(ctx, c, func(p *Progress) error { return upgradeCardSkillNative(p, uuid, skillID, s.Now()) }); err != nil {
			return growthCallbackError(callback, err)
		}
		return []Push{cardMgrPush(c), materialManagerPush(c), Callback(callback, []any{RetSuccess})}, nil
	}
	if method == "upgrade_talent_node" || method == "reset_talent_node" {
		var branch, index, level int
		if len(args) != 4 || json.Unmarshal(args[0], &uuid) != nil || !validObjectID(uuid) || json.Unmarshal(args[1], &branch) != nil || json.Unmarshal(args[2], &index) != nil || json.Unmarshal(args[3], &level) != nil || level < 0 {
			return nil, errors.New("潜质升级原生参数无效")
		}
		response := "on_upgrade_talent_node"
		if method == "reset_talent_node" {
			response = "on_reset_talent_node"
		}
		err := s.updateProgress(ctx, c, func(p *Progress) error {
			if method == "reset_talent_node" {
				return resetTalentNodeNative(p, uuid, branch, index, level)
			}
			return upgradeTalentNodeNative(p, uuid, branch, index, level, s.Now())
		})
		if err != nil {
			var rejected *runeBusinessError
			if !errors.As(err, &rejected) {
				return nil, err
			}
			rules, e := loadIntimacyCatalog()
			if e != nil {
				return nil, e
			}
			code, ok := rules.Errors[rejected.Name]
			if !ok {
				return nil, fmt.Errorf("潜质错误码缺失：%s", rejected.Name)
			}
			return []Push{push("Avatar", response, code, branch, index, level)}, nil
		}
		// 原生回调只使用ret_code刷新；后三项回传原请求，新等级由card_mgr权威属性提供。
		return []Push{cardMgrPush(c), materialManagerPush(c), push("Avatar", response, RetSuccess, branch, index, level)}, nil
	}
	return nil, errors.New("技能潜质方法未实现")
}

// 本版重置界面显示当前等级减一；系统说明明确仅退回一点潜质额度，
// 不返还升级材料，也不消耗重置材料。已升级后置节点不能失去前置资格。
func resetTalentNodeNative(p *Progress, uuid string, branch, index, level int) error {
	_, card := findCard(p, uuid)
	if err := checkCardGrowth(card); err != nil {
		return err
	}
	if p.Battle != nil && !p.Battle.Finished && containsString(p.Battle.Team, uuid) {
		return runeReject("RET_FAILED", "战斗中的幻书不能重置潜质")
	}
	if err := validateCardSkillTalentState(*card); err != nil {
		return err
	}
	rule, ok := androidCardSkillTalent.Talents[card.CardID]
	if !ok || card.Level < rule.UnlockLevel || card.Grade < rule.UnlockLevel/10 {
		return runeReject("RET_FAILED", "潜质树尚未解锁")
	}
	candidate := *card
	candidate.TalentTree = nativeCardTalentTree(*card)
	nodes, ok := candidate.TalentTree[branch]
	if !ok || index < 0 || index >= len(nodes) {
		return runeReject("RET_FAILED", "潜质节点不属于当前幻书")
	}
	node := nodes[index]
	if level != node.Level || level <= 0 || node.State != 1 || androidCardSkillTalent.Nodes[node.ID].Forbid != 0 {
		return runeReject("RET_FAILED", "潜质节点等级已改变或不能重置")
	}
	nodes[index].Level--
	candidate.TalentTree[branch] = nodes
	for key, list := range candidate.TalentTree {
		for i := range list {
			list[i].State = 0
		}
		candidate.TalentTree[key] = list
	}
	candidate.TalentTree = nativeCardTalentTree(candidate)
	for _, list := range candidate.TalentTree {
		for _, n := range list {
			if n.Level > 0 && n.State != 1 {
				return runeReject("RET_FAILED", "请先重置依赖该节点的后置潜质")
			}
		}
	}
	card.TalentTree = candidate.TalentTree
	return nil
}
