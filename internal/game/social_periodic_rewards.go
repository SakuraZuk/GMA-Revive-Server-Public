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

type pvpAwardBus struct {
	mu       sync.Mutex
	key      string
	lastTime int64
}

var pvpAwardBuses sync.Map

// refreshPvpAwards在日界/21点/周日21点/赛季边界串行一次全量事务；离线角色同样入存档。
func (s *Service) refreshPvpAwards(ctx context.Context, c *Connection) error {
	if c != nil && s.backgroundPeriodicStarted.Load() {
		return nil
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil
	}
	value, _ := pvpAwardBuses.LoadOrStore(s, &pvpAwardBus{})
	bus := value.(*pvpAwardBus)
	// 玩家PVP请求只允许抢占空闲结算器；后台正在结算时直接沿用旧的
	// 幂等状态，不让请求排队占住连接和数据库连接池。
	if !bus.mu.TryLock() {
		return nil
	}
	defer bus.mu.Unlock()
	return s.refreshPvpAwardsLocked(ctx, c, store, bus)
}

// 后台时钟不能在另一个连接的全服周期事务之后排队占住自己的连接锁。
// 忙时跳过这一轮，不推进周期键；后续时钟继续按原事务和原幂等收据重试。
func (s *Service) tryRefreshPvpAwards(ctx context.Context, c *Connection) (bool, error) {
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return true, nil
	}
	value, _ := pvpAwardBuses.LoadOrStore(s, &pvpAwardBus{})
	bus := value.(*pvpAwardBus)
	if !bus.mu.TryLock() {
		return false, nil
	}
	defer bus.mu.Unlock()
	return true, s.refreshPvpAwardsLocked(ctx, c, store, bus)
}

func (s *Service) refreshPvpAwardsLocked(ctx context.Context, c *Connection, store SocialAccounts, bus *pvpAwardBus) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := s.Now()
	day, week := pvpAwardCutoffs(now)
	key := fmt.Sprintf("%s/%s/%s/%d", socialDay(now), day, week, syncPvpSeasonID(now))
	if now.Unix() < bus.lastTime {
		return errors.New("社交竞技周期时钟回拨")
	}
	if bus.key == key {
		return nil
	}
	if !s.periodicSettlementMu.TryLock() {
		return nil
	}
	defer s.periodicSettlementMu.Unlock()
	rows, err := updatePeriodicSocial(ctx, store, func(v map[string]*Avatar) error { return s.settlePvpCalendar(v, now, day, week) })
	if err != nil {
		return err
	}
	bus.key = key
	bus.lastTime = now.Unix()
	s.acceptPeriodicRows(c, rows)
	return nil
}
func pvpAwardCutoffs(now time.Time) (string, string) {
	local := now.In(time.FixedZone("北京时间", 8*3600))
	closed := time.Date(local.Year(), local.Month(), local.Day(), 21, 0, 0, 0, local.Location())
	if local.Before(closed) {
		closed = closed.AddDate(0, 0, -1)
	}
	day := closed.Format("2006-01-02")
	week := closed.AddDate(0, 0, -int(closed.Weekday())).Format("2006-01-02")
	return day, week
}
func pvpRankBonus(table string, rank int) (int, error) {
	if rank < 1 {
		return 0, nil
	}
	found := 0
	for _, raw := range socialCatalog.Tables[table] {
		var row struct {
			Range []int `json:"rank_range"`
			Bonus int   `json:"bonus_id"`
		}
		if json.Unmarshal(raw, &row) != nil || len(row.Range) != 2 {
			return 0, errors.New("竞技排名奖励表无效")
		}
		if rank >= row.Range[0] && rank <= row.Range[1] {
			if found != 0 {
				return 0, errors.New("竞技排名奖励区间重叠")
			}
			found = row.Bonus
		}
	}
	return found, nil
}
func fixedPvpMailBonus(bid int) (map[int]int64, int, error) {
	raw, exists := socialCatalog.Tables["bonus"][intString(bid)]
	if !exists {
		return nil, 0, errors.New("周期奖励表缺失")
	}
	var row struct {
		Fixed    [][]int64 `json:"fixed_items"`
		Random   []any     `json:"random_items"`
		Runes    []any     `json:"random_runes"`
		Libs     []any     `json:"random_item_libs"`
		Inner    any       `json:"inner_bonus_id"`
		Template int       `json:"mail_template_id"`
	}
	if json.Unmarshal(raw, &row) != nil || len(row.Random)+len(row.Runes)+len(row.Libs) > 0 || row.Inner != nil {
		return nil, 0, errors.New("周期奖励存在尚无取证的随机或嵌套配置")
	}
	result := map[int]int64{}
	for _, entry := range row.Fixed {
		if len(entry) != 2 || entry[0] <= 0 || entry[1] <= 0 || result[int(entry[0])] > math.MaxInt64-entry[1] {
			return nil, 0, errors.New("周期奖励数值无效")
		}
		result[int(entry[0])] += entry[1]
	}
	return result, row.Template, nil
}
func (s *Service) issuePvpCalendarMail(p *Progress, bid int, receipt string, argument int, now time.Time) error {
	items, template, err := fixedPvpMailBonus(bid)
	if err != nil {
		return err
	}
	var text struct {
		Title   string `json:"mail_title"`
		Content string `json:"mail_content"`
	}
	if json.Unmarshal(socialCatalog.Tables["mail_template"][intString(template)], &text) != nil || text.Title == "" {
		return errors.New("周期奖励原生邮件模板缺失")
	}
	if strings.Count(text.Content, "%s") != 1 {
		return errors.New("周期奖励邮件模板参数数量不符")
	}
	text.Content = strings.Replace(text.Content, "%s", intString(argument), 1)
	hash := sha256.Sum256([]byte("pvp-mail:" + receipt))
	uuid := fmt.Sprintf("%x", hash[:12])
	for _, m := range p.ShortMailInfo {
		if string(m.oid()) == uuid {
			return nil
		}
	}
	mid := 1
	for id := range p.ShortMailInfo {
		if id >= mid {
			if id == math.MaxInt32 {
				return errors.New("周期奖励邮件编号溢出")
			}
			mid = id + 1
		}
	}
	return s.prepareAndInsertMail(p, Mail{MID: mid, UUID: uuid, Title: text.Title, Content: text.Content, Sender: "阿克夏馆务处", Attachments: items, CreatedAt: now.Unix()})
}
func (s *Service) settlePvpCalendar(v map[string]*Avatar, now time.Time, day, week string) error {
	ids := []string{}
	asyncIDs := []string{}
	currentPeriod := syncPvpSeasonID(now)
	for id, av := range v {
		ids = append(ids, id)
		if asyncUnlocked(av.Progress) && av.Progress.AsyncPvp.Score > 0 {
			asyncIDs = append(asyncIDs, id)
		}
	}
	sort.Strings(ids)
	sort.Slice(asyncIDs, func(i, j int) bool {
		a, b := v[asyncIDs[i]].Progress.AsyncPvp.Score, v[asyncIDs[j]].Progress.AsyncPvp.Score
		if a != b {
			return a > b
		}
		return asyncIDs[i] < asyncIDs[j]
	})
	asyncRanks := map[string]int{}
	for rank, id := range asyncIDs {
		asyncRanks[id] = rank + 1
	}
	// 当前进程首次遇到玩家时只建立周期基线；缺失的历史21点快照绝不伪造补奖。
	for _, id := range ids {
		p := &v[id].Progress
		a := &p.AsyncPvp
		if a.RewardDay > day || a.RewardWeek > week {
			return errors.New("异步奖励周期时钟回拨")
		}
		// 零积分表示从未初始化竞技；空奖日期不逐日写全服，首次进入建立基线。
		// 保留旧日期及回拨校验，不删除历史数据；非零积分沿用完整结算。
		if a.Score != 0 {
			if a.RewardDay == "" {
				a.RewardDay = day
			} else if a.RewardDay < day {
				if asyncUnlocked(*p) && a.Score > 0 {
					rule, err := asyncScoreFor(a.Score)
					if err != nil {
						return err
					}
					if err = s.issuePvpCalendarMail(p, rule.Bonus, "async-day:"+id+":"+day, a.Score, now); err != nil {
						return err
					}
				}
				a.RewardDay = day
			}
			if a.RewardWeek == "" {
				a.RewardWeek = week
			} else if a.RewardWeek < week {
				if rank := asyncRanks[id]; rank > 0 {
					bonus, err := pvpRankBonus("asyn_pvp_bonus_rule", rank)
					if err != nil {
						return err
					}
					if bonus > 0 {
						if err = s.issuePvpCalendarMail(p, bonus, "async-week:"+id+":"+week, rank, now); err != nil {
							return err
						}
					}
				}
				a.RewardWeek = week
			}
		}
		assist := &p.Social.Assist
		today := socialDay(now)
		if assist.Day > today {
			return errors.New("被动助战奖励时钟回拨")
		}
		if assist.Day != "" && assist.Day < today && assist.Passive > 0 {
			count := assist.Passive
			if count > 50 {
				count = 50
			}
			var bonus int
			if raw, exists := socialCatalog.Tables["assist_passive_bonus"][intString(count)]; exists {
				if json.Unmarshal(raw, &bonus) != nil {
					return errors.New("被动助战奖励目录无效")
				}
				if err := s.issuePvpCalendarMail(p, bonus, "assist-day:"+id+":"+assist.Day, assist.Passive, now); err != nil {
					return err
				}
			}
		}
		// 无助战计数的角色没有跨日清理或发奖任务，不为日期空写全服存档。
		// 实际进入助战或使用助战的路径仍在本角色事务内建立当天基线。
		if len(assist.Active) != 0 || assist.Passive != 0 || assist.RewardTimes != 0 {
			if err := ensureFriendAssistDay(p, now); err != nil {
				return err
			}
		}
	}
	// 只为尚未本地跨期的完整当前赛季冻结排名；缺历史全服截止快照的旧期保持Rank=0且不伪造排名奖。
	periods := map[int]bool{}
	for _, av := range v {
		if period := av.Progress.SyncPvpMeta.Period; period > 0 && period < currentPeriod {
			periods[period] = true
		}
	}
	for period := range periods {
		players := []string{}
		complete := true
		for _, id := range ids {
			p := v[id].Progress
			if p.SyncPvpMeta.BeginPeriod == 0 || p.SyncPvpMeta.BeginPeriod > period {
				continue
			}
			if p.SyncPvpMeta.Period > period {
				if _, exists := p.SyncPvpMeta.History[period]; !exists {
					complete = false
				}
			}
			players = append(players, id)
		}
		if !complete {
			continue
		}
		score := func(id string) int {
			p := v[id].Progress
			if row, exists := p.SyncPvpMeta.History[period]; exists {
				return row.Score
			}
			return p.SyncPvpScore
		}
		sort.Slice(players, func(i, j int) bool {
			a, b := score(players[i]), score(players[j])
			if a != b {
				return a > b
			}
			return players[i] < players[j]
		})
		for index, id := range players {
			p := &v[id].Progress
			meta := &p.SyncPvpMeta
			if meta.History == nil {
				meta.History = map[int]SyncPvpPeriod{}
			}
			record, exists := meta.History[period]
			if exists && record.Rank > 0 {
				continue
			}
			if !exists {
				rule, err := syncPvpScoreRuleFor(p.SyncPvpScore)
				if err != nil {
					return err
				}
				record = SyncPvpPeriod{Score: p.SyncPvpScore, Highest: p.SyncPvpHighestScore, Wins: p.SyncPvpWeeklyWins, DivisionBonus: rule.DivisionBonus}
			}
			record.Rank = index + 1
			bonus, err := pvpRankBonus("sync_pvp_rank_bonus", record.Rank)
			if err != nil {
				return err
			}
			record.RankBonus = bonus
			meta.History[period] = record
		}
	}
	return nil
}
