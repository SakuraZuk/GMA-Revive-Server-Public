package game

import (
	"crypto/sha256"
	"embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
)

//go:embed sync_pvp_ai_catalog.json
var syncPvpAICatalogFS embed.FS

type syncPvpScoreRule struct {
	ID               int     `json:"_id"`
	MinScore         int     `json:"min_score"`
	MaxScore         int     `json:"max_score"`
	RobotGroupID     int     `json:"robot_group_id"`
	LineupNum        int     `json:"lineup_num"`
	K                int     `json:"k"`
	LoseProtectRatio float64 `json:"lose_protect_ratio"`
	DungeonBattleID  int     `json:"dungeon_battle_id"`
	DivisionBonus    int     `json:"division_bonus_id"`
	LoseMatchTime    []int   `json:"lose_match_time"`
}

type syncPvpRobot struct {
	ID           int    `json:"_id"`
	Nickname     string `json:"nickname"`
	RobotLevel   int    `json:"robot_level"`
	RobotSex     int    `json:"robot_sex"`
	AssistCard   int    `json:"assist_card"`
	SyncPvpCards []int  `json:"sync_pvp_cards"`
	SyncPvpScore []int  `json:"sync_pvp_score"`
	GroupIDs     []int  `json:"sync_pvp_group_id"`
	DefenceCards []int  `json:"defence_cards"`
	AsyncScore   int    `json:"asyn_pvp_score"`
}

// syncPvpRuleT 对应 Android sync_pvp_rule 表（单行）。
type syncPvpRuleT struct {
	ActivityID         int     `json:"activity_id"`
	SeasonOriginalDate int64   `json:"season_original_date"`
	SeasonDuration     int64   `json:"season_duration"`
	InitScore          int     `json:"sync_pvp_init_score"`
	DungeonID          int     `json:"dungeon_id"`
	RecordNum          int     `json:"record_num"`
	HighSectionScore   int     `json:"high_section_score"`
	LevelRate          float64 `json:"level_rate"`
	MinMatchTime       int     `json:"min_match_time"`
	MaxMatchScore      []int   `json:"max_match_score"`
}

// syncPvpRobotCard 对应 robot_card_config 行：机器人卡物化模板。
type syncPvpRobotCard struct {
	ID         int   `json:"_id"`
	CardID     int   `json:"card_id"`
	Level      int   `json:"card_level"`
	Grade      int   `json:"card_grade"`
	Enhance    int   `json:"card_enhance_count"`
	SkillLevel int   `json:"card_skill_level"`
	Runes      []int `json:"card_runes"`
	DressID    *int  `json:"card_dress_id"`
}

// SyncPvpMatch 是服务器锁定的同步 PVP 匹配快照。机器人选定后不会因重试改变。
type SyncPvpMatch struct {
	BattleUUID string       `json:"battle_uuid"`
	Status     string       `json:"status"`
	CreatedAt  int64        `json:"created_at"`
	Score      int          `json:"score"`
	RuleID     int          `json:"rule_id"`
	Robot      syncPvpRobot `json:"robot"`
	// RobotScore 是按匹配种子从机器人分段区间取的稳定敌分；
	// 结算的 ELO 对手分一律用它，不信任客户端上报。
	RobotScore int `json:"robot_score,omitempty"`
	// OwnFighting/OwnSupport 是 send_pvp_cards 校验后服务器锁定的己方阵容。
	OwnFighting []string                `json:"own_fighting,omitempty"`
	OwnSupport  []string                `json:"own_support,omitempty"`
	PresetIDs   []string                `json:"preset_ids,omitempty"`
	Presets     map[string]PresetRecord `json:"presets,omitempty"`
	// Seed 与开战 prepare 共用；机器人对战下双方演算由客户端完成。
	Seed int64 `json:"seed,omitempty"`
	// Extra 仅保存客户端版本相关的诊断和活动上下文；机器人快照仍由服务端生成。
	Extra      map[string]any   `json:"extra,omitempty"`
	OwnProfile SocialProfile    `json:"own_profile,omitempty"`
	OwnCards   []map[string]any `json:"own_cards,omitempty"`
}

type SyncPvpResult struct {
	DeltaScore      int  `json:"delta_score"`
	DeltaCoin       int  `json:"delta_coin"`
	WinContinuously bool `json:"win_continuesly"`
	DivisionUpdated int  `json:"division_updated"`
	OwnScore        int  `json:"own_score"`
	EnemyScore      int  `json:"enemy_score"`
}

type SyncPvpReceipt struct {
	Outcome string        `json:"outcome"`
	Result  SyncPvpResult `json:"result"`
}

type SyncPvpRecord struct {
	BattleUUID string           `json:"battle_uuid"`
	Outcome    string           `json:"outcome"`
	Score      int              `json:"score"`
	CreatedAt  int64            `json:"created_at"`
	OwnInfo    SocialProfile    `json:"own_info"`
	EnemyInfo  SocialProfile    `json:"enemy_info"`
	OwnCards   []map[string]any `json:"own_cards"`
	OwnTeam    []string         `json:"own_team"`
	EnemyCards []map[string]any `json:"enemy_cards"`
	EnemyTeam  []string         `json:"enemy_team"`
}

const (
	syncPvpMatchWaiting = "waiting"
	syncPvpMatchReady   = "ready"
	syncPvpMatchSettled = "settled"
	syncPvpMatchAborted = "aborted"
)

type syncPvpAICatalog struct {
	Tables struct {
		ScoreRules  map[string]syncPvpScoreRule `json:"sync_pvp_score_rule"`
		Robots      map[string]syncPvpRobot     `json:"robot_config"`
		Rule        map[string]syncPvpRuleT     `json:"sync_pvp_rule"`
		RobotCards  map[string]syncPvpRobotCard `json:"robot_card_config"`
		GhostScores map[string][]int            `json:"sync_pvp_ghost_scores"`
		RobotRunes  map[string]robotRuneConfig  `json:"robot_rune_config"`
		CardSkills  map[string]robotCardSkills  `json:"robot_card_skills"`
	} `json:"tables"`
}
type robotRuneConfig struct {
	ID       int   `json:"_id"`
	Suits    []int `json:"suit"`
	Position int   `json:"position"`
	Star     int   `json:"star"`
	Level    int   `json:"level"`
	Base     []int `json:"base_attrs"`
	Extra    []int `json:"extra_attrs"`
	Lib      []int `json:"extra_attrs_lib"`
	Count    int   `json:"extra_attrs_count"`
}
type robotCardSkills struct {
	Skills       []int `json:"skills"`
	Support      []int `json:"support_skills"`
	DefaultDress int   `json:"default_dress"`
	Rarity       int   `json:"rarity"`
}

var syncPvpCatalog = func() syncPvpAICatalog {
	var value syncPvpAICatalog
	raw, err := syncPvpAICatalogFS.ReadFile("sync_pvp_ai_catalog.json")
	if err != nil || json.Unmarshal(raw, &value) != nil || len(value.Tables.ScoreRules) == 0 || len(value.Tables.Robots) == 0 {
		panic("同步PVP Android机器人目录无效")
	}
	return value
}()

// syncPvpRule 返回唯一规则行（缺表时 panic 已在加载期拦截）。
func syncPvpRule() syncPvpRuleT {
	rule, exists := syncPvpCatalog.Tables.Rule["1"]
	if !exists || rule.InitScore <= 0 || rule.SeasonDuration <= 0 {
		panic("Android同步竞技主规则缺失")
	}
	return rule
}

// syncPvpSeasonID 按 season_original_date 与 season_duration 推算赛季编号。
func syncPvpSeasonID(now time.Time) int {
	rule := syncPvpRule()
	if rule.SeasonDuration <= 0 || rule.SeasonOriginalDate <= 0 {
		return 1
	}
	elapsed := now.Unix() - rule.SeasonOriginalDate
	if elapsed < 0 {
		return 1
	}
	return int(elapsed/rule.SeasonDuration) + 1
}

// robotMaterializedScore 从机器人分段区间按种子取稳定敌分。
func robotMaterializedScore(robot syncPvpRobot, seed string) int {
	if len(robot.SyncPvpScore) == 2 {
		lower, upper := robot.SyncPvpScore[0], robot.SyncPvpScore[1]
		if upper < lower {
			lower, upper = upper, lower
		}
		span := upper - lower + 1
		h := sha256.Sum256([]byte("score:" + seed))
		return lower + int(binary.BigEndian.Uint32(h[:4])%uint32(span))
	}
	if len(robot.SyncPvpScore) == 1 {
		return robot.SyncPvpScore[0]
	}
	return syncPvpRule().InitScore
}

// robotCardUUID 为机器人卡模板派生稳定 24 位 hex（E 前缀与玩家卡区分）。
func robotCardUUID(templateID int) string {
	return fmt.Sprintf("e%023d", templateID)
}

// robotAvatarEID 为机器人派生稳定 12 字节 eid。
func robotAvatarEID(robotID int) string {
	return fmt.Sprintf("f%023d", robotID)
}

// robotCardWire 按参考 owned_card_mgr 卡 wire 形态物化机器人卡；
// 契印模板位（card_runes）暂不展开为完整契印，保留数值位待后续接入。
func robotCardWire(template syncPvpRobotCard) map[string]any {
	card := map[string]any{
		"uuid": ObjectID(robotCardUUID(template.ID)), "card_id": template.CardID,
		"level": template.Level, "exp": 0, "grade": template.Grade,
		"awakened": func() int {
			if template.Enhance > 0 {
				return 1
			}
			return 0
		}(), "dress": 0, "lock": 0, "time": 0, "grow_materials": map[string]any{},
		"enhance_count": template.Enhance, "enhance_ids": []any{},
		"skill_enhance_count": template.SkillLevel - 1, "support_skill_level": 1,
		"embed_runes": map[string]any{},
	}
	if template.DressID != nil {
		card["dress"] = *template.DressID
	}
	skills := syncPvpCatalog.Tables.CardSkills[strconv.Itoa(template.CardID)]
	list := []any{}
	for _, id := range skills.Skills {
		list = append(list, map[string]any{"skill_id": id, "level": template.SkillLevel, "enhance_level": 0})
	}
	card["skill_mgr"] = list
	card["skill_enhance_count"] = 0
	if template.DressID == nil {
		card["dress"] = skills.DefaultDress
	}
	if skills.Rarity == 1 {
		card["awakened"] = 1
	}
	embed := map[string]any{}
	for index, id := range template.Runes {
		config, exists := syncPvpCatalog.Tables.RobotRunes[strconv.Itoa(id)]
		if !exists || len(config.Suits) == 0 {
			continue
		}
		h := sha256.Sum256([]byte(fmt.Sprintf("robot-rune:%d:%d", template.ID, index)))
		suit := config.Suits[int(h[0])%len(config.Suits)]
		r := Rune{UUID: fmt.Sprintf("d%015d%08d", template.ID, index), CardUUID: robotCardUUID(template.ID), Star: config.Star, Position: config.Position, Level: config.Level, Suit: suit, BaseAttrs: append([]int{}, config.Base...), ExtraAttrs: append([]int{}, config.Extra...), ExtraAttrsLib: append([]int{}, config.Lib...), ExtraAttrsCount: config.Count, ExtraAttrsFactor: map[int]int{}}
		if r.ExtraAttrsCount < len(r.ExtraAttrs) {
			r.ExtraAttrsCount = len(r.ExtraAttrs)
		}
		embed[strconv.Itoa(r.Position)] = runeProperties(r)
	}
	card["embed_runes"] = embed
	return card
}

// robotCardsWire 物化机器人出战卡列表。
func robotCardsWire(robot syncPvpRobot) []any {
	out := make([]any, 0, len(robot.SyncPvpCards))
	for _, templateID := range robot.SyncPvpCards {
		if template, ok := syncPvpCatalog.Tables.RobotCards[strconv.Itoa(templateID)]; ok {
			out = append(out, robotCardWire(template))
		}
	}
	return out
}

func syncPvpScoreRuleFor(score int) (syncPvpScoreRule, error) {
	var rules []syncPvpScoreRule
	for _, row := range syncPvpCatalog.Tables.ScoreRules {
		if score >= row.MinScore && score <= row.MaxScore {
			rules = append(rules, row)
		}
	}
	if len(rules) == 0 {
		return syncPvpScoreRule{}, errors.New("同步PVP分段不存在")
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	return rules[0], nil
}

// selectSyncPVPRobot 按当前分段和稳定种子选择唯一机器人；同一玩家/匹配 UUID
// 重试会得到同一快照，避免无人匹配时重复切换对手。
func selectSyncPVPRobot(score int, seed string) (syncPvpRobot, syncPvpScoreRule, error) {
	rule, err := syncPvpScoreRuleFor(score)
	if err != nil {
		return syncPvpRobot{}, rule, err
	}
	rows := make([]syncPvpRobot, 0)
	for _, robot := range syncPvpCatalog.Tables.Robots {
		for _, group := range robot.GroupIDs {
			if group == rule.RobotGroupID {
				if len(robot.SyncPvpCards) > 0 {
					rows = append(rows, robot)
				}
				break
			}
		}
	}
	if len(rows) == 0 {
		return syncPvpRobot{}, rule, errors.New("同步PVP分段没有可用机器人")
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	h := sha256.Sum256([]byte(seed))
	index := binary.BigEndian.Uint64(h[:8]) % uint64(len(rows))
	return rows[index], rule, nil
}

func syncPvpBattleUUID(seed string, now time.Time) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("sync-pvp:%s:%d", seed, now.UnixNano())))
	return fmt.Sprintf("%x", h[:12])
}

// prepareSyncPvpMatch 是 AI 兜底入口。动态匹配 RPC 的参数尚未完成取证，
// 因而它只接受服务器内部生成的 seed，不能由客户端直接覆盖对手。
func prepareSyncPvpMatch(p *Progress, seed string, now time.Time) (*SyncPvpMatch, error) {
	if p == nil {
		return nil, errors.New("同步PVP进度为空")
	}
	if p.SyncPvpScore < 1000 {
		p.SyncPvpScore = 1000
	}
	if err := ensureSyncPvpPeriod(p, now); err != nil {
		return nil, err
	}
	if p.SyncPvpMatch != nil {
		// 已就绪匹配复用（含断线重试）；结算/中止后才能开新匹配。
		if p.SyncPvpMatch.Status == syncPvpMatchSettled || p.SyncPvpMatch.Status == syncPvpMatchAborted {
			// 落入下方新建分支。
		} else {
			return p.SyncPvpMatch, nil
		}
	}
	robot, rule, err := selectSyncPVPRobot(p.SyncPvpScore, seed)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256([]byte("pvp-seed:" + seed))
	match := &SyncPvpMatch{BattleUUID: syncPvpBattleUUID(seed, now), Status: syncPvpMatchReady,
		CreatedAt: now.Unix(), Score: p.SyncPvpScore, RuleID: rule.ID, Robot: robot,
		RobotScore: robotMaterializedScore(robot, seed),
		Seed:       int64(binary.BigEndian.Uint32(h[:4]) & 0x7fffffff)}
	p.SyncPvpMatch = match
	match.PresetIDs = append([]string{}, p.SyncPvpPresetIDs...)
	match.Presets = map[string]PresetRecord{}
	if len(match.PresetIDs) == 0 && len(p.Lineup) > 0 {
		id := newBattleUUID(now)
		match.PresetIDs = []string{id}
		match.Presets[id] = PresetRecord{PresetID: id, Cards: BattleLayout{Fighting: append([]string{}, p.Lineup...)}}
	}
	for _, id := range match.PresetIDs {
		if row, ok := p.PresetCardsRecord[id]; ok {
			row.Cards = cloneBattleLayout(row.Cards)
			match.Presets[id] = row
		}
	}
	if p.SyncPvpSettlements == nil {
		p.SyncPvpSettlements = map[string]SyncPvpReceipt{}
	}
	return match, nil
}

func syncPvpRound(value float64) int {
	if value >= 0 {
		return int(math.Floor(value + 0.5))
	}
	return -int(math.Floor(-value + 0.5))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// settleSyncPvpResult 以观察桥的显式胜负为唯一结算来源：
// 原生六参数为服务端下行，数值全部由本地结算规则推导：
//   - 胜负：BattleSession（同一 battle_uuid 的 __battle_event__ result）；
//   - 对手分：匹配时锁定的机器人分快照，不信任客户端上报；
//   - delta_score：本地ELO策略使用Android分段K与保护倍率（胜利至少+1）；
//   - delta_coin：本地无奖励币规则，固定 0（参考实现同值）；
//   - win_continuesly：原生语义是"双倍积分奖励已应用"，本地无该奖励固定 false；
//   - division_updated：新旧分段编号差。
//
// 同一 UUID 的重试只返回旧结果（UUID 收据幂等）。
func settleSyncPvpResult(p *Progress, reportedDeltaScore int, now time.Time) (SyncPvpResult, error) {
	if p == nil || p.SyncPvpMatch == nil {
		return SyncPvpResult{}, errors.New("没有可结算的同步PVP匹配")
	}
	match := p.SyncPvpMatch
	if match.BattleUUID == "" || (match.Status != syncPvpMatchReady && match.Status != syncPvpMatchSettled) {
		return SyncPvpResult{}, errors.New("同步PVP匹配状态不可结算")
	}
	if receipt, ok := p.SyncPvpSettlements[match.BattleUUID]; ok {
		return receipt.Result, nil
	}
	// 结算前提：同一 UUID 的客户端权威战斗已由观察桥给出显式胜负。
	if p.Battle == nil || p.Battle.UUID != match.BattleUUID || !p.Battle.BridgeReady || !p.Battle.BridgeStarted || !p.Battle.Finished ||
		(p.Battle.Outcome != "win" && p.Battle.Outcome != "loss") {
		return SyncPvpResult{}, errors.New("同步PVP结算需要观察桥的显式战斗结果")
	}
	win := p.Battle.Outcome == "win"
	enemyScore := match.RobotScore
	if enemyScore <= 0 || p.SyncPvpScore != match.Score {
		return SyncPvpResult{}, errors.New("同步PVP锁定积分不一致")
	}
	rule, err := syncPvpScoreRuleFor(p.SyncPvpScore)
	if err != nil {
		return SyncPvpResult{}, err
	}
	exponent := math.Max(-100, math.Min(100, float64(enemyScore-p.SyncPvpScore)/400))
	expected := 1 / (1 + math.Pow(10, exponent))
	winScore := 0.0
	if win {
		winScore = 1
	}
	change := float64(rule.K) * (winScore - expected)
	if !win {
		change *= rule.LoseProtectRatio
	}
	delta := syncPvpRound(change)
	if win && delta < 1 {
		delta = 1
	}
	if reportedDeltaScore != 0 && reportedDeltaScore != delta {
		return SyncPvpResult{}, errors.New("同步PVP积分变化与Android规则不一致")
	}
	oldScore := p.SyncPvpScore
	p.SyncPvpScore = maxInt(syncPvpRule().InitScore, oldScore+delta)
	p.SyncPvpHighestScore = maxInt(p.SyncPvpHighestScore, p.SyncPvpScore)
	newRule, err := syncPvpScoreRuleFor(p.SyncPvpScore)
	if err != nil {
		return SyncPvpResult{}, err
	}
	divisionUpdated := newRule.ID - rule.ID
	if win {
		p.SyncPvpWinStreak++
		p.SyncPvpWeeklyWins++
		p.SyncPvpLoseTimes = 0
		recordAchievementCompetitiveWin(p, 5, now)
	} else {
		p.SyncPvpWinStreak = 0
		p.SyncPvpLoseTimes++
	}
	result := SyncPvpResult{DeltaScore: p.SyncPvpScore - oldScore, DeltaCoin: 0,
		WinContinuously: false, DivisionUpdated: divisionUpdated,
		OwnScore: p.SyncPvpScore, EnemyScore: enemyScore}
	outcome := "loss"
	if win {
		outcome = "win"
	}
	match.Status = syncPvpMatchSettled
	if p.SyncPvpSettlements == nil {
		p.SyncPvpSettlements = map[string]SyncPvpReceipt{}
	}
	p.SyncPvpSettlements[match.BattleUUID] = SyncPvpReceipt{Outcome: outcome, Result: result}
	p.SyncPvpRecords = append(p.SyncPvpRecords, SyncPvpRecord{BattleUUID: match.BattleUUID, Outcome: outcome, Score: p.SyncPvpScore, CreatedAt: now.Unix()})
	record := &p.SyncPvpRecords[len(p.SyncPvpRecords)-1]
	record.OwnInfo = match.OwnProfile
	record.OwnInfo.Score = match.Score
	defaults := DefaultAvatarInfo(match.Robot.Nickname)
	record.EnemyInfo = SocialProfile{EID: robotAvatarEID(match.Robot.ID), Nickname: match.Robot.Nickname, Level: match.Robot.RobotLevel, Hostnum: match.OwnProfile.Hostnum, HeadID: defaults.HeadID, HeadBoxID: defaults.HeadBoxID, Score: match.RobotScore}
	record.OwnTeam = append(append([]string{}, match.OwnFighting...), match.OwnSupport...)
	record.OwnCards = match.OwnCards
	for _, item := range robotCardsWire(match.Robot) {
		record.EnemyCards = append(record.EnemyCards, item.(map[string]any))
	}
	for _, id := range match.Robot.SyncPvpCards {
		record.EnemyTeam = append(record.EnemyTeam, robotCardUUID(id))
	}
	recordNum := syncPvpRule().RecordNum
	if recordNum <= 0 {
		recordNum = 10
	}
	if len(p.SyncPvpRecords) > recordNum {
		p.SyncPvpRecords = p.SyncPvpRecords[len(p.SyncPvpRecords)-recordNum:]
	}
	return result, nil
}
