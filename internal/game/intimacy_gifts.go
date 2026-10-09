package game

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/big"
	"sort"
	"strconv"
	"time"
)

// parseIntimacyGifts 保留重复键证据；不能用普通map解码默默覆盖重复材料。
func parseIntimacyGifts(raw json.RawMessage) (map[int]int64, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("送礼材料必须为字典")
	}
	gifts := map[int]int64{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("送礼材料编号无效")
		}
		id, err := strconv.Atoi(key)
		if err != nil || id <= 0 || strconv.Itoa(id) != key {
			return nil, errors.New("送礼材料编号必须为规范整数")
		}
		if _, exists := gifts[id]; exists {
			return nil, errors.New("送礼材料编号重复")
		}
		var count int64
		if d.Decode(&count) != nil || count <= 0 {
			return nil, errors.New("送礼数量必须为正整数")
		}
		gifts[id] = count
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') {
		return nil, errors.New("送礼字典不完整")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, errors.New("送礼字典含额外数据")
	}
	if len(gifts) == 0 {
		return nil, errors.New("送礼材料为空")
	}
	return gifts, nil
}

// consumeIntimacyGifts 必须置于玩家行锁事务；数量限制由资产余额与契印容量决定。
func consumeIntimacyGifts(p *Progress, cardID int, gifts map[int]int64, rules intimacyCatalog, now time.Time) (map[string]Rune, error) {
	card, exists := rules.Cards[cardID]
	if !exists || !ownsCardID(*p, cardID) {
		return nil, intimacyReject("RET_CARD_NOT_EXIST", "幻书不存在")
	}
	if len(gifts) == 0 {
		return nil, errors.New("送礼材料为空")
	}
	value := p.Intimacy[cardID]
	if value < 0 || value > rules.MaxIntimacy || rules.MaxIntimacy <= 0 {
		return nil, errors.New("好感度存档或上限配置无效")
	}
	tables, err := loadRuneTables()
	if err != nil {
		return nil, err
	}
	remaining := tables.Tables.Limits.Max - len(p.Runes)
	// Android DAE1C2E7 girl_gift 缓存赠送前的 like_gift：
	// favor_gift 按 gift.like_tag==cards.tag[1][0] 判断，成功后发现偏好。
	// 整笔送礼仍按旧标记计算，不让多种礼物的遍历顺序改变收益。
	likedBefore := p.IntimacyCommons[cardID].LikeGift
	discoveredPreference := false
	ids := make([]int, 0, len(gifts))
	for id, count := range gifts {
		gift, exists := rules.Gifts[id]
		if !exists {
			return nil, intimacyReject("RET_MATERIAL_WRONG", "所选材料不是好感度礼物")
		}
		if count <= 0 {
			return nil, errors.New("送礼数量无效")
		}
		if p.Materials[id].Count < count {
			return nil, intimacyReject("RET_MATERIAL_NOT_ENOUGH", "送礼材料不足")
		}
		if n := len(gift.ReturnRunes); n != 0 {
			if remaining < 0 || count > int64(remaining/n) {
				return nil, intimacyReject("RET_RUNE_MAX_COUNT_EXCEED", "回礼契印超过背包容量")
			}
			remaining -= int(count) * n
		}
		ids = append(ids, id)
	}
	sort.Ints(ids)
	granted := map[string]Rune{}
	for _, id := range ids {
		gift, count := rules.Gifts[id], gifts[id]
		unit := gift.Intimacy
		if card.FavoredTag != nil && gift.LikeTag != nil && *card.FavoredTag == *gift.LikeTag {
			discoveredPreference = true
			if likedBefore {
				unit = gift.FavoredIntimacy
			}
		}
		if unit < 0 {
			return nil, errors.New("送礼好感度配置无效")
		}
		gap := rules.MaxIntimacy - value
		if unit > 0 && gap > 0 {
			// 先和剩余上限比较，不计算可能溢出的unit*count。
			if count > int64(gap/unit) {
				value = rules.MaxIntimacy
			} else {
				value += unit * int(count)
			}
		}
		for i := int64(0); i < count && len(gift.ReturnRunes) != 0; i++ {
			for _, reward := range gift.ReturnRunes {
				r, err := GrantRuneWithMarks(p, reward.Spec, reward.Marks, now)
				if err != nil {
					return nil, err
				}
				granted[r.UUID] = r
			}
		}
		m := p.Materials[id]
		m.Count -= count // Total是累计获得，不因送礼减少。
		p.Materials[id] = m
	}
	if p.Intimacy == nil {
		p.Intimacy = map[int]int{}
	}
	p.Intimacy[cardID] = value
	for _, count := range gifts {
		advanceAchievementAmount(p, 28, 1, count, now)
	}
	// type38 指收到指定幻书回礼；原生 return_bonus_id 为空的特殊选择不构成回礼。
	// 只在实际返还契印全部生成、材料扣除成功之后记录；同笔回礼盒记一次。
	if len(granted) > 0 {
		advanceAchievementEvent(p, 38, cardID, now)
	}
	// 原生N卡/隐藏援护等级自动提升，不要求人工领取章节。
	if autoIntimacyClaim(cardID, rules) {
		support := rules.Levels[intimacyLevel(value, rules)].Support
		for i := range p.Cards {
			if p.Cards[i].CardID == cardID && p.Cards[i].SupportSkillLevel < support {
				p.Cards[i].SupportSkillLevel = support
			}
		}
	}
	// 材料扣除、回礼生成全部成功后才解锁；外层玩家事务一并持久化。
	if discoveredPreference && !likedBefore {
		if p.IntimacyCommons == nil {
			p.IntimacyCommons = map[int]IntimacyCommon{}
		}
		common := p.IntimacyCommons[cardID]
		common.LikeGift = true
		p.IntimacyCommons[cardID] = common
	}
	return granted, nil
}

func (s *Service) intimacyGiftRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	want := 3
	if method == "consume_intimacy_gift" {
		want = 5
	}
	if c.phase != Playing || len(args) != want {
		return nil, errors.New("送礼RPC参数无效")
	}
	var callback, cardID int
	if json.Unmarshal(args[0], &callback) != nil || json.Unmarshal(args[1], &cardID) != nil || cardID <= 0 {
		return nil, errors.New("送礼回调或幻书编号无效")
	}
	var gifts map[int]int64
	var err error
	specialChoice := 0
	if method == "consume_intimacy_gift" {
		var id, choice int
		var count int64
		if json.Unmarshal(args[2], &id) != nil || id <= 0 || json.Unmarshal(args[3], &count) != nil || count <= 0 || string(args[4]) == "null" || json.Unmarshal(args[4], &choice) != nil {
			return nil, errors.New("单份送礼参数无效")
		}
		specialChoice = choice
		gifts = map[int]int64{id: count}
	} else {
		gifts, err = parseIntimacyGifts(args[2])
		if err != nil {
			return nil, err
		}
	}
	rules, err := loadIntimacyCatalog()
	if err != nil {
		return nil, err
	}
	var granted map[string]Rune
	err = s.updateProgress(ctx, c, func(p *Progress) error {
		var giftErr error
		if specialChoice != 0 {
			granted, giftErr = consumeSpecialIntimacyGift(p, cardID, gifts, specialChoice, rules, s.Now())
		} else {
			granted, giftErr = consumeIntimacyGifts(p, cardID, gifts, rules, s.Now())
		}
		if giftErr != nil {
			return giftErr
		}
		recordMikuGift(p, cardID)
		return nil
	})
	box := map[string]any{"__custom_type": "box.box", "materials": map[string]any{}, "runes": runeMgrProperties(granted)}
	if err != nil {
		var reject *intimacyBusinessError
		if errors.As(err, &reject) {
			code, exists := rules.Errors[reject.name]
			if !exists {
				return nil, fmt.Errorf("送礼错误码缺失：%s", reject.name)
			}
			box["runes"] = map[string]any{}
			return []Push{Callback(callback, []any{code, box})}, nil
		}
		return nil, err
	}
	p := c.SelectedAvatarUnsafe().Progress
	pushes := []Push{push("Avatar", "client_prop_changed", []any{"card_common_mgr", cardCommonMgrPropertiesWithProgress(p)}),
		cardMgrPush(c), materialManagerPush(c), runePush(c)}
	if specialChoice != 0 {
		pushes = append(pushes, push("Avatar", "client_prop_changed", []any{"special_gift_card_2_count", specialGiftCounts(p, s.Now())}))
	}
	return append(pushes, Callback(callback, []any{0, box})), nil
}

// consumeSpecialIntimacyGift 按 Android special_gift_rule 处理单份特殊礼物。
// 先确定选择倍率，再由普通送礼逻辑完成材料/回礼事务；
// 事务失败时由上层 JSONB 行锁回滚，特殊次数不会提前消耗。
// 原生 material_mgr / girl_gift 使用 special_gift_card_2_count 计算当日次数；
// common.SpecialCount 仅用于累计对话序号。回礼契印透传给 box.runes。
func consumeSpecialIntimacyGift(p *Progress, cardID int, gifts map[int]int64, choice int, rules intimacyCatalog, now time.Time) (map[string]Rune, error) {
	if len(gifts) != 1 {
		return nil, errors.New("特殊礼物只能选择一份材料")
	}
	var giftID int
	var count int64
	for id, n := range gifts {
		giftID, count = id, n
	}
	gift, ok := rules.Gifts[giftID]
	rule, ruleOK := rules.SpecialGiftRules[1]
	if !ok || gift.ForbidSpecial != 0 || !ruleOK {
		return nil, intimacyReject("RET_MATERIAL_WRONG", "该礼物不支持特殊选择")
	}
	if count != 1 {
		return nil, errors.New("特殊礼物每次只能赠送一份")
	}
	multiplier, ok := rule.ChoiceParams[choice]
	if !ok || multiplier <= 0 {
		return nil, errors.New("特殊礼物选择无效")
	}
	common := p.IntimacyCommons[cardID]
	if !ownsCardID(*p, cardID) {
		return nil, intimacyReject("RET_CARD_NOT_EXIST", "幻书不存在")
	}
	if len(rules.SpecialPresents[captainDress(*p, cardID)].Chats) == 0 {
		return nil, intimacyReject("RET_FAILED", "当前幻书外观没有特殊送礼剧情")
	}
	if p.Intimacy[cardID] >= rules.MaxIntimacy {
		return nil, intimacyReject("RET_FAILED", "好感度已满，不能使用特殊送礼倍率")
	}
	day := now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
	if p.SpecialGiftDay != "" && day < p.SpecialGiftDay {
		return nil, errors.New("服务时钟早于特殊送礼存档日，暂不能送礼")
	}
	counts := specialGiftCounts(*p, now)
	if counts[cardID] >= rule.SingleCardTimes {
		return nil, intimacyReject("RET_CARD_GROUP_GIFT_MAX", "该幻书特殊礼物次数已用尽")
	}
	total := 0
	for _, n := range counts {
		if n < 0 || n > rule.TotalTimes {
			return nil, errors.New("当日特殊送礼次数存档无效")
		}
		total += n
	}
	if total >= rule.TotalTimes {
		return nil, intimacyReject("RET_CARD_GROUP_GIFT_MAX", "特殊礼物总次数已用尽")
	}
	amount, verified := gift.SpecialAmounts[choice]
	if common.LikeGift && rules.Cards[cardID].FavoredTag != nil && gift.LikeTag != nil && *rules.Cards[cardID].FavoredTag == *gift.LikeTag {
		amount, verified = gift.SpecialFavoredAmounts[choice]
	}
	if !verified && remainingPolicyEnabled() {
		base := gift.Intimacy
		if common.LikeGift && rules.Cards[cardID].FavoredTag != nil && gift.LikeTag != nil && *rules.Cards[cardID].FavoredTag == *gift.LikeTag {
			base = gift.FavoredIntimacy
		}
		var err error
		amount, err = specialIntimacyHalfUp(base, multiplier)
		if err != nil {
			return nil, err
		}
		verified = true
	}
	if !verified || amount < 0 {
		return nil, errors.New("特殊礼物倍率缺少精确整数配置")
	}
	// 在生成回礼和同步援护等级之前确定最终增量，避免低倍率先升错等级。
	adjusted := rules
	adjusted.Gifts = maps.Clone(rules.Gifts)
	gift.Intimacy, gift.FavoredIntimacy = amount, amount
	adjusted.Gifts[giftID] = gift
	granted, err := consumeIntimacyGifts(p, cardID, gifts, adjusted, now)
	if err != nil {
		return nil, err
	}
	// 普通赠礼已更新偏好发现状态，不能用计算倍率时的旧快照覆盖它。
	common = p.IntimacyCommons[cardID]
	common.SpecialCount++
	if p.IntimacyCommons == nil {
		p.IntimacyCommons = map[int]IntimacyCommon{}
	}
	p.IntimacyCommons[cardID] = common
	p.IntimacySpecialTotal++
	p.SpecialGiftDay = day
	p.SpecialGiftCounts = counts
	p.SpecialGiftCounts[cardID]++
	return granted, nil
}

// 经批准的本服策略：正收益最终乘积精确有理数四舍五入一次，不改变精确整数分支。
func specialIntimacyHalfUp(base int, multiplier float64) (int, error) {
	if base < 0 {
		return 0, errors.New("特殊送礼基础收益无效")
	}
	ratio, ok := new(big.Rat).SetString(strconv.FormatFloat(multiplier, 'f', -1, 64))
	if !ok || ratio.Sign() <= 0 {
		return 0, errors.New("特殊送礼倍率无效")
	}
	amount := new(big.Rat).Mul(new(big.Rat).SetInt64(int64(base)), ratio)
	amount.Add(amount, big.NewRat(1, 2))
	whole := new(big.Int).Quo(amount.Num(), amount.Denom())
	if !whole.IsInt64() || whole.Int64() > int64(^uint(0)>>1) {
		return 0, errors.New("特殊送礼收益溢出")
	}
	return int(whole.Int64()), nil
}

// 日界为本地服务策略 UTC+8 零点；累计对话进度不随日期清空。
func specialGiftCounts(p Progress, now time.Time) map[int]int {
	day := now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
	if p.SpecialGiftDay == "" || day > p.SpecialGiftDay {
		return map[int]int{}
	}
	counts := maps.Clone(p.SpecialGiftCounts)
	if counts == nil {
		counts = map[int]int{}
	}
	return counts
}
