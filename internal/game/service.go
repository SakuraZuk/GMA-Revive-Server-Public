// Package game 实现已证实的登录、时间同步和双通道热修业务语义。
// 参数编解码、实体创建和加密交接由 cmd/gameserver 的传输适配器负责。
package game

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log"
	"os"
	"sync"
	"time"

	"hs-server/internal/hotfix"
	"hs-server/internal/nativeengine"
	"sort"
)

const (
	RetSuccess      = 0
	RetAccountEmpty = 9001
	RetAuthFailed   = 9002
	RetHostnumEmpty = 9011
	// 9012 复用客户端 error_code.RET_LOGIN_ACCOUNT_ERROR（out/login-constants.txt）。
	RetAccountError = 9012
)

// 账号边界错误语义：登录与注册分离（AUDIT-2026-10-05 P0），
var (
	ErrAccountNotFound = errors.New("账号不存在")
	ErrAccountExists   = errors.New("账号已存在")
	ErrSDKUnavailable  = errors.New("SDK 票据鉴权未实现")
)

// intList 容忍 BSON 空数组在传输上与空对象无法区分的固有歧义（{} / null → 空列表）。
type intList []int

func (l *intList) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "{}" || s == "null" || s == "" {
		*l = nil
		return nil
	}
	return json.Unmarshal(b, (*[]int)(l))
}

// ClientInfo 六个基础键已由 B9F5C696 指令确认；可选键原样保留。
type ClientInfo struct {
	Hostnum          int             `json:"hostnum"`
	Account          string          `json:"account"`
	Password         string          `json:"password"`
	HotfixIndex      int             `json:"hotfix_index"`
	NeedGuideIDs     intList         `json:"need_guide_ids"`
	ConnType         int             `json:"conn_type"`
	GMRoleEnterType  *int            `json:"gm_role_enter_type,omitempty"`
	GMOnlineTimeLeft *int            `json:"gm_online_time_left,omitempty"`
	RegisterInfo     json.RawMessage `json:"register_info,omitempty"`
}

// AvatarInfo 仅包含登录 UI 明确读取的字段；图标编号必须来自有效客户端数据。
type AvatarInfo struct {
	Nickname           string `json:"nickname"`
	Level              int    `json:"level"`
	HeadID             int    `json:"head_id"`
	CustomHeadImageURL string `json:"custom_head_image_url"`
	HeadBoxID          int    `json:"head_box_id"`
}

type Avatar struct {
	OID         []byte     `json:"-"`
	UID         int64      `json:"-"`
	Account     string     `json:"-"`
	Gender      int        `json:"-"`
	NicknameSet bool       `json:"-"`
	CreatedAt   time.Time  `json:"-"`
	Progress    Progress   `json:"-"`
	Hostnum     int        `json:"hostnum"`
	Info        AvatarInfo `json:"avatar_info"`
}

type Identity struct {
	Account string
	Avatars []Avatar
}

// Accounts 是真实账号鉴权及角色持久化的替换边界；SDK 鉴权必须单独实现。
// QuickLogin 只校验既有账号；Register 是唯一的建号入口；SDKLogin 在真实
// 票据鉴权实现前必须拒绝，不得自动建号。
type Accounts interface {
	QuickLogin(context.Context, ClientInfo) (Identity, error)
	Register(context.Context, ClientInfo) (Identity, error)
	SDKLogin(context.Context, ClientInfo, json.RawMessage) (Identity, error)
	// SyncPvpRankings 返回跨账号同步 PVP 积分榜（真账号，不含幽灵填充）。
	SyncPvpRankings(context.Context, int, ...int) ([]SyncPvpRankEntry, error)
	SyncPvpRank(context.Context, []byte, int) (int, error)
}

// SyncPvpRankEntry 是跨账号排行榜行（真账号）。
type SyncPvpRankEntry struct {
	OID      []byte
	UID      int64
	Hostnum  int
	Nickname string
	Level    int
	Score    int
}

type FixtureAccount struct {
	Password string   `json:"password"`
	Avatars  []Avatar `json:"avatars"`
}

// FixtureAccounts 仅用于本机开发；不是网易 SDK 鉴权或正式数据库。
type FixtureAccounts struct {
	mu        sync.Mutex
	records   map[string]FixtureAccount
	reconnect map[string]reconnectRecord
	nextUID   int64
}

func NewFixtureAccounts(records map[string]FixtureAccount) *FixtureAccounts {
	if records == nil {
		records = make(map[string]FixtureAccount)
	}
	return &FixtureAccounts{records: records}
}

func LoadFixtures(path string) (*FixtureAccounts, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var accounts map[string]FixtureAccount
	err = json.Unmarshal(body, &accounts)
	return NewFixtureAccounts(accounts), err
}

func (a *FixtureAccounts) QuickLogin(ctx context.Context, info ClientInfo) (Identity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Identity{}, err
	}
	// 开发夹具与 dbstore 同契约：登录只认既有账号；空角色列表会令登录 UI
	// on_get_all_avatars 619 行异常（实机证实），故按 hostnum 补默认角色。
	if info.Account == "" || info.Password == "" || info.Hostnum <= 0 {
		return Identity{}, errors.New("开发账号登录参数无效")
	}
	record, ok := a.records[info.Account]
	if !ok {
		return Identity{}, ErrAccountNotFound
	}
	if record.Password == "" || subtle.ConstantTimeCompare([]byte(record.Password), []byte(info.Password)) != 1 {
		return Identity{}, errors.New("开发账号鉴权失败")
	}
	found := false
	for i := range record.Avatars {
		if len(record.Avatars[i].OID) != 12 {
			record.Avatars[i].OID = NewAvatarOID(time.Now())
		}
		if record.Avatars[i].Hostnum == info.Hostnum {
			found = true
		}
		if record.Avatars[i].UID == 0 {
			a.nextUID++
			record.Avatars[i].UID = a.nextUID
		}
		record.Avatars[i].Account = info.Account
		if record.Avatars[i].Progress.GuideTasks == nil {
			record.Avatars[i].Progress = NewProgress(record.Avatars[i].Info.Level, time.Now())
		}
	}
	if !found {
		a.nextUID++
		record.Avatars = append(record.Avatars, Avatar{OID: NewAvatarOID(time.Now()), UID: a.nextUID, Account: info.Account, Progress: NewAvatarProgress(1, time.Now()), Hostnum: info.Hostnum, Info: DefaultAvatarInfo(info.Account)})
	}
	a.records[info.Account] = record
	avatars := append([]Avatar{}, record.Avatars...)
	return Identity{Account: info.Account, Avatars: avatars}, nil
}

// Register 显式建号；已存在账号必须拒绝，不能覆盖口令。
func (a *FixtureAccounts) Register(ctx context.Context, info ClientInfo) (Identity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Identity{}, err
	}
	if info.Account == "" || info.Password == "" || info.Hostnum <= 0 {
		return Identity{}, errors.New("开发账号注册参数无效")
	}
	if _, ok := a.records[info.Account]; ok {
		return Identity{}, ErrAccountExists
	}
	a.nextUID++
	record := FixtureAccount{Password: info.Password, Avatars: []Avatar{{
		OID: NewAvatarOID(time.Now()), UID: a.nextUID, Account: info.Account, Progress: NewAvatarProgress(1, time.Now()), Hostnum: info.Hostnum, Info: DefaultAvatarInfo(info.Account),
	}}}
	a.records[info.Account] = record
	return Identity{Account: info.Account, Avatars: append([]Avatar{}, record.Avatars...)}, nil
}

func (a *FixtureAccounts) SDKLogin(ctx context.Context, info ClientInfo, sdkInfo json.RawMessage) (Identity, error) {
	// 与 dbstore.SDKLogin 同契约：真实票据鉴权未实现前一律拒绝，禁止自动建号。
	return Identity{}, ErrSDKUnavailable
}

// SyncPvpRankings 从内存夹具读取跨账号同步 PVP 榜（与 dbstore 同契约）。
func (a *FixtureAccounts) SyncPvpRankings(ctx context.Context, limit int, hosts ...int) ([]SyncPvpRankEntry, error) {
	if limit <= 0 || limit > 1000 {
		return nil, errors.New("排行榜数量越界")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]SyncPvpRankEntry, 0, limit)
	for _, record := range a.records {
		for _, av := range record.Avatars {
			if av.Progress.SyncPvpScore > 0 && (len(hosts) == 0 || hosts[0] == 0 || av.Hostnum == hosts[0]) {
				out = append(out, SyncPvpRankEntry{OID: av.OID, UID: av.UID, Hostnum: av.Hostnum, Nickname: av.Info.Nickname,
					Level: av.Info.Level, Score: av.Progress.SyncPvpScore})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return string(out[i].OID) < string(out[j].OID)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Push 是待传输适配器编码的业务推送，不能直接作为 MobileRPC 负载发送。
type Push struct {
	Target string `json:"target"`
	Method string `json:"method"`
	Args   []any  `json:"args"`
}

func push(target, method string, args ...any) Push {
	if args == nil {
		args = []any{}
	}
	return Push{Target: target, Method: method, Args: args}
}

type Phase string

const (
	Connected     Phase = "已连接"
	Authenticated Phase = "已鉴权"
	Playing       Phase = "已成为玩家"
	Closed        Phase = "已关闭"
)

// Connection 按连接串行处理业务，多个连接之间无需共用业务锁。
type Connection struct {
	mu                      sync.Mutex
	phase                   Phase
	identity                Identity
	connType                int
	hostnum                 int
	speedChecks             int
	refreshLoginSent        bool
	finishedGuides          map[int]bool
	reconnectAuth           json.RawMessage
	deviceID                string
	battleStartSent         bool
	battleEvents            []BattleEvent
	ordinaryObservation     *BattleSession
	pendingOrdinaryResult   *battleEnvelope
	nextOrdinaryResultRetry int64
	nextBackgroundSnapshot  int64
	ordinaryObservationAt   int64
	ordinaryPendingAuto     *bool
	// 原生10001结束链会在result上报前抢先请求下一战；先暂存，
	// 当前战斗事务结算后再按原回调编号建立下一会话。
	pendingDungeonArgs []json.RawMessage
	// 客户端复用战斗场景时可能先发 battle_fighting、后发 load_entity_finish。
	// 按当前战斗 UUID 暂存，加载完成后在同一连接内补执行，禁止串到下一场。
	pendingBattleFightingArgs []json.RawMessage
	pendingBattleFightingUUID string
	ordinaryPrepareUUID       string
	humanSnapshotOID          string
	humanSnapshotRevision     int64
	humanSnapshotKnown        bool
	onlineOID                 string
	pendingMailbox            *Progress
	pendingMailboxNotify      bool
	pendingPlayerReload       bool
	pendingSocialOIDs         [][]byte
	humanDeliveryUUID         string
	// 单端PVP投递缓存；恢复世代变化时重新读取持久播放描述。
	nativeSoloUUID           string
	nativeSoloGeneration     int64
	nativeSoloCursor         int
	humanDeliverySequence    int64
	humanRefusalSequence     int64
	humanPresenceID          string
	nextHumanPresenceRefresh int64
	nativeRecordLists        map[int][]string
	lastDailyCheckDay        string
	nextDailyRetry           int64
	nextPvpAwardRetry        int64
	nextLoginFinishRetry     int64
	diagnosticWindow         int64
	diagnosticCount          int
	telemetryWindow          int64
	telemetryCount           int
}

func NewConnection() *Connection {
	return &Connection{phase: Connected, finishedGuides: map[int]bool{}}
}
func (c *Connection) Phase() Phase { c.mu.Lock(); defer c.mu.Unlock(); return c.phase }

// SelectedAvatar 返回当前鉴权账号在所选服务器上的角色，供传输层绑定实体。
func (c *Connection) SelectedAvatar() (Avatar, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.phase != Authenticated && c.phase != Playing {
		return Avatar{}, false
	}
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			av.OID = append([]byte(nil), av.OID...)
			av.Progress = CloneProgress(av.Progress)
			return av, true
		}
	}
	return Avatar{}, false
}

type Service struct {
	Accounts             Accounts
	Hotfix               *hotfix.Catalog
	Now                  func() time.Time
	TimeZone             int
	onlineMu             sync.Mutex
	online               map[string]map[*Connection]bool
	nativePvpMu          sync.Mutex
	nativePvpConfig      *nativeengine.Config
	nativePvpResource    string
	nativePvpAuthorities map[string]*nativeengine.Authority
}

func New(accounts Accounts, catalog *hotfix.Catalog) *Service {
	// 客户端 reset_time_function 会取负值：timezone_info(-tz)。协议沿用 time.timezone
	// 的西向秒数；中国标准时间 UTC+8 必须发送 -28800（9AD0A456 @24-31）。
	s := &Service{Accounts: accounts, Hotfix: catalog, Now: time.Now, TimeZone: -8 * 3600, nativePvpAuthorities: map[string]*nativeengine.Authority{}}
	s.loadNativePvpEnvironment()
	return s
}

// Handle 只接收适配器完成解码后的方法及实参，不猜测 BSON、索引或指令号。
func (s *Service) Handle(ctx context.Context, c *Connection, method string, args []json.RawMessage) (result []Push, resultErr error) {
	c.mu.Lock()
	mailBefore := c.SelectedAvatarUnsafe().Progress.MailRevision
	defer func() {
		p := c.SelectedAvatarUnsafe().Progress
		changed := resultErr == nil && c.phase == Playing && p.MailRevision > mailBefore
		var snapshot Progress
		var oid []byte
		if changed {
			snapshot = CloneProgress(p)
			oid = append([]byte(nil), selectedOID(c)...)
		}
		pendingSocial := c.pendingSocialOIDs
		c.pendingSocialOIDs = nil
		c.mu.Unlock()
		for _, target := range pendingSocial {
			s.publishPlayerRefresh(target)
		}
		if changed {
			s.publishMailboxChange(oid, snapshot, false)
		}
	}()
	if c.phase == Playing && !ordinaryObserverMessage(method, args) {
		beforeBasic, _ := json.Marshal(basicRewardProperties(c.SelectedAvatarUnsafe().Progress))
		before, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress.Achievements)
		beforeStats := profileStatisticsProperties(c.SelectedAvatarUnsafe().Progress)
		beforeLevel := c.SelectedAvatarUnsafe().Progress.AvatarLevel
		beforeExp := c.SelectedAvatarUnsafe().Progress.AvatarExp
		defer func() {
			if resultErr != nil || c.phase != Playing {
				return
			}
			// 业务拒绝/全服事务失败不能先提交单边日界。在线刷新只随成功心跳，
			// 领奖和成功事件则在自身资产事务内刷新，避免破坏跨角色回滚边界。
			if b := c.SelectedAvatarUnsafe().Progress.BasicRewards; method == "heart_beat" && b != nil && b.Day != socialDay(s.Now()) {
				if err := s.updateProgress(ctx, c, func(p *Progress) error { return ensureBasicRewards(p, s.Now(), true) }); err != nil {
					log.Printf("基础奖励在线日界刷新失败，将随心跳重试：%v", err)
				}
			}
			p := c.SelectedAvatarUnsafe().Progress
			extra := []Push{}
			afterBasic, _ := json.Marshal(basicRewardProperties(p))
			if string(beforeBasic) != string(afterBasic) {
				extra = append(extra, basicRewardPushes(p)...)
			}
			stats := profileStatisticsProperties(p)
			for _, key := range []string{"cards_count", "last_main_chapter_dungeon_id"} {
				if stats[key] != beforeStats[key] {
					extra = append(extra, push("Avatar", "client_prop_changed", []any{key, stats[key]}))
				}
			}
			level := c.SelectedAvatarUnsafe().Info.Level
			if p.AvatarExp != beforeExp {
				extra = append(extra, push("Avatar", "client_prop_changed", []any{"exp", p.AvatarExp}))
			}
			if level != beforeLevel {
				extra = append(extra, push("Avatar", "client_prop_changed", []any{"level", level}), powerPush(c), push("Avatar", "on_avatar_level_up", beforeLevel, level, emptyActivityBox()))
			}
			after, _ := json.Marshal(p.Achievements)
			if string(before) != string(after) {
				extra = append(extra, push("Avatar", "client_prop_changed", []any{"achves", achievementProperties(p)}), push("Avatar", "client_prop_changed", []any{"achv_value", achievementPoints(p)}))
			}
			if len(extra) > 0 {
				// 专用结果已同步同一属性时，不再发送第二份相同setter。
				existing := map[string]bool{}
				for _, item := range result {
					if item.Method == "client_prop_changed" && len(item.Args) == 1 {
						if pair, ok := item.Args[0].([]any); ok && len(pair) > 0 {
							if key, ok := pair[0].(string); ok {
								existing[key] = true
							}
						}
					}
				}
				filtered := extra[:0]
				for _, item := range extra {
					if item.Method == "client_prop_changed" && len(item.Args) == 1 {
						if pair, ok := item.Args[0].([]any); ok && len(pair) > 0 {
							if key, ok := pair[0].(string); ok && existing[key] {
								continue
							}
						}
					}
					filtered = append(filtered, item)
				}
				extra = filtered
				at := len(result)
				for i, push := range result {
					if push.Method == "battle_result" {
						at = i
						break
					}
				}
				if at > 0 && result[at-1].Method == "call_client_callback" {
					at--
				}
				result = append(append(result[:at:at], extra...), result[at:]...)
			}
		}()
	}
	if c.phase == Closed {
		return nil, errors.New("连接已关闭")
	}
	if c.phase == Playing && humanConnectionMethod(method) && !cachedOrdinaryObserverMessage(c, method, args) {
		if err := s.refreshHumanConnection(ctx, c); err != nil {
			return nil, err
		}
	}
	// 普通观察不能等待全服结奖锁而丢序号；普通结果由自身事务和收据处理。
	// 日历仍在入场、排行和Tick刷新，真人及原生PVP结果维持原结奖路径。
	if c.phase == Playing && activityAwardMethod(method) && !ordinaryObserverMessage(method, args) && !cachedOrdinaryObserverMessage(c, method, args) {
		if err := s.refreshActivityAwards(ctx, c); err != nil {
			return nil, err
		}
	}
	switch method {
	case "reset_gift_box":
		return s.resetShopGiftBoxRPC(ctx, c, args)
	case "update_recommend_gift_state":
		return s.updateRecommendationRPC(ctx, c, args)
	case "enter_asyn_pvp", "refresh_asyn_pvp", "set_defence_cards", "get_candidate_cards", "set_asyn_pvp_auto":
		return s.asyncPvpRPC(ctx, c, method, args)
	case "get_record_result":
		return s.getRecordResult(ctx, c, args)
	case "upgrade_card_skill", "upgrade_talent_node", "reset_talent_node":
		return s.cardSkillTalentRPC(ctx, c, method, args)
	case "upload_native_battle_record":
		return s.uploadNativeBattleRecord(ctx, c, args)
	case "start_record_battle":
		return s.startNativeRecordBattle(ctx, c, args)
	case "receive_sync_pvp_weekly_win_bonus":
		return s.receiveSyncPvpWeekly(ctx, c, args)
	case "receive_sync_pvp_season_bonus":
		return s.receiveSyncPvpSeason(ctx, c, args)
	case "refresh_cthulhu_activity", "cthulhu_visit_init_item", "cthulhu_visit_layer_init_item", "cthulhu_visit_layer_finish_item", "cthulhu_move", "cthulhu_ctrl_move", "cthulhu_giveup", "add_cthulhu_point", "cthulhu_item_check", "cthulhu_check_all_in", "set_cthulhu_title":
		return s.activityRPC(ctx, c, method, args)
	case "enter_miku_node", "leave_miku_map", "receive_miku_task_bonus", "receive_miku_like_song_bonus", "receive_miku_surprise_bonus", "receive_miku_achv_bonus":
		return s.activityRPC(ctx, c, method, args)
	case "open_grid":
		return s.activityRPC(ctx, c, method, args)
	case "query_rank_list", "query_mountain_sea_own_rank":
		return s.activityRPC(ctx, c, method, args)
	case "player_enter_house", "set_restroom_index", "unlock_dormitory", "unlock_facility", "upgrade_facility", "dormitory_change_house_card", "set_restroom_girls", "house_card_change_dress_id", "receive_restroom_exp", "update_visiting_setting", "receive_furniture_handbook_bonus", "receive_all_furniture_handbook_bonus", "receive_furniture_theme_handbook_bonus", "receive_all_furniture_theme_handbook_bonus", "setup_furniture", "withdraw_furniture", "update_restroom_grid_info", "update_restroom_grid_wallpapers", "exchange_furniture":
		return s.collectionRPC(ctx, c, method, args)
	case "query_friend_dormitory", "like_friend_house", "end_visiting_house":
		return s.collectionSocialRPC(ctx, c, method, args)
	case "enter_free_stage", "leave_free_stage", "enter_stage_site", "set_free_stage_power", "set_auto_free_stage_power", "set_auto_state", "set_auto_fighting", "set_auto_list", "unlock_chapter":
		return s.freeStageRPC(ctx, c, method, args)
	case "enter_task_tower":
		return s.taskTowerRPC(ctx, c, args)
	case "refresh_consign_task_one", "start_consign_task", "drop_consign_task", "commit_consign_task", "reward_consign_phase_bonus":
		return s.consignRPC(ctx, c, method, args)
	case "create_new_league", "apply_league", "agree_league_apply", "refuse_league_apply", "league_setup_auto_agree", "leave_league", "query_total_league_info", "search_league", "appoint_league_member":
		return s.leagueRPC(ctx, c, method, args)
	case "player_get_house_random_reward":
		return s.collectionResidentRPC(ctx, c, args)
	case "gather_produce_material_speed_up":
		return s.collectionTutorialRPC(ctx, c, args)
	case "league_protect_start":
		return s.leagueProtectStartRPC(ctx, c, args)
	case "gather_produce_material":
		return s.collectionProductionRPC(ctx, c, args)
	case "get_house_reward":
		return s.collectionDailyRPC(ctx, c, args)
	case "update_last_get_time":
		return s.collectionEnergyRPC(ctx, c, args)
	case "compose_furniture_special":
		return s.collectionFusionRPC(ctx, c, args)
	case "set_up_furniture_material_id", "reset_up_furniture_material_id":
		return s.collectionWishlistRPC(ctx, c, method, args)
	case "facility_card_upgrade", "get_facility_card_reward", "upgrade_house_facility_card":
		return s.collectionCardFacilityRPC(ctx, c, method, args)
	case "compose_material", "compose_furniture_normal", "unlock_furniture_compose":
		return s.craftRPC(ctx, c, method, args)
	case "enter_house_frage", "set_mark_card", "house_frage_move", "house_frage_ctrl_move", "open_game_site", "house_frage_use_prop", "buy_frage_stone", "get_house_board_game_reward":
		return s.activityRPC(ctx, c, method, args)
	case "enter_wangyan_game", "wangyan_game_enter_map", "wangyan_game_enter_dungeon", "submit_wangyan_consign_task", "summer_game_enter_map", "summer_game_move_to", "summer_game_move_boat", "summer_game_enter_node", "summer_game_choose_item", "summer_game_clear_sp", "start_fishing", "check_fishing_result", "receive_fish_handbook_bonus", "receive_all_fish_handbook_bonus", "receive_summer_banner_bonus", "mountainsea_enter_dungeon", "change_guard_site_cards", "receive_mountain_dungeon_bonus", "receive_monster_nian_progress_bonus", "prev_exam_choose_opt", "receive_exam_study_bonus_all", "receive_exam_study_bonus", "receive_exam_review_bonus", "receive_exam_test_bonus", "exam_answer_problem", "exam_answer_bonus", "receive_storyline_bonus", "enter_miku_map", "activate_handbook_item", "set_miku_auto_path", "enter_story_dungeon", "enter_curse_abyss_dungeon":
		return s.activityRPC(ctx, c, method, args)
	case "apply_friend", "agree_apply_friend", "agree_all_apply_friend", "refuse_apply_friend", "refuse_all_apply_friend", "clear_friend_request_flag", "delete_friend", "add_black_list", "delete_black_list", "search_friend", "get_recommend_friends", "get_player_details", "send_card_comment", "update_card_comment", "remove_card_comment", "like_card_comment", "unlike_card_comment", "query_card_comments", "like_card_remark", "unlike_card_remark", "select_card_tags", "query_card_remark":
		return s.socialRPC(ctx, c, method, args)
	case "challenge_friend", "agree_challenge", "cancel_challenge", "refuse_challenge":
		return s.humanChallengeRPC(ctx, c, method, args)
	case "set_nickname_gender":
		return s.setNicknameGender(ctx, c, args)
	case "set_global_vo", "set_story_vo":
		return s.setVoice(ctx, c, method, args)
	case "enter_dungeon":
		return s.enterDungeon(ctx, c, args)
	case "load_entity_finish":
		return s.battleLoaded(ctx, c, args)
	case "battle_fighting":
		return s.battleFighting(ctx, c, args)
	case "exit_battle", "leave_battle", "quit_battle":
		return s.humanExitBattle(ctx, c, args)
	case "do_command":
		return s.doCommand(ctx, c, args)
	case "battle_guide_end":
		// 教学战斗演出结束上报。客户端在每段本地演出完成后调用，
		// 不带回调且不需要服务端回包；权威战斗时钟仍由 Tick 推进。
		if c.phase != Playing {
			return nil, errors.New("教学演出结束上报需要玩家状态")
		}
		log.Printf("教学战斗演出结束上报 args=%d", len(args))
		return []Push{}, nil
	case "client_trigger_action":
		// Android battle.client_trigger_action 会先在本地 battle.event_mgr 执行，
		// 再把 event_id 上报服务端；无回调。这里仅确认当前战斗归属并记录，
		// 不可再次 sync 回客户端，否则同一 instance_event 会被重复执行。
		if c.phase != Playing || len(args) != 1 {
			return nil, errors.New("战斗剧情事件上报需要玩家状态及单个事件编号")
		}
		var eventID int
		if json.Unmarshal(args[0], &eventID) != nil || eventID <= 0 {
			return nil, errors.New("战斗剧情事件编号无效")
		}
		battle := c.SelectedAvatarUnsafe().Progress.Battle
		if battle == nil || battle.Finished || !battle.Started {
			return nil, errors.New("战斗剧情事件没有进行中的战斗")
		}
		log.Printf("客户端战斗剧情事件 dungeon=%d event=%d", battle.DungeonID, eventID)
		return []Push{}, nil
	case "finished_dungeon_event":
		// Android dungeon_mgr及preload_mgr两处均上报单个dungeon_id，
		// 剧情结束后先上报，再推进引导/返回城市。此消息不是战斗result。
		if c.phase != Playing || len(args) != 1 {
			return nil, errors.New("副本完成事件需要玩家状态及单个副本编号")
		}
		var dungeonID int
		if json.Unmarshal(args[0], &dungeonID) != nil {
			return nil, errors.New("副本完成事件编号类型无效")
		}
		if _, ok := dungeonCatalog[dungeonID]; !ok {
			return nil, errors.New("副本完成事件编号未知")
		}
		p := c.SelectedAvatarUnsafe().Progress
		relation := "剧情完成上报，未关联当前结算"
		if p.Battle != nil && p.Battle.DungeonID == dungeonID && p.Battle.Finished {
			relation = "关联当前已结束会话"
		}
		log.Printf("副本剧情完成上报 dungeon=%d relation=%s；不发奖", dungeonID, relation)
		return []Push{}, nil
	case "set_battle_speed":
		return s.setBattleSpeed(ctx, c, args)
	case "sync_pvp_battle_result":
		return nil, errors.New("同步PVP结算结果仅允许服务端下发")
	case "start_sync_pvp_match", "send_pvp_cards", "pvp_load_complete", "cancel_sync_pvp_match",
		"query_sync_pvp_rank", "query_sync_pvp_world_rank", "query_sync_pvp_local_rank_list",
		"query_sync_pvp_world_rank_list", "query_sync_pvp_season_id":
		if handled, pushes, err := s.humanCardsRPC(ctx, c, method, args); handled {
			return pushes, err
		}
		return s.syncPvpRPC(ctx, c, method, args)
	case "client_need_recover_battle":
		return s.clientNeedRecoverBattle(ctx, c, args)
	case "random_cards":
		return s.handleGachaContract(ctx, c, args)
	case "decompose_cards":
		return s.decomposeCardsRPC(ctx, c, args)
	case "card_enhance", "up_level_card", "upgrade_card":
		return s.cardGrowthRPC(ctx, c, method, args)
	case "consume_ring", "update_level_one":
		return s.cardOathRPC(ctx, c, method, args)
	case "change_head", "change_head_box", "update_head_box_by_client":
		return s.profileCosmeticRPC(ctx, c, method, args)
	case "buy_commodity":
		return s.buyCommodityRPC(ctx, c, args)
	case "receive_achv_bonus", "receive_all_achv_bonus":
		return s.achievementRPC(ctx, c, method, args)
	case "embed_rune", "unembed_rune", "up_level_rune", "decompose_runes", "lock_rune", "unlock_rune", "change_rune_extra_attr":
		return s.runeRPC(ctx, c, method, args)
	case "add_runes_templates", "change_runes_templates_name", "delete_runes_templates", "update_runes_templates", "set_top_runes_templates", "embed_rune_by_template":
		return s.runePresetRPC(ctx, c, method, args)
	case "set_assist_card", "del_assist_card", "select_assist_card", "refresh_assist_use_times":
		return s.friendAssistRPC(ctx, c, method, args)
	case "set_captain_card_id":
		return s.setCaptainCard(ctx, c, args)
	case "set_layout_cards":
		return s.setLayoutCards(ctx, c, args)
	case "cover_preset_record", "del_preset_record", "update_preset_name", "update_preset_index", "set_sync_pvp_preset_record", "activity_set_preset_record":
		return s.formationPresetRPC(ctx, c, method, args)
	case "signin_monthly":
		return s.signinMonthly(ctx, c, args)
	case "receive_power_supply", "receive_new_task_bonus", "receive_daily_task_active", "receive_daily_active_bonus", "receive_weekly_active_bonus", "receive_check_in_bonus", "receive_all_check_in_bonus", "receive_score_bonus":
		return s.basicRewardRPC(ctx, c, method, args)
	case "receive_intimacy_bonus":
		return s.receiveIntimacyBonus(ctx, c, args)
	case "consume_intimacy_gift", "consume_multi_intimacy_gift":
		return s.intimacyGiftRPC(ctx, c, method, args)
	case "change_dungeon_skip_edit_state":
		return s.setDungeonSkipEditState(ctx, c, args)
	case "set_card_vo", "set_show_cards", "set_girl_random_enable", "set_explore_auto_agent":
		return s.loggedPreferenceRPC(ctx, c, method, args)
	case "output_shop_enter_log", "output_shop_leave_log", "change_graphics_quality":
		return s.loggedClientTelemetry(c, method, args)
	case "query_mail_content", "read_mail", "delete_mail", "receive_attachment", "receive_all_attachments":
		return s.mailRPC(ctx, c, method, args)
	case "compose_cards":
		return s.composeCardsRPC(ctx, c, args)
	case "send_loading_percent":
		if c.phase != Playing || len(args) != 1 {
			return nil, errors.New("战斗加载进度参数无效")
		}
		var percent float64
		if json.Unmarshal(args[0], &percent) != nil || percent < 0 || percent > 100 {
			return nil, errors.New("战斗加载进度越界")
		}
		return nil, nil
	case "quick_login", "sdk_login", "register_login":
		want := 1
		if method == "sdk_login" {
			want = 2
		}
		if len(args) != want || c.phase != Connected {
			return nil, errors.New("登录参数数量或连接状态错误")
		}
		var info ClientInfo
		if err := json.Unmarshal(args[0], &info); err != nil {
			return nil, err
		}
		if info.HotfixIndex < 0 {
			return nil, errors.New("热修索引不能为负数")
		}
		fail := func(code int, reason string) ([]Push, error) {
			c.phase = Closed
			return []Push{push("Account", "login_result", code, reason, info.ConnType)}, nil
		}
		if info.Account == "" {
			return fail(RetAccountEmpty, "账号不能为空")
		}
		if info.Hostnum <= 0 {
			return fail(RetHostnumEmpty, "服务器编号不能为空")
		}
		authCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		var identity Identity
		var err error
		switch method {
		case "quick_login":
			// B9F5C696 Account.quick_login：客户端没有独立 register_login 上行——
			// ①register_info 附加字段（gworld.register_info 真值时上行）= 真实输入 UI 的显式注册；
			// ②无标记的未知设备账号自动注册是原厂"设备快速账号"语义（login-flow-spec：
			// 原登录按钮以本地设备账号自动注册；重装设备的新 hs_<uuid> 依赖该路径进服，
			// 2026-10-06 hotfix-e2e 实机 9012 卡死即缺此分支）。
			if len(info.RegisterInfo) > 0 {
				identity, err = s.Accounts.Register(authCtx, info)
			} else {
				identity, err = s.Accounts.QuickLogin(authCtx, info)
				if errors.Is(err, ErrAccountNotFound) {
					identity, err = s.Accounts.Register(authCtx, info)
				}
			}
		case "register_login":
			identity, err = s.Accounts.Register(authCtx, info)
		default:
			identity, err = s.Accounts.SDKLogin(authCtx, info, args[1])
		}
		if err != nil {
			code, reason := RetAuthFailed, "账号鉴权失败"
			switch {
			case errors.Is(err, ErrAccountNotFound):
				code, reason = RetAccountError, "账号不存在"
			case errors.Is(err, ErrAccountExists):
				code, reason = RetAccountError, "账号已存在"
			case errors.Is(err, ErrSDKUnavailable):
				code, reason = RetAuthFailed, "SDK 鉴权未实现"
			}
			return fail(code, reason)
		}
		c.identity, c.connType, c.hostnum, c.phase = identity, info.ConnType, info.Hostnum, Authenticated
		if _, supported := s.Accounts.(ProgressAccounts); supported && len(identity.Avatars) > 0 {
			if err := s.updateProgress(ctx, c, func(p *Progress) error {
				if err := ensureBasicRewards(p, s.Now(), true); err != nil {
					return err
				}
				if err := recordAchievementLogin(p, s.Now()); err != nil {
					return err
				}
				if err := ensureCollection(p, s.Now()); err != nil {
					return err
				}
				if err := ensureActivityLogin(p, p.AvatarLevel, s.Now()); err != nil {
					return err
				}
				return refreshShopSubscription(s, p, s.Now())
			}); err != nil {
				return fail(RetAuthFailed, "登录成就状态保存失败")
			}
			identity = c.identity
		}
		if identity.Avatars == nil {
			identity.Avatars = []Avatar{}
		}
		pushes := []Push{push("Account", "login_result", RetSuccess, "", info.ConnType), push("Account", "on_get_all_avatars", identity.Avatars)}
		hf := s.Hotfix.Query(info.HotfixIndex)
		return append(pushes, push("Account", "on_hotfix_when_login", hf.Script, hf.Index)), nil
	case "query_hotfix":
		if c.phase != Playing || len(args) != 1 {
			return nil, errors.New("热修查询需要玩家状态及一个索引参数")
		}
		var index int
		if err := json.Unmarshal(args[0], &index); err != nil || index < 0 {
			return nil, errors.New("热修索引必须是非负整数")
		}
		hf := s.Hotfix.Query(index)
		return []Push{push("Avatar", "on_query_hotfix_success", hf.Script, hf.Index)}, nil
	case "query_server_time":
		if c.phase != Playing || len(args) != 0 {
			return nil, errors.New("时间查询需要玩家状态及空参数")
		}
		return []Push{s.serverTime()}, nil
	case "heart_beat":
		// 客户端 net_delay.send_heart_beat 上行 last_send_time（本地时间）；服务端以
		// heart_beat(server_time, last_send_time) 应答（dis 908202EE：客户端据此 fix_client_time
		// 并计算 net_delay_time）；不应答则每 7 秒超时一次并提示“网络不稳定”。
		if c.phase != Playing || len(args) != 1 {
			return nil, errors.New("心跳需要玩家状态及一个参数")
		}
		var lastSend float64
		if err := json.Unmarshal(args[0], &lastSend); err != nil {
			return nil, errors.New("心跳参数必须是数字")
		}
		return []Push{push("Avatar", "heart_beat", s.unixFloat(), lastSend)}, nil
	case "set_reconnect_auth_msg":
		if c.phase != Playing || len(args) != 1 {
			return nil, errors.New("重连凭证需要玩家状态及一个参数")
		}
		c.reconnectAuth = append(json.RawMessage(nil), args[0]...)
		return s.finishLogin(ctx, c)
	case "finished_storyline":
		return s.finishedStoryline(ctx, c, args)
	case "finished_guide":
		return s.finishedUIGuide(ctx, c, args)
	case "client_sa_log":
		return s.clientDiagnostic(c, args)
	case "guide_task_finished":
		// 客户端 guide_task_mgr.guide_task_finished 经 AvatarEntity.call_server 上行：
		// 参数 [callback_id, task_id]（wrapper/entity.py call_server 把 callback_id 放首位）。
		// 必须用 call_client_callback 应答，否则客户端 server_callback 不执行、引导链卡住。
		if c.phase != Playing || len(args) != 2 {
			return nil, errors.New("引导完成上报需要玩家状态及两个参数")
		}
		var callbackID, taskID int
		if err := json.Unmarshal(args[0], &callbackID); err != nil {
			return nil, errors.New("引导完成上报的 callback_id 必须是整数")
		}
		if err := json.Unmarshal(args[1], &taskID); err != nil {
			return nil, errors.New("引导完成上报的 task_id 必须是整数")
		}
		level := 1
		for _, av := range c.identity.Avatars {
			if av.Hostnum == c.hostnum {
				level = av.Info.Level
			}
		}
		if err := s.updateProgress(ctx, c, func(p *Progress) error { return AdvanceGuide(p, taskID, s.Now(), level) }); err != nil {
			av := c.SelectedAvatarUnsafe()
			log.Printf("引导完成被拒绝 uid=%d oid=%s task_id=%d 原因=%v", av.UID, hexOf(av.OID), taskID, err)
			// 原生guide_task_mgr直接将msg交给C++set_string，必须传UTF-8字节。
			// msgpack文本在encoding=utf-8下变为unicode，会触发std::string转换异常。
			return []Push{Callback(callbackID, []any{false, []byte(err.Error())})}, nil
		}
		c.finishedGuides[taskID] = true
		log.Printf("引导任务完成 task_id=%d", taskID)
		return []Push{Callback(callbackID, []any{true, ""})}, nil
	case "upload_guide_tasks":
		// 客户端 guide_task_mgr.upload_guide_tasks 上行：参数 [callback_id, guide_tasks]。
		// 上报型（客户端 callback 为 None），只接收记录、不回 call_client_callback（避免调用空回调）。
		if c.phase != Playing || len(args) != 2 {
			return nil, errors.New("引导状态上报需要玩家状态及两个参数")
		}
		var tasks map[int]GuideTask
		if err := json.Unmarshal(args[1], &tasks); err != nil {
			return nil, errors.New("引导状态必须是任务字典")
		}
		if err := ValidateGuideUpload(tasks); err != nil {
			return nil, err
		}
		log.Printf("引导状态上报 %d 字节", len(args[1]))
		return []Push{}, nil
	case "start_speed_check":
		// 客户端 guard_mgr.speed_check 定时器上行（0 参）；真实服务器应答
		// start_speed_check(check_type)，客户端延时后上行 speed_check 回传。
		if c.phase != Playing || len(args) != 0 {
			return nil, errors.New("测速启动需要玩家状态及空参数")
		}
		c.speedChecks++
		return []Push{push("Avatar", "start_speed_check", c.speedChecks)}, nil
	case "speed_check":
		// 参数：check_type、dungeon_id、fps（guard_mgr.send_speed_check 实证）。
		// 上报型方法，接收即完成，无回包。
		if c.phase != Playing || len(args) != 3 {
			return nil, errors.New("测速上报需要玩家状态及三个参数")
		}
		var numbers [3]float64
		for i, raw := range args {
			if err := json.Unmarshal(raw, &numbers[i]); err != nil {
				return nil, errors.New("测速上报参数必须是数字")
			}
		}
		// 成功路径无回包，必须留接收日志：否则无法从远端日志正向确认该上报（AUDIT P0-进展验收要求）。
		log.Printf("测速上报 check_type=%v dungeon_id=%v fps=%v", numbers[0], numbers[1], numbers[2])
		return []Push{}, nil
	case "receive_coordinate_check":
		// Android guard_mgr.send_coordinate_list/send_coordinate_check_list 均上行
		// [采样起始时间, 当前场景编号, 坐标点列表]；这是防护遥测，不带回调。
		if c.phase != Playing || len(args) != 3 {
			return nil, errors.New("坐标校验上报需要玩家状态及三个参数")
		}
		var startedAt float64
		var dungeonID int
		var points [][]float64
		if json.Unmarshal(args[0], &startedAt) != nil || startedAt < 0 {
			return nil, errors.New("坐标校验时间无效")
		}
		if json.Unmarshal(args[1], &dungeonID) != nil || dungeonID < 0 {
			return nil, errors.New("坐标校验场景编号无效")
		}
		if json.Unmarshal(args[2], &points) != nil || len(points) > 4096 {
			return nil, errors.New("坐标校验点列表无效")
		}
		for _, point := range points {
			if len(point) != 2 {
				return nil, errors.New("坐标校验点格式无效")
			}
		}
		log.Printf("坐标校验上报 dungeon=%d points=%d", dungeonID, len(points))
		return []Push{}, nil
	case "query_league_message_board":
		// 客户端 league_mgr.on_login_success 触发（0 参）；服务端以
		// on_query_league_message_board(message.LeagueMessageBoard) 应答。
		// 空角色未入公会，空板是真实语义，不是占位成功。
		if c.phase != Playing || len(args) != 0 {
			return nil, errors.New("消息板查询需要玩家状态及空参数")
		}
		return []Push{push("Avatar", "on_query_league_message_board", []any{})}, nil
	default:
		return nil, errors.New("业务方法尚未实现")
	}
}

// BecomePlayer 必须由已验证的角色实体创建及绑定流程调用；调试接口仅演练序列。
func (s *Service) BecomePlayer(c *Connection) ([]Push, error) {
	c.mu.Lock()
	defer func() {
		pending := c.pendingSocialOIDs
		c.pendingSocialOIDs = nil
		c.mu.Unlock()
		for _, oid := range pending {
			s.publishPlayerRefresh(oid)
		}
	}()
	if c.phase != Authenticated {
		return nil, errors.New("成为玩家前必须完成账号鉴权")
	}
	c.phase = Playing
	s.attachPlayer(c)
	s.initializeHumanDelivery(c)
	if err := s.refreshHumanPresence(context.Background(), c); err != nil {
		log.Printf("真人在线租约登记失败，将由Tick重试：%v", err)
	}
	if err := s.refreshActivityAwards(context.Background(), c); err != nil {
		log.Printf("活动排行结奖初始化失败，将由Tick重试：%v", err)
	}
	if err := s.refreshPvpAwards(context.Background(), c); err != nil {
		log.Printf("首次玩家竞技结奖刷新失败，将由Tick重试：%v", err)
	}
	// on_login_success/on_become_player 是无参普通方法（argc=1 仅 self），客户端 entity_message
	// 分发必然传入 parameters 实参而无法调用（实机 TypeError）；改由启动期热修触发。
	// on_refresh_login（主城初始化链入口，dis/2034D84C.asm、preload_mgr.py）不在此推送：
	// 实体创建窗口期 player 尚未绑定角色，客户端 on_login_success 读 Account.card_mgr 抛
	// AttributeError（2026-10-05 15:33 实机）；改在收到 set_reconnect_auth_msg 后推送。
	return []Push{s.serverTime()}, nil
}

func (s *Service) serverTime() Push {
	return push("Avatar", "sync_server_time", s.unixFloat(), s.TimeZone)
}

// unixFloat 返回当前 Unix 秒（含小数），时间同步与心跳应答共用。
func (s *Service) unixFloat() float64 {
	now := s.Now()
	return float64(now.Unix()) + float64(now.Nanosecond())/1e9
}

// Callback 仅由完成实际业务的处理器使用，编号沿用客户端的首个实参。
func Callback(callbackID int, resultArgs []any) Push {
	if resultArgs == nil {
		resultArgs = []any{}
	}
	return push("Avatar", "call_client_callback", callbackID, resultArgs)
}

func Kick(c *Connection, kickType int, reason string) Push {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.phase = Closed
	return push("Avatar", "on_kick_avatar", kickType, reason)
}

// NewAvatarOID 生成 12 字节 bson ObjectId 兼容 id（gate 契约）。
func NewAvatarOID(now time.Time) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint32(b[:4], uint32(now.Unix()))
	if _, err := rand.Read(b[4:]); err != nil {
		panic("生成角色随机标识失败")
	}
	return b
}

// DefaultAvatarInfo 沿用先前实机解析通过的最小角色字段，不代表完整主城属性。
func DefaultAvatarInfo(account string) AvatarInfo {
	chars := []rune(account)
	if len(chars) > 4 {
		chars = chars[len(chars)-4:]
	}
	return AvatarInfo{Nickname: "书灵" + string(chars), Level: 1, HeadID: 1, HeadBoxID: 3}
}
