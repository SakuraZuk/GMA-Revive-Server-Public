package game

import "errors"

// 本版sites仅714/417配置card_auto_enhance=[4401,2]；原生return_to_city
// 在整章主线完成后展示补完。按真实完成章累计最低应有阶段，不消耗幻书或材料。
// 沙之书原表禁止手动补完/分解；取第一个拥有实例与原生show_special_card一致。
// 用绝对最低阶段修复历史漏发，重复结算/冷登录不会重复增加，也不降低已有阶段。
func ensureMainlineAutoEnhance(p *Progress) error {
	expected := 0
	for _, site := range []int{714, 417} {
		row := remainingData("sites", site)
		values := row.ids("card_auto_enhance")
		if len(values) != 2 || values[0] != 4401 || values[1] != 2 {
			return errors.New("主线自动补完原生配置不一致")
		}
		if remainingChapterFinished(*p, row.integer("chapter_id")) {
			expected += values[1]
		}
	}
	if expected == 0 {
		return nil
	}
	for i := range p.Cards {
		card := &p.Cards[i]
		if card.CardID != 4401 {
			continue
		}
		rule := androidCardGrowth.Rarities[androidCardGrowth.Cards[card.CardID].Rarity]
		if card.EnhanceCount < 0 || card.EnhanceCount > len(rule.Sort) || expected > len(rule.Sort) {
			return errors.New("主线自动补完阶段无效")
		}
		if card.EnhanceCount >= expected {
			return nil
		}
		for _, stage := range rule.Sort[card.EnhanceCount:expected] {
			for _, id := range stage {
				if androidCardGrowth.Enhancements[id].Skill != 0 {
					card.SkillEnhanceCount++
				}
			}
		}
		card.EnhanceCount = expected
		return nil
	}
	return nil
}
