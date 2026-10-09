package game

import (
	"context"
	"encoding/json"
	"errors"
	"hs-server/internal/mobileproto"
	"sort"
	"strconv"
	"time"
)

// 同步 PVP 的进入、阵容锁定、加载和查询契约。
// 对手始终由服务端依据 Android 目录选择；客户端不能提交机器人资料覆盖快照。
// 全流程：start_sync_pvp_match（机器人匹配+select_pvp_cards 下发双方快照）
// → send_pvp_cards（锁定己方阵容并下发加载/prepare）→ 原生加载与观察桥
// → 桥 result 显式胜负 → sync_pvp_battle_result（仅服务端下行）。
func (s *Service) syncPvpRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("同步PVP需要玩家状态")
	}
	switch method {
	case "start_sync_pvp_match":
		return s.startSyncPVP(ctx, c, args)
	case "send_pvp_cards":
		return s.sendSyncPVPCards(ctx, c, args)
	case "pvp_load_complete":
		return s.syncPVPLoaded(ctx, c, args)
	case "cancel_sync_pvp_match":
		return s.cancelSyncPVP(ctx, c, args)
	case "query_sync_pvp_rank", "query_sync_pvp_world_rank", "query_sync_pvp_local_rank_list", "query_sync_pvp_world_rank_list":
		return s.querySyncPVPRank(ctx, c, method, args)
	case "query_sync_pvp_season_id":
		return s.querySyncPVPSeason(ctx, c, args)
	default:
		return nil, errors.New("未知同步PVP请求")
	}
}

func callbackArg(args []json.RawMessage) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	var id int
	if json.Unmarshal(args[0], &id) != nil || id < 0 {
		return 0, false
	}
	return id, true
}

// startSyncPVP 匹配（AI 兜底即时机器人）并按参考契约下发
// select_pvp_cards(infos, lineup_num)：双方 avatar_info、预设卡组与卡快照，
// 客户端据此进入选卡界面并物化双方单位。
func (s *Service) startSyncPVP(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if err := s.refreshPvpAwards(ctx, c); err != nil {
		return nil, err
	}
	cb, ok := callbackArg(args)
	if !ok || len(args) > 2 {
		return nil, errors.New("同步PVP匹配参数无效")
	}
	if room := c.SelectedAvatarUnsafe().Progress.Social.HumanRoom; humanRoomActive(room) {
		return []Push{Callback(cb, []any{RetSuccess}), humanSelection(room)}, nil
	}
	var match *SyncPvpMatch
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if p.Battle != nil && !p.Battle.Finished {
			return errors.New("已有进行中的战斗")
		}
		if err := ensureSyncPvpPeriod(p, s.Now()); err != nil {
			return err
		}
		if p.SyncPvpMatch != nil && (p.SyncPvpMatch.Status == syncPvpMatchWaiting || p.SyncPvpMatch.Status == syncPvpMatchReady) {
			match = p.SyncPvpMatch
			return nil
		}
		av := c.SelectedAvatarUnsafe()
		av.Progress = *p
		match = &SyncPvpMatch{BattleUUID: newBattleUUID(s.Now()), Status: syncPvpMatchWaiting, CreatedAt: s.Now().Unix(), Score: p.SyncPvpScore, OwnProfile: socialProfile(av)}
		p.SyncPvpMatch = match
		return nil
	}); err != nil {
		return nil, err
	}
	if match.Status == syncPvpMatchWaiting {
		return []Push{Callback(cb, []any{RetSuccess})}, nil
	}
	rule, err := syncPvpScoreRuleFor(match.Score)
	if err != nil {
		return nil, err
	}
	infos, err := s.syncPvpSelectInfos(c, match)
	if err != nil {
		return nil, err
	}
	return []Push{
		Callback(cb, []any{RetSuccess}),
		push("Avatar", "select_pvp_cards", infos, rule.LineupNum),
	}, nil
}

// syncPvpSelectInfos 组装 select_pvp_cards 首参：{eid: {avatar_info,
// preset_ids, cards_record, cards}}。己方 eid 为玩家 ObjectId，敌方为机器人
// 派生 eid；卡快照使用客户端 card.load 兼容字典形态。
func (s *Service) syncPvpSelectInfos(c *Connection, match *SyncPvpMatch) (mobileproto.Map, error) {
	ownEID := hexOf(selectedOID(c))
	var p *Progress
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			progress := av.Progress
			p = &progress
		}
	}
	if p == nil {
		return nil, errors.New("同步PVP缺少玩家进度")
	}
	ownCards := make([]any, 0, len(p.Cards))
	for _, card := range cardMgrPropertiesWithRunes(p.Cards, p.Runes) {
		entry := card.(map[string]any)
		entry["uuid"] = ObjectID(entry["uuid"].(string))
		ownCards = append(ownCards, card)
	}
	presetIDs := make([]any, 0, len(match.PresetIDs))
	for _, id := range match.PresetIDs {
		presetIDs = append(presetIDs, ObjectID(id))
	}
	presetRecord := match.Presets
	ownWire := mobileproto.Map{}
	for presetID, record := range presetRecord {
		fighting := make([]any, 0, len(record.Cards.Fighting))
		for _, uuid := range record.Cards.Fighting {
			fighting = append(fighting, ObjectID(uuid))
		}
		support := make([]any, 0, len(record.Cards.Support))
		for _, uuid := range record.Cards.Support {
			support = append(support, ObjectID(uuid))
		}
		ownWire = append(ownWire, mobileproto.Pair{Key: ObjectID(presetID), Value: map[string]any{"fighting_cards": fighting, "support_cards": support}})
	}
	ownProfile := match.OwnProfile
	if ownProfile.EID == "" {
		ownProfile = socialProfile(c.SelectedAvatarUnsafe())
	}
	ownProfile.Score = match.Score
	infos := mobileproto.Map{{Key: ObjectID(ownEID), Value: map[string]any{
		"avatar_info":  ownProfile.wire(),
		"preset_ids":   presetIDs,
		"cards_record": ownWire,
		"cards":        ownCards,
	}}}
	enemyEID := robotAvatarEID(match.Robot.ID)
	enemyDefaults := DefaultAvatarInfo(match.Robot.Nickname)
	enemyFighting := make([]any, 0, len(match.Robot.SyncPvpCards))
	for _, templateID := range match.Robot.SyncPvpCards {
		enemyFighting = append(enemyFighting, ObjectID(robotCardUUID(templateID)))
	}
	infos = append(infos, mobileproto.Pair{Key: ObjectID(enemyEID), Value: map[string]any{
		"avatar_info": map[string]any{"eid": ObjectID(enemyEID), "nickname": match.Robot.Nickname,
			"hostnum": ownProfile.Hostnum, "level": match.Robot.RobotLevel, "head_id": enemyDefaults.HeadID, "head_box_id": enemyDefaults.HeadBoxID,
			"extra": map[string]any{"sync_pvp_score": match.RobotScore}},
		"preset_ids":   []any{ObjectID(robotAvatarEID(match.Robot.ID))},
		"cards_record": mobileproto.Map{{Key: ObjectID(enemyEID), Value: map[string]any{"fighting_cards": enemyFighting, "support_cards": []any{}}}},
		"cards":        robotCardsWire(match.Robot),
	}})
	return infos, nil
}

// selectedAvatarProfile 返回当前角色的昵称与等级（PVP avatar_info 用）。
func selectedAvatarProfile(c *Connection) (string, int) {
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			return av.Info.Nickname, av.Info.Level
		}
	}
	return "书客", 1
}

// presetSnapshotFor 返回同步 PVP 视角的预设 ID 与记录
// （无同步 PVP 预设时回退当前阵容单卡组）。
func presetSnapshotFor(p *Progress, now time.Time) ([]any, map[string]PresetRecord) {
	if len(p.SyncPvpPresetIDs) == 0 {
		if len(p.Lineup) == 0 {
			return []any{}, map[string]PresetRecord{}
		}
		id := newBattleUUID(now)
		record := PresetRecord{PresetID: id, Cards: BattleLayout{Fighting: append([]string{}, p.Lineup...)}}
		return []any{ObjectID(id)}, map[string]PresetRecord{id: record}
	}
	ids := make([]any, 0, len(p.SyncPvpPresetIDs))
	records := map[string]PresetRecord{}
	for _, presetID := range p.SyncPvpPresetIDs {
		ids = append(ids, ObjectID(presetID))
		if row, ok := p.PresetCardsRecord[presetID]; ok {
			records[presetID] = row
		}
	}
	return ids, records
}

// sendSyncPVPCards 校验并锁定己方出战阵容：fighting≤4、support≤2、
// 必须本人拥有且不重复；成功后立即启动加载，避免等待加载完成才通知加载。
func (s *Service) sendSyncPVPCards(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if len(args) != 1 && len(args) != 2 {
		return nil, errors.New("同步PVP阵容参数无效")
	}
	index := 0
	if len(args) == 2 {
		if _, ok := callbackArg(args); !ok {
			return nil, errors.New("同步PVP回调参数无效")
		}
		index = 1
	}
	var presetIndex int
	if string(args[index]) != "null" && json.Unmarshal(args[index], &presetIndex) == nil {
		if err := s.updateProgress(ctx, c, func(p *Progress) error {
			m := p.SyncPvpMatch
			if p.Battle != nil && !p.Battle.Finished {
				return errors.New("战斗开始后不能重新选卡")
			}
			if m == nil || m.Status != syncPvpMatchReady || presetIndex < 0 || presetIndex >= len(m.PresetIDs) {
				return errors.New("同步PVP预设索引无效")
			}
			row, ok := m.Presets[m.PresetIDs[presetIndex]]
			if !ok || len(row.Cards.team()) == 0 {
				return errors.New("同步PVP预设为空")
			}
			if err := validateBattleLayout(*p, row.Cards); err != nil {
				return err
			}
			m.OwnFighting, m.OwnSupport = append([]string{}, row.Cards.Fighting...), append([]string{}, row.Cards.Support...)
			return nil
		}); err != nil {
			return nil, err
		}
		loading, err := s.syncPVPLoaded(ctx, c, nil)
		if err != nil {
			return nil, err
		}
		return append([]Push{push("Avatar", "set_cards_result", true, "")}, loading...), nil
	}
	var teams struct {
		Fighting []any `json:"fighting_cards"`
		Support  []any `json:"support_cards"`
	}
	if err := json.Unmarshal(args[index], &teams); err != nil {
		return nil, errors.New("同步PVP阵容必须是容器")
	}
	owned := map[string]bool{}
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			for _, card := range av.Progress.Cards {
				owned[card.UUID] = true
			}
		}
	}
	seen := map[string]bool{}
	parse := func(items []any, limit int) ([]string, error) {
		out := make([]string, 0, len(items))
		for _, item := range items {
			uuid, ok := item.(string)
			if !ok || !owned[uuid] || seen[uuid] {
				return nil, errors.New("同步PVP阵容含未拥有或重复卡")
			}
			seen[uuid] = true
			out = append(out, uuid)
		}
		if len(out) > limit {
			return nil, errors.New("同步PVP阵容超出容量")
		}
		return out, nil
	}
	fighting, err := parse(teams.Fighting, 4)
	if err != nil {
		return nil, err
	}
	support, err := parse(teams.Support, 2)
	if err != nil {
		return nil, err
	}
	if len(fighting) == 0 {
		return nil, errors.New("同步PVP出战阵容为空")
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if p.SyncPvpMatch == nil || p.SyncPvpMatch.Status != syncPvpMatchReady {
			return errors.New("没有可锁定的同步PVP匹配")
		}
		if p.Battle != nil && !p.Battle.Finished {
			return errors.New("战斗开始后不能重新选卡")
		}
		if err := validateBattleLayout(*p, BattleLayout{Fighting: fighting, Support: support}); err != nil {
			return err
		}
		p.SyncPvpMatch.OwnFighting = fighting
		p.SyncPvpMatch.OwnSupport = support
		return nil
	}); err != nil {
		return nil, err
	}
	loading, err := s.syncPVPLoaded(ctx, c, nil)
	if err != nil {
		return nil, err
	}
	if len(args) == 2 {
		cb, _ := callbackArg(args)
		return append([]Push{Callback(cb, []any{RetSuccess})}, loading...), nil
	}
	return append([]Push{push("Avatar", "set_cards_result", true, "")}, loading...), nil
}

// syncPVPLoaded 建立客户端权威战斗会话并与 enter_dungeon 同链开战：
// 装桥 → start_server_battle_ok → set_last_fighting_cards → prepare。
// 战斗过程由客户端原生引擎演算，胜负经观察桥 result 事件回传。
func (s *Service) syncPVPLoaded(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if len(args) != 0 && len(args) != 1 {
		return nil, errors.New("同步PVP加载参数无效")
	}
	var cb int
	if len(args) == 1 {
		var ok bool
		cb, ok = callbackArg(args)
		if !ok {
			return nil, errors.New("同步PVP回调参数无效")
		}
	}
	var match *SyncPvpMatch
	repeated := false
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if p.SyncPvpMatch == nil || p.SyncPvpMatch.Status != syncPvpMatchReady {
			return errors.New("同步PVP匹配尚未准备")
		}
		if len(p.SyncPvpMatch.OwnFighting) == 0 {
			return errors.New("同步PVP阵容尚未锁定")
		}
		match = p.SyncPvpMatch
		match.OwnCards = nil
		mgr := cardMgrPropertiesWithRunes(p.Cards, p.Runes)
		for _, id := range append(append([]string{}, match.OwnFighting...), match.OwnSupport...) {
			if entry, ok := mgr[id].(map[string]any); ok {
				match.OwnCards = append(match.OwnCards, entry)
			}
		}
		if p.Battle != nil && !p.Battle.Finished {
			if p.Battle.UUID != match.BattleUUID {
				return errors.New("已有其他战斗会话")
			}
			repeated = true
			return nil
		}
		// 与 enter_dungeon 共用 BattleSession 观察链：同一 UUID 的
		// __battle_event__ 才是结算胜负来源。
		scoreRule, e := syncPvpScoreRuleFor(match.Score)
		if e != nil {
			return e
		}
		battleID := scoreRule.DungeonBattleID
		if battleID <= 0 {
			return errors.New("同步PVP战斗表缺失")
		}
		p.Battle = &BattleSession{UUID: match.BattleUUID, DungeonID: syncPvpRule().DungeonID,
			BattleID: battleID, Seed: match.Seed, Status: "准备",
			CreatedAt: s.Now().Unix(), Team: append(append([]string{}, match.OwnFighting...), match.OwnSupport...),
			Extra: map[string]any{"sync_pvp": true, "activity_id": syncPvpRule().ActivityID}}
		s.markNativeSoloSession(p.Battle)
		return nil
	}); err != nil {
		return nil, err
	}
	if repeated {
		if len(args) == 1 {
			return []Push{Callback(cb, []any{RetSuccess})}, nil
		}
		return nil, nil
	}
	return s.syncPVPLoadPushes(c, match, cb, len(args) == 1), nil
}

// syncPVPLoadPushes 重连与首次加载共用同一冻结快照。
func (s *Service) syncPVPLoadPushes(c *Connection, match *SyncPvpMatch, cb int, callback bool) []Push {
	c.battleStartSent = false
	playerID := ObjectID(hexOf(selectedOID(c)))
	enemyID := ObjectID(robotAvatarEID(match.Robot.ID))
	enemyTeam := make([]string, 0, len(match.Robot.SyncPvpCards))
	for _, id := range match.Robot.SyncPvpCards {
		enemyTeam = append(enemyTeam, robotCardUUID(id))
	}
	loadTeams := mobileproto.Map{{Key: playerID, Value: map[string]any{"fighting_cards": battleSlotWire(match.OwnFighting), "support_cards": battleSlotWire(match.OwnSupport)}}, {Key: enemyID, Value: map[string]any{"fighting_cards": battleSlotWire(enemyTeam), "support_cards": []any{}}}}
	rule, _ := syncPvpScoreRuleFor(match.Score)
	pushes := []Push{
		push("Avatar", "on_query_hotfix_success", clientExtensionsScript(), bridgeHotfixIndex),
		push("Avatar", "start_server_battle_ok", 2, syncPvpRule().DungeonID,
			ObjectID(match.BattleUUID), s.nativeSoloLoadExtra(c.SelectedAvatarUnsafe().Progress, map[string]any{"sync_pvp": true})),
		push("Avatar", "pvp_battle_start_load", loadTeams),
		battleSync("set_last_fighting_cards", lastFightingWire(playerID, match.OwnFighting, match.OwnSupport)),
		battleSync("prepare", []any{playerID, enemyID}, rule.DungeonBattleID, match.Seed, syncPvpRule().DungeonID),
	}
	prefs := BattlePrefs{}
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			prefs = av.Progress.battlePreferences()
		}
	}
	pushes = append(pushes, battleSync("revival_set_battle_preferences", prefs.BattleSpeed, prefs.AutoBattle))
	if callback {
		pushes = append(pushes, Callback(cb, []any{RetSuccess}))
	}
	return pushes
}

func (s *Service) cancelSyncPVP(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	cb, ok := callbackArg(args)
	if !ok || len(args) != 1 {
		return nil, errors.New("取消同步PVP参数无效")
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if p.Battle != nil && !p.Battle.Finished && p.SyncPvpMatch != nil && p.Battle.UUID == p.SyncPvpMatch.BattleUUID {
			return errors.New("战斗加载后不能取消匹配")
		}
		if p.SyncPvpMatch != nil && p.SyncPvpMatch.Status != syncPvpMatchSettled {
			p.SyncPvpMatch.Status = syncPvpMatchAborted
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return []Push{Callback(cb, []any{RetSuccess})}, nil
}

// syncPvpRankRow 是真实角色排行榜行。
type syncPvpRankRow struct {
	oid      string
	uid      int64
	hostnum  int
	rank     int
	score    int
	nickname string
	level    int
	info     SocialProfile
}

// querySyncPVPRank 返回跨账号真实排行，本服过滤在存储层分页前完成。
func (s *Service) querySyncPVPRank(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	var ownScore int
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			ownScore = av.Progress.SyncPvpScore
		}
	}
	hostFilter := 0
	if method == "query_sync_pvp_local_rank_list" || method == "query_sync_pvp_rank" {
		hostFilter = c.hostnum
	}
	entries, err := s.Accounts.SyncPvpRankings(ctx, 1000, hostFilter)
	if err != nil {
		return nil, err
	}
	rows := make([]syncPvpRankRow, 0, len(entries))
	for _, entry := range entries {
		if (method == "query_sync_pvp_local_rank_list" || method == "query_sync_pvp_rank") && entry.Hostnum != c.hostnum {
			continue
		}
		rows = append(rows, syncPvpRankRow{oid: hexOf(entry.OID), uid: entry.UID, hostnum: entry.Hostnum, score: entry.Score, nickname: entry.Nickname, level: entry.Level})
	}
	if store, ok := s.Accounts.(SocialAccounts); ok {
		ids := []string{}
		for _, r := range rows {
			ids = append(ids, r.oid)
		}
		if len(ids) > 0 {
			avatars, e := store.SocialAvatars(ctx, SocialSearch{OIDs: ids, Limit: 1000})
			if e != nil {
				return nil, e
			}
			lookup := map[string]SocialProfile{}
			for _, av := range avatars {
				lookup[hexOf(av.OID)] = socialProfile(av)
			}
			for i := range rows {
				rows[i].info = lookup[rows[i].oid]
				rows[i].info.Score = rows[i].score
			}
		}
	}
	name := map[string]string{
		"query_sync_pvp_rank":            "on_query_sync_pvp_rank",
		"query_sync_pvp_world_rank":      "on_query_sync_pvp_world_rank",
		"query_sync_pvp_local_rank_list": "on_query_sync_pvp_local_rank_list",
		"query_sync_pvp_world_rank_list": "on_query_sync_pvp_world_rank_list",
	}[method]
	if name == "" {
		return nil, errors.New("未知同步PVP排行请求")
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score > rows[j].score
		}
		return rows[i].oid < rows[j].oid
	})
	for i := range rows {
		rows[i].rank = i + 1
	}
	if name == "on_query_sync_pvp_local_rank_list" || name == "on_query_sync_pvp_world_rank_list" {
		list := make([]any, 0, len(rows))
		for _, row := range rows {
			list = append(list, syncPvpRankWire(row))
		}
		return []Push{push("Avatar", name, list)}, nil
	}
	cb, ok := callbackArg(args)
	if !ok || len(args) != 1 {
		return nil, errors.New("同步PVP排行回调参数无效")
	}
	_ = ownScore
	hostnum := 0
	if method == "query_sync_pvp_rank" {
		hostnum = c.hostnum
	}
	rank, err := s.Accounts.SyncPvpRank(ctx, selectedOID(c), hostnum)
	if err != nil {
		return nil, err
	}
	return []Push{Callback(cb, []any{rank})}, nil
}

func syncPvpRankWire(row syncPvpRankRow) map[string]any {
	rule, _ := syncPvpScoreRuleFor(row.score)
	info := row.info
	if info.EID == "" {
		defaults := DefaultAvatarInfo(row.nickname)
		info = SocialProfile{EID: row.oid, UID: row.uid, Hostnum: row.hostnum, Nickname: row.nickname, Level: row.level, HeadID: defaults.HeadID, HeadBoxID: defaults.HeadBoxID, Score: row.score}
	}
	return map[string]any{"rank": row.rank, "score": row.score,
		"avatar_info": info.wire(),
		"division":    rule.ID}
}

// querySyncPVPSeason 按 Android sync_pvp_rule 的 season_original_date 与
// season_duration 推算赛季编号，不再把空赛季默认为 1。
func (s *Service) querySyncPVPSeason(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if len(args) != 0 && len(args) != 1 {
		return nil, errors.New("查询同步PVP赛季参数无效")
	}
	season := syncPvpSeasonID(s.Now())
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		return ensureSyncPvpPeriod(p, s.Now())
	}); err != nil {
		return nil, err
	}
	begin := c.SelectedAvatarUnsafe().Progress.SyncPvpMeta.BeginPeriod
	pushes := []Push{push("Avatar", "on_query_sync_pvp_season_id", season, begin)}
	if len(args) == 1 {
		cb, ok := callbackArg(args)
		if !ok {
			return nil, errors.New("查询同步PVP赛季回调参数无效")
		}
		pushes = append(pushes, Callback(cb, []any{season, begin}))
	}
	return pushes, nil
}

func syncPVPMatchProperties(match *SyncPvpMatch) map[string]any {
	if match == nil {
		return map[string]any{}
	}
	return map[string]any{"battle_uuid": match.BattleUUID, "status": match.Status, "score": match.Score,
		"rule_id": match.RuleID, "robot": match.Robot.Nickname, "created_at": match.CreatedAt}
}

func ensureSyncPVPExtra(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func parsePVPDungeonExtra(extra map[string]any) bool {
	if extra == nil {
		return false
	}
	value, ok := extra["activity_id"]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case float64:
		return int(v) == syncPvpRule().ActivityID
	case int:
		return v == syncPvpRule().ActivityID
	case string:
		i, _ := strconv.Atoi(v)
		return i == syncPvpRule().ActivityID
	default:
		return false
	}
}
