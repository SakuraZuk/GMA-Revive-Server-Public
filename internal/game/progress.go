package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

//go:embed player_defaults.json
var playerDefaults []byte

type GuideDefinition struct {
	ID           int   `json:"_id"`
	Next         int   `json:"next_guide_task"`
	Trigger      *int  `json:"trigger_condition"`
	TriggerParam any   `json:"trigger_condition_params"`
	DoType       int   `json:"do_type"`
	DoParams     []any `json:"do_type_params"`
}

type baseline struct {
	Init struct {
		Level     int     `json:"init_level"`
		Power     int     `json:"init_power"`
		Materials [][]int `json:"init_materials"`
		Cards     [][]int `json:"init_cards"`
	} `json:"初始化"`
	LevelPower map[int]struct {
		Max int `json:"level_max_power"`
	} `json:"等级体力"`
	Guides   map[int]GuideDefinition `json:"引导任务"`
	UIGuides map[int]json.RawMessage `json:"界面引导"`
	Interval int                     `json:"体力恢复间隔"`
	PerValue int                     `json:"每次恢复体力"`
	Naming   struct {
		Min     int    `json:"min"`
		Max     int    `json:"max"`
		Words   string `json:"words"`
		Special string `json:"special"`
	} `json:"取名规则"`
	Dungeons map[int]struct {
		ID       int `json:"dungeon_id"`
		BattleID int `json:"dungeon_battle_id"`
		Type     int `json:"dungeon_type"`
		Power    int `json:"need_power"`
	} `json:"教学副本"`
	Battles map[int]struct {
		ID            int      `json:"battle_id"`
		SceneID       int      `json:"scene_id"`
		Avatars       []int    `json:"my_avatar_list"`
		Enemies       []int    `json:"enemy_avater_list"`
		StartTriggers []int    `json:"_start_triggers"`
		EnterShowTime *float64 `json:"_enter_show_time"`
	} `json:"教学战斗"`
	BattleFactors map[int]struct {
		HP      *float64 `json:"hp"`
		ATK     *float64 `json:"atk"`
		Defence *float64 `json:"defence"`
	} `json:"教学战斗系数"`
	BattleRoles map[int]struct {
		RoleID  int     `json:"role_id"`
		HP      int     `json:"hp"`
		ATK     int     `json:"atk"`
		Defence int     `json:"defence"`
		APSpeed float64 `json:"ap_speed"`
		Skills  []int   `json:"skill_list"`
	} `json:"战斗角色"`
	SkillProfiles map[int][]struct {
		ActTime float64 `json:"act_time"`
		Effects []struct {
			Rate    float64   `json:"rate"`
			Grow    []float64 `json:"grow"`
			Percent float64   `json:"percent"`
		} `json:"effects"`
	} `json:"技能效果"`
	// instance_trigger/instance_event 数据驱动解释器基线（export_player_defaults.py
	// 从每场战斗 start_triggers 出发按 trigger_events/trigger_ids 闭包收集）。
	WaveTriggers map[int]triggerDef       `json:"教学波次触发"`
	WaveEvents   map[int]instanceEventDef `json:"教学波次事件"`
	// new_task 表（成就/累计任务，109 条全量；服务端按 target_id 计数推进）。
	NewTasks map[int]struct {
		ID              int     `json:"_id"`
		TargetID        [][]int `json:"target_id"`
		TargetNeedCount int     `json:"target_need_count"`
	} `json:"新任务"`
	// system_unlock 键值条目扁平列表；unlock_condition 为三层嵌套（组×条件×[类型,值]），
	// 类型 1=通关副本（教学 10001 不在条件内）；少数类型（如 timber_pile 的 [13,"382#0"]）
	// 值为字符串，故元素用 any。
	SystemUnlocks []struct {
		System     string    `json:"system"`
		Version    int       `json:"version"`
		Conditions [][][]any `json:"conditions"`
	} `json:"系统解锁"`
}

// triggerDef 对应 instance_trigger 表行（服务器权威字段子集）。
type triggerDef struct {
	ID                 int     `json:"trigger_id"`
	Camp               *int    `json:"camp"`
	RoleIDs            []int   `json:"role_ids"`
	TriggerType        string  `json:"trigger_type"`
	ActionEndCounter   *int    `json:"action_end_counter"`
	ActionBeginCounter *int    `json:"action_begin_counter"`
	EntityRemainCount  *int    `json:"entity_remain_count"`
	ActivateTime       float64 `json:"activate_time"`
	TriggerEvents      []int   `json:"trigger_events"`
}

// instanceEventDef 对应 instance_event 表行（服务器权威字段子集）。
type instanceEventDef struct {
	EventID    int      `json:"event_id"`
	EventTime  *float64 `json:"event_time"`
	Camp       int      `json:"camp"`
	RoleIDs    []int    `json:"role_ids"`
	InitAP     *float64 `json:"init_ap"`
	SetAP      *float64 `json:"set_ap"`
	TriggerIDs []int    `json:"trigger_ids"`
	BattleEnd  *string  `json:"battle_end"`
}

var clientBaseline = func() baseline {
	var b baseline
	if err := json.Unmarshal(playerDefaults, &b); err != nil {
		panic(err)
	}
	if b.Interval <= 0 || b.Guides[1000].ID != 1000 {
		panic("客户端初始化基线无效")
	}
	return b
}()

type GuideTask struct {
	ID         int   `json:"task_id"`
	Status     int   `json:"task_status"`
	BeginTime  int64 `json:"begin_time"`
	BeginLevel int   `json:"begin_level"`
}

type Material struct {
	ID    int   `json:"material_id"`
	Count int64 `json:"count"`
	Total int64 `json:"total"`
}

type Power struct {
	Value    int     `json:"value"`
	LastTime float64 `json:"last_time"`
	Interval int     `json:"interval"`
	PerValue int     `json:"per_value"`
	Max      int     `json:"max_limit"`
}

// Progress 是服务器权威动态状态；上传引导数据不得改货币、材料、体力或系统解锁。
type Progress struct {
	AvatarType     int   `json:"avatar_type"`
	FinishedGuides []int `json:"finished_guides"`
	// 仅记录客户端已播放剧情，不作为通关、发奖或系统解锁证明。
	PlayedStorylines []string          `json:"server_played_storylines,omitempty"`
	BasicRewards     *BasicRewardState `json:"server_basic_rewards,omitempty"`
	GuideTasks       map[int]GuideTask `json:"guide_tasks"`
	SkipGuide        bool              `json:"skip_guide"`
	UnlockSystems    map[string]int    `json:"unlock_systems"`
	UnlockedChapters map[int]int       `json:"unlocked_chapters"`
	FreeYuanbao      int64             `json:"free_yuanbao"`
	PayCoin          int64             `json:"pay_coin"`
	Materials        map[int]Material  `json:"material_mgr"`
	MaterialRecords  map[int][]int     `json:"material_records"`
	CheckInRecords   map[int]any       `json:"check_in_records"`
	Power            Power             `json:"power"`
	AvatarLevel      int               `json:"server_avatar_level,omitempty"`
	AvatarExp        int64             `json:"exp"`
	GlobalVO         int               `json:"global_vo"`
	StoryVO          int               `json:"story_vo"`
	CardVoices       map[int]int       `json:"server_card_voices,omitempty"`
	ShowCards        []string          `json:"server_show_cards,omitempty"`
	GirlRandomEnable *bool             `json:"girl_random_enable,omitempty"`
	ExploreAutoAgent map[int]bool      `json:"activity_explore_agent,omitempty"`
	// 玩家业务（P0-5）：拥有卡与 new_task 进度；形态见 custom_types/card.py 与
	// custom_types/new_task.py 的 prop 定义（client save 通道）。
	Cards                   []Card                        `json:"cards,omitempty"`
	RandomCardsRecord       map[int]RandomCardRecord      `json:"random_cards_record,omitempty"`
	PromiseCardsRecord      map[int]PromiseCardRecord     `json:"promise_cards_record,omitempty"`
	AssistCardUUID          string                        `json:"assist_card_uuid,omitempty"`
	DungeonSkipEditState    map[int]bool                  `json:"dungeon_skip_edit_state,omitempty"`
	MailAdminReceipts       map[string]MailAdminReceipt   `json:"server_mail_admin_receipts,omitempty"`
	PlayerAdminReceipts     map[string]PlayerAdminReceipt `json:"server_player_admin_receipts,omitempty"`
	Social                  SocialState                   `json:"server_social,omitempty"`
	Activities              ActivityState                 `json:"server_activities,omitempty"`
	Collection              *CollectionState              `json:"server_collection,omitempty"`
	MailRevision            int64                         `json:"server_mail_revision,omitempty"`
	ShortMailInfo           map[int]Mail                  `json:"short_mail_info,omitempty"`
	Runes                   map[string]Rune               `json:"rune_mgr,omitempty"`
	RuneTemplates           map[string]RuneTemplate       `json:"runes_templates,omitempty"`
	RunePresetSchemaVersion int                           `json:"rune_preset_schema_version,omitempty"`
	RunePresetQuarantine    map[string]any                `json:"rune_preset_quarantine,omitempty"`
	RuneSchemaVersion       int                           `json:"rune_schema_version,omitempty"`
	RuneMigrationOriginals  map[string]Rune               `json:"rune_migration_originals,omitempty"`
	RuneQuarantine          map[string]QuarantinedRune    `json:"rune_quarantine,omitempty"`
	RuneGrantReceipts       map[string]RuneGrantReceipt   `json:"rune_grant_receipts,omitempty"`
	RuneAdminAudit          map[string]RuneAdminAudit     `json:"rune_admin_audit,omitempty"`
	NewTasks                map[int]NewTaskProgress       `json:"new_tasks,omitempty"`
	Battle                  *BattleSession                `json:"server_battle,omitempty"`
	// 客户端权威战斗（bridge rev 12）：偏好、通关记录与保存的出战阵容。
	BattlePreferences BattlePrefs          `json:"battle_preferences,omitempty"`
	ClearedDungeons   []int                `json:"cleared_dungeons,omitempty"`
	Lineup            []string             `json:"lineup,omitempty"`
	BattleLayouts     map[int]BattleLayout `json:"battle_layouts,omitempty"`
	// 原生 preset_record_mgr：普通20槽、同步PVP槽和活动槽均保存稳定ObjectID。
	PresetIDs           []string                  `json:"preset_ids,omitempty"`
	PresetCardsRecord   map[string]PresetRecord   `json:"preset_cards_record,omitempty"`
	SyncPvpPresetIDs    []string                  `json:"sync_pvp_preset_ids,omitempty"`
	ActivityPresetIDs   map[string]string         `json:"activity_preset_ids,omitempty"`
	SyncPvpScore        int                       `json:"sync_pvp_score,omitempty"`
	SyncPvpHighestScore int                       `json:"sync_pvp_highest_score,omitempty"`
	SyncPvpWinStreak    int                       `json:"sync_pvp_continues_win_count,omitempty"`
	SyncPvpLoseTimes    int                       `json:"sync_pvp_lose_times,omitempty"`
	SyncPvpSeasonID     int                       `json:"sync_pvp_season_id,omitempty"`
	SyncPvpWeeklyWins   int                       `json:"sync_weekly_win_count,omitempty"`
	SyncPvpMatch        *SyncPvpMatch             `json:"sync_pvp_match,omitempty"`
	SyncPvpMeta         SyncPvpMeta               `json:"server_sync_pvp_meta,omitempty"`
	AsyncPvp            AsyncPvpState             `json:"server_async_pvp,omitempty"`
	SyncPvpSettlements  map[string]SyncPvpReceipt `json:"sync_pvp_settlements,omitempty"`
	SyncPvpRecords      []SyncPvpRecord           `json:"sync_pvp_records,omitempty"`
	CaptainCardUUID     string                    `json:"captain_card_uuid,omitempty"`
	CaptainID           int                       `json:"captain_id"`
	CaptainDresses      map[int]int               `json:"captain_dresses,omitempty"`
	OwnedDresses        map[int][]int             `json:"owned_dresses,omitempty"`
	Intimacy            map[int]int               `json:"intimacy,omitempty"`
	IntimacyCommons     map[int]IntimacyCommon    `json:"intimacy_commons,omitempty"`
	// 旧累计次数保留用于审计，不作为当日限制；累计对话序号仍在 common。
	IntimacySpecialTotal int                     `json:"intimacy_special_total,omitempty"`
	SpecialGiftDay       string                  `json:"special_gift_day,omitempty"`
	SpecialGiftCounts    map[int]int             `json:"special_gift_card_2_count,omitempty"`
	OwnedHeadBox         map[int]float64         `json:"owned_head_box,omitempty"`
	HeadFrameGrantTime   map[int]float64         `json:"head_frame_grant_time,omitempty"`
	SelectedHeadID       int                     `json:"selected_head_id,omitempty"`
	SelectedHeadBoxID    int                     `json:"selected_head_box_id,omitempty"`
	CustomHeadCleared    bool                    `json:"custom_head_cleared,omitempty"`
	ObtainedCardIDs      map[int]int64           `json:"obtained_card_ids,omitempty"`
	CommodityDetails     map[int]CommodityDetail `json:"commodity_detail_info,omitempty"`
	GiftBoxInfo          map[int]ShopGiftBox     `json:"gift_box_info,omitempty"`
	Achievements         map[int]Achievement     `json:"achves,omitempty"`
	RemainingGameplay    *RemainingGameplayState `json:"remaining_gameplay,omitempty"`
	LeagueID             string                  `json:"server_league_id,omitempty"`
	LastLeaveLeague      int64                   `json:"server_last_leave_league,omitempty"`
	OwnedLeagues         map[string]*LeagueState `json:"server_owned_leagues,omitempty"`
	LeagueProtect        *LeagueProtectProgress  `json:"server_league_protect,omitempty"`
	AchievementLoginDay  string                  `json:"achievement_login_day,omitempty"`
	SigninMonth          string                  `json:"signin_month,omitempty"`
	SigninTimes          int                     `json:"signin_times,omitempty"`
	SigninDay            string                  `json:"signin_day,omitempty"`
}

// Card 对应客户端 card 类的 client save 字段子集（card_mgr 值结构）。
// uuid 为 24 位十六进制（12 字节 ObjectId 的字符串形态，gworld.gen_object_id 等价）。
type Card struct {
	UUID              string                   `json:"uuid"`
	CardID            int                      `json:"card_id"`
	Level             int                      `json:"level"`
	Exp               int                      `json:"exp"`
	Grade             int                      `json:"grade"`
	Awakened          int                      `json:"awakened"`
	Dress             int                      `json:"dress"`
	Lock              int                      `json:"lock"`
	Time              int64                    `json:"time"`
	EnhanceCount      int                      `json:"enhance_count"`
	SkillEnhanceCount int                      `json:"skill_enhance_count"`
	SupportSkillLevel int                      `json:"support_skill_level"`
	RingID            int                      `json:"ring_id"`
	IsUpdateLevelOne  bool                     `json:"is_update_level_one"`
	Skills            []CardSkill              `json:"skill_mgr,omitempty"`
	TalentTree        map[int][]CardTalentNode `json:"talent_tree,omitempty"`
}

// RandomCardRecord and PromiseCardRecord persist draw counters and guarantee state.
// The Android pool values are intentionally kept outside the player state until
// the deployed catalog is wired in; counters are still durable now.
type RandomCardRecord struct {
	PoolID      int `json:"pool_id"`
	RandomCount int `json:"random_count"`
}

type PromiseCardRecord struct {
	RuleID      int `json:"rule_id"`
	Count       int `json:"count"`
	FinishCount int `json:"finish_count"`
}

type Rune struct {
	UUID             string          `json:"uuid"`
	RuneID           int             `json:"rune_id"`
	Star             int             `json:"star,omitempty"`
	Position         int             `json:"pos,omitempty"`
	Suit             int             `json:"suit,omitempty"`
	Level            int             `json:"level"`
	Attrs            map[int]float64 `json:"attrs,omitempty"`
	CardUUID         string          `json:"card_uuid,omitempty"`
	Locked           bool            `json:"lock,omitempty"`
	ExtraAttrs       []int           `json:"extra_attrs,omitempty"`
	ExtraSuit        int             `json:"extra_suit,omitempty"`
	BaseAttrs        []int           `json:"base_attrs,omitempty"`
	ExtraAttrsCount  int             `json:"extra_attrs_count,omitempty"`
	ExtraAttrsLib    []int           `json:"extra_attrs_lib,omitempty"`
	ExtraAttrsFactor map[int]int     `json:"extra_attrs_factor,omitempty"`
	CreateTime       float64         `json:"create_time,omitempty"`
}

// RuneTemplate 对应 Android rune_mgr.runes_templates 的持久字段。
type RuneTemplate struct {
	UUID     string         `json:"uuid"`
	Name     string         `json:"name"`
	Runes    map[int]string `json:"uuids,omitempty"`
	Time     int64          `json:"time"`
	TopTimes map[int]int64  `json:"top_times,omitempty"`
}

// NewTaskProgress 对应客户端 new_task 类（custom_types/new_task.py）。
type NewTaskProgress struct {
	TaskID          int         `json:"task_id"`
	FinishedTargets map[int]int `json:"finished_targets"`
	Status          int         `json:"status"`
	Expired         bool        `json:"expired"`
	Time            int64       `json:"time"`
}

type ProgressAccounts interface {
	UpdateProgress(context.Context, []byte, func(*Progress) error) (Progress, error)
}

func NewProgress(level int, now time.Time) Progress {
	max := clientBaseline.LevelPower[level].Max
	if max == 0 {
		max = clientBaseline.LevelPower[1].Max
	}
	p := Progress{AvatarLevel: level, AvatarType: 1, FinishedGuides: []int{}, GuideTasks: map[int]GuideTask{}, UnlockSystems: map[string]int{}, UnlockedChapters: map[int]int{},
		GlobalVO: 2, StoryVO: 2, SyncPvpScore: 1000, SyncPvpHighestScore: 1000, SyncPvpSeasonID: 1,
		Materials: map[int]Material{}, MaterialRecords: map[int][]int{}, CheckInRecords: map[int]any{},
		NewTasks:          map[int]NewTaskProgress{},
		RandomCardsRecord: map[int]RandomCardRecord{}, PromiseCardsRecord: map[int]PromiseCardRecord{},
		DungeonSkipEditState: map[int]bool{}, PresetCardsRecord: map[string]PresetRecord{},
		ActivityPresetIDs: map[string]string{},
		ShortMailInfo:     map[int]Mail{}, Runes: map[string]Rune{}, RuneTemplates: map[string]RuneTemplate{},
		SyncPvpSettlements: map[string]SyncPvpReceipt{}, SyncPvpRecords: []SyncPvpRecord{},
		Power: Power{clientBaseline.Init.Power, float64(now.UnixNano()) / 1e9, clientBaseline.Interval, clientBaseline.PerValue, max}}
	ensurePresetState(&p, now)
	// 1000 的无触发条件、task_status=1 的执行态与 preloader.need_guide_task 均有客户端指令证据。
	p.GuideTasks[1000] = GuideTask{1000, 1, now.Unix(), level}
	for _, entry := range clientBaseline.Init.Materials {
		p.Materials[entry[0]] = Material{entry[0], int64(entry[1]), int64(entry[1])}
	}
	// init_config初始卡等级取原表，原生cards_max_level推导品阶（等级1→品阶0）。
	for _, entry := range clientBaseline.Init.Cards {
		cardLevel := 1
		if len(entry) > 1 && entry[1] > 0 {
			cardLevel = entry[1]
		}
		p.Cards = append(p.Cards, newCard(entry[0], cardLevel, now))
	}
	// 新角色按真实初始拥有态建成就目录，避免无关首次RPC补发整目录。
	// 不补造登录/胜利/消费事件；旧空目录仍由成功事务首次事件初始化。
	reconcileAchievementState(&p, level, now)
	return p
}

func CloneProgress(p Progress) Progress {
	raw, _ := json.Marshal(p)
	var clone Progress
	_ = json.Unmarshal(raw, &clone)
	return clone
}

func AdvanceGuide(p *Progress, taskID int, now time.Time, level int) error {
	definition, known := clientBaseline.Guides[taskID]
	current, active := p.GuideTasks[taskID]
	if !known || !active || (current.Status == 0 && !guideTriggerSatisfied(p, definition)) {
		return errors.New("引导任务不存在或未激活")
	}
	if current.Status == 2 {
		return nil
	}
	current.Status = 2
	p.GuideTasks[taskID] = current
	if definition.Next > 0 {
		if _, ok := p.GuideTasks[definition.Next]; !ok {
			next := clientBaseline.Guides[definition.Next]
			status := 0
			if next.Trigger == nil {
				status = 1
			}
			p.GuideTasks[definition.Next] = GuideTask{definition.Next, status, now.Unix(), level}
		}
	}
	ActivateGuideTriggers(p)
	return nil
}

// ActivateGuideTriggers 仅激活已存在且有服务端通关证据的等待任务。
// 结算与登录迁移都调用它，保证在教学中断后重登仍处于执行态。
func ActivateGuideTriggers(p *Progress) bool {
	changed := false
	for id, task := range p.GuideTasks {
		if task.Status == 0 && guideTriggerSatisfied(p, clientBaseline.Guides[id]) {
			task.Status = 1
			p.GuideTasks[id] = task
			changed = true
		}
	}
	return changed
}

// Android guide_task_mgr 在本地 do_task 后才上报完成，不上报中间激活态。
// 等待态任务只能由服务端已有通关证据激活，不能信任上传字典或越级完成。
func guideTriggerSatisfied(p *Progress, definition GuideDefinition) bool {
	if definition.Trigger == nil || *definition.Trigger != 2 {
		return false
	}
	dungeon, ok := definition.TriggerParam.(float64)
	if !ok || dungeon <= 0 || dungeon != float64(int(dungeon)) {
		return false
	}
	for _, cleared := range p.ClearedDungeons {
		if cleared == int(dungeon) {
			return true
		}
	}
	return false
}

// 客户端上报用于诊断校验，完成状态只能由 guide_task_finished 提交；不能凭上报跳过任务。
func ValidateGuideUpload(tasks map[int]GuideTask) error {
	if len(tasks) > len(clientBaseline.Guides) {
		return errors.New("引导状态数量越界")
	}
	for id, task := range tasks {
		if clientBaseline.Guides[id].ID != id || task.ID != id || task.Status < 0 || task.Status > 2 {
			return errors.New("引导状态字段无效")
		}
	}
	return nil
}

// InitialProperties 沿用 CustomDict.load/CustomAttr.load 的字符串键字典输入；数字由 gate 还原 Int。
func (av Avatar) InitialProperties(account string) map[string]any {
	raw, _ := json.Marshal(av.Progress)
	var props map[string]any
	_ = json.Unmarshal(raw, &props)
	if props == nil {
		props = map[string]any{}
	}
	props["signin_info"] = signinProperties(av.Progress, time.Now())
	props["exp_pool"] = av.Progress.Materials[4].Count
	delete(props, "server_played_storylines")
	delete(props, "server_basic_rewards")
	for key, value := range basicRewardProperties(av.Progress) {
		props[key] = value
	}
	props["extra_info_mgr"] = map[string]any{"storyline": storylineProperties(av.Progress)}
	props["short_mail_info"] = mailProperties(av.Progress.ShortMailInfo, time.Now().Unix())
	props["special_gift_card_2_count"] = specialGiftCounts(av.Progress, time.Now())
	delete(props, "server_battle")     // 服务器会话不作为未知客户端属性传入。
	delete(props, "cards")             // 内部模型形态；对外只发 card_mgr 卡字典。
	delete(props, "captain_card_uuid") // 旧内部字段仅用于存档兼容。
	delete(props, "captain_dresses")
	delete(props, "owned_dresses")  // 外观归属通过 card_common_mgr 下发。
	delete(props, "battle_layouts") // 内部布阵通过战斗方法同步，不发送未知Avatar属性。
	delete(props, "runes_templates")
	delete(props, "rune_preset_schema_version")
	delete(props, "rune_preset_quarantine")
	delete(props, "lineup")
	delete(props, "preset_ids")
	delete(props, "preset_cards_record")
	delete(props, "sync_pvp_preset_ids")
	delete(props, "activity_preset_ids")
	delete(props, "intimacy_commons") // 每卡好感度领取状态通过原生common容器投影。
	delete(props, "selected_head_id")
	delete(props, "selected_head_box_id")
	delete(props, "custom_head_cleared")
	delete(props, "obtained_card_ids")
	delete(props, "server_mail_revision")
	delete(props, "server_mail_admin_receipts")
	delete(props, "server_player_admin_receipts")
	delete(props, "server_social")
	delete(props, "server_activities")
	delete(props, "server_collection")
	delete(props, "server_sync_pvp_meta")
	delete(props, "server_async_pvp")
	delete(props, "server_avatar_level")
	delete(props, "remaining_gameplay")
	delete(props, "server_league_id")
	delete(props, "server_last_leave_league")
	delete(props, "server_owned_leagues")
	delete(props, "server_league_protect")
	delete(props, "head_frame_grant_time")
	for key, value := range SocialProperties(av.Progress.Social) {
		props[key] = value
	}
	for key, value := range collectionProperties(av.Progress, time.Now()) {
		props[key] = value
	}
	for key, value := range activityProperties(av.Progress, time.Now()) {
		props[key] = value
	}
	for key, value := range remainingGameplayProperties(av.Progress) {
		props[key] = value
	}
	for key, value := range leagueMembershipProperties(av.Progress) {
		props[key] = value
	}
	for key, value := range leagueProtectProperties(av.Progress) {
		props[key] = value
	}
	for key, value := range profileStatisticsProperties(av.Progress) {
		props[key] = value
	}
	for key, value := range pvpExtraProperties(av.Progress) {
		props[key] = value
	}
	delete(props, "achievement_login_day")
	props["commodity_detail_info"] = commodityProperties(av.Progress, time.Now())
	props["recommend_gifts"] = recommendationProperties(av.Progress)
	props["achves"] = achievementProperties(av.Progress)
	props["achv_value"] = achievementPoints(av.Progress)
	for _, key := range []string{"special_gift_day", "intimacy_special_total", "sync_pvp_match", "sync_pvp_settlements", "sync_pvp_records"} {
		delete(props, key)
	}
	props["captain_id"] = effectiveCaptainID(av.Progress)
	props["uid"], props["hostnum"], props["account"], props["account_id"] = av.UID, av.Hostnum, account, account
	props["nickname"], props["level"], props["head_id"], props["head_box_id"], props["custom_head_image_url"] = av.Info.Nickname, av.Info.Level, av.Info.HeadID, av.Info.HeadBoxID, av.Info.CustomHeadImageURL
	for key, value := range profileCosmeticProperties(av, time.Now()) {
		props[key] = value
	}
	props["cards_count"] = countedCardCount(av.Progress)
	props["show_cards"] = showCardsProperties(av.Progress)
	if av.Progress.GirlRandomEnable != nil {
		props["girl_random_enable"] = *av.Progress.GirlRandomEnable
	}
	if av.Progress.ExploreAutoAgent != nil {
		props["activity_explore_agent"] = av.Progress.ExploreAutoAgent
	}
	gender := av.Gender
	if gender == 0 {
		gender = 1
	}
	props["gender"], props["nickname_flag"] = gender, av.NicknameSet
	if !av.CreatedAt.IsZero() {
		props["create_time"] = av.CreatedAt.Unix()
	}
	ids := []any{}
	for id, task := range av.Progress.GuideTasks {
		if task.Status == 1 {
			ids = append(ids, id)
		}
	}
	props["need_guide_ids"] = ids
	// P0-5：卡牌拥有与 new_task 进度（card_mgr/new_tasks 属性，client save 通道）。
	if len(av.Progress.Cards) > 0 {
		props["card_mgr"] = cardMgrPropertiesWithRunes(av.Progress.Cards, av.Progress.Runes)
		props["card_common_mgr"] = cardCommonMgrPropertiesWithProgress(av.Progress)
	}
	if len(av.Progress.NewTasks) > 0 {
		props["new_tasks"] = newTaskProperties(av.Progress.NewTasks)
	}
	if len(av.Progress.Runes) > 0 {
		props["rune_mgr"] = runeMgrProperties(av.Progress.Runes)
	}
	props["runes_templates"] = runeTemplateProperties(av.Progress)
	for key, value := range presetProperties(av.Progress) {
		props[key] = value
	}
	if len(av.Progress.ClearedDungeons) > 0 || av.Progress.Battle != nil {
		mgr := map[string]any{}
		for _, id := range av.Progress.ClearedDungeons {
			mgr[strconv.Itoa(id)] = map[string]any{"dungeon_id": id, "finished": 1}
		}
		if b := av.Progress.Battle; b != nil && !b.Finished {
			key := strconv.Itoa(b.DungeonID)
			if _, cleared := mgr[key]; !cleared {
				mgr[key] = map[string]any{"dungeon_id": b.DungeonID, "finished": 0}
			}
		}
		props["dungeon_mgr"] = mgr
	}
	return props
}

func (a *FixtureAccounts) UpdateProgress(ctx context.Context, oid []byte, update func(*Progress) error) (Progress, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Progress{}, err
	}
	for account, record := range a.records {
		for i, av := range record.Avatars {
			if string(av.OID) == string(oid) {
				p := CloneProgress(av.Progress)
				p.AvatarLevel = av.Info.Level
				if err := update(&p); err != nil {
					return Progress{}, err
				}
				record.Avatars[i].Progress = p
				record.Avatars[i].Info.Level = p.AvatarLevel
				a.records[account] = record
				return CloneProgress(p), nil
			}
		}
	}
	return Progress{}, errors.New("角色不存在")
}

func (s *Service) updateProgress(ctx context.Context, c *Connection, update func(*Progress) error) error {
	store, ok := s.Accounts.(ProgressAccounts)
	if !ok {
		return errors.New("账号存储不支持玩家进度")
	}
	for i, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			p, err := store.UpdateProgress(ctx, av.OID, func(p *Progress) error {
				mergeOrdinaryObservation(p.Battle, c.ordinaryObservation)
				if p.Battle != nil && !p.Battle.Finished && c.ordinaryObservation != nil && p.Battle.UUID == c.ordinaryObservation.UUID && c.ordinaryPendingAuto != nil {
					prefs := p.battlePreferences()
					prefs.AutoBattle = *c.ordinaryPendingAuto
					p.BattlePreferences = prefs
					p.Battle.AutoBattle = *c.ordinaryPendingAuto
				}
				if err := update(p); err != nil {
					return err
				}
				// 首次真实业务也要创建成就容器并投影当前状态；失败回调已在前面返回，
				// 初始化与资产仍属于同一事务，不补造被删除的旧历史。
				reconcileAchievementState(p, p.AvatarLevel, s.Now())
				reconcileBasicRewardState(p)
				return nil
			})
			if err != nil {
				return err
			}
			c.identity.Avatars[i].Progress = p
			c.ordinaryPendingAuto = nil
			c.identity.Avatars[i].Info.Level = p.AvatarLevel
			return nil
		}
	}
	return errors.New("没有绑定角色")
}

// GuideDefinitionJSON 为文档与验收报告提供同一份内嵌表的可核对入口。
func GuideDefinitionJSON(taskID int) string {
	raw, _ := json.Marshal(clientBaseline.Guides[taskID])
	return strconv.Itoa(taskID) + ":" + string(raw)
}
