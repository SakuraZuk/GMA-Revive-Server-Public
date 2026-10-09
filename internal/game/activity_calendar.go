package game

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// 原生年兽日榜23:59:59（不含周三）、山海各挑战rank_record_day七日窗口/22点。
// Calendar保存冻结名次/奖表/缺失历史原因，邮件资产与收据同一个全服事务。
type ActivityCalendar struct {
	NianDay         string                         `json:"nian_day"`
	NianWeek        string                         `json:"nian_week"`
	NianDaily       map[int]ActivityRankReceipt    `json:"nian_daily"`
	NianWeeks       map[string]ActivityNianWeek    `json:"nian_weeks"`
	Mountain        map[int]ActivityRankReceipt    `json:"mountain"`
	MountainSeasons map[string]ActivityRankReceipt `json:"mountain_seasons,omitempty"`
	NianTotal       *NianTotalSnapshot             `json:"nian_total,omitempty"`
}
type ActivityRankReceipt struct {
	Cutoff     int64   `json:"cutoff"`
	ObservedAt int64   `json:"observed_at"`
	Closed     bool    `json:"closed"`
	Rank       int     `json:"rank"`
	Score      []int64 `json:"score,omitempty"`
	Bonus      int     `json:"bonus_id,omitempty"`
	Issued     bool    `json:"issued"`
	Missing    string  `json:"missing,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}
type ActivityNianWeek struct {
	Cutoff    int64    `json:"cutoff"`
	Ranks     [3]int   `json:"sub_ranks"`
	Damage    [3]int64 `json:"sub_damage"`
	Pending   string   `json:"pending"`
	TotalRank int      `json:"total_rank,omitempty"`
	Score     int64    `json:"score,omitempty"`
	Joined    int      `json:"joined,omitempty"`
	Bonus     int      `json:"bonus_id,omitempty"`
	Issued    bool     `json:"issued,omitempty"`
}
type activityAwardBus struct {
	mu       sync.Mutex
	key      string
	lastTime int64
}

var activityAwardBuses sync.Map

func activityAwardMethod(method string) bool {
	if method == "enter_dungeon" || method == "do_command" || method == "client_need_recover_battle" || method == "query_rank_list" || method == "query_mountain_sea_own_rank" {
		return true
	}
	for _, prefix := range []string{"mountainsea_", "change_guard_", "receive_mountain_", "receive_monster_nian_", "open_grid", "summer_", "start_fishing", "check_fishing", "receive_fish_", "receive_all_fish_", "receive_summer_", "enter_wangyan_", "wangyan_", "submit_wangyan_", "enter_miku_", "activate_handbook_", "set_miku_", "leave_miku_", "receive_miku_", "enter_story_dungeon", "enter_curse_", "cthulhu_", "refresh_cthulhu_", "add_cthulhu_", "set_cthulhu_", "exam_", "prev_exam_", "receive_exam_", "receive_storyline_", "enter_house_frage", "house_frage_", "open_game_site", "buy_frage_", "get_house_board_"} {
		if strings.HasPrefix(method, prefix) {
			return true
		}
	}
	return false
}

func activityCutoffs(now time.Time) (time.Time, time.Time) {
	local := now.In(shanghaiZone)
	day := time.Date(local.Year(), local.Month(), local.Day(), 23, 59, 59, 0, shanghaiZone)
	if local.Before(day) {
		day = day.AddDate(0, 0, -1)
	}
	offset := (int(day.Weekday()) - int(time.Wednesday) + 7) % 7
	return day, day.AddDate(0, 0, -offset)
}
func mountainRankCutoff(pid int) int64 {
	window := activityData("mountain_game_bonus", pid).ids("rank_record_day")
	if len(window) != 2 || window[1]-window[0] != 6 {
		return 0
	}
	schedule := activitySchedule(203)
	if schedule.Begin <= 0 {
		return 0
	}
	begin := time.Unix(schedule.Begin, 0).In(shanghaiZone)
	hour := activityData("mountain_game_base_rule", 1).integer("rank_settlement_hour")
	return time.Date(begin.Year(), begin.Month(), begin.Day(), hour, 0, 0, 0, shanghaiZone).AddDate(0, 0, window[1]-1).Unix()
}
func mountainRankCompetitionOpen(pid int, now time.Time) bool {
	cutoff := mountainRankCutoff(pid)
	return cutoff > 0 && now.Unix() < cutoff
}
func (s *Service) refreshActivityAwards(ctx context.Context, c *Connection) error {
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil
	}
	value, _ := activityAwardBuses.LoadOrStore(s, &activityAwardBus{})
	bus := value.(*activityAwardBus)
	bus.mu.Lock()
	defer bus.mu.Unlock()
	// 必须在串行锁内取时刻；排队较早的请求不代表服务器真的回拨。
	if err := ctx.Err(); err != nil {
		return err
	}
	now := s.Now()
	day, week := activityCutoffs(now)
	key := day.Format("2006-01-02") + "/" + week.Format("2006-01-02")
	if remainingPolicyEnabled() {
		key += fmt.Sprintf("/hour:%d", now.Unix()/3600)
	}
	for pid := 1; pid <= 7; pid++ {
		key += fmt.Sprintf("/%d:%t", mountainRankCutoff(pid), now.Unix() >= mountainRankCutoff(pid))
		if remainingPolicyEnabled() {
			season, _, end := mountainSeasonAt(pid, now)
			key += fmt.Sprintf("/%d:%d:%t", pid, season, now.Unix() >= end)
		}
	}
	if now.Unix() < bus.lastTime {
		return errors.New("活动排行周期时钟回拨")
	}
	if bus.key == key && c.SelectedAvatarUnsafe().Progress.Activities.Calendar != nil {
		return nil
	}
	if bus.key == key {
		// 新角色或旧连接快照缺日历时，先只锁本角色回载/初始化。
		// 没有任何排行成绩和历史的角色不需要扫描并锁住全服背包。
		needsGlobal := errors.New("该角色已有排行历史，必须全服结算")
		err := s.updateProgress(ctx, c, func(p *Progress) error {
			if p.Activities.Calendar != nil {
				return nil
			}
			if activityCalendarHasRankHistory(*p) {
				return needsGlobal
			}
			av := c.SelectedAvatarUnsafe()
			av.Progress = *p
			if err := s.settleActivityCalendar(map[string]*Avatar{hexOf(av.OID): &av}, now, day, week); err != nil {
				return err
			}
			*p = av.Progress
			return nil
		})
		if !errors.Is(err, needsGlobal) {
			return err
		}
	}
	rows, err := store.UpdateAllSocial(ctx, func(v map[string]*Avatar) error { return s.settleActivityCalendar(v, now, day, week) })
	if err != nil {
		return err
	}
	bus.key, bus.lastTime = key, now.Unix()
	acceptSocialRows(c, rows)
	return nil
}

func activityCalendarHasRankHistory(p Progress) bool {
	if len(p.Activities.NianHistory) != 0 {
		return true
	}
	has := func(d ActivityProgress) bool {
		return d.Ranked || d.RankAt != 0 || d.RankHard != 0 || d.RankDamage != 0
	}
	for _, d := range p.Activities.Nian {
		if has(d) {
			return true
		}
	}
	if m := p.Activities.Mountain; m != nil {
		for _, d := range m.Dungeons {
			if has(d) {
				return true
			}
		}
		for _, seasons := range m.Seasons {
			for _, d := range seasons {
				if has(d) {
					return true
				}
			}
		}
	}
	return false
}
func activityRankBonus(table string, rank int) (int, error) {
	if rank < 1 {
		return 0, nil
	}
	var entries []struct {
		Value struct {
			Range []int `json:"rank_range"`
			Bonus int   `json:"bonus_id"`
		} `json:"值"`
	}
	if json.Unmarshal(androidActivities[table]["键值条目"], &entries) != nil {
		return 0, errors.New("活动排名奖励区间配置无效")
	}
	bonus := 0
	for _, entry := range entries {
		if len(entry.Value.Range) != 2 {
			return 0, errors.New("活动排名奖励区间缺失")
		}
		if rank >= entry.Value.Range[0] && rank <= entry.Value.Range[1] {
			if bonus != 0 {
				return 0, errors.New("活动排名奖励区间重叠")
			}
			bonus = entry.Value.Bonus
		}
	}
	return bonus, nil
}
func (s *Service) issueActivityRankMail(p *Progress, bid int, receipt string, now time.Time) error {
	r := activityData("bonus", bid)
	var fixed [][]int64
	if len(r) == 0 || json.Unmarshal(r["fixed_items"], &fixed) != nil || len(r.ids("random_runes"))+len(r.ids("random_item_libs")) > 0 || string(r["inner_bonus_id"]) != "null" {
		return errors.New("活动排名邮件奖励配置未支持")
	}
	for _, name := range []string{"random_items", "random_runes", "random_item_libs"} {
		var values []any
		if json.Unmarshal(r[name], &values) != nil || len(values) != 0 {
			return errors.New("活动排名邮件不可猜随机奖励")
		}
	}
	items := map[int]int64{}
	for _, row := range fixed {
		if len(row) != 2 || row[0] <= 0 || row[1] <= 0 || items[int(row[0])] > math.MaxInt64-row[1] {
			return errors.New("活动排名邮件资产无效")
		}
		items[int(row[0])] += row[1]
	}
	var text struct {
		Title   string `json:"mail_title"`
		Content string `json:"mail_content"`
	}
	if json.Unmarshal(socialCatalog.Tables["mail_template"][intString(r.integer("mail_template_id"))], &text) != nil || text.Title == "" || text.Content == "" || strings.Contains(text.Content, "%s") {
		return errors.New("活动原生排名邮件模板缺失或参数未取证")
	}
	hash := sha256.Sum256([]byte("activity-rank:" + receipt))
	uuid := fmt.Sprintf("%x", hash[:12])
	for _, mail := range p.ShortMailInfo {
		if string(mail.oid()) == uuid {
			return nil
		}
	}
	mid := 1
	for id := range p.ShortMailInfo {
		if id >= mid {
			if id == math.MaxInt32 {
				return errors.New("活动排名邮件编号溢出")
			}
			mid = id + 1
		}
	}
	return s.prepareAndInsertMail(p, Mail{MID: mid, UUID: uuid, Title: text.Title, Content: text.Content, Sender: "阿克夏馆务处", Attachments: items, CreatedAt: now.Unix()})
}
func activityCalendarRanks(v map[string]*Avatar, kind, sub int, cutoff int64) map[string]ActivityRankEntry {
	groups := map[int][]ActivityRankEntry{}
	for _, av := range v {
		entry, ok := activityRankValue(*av, kind, sub)
		if !ok {
			continue
		}
		if kind == 10 {
			d := av.Progress.Activities.Nian[sub]
			if d.RankAt <= 0 || d.RankAt > cutoff {
				continue
			}
		}
		groups[av.Hostnum] = append(groups[av.Hostnum], entry)
	}
	out := map[string]ActivityRankEntry{}
	for _, rows := range groups {
		sort.Slice(rows, func(i, j int) bool {
			for k := range rows[i].Score {
				if rows[i].Score[k] != rows[j].Score[k] {
					return rows[i].Score[k] > rows[j].Score[k]
				}
			}
			return hexOf(rows[i].Avatar.OID) < hexOf(rows[j].Avatar.OID)
		})
		for i, entry := range rows {
			entry.Rank = i + 1
			out[hexOf(entry.Avatar.OID)] = entry
		}
	}
	return out
}

// 旧版本或漏掉截止接线时，截止后的best已覆盖旧值，整个同服分榜缺历史。
// 不能删除该角色后给其他角色生成更高的虚假旧名次。
func activityCalendarIncomplete(v map[string]*Avatar, sub int, cutoff int64, key string, weekly bool) map[int]bool {
	missing := map[int]bool{}
	for _, av := range v {
		cal := av.Progress.Activities.Calendar
		if cal == nil {
			continue
		}
		previous := cal.NianDay
		if weekly {
			previous = cal.NianWeek
		}
		if previous == "" || previous >= key {
			continue
		}
		d := av.Progress.Activities.Nian[sub]
		if d.Ranked && d.RankDamage > 0 && (d.RankAt <= 0 || d.RankAt > cutoff) {
			missing[av.Hostnum] = true
		}
	}
	return missing
}
func (s *Service) settleActivityCalendar(v map[string]*Avatar, now, day, week time.Time) error {
	dayKey, weekKey := day.Format("2006-01-02"), week.Format("2006-01-02")
	ids := []string{}
	for id := range v {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	dayRanks := map[int]map[string]ActivityRankEntry{}
	for _, did := range []int{20200001, 20200002, 20200003} {
		dayRanks[did] = activityCalendarRanks(v, 10, did, day.Unix())
	}
	weekRanks := map[int]map[string]ActivityRankEntry{}
	dayMissing := map[int]map[int]bool{}
	weekMissing := map[int]map[int]bool{}
	for _, did := range []int{20200001, 20200002, 20200003} {
		weekRanks[did] = activityCalendarRanks(v, 10, did, week.Unix())
		dayMissing[did] = activityCalendarIncomplete(v, did, day.Unix(), dayKey, false)
		weekMissing[did] = activityCalendarIncomplete(v, did, week.Unix(), weekKey, true)
	}
	mountainRanks := map[int]map[string]ActivityRankEntry{}
	for pid := 1; pid <= 7; pid++ {
		mountainRanks[pid] = activityCalendarRanks(v, 8, pid, mountainRankCutoff(pid))
	}
	var totalWeek map[string]NianTotalSnapshot
	totalWeekMissing := map[int]bool{}
	if remainingPolicyEnabled() {
		totalWeek = compositeNianRanks(v, week.Unix()+1, week.AddDate(0, 0, -7).Unix())
		// 新政策上线前没有小时历史时，截止后覆盖旧best不能重建旧奖。
		for _, av := range v {
			for _, did := range []int{20200001, 20200002, 20200003} {
				d := av.Progress.Activities.Nian[did]
				if d.Ranked && d.RankDamage > 0 && (d.RankAt <= 0 || d.RankAt > week.Unix()) {
					if _, ok := nianScoreBefore(av.Progress, did, week.Unix()+1, week.AddDate(0, 0, -7).Unix()); !ok && len(av.Progress.Activities.NianHistory[did]) == 0 {
						totalWeekMissing[av.Hostnum] = true
					}
				}
			}
		}
		// 周三23:59:59清周成绩前冻结当日23点榜，不能清空当前小时展示。
		freezeNianHourRanking(v, now)
	}
	for _, id := range ids {
		av := v[id]
		p := &av.Progress
		if p.Activities.Calendar == nil {
			p.Activities.Calendar = &ActivityCalendar{}
		}
		cal := p.Activities.Calendar
		if cal.NianDay > dayKey || cal.NianWeek > weekKey {
			return errors.New("活动排名奖励日期回拨")
		}
		if cal.NianDaily == nil {
			cal.NianDaily = map[int]ActivityRankReceipt{}
		}
		if cal.NianWeeks == nil {
			cal.NianWeeks = map[string]ActivityNianWeek{}
		}
		if cal.Mountain == nil {
			cal.Mountain = map[int]ActivityRankReceipt{}
		}
		if cal.NianDay != "" && cal.NianDay < dayKey {
			previous, err := time.ParseInLocation("2006-01-02", cal.NianDay, shanghaiZone)
			if err != nil {
				return errors.New("年兽日榜账本日期无效")
			}
			complete := previous.AddDate(0, 0, 1).Format("2006-01-02") == dayKey
			for _, did := range []int{20200001, 20200002, 20200003} {
				record := ActivityRankReceipt{Cutoff: day.Unix(), ObservedAt: now.Unix(), Closed: true}
				if !complete || dayMissing[did][av.Hostnum] {
					record.Missing = "缺少历史日榜截止快照"
				} else if entry, exists := dayRanks[did][id]; exists {
					openDay, err := activityOpen(*p, 328, av.Info.Level, day)
					if err == nil && containsInt(activityData("monster_nian_dungeon", did).ids("open_days"), openDay) {
						record.Rank, record.Score = entry.Rank, entry.Score
						// 原生monster_nian_rank_reward明确日奖不含周三；仍保留截止名次供核查。
						if day.Weekday() == time.Wednesday {
							record.Reason = "原生日奖不含周三，仅记录截止名次"
							cal.NianDaily[did] = record
							continue
						}
						bonus, err := activityRankBonus("monster_nian_daily_rank", record.Rank)
						if err != nil {
							return err
						}
						record.Bonus = bonus
						if bonus > 0 {
							if err = s.issueActivityRankMail(p, bonus, fmt.Sprintf("nian:%s:%s:%d", id, dayKey, did), now); err != nil {
								return err
							}
							record.Issued = true
						}
					}
				}
				cal.NianDaily[did] = record
			}
		}
		cal.NianDay = dayKey
		// 周三周奖采用已批准本服综合公式；缺失历史快照仍保留待处理收据。
		if cal.NianWeek != "" && cal.NianWeek < weekKey {
			previous, err := time.ParseInLocation("2006-01-02", cal.NianWeek, shanghaiZone)
			if err != nil {
				return errors.New("年兽周榜账本日期无效")
			}
			record := ActivityNianWeek{Cutoff: week.Unix(), Pending: "三分榜名次综合公式与空榜惩罚未取证"}
			if previous.AddDate(0, 0, 7).Format("2006-01-02") == weekKey {
				for i, did := range []int{20200001, 20200002, 20200003} {
					if weekMissing[did][av.Hostnum] {
						record.Pending = "截止后的best已覆盖旧值，缺历史周榜快照"
						record.Ranks = [3]int{}
						record.Damage = [3]int64{}
						break
					}
					if entry, ok := weekRanks[did][id]; ok {
						record.Ranks[i] = entry.Rank
						record.Damage[i] = entry.Score[0]
					}
				}
			} else {
				record.Pending = "缺少历史周榜截止快照"
			}
			if remainingPolicyEnabled() && previous.AddDate(0, 0, 7).Format("2006-01-02") == weekKey && !totalWeekMissing[av.Hostnum] {
				record.Pending = ""
				if total, ok := totalWeek[id]; ok {
					record.Ranks = total.SubRanks
					for i, did := range []int{20200001, 20200002, 20200003} {
						if score, exists := nianScoreBefore(*p, did, week.Unix()+1, week.AddDate(0, 0, -7).Unix()); exists {
							record.Damage[i] = score.Damage
						}
					}
					record.TotalRank = total.Rank
					record.Score = total.Score
					record.Joined = total.Joined
					record.Pending = ""
					bonus, err := activityRankBonus("monster_nian_total_rank", total.Rank)
					if err != nil {
						return err
					}
					record.Bonus = bonus
					if bonus > 0 {
						if err = s.issueActivityRankMail(p, bonus, fmt.Sprintf("nian-week:%s:%s", id, weekKey), now); err != nil {
							return err
						}
						record.Issued = true
					}
				}
			}
			cal.NianWeeks[weekKey] = record
			for did, d := range p.Activities.Nian {
				d.RankDamage, d.RankAt = 0, 0
				d.RankCards = nil
				d.Ranked = false
				p.Activities.Nian[did] = d
			}
			if remainingPolicyEnabled() {
				clearNianHistoryThrough(p, week.Unix())
			}
		}
		cal.NianWeek = weekKey
		for pid := 1; pid <= 7; pid++ {
			cutoff := mountainRankCutoff(pid)
			if cutoff == 0 {
				continue
			}
			record, exists := cal.Mountain[pid]
			if exists && record.Cutoff != cutoff {
				return errors.New("山海排行日程变化，历史奖需显式迁移")
			}
			if !exists {
				record = ActivityRankReceipt{Cutoff: cutoff, ObservedAt: now.Unix()}
				if now.Unix() >= cutoff {
					record.Closed = true
					record.Missing = "首次见到已结束挑战，缺历史截止快照"
				}
			}
			if !record.Closed && now.Unix() >= cutoff {
				record.Closed = true
				if entry, ok := mountainRanks[pid][id]; ok {
					record.Rank, record.Score = entry.Rank, entry.Score
					bonus, err := activityRankBonus("mountain_game_rank", entry.Rank)
					if err != nil {
						return err
					}
					record.Bonus = bonus
					if bonus > 0 {
						if err = s.issueActivityRankMail(p, bonus, fmt.Sprintf("mountain:%s:%d:%d", id, pid, cutoff), now); err != nil {
							return err
						}
						record.Issued = true
					}
				}
			}
			cal.Mountain[pid] = record
		}
	}
	return s.refreshMountainCycles(v, now)
}
