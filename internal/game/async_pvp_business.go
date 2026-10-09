package game

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hs-server/internal/mobileproto"
	"math"
	"sort"
	"time"
)

type AsyncPvpCandidate struct {
	Info  SocialProfile    `json:"info"`
	Cards []map[string]any `json:"cards"`
	Team  []string         `json:"team"`
	Robot bool             `json:"robot"`
}
type AsyncPvpMatch struct {
	UUID           string            `json:"uuid"`
	OwnInfo        SocialProfile     `json:"own_info"`
	Enemy          AsyncPvpCandidate `json:"enemy"`
	OwnCards       []map[string]any  `json:"own_cards"`
	OwnTeam        []string          `json:"own_team"`
	OwnScore       int               `json:"own_score"`
	CreatedAt      int64             `json:"created_at"`
	Receipt        *AsyncPvpRecord   `json:"receipt,omitempty"`
	ResultSequence int64             `json:"result_sequence,omitempty"`
	ResultDigest   string            `json:"result_digest,omitempty"`
	Rewards        map[int]int64     `json:"rewards,omitempty"`
}
type AsyncPvpState struct {
	Score          int                          `json:"score"`
	MaxScore       int                          `json:"max_score"`
	Rank           int                          `json:"rank"`
	Auto           bool                         `json:"auto"`
	Defence        []string                     `json:"defence,omitempty"`
	Candidates     map[string]AsyncPvpCandidate `json:"candidates,omitempty"`
	RefreshAt      int64                        `json:"refresh_at"`
	Match          *AsyncPvpMatch               `json:"match,omitempty"`
	AttackRecords  []AsyncPvpRecord             `json:"attack_records,omitempty"`
	DefenceRecords []AsyncPvpRecord             `json:"defence_records,omitempty"`
	Receipts       map[string]bool              `json:"receipts,omitempty"`
	RewardDay      string                       `json:"reward_day,omitempty"`
	RewardWeek     string                       `json:"reward_week,omitempty"`
}
type AsyncPvpRecord struct {
	UUID       string           `json:"uuid"`
	OwnScore   int              `json:"old_score"`
	NewScore   int              `json:"new_score"`
	EnemyInfo  SocialProfile    `json:"enemy_info"`
	EnemyOld   int              `json:"enemy_old_score"`
	EnemyNew   int              `json:"enemy_new_score"`
	OwnTeam    []string         `json:"card_uuids"`
	EnemyTeam  []string         `json:"enemy_card_uuids"`
	OwnCards   []map[string]any `json:"cards"`
	EnemyCards []map[string]any `json:"enemy_cards"`
	Time       int64            `json:"time"`
	Win        bool             `json:"win"`
}

func (r AsyncPvpRecord) wire() map[string]any {
	result := 0
	if r.Win {
		result = 1
	}
	return map[string]any{"card_uuids": battleSlotWire(r.OwnTeam), "cards": r.OwnCards, "old_score": r.OwnScore, "new_score": r.NewScore, "enemy_info": r.EnemyInfo.wire(), "enemy_card_uuids": battleSlotWire(r.EnemyTeam), "enemy_cards": r.EnemyCards, "enemy_old_score": r.EnemyOld, "enemy_new_score": r.EnemyNew, "time": float64(r.Time), "version": "1.0.128", "result": result}
}
func ensureAsync(v *AsyncPvpState) {
	if v.Score < 1000 {
		v.Score = 1000
	}
	if v.MaxScore < v.Score {
		v.MaxScore = v.Score
	}
	if v.Candidates == nil {
		v.Candidates = map[string]AsyncPvpCandidate{}
	}
	if v.Receipts == nil {
		v.Receipts = map[string]bool{}
	}
}
func ensureAsyncCalendar(v *AsyncPvpState, now time.Time) {
	day, week := pvpAwardCutoffs(now)
	if v.RewardDay == "" {
		v.RewardDay = day
	}
	if v.RewardWeek == "" {
		v.RewardWeek = week
	}
}

type asyncPvpRule struct {
	DungeonID int   `json:"dungeon_id"`
	Ticket    int   `json:"asyn_pvp_material_id"`
	Refresh   int64 `json:"refresh_async_pvp_interval"`
	RecordNum int   `json:"record_num"`
}

func asyncRule() asyncPvpRule {
	var r asyncPvpRule
	if json.Unmarshal(socialCatalog.Tables["asyn_pvp_base_rule"]["1"], &r) != nil || r.DungeonID <= 0 {
		panic("异步竞技规则目录无效")
	}
	return r
}

type asyncScoreRule struct {
	ID            int     `json:"_id"`
	Min           int     `json:"min_score"`
	Max           int     `json:"max_score"`
	K             int     `json:"k"`
	DefenceFactor float64 `json:"defence_win_factor"`
	FailFactor    float64 `json:"fail_factor"`
	Bonus         int     `json:"finish_bonus"`
}

func asyncScoreFor(score int) (asyncScoreRule, error) {
	for _, raw := range socialCatalog.Tables["asyn_pvp_score_rule"] {
		var row asyncScoreRule
		if json.Unmarshal(raw, &row) == nil && score >= row.Min && score <= row.Max {
			return row, nil
		}
	}
	return asyncScoreRule{}, errors.New("异步竞技分段缺失")
}
func asyncUnlocked(p Progress) bool { return p.UnlockSystems["panel_async_pvp"] > 0 }

func (s *Service) asyncPvpRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("异步竞技需要玩家状态")
	}
	if err := s.refreshPvpAwards(ctx, c); err != nil {
		return nil, err
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil, errors.New("账号存储不支持异步竞技")
	}
	own := c.SelectedAvatarUnsafe()
	if method == "enter_asyn_pvp" || method == "refresh_asyn_pvp" {
		if len(args) != 0 {
			return nil, errors.New("异步竞技列表不带参数")
		}
		rows, err := store.SocialAvatars(ctx, SocialSearch{Limit: 1000})
		if err != nil {
			return nil, err
		}
		var candidates map[string]AsyncPvpCandidate
		err = s.updateProgress(ctx, c, func(p *Progress) error {
			if !asyncUnlocked(*p) {
				return errors.New("异步竞技系统尚未解锁")
			}
			ensureAsync(&p.AsyncPvp)
			ensureAsyncCalendar(&p.AsyncPvp, s.Now())
			v := &p.AsyncPvp
			if s.Now().Unix() < v.RefreshAt {
				return errors.New("异步竞技刷新时钟回拨")
			}
			if method == "refresh_asyn_pvp" && s.Now().Unix() < v.RefreshAt+asyncRule().Refresh {
				return errors.New("异步竞技刷新尚未到期")
			}
			if method == "enter_asyn_pvp" && len(v.Candidates) > 0 {
				candidates = v.Candidates
				return nil
			}
			v.Candidates = map[string]AsyncPvpCandidate{}
			for _, av := range rows {
				id := hexOf(av.OID)
				if id == hexOf(own.OID) || len(av.Progress.AsyncPvp.Defence) == 0 {
					continue
				}
				ensureAsync(&av.Progress.AsyncPvp)
				if !asyncUnlocked(av.Progress) {
					continue
				}
				team := av.Progress.AsyncPvp.Defence
				if err := validateBattleLayout(av.Progress, BattleLayout{Fighting: team}); err != nil {
					continue
				}
				info := socialProfile(av)
				info.Score = av.Progress.AsyncPvp.Score
				candidate := AsyncPvpCandidate{Info: info, Team: append([]string{}, team...)}
				mgr := cardMgrPropertiesWithRunes(av.Progress.Cards, av.Progress.Runes)
				for _, id := range team {
					if item, ok := mgr[id].(map[string]any); ok {
						candidate.Cards = append(candidate.Cards, item)
					}
				}
				v.Candidates[id] = candidate
				if len(v.Candidates) >= 3 {
					break
				}
			}
			if len(v.Candidates) < 3 {
				bots := []syncPvpRobot{}
				for _, bot := range syncPvpCatalog.Tables.Robots {
					if len(bot.DefenceCards) > 0 {
						bots = append(bots, bot)
					}
				}
				sort.Slice(bots, func(i, j int) bool {
					di := int(math.Abs(float64(bots[i].AsyncScore - v.Score)))
					dj := int(math.Abs(float64(bots[j].AsyncScore - v.Score)))
					if di != dj {
						return di < dj
					}
					return bots[i].ID < bots[j].ID
				})
				for _, bot := range bots {
					defaults := DefaultAvatarInfo(bot.Nickname)
					candidate := AsyncPvpCandidate{Info: SocialProfile{EID: robotAvatarEID(bot.ID), Nickname: bot.Nickname, Hostnum: c.hostnum, Level: bot.RobotLevel, HeadID: defaults.HeadID, HeadBoxID: defaults.HeadBoxID, Score: bot.AsyncScore}, Robot: true}
					for _, id := range bot.DefenceCards {
						template, exists := syncPvpCatalog.Tables.RobotCards[intString(id)]
						if !exists {
							return errors.New("异步机器人幻书模板缺失")
						}
						candidate.Team = append(candidate.Team, robotCardUUID(id))
						candidate.Cards = append(candidate.Cards, robotCardWire(template))
					}
					v.Candidates[candidate.Info.EID] = candidate
					if len(v.Candidates) >= 3 {
						break
					}
				}
			}
			if len(v.Candidates) == 0 {
				return errors.New("异步竞技没有合法对手")
			}
			v.RefreshAt = s.Now().Unix()
			candidates = v.Candidates
			return nil
		})
		if err != nil {
			return nil, err
		}
		out := mobileproto.Map{}
		ids := []string{}
		for id := range candidates {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			info := candidates[id].Info.wire()
			info["extra"] = map[string]any{"score": candidates[id].Info.Score}
			info["__custom_type"] = "avatar_info.avatar_info"
			out = append(out, mobileproto.Pair{Key: ObjectID(id), Value: info})
		}
		p := c.SelectedAvatarUnsafe().Progress
		rank, e := store.AsyncPvpRank(ctx, selectedOID(c), 0)
		if e != nil {
			return nil, e
		}
		if e = s.updateProgress(ctx, c, func(p *Progress) error { p.AsyncPvp.Rank = rank; return nil }); e != nil {
			return nil, e
		}
		return []Push{push("Avatar", "client_prop_changed", []any{"asyn_pvp_score", p.AsyncPvp.Score}), push("Avatar", "client_prop_changed", []any{"asyn_pvp_rank", rank}), push("Avatar", "on_enter_asyn_pvp", out)}, nil
	}
	cb, valid := callbackArg(args)
	if !valid {
		return nil, errors.New("异步竞技回调无效")
	}
	switch method {
	case "set_defence_cards":
		var team []string
		if len(args) != 2 || json.Unmarshal(args[1], &team) != nil || len(team) == 0 || len(team) > 4 {
			return nil, errors.New("异步防守阵容参数无效")
		}
		if err := s.updateProgress(ctx, c, func(p *Progress) error {
			if !asyncUnlocked(*p) {
				return errors.New("异步竞技尚未解锁")
			}
			if err := validateBattleLayout(*p, BattleLayout{Fighting: team}); err != nil {
				return err
			}
			ensureAsync(&p.AsyncPvp)
			p.AsyncPvp.Defence = append([]string{}, team...)
			return nil
		}); err != nil {
			return nil, err
		}
		return []Push{push("Avatar", "client_prop_changed", []any{"asyn_pvp_cards", battleSlotWire(team)}), Callback(cb, []any{RetSuccess})}, nil
	case "get_candidate_cards":
		var id string
		if len(args) != 2 || json.Unmarshal(args[1], &id) != nil {
			return nil, errors.New("异步候选卡牌参数无效")
		}
		rows, err := store.SocialAvatars(ctx, SocialSearch{OIDs: []string{hexOf(own.OID)}, Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(rows) != 1 {
			return nil, errors.New("竞技角色不存在")
		}
		candidate, exists := rows[0].Progress.AsyncPvp.Candidates[id]
		if !exists {
			return []Push{Callback(cb, []any{false, []any{}})}, nil
		}
		return []Push{Callback(cb, []any{true, candidate.Cards})}, nil
	case "set_asyn_pvp_auto":
		var auto bool
		if len(args) != 2 || json.Unmarshal(args[1], &auto) != nil {
			return nil, errors.New("异步自动战斗参数无效")
		}
		if err := s.updateProgress(ctx, c, func(p *Progress) error {
			if !asyncUnlocked(*p) {
				return errors.New("异步竞技尚未解锁")
			}
			p.AsyncPvp.Auto = auto
			return nil
		}); err != nil {
			return nil, err
		}
		return []Push{push("Avatar", "client_prop_changed", []any{"asyn_pvp_auto", auto}), Callback(cb, []any{RetSuccess})}, nil
	}
	return nil, errors.New("未知异步竞技请求")
}

// enterAsyncPvp 使用持久候选快照冻结敌方，票据与新会话在同事务扣除。没有候选绝不放行。
func (s *Service) enterAsyncPvp(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if err := s.refreshPvpAwards(ctx, c); err != nil {
		return nil, err
	}
	cb, ok := callbackArg(args)
	var dungeon int
	var extra struct {
		EID string `json:"asyn_pvp_eid"`
	}
	if !ok || len(args) != 3 || json.Unmarshal(args[1], &dungeon) != nil || dungeon != asyncRule().DungeonID || json.Unmarshal(args[2], &extra) != nil || extra.EID == "" {
		return nil, errors.New("异步挑战签名无效")
	}
	var match *AsyncPvpMatch
	own := c.SelectedAvatarUnsafe()
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if !asyncUnlocked(*p) {
			return errors.New("异步竞技尚未解锁")
		}
		ensureAsync(&p.AsyncPvp)
		ensureAsyncCalendar(&p.AsyncPvp, s.Now())
		candidate, exists := p.AsyncPvp.Candidates[extra.EID]
		if !exists {
			return errors.New("该对手不在已授权候选中")
		}
		if p.Battle != nil && !p.Battle.Finished {
			if p.AsyncPvp.Match != nil && p.Battle.UUID == p.AsyncPvp.Match.UUID && p.AsyncPvp.Match.Enemy.Info.EID == extra.EID {
				match = p.AsyncPvp.Match
				return nil
			}
			return errors.New("已有未结束的战斗")
		}
		rule := asyncRule()
		if err := runeMaterials(p, [][]int{{rule.Ticket, 1}}, -1); err != nil {
			return err
		}
		uuid := newBattleUUID(s.Now())
		own.Progress = *p
		info := socialProfile(own)
		info.Score = p.AsyncPvp.Score
		match = &AsyncPvpMatch{UUID: uuid, OwnInfo: info, OwnScore: p.AsyncPvp.Score, Enemy: candidate, CreatedAt: s.Now().Unix()}
		p.AsyncPvp.Match = match
		session := BattleSession{UUID: uuid, DungeonID: dungeon, BattleID: dungeonCatalog[dungeon].BattleID, Seed: s.Now().UnixNano() & 0x7fffffff, Status: "准备", CreatedAt: s.Now().Unix(), Extra: map[string]any{"asyn_pvp": true}}
		p.Battle = &session
		s.markNativeSoloSession(p.Battle)
		return nil
	})
	if err != nil {
		return []Push{Callback(cb, []any{false, err.Error()})}, nil
	}
	return append([]Push{materialManagerPush(c), Callback(cb, []any{0})}, s.asyncPvpLoadPushes(c, match)...), nil
}
func (s *Service) asyncPvpLoadPushes(c *Connection, match *AsyncPvpMatch) []Push {
	p := c.SelectedAvatarUnsafe().Progress
	b := p.Battle
	if b == nil {
		return nil
	}
	c.battleStartSent = false
	own := ObjectID(hexOf(selectedOID(c)))
	enemy := ObjectID(match.Enemy.Info.EID)
	return []Push{push("Avatar", "on_query_hotfix_success", clientExtensionsScript(), bridgeHotfixIndex), push("Avatar", "start_server_battle_ok", 3, b.DungeonID, ObjectID(b.UUID), s.nativeSoloLoadExtra(p, map[string]any{"asyn_pvp": true})), battleSync("set_last_fighting_cards", lastFightingWire(own, p.Lineup, nil)), battleSync("prepare", []any{own, enemy}, b.BattleID, b.Seed, b.DungeonID), battleSync("revival_set_battle_preferences", p.battlePreferences().BattleSpeed, p.AsyncPvp.Auto)}
}
func freezeAsyncOwnTeam(p *Progress) {
	if p.AsyncPvp.Match == nil || p.Battle == nil || p.AsyncPvp.Match.UUID != p.Battle.UUID {
		return
	}
	m := p.AsyncPvp.Match
	m.OwnTeam = append([]string{}, p.Battle.Team...)
	m.OwnCards = nil
	mgr := cardMgrPropertiesWithRunes(p.Cards, p.Runes)
	for _, id := range m.OwnTeam {
		if entry, ok := mgr[id].(map[string]any); ok {
			m.OwnCards = append(m.OwnCards, entry)
		}
	}
}
func asyncEnemyPush(p Progress) []Push {
	if p.AsyncPvp.Match == nil || p.Battle == nil || p.AsyncPvp.Match.UUID != p.Battle.UUID {
		return nil
	}
	m := p.AsyncPvp.Match
	return []Push{battleSync("add_fighting_cards", battleSlotWire(m.Enemy.Team), pvpCardList(m.Enemy.Cards), ObjectID(m.Enemy.Info.EID))}
}
func pvpCardList(cards []map[string]any) []any {
	list := make([]any, 0, len(cards)+2)
	for _, row := range cards {
		list = append(list, row)
	}
	return append(list, "card.card_list", "__custom_type")
}

// absorbAsyncPvpResult 将攻击者与真实防守者积分/记录在有序双行锁同事务结算。
// 积分为本项目ELO政策：Android K、fail_factor和defence_win_factor为表参数，并非原厂公式取证。
func (s *Service) absorbAsyncPvpResult(ctx context.Context, c *Connection, envelope *battleEnvelope) (bool, []Push, error) {
	snapshot := c.SelectedAvatarUnsafe().Progress
	if envelope.Kind != "result" || snapshot.AsyncPvp.Match == nil || snapshot.AsyncPvp.Match.UUID != envelope.BattleUUID {
		return false, nil, nil
	}
	if err := s.refreshPvpAwards(ctx, c); err != nil {
		return true, nil, err
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return true, nil, errors.New("异步结算存储不支持事务")
	}
	selfID := hexOf(selectedOID(c))
	ids := []string{selfID}
	if !snapshot.AsyncPvp.Match.Enemy.Robot {
		ids = append(ids, snapshot.AsyncPvp.Match.Enemy.Info.EID)
	}
	var rewards map[int]int64
	var own gameAsyncResult
	rows, err := store.UpdateSocial(ctx, ids, func(avatars map[string]*Avatar) error {
		av := avatars[selfID]
		p := &av.Progress
		ensureAsync(&p.AsyncPvp)
		m := p.AsyncPvp.Match
		b := p.Battle
		if m == nil || b == nil || b.UUID != envelope.BattleUUID || m.UUID != envelope.BattleUUID {
			return errors.New("异步结算会话归属不符")
		}
		if m.Receipt != nil {
			if b.NativeSolo != nil {
				client := b.NativeSolo.Native.Clients[selfID]
				if envelope.Generation != client.Generation {
					return errors.New("异步PVP结算重试世代失配")
				}
				if e := validateNativeSoloTerminal(b, selfID); e != nil {
					return e
				}
			}
			raw, _ := json.Marshal(envelope.Data)
			digest := sha256.Sum256(raw)
			if envelope.Sequence != m.ResultSequence || hex.EncodeToString(digest[:]) != m.ResultDigest {
				return errors.New("异步结果重试与原结算收据不一致")
			}
			rewards = m.Rewards
			own = gameAsyncResult{Record: *m.Receipt, Finished: true}
			return nil
		}
		if b.Finished {
			return errors.New("异步会话已结束但无收据")
		}
		if b.NativeSolo != nil {
			if e := s.nativeObservePlayback(ctx, b.NativeSolo, b, selfID, envelope); e != nil {
				return e
			}
			if e := validateNativeSoloTerminal(b, selfID); e != nil {
				return e
			}
		} else if e := acceptBattleEvent(b, envelope); e != nil {
			return e
		}
		winners, e := battleWinnerEIDs(envelope.Data)
		if e != nil {
			return e
		}
		win := false
		enemyWin := false
		for _, id := range winners {
			if id != selfID && id != m.Enemy.Info.EID {
				return errors.New("异步胜方不属于对局")
			}
			if id == selfID {
				win = true
			}
			if id == m.Enemy.Info.EID {
				enemyWin = true
			}
		}
		if win && enemyWin {
			return errors.New("异步对局不能同时有双方胜利")
		}
		if len(m.OwnCards) == 0 || len(m.OwnTeam) == 0 {
			return errors.New("异步结算缺少冻结己方阵容")
		}
		if reported, yes := envelope.Data["outcome"].(string); yes && (reported == "win") != win {
			return errors.New("异步胜负与鉴权角色不一致")
		}
		rule, e := asyncScoreFor(m.OwnScore)
		if e != nil {
			return e
		}
		old := p.AsyncPvp.Score
		expected := 1 / (1 + math.Pow(10, math.Max(-100, math.Min(100, float64(m.Enemy.Info.Score-m.OwnScore)/400))))
		value := 0.0
		if win {
			value = 1
		}
		change := float64(rule.K) * (value - expected)
		if !win {
			change *= rule.FailFactor
		}
		delta := syncPvpRound(change)
		if win && delta < 1 {
			delta = 1
		}
		p.AsyncPvp.Score = maxInt(1000, old+delta)
		p.AsyncPvp.MaxScore = maxInt(p.AsyncPvp.MaxScore, p.AsyncPvp.Score)
		enemyNew := m.Enemy.Info.Score
		enemyOld := m.Enemy.Info.Score
		if !m.Enemy.Robot {
			enemy := avatars[m.Enemy.Info.EID]
			if enemy == nil {
				return errors.New("异步防守方不存在")
			}
			ensureAsync(&enemy.Progress.AsyncPvp)
			enemyRule, e := asyncScoreFor(enemy.Progress.AsyncPvp.Score)
			if e != nil {
				return e
			}
			enemyExpected := 1 / (1 + math.Pow(10, math.Max(-100, math.Min(100, float64(old-enemy.Progress.AsyncPvp.Score)/400))))
			enemyWin := 0.0
			if !win {
				enemyWin = 1
			}
			enemyDelta := float64(enemyRule.K) * (enemyWin - enemyExpected)
			if !win {
				enemyDelta *= enemyRule.DefenceFactor
			} else {
				enemyDelta *= enemyRule.FailFactor
			}
			enemyOld = enemy.Progress.AsyncPvp.Score
			enemyNew = maxInt(1000, enemyOld+syncPvpRound(enemyDelta))
			enemy.Progress.AsyncPvp.Score = enemyNew
			enemy.Progress.AsyncPvp.MaxScore = maxInt(enemy.Progress.AsyncPvp.MaxScore, enemyNew)
			def := AsyncPvpRecord{UUID: m.UUID, OwnScore: enemyOld, NewScore: enemyNew, EnemyInfo: m.OwnInfo, EnemyOld: old, EnemyNew: p.AsyncPvp.Score, OwnTeam: m.Enemy.Team, OwnCards: m.Enemy.Cards, EnemyTeam: m.OwnTeam, EnemyCards: m.OwnCards, Time: s.Now().Unix(), Win: !win}
			enemy.Progress.AsyncPvp.DefenceRecords = append(enemy.Progress.AsyncPvp.DefenceRecords, def)
			trimAsyncRecords(&enemy.Progress.AsyncPvp)
		}
		record := AsyncPvpRecord{UUID: m.UUID, OwnScore: old, NewScore: p.AsyncPvp.Score, EnemyInfo: m.Enemy.Info, EnemyOld: enemyOld, EnemyNew: enemyNew, OwnTeam: m.OwnTeam, OwnCards: m.OwnCards, EnemyTeam: m.Enemy.Team, EnemyCards: m.Enemy.Cards, Time: s.Now().Unix(), Win: win}
		p.AsyncPvp.AttackRecords = append(p.AsyncPvp.AttackRecords, record)
		trimAsyncRecords(&p.AsyncPvp)
		// finish_bonus是原生每日21点分段邮件；战斗胜利仅结算积分与记录，不提前领取每日奖。
		m.Receipt = &record
		raw, _ := json.Marshal(envelope.Data)
		digest := sha256.Sum256(raw)
		m.ResultSequence = envelope.Sequence
		m.ResultDigest = hex.EncodeToString(digest[:])
		m.Rewards = rewards
		b.Outcome = "loss"
		if win {
			b.Outcome = "win"
			recordAchievementCompetitiveWin(p, 4, s.Now())
		}
		b.WinnerEIDs = winners
		b.Finished = true
		b.Status = "结束"
		b.RewardGranted = true
		delete(p.AsyncPvp.Candidates, m.Enemy.Info.EID)
		// 双角色事务不经过updateProgress包装器：胜利产生的成就积分和
		// 派生目标必须在首次结算内投影，不能留到result重试或恢复才改变。
		for _, avatar := range avatars {
			reconcileAchievementState(&avatar.Progress, avatar.Progress.AvatarLevel, s.Now())
		}
		own = gameAsyncResult{Record: record, Finished: true}
		return nil
	})
	if err != nil {
		return true, nil, err
	}
	for _, av := range rows {
		c.pendingSocialOIDs = append(c.pendingSocialOIDs, append([]byte{}, av.OID...))
		if hexOf(av.OID) == selfID {
			for i, current := range c.identity.Avatars {
				if current.Hostnum == c.hostnum {
					c.identity.Avatars[i].Progress = av.Progress
				}
			}
		}
	}
	rank, e := store.AsyncPvpRank(ctx, selectedOID(c), 0)
	if e != nil {
		return true, nil, e
	}
	return true, append([]Push{push("Avatar", "client_prop_changed", []any{"asyn_pvp_rank", rank})}, asyncResultPushes(c, own.Record, rewards)...), nil
}
func asyncResultPushes(c *Connection, record AsyncPvpRecord, rewards map[int]int64) []Push {
	if rewards == nil {
		rewards = map[int]int64{}
	}
	box := map[string]any{"__custom_type": "box.box", "materials": rewards}
	return []Push{materialManagerPush(c), push("Avatar", "client_prop_changed", []any{"asyn_pvp_score", c.SelectedAvatarUnsafe().Progress.AsyncPvp.Score}), push("Avatar", "client_prop_changed", []any{"asyn_pvp_max_score", c.SelectedAvatarUnsafe().Progress.AsyncPvp.MaxScore}), push("Avatar", "battle_result", record.Win, box, map[string]any{}, nativeSoloResultExtra(c.SelectedAvatarUnsafe().Progress.Battle, map[string]any{"client_authoritative": true, "verified": false, "dungeon_id": asyncRule().DungeonID, "asyn_pvp": true}))}
}

type gameAsyncResult struct {
	Record   AsyncPvpRecord
	Finished bool
}

func trimAsyncRecords(p *AsyncPvpState) {
	n := asyncRule().RecordNum
	if n < 1 {
		n = 10
	}
	if len(p.AttackRecords) > n {
		p.AttackRecords = p.AttackRecords[len(p.AttackRecords)-n:]
	}
	if len(p.DefenceRecords) > n {
		p.DefenceRecords = p.DefenceRecords[len(p.DefenceRecords)-n:]
	}
}
