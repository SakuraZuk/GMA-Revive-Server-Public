package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"strconv"
	"time"
)

// 服务器权威教学战斗引擎（battle_id=10001）。
// 依据 out/dis 反汇编与客户端表取证：
//   - CF8BE798 controler/do_command：剥离 SKILL_USED_TYPES=('ck_monster','ck_skill','ck_head')
//     来源标记后，user_eid 必须是当前输入角色，技能必须属于该角色，目标必须存在。
//   - 伤害公式（2026-10-06 全链反汇编证实）：
//     effect_change_hp.calc_base_value/_calc_base_value(E882CCCF)：
//       base_value = float(使用者 atk) × effect_rate，
//       effect_rate = get_upgrade_value(skill_level, enhance_level, effect_rates[0])
//       = rate + level_rate×(level-1) + grade_2(≥2) + grade_3(≥3) + enhance_1(≥1) + enhance_2(≥2)
//       （教学 lv1/grade1/enhance0 时恒等于表值 rate，如 440302=0.64）；
//     defence.py(56AB450B) INIT_DAMAGE_RATE：rates['defence'] = 1000.0/(1000+max(防御-穿透,0))，
//     effect_rate_percent 是效果触发概率（effect_probability），不进入伤害数值；
//     base_hp_change_struct.get_expect_value(DE67DC42)：
//       expect = max(round_func(base_value × ∏rates), 1)，real = int(expect)；
//     common_const.round_func(37C6CA84)：|x%1-0.5|<0.01 时 int(x) 截断，否则 int(round(x))。
//   - battle_base.reset_enemy_attrs(2CC3F05D)：敌方 max_hp=round_func(hp×enemy_hp_factor)，
//     atk/defence 乘因子后不取整（battle_info 10001：hp=0.21、atk=1.0、defence=1.0）。
//   - dead_logic.try_kill_target(D9911F72)：hp>0 存活，即 hp<=0 判死。
//   - battle_utils.sync_method：服务端仅在 on_battle_start/real_round_end/real_start_next_round/
//     after_pre_play/real_continue_old_round/continue_wait_for_player_input 六个跳转点同步客户端，
//     演出由客户端本地确定性演算，服务端不下发 play 数据。
//   - instance_trigger 1000102：敌方首次行动结束后新增 camp1[107,107,106]（init_ap=900）与
//     camp2[202,202,202]；新增实体 eid 沿既有编号按 camp_id 升序连续分配（camp_mgr.set_up_eid_to_camp）。

const (
	skillUsedClickMonster = "ck_monster"
	skillUsedClickSkill   = "ck_skill"
	skillUsedClickHead    = "ck_head"
	actionAPConsume       = 1000.0
	guideBattleID         = 10001
	playerCamp            = 1
	enemyCamp             = 2
	moveSkillID           = 100 // common_const.MOVE_SKILL_ID
	moveActTimeMs         = 1000
	// defence.py get_defence_rate 的分子与分母常数：1000.0/(1000+defence)。
	defenceRateBase = 1000.0
)

// phase 常量沿用 BattleSession.Bootstrap 字段持久化；输入等待两态对 do_command 等价。
const (
	phaseEnterShow  = "入场演出"
	phaseOpeningEnd = "开场结束"
	phaseFirstInput = "等待首次输入"
	phaseActionPlay = "行动演出"
	phaseWaitInput  = "等待输入"
	phaseBattleOver = "已结束"
)

func waitingForInput(phase string) bool {
	return phase == phaseFirstInput || phase == phaseWaitInput
}

// ensureEntities 为旧格式会话（引擎上线前已处于等待输入）补建权威实体；
// 实体是服务器私有状态，补建对客户端不可见。触发器态一并初始化（battle_start 链等效 set_ap）。
func ensureEntities(b *BattleSession, now func() time.Time) error {
	if len(b.Entities) > 0 || !waitingForInput(b.Bootstrap) {
		return nil
	}
	entities, err := buildGuideBattleEntities(b.BattleID)
	if err != nil {
		return err
	}
	b.Entities = entities
	if err := b.armStartTriggers(); err != nil {
		return err
	}
	b.LastClock = now().UnixMilli()
	log.Printf("教学战斗旧会话补建权威实体 n=%d", len(b.Entities))
	return nil
}

type damageEffect struct {
	Rate    float64 `json:"rate"`
	Percent float64 `json:"percent"`
}

type skillProfile struct {
	ID        int            `json:"id"`
	ActTimeMs int64          `json:"act_time_ms"`
	Effects   []damageEffect `json:"effects"`
}

type battleEntity struct {
	EID     string         `json:"eid"`
	RoleID  int            `json:"role_id"`
	Camp    int            `json:"camp"`
	Master  bool           `json:"master"`
	HP      int            `json:"hp"`
	MaxHP   int            `json:"max_hp"`
	ATK     float64        `json:"atk"`
	Defence float64        `json:"defence"`
	APSpeed float64        `json:"ap_speed"`
	AP      float64        `json:"ap"`
	Alive   bool           `json:"alive"`
	Skills  []skillProfile `json:"skills"`
}

// roundFunc 复刻 common_const.round_func：小数部分落在 0.5±0.01 内时 int() 截断，
// 否则四舍五入。敌方 202 的 max_hp=round_func(150×0.21)=int(31.5)=31 依赖该边界分支。
func roundFunc(x float64) int {
	frac := math.Mod(x, 1)
	if math.Abs(frac-0.5) < 0.01 {
		return int(x)
	}
	return int(math.Round(x))
}

// buildGuideBattleEntities 依据 battle_info 10001、role_info 与技能效果基线生成权威实体。
// eid 分配与 camp_mgr 一致：camp_id 升序连续编号，camp1 首槽为 "1"。
func buildGuideBattleEntities(battleID int) ([]battleEntity, error) {
	battle, ok := clientBaseline.Battles[battleID]
	if !ok || battle.ID == 0 {
		return nil, fmt.Errorf("战斗 %d 不在客户端基线", battleID)
	}
	factors := clientBaseline.BattleFactors[battleID]
	factor := func(v *float64) float64 {
		if v == nil {
			return 1
		}
		return *v
	}
	next := 1
	build := func(roleID int, camp int, master bool) (battleEntity, error) {
		base, ok := clientBaseline.BattleRoles[roleID]
		if !ok {
			return battleEntity{}, fmt.Errorf("角色 %d 不在客户端基线", roleID)
		}
		profiles, ok := clientBaseline.SkillProfiles[roleID]
		if !ok || len(profiles) == 0 {
			return battleEntity{}, fmt.Errorf("角色 %d 缺少技能效果基线", roleID)
		}
		entity := battleEntity{
			EID: strconv.Itoa(next), RoleID: roleID, Camp: camp, Master: master,
			MaxHP: base.HP, ATK: float64(base.ATK), Defence: float64(base.Defence),
			APSpeed: base.APSpeed, Alive: true,
		}
		if camp == enemyCamp {
			// reset_enemy_attrs：battle_info enemy_*_factor 只作用于敌方；
			// max_hp 取整用 round_func，atk/defence 乘因子后不取整。
			entity.MaxHP = roundFunc(float64(base.HP) * factor(factors.HP))
			entity.ATK = float64(base.ATK) * factor(factors.ATK)
			entity.Defence = float64(base.Defence) * factor(factors.Defence)
		}
		entity.HP = entity.MaxHP
		for i, profile := range profiles {
			if i >= len(base.Skills) {
				break
			}
			effects := make([]damageEffect, 0, len(profile.Effects))
			for _, effect := range profile.Effects {
				effects = append(effects, damageEffect{Rate: effect.Rate, Percent: effect.Percent})
			}
			entity.Skills = append(entity.Skills, skillProfile{ID: base.Skills[i], ActTimeMs: int64(profile.ActTime * 1000), Effects: effects})
		}
		// set_up_base_battle_skill 为每个实体注入基础移动技能（common_const.MOVE_SKILL_ID=100）。
		entity.Skills = append(entity.Skills, skillProfile{ID: moveSkillID, ActTimeMs: moveActTimeMs})
		next++
		return entity, nil
	}
	var entities []battleEntity
	for _, roleID := range battle.Avatars {
		entity, err := build(roleID, playerCamp, true)
		if err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}
	for _, roleID := range battle.Enemies {
		entity, err := build(roleID, enemyCamp, false)
		if err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}
	if len(entities) == 0 {
		return nil, errors.New("战斗没有可用角色")
	}
	return entities, nil
}

func (e *battleEntity) profile(skillID int) *skillProfile {
	for i := range e.Skills {
		if e.Skills[i].ID == skillID {
			return &e.Skills[i]
		}
	}
	return nil
}

func (e *battleEntity) attackProfile() *skillProfile {
	if len(e.Skills) == 0 {
		return nil
	}
	return &e.Skills[0]
}

// rollDamage 按 effect_change_hp/defence/get_expect_value 公式链结算：
//  1. effect_rate_percent 是效果触发概率（effect_probability），按 percent 加权互斥抽取效果行，
//     抽中的 rate 即 get_upgrade_value 结果（教学 lv1/grade1/enhance0 无成长增量）；
//  2. defence.init_damage_rate：乘 rates['defence'] = 1000.0/(1000+max(目标防御-穿透,0))；
//  3. get_expect_value：expect = max(round_func(atk × rate × defence_rate), 1)，real = int(expect)。
func rollDamage(entity, target *battleEntity, profile *skillProfile, rng *rand.Rand) int {
	total := 0.0
	for _, effect := range profile.Effects {
		total += effect.Percent
	}
	rate := profile.Effects[len(profile.Effects)-1].Rate
	if total > 0 {
		pick := rng.Float64() * total
		for _, effect := range profile.Effects {
			pick -= effect.Percent
			if pick > 0 {
				continue
			}
			rate = effect.Rate
			break
		}
	}
	defence := target.Defence
	if defence < 0 {
		defence = 0
	}
	defenceRate := defenceRateBase / (defenceRateBase + defence)
	value := entity.ATK * rate * defenceRate
	damage := roundFunc(value)
	if damage < 1 {
		return 1
	}
	return damage
}

func actionRNG(seed int64, counter int64) *rand.Rand {
	return rand.New(rand.NewSource(seed*1000003 + counter))
}

func (b *BattleSession) entity(eid string) *battleEntity {
	for i := range b.Entities {
		if b.Entities[i].EID == eid {
			return &b.Entities[i]
		}
	}
	return nil
}

// campWiped 判定某阵营是否全灭。
func (b *BattleSession) campWiped(camp int) bool {
	return b.countAlive(camp) == 0
}

func entityEIDLess(a, e string) bool {
	ai, _ := strconv.Atoi(a)
	ei, _ := strconv.Atoi(e)
	return ai < ei
}

// nextActor 依据 AP 行动模型挑选下一个行动者：AP 最高者先行动，同值先比 ap_speed 再比 eid。
// 这是 driver.calc_next_action_time 排序键 (next_action_time, -ap, random_speed_rate, eid) 的等效实现。
func (b *BattleSession) nextActor() *battleEntity {
	var best *battleEntity
	for i := range b.Entities {
		candidate := &b.Entities[i]
		if !candidate.Alive {
			continue
		}
		if best == nil || candidate.AP > best.AP ||
			(candidate.AP == best.AP && candidate.APSpeed > best.APSpeed) ||
			(candidate.AP == best.AP && candidate.APSpeed == best.APSpeed && entityEIDLess(candidate.EID, best.EID)) {
			best = candidate
		}
	}
	return best
}

// spawnEntity 按 instance_event 增援单个实体；eid 沿既有编号顺序分配。
// eid 顺序证据（20:19:58 实机 addentity）：事件按 trigger_events 升序处理，
// 10001021 camp2 [202,202,202] → '5','6','7'，10001023 camp1 [107,107,106] → '8','9','10'。
func (b *BattleSession) spawnEntity(camp int, roleID int, initAP *float64) error {
	base, ok := clientBaseline.BattleRoles[roleID]
	if !ok {
		return fmt.Errorf("波次角色 %d 不在客户端基线", roleID)
	}
	profiles, ok := clientBaseline.SkillProfiles[roleID]
	if !ok || len(profiles) == 0 {
		return fmt.Errorf("波次角色 %d 缺少技能效果基线", roleID)
	}
	factors := clientBaseline.BattleFactors[b.BattleID]
	factor := func(v *float64) float64 {
		if v == nil {
			return 1
		}
		return *v
	}
	ap := 0.0
	if initAP != nil {
		ap = *initAP
	}
	entity := battleEntity{EID: strconv.Itoa(len(b.Entities) + 1), RoleID: roleID, Camp: camp,
		MaxHP: base.HP, ATK: float64(base.ATK), Defence: float64(base.Defence),
		APSpeed: base.APSpeed, AP: ap, Alive: true}
	if camp == enemyCamp {
		// 波次敌方与初盘敌方同样过 reset_enemy_attrs（此前漏乘因子导致 202 波次 150 血）。
		entity.MaxHP = roundFunc(float64(base.HP) * factor(factors.HP))
		entity.ATK = float64(base.ATK) * factor(factors.ATK)
		entity.Defence = float64(base.Defence) * factor(factors.Defence)
	}
	entity.HP = entity.MaxHP
	for i, profile := range profiles {
		if i >= len(base.Skills) {
			break
		}
		effects := make([]damageEffect, 0, len(profile.Effects))
		for _, effect := range profile.Effects {
			effects = append(effects, damageEffect{Rate: effect.Rate, Percent: effect.Percent})
		}
		entity.Skills = append(entity.Skills, skillProfile{ID: base.Skills[i], ActTimeMs: int64(profile.ActTime * 1000), Effects: effects})
	}
	b.Entities = append(b.Entities, entity)
	return nil
}

// aiAttack 敌方/增援 AI 行动。目标证据（20:19:57-20:20:02 实机 shadow_battle 日志）：
// 敌方 201/202 的四次行动全部攻击增援盟友 '8','9','10'，未攻击主角——
// 增援生成位贴近敌方（add_entity_pos 9,0 与 slay_pos 7,0 同侧），主角远在另一侧。
// 近似规则：优先攻击对方阵营主角以外的存活实体（eid 最小），仅剩主角时才攻击主角。
func (s *Service) aiAttack(b *BattleSession, actor *battleEntity) bool {
	profile := actor.attackProfile()
	if profile == nil {
		return false
	}
	var target *battleEntity
	for i := range b.Entities {
		candidate := &b.Entities[i]
		if !candidate.Alive || candidate.Camp == actor.Camp || candidate.Master {
			continue
		}
		if target == nil || entityEIDLess(candidate.EID, target.EID) {
			target = candidate
		}
	}
	if target == nil {
		for i := range b.Entities {
			candidate := &b.Entities[i]
			if candidate.Alive && candidate.Camp != actor.Camp {
				target = candidate
				break
			}
		}
	}
	if target == nil {
		return false
	}
	damage := rollDamage(actor, target, profile, actionRNG(b.Seed, b.ActionCounter))
	target.HP -= damage
	if target.HP <= 0 {
		target.HP = 0
		target.Alive = false
	}
	actor.AP -= actionAPConsume
	b.ActionCounter++
	b.ActionEID = actor.EID
	b.AttackDamage = damage
	b.AttackTarget = target.EID
	b.Bootstrap = phaseActionPlay
	b.DueAt = s.Now().UnixMilli() + profile.ActTimeMs
	log.Printf("教学战斗AI行动 actor=%s role=%d target=%s damage=%d targetHP=%d", actor.EID, actor.RoleID, target.EID, damage, target.HP)
	return true
}

// finishBattle 结束战斗并下发结算。battleEnd 非空时为事件链判定
// （instance_event.battle_end= victory/fail，如 10001 的 T1000105：107 第二次行动结束）；
// 为空时按阵营全灭兜底（玩家阵营存活即胜）。
func finishBattle(b *BattleSession, dungeonID int, battleEnd string) []Push {
	win := !b.campWiped(playerCamp)
	switch battleEnd {
	case "victory":
		win = true
	case "fail":
		win = false
	}
	b.Bootstrap = phaseBattleOver
	b.DueAt = 0
	b.Status = "结束"
	log.Printf("教学战斗结束 win=%v 判定=%q", win, battleEnd)
	return []Push{push("Avatar", "battle_result", win, []any{}, []any{}, map[string]any{"dungeon_id": dungeonID})}
}

// doCommand 实现 CF8BE798.do_command 的分流：客户端权威模式下
// __battle_event__ 走观察链，move_to/use_skill 转发客户端原生执行器；
// 旧服务器权威技能链（doMoveTo/use_skill）保留作回滚参考，不再进入。
func (s *Service) doCommand(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("战斗指令需要玩家状态及命令名、参数表")
	}
	var command string
	if err := json.Unmarshal(args[0], &command); err != nil {
		return nil, errors.New("战斗指令名无效")
	}
	var rawArgs []any
	if err := json.Unmarshal(args[1], &rawArgs); err != nil {
		return nil, errors.New("战斗指令参数必须是列表")
	}
	if command == battleEventCommand {
		return s.absorbBattleEvent(ctx, c, rawArgs)
	}
	if handled, pushes, err := s.humanDoCommand(ctx, c, command, rawArgs); handled {
		return pushes, err
	}
	if handled, pushes, err := s.nativeSoloDoCommand(ctx, c, command, rawArgs); handled {
		return pushes, err
	}
	if command != "move_to" && command != "use_skill" {
		// 其余指令（use_support_skill 等）由客户端本地执行；记录后不转发。
		log.Printf("战斗指令本地处理 command=%s", command)
		return nil, nil
	}
	var started, finished bool
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum && av.Progress.Battle != nil {
			started = av.Progress.Battle.BridgeStarted
			finished = av.Progress.Battle.Finished
		}
	}
	if !started || finished {
		log.Printf("战斗指令丢弃（未开战或已结束）command=%s", command)
		return nil, nil
	}
	// 目标解析、技能归属与移动校验由客户端原生 1.0.8 校验器在实体上执行；
	// 服务端不再用表推导 eid/AP/坐标当权威（参考项目同款结论）。
	return []Push{battleSync("revival_do_command", command, rawArgs)}, nil
}

// doCommandAuthoritative 是旧服务器权威技能执行链（doMoveTo/use_skill），
// 客户端权威改造后不再被调用；保留以支持架构回滚与数值对照。
func (s *Service) doCommandAuthoritative(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("战斗指令需要玩家状态及命令名、参数表")
	}
	var command string
	if err := json.Unmarshal(args[0], &command); err != nil {
		return nil, errors.New("战斗指令名无效")
	}
	var rawArgs []any
	if err := json.Unmarshal(args[1], &rawArgs); err != nil {
		return nil, errors.New("战斗指令参数必须是列表")
	}
	if command == "move_to" {
		return s.doMoveTo(ctx, c, rawArgs)
	}
	if command != "use_skill" {
		// 其余指令（use_support_skill 等）教学战斗未涉及；记录后丢弃。
		log.Printf("战斗指令暂不支持 command=%s", command)
		return nil, nil
	}
	// SKILL_USED_TYPES 是客户端点击来源标记，不属于技能实参（common_const 8099）。
	if len(rawArgs) > 0 {
		if marker, ok := rawArgs[0].(string); ok && (marker == skillUsedClickMonster || marker == skillUsedClickSkill || marker == skillUsedClickHead) {
			rawArgs = rawArgs[1:]
		}
	}
	if len(rawArgs) < 3 {
		return nil, errors.New("use_skill 参数不足")
	}
	userEID := eidString(rawArgs[0])
	skillID := int(toFloat(rawArgs[1]))
	targetInfo := eidString(rawArgs[2])
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		b := p.Battle
		if b == nil || b.BattleID != guideBattleID {
			return errors.New("没有进行中的教学战斗")
		}
		if b.Bootstrap == phaseBattleOver {
			return errors.New("战斗已经结束")
		}
		if err := ensureEntities(b, s.Now); err != nil {
			return err
		}
		if !waitingForInput(b.Bootstrap) {
			return errors.New("当前不等待玩家输入")
		}
		user := b.entity(userEID)
		if user == nil || !user.Alive || !user.Master {
			return fmt.Errorf("command no user:%s", userEID)
		}
		profile := user.profile(skillID)
		if profile == nil {
			return fmt.Errorf("command user:%s do not have skill:%d", userEID, skillID)
		}
		target := b.entity(targetInfo)
		if target == nil {
			return fmt.Errorf("command target is None, target_eid:%s", targetInfo)
		}
		if !target.Alive || target.Camp == user.Camp {
			// 客户端本地演算与服务端数值存在残余分歧：客户端仍视为存活的目标在服务端可能已死亡。
			// 拒绝会让客户端本地已执行的行动永久等待回合同步而冻结输入（21:05 实机冻结）；
			// 按空挥处理：消耗行动并推进回合，保证引导永不卡死。
			user.AP -= actionAPConsume
			b.ActionCounter++
			b.Bootstrap = phaseActionPlay
			b.ActionEID = user.EID
			b.DueAt = s.Now().UnixMilli() + profile.ActTimeMs
			b.AttackDamage = 0
			b.AttackTarget = target.EID
			log.Printf("教学战斗空挥 user=%s skill=%d target=%s 目标服务端已死亡或同阵营", userEID, skillID, targetInfo)
			return nil
		}
		// 权威结算：伤害、死亡、AP 消耗一次事务完成。
		damage := rollDamage(user, target, profile, actionRNG(b.Seed, b.ActionCounter))
		target.HP -= damage
		if target.HP <= 0 {
			target.HP = 0
			target.Alive = false
		}
		user.AP -= actionAPConsume
		b.ActionCounter++
		b.Bootstrap = phaseActionPlay
		b.ActionEID = user.EID
		b.DueAt = s.Now().UnixMilli() + profile.ActTimeMs
		b.AttackDamage = damage
		b.AttackTarget = target.EID
		log.Printf("教学战斗玩家指令 user=%s skill=%d target=%s damage=%d targetHP=%d", userEID, skillID, targetInfo, damage, target.HP)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func toFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return 0
}

// eidString 容忍 eid 的字符串与数字两种上行形态（实机为字符串 "1"/"2"）。
func eidString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.Itoa(int(v))
	}
	return ""
}

// doMoveTo 落实 CF8BE798 的 move_to 分支：必须是当前输入角色、携带移动技能。
// 服务端无六边形地图数据，无法复算 can_move_to_hex，坐标合法性由客户端本地演算；
// 服务器权威部分是行动消耗与回合推进——否则客户端本地已提交的移动会永久等待，
// 输入锁不再释放（2026-10-05 21:05 实机冻结的直接原因）。
func (s *Service) doMoveTo(ctx context.Context, c *Connection, rawArgs []any) ([]Push, error) {
	if len(rawArgs) < 2 {
		return nil, errors.New("move_to 参数不足")
	}
	userEID := eidString(rawArgs[0])
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		b := p.Battle
		if b == nil || b.BattleID != guideBattleID {
			return errors.New("没有进行中的教学战斗")
		}
		if b.Bootstrap == phaseBattleOver {
			return errors.New("战斗已经结束")
		}
		if !waitingForInput(b.Bootstrap) {
			return errors.New("当前不等待玩家输入")
		}
		if err := ensureEntities(b, s.Now); err != nil {
			return err
		}
		user := b.entity(userEID)
		if user == nil || !user.Alive || !user.Master {
			return fmt.Errorf("command no user:%s", userEID)
		}
		if user.profile(moveSkillID) == nil {
			return fmt.Errorf("command user:%s do not have skill:%d", userEID, moveSkillID)
		}
		user.AP -= actionAPConsume
		b.ActionCounter++
		b.Bootstrap = phaseActionPlay
		b.ActionEID = user.EID
		b.DueAt = s.Now().UnixMilli() + moveActTimeMs
		b.AttackDamage, b.AttackTarget = 0, ""
		log.Printf("教学战斗玩家移动 user=%s coord=%v", userEID, rawArgs[1])
		return nil
	})
	return nil, err
}

// accrueAP 按真实流逝秒数累计行动点；单次上限 10 秒防止停服跳变。
// AP 增量按 combat_info.pass_by_time 的 0.1 精度量化：round_func(ap_speed×delta×10)/10。
func accrueAP(entities []battleEntity, last, now int64) {
	if last == 0 || now <= last {
		return
	}
	seconds := float64(now-last) / 1000
	if seconds > 10 {
		seconds = 10
	}
	for i := range entities {
		if !entities[i].Alive {
			continue
		}
		apChange := float64(roundFunc(entities[i].APSpeed*seconds*10)) / 10
		entities[i].AP += apChange
	}
}

// tickGuideBattle 推进行动演出到期后的回合跳转；一次 Tick 至多推进一个行动者。
// 触发器时序：行动结束先评 turn_end（增援/注册），再评 entity_reduce（第二波/判负），
// 然后按 nextActor 评 turn_start（注册），全灭兜底判胜保留（事件链 battle_end 优先）。
func (s *Service) tickGuideBattle(b *BattleSession, now int64) ([]Push, error) {
	if err := ensureEntities(b, s.Now); err != nil {
		return nil, err
	}
	accrueAP(b.Entities, b.LastClock, now)
	b.LastClock = now
	if b.Bootstrap != phaseActionPlay || b.DueAt == 0 || now < b.DueAt {
		return nil, nil
	}
	actorEID := b.ActionEID
	effects := []Push{battleSync("real_round_end", actorEID)}
	actor := b.entity(actorEID)
	if end, err := b.onActionEnd(actor); err != nil {
		return nil, err
	} else if end != "" {
		return append(effects, finishBattle(b, b.DungeonID, end)...), nil
	}
	if end, err := b.onEntityReduce(); err != nil {
		return nil, err
	} else if end != "" {
		return append(effects, finishBattle(b, b.DungeonID, end)...), nil
	}
	if b.campWiped(playerCamp) || b.campWiped(enemyCamp) {
		return append(effects, finishBattle(b, b.DungeonID, "")...), nil
	}
	next := b.nextActor()
	if next == nil {
		return append(effects, finishBattle(b, b.DungeonID, "")...), nil
	}
	if end, err := b.onActionBegin(next); err != nil {
		return nil, err
	} else if end != "" {
		return append(effects, finishBattle(b, b.DungeonID, end)...), nil
	}
	effects = append(effects, battleSync("real_start_next_round", 0))
	if next.Master {
		effects = append(effects, battleSync("after_pre_play", next.EID, 0, 0))
		effects = append(effects, battleSync("continue_wait_for_player_input"))
		b.Bootstrap = phaseWaitInput
		b.DueAt = 0
		b.ActionEID = next.EID
		return effects, nil
	}
	effects = append(effects, battleSync("after_pre_play", next.EID, 0, 0))
	if !s.aiAttack(b, next) {
		return append(effects, finishBattle(b, b.DungeonID, "")...), nil
	}
	return effects, nil
}
