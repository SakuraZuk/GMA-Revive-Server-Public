package game

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"sort"
	"strconv"
	"time"
)

// 玩家业务（P0-5）：卡牌拥有、new_task 进度、系统解锁与副本体力消费。
// 客户端结构取证：custom_types/card.py（2948C748，card 类 prop 全表：uuid/card_id/level/exp/
// grade/awakened/dress/lock/time/grow_materials/enhance_count/enhance_ids/skill_enhance_count/
// skill_mgr/embed_runes/talent_tree/habit_mgr/support_skill_level；card_mgr=ObjId→card 容器）、
// custom_types/new_task.py（9EFA3F3A：task_id/finished_targets/status/expired/time）。
// 卡 uuid 下发为 24 位 hex 字符串；客户端 ObjId.load 兼容 bytes2id/str2id 两形态，
// ExtType42 精确形态待实机校验（必要时切换 ObjectID 编码）。

// newCard 服务端建卡（card_mgr.new_card 的权威侧）：品阶按 cards_max_level 映射。
func newCard(cardID, level int, now time.Time) Card {
	return Card{
		UUID:              newCardUUID(),
		CardID:            cardID,
		Level:             level,
		Grade:             gradeByLevel(level),
		Time:              now.Unix(),
		SupportSkillLevel: 1,
		Dress:             androidCardAppearances[cardID].DefaultDress,
	}
}

// gradeByLevel 复刻 creator.compute_grade_by_level 默认 breakthroughed=0：
// 原生按品阶升序选择首个 level<=cards_max_level.max_level；等级1对应品阶0。
// 只用于新建卡，不倒推或覆盖已经保存的品阶。
func gradeByLevel(level int) int {
	grades := make([]int, 0, len(androidOath.Caps))
	for grade := range androidOath.Caps {
		grades = append(grades, grade)
	}
	sort.Ints(grades)
	for _, grade := range grades {
		if level <= androidOath.Caps[grade] {
			return grade
		}
	}
	return androidOath.MaxGrade
}

// newCardUUID 生成 12 字节 ObjectId 的 hex 形态（gworld.gen_object_id 等价）。
func newCardUUID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 退化为时间戳熵源；uuid 唯一性由 (avatar, uuid) 使用面保证。
		now := time.Now().UnixNano()
		for i := 0; i < 12; i++ {
			b[i] = byte(now >> (uint(i) * 8))
		}
	}
	return hex.EncodeToString(b[:])
}

// cardMgrProperties 组装 card_mgr 下发字典（CustomDict.load 输入）。
func cardMgrProperties(cards []Card) map[string]any {
	mgr := make(map[string]any, len(cards))
	for _, card := range cards {
		mgr[card.UUID] = map[string]any{
			"uuid":                card.UUID,
			"card_id":             card.CardID,
			"level":               card.Level,
			"exp":                 card.Exp,
			"grade":               card.Grade,
			"awakened":            card.Awakened,
			"dress":               effectiveCardDress(card),
			"lock":                card.Lock,
			"time":                card.Time,
			"enhance_count":       card.EnhanceCount,
			"enhance_ids":         nativeCardEnhanceIDs(card),
			"skill_enhance_count": card.SkillEnhanceCount,
			"support_skill_level": card.SupportSkillLevel,
			"grow_materials":      map[string]any{},
			"ring_id":             card.RingID,
			"is_update_level_one": card.IsUpdateLevelOne,
			"skill_mgr":           nativeCardSkills(card),
			"talent_tree":         nativeCardTalentTree(card),
		}
	}
	return mgr
}

func cardMgrPropertiesWithRunes(cards []Card, runes map[string]Rune) map[string]any {
	mgr := cardMgrProperties(cards)
	for _, entry := range mgr {
		entry.(map[string]any)["embed_runes"] = map[string]any{}
	}
	for _, rune := range runes {
		if rune.CardUUID == "" {
			continue
		}
		card, ok := mgr[rune.CardUUID].(map[string]any)
		if !ok {
			continue
		}
		embed, _ := card["embed_runes"].(map[string]any)
		if embed == nil {
			embed = map[string]any{}
		}
		embed[strconv.Itoa(rune.Position)] = runeProperties(rune)
		card["embed_runes"] = embed
	}
	return mgr
}

// cardCommonMgrProperties 组装 card_common_mgr 下发字典（CustomDict<Int, card_common>）。
// 键用字符串即可：客户端 CustomDict.__setitem__ 会经 _key_type(Int).convert 还原整型键。
// 必发原因：客户端 sound_mgr.init_vo_language_map 在 on_login_success 内遍历 card_mgr 后
// 读 card_common_mgr[card_id].card_vo；条目缺失直接 KeyError（实机 4401）中断登录收尾，
// 过场视频播完后永久卡死、主城不加载。结构取证：custom_types/card_common.py（card_vo 等字段）。
func cardCommonMgrProperties(cards []Card) map[string]any {
	mgr := make(map[string]any, len(cards))
	counts := make(map[int]int)
	for _, card := range cards {
		counts[card.CardID]++
	}
	for _, card := range cards {
		key := strconv.Itoa(card.CardID)
		if _, dup := mgr[key]; dup {
			continue
		}
		mgr[key] = map[string]any{
			"card_id":  card.CardID,
			"count":    counts[card.CardID],
			"level":    card.Level,
			"grade":    card.Grade,
			"awakened": card.Awakened,
			"card_vo":  0,
		}
	}
	return mgr
}

// newTaskProperties 组装 new_tasks 下发字典（new_task_dict 容器）。
func newTaskProperties(tasks map[int]NewTaskProgress) map[string]any {
	out := make(map[string]any, len(tasks))
	for id, task := range tasks {
		targets := map[string]any{}
		for targetID, count := range task.FinishedTargets {
			targets[strconv.Itoa(targetID)] = count
		}
		out[strconv.Itoa(id)] = map[string]any{
			"task_id":          task.TaskID,
			"finished_targets": targets,
			"status":           task.Status,
			"expired":          task.Expired,
			"time":             task.Time,
		}
	}
	return out
}

// advanceNewTask 推进 new_task 目标计数（target_id 命中 +1），达 target_need_count 置完成。
// 任务表 new_task.json：target_id=[[20102]] 形态（多目标联合）。
func (p *Progress) advanceNewTask(targetID int, now time.Time) bool {
	changed := false
	for taskID, task := range p.NewTasks {
		if task.Status != 1 || task.Expired {
			continue
		}
		def, ok := clientBaseline.NewTasks[taskID]
		if !ok {
			continue
		}
		need := 1
		matched := false
		for _, group := range def.TargetID {
			for _, tid := range group {
				if tid == targetID {
					matched = true
				}
			}
		}
		if !matched {
			continue
		}
		if len(def.TargetID) > 0 && len(def.TargetID[0]) > 0 {
			need = def.TargetNeedCount
		}
		if task.FinishedTargets == nil {
			task.FinishedTargets = map[int]int{}
		}
		if task.FinishedTargets[targetID] >= need {
			continue // 完成只冻结计数；status=2仅由真实领奖事务写入。
		}
		task.FinishedTargets[targetID]++
		task.Time = now.Unix()
		p.NewTasks[taskID] = task
		changed = true
		log.Printf("new_task 进度 task=%d target=%d count=%d status=%d", taskID, targetID, task.FinishedTargets[targetID], task.Status)
	}
	return changed
}

// currentPower 结算按恢复间隔推进后的当前体力（客户端 time_auto_attr 自治显示，
// 服务器扣费前必须按 last_time 结算，否则长时间在线后首扣读旧值）。
func (p *Progress) currentPower(now time.Time) int {
	value := p.Power.Value
	// 原生time_auto_attr.add(auto=false)保留奖励超上限余额；自动恢复不增加也不抹掉该余额。
	if p.Power.Max > 0 && value >= p.Power.Max {
		return value
	}
	if p.Power.Interval <= 0 || p.Power.LastTime <= 0 {
		return clampPower(value, p.Power.Max)
	}
	elapsed := now.Unix() - int64(p.Power.LastTime)
	if elapsed > 0 && p.Power.PerValue > 0 {
		value += int(elapsed/int64(p.Power.Interval)) * p.Power.PerValue
	}
	return clampPower(value, p.Power.Max)
}

func clampPower(v, max int) int {
	if v < 0 {
		return 0
	}
	if max > 0 && v > max {
		return max
	}
	return v
}

// advanceUnlocks 按表条件推进系统解锁并返回新解锁的 system 列表。
// system_unlock.unlock_condition：类型 1=通关副本（其余类型语义待取证，先跳过）。
func (p *Progress) advanceUnlocks(clearedDungeon int) []string {
	if p.UnlockSystems == nil {
		p.UnlockSystems = map[string]int{}
	}
	var unlocked []string
	for _, def := range clientBaseline.SystemUnlocks {
		if _, done := p.UnlockSystems[def.System]; done {
			continue
		}
		matched := false
		for _, group := range def.Conditions {
			for _, cond := range group {
				if len(cond) != 2 {
					continue
				}
				condType, okType := cond[0].(float64)
				condValue, okValue := cond[1].(float64)
				if okType && okValue && int(condType) == 1 && int(condValue) == clearedDungeon {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			continue
		}
		p.UnlockSystems[def.System] = def.Version
		unlocked = append(unlocked, def.System)
		log.Printf("系统解锁 %s v%d（通关 %d）", def.System, def.Version, clearedDungeon)
	}
	return unlocked
}

// consumePower 扣减副本体力（need_power>0 的普通副本）；不足返回 false。
func (p *Progress) consumePower(need int, now time.Time) bool {
	if need <= 0 {
		return true
	}
	current := p.currentPower(now)
	if current < need {
		return false
	}
	p.Power.Value = current - need
	p.Power.LastTime = float64(now.UnixNano()) / 1e9
	advanceBasicRewardEvent(p, 34, []any{1}, int64(need), now)
	return true
}
