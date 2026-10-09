package game

// 客户端权威战斗观察链：客户端原生引擎本地执行全部战斗演算，服务端经
// do_command("__battle_event__", envelope) 接收桥接事件、校验握手与序号、
// 记录事件并在显式 winner_eids 结果后结算发奖。移植自 GMA-Revive-Server
// turnclock.py / server.py（bridge revision 15，握手协议版本 1）。

import (
	"context"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"time"

	"hs-server/internal/mobileproto"
)

//go:embed battle_bridge_script.py
var battleBridgeScript string

//go:embed dungeon_catalog.json
var dungeonCatalogData []byte

const (
	bridgeHotfixIndex   = 2026100911
	bridgeProtocol      = 1
	battleEventCommand  = "__battle_event__"
	battleEventMaxJSON  = 512000
	battleEventLogMax   = 128
	battleEventLogBytes = 2 * 1024 * 1024
	battleRosterMax     = 512
)

type dungeonCatalogEntry struct {
	BattleID int `json:"battle_id"`
	Type     int `json:"type"`
	Power    int `json:"power"`
}

// dungeonCatalog 为全量副本目录（1.0.128 导出）；battle_id<=0 为纯剧情节点。
var dungeonCatalog = func() map[int]dungeonCatalogEntry {
	var raw struct {
		Dungeons map[string]dungeonCatalogEntry `json:"dungeons"`
	}
	if err := json.Unmarshal(dungeonCatalogData, &raw); err != nil {
		panic(err)
	}
	out := make(map[int]dungeonCatalogEntry, len(raw.Dungeons))
	for key, value := range raw.Dungeons {
		id, err := strconv.Atoi(key)
		if err != nil {
			panic("副本目录编号无效: " + key)
		}
		out[id] = value
	}
	return out
}()

// BattlePrefs 按账号保存战斗速度与自动战斗（断线重连、重启后恢复）。
type BattlePrefs struct {
	BattleSpeed float64 `json:"battle_speed"`
	AutoBattle  bool    `json:"auto_battle"`
}

// BattleEvent 是客户端桥上报的观察事件；事件流保留在连接内存供诊断，
// result/settings 等关键状态同步持久化到 avatar_progress。
type BattleEvent struct {
	Sequence int64          `json:"sequence"`
	Kind     string         `json:"kind"`
	Data     map[string]any `json:"data,omitempty"`
}

var battleEventKinds = map[string]bool{
	"ready": true, "started": true, "settings": true, "turn": true, "input": true,
	"command": true, "skill_targets": true, "snapshot": true, "round_end": true,
	"story": true, "result": true, "error": true,
}

// battleEnvelope 是 do_command("__battle_event__", [envelope]) 的载荷。
type battleEnvelope struct {
	BattleUUID string         `json:"battle_uuid"`
	Sequence   int64          `json:"sequence"`
	Generation int64          `json:"generation,omitempty"`
	Kind       string         `json:"kind"`
	Data       map[string]any `json:"data"`
}

func (p *Progress) battlePreferences() BattlePrefs {
	prefs := p.BattlePreferences
	if prefs.BattleSpeed != 1 && prefs.BattleSpeed != 1.75 {
		prefs.BattleSpeed = 1
	}
	return prefs
}

func hexOf(oid []byte) string { return hex.EncodeToString(oid) }

func newBattleUUID(now time.Time) string { return hexOf(NewAvatarOID(now)) }

// absorbBattleEvent 处理 __battle_event__ 上行；校验通过则推进观察状态机。
func (s *Service) absorbBattleEvent(ctx context.Context, c *Connection, rawArgs []any) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("战斗事件需要玩家状态")
	}
	if len(rawArgs) != 1 {
		return nil, errors.New("战斗事件载荷必须为单元素列表")
	}
	raw, ok := rawArgs[0].(map[string]any)
	if !ok {
		return nil, errors.New("战斗事件信封无效")
	}
	var envelope battleEnvelope
	blob, _ := json.Marshal(raw)
	if err := json.Unmarshal(blob, &envelope); err != nil {
		return nil, errors.New("战斗事件信封字段无效")
	}
	if envelope.Kind == "result" && c.SelectedAvatarUnsafe().Progress.SyncPvpMatch != nil {
		if err := s.refreshPvpAwards(ctx, c); err != nil {
			return nil, err
		}
	}
	if handled, pushes, err := s.absorbHumanBattleEvent(ctx, c, &envelope); handled {
		return pushes, err
	}
	if handled, pushes, err := s.absorbNativeSoloEvent(ctx, c, &envelope); handled {
		return pushes, err
	}
	if handled, pushes, err := s.absorbAsyncPvpResult(ctx, c, &envelope); handled {
		return pushes, err
	}
	avatarEID := ""
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			avatarEID = hexOf(av.OID)
		}
	}
	var settle *battleSettlement
	var pvpResult *SyncPvpResult
	// 普通战斗演算已经在客户端完成；逐回合诊断不能重写整份玩家JSONB。
	// 严格序号校验保留在串行连接内，握手/设置/结算和其他资产事务带入检查点。
	pve := c.SelectedAvatarUnsafe().Progress
	ordinary := pve.SyncPvpMatch == nil && (pve.AsyncPvp.Match == nil || pve.AsyncPvp.Match.UUID != envelope.BattleUUID)
	if ordinary && envelope.Kind != "result" {
		b := c.SelectedAvatarUnsafe().Progress.Battle
		if b == nil || b.Finished || b.UUID != envelope.BattleUUID {
			log.Printf("战斗事件拒绝 uid=%d uuid=%s seq=%d kind=%s 原因=战斗事件归属不符", c.SelectedAvatarUnsafe().UID, envelope.BattleUUID, envelope.Sequence, envelope.Kind)
			return nil, nil
		}
		if c.ordinaryObservation == nil || c.ordinaryObservation.UUID != b.UUID {
			c.ordinaryObservation = cloneOrdinaryObservation(b)
		}
		candidate := cloneOrdinaryObservation(c.ordinaryObservation)
		if err := acceptBattleEvent(candidate, &envelope); err != nil {
			log.Printf("战斗事件拒绝 uid=%d dungeon=%d uuid=%s seq=%d expected=%d kind=%s 原因=%v", c.SelectedAvatarUnsafe().UID, b.DungeonID, envelope.BattleUUID, envelope.Sequence, candidate.LastSequence+1, envelope.Kind, err)
			return nil, nil
		}
		appendOrdinaryEvent(candidate, &envelope)
		if envelope.Kind == "settings" {
			automatic := envelope.Data["auto_battle"].(bool)
			c.ordinaryPendingAuto = &automatic
		}
		c.ordinaryObservation = candidate
		c.ordinaryObservationAt = s.Now().UnixMilli()
		mergeOrdinaryObservation(b, candidate)
		if envelope.Kind != "ready" && envelope.Kind != "started" && envelope.Kind != "settings" {
			return nil, nil
		}
		err := s.updateProgress(ctx, c, func(p *Progress) error {
			if p.Battle == nil || p.Battle.UUID != envelope.BattleUUID || p.Battle.Finished {
				return errors.New("战斗事件归属不符")
			}
			if envelope.Kind == "settings" {
				auto, _ := envelope.Data["auto_battle"].(bool)
				prefs := p.battlePreferences()
				prefs.AutoBattle = auto
				p.BattlePreferences, p.Battle.AutoBattle = prefs, auto
			}
			return nil
		})
		if err != nil {
			log.Printf("战斗观察检查点延后 uid=%d dungeon=%d uuid=%s seq=%d 原因=%v", c.SelectedAvatarUnsafe().UID, b.DungeonID, b.UUID, envelope.Sequence, err)
		}
		return nil, nil
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		b := p.Battle
		if b == nil || b.Finished {
			return errors.New("没有进行中的战斗")
		}
		if envelope.BattleUUID != b.UUID {
			return errors.New("战斗事件归属不符")
		}
		if err := acceptBattleEvent(b, &envelope); err != nil {
			return err
		}
		b.EventLog = append(b.EventLog, BattleEvent{Sequence: envelope.Sequence, Kind: envelope.Kind, Data: cloneEventData(envelope.Data)})
		if len(b.EventLog) > battleEventLogMax {
			b.EventLog = b.EventLog[len(b.EventLog)-battleEventLogMax:]
		}
		for eventLogBytes(b.EventLog) > battleEventLogBytes && len(b.EventLog) > 1 {
			b.EventLog = b.EventLog[1:]
		}
		if envelope.Kind == "settings" {
			auto, _ := envelope.Data["auto_battle"].(bool)
			prefs := p.battlePreferences()
			prefs.AutoBattle = auto
			p.BattlePreferences = prefs
			b.AutoBattle = auto
		}
		if envelope.Kind == "result" {
			if p.SyncPvpMatch != nil && p.SyncPvpMatch.BattleUUID == b.UUID {
				winners, _ := battleWinnerEIDs(envelope.Data)
				outcome := "loss"
				for _, eid := range winners {
					if eid == avatarEID {
						outcome = "win"
					}
				}
				if reported, ok := envelope.Data["outcome"].(string); ok && reported != outcome {
					return errors.New("同步PVP胜方与已锁定玩家不符")
				}
				b.Outcome, b.WinnerEIDs, b.Finished, b.Status = outcome, winners, true, "结束"
				result, err := settleSyncPvpResult(p, 0, s.Now())
				if err != nil {
					return err
				}
				pvpResult = &result
				return nil
			}
			var activityBox map[string]any
			winners, _ := battleWinnerEIDs(envelope.Data)
			player, _ := envelope.Data["player_eid"].(string)
			if player == "" {
				player = avatarEID
			}
			win := false
			for _, id := range winners {
				if id == player {
					win = true
				}
			}
			if outcome, ok := envelope.Data["outcome"].(string); ok {
				win = outcome == "win"
			}
			if b.ActivityContext != nil {
				if err := recordLeagueProtectStatistics(b, envelope.Data); err != nil {
					return err
				}
				if err := recordActivityBattleStatistics(b, envelope.Data); err != nil {
					return err
				}
				tasks, err := battleTaskList(envelope.Data["finished_task_list"])
				if err != nil {
					return err
				}
				activityBox, err = settleActivityDungeon(p, b, win, tasks, s.Now())
				if err != nil {
					return err
				}
				if err = applyBattleCardExp(p, b, activityBox); err != nil {
					return err
				}
			} else {
				var err error
				var tasks []int
				if raw := envelope.Data["finished_task_list"]; raw != nil {
					var taskErr error
					tasks, taskErr = battleTaskList(raw)
					if taskErr != nil {
						return taskErr
					}
				}
				if err = recordSpecialDrillAchievements(p, b, win, tasks, s.Now()); err != nil {
					return err
				}
				activityBox, err = settleOrdinaryDungeonRewards(p, b, win, s.Now())
				if err != nil {
					return err
				}
				if err := settleRemainingDungeon(p, b, win, tasks, activityBox, s.Now()); err != nil {
					return err
				}
			}
			settle = settleClientBattle(p, b, &envelope, avatarEID, s.Now())
			settle.remainingCompletion = append(remainingDungeonCompletionPushes(*p, b), leagueProtectCompletionPushes(*p, b)...)
			if extra := leagueProtectSettlementExtra(b); extra != nil {
				settle.leagueExtra = map[string]any{"league_activity_battle_result": extra}
			}
			settle.fullBonus = activityBox
			if activityBox != nil {
				b.SettlementBox = activityBox
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			copy := envelope
			copy.Data = cloneEventData(envelope.Data)
			c.pendingOrdinaryResult = &copy
			c.nextOrdinaryResultRetry = s.Now().UnixMilli() + 1000
		} else {
			c.pendingOrdinaryResult = nil
		}
		log.Printf("战斗事件拒绝 uid=%d uuid=%s seq=%d kind=%s 原因=%v", c.SelectedAvatarUnsafe().UID, envelope.BattleUUID, envelope.Sequence, envelope.Kind, err)
		return nil, nil
	}
	c.pendingOrdinaryResult = nil
	c.battleEvents = append(c.battleEvents, BattleEvent{Sequence: envelope.Sequence, Kind: envelope.Kind})
	if len(c.battleEvents) > battleEventLogMax {
		c.battleEvents = c.battleEvents[len(c.battleEvents)-battleEventLogMax:]
	}
	if settle != nil {
		log.Printf("客户端引擎结算 uuid=%s outcome=%s dungeon=%d", envelope.BattleUUID, settle.outcome, settle.dungeonID)
		pushes, err := settle.pushes(c)
		if err != nil {
			return nil, err
		}
		if len(c.pendingDungeonArgs) != 0 {
			pending := c.pendingDungeonArgs
			next, nextErr := s.enterDungeon(ctx, c, pending)
			if nextErr != nil {
				log.Printf("当前战斗已结算但下一副本建立失败 dungeon=%s 原因=%v", string(pending[1]), nextErr)
				return pushes, nil
			}
			c.pendingDungeonArgs = nil
			pushes = append(pushes, next...)
		}
		return pushes, nil
	}
	if pvpResult != nil {
		return syncPvpResultPushes(c, *pvpResult), nil
	}
	return nil, nil
}

func cloneEventData(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	raw, _ := json.Marshal(data)
	var clone map[string]any
	_ = json.Unmarshal(raw, &clone)
	return clone
}

func ordinaryObserverMessage(method string, args []json.RawMessage) bool {
	if method != "do_command" || len(args) != 2 {
		return false
	}
	var command string
	var envelopes []struct {
		Kind string `json:"kind"`
	}
	return json.Unmarshal(args[0], &command) == nil && command == battleEventCommand && json.Unmarshal(args[1], &envelopes) == nil && len(envelopes) == 1 && envelopes[0].Kind != "result"
}

func cachedOrdinaryObserverMessage(c *Connection, method string, args []json.RawMessage) bool {
	p := c.SelectedAvatarUnsafe().Progress
	b := p.Battle
	var command string
	var envelopes []struct {
		UUID string `json:"battle_uuid"`
	}
	observer := method == "do_command" && len(args) == 2 && json.Unmarshal(args[0], &command) == nil && command == battleEventCommand && json.Unmarshal(args[1], &envelopes) == nil && len(envelopes) == 1
	// 首条ready也只观察本连接已授权会话；检查点和结果事务仍核对最新持久UUID。
	return observer && b != nil && envelopes[0].UUID == b.UUID && !b.Finished && b.NativeSolo == nil && p.Social.HumanRoom == nil && p.SyncPvpMatch == nil && (p.AsyncPvp.Match == nil || p.AsyncPvp.Match.UUID != b.UUID)
}

func cloneOrdinaryObservation(b *BattleSession) *BattleSession {
	if b == nil {
		return nil
	}
	return &BattleSession{UUID: b.UUID, DungeonID: b.DungeonID, LastSequence: b.LastSequence,
		BridgeReady: b.BridgeReady, BridgeStarted: b.BridgeStarted, EventLog: append([]BattleEvent(nil), b.EventLog...)}
}

func mergeOrdinaryObservation(b, observation *BattleSession) {
	if b == nil || observation == nil || b.Finished || b.UUID != observation.UUID || b.LastSequence > observation.LastSequence {
		return
	}
	b.LastSequence, b.BridgeReady, b.BridgeStarted = observation.LastSequence, observation.BridgeReady, observation.BridgeStarted
	b.EventLog = append([]BattleEvent(nil), observation.EventLog...)
}

func appendOrdinaryEvent(b *BattleSession, e *battleEnvelope) {
	b.EventLog = append(b.EventLog, BattleEvent{Sequence: e.Sequence, Kind: e.Kind, Data: cloneEventData(e.Data)})
	if len(b.EventLog) > battleEventLogMax {
		b.EventLog = b.EventLog[len(b.EventLog)-battleEventLogMax:]
	}
	for eventLogBytes(b.EventLog) > battleEventLogBytes && len(b.EventLog) > 1 {
		b.EventLog = b.EventLog[1:]
	}
}

func (s *Service) retryOrdinaryResult(ctx context.Context, c *Connection) ([]Push, error) {
	e := c.pendingOrdinaryResult
	if e == nil || s.Now().UnixMilli() < c.nextOrdinaryResultRetry {
		return nil, nil
	}
	b := c.SelectedAvatarUnsafe().Progress.Battle
	if b == nil || b.UUID != e.BattleUUID {
		c.pendingOrdinaryResult = nil
		return nil, nil
	}
	if b.Finished {
		c.pendingOrdinaryResult = nil
		return s.clientNeedRecoverBattle(ctx, c, nil)
	}
	c.nextOrdinaryResultRetry = s.Now().UnixMilli() + 1000
	raw, _ := json.Marshal(e)
	var value map[string]any
	_ = json.Unmarshal(raw, &value)
	return s.absorbBattleEvent(ctx, c, []any{value})
}

func eventLogBytes(events []BattleEvent) int {
	total := 0
	for _, event := range events {
		raw, _ := json.Marshal(event.Data)
		total += len(raw)
	}
	return total
}

// acceptBattleEvent 校验序号、握手顺序与载荷结构；按参考实现逐条对齐。
func acceptBattleEvent(b *BattleSession, envelope *battleEnvelope) error {
	if envelope.Sequence <= b.LastSequence {
		return errors.New("战斗事件序号回退")
	}
	if envelope.Sequence != b.LastSequence+1 {
		return errors.New("战斗事件序号跳号")
	}
	if !battleEventKinds[envelope.Kind] || envelope.Data == nil {
		return errors.New("战斗事件类型或数据无效")
	}
	if blob, _ := json.Marshal(envelope.Data); len(blob) > battleEventMaxJSON {
		return errors.New("战斗事件数据过大")
	}
	if !b.BridgeReady && envelope.Kind != "ready" {
		return errors.New("战斗桥尚未握手")
	}
	switch envelope.Kind {
	case "ready":
		if b.BridgeReady {
			return errors.New("战斗桥重复握手")
		}
		if version, _ := envelope.Data["version"].(float64); int(version) != bridgeProtocol {
			return errors.New("战斗桥协议版本不符")
		}
	case "started":
		if b.BridgeStarted {
			return errors.New("战斗已开始")
		}
		if _, ok := envelope.Data["units"]; !ok {
			return errors.New("开局缺少客户端阵容快照")
		}
		if err := validateBattleSnapshot(envelope.Data); err != nil {
			return err
		}
	case "settings":
		if len(envelope.Data) != 1 {
			return errors.New("自动战斗设置无效")
		}
		if _, ok := envelope.Data["auto_battle"].(bool); !ok {
			return errors.New("自动战斗设置无效")
		}
	case "result":
		if !b.BridgeStarted {
			return errors.New("战斗尚未开始")
		}
		if _, err := battleWinnerEIDs(envelope.Data); err != nil {
			return err
		}
		// 桥 rev12 的 notify_finish 只上报 winner_eids 与 finished_task_list
		//（已核对 battle_bridge_script.py）；player_eid/outcome 是服务端可
		// 选兼容字段，存在时校验形态，缺失时由 winner_eids 推导胜负
		//（参考实现 "win" if avatar_id in winners 同语义）。
		player, hasPlayer := "", false
		if rawPlayer, exists := envelope.Data["player_eid"]; exists {
			var ok bool
			player, ok = rawPlayer.(string)
			if !ok || player == "" || len(player) > 128 {
				return errors.New("客户端玩家实体标识无效")
			}
			hasPlayer = true
		}
		if rawOutcome, exists := envelope.Data["outcome"]; exists {
			outcome, ok := rawOutcome.(string)
			if !ok {
				return errors.New("客户端胜负标识无效")
			}
			if outcome != "win" && outcome != "loss" {
				return errors.New("客户端胜负标识无效")
			}
			if hasPlayer {
				wins := false
				for _, eid := range envelope.Data["winner_eids"].([]any) {
					if eid == player {
						wins = true
					}
				}
				if (outcome == "win") != wins {
					return errors.New("客户端胜负与胜方列表不一致")
				}
			}
		}
		if tasks, ok := envelope.Data["finished_task_list"]; ok {
			if _, err := battleTaskList(tasks); err != nil {
				return err
			}
		}
	default:
		if !b.BridgeStarted {
			return errors.New("战斗尚未开始")
		}
		if _, ok := envelope.Data["units"]; ok {
			if err := validateBattleSnapshot(envelope.Data); err != nil {
				return err
			}
		}
	}
	b.LastSequence = envelope.Sequence
	if envelope.Kind == "ready" {
		b.BridgeReady = true
	}
	if envelope.Kind == "started" {
		b.BridgeStarted = true
	}
	return nil
}

// validateBattleSnapshot 对齐参考 snapshot_state：units 上限、eid/hex/role/kind 结构。
func validateBattleSnapshot(data map[string]any) error {
	units, ok := data["units"].([]any)
	if !ok {
		return errors.New("战斗快照缺少单位列表")
	}
	if len(units) > battleRosterMax {
		return errors.New("战斗单位数量超限")
	}
	seen := map[string]bool{}
	for _, item := range units {
		unit, ok := item.(map[string]any)
		if !ok {
			return errors.New("战斗单位结构无效")
		}
		eid, _ := unit["eid"].(string)
		if eid == "" || len(eid) > 128 || seen[eid] {
			return errors.New("战斗单位标识无效")
		}
		seen[eid] = true
		coord, ok := unit["hex"].([]any)
		if !ok || len(coord) != 3 {
			return errors.New("六维坐标无效")
		}
		total := 0.0
		for _, axis := range coord {
			value, ok := axis.(float64)
			if !ok || value != float64(int64(value)) {
				return errors.New("六维坐标非整数")
			}
			total += value
		}
		if total != 0 {
			return errors.New("六维坐标不合法")
		}
		rawRole, hasRole := unit["role"]
		role, roleOK := rawRole.(float64)
		if !hasRole || !roleOK || role < 0 || role != float64(int64(role)) {
			return errors.New("角色编号无效")
		}
		if kind, _ := unit["kind"].(string); kind != "" {
			switch kind {
			case "ally", "script", "support", "enemy", "neutral", "field":
			default:
				return errors.New("单位类别无效")
			}
		}
	}
	return nil
}

func battleWinnerEIDs(data map[string]any) ([]string, error) {
	raw, ok := data["winner_eids"].([]any)
	if !ok || len(raw) > 32 {
		return nil, errors.New("缺少显式胜方列表")
	}
	winners := make([]string, 0, len(raw))
	for _, item := range raw {
		eid, ok := item.(string)
		if !ok || len(eid) == 0 {
			return nil, errors.New("胜方标识无效")
		}
		winners = append(winners, eid)
	}
	return winners, nil
}

func battleTaskList(raw any) ([]int, error) {
	items, ok := raw.([]any)
	if !ok || len(items) > 1024 {
		return nil, errors.New("完成任务列表无效")
	}
	seen := map[int]bool{}
	tasks := make([]int, 0, len(items))
	for _, item := range items {
		value, ok := item.(float64)
		if !ok || value <= 0 || value != float64(int64(value)) || int64(value) > 2147483647 {
			return nil, errors.New("完成任务编号无效")
		}
		task := int(value)
		if seen[task] {
			return nil, errors.New("完成任务重复")
		}
		seen[task] = true
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// battleSettlement 汇总结算产物；pushes 组装最终客户端回包。
type battleSettlement struct {
	outcome             string
	dungeonID           int
	rewards             map[int]int
	unlocked            []string
	firstClear          bool
	finishedTasks       []int
	achievementsChanged bool
	fullBonus           map[string]any
	settledAt           time.Time
	remainingCompletion []Push
	leagueExtra         map[string]any
}

func (settle *battleSettlement) pushes(c *Connection) ([]Push, error) {
	bonus := map[string]any{"__custom_type": "box.box", "materials": map[string]any{}}
	if len(settle.rewards) > 0 {
		materials := map[string]any{}
		for id, amount := range settle.rewards {
			materials[strconv.Itoa(id)] = amount
		}
		bonus = map[string]any{"__custom_type": "box.box", "materials": materials}
	}
	extra := map[string]any{"dungeon_id": settle.dungeonID, "client_authoritative": true, "verified": false}
	for key, value := range settle.leagueExtra {
		extra[key] = value
	}
	if settle.fullBonus != nil {
		var err error
		bonus, err = battleSettlementBoxWire(settle.fullBonus)
		if err != nil {
			return nil, err
		}
	}
	if len(settle.finishedTasks) > 0 {
		extra["task_statistics"] = map[string]any{"tower_finished_task": settle.finishedTasks}
	}
	out := []Push{}
	if settle.firstClear {
		out = append(out, push("Avatar", "client_prop_set", []any{"dungeon_mgr", settle.dungeonID,
			map[string]any{"dungeon_id": settle.dungeonID, "finished": 1}}))
	}
	if len(settle.rewards) > 0 {
		out = append(out, materialManagerPush(c))
	}
	if settle.fullBonus != nil {
		out = append(out, materialManagerPush(c), cardMgrPush(c), runePush(c), knowledgePush(c), powerPush(c))
		now := settle.settledAt
		if now.IsZero() {
			now = time.Now()
		}
		out = append(out, activityPushes(c, now)...)
		out = append(out, remainingGameplayPushes(c.SelectedAvatarUnsafe().Progress)...)
		out = append(out, settle.remainingCompletion...)
		out = append(out, activitySettlementPushes(c, now)...)
	}
	if len(settle.unlocked) > 0 {
		unlocks := map[string]any{}
		for _, av := range c.identity.Avatars {
			if av.Hostnum != c.hostnum {
				continue
			}
			for system, version := range av.Progress.UnlockSystems {
				unlocks[system] = version
			}
		}
		out = append(out, push("Avatar", "client_prop_changed", []any{"unlock_systems", unlocks}))
	}
	if settle.achievementsChanged {
		p := c.SelectedAvatarUnsafe().Progress
		out = append(out, push("Avatar", "client_prop_changed", []any{"achves", achievementProperties(p)}), push("Avatar", "client_prop_changed", []any{"achv_value", achievementPoints(p)}))
	}
	return append(out, push("Avatar", "battle_result", settle.outcome == "win", bonus, map[string]any{}, extra)), nil
}

// settleClientBattle 在 result 事件后结算：胜负以显式 winner_eids 为准，
// 首通推进 new_task 与系统解锁并按主线首通表发奖；重复通关只记录进度。
func settleClientBattle(p *Progress, b *BattleSession, envelope *battleEnvelope, avatarEID string, now time.Time) *battleSettlement {
	winners, _ := battleWinnerEIDs(envelope.Data)
	playerEID, _ := envelope.Data["player_eid"].(string)
	if playerEID == "" {
		playerEID = avatarEID
	}
	outcome, _ := envelope.Data["outcome"].(string)
	if outcome != "win" && outcome != "loss" {
		outcome = "loss"
		for _, eid := range winners {
			if eid == playerEID {
				outcome = "win"
			}
		}
	}
	b.Outcome = outcome
	b.ClientPlayerEID = playerEID
	b.WinnerEIDs = winners
	b.Status = "结束"
	if tasks, err := battleTaskList(envelope.Data["finished_task_list"]); err == nil && tasks != nil {
		b.FinishedTaskList = tasks
	}
	b.Finished = true
	settle := &battleSettlement{outcome: outcome, dungeonID: b.DungeonID, finishedTasks: append([]int(nil), b.FinishedTaskList...), settledAt: now}
	if outcome != "win" {
		return settle
	}
	settle.achievementsChanged = advanceAchievementEvent(p, 2, b.DungeonID, now)
	advanceBasicRewardEvent(p, 4, []any{dungeonCatalog[b.DungeonID].Type}, 1, now)
	cleared := false
	for _, id := range p.ClearedDungeons {
		if id == b.DungeonID {
			cleared = true
		}
	}
	if !cleared {
		p.ClearedDungeons = append(p.ClearedDungeons, b.DungeonID)
		ActivateGuideTriggers(p)
		settle.unlocked = p.advanceUnlocks(b.DungeonID)
		settle.firstClear = true
		if !b.RewardGranted {
			if rewards := androidMainlineRewards[b.DungeonID]; len(rewards) > 0 {
				grantMaterials(p, rewards)
				settle.rewards = rewards
				b.RewardGranted = true
			}
		}
	}
	return settle
}

func grantMaterials(p *Progress, rewards map[int]int) {
	if p.Materials == nil {
		p.Materials = map[int]Material{}
	}
	for id, amount := range rewards {
		material := p.Materials[id]
		material.ID = id
		material.Count += int64(amount)
		material.Total += int64(amount)
		p.Materials[id] = material
	}
}

// materialManagerPush 组装全量材料余额推送（结算/发奖共用）。
func materialManagerPush(c *Connection) Push {
	props := map[string]any{}
	for _, av := range c.identity.Avatars {
		if av.Hostnum != c.hostnum {
			continue
		}
		for id, m := range av.Progress.Materials {
			props[strconv.Itoa(id)] = map[string]any{"material_id": m.ID, "count": m.Count, "total": m.Total}
		}
	}
	return push("Avatar", "client_prop_changed", []any{"material_mgr", props})
}

// setBattleSpeed 持久化 1/1.75 倍速；仅战斗中接受，并同步给客户端引擎。
func (s *Service) setBattleSpeed(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 1 {
		return nil, errors.New("战斗速度需要玩家状态及数值")
	}
	var speed float64
	if json.Unmarshal(args[0], &speed) != nil || (speed != 1 && speed != 1.75) {
		return nil, errors.New("战斗速度仅支持 1 或 1.75")
	}
	var active bool
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if p.Battle == nil || p.Battle.Finished {
			return errors.New("没有进行中的战斗")
		}
		prefs := p.battlePreferences()
		prefs.BattleSpeed = speed
		p.BattlePreferences = prefs
		p.Battle.BattleSpeed = speed
		active = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, nil
	}
	return []Push{battleSync("set_battle_speed", speed)}, nil
}

// clientNeedRecoverBattle 对齐参考语义：已有结果按原结果补发结算，
// 未完成战斗从准备阶段重新开始，不恢复中途 HP/AP。
func (s *Service) clientNeedRecoverBattle(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("战斗恢复需要玩家状态")
	}
	if handled, pushes, err := s.humanResume(ctx, c); handled {
		return pushes, err
	}
	if handled, pushes, err := s.resumeNativeSolo(ctx, c); handled {
		return pushes, err
	}
	if b := c.SelectedAvatarUnsafe().Progress.Battle; b != nil && !b.Finished && c.ordinaryObservation != nil && b.UUID == c.ordinaryObservation.UUID && c.ordinaryObservation.BridgeStarted && s.Now().UnixMilli() >= c.ordinaryObservationAt && s.Now().UnixMilli()-c.ordinaryObservationAt < 60000 {
		// 原生net_delay超时会在TCP仍连通时请求恢复；观察流仍在同一连接，
		// 不清空本地第三波、不更换UUID。真正断线后的新连接没有此观察缓存。
		log.Printf("普通战斗同连接延迟恢复保留进度 uid=%d dungeon=%d uuid=%s seq=%d", c.SelectedAvatarUnsafe().UID, b.DungeonID, b.UUID, c.ordinaryObservation.LastSequence)
		c.nextOrdinaryResultRetry = 0
		return s.retryOrdinaryResult(ctx, c)
	}
	var pushes []Push
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		b := p.Battle
		if b == nil {
			return errors.New("没有可恢复的战斗")
		}
		if b.Finished && b.Outcome != "" {
			// 只读取收据。提交前不能用c旧快照组属性推送，也不能重新发奖。
			return nil
		}
		// 未完成：标中断并重开准备阶段（新战斗 UUID）。
		b.RecoveredFrom = b.UUID
		b.UUID = newBattleUUID(s.Now())
		if err := migrateLeagueProtectAttempt(p, b, b.RecoveredFrom); err != nil {
			return err
		}
		if frozen := p.Social.Assist.Frozen; frozen != nil && frozen.BattleUUID == b.RecoveredFrom {
			frozen.BattleUUID = b.UUID
			p.Social.Assist.LastBattle = b.UUID
		}
		if p.AsyncPvp.Match != nil && p.AsyncPvp.Match.Receipt == nil && b.DungeonID == asyncRule().DungeonID {
			p.AsyncPvp.Match.UUID = b.UUID
			p.AsyncPvp.Match.ResultSequence = 0
			p.AsyncPvp.Match.ResultDigest = ""
			p.AsyncPvp.Match.Rewards = nil
		}
		if p.SyncPvpMatch != nil && p.SyncPvpMatch.BattleUUID != "" && b.DungeonID == syncPvpRule().DungeonID {
			if p.SyncPvpMatch.Status != syncPvpMatchReady {
				return errors.New("同步PVP匹配不可恢复")
			}
			p.SyncPvpMatch.BattleUUID = b.UUID
		}
		b.Status = "准备"
		b.BridgeReady, b.BridgeStarted, b.Finished = false, false, false
		b.LastSequence = 0
		b.Outcome, b.WinnerEIDs, b.FinishedTaskList = "", nil, nil
		b.ClientPlayerEID = ""
		b.EventLog = nil
		b.Entities, b.Triggers = nil, nil
		b.Bootstrap, b.DueAt = "", 0
		b.Loaded, b.Started = false, false
		b.ResultCounted, b.RewardGranted = false, false
		prefs := p.battlePreferences()
		pushes = append(pushes,
			push("Avatar", "on_query_hotfix_success", clientExtensionsScript(), bridgeHotfixIndex),
			battleSync("revival_set_battle_preferences", prefs.BattleSpeed, prefs.AutoBattle))
		log.Printf("战斗恢复重开准备阶段 dungeon=%d 新uuid=%s", b.DungeonID, b.UUID)
		return nil
	})
	if err == nil {
		p := c.SelectedAvatarUnsafe().Progress
		if b := p.Battle; b != nil && b.Finished && b.Outcome != "" {
			if p.AsyncPvp.Match != nil && p.AsyncPvp.Match.UUID == b.UUID && p.AsyncPvp.Match.Receipt != nil {
				return asyncResultPushes(c, *p.AsyncPvp.Match.Receipt, p.AsyncPvp.Match.Rewards), nil
			}
			if p.SyncPvpMatch != nil && p.SyncPvpMatch.BattleUUID == b.UUID {
				if receipt, ok := p.SyncPvpSettlements[b.UUID]; ok {
					return syncPvpResultPushes(c, receipt.Result), nil
				}
			}
			settle := &battleSettlement{outcome: b.Outcome, dungeonID: b.DungeonID,
				fullBonus: b.SettlementBox, finishedTasks: append([]int(nil), b.FinishedTaskList...), settledAt: s.Now()}
			settle.remainingCompletion = leagueProtectCompletionPushes(p, b)
			if extra := leagueProtectSettlementExtra(b); extra != nil {
				settle.leagueExtra = map[string]any{"league_activity_battle_result": extra}
			}
			// 没有完整盒的历史记录只保留当时已有固定奖励显示，不补造随机资产。
			if b.SettlementBox == nil && b.RewardGranted {
				settle.rewards = androidMainlineRewards[b.DungeonID]
			}
			return settle.pushes(c)
		}
		if p.Battle != nil && !p.Battle.Finished {
			c.battleStartSent = false
			pushes = s.reopenOrdinaryBattlePushes(c, p.Battle)
		}
		if p.Battle != nil && !p.Battle.Finished && p.SyncPvpMatch != nil && p.Battle.UUID == p.SyncPvpMatch.BattleUUID {
			pushes = s.syncPVPLoadPushes(c, p.SyncPvpMatch, 0, false)
		}
		if p.Battle != nil && !p.Battle.Finished && p.AsyncPvp.Match != nil && p.Battle.UUID == p.AsyncPvp.Match.UUID {
			pushes = s.asyncPvpLoadPushes(c, p.AsyncPvp.Match)
		}
	}
	return pushes, err
}

// 断线中断重开需要完整原生加载链；只重发偏好不会创建战斗实体。
func (s *Service) reopenOrdinaryBattlePushes(c *Connection, b *BattleSession) []Push {
	c.ordinaryPrepareUUID = b.UUID
	av := c.SelectedAvatarUnsafe()
	player := ObjectID(hexOf(av.OID))
	layout := savedBattleLayout(av.Progress, b.DungeonID)
	if b.Layout != nil {
		layout = cloneBattleLayout(*b.Layout)
	}
	extra := activityBattleExtra(b.ActivityContext, b.Extra)
	if b.RecoveredFrom != "" {
		copyExtra := make(map[string]any, len(extra)+1)
		for key, value := range extra {
			copyExtra[key] = value
		}
		// 只授权客户端清理此次恢复所替代的旧对象，不触发通关或奖励。
		copyExtra["hs_recover_previous_uuid"] = b.RecoveredFrom
		extra = copyExtra
	}
	out := []Push{push("Avatar", "on_query_hotfix_success", clientExtensionsScript(), bridgeHotfixIndex), push("Avatar", "start_server_battle_ok", dungeonCatalog[b.DungeonID].Type, b.DungeonID, ObjectID(b.UUID), extra)}
	if len(layout.Fighting) > 0 || len(layout.Support) > 0 {
		out = append(out, battleSync("set_last_fighting_cards", lastFightingWire(player, layout.Fighting, layout.Support)))
	}
	out = append(out, battleSync("prepare", []any{player}, b.BattleID, b.Seed, b.DungeonID))
	prefs := av.Progress.battlePreferences()
	return append(out, battleSync("revival_set_battle_preferences", prefs.BattleSpeed, prefs.AutoBattle), dungeonEntryPush(av.Progress, b.DungeonID))
}

// lastFightingWire 组装 set_last_fighting_cards 载荷：ObjectId 与 hex 双键
// （客户端 get_default_layout_cards 按 master_eid 直查，bytes 键为主路径）。
func lastFightingWire(player ObjectID, fighting, support []string) mobileproto.Map {
	fight := battleSlotWire(fighting)
	bench := battleSlotWire(support)
	nested := mobileproto.Map{
		{Key: ObjectID(player), Value: bench},
		{Key: string(player), Value: bench},
	}
	return mobileproto.Map{
		{Key: ObjectID(player), Value: fight},
		{Key: string(player), Value: fight},
		{Key: "support", Value: nested},
	}
}

// cardListWire 组装 add_fighting_cards 的卡字典列表（card.card_list 载荷）。
func cardListWire(p *Progress, uuids []string) []any {
	mgr := cardMgrPropertiesWithRunes(p.Cards, p.Runes)
	out := make([]any, 0, len(uuids))
	for _, uuid := range uuids {
		if card, ok := mgr[uuid]; ok {
			out = append(out, card)
		}
	}
	return out
}
