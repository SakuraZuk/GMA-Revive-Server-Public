package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"time"
)

//go:embed league_protect_catalog.json
var leagueProtectCatalogRaw []byte
var androidLeagueProtect = func() struct {
	Guardian struct {
		Skills  []int `json:"skill_list"`
		Support []int `json:"extra_support_skill_list"`
	} `json:"guardian"`
	Treasures []int `json:"treasure_roles"`
} {
	var c struct {
		Guardian struct {
			Skills  []int `json:"skill_list"`
			Support []int `json:"extra_support_skill_list"`
		} `json:"guardian"`
		Treasures []int `json:"treasure_roles"`
	}
	if json.Unmarshal(leagueProtectCatalogRaw, &c) != nil || len(c.Guardian.Support) != 3 || len(c.Treasures) != 1 {
		panic("学会守门人原生目录无效")
	}
	return c
}()

type LeagueProtectProgress struct {
	Week         string                         `json:"week"`
	TotalWeekly  int                            `json:"total_weekly_protect_times"`
	CurrentTimes int                            `json:"cur_protect_times"`
	Contribution int64                          `json:"protect_contribute_point"`
	ProtectID    int                            `json:"protect_id"`
	Kills        map[int]int                    `json:"protect_kill_count_map"`
	Unlocked     map[int]bool                   `json:"protect_unlock_map"`
	Updated      int64                          `json:"protect_update_time"`
	LastTime     int64                          `json:"last_time"`
	Pending      *LeagueProtectAuthorization    `json:"pending,omitempty"`
	Results      map[string]LeagueProtectResult `json:"results,omitempty"`
	Consumed     map[string]string              `json:"consumed,omitempty"`
}
type LeagueProtectAuthorization struct {
	LeagueID        string                   `json:"league_id"`
	MemberID        string                   `json:"member_id"`
	Host            int                      `json:"host"`
	ProtectID       int                      `json:"protect_id"`
	DungeonID       int                      `json:"dungeon_id"`
	Week            string                   `json:"week"`
	Prepared        int64                    `json:"prepared"`
	CardID          int                      `json:"card_id"`
	CardLevel       int                      `json:"card_level"`
	SupportIndex    int                      `json:"extra_support_skill_idx"`
	CardWire        map[string]any           `json:"card_wire"`
	Statistics      *LeagueProtectStatistics `json:"statistics,omitempty"`
	Result          *LeagueProtectResult     `json:"result,omitempty"`
	AttemptConsumed bool                     `json:"attempt_consumed,omitempty"`
}
type LeagueProtectStatistics struct {
	Kills    int `json:"kill_count"`
	Treasure int `json:"treasure_role_id"`
}
type LeagueProtectResult struct {
	Win          bool           `json:"win_flag"`
	ProtectID    int            `json:"protect_id"`
	Kills        int            `json:"kill_count"`
	Treasure     int            `json:"treasure_role_id"`
	Box          map[string]any `json:"box"`
	TreasureBox  map[string]any `json:"treasure_box"`
	Card         map[string]any `json:"league_protect_card"`
	Contribution int64          `json:"delta_contribution"`
}

// JSONB冷读会把动态字典的数字与整数键转为float/string，原生卡记录在wire边界恢复类型。
func leagueGuardianSnapshot(raw map[string]any) any {
	var c struct {
		CardID       int          `json:"card_id"`
		Level        int          `json:"level"`
		Exp          int          `json:"exp"`
		Grade        int          `json:"grade"`
		Awakened     int          `json:"awakened"`
		Dress        int          `json:"dress"`
		Intimacy     int          `json:"intimacy"`
		Lock         int          `json:"lock"`
		Time         int64        `json:"time"`
		Enhance      int          `json:"enhance_count"`
		EnhanceIDs   []int        `json:"enhance_ids"`
		SkillEnhance int          `json:"skill_enhance_count"`
		Skills       []CardSkill  `json:"skill_mgr"`
		Runes        map[int]any  `json:"embed_runes"`
		GradeScore   int          `json:"grade_score"`
		EnhanceScore int          `json:"enhance_score"`
		SkillIndex   int          `json:"skill_index"`
		SkillScores  []int        `json:"skill_scores"`
		Extra        int          `json:"extra_support_skill_idx"`
		Psychic      int          `json:"psychic_score"`
		Unlocked     map[int]bool `json:"unlock_skills"`
	}
	data, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(data, &c) != nil {
		return nil
	}
	return activityWire(c)
}

func leagueProtectWeek(now time.Time) string {
	n := now.In(shanghaiZone)
	for n.Weekday() != time.Tuesday && n.Weekday() != time.Thursday {
		n = n.AddDate(0, 0, -1)
	}
	return n.Format("2006-01-02")
}
func ensureLeagueProtect(p *Progress, now time.Time) (*LeagueProtectProgress, error) {
	if p.LeagueProtect == nil {
		p.LeagueProtect = &LeagueProtectProgress{}
	}
	w := p.LeagueProtect
	if w.LastTime > now.Unix() || w.TotalWeekly < 0 || w.CurrentTimes < 0 || w.Contribution < 0 {
		return nil, errors.New("学会雅努斯计次存档无效或时间回拨")
	}
	week := leagueProtectWeek(now)
	if w.Week > week {
		return nil, errors.New("学会雅努斯周界回拨")
	}
	if w.Week != week {
		w.Week = week
		w.TotalWeekly = 0
		w.CurrentTimes = 0
		w.Contribution = 0
		w.Kills = map[int]int{}
		w.Updated = 0
	}
	if w.Kills == nil {
		w.Kills = map[int]int{}
	}
	if w.Unlocked == nil {
		w.Unlocked = map[int]bool{}
	}
	if w.Results == nil {
		w.Results = map[string]LeagueProtectResult{}
	}
	if w.Consumed == nil {
		w.Consumed = map[string]string{}
	}
	w.LastTime = now.Unix()
	return w, nil
}
func leagueProtectProperties(p Progress) map[string]any {
	w := p.LeagueProtect
	if w == nil {
		w = &LeagueProtectProgress{}
	}
	return map[string]any{"league_activity_weekly_info": map[string]any{"__custom_type": "league_activity_weekly_info.league_activity_weekly_info", "total_weekly_protect_times": w.TotalWeekly, "cur_protect_times": w.CurrentTimes, "protect_contribute_point": w.Contribution, "protect_id": w.ProtectID, "protect_kill_count_map": activityWire(w.Kills), "protect_unlock_map": activityWire(w.Unlocked), "protect_update_time": float64(w.Updated)}}
}
func leagueGuardianWire(state *LeagueState, index int) (map[string]any, error) {
	if index < 0 || index >= len(androidLeagueProtect.Guardian.Support) {
		return nil, runeReject("RET_LEAGUE_CARD_EXTRA_SKILL_INDEX_ERROR", "守门人援护技能索引无效")
	}
	level := 1
	selected := androidLeagueProtect.Guardian.Support[index]
	if level < leagueData("league_card_skill_unlock", selected).integer("unlock_level") {
		return nil, runeReject("RET_LEAGUE_CARD_SKILL_NOT_UNLOCK", "守门人援护技能尚未解锁")
	}
	skills := []CardSkill{}
	unlocked := map[int]bool{}
	for _, id := range androidLeagueProtect.Guardian.Skills {
		skills = append(skills, CardSkill{ID: id, Level: 1})
		if level >= leagueData("league_card_skill_unlock", id).integer("unlock_level") {
			unlocked[id] = true
		}
	}
	return map[string]any{"card_id": state.CardID, "level": level, "exp": 0, "grade": 0, "awakened": 0, "dress": 0, "intimacy": 0, "lock": 0, "time": state.Created, "enhance_count": 0, "enhance_ids": []int{}, "skill_enhance_count": 0, "skill_mgr": skills, "embed_runes": map[int]any{}, "grade_score": 0, "enhance_score": 0, "skill_index": 0, "skill_scores": []int{}, "extra_support_skill_idx": index, "psychic_score": 0, "unlock_skills": unlocked}, nil
}

func (s *Service) leagueProtectStartRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 || !remainingPolicyEnabled() {
		return nil, errors.New("雅努斯之门需要已启用的真实玩家")
	}
	var id, index int
	if json.Unmarshal(args[0], &id) != nil || json.Unmarshal(args[1], &index) != nil || id < 1 || id > 3 {
		return nil, errors.New("雅努斯难度或技能参数无效")
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil, errors.New("存储不支持真实学会事务")
	}
	now := s.Now()
	selfID := hexOf(selectedOID(c))
	did := leagueData("league_protect", id).integer("dungeon_id")
	rows, err := store.UpdateAllSocial(ctx, func(avatars map[string]*Avatar) error {
		self := avatars[selfID]
		if self == nil {
			return errors.New("雅努斯当前玩家不存在")
		}
		_, state := leagueOwner(avatars, self.Progress.LeagueID)
		if state == nil || state.Host != self.Hostnum || state.Members[selfID].Joined <= 0 || self.Progress.AvatarLevel < 6 {
			return runeReject("RET_LEAGUE_NOT_JOINED", "雅努斯需要真实同服学会会员")
		}
		p := &self.Progress
		if p.Battle != nil && !p.Battle.Finished {
			if p.Battle.DungeonID == did && p.Battle.ActivityContext != nil && p.Battle.ActivityContext.LeagueProtect != nil && p.Battle.ActivityContext.LeagueProtect.SupportIndex == index {
				old := p.Battle.ActivityContext.LeagueProtect
				if !old.AttemptConsumed && old.Week != leagueProtectWeek(now) {
					p.Battle = nil
				} else {
					return nil
				}
			} else {
				return runeReject("RET_LEAGUE_CHALLENGE_DULPLICATE", "已有其他战斗不能重建雅努斯会话")
			}
		}
		weekday := int(now.In(shanghaiZone).Weekday())
		if weekday == 0 {
			weekday = 7
		}
		if !containsInt(activityData("activity_type", 14).ids("open_time"), weekday) {
			return runeReject("RET_LEAGUE_ACTIVITY_REFRESH", "雅努斯之门未到原表开放日")
		}
		if _, err := activityOpen(*p, 14, p.AvatarLevel, now); err != nil {
			return err
		}
		w, err := ensureLeagueProtect(p, now)
		if err != nil {
			return err
		}
		if w.TotalWeekly >= leagueData("league_base", 1).integer("protect_times_limit") {
			return runeReject("RET_LEAGUE_PROTECT_MAX_LIMIT", "雅努斯本次开放参加次数已达原表上限")
		}
		based := leagueData("league_protect", id).integer("unlock_based_id")
		if based > 0 && !w.Unlocked[based] {
			return runeReject("RET_LEAGUE_ERROR", "雅努斯前一难度尚未真实全击杀解锁")
		}
		card, err := leagueGuardianWire(state, index)
		if err != nil {
			return err
		}
		w.Pending = &LeagueProtectAuthorization{LeagueID: state.ID, MemberID: selfID, Host: self.Hostnum, ProtectID: id, DungeonID: did, Week: w.Week, Prepared: now.Unix(), CardID: state.CardID, CardLevel: 1, SupportIndex: index, CardWire: card}
		return nil
	})
	if err != nil {
		return leagueProtectStartFailure(id, err)
	}
	acceptSocialRows(c, rows)
	extra := map[string]any{}
	// 原会话恢复时Pending已经消费，额外卡只从冻结上下文重建。
	if b := c.SelectedAvatarUnsafe().Progress.Battle; b != nil && !b.Finished && b.ActivityContext != nil && b.ActivityContext.LeagueProtect != nil {
		extra = leagueProtectBattleExtra(b.ActivityContext)
	} else if pending := c.SelectedAvatarUnsafe().Progress.LeagueProtect.Pending; pending != nil {
		extra = map[string]any{"activity_id": 14, "league_protect_card": pending.CardWire}
	} else {
		return nil, errors.New("雅努斯冻结授权丢失")
	}
	raw, _ := json.Marshal(extra)
	out, err := s.enterDungeon(ctx, c, []json.RawMessage{json.RawMessage("0"), json.RawMessage(intString(did)), raw})
	if err != nil {
		return leagueProtectStartFailure(id, err)
	}
	result := []Push{push("Avatar", "on_league_protect_start", RetSuccess, id)}
	for _, item := range out {
		if item.Method == "call_client_callback" || item.Method == "callback" || item.Method == "on_rpc_callback" || item.Method == "rpc_callback" {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}
func leagueProtectStartFailure(id int, err error) ([]Push, error) {
	var refusal *runeBusinessError
	if !errors.As(err, &refusal) {
		return nil, err
	}
	code, known := androidLeague.Errors[refusal.Name]
	if !known {
		return nil, err
	}
	return []Push{push("Avatar", "on_league_protect_start", code, id)}, nil
}
func prepareLeagueProtectDungeon(p *Progress, did int, now time.Time) (*ActivityBattleContext, error) {
	if !remainingPolicyEnabled() {
		return nil, errors.New("雅努斯之门本服政策未启用")
	}
	w, err := ensureLeagueProtect(p, now)
	if err != nil {
		return nil, err
	}
	a := w.Pending
	if a == nil || a.DungeonID != did || a.LeagueID != p.LeagueID || a.Week != w.Week || a.Prepared > now.Unix() || a.CardID <= 0 || a.CardWire == nil {
		return nil, errors.New("雅努斯副本没有真实学会冻结授权")
	}
	copy := *a
	w.Pending = nil
	return &ActivityBattleContext{ActivityID: 14, DungeonID: did, PreparedAt: now.Unix(), LeagueProtect: &copy}, nil
}
func recordLeagueProtectStatistics(b *BattleSession, data map[string]any) error {
	if b.ActivityContext == nil || b.ActivityContext.ActivityID != 14 {
		return nil
	}
	a := b.ActivityContext.LeagueProtect
	if a == nil || a.DungeonID != b.DungeonID || b.RewardGranted {
		return errors.New("雅努斯战报没有合法冻结会话")
	}
	raw, ok := data["league_protect_statistics"].(map[string]any)
	if !ok {
		return errors.New("雅努斯战报缺少原生死亡统计")
	}
	parse := func(value any) (int, error) {
		var n float64
		switch v := value.(type) {
		case float64:
			n = v
		case int:
			n = float64(v)
		case int64:
			n = float64(v)
		default:
			return 0, errors.New("雅努斯死亡统计必须为整数")
		}
		if n < 0 || n > math.MaxInt32 || math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) {
			return 0, errors.New("雅努斯死亡统计越界")
		}
		return int(n), nil
	}
	kills, err := parse(raw["kill_count"])
	if err != nil {
		return err
	}
	treasure, err := parse(raw["treasure_role_id"])
	if err != nil {
		return err
	}
	if kills > leagueData("league_protect", a.ProtectID).integer("enemy_count") || treasure != 0 && !containsInt(androidLeagueProtect.Treasures, treasure) {
		return errors.New("雅努斯击杀数或独计宝藏不在原生范围")
	}
	a.Statistics = &LeagueProtectStatistics{kills, treasure}
	return nil
}

// 原文：布阵退出不消耗；无论胜负均消耗，每周二与周四下一次开启恢复。
func consumeLeagueProtectAttempt(p *Progress, b *BattleSession, now time.Time) error {
	if b == nil || b.ActivityContext == nil || b.ActivityContext.ActivityID != 14 {
		return nil
	}
	a := b.ActivityContext.LeagueProtect
	if a == nil || a.DungeonID != b.DungeonID || b.UUID == "" {
		return errors.New("雅努斯计次没有冻结会话")
	}
	w, err := ensureLeagueProtect(p, now)
	if err != nil {
		return err
	}
	if window, exists := w.Consumed[b.UUID]; exists {
		if window != a.Week {
			return errors.New("雅努斯UUID的开放窗口收据不符")
		}
		a.AttemptConsumed = true
		return nil
	}
	if a.AttemptConsumed {
		return errors.New("雅努斯计次标记缺少原子收据")
	}
	weekday := now.In(shanghaiZone).Weekday()
	if (weekday != time.Tuesday && weekday != time.Thursday) || a.Week != w.Week {
		return errors.New("雅努斯布阵授权已离开原开放窗口")
	}
	if w.TotalWeekly >= leagueData("league_base", 1).integer("protect_times_limit") {
		return errors.New("雅努斯本次开放参加次数已达原表上限")
	}
	w.TotalWeekly++
	w.CurrentTimes++
	w.Consumed[b.UUID] = a.Week
	a.AttemptConsumed = true
	return nil
}

// 未完成恢复更换UUID时迁移已消费的冻结窗口收据；两个UUID仅为同一次资格别名。
func migrateLeagueProtectAttempt(p *Progress, b *BattleSession, oldUUID string) error {
	if b == nil || b.ActivityContext == nil || b.ActivityContext.ActivityID != 14 {
		return nil
	}
	a := b.ActivityContext.LeagueProtect
	if a == nil {
		return errors.New("雅努斯恢复缺冻结授权")
	}
	if !a.AttemptConsumed {
		return nil
	}
	w := p.LeagueProtect
	if w == nil || !validObjectID(oldUUID) || !validObjectID(b.UUID) {
		return errors.New("雅努斯恢复缺计次账本或合法UUID")
	}
	window, exists := w.Consumed[oldUUID]
	if !exists || window != a.Week {
		return errors.New("雅努斯恢复原UUID窗口收据不匹配")
	}
	if _, settled := w.Results[oldUUID]; settled {
		return errors.New("已经结算雅努斯不能重开")
	}
	if next, exists := w.Consumed[b.UUID]; exists && next != window {
		return errors.New("雅努斯恢复新UUID已有不同收据")
	}
	w.Consumed[b.UUID] = window
	a.Statistics = nil
	return nil
}
func settleLeagueProtectDungeon(p *Progress, b *BattleSession, win bool, now time.Time) (map[string]any, error) {
	bc := b.ActivityContext
	if bc == nil || bc.LeagueProtect == nil || bc.ActivityID != 14 {
		return nil, errors.New("雅努斯结算上下文无效")
	}
	a := bc.LeagueProtect
	if a.DungeonID != b.DungeonID || a.Statistics == nil || !a.AttemptConsumed {
		return nil, errors.New("雅努斯结算没有原生实际击杀统计")
	}
	w, err := ensureLeagueProtect(p, now)
	if err != nil {
		return nil, err
	}
	if result, done := w.Results[b.UUID]; done {
		a.Result = &result
		return emptyActivityBox(), nil
	}
	if b.RewardGranted {
		return emptyActivityBox(), nil
	}
	row := leagueData("league_protect", a.ProtectID)
	kills := a.Statistics.Kills
	var bands [][]int
	if json.Unmarshal(row["kill_nums"], &bands) != nil {
		return nil, errors.New("雅努斯击杀奖档原表无效")
	}
	ids := row.ids("bonus_ids")
	if len(bands) != len(ids) {
		return nil, errors.New("雅努斯击杀奖档数量不匹配")
	}
	bonus := 0
	for i, band := range bands {
		if len(band) != 2 {
			return nil, errors.New("雅努斯击杀奖档范围无效")
		}
		if kills >= band[0] && kills <= band[1] {
			bonus = ids[i]
			break
		}
	}
	box, err := grantActivityBonus(p, []int{bonus}, now)
	if err != nil {
		return nil, err
	}
	treasureBox := emptyActivityBox()
	if a.Statistics.Treasure != 0 {
		treasureBox, err = grantActivityBonus(p, []int{row.integer("treasure_monster_bonus_id")}, now)
		if err != nil {
			return nil, err
		}
	}
	// 原文明确每击倒一个敌人获学分；原表basic为每普通击倒贡献，独计501不纳入。
	contribution := int64(kills) * int64(row.integer("basic_contribution"))
	if a.Week == w.Week {
		if w.Contribution > math.MaxInt32-contribution {
			return nil, errors.New("雅努斯计次贡献溢出")
		}
		w.Contribution += contribution
		w.Kills[a.ProtectID] = max(w.Kills[a.ProtectID], kills)
		w.ProtectID = a.ProtectID
		w.Updated = now.Unix()
	}
	if win && kills == row.integer("enemy_count") {
		w.Unlocked[a.ProtectID] = true
		advanceAchievementAmount(p, 29, []int{100, a.ProtectID}, 1, now)
	}
	advanceAchievementEvent(p, 21, 14, now)
	result := LeagueProtectResult{win, a.ProtectID, kills, a.Statistics.Treasure, box, treasureBox, a.CardWire, contribution}
	w.Results[b.UUID] = result
	a.Result = &result
	b.RewardGranted = true
	combined := emptyActivityBox()
	if err := mergeActivityBox(combined, box); err != nil {
		return nil, err
	}
	if err := mergeActivityBox(combined, treasureBox); err != nil {
		return nil, err
	}
	return combined, nil
}
func leagueProtectBattleExtra(bc *ActivityBattleContext) map[string]any {
	if bc == nil || bc.ActivityID != 14 || bc.LeagueProtect == nil {
		return map[string]any{}
	}
	return map[string]any{"activity_id": 14, "league_protect_card": leagueGuardianSnapshot(bc.LeagueProtect.CardWire)}
}
func leagueProtectSettlementExtra(b *BattleSession) map[string]any {
	if b == nil || b.ActivityContext == nil || b.ActivityContext.LeagueProtect == nil || b.ActivityContext.LeagueProtect.Result == nil {
		return nil
	}
	r := b.ActivityContext.LeagueProtect.Result
	box, err := battleSettlementBoxWire(r.Box)
	if err != nil {
		return nil
	}
	treasure, err := battleSettlementBoxWire(r.TreasureBox)
	if err != nil {
		return nil
	}
	return map[string]any{"win_flag": r.Win, "protect_id": r.ProtectID, "kill_count": r.Kills, "treasure_role_id": r.Treasure, "box": box, "treasure_box": treasure, "league_protect_card": leagueGuardianSnapshot(r.Card), "delta_contribution": r.Contribution}
}

// 原生dungeon_mgr消费league_activity_battle_result后自行调on_league_protect_end，禁止额外重复弹结果。
func leagueProtectCompletionPushes(p Progress, b *BattleSession) []Push {
	if b == nil || b.ActivityContext == nil || b.ActivityContext.ActivityID != 14 {
		return nil
	}
	out := []Push{}
	for key, value := range leagueProtectProperties(p) {
		out = append(out, push("Avatar", "client_prop_changed", []any{key, value}))
	}
	out = append(out, push("Avatar", "client_prop_changed", []any{"achves", achievementProperties(p)}), push("Avatar", "client_prop_changed", []any{"achv_value", achievementPoints(p)}))
	return out
}
