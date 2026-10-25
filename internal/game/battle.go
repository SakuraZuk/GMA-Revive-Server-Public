package game

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
)

// ObjectID 在传输层编码为客户端 ExtType 42，避免嵌套玩家 ID 被当成文本。
type ObjectID string

// 原生37C6CA84：NORMAL_BATTLE=1。副本活动类型与战斗模式是不同枚举；
// 探索活动类型2若写入模式，会触发5FBB9C5E的同步PVP跳过布阵分支。
const nativeNormalBattleType = 1

func ordinaryBattleTransportType(activityType int) int {
	if activityType == 2 {
		return nativeNormalBattleType
	}
	// 特殊活动的原生模式另有专用检查；本批只修复已取证的探索碰撞。
	return activityType
}

type BattleSession struct {
	UUID      string `json:"uuid"`
	DungeonID int    `json:"dungeon_id"`
	BattleID  int    `json:"battle_id"`
	Seed      int64  `json:"seed"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
	Loaded    bool   `json:"loaded"`
	Started   bool   `json:"started"`
	Bootstrap string `json:"bootstrap,omitempty"`
	DueAt     int64  `json:"due_at,omitempty"`
	// 服务器权威战斗引擎状态；演出由客户端本地确定性演算。
	Entities      []battleEntity `json:"entities,omitempty"`
	ActionEID     string         `json:"action_eid,omitempty"`
	ActionCounter int64          `json:"action_counter,omitempty"`
	AttackDamage  int            `json:"attack_damage,omitempty"`
	AttackTarget  string         `json:"attack_target,omitempty"`
	LastClock     int64          `json:"last_clock,omitempty"`
	// instance_trigger/instance_event 数据驱动解释器运行态（battle_triggers.go）。
	Triggers *triggerRuntime `json:"triggers,omitempty"`
	// 结算计数防重：battle_result 只推一次，new_task 通关计数同点推进一次。
	ResultCounted bool `json:"result_counted,omitempty"`
	RewardGranted bool `json:"reward_granted,omitempty"`
	// 结果接收先持久抽奖收据，再在结算事务兑现，失败重试不重新抽取。
	RewardBonusPlan      []BattleBonusReceipt `json:"reward_bonus_plan,omitempty"`
	RewardPlanFrozen     bool                 `json:"reward_plan_frozen,omitempty"`
	RewardResult         *battleEnvelope      `json:"reward_result,omitempty"`
	RewardPlannedAt      int64                `json:"reward_planned_at,omitempty"`
	RewardOverflowMailed bool                 `json:"reward_overflow_mailed,omitempty"`
	RewardExtraPlanStart *int                 `json:"reward_extra_plan_start,omitempty"`
	// 客户端权威观察链（bridge rev 12）：桥握手、事件序号与显式结果。
	Team             []string               `json:"team,omitempty"`
	RecoveredFrom    string                 `json:"recovered_from,omitempty"`
	Layout           *BattleLayout          `json:"layout,omitempty"`
	Extra            map[string]any         `json:"extra,omitempty"`
	ActivityContext  *ActivityBattleContext `json:"activity_context,omitempty"`
	BridgeReady      bool                   `json:"bridge_ready,omitempty"`
	BridgeStarted    bool                   `json:"bridge_started,omitempty"`
	LastSequence     int64                  `json:"last_sequence,omitempty"`
	Outcome          string                 `json:"outcome,omitempty"`
	WinnerEIDs       []string               `json:"winner_eids,omitempty"`
	FinishedTaskList []int                  `json:"finished_task_list,omitempty"`
	BattleSpeed      float64                `json:"battle_speed,omitempty"`
	AutoBattle       bool                   `json:"auto_battle,omitempty"`
	// ClientPlayerEID 由客户端原生引擎在结果包中明确报告；它可能不是 Avatar 的 ObjectID。
	ClientPlayerEID string         `json:"client_player_eid,omitempty"`
	EventLog        []BattleEvent  `json:"event_log,omitempty"`
	Finished        bool           `json:"finished,omitempty"`
	SettlementBox   map[string]any `json:"settlement_box,omitempty"`
	// 单客户端PVP原生权威，Journal和播放确认随玩家事务持久。
	NativeSolo    *HumanPvpRoom `json:"native_solo,omitempty"`
	PaidPower     int64         `json:"paid_power,omitempty"`
	PaidMaterials map[int]int64 `json:"paid_materials,omitempty"`
}

func canEnterGuideDungeon(p *Progress, dungeonID int) bool {
	for id, task := range p.GuideTasks {
		if task.Status != 1 {
			continue
		}
		definition := clientBaseline.Guides[id]
		// guide_task_finished 先在客户端启动下一任务，然后才上报当前完成，真实 TCP 顺序已证。
		for _, candidate := range []GuideDefinition{definition, clientBaseline.Guides[definition.Next]} {
			if candidate.DoType != 2 || len(candidate.DoParams) == 0 {
				continue
			}
			target, err := strconv.Atoi(fmt.Sprint(candidate.DoParams[0]))
			if err == nil && target == dungeonID {
				return true
			}
		}
	}
	return false
}

func (s *Service) enterDungeon(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 3 {
		return nil, errors.New("进入副本需要玩家状态及回调、编号、附加字典")
	}
	var cb, id int
	var extra map[string]any
	if json.Unmarshal(args[0], &cb) != nil || json.Unmarshal(args[1], &id) != nil || json.Unmarshal(args[2], &extra) != nil {
		return nil, errors.New("进入副本参数无效")
	}
	definition, known := dungeonCatalog[id]
	if !known {
		return nil, runeReject("RET_DUNGEON_DATA_ERROR", "未知副本")
	}
	if id == asyncRule().DungeonID {
		if _, ok := extra["asyn_pvp_eid"]; !ok {
			return nil, errors.New("竞技副本必须使用已授权异步对手")
		}
		return s.enterAsyncPvp(ctx, c, args)
	}
	// Android 1.0.128 同步 PVP 使用副本 21、activity_id=1。
	// 该入口只建立服务端锁定的 AI 匹配快照；战斗仍由 pvp_load_complete
	// 触发客户端原生桥，不能落入普通副本的引导门槛和体力扣除路径。
	if id == 21 && parsePVPDungeonExtra(extra) {
		var match *SyncPvpMatch
		seed := fmt.Sprintf("%x:%d", selectedOID(c), s.Now().Unix()/5)
		if err := s.updateProgress(ctx, c, func(p *Progress) error {
			var err error
			match, err = prepareSyncPvpMatch(p, seed, s.Now())
			if err == nil {
				match.Extra = ensureSyncPVPExtra(match.Extra)
				for key, value := range extra {
					match.Extra[key] = value
				}
			}
			return err
		}); err != nil {
			return nil, err
		}
		infos, err := s.syncPvpSelectInfos(c, match)
		if err != nil {
			return nil, err
		}
		rule, err := syncPvpScoreRuleFor(match.Score)
		if err != nil {
			return nil, err
		}
		return []Push{Callback(cb, []any{RetSuccess}), push("Avatar", "select_pvp_cards", infos, rule.LineupNum)}, nil
	}
	// battle_id<=0 是纯剧情节点：直接结算通关（对齐参考 _story_dungeon）。
	if definition.BattleID <= 0 {
		return s.storyDungeonExtra(ctx, c, cb, id, extra)
	}
	var oid []byte
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			if !av.NicknameSet {
				return nil, ErrProfileState
			}
			oid = av.OID
		}
	}
	var session BattleSession
	queued := false
	duplicatePrepare := false
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		// 已授权的同副本恢复先于当前教学门槛；结算后激活的界面教学
		// 不能使先前已扣费但尚未结束的战斗无法重入。
		recovering := p.Battle != nil && !p.Battle.Finished && p.Battle.DungeonID == id
		if !recovering && !canEnterDungeon(p, id) {
			return runeReject("RET_DUNGEON_LOCKED", "引导期间的副本与当前引导不匹配")
		}
		if p.Battle != nil && !p.Battle.Finished && p.Battle.DungeonID != id {
			queued = true
			return nil
		}
		if p.Battle != nil && p.Battle.Status == "准备" {
			if p.Battle.DungeonID != id {
				return errors.New("已有其他战斗会话")
			}
			session = *p.Battle
			// 普通/活动关卡同连接同UUID已创建模型时，重复入口不能再次prepare
			// 或重置Loaded/Started。学会守护的专用重新布阵保留原有路径。
			deduplicate := p.Battle.ActivityContext == nil || p.Battle.ActivityContext.LeagueProtect == nil
			if deduplicate && c.ordinaryPrepareUUID == session.UUID {
				duplicatePrepare = true
				return nil
			}
			p.Battle.Loaded = false
			p.Battle.Started = false
			p.Battle.Bootstrap = ""
			p.Battle.DueAt = 0
			return nil
		}
		var seed [8]byte
		if p.Battle != nil && !p.Battle.Finished {
			return errors.New("已有进行中的战斗")
		}
		activity, err := prepareActivityDungeon(p, id, p.AvatarLevel, s.Now())
		if err != nil {
			return err
		}
		if activity != nil {
			if err := validateActivityDungeonExtra(p, activity, extra, p.AvatarLevel); err != nil {
				return err
			}
			if err := freezeActivityConfirmedBuffs(p, activity); err != nil {
				return err
			}
		}
		var paidMaterials map[int]int64
		if activity == nil {
			var err error
			paidMaterials, err = prepareOrdinaryDungeonCosts(p, id, s.Now())
			if err != nil {
				return err
			}
		}
		if _, err := rand.Read(seed[:]); err != nil {
			return err
		}
		// need_power 扣费仅新会话时发生（幂等重入不重复扣）；恢复结算按
		// power.last_time 推进，见 Progress.currentPower。
		powerCost, err := activityPowerCost(*p, id, definition.Power)
		if err != nil {
			return err
		}
		powerCost *= remainingDungeonPowerMultiple(p, id)
		powerCost, err = prepareOrdinaryDoubleCosts(p, id, powerCost, paidMaterials)
		if err != nil {
			return err
		}
		if !p.consumePower(powerCost, s.Now()) {
			return runeReject("RET_DUNGEON_POWER_NOT_ENOUGH", "体力不足")
		}
		if activity != nil {
			activity.PaidPower = int64(powerCost)
		}
		session = BattleSession{UUID: hex.EncodeToString(NewAvatarOID(s.Now())), DungeonID: id, BattleID: definition.BattleID, Seed: int64(binary.LittleEndian.Uint64(seed[:]) & 0x7fffffff), Status: "准备", CreatedAt: s.Now().Unix(), Extra: activityBattleExtra(activity, extra), ActivityContext: activity}
		p.Battle = &session
		p.Battle.PaidPower = int64(powerCost)
		p.Battle.PaidMaterials = paidMaterials
		if err := prepareRemainingDungeon(p, p.Battle, s.Now()); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if queued {
		if len(c.pendingDungeonArgs) != 0 && string(c.pendingDungeonArgs[1]) != string(args[1]) {
			return nil, errors.New("已有等待结算的下一副本")
		}
		c.pendingDungeonArgs = make([]json.RawMessage, len(args))
		for index := range args {
			c.pendingDungeonArgs[index] = append(json.RawMessage(nil), args[index]...)
		}
		log.Printf("下一副本等待当前客户端结算 current=%d next=%d", c.SelectedAvatarUnsafe().Progress.Battle.DungeonID, id)
		return nil, nil
	}
	if duplicatePrepare {
		// 重复入场只应答原callback，不能在已有客户端battle上再次创建/prepare。
		return []Push{Callback(cb, []any{RetSuccess})}, nil
	}
	c.ordinaryPrepareUUID = session.UUID
	// 新会话或同副本重新准备都会使旧的开战请求失效。
	c.pendingBattleFightingArgs = nil
	c.pendingBattleFightingUUID = ""
	c.battleStartSent = false
	playerID := ObjectID(hex.EncodeToString(oid))
	var prefs BattlePrefs
	var layout BattleLayout
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			prefs = av.Progress.battlePreferences()
			layout = savedBattleLayout(av.Progress, id)
		}
	}
	pushes := []Push{
		// 桥脚本必须在 shadow_battle 构造前安装（on_query_hotfix_success 直接执行源码）。
		push("Avatar", "on_query_hotfix_success", clientExtensionsScript(), bridgeHotfixIndex),
		Callback(cb, []any{0}),
		push("Avatar", "start_server_battle_ok", ordinaryBattleTransportType(definition.Type), id, ObjectID(session.UUID), activityBattleExtra(session.ActivityContext, session.Extra)),
	}
	if len(layout.Fighting) > 0 || len(layout.Support) > 0 {
		pushes = append(pushes, battleSync("set_last_fighting_cards", lastFightingWire(playerID, layout.Fighting, layout.Support)))
	}
	pushes = append(pushes,
		battleSync("prepare", []any{playerID}, session.BattleID, session.Seed, id),
		battleSync("revival_set_battle_preferences", prefs.BattleSpeed, prefs.AutoBattle),
		dungeonEntryPush(c.SelectedAvatarUnsafe().Progress, id))
	log.Printf("战斗会话建立（客户端权威） dungeon=%d battle=%d uuid=%s", id, session.BattleID, session.UUID)
	return pushes, nil
}

// 原生退出确认会直接索引dungeon_mgr；未通关也必须有本次已授权入场的行。
func dungeonEntryPush(p Progress, id int) Push {
	finished := 0
	if containsInt(p.ClearedDungeons, id) {
		finished = 1
	}
	return push("Avatar", "client_prop_set", []any{"dungeon_mgr", id,
		map[string]any{"dungeon_id": id, "finished": finished, "double": p.DungeonBonusDouble[id]}})
}

// storyDungeon 结算纯剧情节点：进度推进后回 battle_result，不开战。
func (s *Service) storyDungeon(ctx context.Context, c *Connection, cb, id int) ([]Push, error) {
	return s.storyDungeonExtra(ctx, c, cb, id, map[string]any{})
}
func (s *Service) storyDungeonExtra(ctx context.Context, c *Connection, cb, id int, extra map[string]any) ([]Push, error) {
	var settle *battleSettlement
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if !canEnterDungeon(p, id) {
			return errors.New("剧情副本与引导状态不匹配")
		}
		if p.Battle != nil && !p.Battle.Finished {
			return errors.New("进行中战斗不能被剧情覆盖")
		}
		activity, err := prepareActivityDungeon(p, id, p.AvatarLevel, s.Now())
		if err != nil {
			return err
		}
		if activity != nil {
			if err := validateActivityDungeonExtra(p, activity, extra, p.AvatarLevel); err != nil {
				return err
			}
			if err := freezeActivityConfirmedBuffs(p, activity); err != nil {
				return err
			}
		}
		var paidMaterials map[int]int64
		if activity == nil {
			paidMaterials, err = prepareOrdinaryDungeonCosts(p, id, s.Now())
			if err != nil {
				return err
			}
		}
		powerCost, err := activityPowerCost(*p, id, dungeonCatalog[id].Power)
		if err != nil {
			return err
		}
		powerCost *= remainingDungeonPowerMultiple(p, id)
		powerCost, err = prepareOrdinaryDoubleCosts(p, id, powerCost, paidMaterials)
		if err != nil {
			return err
		}
		if !p.consumePower(powerCost, s.Now()) {
			return errors.New("剧情副本体力不足")
		}
		if activity != nil {
			activity.PaidPower = int64(powerCost)
		}
		fake := &BattleSession{UUID: newBattleUUID(s.Now()), DungeonID: id, ActivityContext: activity, PaidPower: int64(powerCost), PaidMaterials: paidMaterials, Extra: activityBattleExtra(activity, extra)}
		if err := prepareRemainingDungeon(p, fake, s.Now()); err != nil {
			return err
		}
		envelope := &battleEnvelope{Kind: "result", Data: map[string]any{"winner_eids": []any{}}}
		// 剧情节点视为胜利通关（首通推进与奖励同战斗路径）。
		envelope.Data["winner_eids"] = []any{hexOf(selectedOID(c))}
		box, err := settleActivityDungeon(p, fake, true, nil, s.Now())
		if err != nil {
			return err
		}
		if activity == nil {
			box, err = settleOrdinaryDungeonRewards(p, fake, true, s.Now())
			if err != nil {
				return err
			}
			if err := settleRemainingDungeon(p, fake, true, nil, box, s.Now()); err != nil {
				return err
			}
		}
		settle = settleClientBattle(p, fake, envelope, hexOf(selectedOID(c)), s.Now())
		if err := ensureMainlineAutoEnhance(p); err != nil {
			return err
		}
		settle.remainingCompletion = remainingDungeonCompletionPushes(*p, fake)
		settle.fullBonus = box
		p.Battle = nil
		return nil
	}); err != nil {
		return nil, err
	}
	settle.firstClear = false
	settlementPushes, err := settle.pushes(c)
	if err != nil {
		return nil, err
	}
	// 原生check_site_finished只读取dungeon_mgr.finished，不读取服务器内部
	// ClearedDungeons。剧情同样必须在battle_result之前同步真实完成行；
	// 重入同步已提交进度，不重复首通奖励，也不靠重启刷新世界地图。
	pushes := append([]Push{Callback(cb, []any{0}), dungeonEntryPush(c.SelectedAvatarUnsafe().Progress, id)}, settlementPushes...)
	return pushes, nil
}

func selectedOID(c *Connection) []byte {
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			return av.OID
		}
	}
	return nil
}

// ownedLineup 返回保存的出战阵容中仍拥有的卡（过滤已删卡）。
func ownedLineup(p *Progress) []string {
	owned := map[string]bool{}
	for _, card := range p.Cards {
		owned[card.UUID] = true
	}
	out := make([]string, 0, len(p.Lineup))
	for _, uuid := range p.Lineup {
		if owned[uuid] {
			out = append(out, uuid)
		}
	}
	return out
}

// 初始强制教学限制入口；通关501后的召唤/邮件等界面教学不封锁普通副本。
// 普通副本仍由原有章节、通关和资产规则校验。
func canEnterDungeon(p *Progress, dungeonID int) bool {
	active := false
	for id, task := range p.GuideTasks {
		if task.Status == 1 && id <= 1013 {
			active = true
		}
	}
	if !active {
		return true
	}
	return canEnterGuideDungeon(p, dungeonID)
}

func battleSync(name string, args ...any) Push {
	return push("Avatar", "sync_battle_method", name, args, map[string]any{})
}

func (s *Service) battleLoaded(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 0 {
		return nil, errors.New("战斗加载完成需要玩家状态及空参数")
	}
	if r := c.SelectedAvatarUnsafe().Progress.Social.HumanRoom; humanRoomActive(r) {
		// Android实际上报load_entity_finish；加载状态必须在共同房间双角色
		// 事务内同步，不能只改本人Battle而永久等待pvp_load_complete。
		_, err := s.humanTransaction(ctx, c, func(r *HumanPvpRoom, v map[string]*Avatar, id string) error {
			b := v[id].Progress.Battle
			if b == nil || b.UUID != r.UUID || b.Finished || (r.Status != "loading" && r.Status != "fighting") {
				return errors.New("真人加载完成的战斗归属无效")
			}
			own := r.Players[id]
			own.Loaded = true
			r.Players[id] = own
			b.Loaded = true
			return nil
		})
		if err != nil {
			return nil, err
		}
		return s.advancePvpLoadBarrier(ctx, c)
	}
	battleUUID := ""
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if p.Battle == nil {
			return errors.New("没有战斗会话")
		}
		p.Battle.Loaded = true
		battleUUID = p.Battle.UUID
		return nil
	}); err != nil {
		return nil, err
	}
	log.Printf("教学战斗实体加载完成")
	if len(c.pendingBattleFightingArgs) != 0 {
		if c.pendingBattleFightingUUID != battleUUID {
			log.Printf("丢弃过期的延迟开战请求 pending=%s current=%s", c.pendingBattleFightingUUID, battleUUID)
			c.pendingBattleFightingArgs = nil
			c.pendingBattleFightingUUID = ""
			return nil, nil
		}
		pending := c.pendingBattleFightingArgs
		c.pendingBattleFightingArgs = nil
		c.pendingBattleFightingUUID = ""
		log.Printf("实体加载完成，接续延迟的 battle_fighting uuid=%s", battleUUID)
		return s.battleFighting(ctx, c, pending)
	}
	return s.advancePvpLoadBarrier(ctx, c)
}

func (s *Service) battleFighting(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if handled, pushes, err := s.humanBattleFighting(ctx, c, args); handled {
		return pushes, err
	}
	if c.phase != Playing || len(args) != 1 {
		return nil, errors.New("战斗准备需要玩家状态及阵容字典")
	}
	current := c.SelectedAvatarUnsafe().Progress.Battle
	if current == nil {
		return nil, errors.New("没有战斗会话")
	}
	// 原生客户端在快速切场、教学演出回调或网络重试时，可能在开战成功后
	// 再补发一份已经失去battle_cards对象的battle_fighting。阵容已在首次
	// 开战事务中冻结，重复请求必须幂等忽略，不能重新解析旧对象后向客户端
	// 回送业务失败；任何伪造内容也不会改变已开始会话或阵容。
	if c.battleStartSent || current.Started {
		log.Printf("忽略已开始战斗的重复 battle_fighting dungeon=%d", current.DungeonID)
		return nil, nil
	}
	// 同步PVP在send_pvp_cards阶段已由服务器验证并锁定阵容。
	// 部分旧客户端在收到就绪通知后，会把battle_cards自定义对象
	// 重复上报成null或旧形状；此处必须使用锁定快照，不能再信任
	// 或依赖客户端的第二份阵容。普通PVE仍走严格原生字典解析。
	layout, lockedSyncPvp := syncPvpLockedBattleLayout(c.SelectedAvatarUnsafe().Progress)
	if !lockedSyncPvp {
		var err error
		layout, err = parseBattleLayoutForDungeon(args[0], current.DungeonID)
		if err != nil {
			return nil, err
		}
	}
	if !current.Loaded {
		c.pendingBattleFightingArgs = []json.RawMessage{append(json.RawMessage(nil), args[0]...)}
		c.pendingBattleFightingUUID = current.UUID
		log.Printf("battle_fighting 等待实体加载 uuid=%s dungeon=%d", current.UUID, current.DungeonID)
		return nil, nil
	}
	var team []string
	started := false
	var progress *Progress
	if err := s.updateBattleFormation(ctx, c, layout, 0, true, func(p *Progress) error {
		if p.Battle == nil || !p.Battle.Loaded {
			return errors.New("战斗实体尚未加载")
		}
		if c.battleStartSent {
			return nil
		}
		// 入事务后重读锁定快照，避免用于开战的阵容依赖事务外旧值。
		if locked, ok := syncPvpLockedBattleLayout(*p); ok {
			layout = locked
		} else if lockedSyncPvp {
			return errors.New("同步PVP锁定阵容已失效")
		}
		if err := validateBattleLayout(*p, layout); err != nil {
			return err
		}
		if p.SyncPvpMatch != nil && p.Battle.UUID == p.SyncPvpMatch.BattleUUID {
			if !sameBattleSlots(layout.Fighting, p.SyncPvpMatch.OwnFighting) || !sameBattleSlots(layout.Support, p.SyncPvpMatch.OwnSupport) {
				return errors.New("同步PVP开战阵容与锁定预设不符")
			}
		}
		copy := cloneBattleLayout(layout)
		team = layout.team()
		p.Battle.Layout = &copy
		p.Battle.Team = append([]string{}, team...)
		if err := freezeActivityRankCards(p, p.Battle); err != nil {
			return err
		}
		freezeAsyncOwnTeam(p)
		if err := s.beginNativeSolo(ctx, p, hexOf(selectedOID(c))); err != nil {
			return err
		}
		if err := consumeLeagueProtectAttempt(p, p.Battle, s.Now()); err != nil {
			return err
		}
		p.Battle.Started = true
		// 客户端权威：清理旧权威引擎运行态，Tick 不再推进本场战斗。
		p.Battle.Entities, p.Battle.Triggers = nil, nil
		p.Battle.Bootstrap, p.Battle.DueAt = "", 0
		if p.BattleLayouts == nil {
			p.BattleLayouts = map[int]BattleLayout{}
		}
		p.BattleLayouts[p.Battle.DungeonID] = cloneBattleLayout(layout)
		p.Lineup = append([]string{}, team...)
		started = true
		progress = p
		return nil
	}); err != nil {
		return nil, err
	}
	if !started {
		return nil, nil
	}
	placeholder := len(team) == 0 // 只有纯占位阵容走教学角色，混合空槽不得漏卡。
	c.battleStartSent = true
	var playerID ObjectID
	// Handle 已持有连接锁，此处直接读取鉴权身份，不能再调用 SelectedAvatar 加锁。
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			playerID = ObjectID(hex.EncodeToString(av.OID))
		}
	}
	if playerID == "" {
		return nil, errors.New("战斗玩家不存在")
	}
	// assemble.py 将同名组件方法依次执行，而非覆盖。add_fighting_cards 同时初始化
	// battle_base 的玩家字典与 server_controler 的录像；空 card_list 仍须带自定义类型标记。
	// 教学角色由 battle_info.my_avatar_list 创建，零占位不是玩家已拥有的卡。
	var fightingCards []any
	if placeholder {
		fightingCards = []any{[]any{}, []any{"card.card_list", "__custom_type"}, playerID}
	} else {
		oidTeam := make([]any, 0, len(team))
		for _, uuid := range team {
			oidTeam = append(oidTeam, ObjectID(uuid))
		}
		cards := append(battleCardListWire(progress, team), "card.card_list", "__custom_type")
		fightingCards = []any{oidTeam, cards, playerID}
	}
	calculation := "客户端原生计算"
	if progress.Battle.NativeSolo != nil {
		calculation = "服务端单原生权威"
	}
	log.Printf("战斗开始（%s）阵容=%d 占位=%v", calculation, len(team), placeholder)
	pushes := []Push{battleSync("add_fighting_cards", fightingCards...)}
	pushes = append(pushes, asyncEnemyPush(*progress)...)
	if progress.SyncPvpMatch != nil && progress.Battle.UUID == progress.SyncPvpMatch.BattleUUID {
		m := progress.SyncPvpMatch
		enemyTeam := make([]any, 0, len(m.Robot.SyncPvpCards))
		for _, id := range m.Robot.SyncPvpCards {
			enemyTeam = append(enemyTeam, ObjectID(robotCardUUID(id)))
		}
		cards := append(robotCardsWire(m.Robot), "card.card_list", "__custom_type")
		pushes = append(pushes, battleSync("add_fighting_cards", enemyTeam, cards, ObjectID(robotAvatarEID(m.Robot.ID))))
	}
	return append(pushes, battleSync("battle_fighting"), battleSync("start"), battleSync("check_on_battle_start")), nil
}

// syncPvpLockedBattleLayout只识别当前UUID的已就绪同步PVP。
// 返回独立切片，防止战斗会话与匹配快照共享可变底层数组。
func syncPvpLockedBattleLayout(p Progress) (BattleLayout, bool) {
	if p.Battle == nil || p.SyncPvpMatch == nil ||
		p.SyncPvpMatch.Status != syncPvpMatchReady ||
		p.Battle.UUID == "" || p.Battle.UUID != p.SyncPvpMatch.BattleUUID ||
		len(p.SyncPvpMatch.OwnFighting) == 0 {
		return BattleLayout{}, false
	}
	return BattleLayout{
		Fighting: append([]string{}, p.SyncPvpMatch.OwnFighting...),
		Support:  append([]string{}, p.SyncPvpMatch.OwnSupport...),
	}, true
}

func sameBattleSlots(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Tick 推进已加载的首场教学启动。业务时钟由传输定时器驱动，不依赖客户端心跳推进回合。
// 仅覆盖空开场演出到第一次输入；技能执行、后续回合及结算需另外实现。
func (s *Service) Tick(ctx context.Context, c *Connection) (result []Push, resultErr error) {
	c.mu.Lock()
	finishingLogin := c.phase == Playing && !c.refreshLoginSent && len(c.reconnectAuth) != 0
	mailBefore := c.SelectedAvatarUnsafe().Progress.MailRevision
	defer func() {
		p := c.SelectedAvatarUnsafe().Progress
		changed := c.phase == Playing && p.MailRevision > mailBefore
		var snapshot Progress
		var oid []byte
		if changed {
			snapshot = CloneProgress(p)
			oid = append([]byte(nil), selectedOID(c)...)
			result = append([]Push{push("Avatar", "client_prop_changed", []any{"short_mail_info", mailProperties(p.ShortMailInfo, s.Now().Unix())}), push("Avatar", "notify_new_mail")}, result...)
		}
		pendingSocial := c.pendingSocialOIDs
		c.pendingSocialOIDs = nil
		c.mu.Unlock()
		for _, target := range pendingSocial {
			s.publishPlayerRefresh(target)
		}
		if changed {
			s.publishMailboxChange(oid, snapshot, true, c)
		}
	}()
	defer func() {
		if resultErr == nil && !finishingLogin && ctx.Err() == nil {
			if c.phase == Playing {
				daily, err := s.refreshDailyState(ctx, c)
				result = append(daily, result...)
				if err != nil {
					log.Printf("在线日界刷新失败，将在60秒后重试：%v", err)
				}
			}
			result = append(s.flushPlayerRefresh(ctx, c), result...)
			if c.phase == Playing {
				result = append(s.tickNativeSolo(ctx, c), result...)
				result = append(s.tickHumanPvp(ctx, c), result...)
				result = append(s.flushHumanPvpSnapshot(c), result...)
			}
			result = append(s.flushMailbox(c), result...)
		}
	}()
	if ctx.Err() != nil {
		return nil, nil
	}
	if finishingLogin {
		if s.Now().UnixMilli() < c.nextLoginFinishRetry {
			return nil, nil
		}
		pushes, err := s.finishLogin(ctx, c)
		if err != nil {
			return nil, nil // 原凭据已留在连接，失败不标记成功，稍后重试。
		}
		return pushes, nil
	}
	// 已卡在实际加载与握手均完成的会话也沿冻结阵容推进，无须新的UI回调。
	if c.phase == Playing {
		if pushes, err := s.advancePvpLoadBarrier(ctx, c); err != nil || len(pushes) > 0 {
			return pushes, err
		}
	}
	if c.phase != Playing || !c.battleStartSent {
		return nil, nil
	}
	if c.pendingOrdinaryResult != nil {
		return s.retryOrdinaryResult(ctx, c)
	}
	var session *BattleSession
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			session = av.Progress.Battle
		}
	}
	now := s.Now().UnixMilli()
	if session == nil || session.BattleID != 10001 || session.DueAt == 0 || now < session.DueAt {
		return nil, nil
	}
	// 所有已完成 bridge 握手的主线、活动、关卡和引导战斗均由客户端
	// 原生引擎推进。旧服务端引擎仅在没有 bridge 握手的本地回归夹具中保留。
	if session.BridgeReady {
		return nil, nil
	}
	var effects []Push
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		b := p.Battle
		if b == nil {
			return nil
		}
		if b.BattleID == guideBattleID && len(b.Entities) == 0 && waitingForInput(b.Bootstrap) {
			// 引擎上线前的旧会话：等待输入但尚无权威实体，补建后进入引擎。
			if err := ensureEntities(b, s.Now); err != nil {
				return err
			}
		}
		if len(b.Entities) > 0 && b.Bootstrap != phaseEnterShow && b.Bootstrap != phaseOpeningEnd {
			// 权威引擎阶段（行动演出/等待输入/已结束）：由 battle_engine 推进。
			engineEffects, err := s.tickGuideBattle(b, now)
			if err != nil {
				return err
			}
			if b.Bootstrap == phaseBattleOver && !b.ResultCounted {
				// 通关一次推 new_task 计数与 system_unlock 解锁（幂等由 status/已解锁跳过保证）。
				b.ResultCounted = true
				if !b.RewardGranted {
					if reward := androidMainlineRewards[b.DungeonID]; len(reward) > 0 {
						if p.Materials == nil {
							p.Materials = map[int]Material{}
						}
						for id, amount := range reward {
							m := p.Materials[id]
							m.ID = id
							m.Count += int64(amount)
							m.Total += int64(amount)
							p.Materials[id] = m
						}
						b.RewardGranted = true
						props := map[string]any{}
						for id, m := range p.Materials {
							props[strconv.Itoa(id)] = map[string]any{"material_id": m.ID, "count": m.Count, "total": m.Total}
						}
						engineEffects = append(engineEffects, push("Avatar", "client_prop_changed", []any{"material_mgr", props}))
					}
				}
				if unlocked := p.advanceUnlocks(b.DungeonID); len(unlocked) > 0 {
					unlocks := map[string]any{}
					for system, version := range p.UnlockSystems {
						unlocks[system] = version
					}
					engineEffects = append(engineEffects, push("Avatar", "client_prop_changed", []any{"unlock_systems", unlocks}))
				}
			}
			effects = engineEffects
			return nil
		}
		if b.DueAt == 0 || now < b.DueAt {
			return nil
		}
		switch b.Bootstrap {
		case "入场演出":
			effects = []Push{battleSync("on_battle_start")}
			b.Bootstrap = "开场结束"
			// driver.on_battle_start: 无 BATTLE_START 被动，空 buff 演出耗时0，固定再等1秒。
			b.DueAt = now + 1000
		case "开场结束":
			// camp_mgr 按 camp_id 排序生成槽位，camp1 第一个槽位eid="1"。
			// instance_event[10001011]将camp1 AP置500，必定先于AP=0的敌人行动。
			effects = []Push{battleSync("real_round_end", nil), battleSync("real_start_next_round", 0),
				battleSync("after_pre_play", "1", 0, 0), battleSync("continue_wait_for_player_input")}
			b.Bootstrap = "等待首次输入"
			b.DueAt = 0
		}
		return nil
	})
	if len(effects) > 0 && err == nil {
		log.Printf("教学战斗启动推进 阶段=%s", session.Bootstrap)
	}
	return effects, err
}
